package regression

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// TestModelFallbackLifecycle 验证真实管理员配置、持久化、三类回退、调用权限和账单。
// 前置隔离PostgreSQL、真实网关与本地上游；测试结束清除策略和部署，harness删除隔离schema。
func TestModelFallbackLifecycle(t *testing.T) {
	h := newHarness(t)
	admin := h.adminSession()
	owner := h.openScope(t, admin, "model-fallback")
	var mu sync.Mutex
	var calls []string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		model, _ := body["model"].(string)
		mu.Lock()
		calls = append(calls, model)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if body["disable_fallbacks"] != nil {
			t.Error("网关回退开关泄露到上游")
		}
		if strings.HasPrefix(model, "fail-") {
			code, status := "api_error", 500
			switch model {
			case "fail-context":
				code, status = "context_length_exceeded", 400
			case "fail-content":
				code, status = "content_policy_violation", 400
			case "fail-normal":
				code, status = "invalid_request", 400
			case "fail-rate-limit":
				code, status = "rate_limit_exceeded", 429
			}
			w.WriteHeader(status)
			writeJSON(w, map[string]any{"error": map[string]any{"code": code}})
			return
		}
		writeJSON(w, map[string]any{"id": "chat-fallback", "object": "chat.completion", "model": model, "choices": []any{map[string]any{"index": 0, "message": map[string]any{"role": "assistant", "content": "fallback-ok"}, "finish_reason": "stop"}}, "usage": map[string]any{"prompt_tokens": 11, "completion_tokens": 5, "total_tokens": 16}})
	}))
	defer upstream.Close()
	names := []string{"primary", "backup", "third"}
	for _, name := range names {
		h.ok("POST", "/model/new", admin, map[string]any{"model_name": name, "litellm_params": map[string]any{"model": "good-" + name, "api_base": upstream.URL, "api_key": "sk-local", "custom_llm_provider": "custom", "input_cost_per_token": 0.000001, "output_cost_per_token": 0.000002}, "model_info": map[string]any{"id": "fallback-" + name, "transport": "bypass_openai_chat", "endpoint_types": []string{"chat", "bypass:openai-chat"}, "pricing_source": "manual"}})
	}
	defer func() {
		for _, name := range names {
			h.ok("PUT", "/model/fallback", admin, map[string]any{"model_name": name, "policy": map[string]any{}})
		}
		for _, name := range names {
			h.ok("POST", "/model/delete", admin, map[string]any{"id": "fallback-" + name})
		}
	}()
	policy := map[string]any{"fallbacks": []string{"backup", "third"}, "context_window_fallbacks": []string{"third"}, "content_policy_fallbacks": []string{"backup"}}
	h.ok("PUT", "/model/fallback", admin, map[string]any{"model_name": "primary", "policy": policy})
	groups := listField(h.ok("GET", "/model/groups?search=primary", admin, nil).json(), "data")
	if len(groups) != 1 || len(groups[0]["fallback_policy"].(map[string]any)["fallbacks"].([]any)) != 2 {
		t.Fatalf("回退策略未持久化: %v", groups)
	}
	for _, tc := range []struct {
		credential string
		want       int
	}{{"", 401}, {owner.session, 403}} {
		if r := h.do("PUT", "/model/fallback", tc.credential, map[string]any{"model_name": "primary", "policy": policy}); r.status != tc.want {
			t.Fatalf("配置鉴权: %s", r.describe())
		}
	}
	for _, bad := range []map[string]any{{"fallbacks": []string{"primary"}}, {"fallbacks": []string{"absent"}}, {"fallbacks": []string{"backup", "backup"}}, {"unknown": []string{}}} {
		if r := h.do("PUT", "/model/fallback", admin, map[string]any{"model_name": "primary", "policy": bad}); r.status != 400 {
			t.Fatalf("错误策略被接受: %s", r.describe())
		}
	}
	if r := h.do("PUT", "/model/fallback", admin, map[string]any{"model_name": "backup", "policy": map[string]any{"content_policy_fallbacks": []string{"primary"}}}); r.status != 400 {
		t.Fatalf("跨类别循环未拒绝: %s", r.describe())
	}
	for _, action := range []string{"delete", "update"} {
		body := map[string]any{"id": "fallback-backup", "model_name": "renamed"}
		if r := h.do("POST", "/model/"+action, admin, body); r.status != 409 {
			t.Fatalf("引用中的模型被%s: %s", action, r.describe())
		}
	}
	for _, tc := range []struct {
		model, target string
		status        int
	}{{"fail-server", "good-backup", 200}, {"fail-rate-limit", "good-backup", 200}, {"fail-context", "good-third", 200}, {"fail-content", "good-backup", 200}, {"fail-normal", "", 400}} {
		t.Run(tc.model, func(t *testing.T) {
			h.ok("POST", "/model/update", admin, map[string]any{"id": "fallback-primary", "litellm_params": map[string]any{"model": tc.model}})
			for _, path := range []string{"/v1/chat/completions", "/bypass/openai/v1/chat/completions"} {
				mu.Lock()
				calls = nil
				mu.Unlock()
				before := h.moneyOf(t, owner)
				r := h.do("POST", path, owner.key, chatRequest("primary", tc.model+path))
				if r.status != tc.status {
					t.Fatalf("%s 回退结果: %s", path, r.describe())
				}
				mu.Lock()
				got := append([]string(nil), calls...)
				mu.Unlock()
				if tc.target != "" {
					if len(got) < 2 || got[len(got)-1] != tc.target {
						t.Fatalf("回退目标: %v", got)
					}
					h.flushSpend()
					after := h.moneyOf(t, owner)
					if after.key <= before.key {
						t.Fatal("成功回退未按实际部署计费")
					}
				} else if len(got) != 1 {
					t.Fatalf("普通400触发了回退: %v", got)
				}
			}
		})
	}
	// 专用列表独立于初始通用列表；中间目标5xx不能阻止余下专用目标。
	h.ok("PUT", "/model/fallback", admin, map[string]any{"model_name": "primary", "policy": map[string]any{"context_window_fallbacks": []string{"backup", "third"}}})
	h.ok("POST", "/model/update", admin, map[string]any{"id": "fallback-primary", "litellm_params": map[string]any{"model": "fail-context"}})
	h.ok("POST", "/model/update", admin, map[string]any{"id": "fallback-backup", "litellm_params": map[string]any{"model": "fail-server"}})
	for _, path := range []string{"/v1/chat/completions", "/bypass/openai/v1/chat/completions"} {
		mu.Lock()
		calls = nil
		mu.Unlock()
		h.ok("POST", path, owner.key, chatRequest("primary", "dedicated-chain"+path))
		mu.Lock()
		got := strings.Join(calls, ",")
		mu.Unlock()
		if got != "fail-context,fail-server,good-third" {
			t.Fatalf("专用链中途失败未继续: %s", got)
		}
	}
	h.ok("POST", "/model/update", admin, map[string]any{"id": "fallback-backup", "litellm_params": map[string]any{"model": "good-backup"}})
	h.ok("PUT", "/model/fallback", admin, map[string]any{"model_name": "primary", "policy": policy})
	h.ok("POST", "/model/update", admin, map[string]any{"id": "fallback-primary", "litellm_params": map[string]any{"model": "fail-server"}})
	h.ok("POST", "/key/update", admin, map[string]any{"key": owner.key, "models": []string{"primary"}})
	mu.Lock()
	calls = nil
	mu.Unlock()
	r := h.do("POST", "/v1/chat/completions", owner.key, chatRequest("primary", "denied-target"))
	if r.status != 401 {
		t.Fatalf("目标模型权限绕过: %s", r.describe())
	}
	mu.Lock()
	for _, model := range calls {
		if model == "good-backup" {
			t.Error("无权限目标被发送到上游")
		}
	}
	mu.Unlock()
	h.ok("POST", "/key/update", admin, map[string]any{"key": owner.key, "models": []string{}})
	// 真实数据面分别验证请求禁用、嵌套链、耗尽不扣费和停用目标跳过；每项恢复策略与部署。
	for _, tc := range []struct {
		name                              string
		disabled, nested, exhausted, skip bool
	}{
		{name: "request-disabled", disabled: true},
		{name: "nested-chain", nested: true},
		{name: "exhausted-chain", nested: true, exhausted: true},
		{name: "disabled-target", skip: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			primaryTargets := []string{"backup", "third"}
			if tc.nested {
				primaryTargets = primaryTargets[:1]
			}
			h.ok("PUT", "/model/fallback", admin, map[string]any{"model_name": "primary", "policy": map[string]any{"fallbacks": primaryTargets}})
			h.ok("PUT", "/model/fallback", admin, map[string]any{"model_name": "backup", "policy": map[string]any{"fallbacks": []string{"third"}}})
			if tc.nested {
				h.ok("POST", "/model/update", admin, map[string]any{"id": "fallback-backup", "litellm_params": map[string]any{"model": "fail-server"}})
			}
			if tc.exhausted {
				h.ok("POST", "/model/update", admin, map[string]any{"id": "fallback-third", "litellm_params": map[string]any{"model": "fail-server"}})
			}
			if tc.skip {
				h.ok("POST", "/model/disable", admin, map[string]any{"id": "fallback-backup"})
			}
			for _, path := range []string{"/v1/chat/completions", "/bypass/openai/v1/chat/completions"} {
				mu.Lock()
				calls = nil
				mu.Unlock()
				h.flushSpend()
				before := h.moneyOf(t, owner)
				body := chatRequest("primary", tc.name+path)
				body["disable_fallbacks"] = tc.disabled
				r := h.do("POST", path, owner.key, body)
				wantStatus, wantCalls := 200, "fail-server,fail-server,good-third"
				if tc.skip {
					wantCalls = "fail-server,good-third"
				}
				if tc.disabled {
					wantStatus, wantCalls = 502, "fail-server"
				}
				if tc.exhausted {
					wantStatus, wantCalls = 502, "fail-server,fail-server,fail-server"
				}
				if path != "/v1/chat/completions" && wantStatus == 502 {
					wantStatus = 500
				}
				mu.Lock()
				got := strings.Join(calls, ",")
				mu.Unlock()
				if r.status != wantStatus || got != wantCalls {
					t.Fatalf("%s 状态或调用链错误: %s calls=%s want=%s", path, r.describe(), got, wantCalls)
				}
				h.flushSpend()
				after := h.moneyOf(t, owner)
				if tc.disabled || tc.exhausted {
					if !after.same(before) {
						t.Fatalf("失败回退不应扣费: %+v -> %+v", before, after)
					}
				} else if cost := parseFloatOrZero(r.header("x-litellm-response-cost")); cost <= 0 || !after.grewBy(before, cost) {
					t.Fatalf("应只按成功部署扣费: cost=%v before=%+v after=%+v", cost, before, after)
				}
			}
			h.ok("POST", "/model/enable", admin, map[string]any{"id": "fallback-backup"})
			for _, name := range []string{"backup", "third"} {
				h.ok("POST", "/model/update", admin, map[string]any{"id": "fallback-" + name, "litellm_params": map[string]any{"model": "good-" + name}})
			}
			h.ok("PUT", "/model/fallback", admin, map[string]any{"model_name": "backup", "policy": map[string]any{}})
		})
	}
	h.ok("PUT", "/model/fallback", admin, map[string]any{"model_name": "primary", "policy": map[string]any{}})
	if r := h.do("POST", "/v1/chat/completions", owner.key, chatRequest("primary", "cleared")); r.status != 502 {
		t.Fatalf("清空后仍回退: %s", r.describe())
	}
}

// TestModelFallbackPersistenceBoundaries 验证真实数据库的策略边界、失败写入原子性和部署引用保护。
// 前置隔离网关与34个自建模型；验证三类32/33边界、独立配置、重复清空、源/目标删除改名。
// 不调用外部上游；结束清空策略并删除自建部署，harness最终删除schema，失败路径同样清理。
func TestModelFallbackPersistenceBoundaries(t *testing.T) {
	h := newHarness(t)
	admin := h.adminSession()
	names := []string{"boundary-source"}
	for i := 0; i < 33; i++ {
		names = append(names, fmt.Sprintf("boundary-target-%02d", i))
	}
	for _, name := range names {
		h.addDBModel(t, admin, name, name, "dep-"+name, nil)
	}
	defer func() {
		for _, name := range names {
			h.ok("PUT", "/model/fallback", admin, map[string]any{"model_name": name, "policy": map[string]any{}})
		}
		for _, name := range names {
			h.ok("POST", "/model/delete", admin, map[string]any{"id": "dep-" + name})
		}
	}()
	// read 接收公开模型名，经真实目录接口返回规范JSON，保留目标顺序，无写入。
	// 供边界与原子性断言调用，缺少模型时终止父测试；清理由外层defer执行。
	read := func(name string) string {
		t.Helper()
		rows := listField(h.ok("GET", "/model/groups?search="+name, admin, nil).json(), "data")
		for _, row := range rows {
			if row["model_name"] == name {
				raw, _ := json.Marshal(row["fallback_policy"])
				return string(raw)
			}
		}
		t.Fatalf("公开目录缺失模型%s", name)
		return ""
	}
	independent := names[33]
	h.ok("PUT", "/model/fallback", admin, map[string]any{"model_name": independent, "policy": map[string]any{"fallbacks": []string{names[1]}}})
	independentBefore := read(independent)
	for _, field := range []string{"fallbacks", "context_window_fallbacks", "content_policy_fallbacks"} {
		t.Run(field, func(t *testing.T) {
			accepted := h.ok("PUT", "/model/fallback", admin, map[string]any{"model_name": names[0], "policy": map[string]any{field: names[1:33]}})
			if accepted.json()["model_name"] != names[0] || len(accepted.json()["fallback_policy"].(map[string]any)[field].([]any)) != 32 {
				t.Fatalf("保存响应未返回完整策略: %s", accepted.describe())
			}
			before := read(names[0])
			if r := h.do("PUT", "/model/fallback", admin, map[string]any{"model_name": names[0], "policy": map[string]any{field: names[1:]}}); r.status != 400 || errorMessage(r) == "" {
				t.Fatalf("33目标应返回明确400: %s", r.describe())
			}
			if read(names[0]) != before || read(independent) != independentBefore {
				t.Fatal("失败保存修改了已有或其他模型策略")
			}
		})
	}
	before := read(names[0])
	for _, raw := range []string{`{}`, `null`, `{"model_name":"boundary-source"}`, `{"model_name":"boundary-source","policy":null}`, `{"model_name":"boundary-source","policy":[],"unexpected":true}`, `{"model_name":"boundary-source","policy":{"fallbacks":["boundary-target-00 "]}}`} {
		if r := h.do("PUT", "/model/fallback", admin, json.RawMessage(raw)); r.status != 400 || errorMessage(r) == "" {
			t.Fatalf("非法正文错误契约: %s %s", raw, r.describe())
		}
		if read(names[0]) != before {
			t.Fatal("非法正文破坏持久化策略")
		}
	}
	for _, name := range []string{names[0], names[1]} {
		for _, action := range []string{"delete", "update"} {
			if r := h.do("POST", "/model/"+action, admin, map[string]any{"id": "dep-" + name, "model_name": "renamed"}); r.status != 409 {
				t.Fatalf("策略源或目标应保护%s: %s", action, r.describe())
			}
		}
	}
	// 保留同名部署可以删除其中一个，公开模型仍存在，引用不能被悄悄移除。
	h.addDBModel(t, admin, names[1], "duplicate-target", "duplicate-target-dep", nil)
	h.ok("POST", "/model/delete", admin, map[string]any{"id": "duplicate-target-dep"})
	if read(names[0]) != before {
		t.Fatal("删除同名冗余部署改变策略")
	}
	for i := 0; i < 2; i++ {
		h.ok("PUT", "/model/fallback", admin, map[string]any{"model_name": names[0], "policy": map[string]any{}})
	}
	if read(independent) != independentBefore {
		t.Fatal("清空策略影响了其他模型")
	}
	// 清空应删除配置行，不能仅把旧目标隐藏在目录响应中；重复清空保持幂等。
	stored, err := h.store.ListConfig("model_fallbacks")
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := stored[names[0]]; exists {
		t.Fatal("清空后策略行仍存在")
	}
	// 模拟历史损坏配置：管理和数据面均应显式失败，管理员可覆盖损坏源进行恢复。
	t.Run("corrupted-storage", func(t *testing.T) {
		if err := h.store.PutConfig("model_fallbacks", independent, map[string]any{"fallbacks": "invalid"}); err != nil {
			t.Fatal(err)
		}
		defer h.ok("PUT", "/model/fallback", admin, map[string]any{"model_name": independent, "policy": map[string]any{}})
		if r := h.do("GET", "/model/groups?search="+independent, admin, nil); r.status != 500 || errorMessage(r) == "" {
			t.Fatalf("损坏策略目录应返回明确错误: %s", r.describe())
		}
		if r := h.do("PUT", "/model/fallback", admin, map[string]any{"model_name": names[0], "policy": map[string]any{"fallbacks": []string{names[1]}}}); r.status != 503 {
			t.Fatalf("其他损坏策略不应被忽略: %s", r.describe())
		}
		if settings := h.gw.RouteSettingsFor(nil); settings.Err == nil {
			t.Fatal("数据面配置忽略了损坏策略")
		}
		h.ok("PUT", "/model/fallback", admin, map[string]any{"model_name": independent, "policy": map[string]any{"fallbacks": []string{names[1]}}})
		if read(independent) != independentBefore {
			t.Fatal("覆盖损坏策略后未恢复原策略")
		}
		if settings := h.gw.RouteSettingsFor(nil); settings.Err != nil {
			t.Fatalf("修复后数据面仍拒绝执行: %v", settings.Err)
		}
	})
}

// TestModelFallbackBudgetRecheck 验证跨公开模型回退之前重新核对真实项目预算。
// 前置隔离数据库和本地上游，主模型失败回调将预算收紧；目标不得发出且不得扣费，schema统一清理。
func TestModelFallbackBudgetRecheck(t *testing.T) {
	h := newHarness(t)
	admin := h.adminSession()
	owner := h.openScope(t, admin, "cross-model-budget")
	for _, name := range []string{"budget-primary", "budget-backup"} {
		h.addDBModel(t, admin, name, name, "dep-"+name, nil)
	}
	h.ok("PUT", "/model/fallback", admin, map[string]any{"model_name": "budget-primary", "policy": map[string]any{"fallbacks": []string{"budget-backup"}}})
	defer func() {
		h.onUpstream("budget-primary", nil)
		h.ok("PUT", "/model/fallback", admin, map[string]any{"model_name": "budget-primary", "policy": map[string]any{}})
		for _, name := range []string{"budget-primary", "budget-backup"} {
			h.ok("POST", "/model/delete", admin, map[string]any{"id": "dep-" + name})
		}
	}()
	// 先产生真实消费，避免预算0表示不限额而使测试失去意义。
	h.ok("POST", "/v1/chat/completions", owner.key, chatRequest("budget-backup", "seed-budget-spend"))
	h.flushSpend()
	before := h.moneyOf(t, owner)
	h.scriptStatus("budget-primary", 500)
	h.onUpstream("budget-primary", func() { h.setProjectBudget(t, admin, owner.projectID, before.project) })
	h.resetUpstream()
	result := h.do("POST", "/v1/chat/completions", owner.key, chatRequest("budget-primary", "budget-recheck"))
	if result.status != 429 || !strings.Contains(errorMessage(result), "Project") {
		t.Fatalf("跨模型预算未复核: %s", result.describe())
	}
	if got := strings.Join(h.upstreamSince(0), ","); got != "budget-primary" {
		t.Fatalf("预算耗尽仍发送目标: %s", got)
	}
	h.flushSpend()
	if after := h.moneyOf(t, owner); !after.same(before) {
		t.Fatalf("预算拒绝仍扣费: %+v -> %+v", before, after)
	}
}

// TestModelFallbackTemplatePrecedence 验证真实模板绑定覆盖模型默认，显式空链禁用，移除覆盖实时恢复。
// 前置隔离PostgreSQL、三个模型和本地失败上游；分别执行适配/原生入口，验证调用顺序及持久化不变。
// 合并循环必须在保存时拒绝且保留原模板；defer解除绑定、删除模板、清空策略和部署，harness清理schema。
func TestModelFallbackTemplatePrecedence(t *testing.T) {
	h := newHarness(t)
	admin := h.adminSession()
	owner := h.openScope(t, admin, "template-fallback")
	for _, name := range []string{"template-primary", "template-default", "template-override"} {
		h.addDBModel(t, admin, name, name, "dep-"+name, nil)
		// 本用例执行两个入口，必须显式声明原生端点，不能依赖chat端点的隐式推断。
		h.ok("POST", "/model/update", admin, map[string]any{"id": "dep-" + name, "model_info": map[string]any{"endpoint_types": []string{"chat", "bypass:openai-chat"}}})
	}
	h.scriptStatus("template-primary", 500)
	h.ok("PUT", "/model/fallback", admin, map[string]any{"model_name": "template-primary", "policy": map[string]any{"fallbacks": []string{"template-default"}}})
	body := routeTemplateBody([]any{}, 1, 60, 0, 0)
	body["fallbacks"] = []any{map[string]any{"template-primary": []any{"template-override"}}}
	id := routeTemplate(t, h, admin, "fallback override", body)
	h.ok("POST", "/key/update", admin, map[string]any{"key": owner.key, "route_template_id": id})
	defer func() {
		h.ok("POST", "/key/update", admin, map[string]any{"key": owner.key, "route_template_id": ""})
		h.ok("POST", "/route_template/"+id+"/delete", admin, map[string]any{})
		h.ok("PUT", "/model/fallback", admin, map[string]any{"model_name": "template-primary", "policy": map[string]any{}})
		for _, name := range []string{"template-primary", "template-default", "template-override"} {
			h.ok("POST", "/model/delete", admin, map[string]any{"id": "dep-" + name})
		}
	}()
	for _, tc := range []struct {
		name   string
		local  []any
		target string
		cycle  bool
	}{
		{"override", []any{map[string]any{"template-primary": []any{"template-override"}}}, "template-override", false},
		{"disable", []any{map[string]any{"template-primary": []any{}}}, "", false},
		{"inherit", []any{}, "template-default", false},
		{"merged-cycle", []any{map[string]any{"template-default": []any{"template-primary"}}}, "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body["fallbacks"] = tc.local
			if tc.cycle {
				before := h.ok("GET", "/route_template/"+id, admin, nil)
				result := h.do("POST", "/route_template/"+id+"/update", admin, map[string]any{"body": body})
				if result.status != 400 || !strings.Contains(errorMessage(result), "cycle") {
					t.Fatalf("默认策略合并环未在保存时拒绝: %s", result.describe())
				}
				after := h.ok("GET", "/route_template/"+id, admin, nil)
				oldBody, _ := json.Marshal(before.json()["body"])
				newBody, _ := json.Marshal(after.json()["body"])
				if string(oldBody) != string(newBody) {
					t.Fatal("拒绝合并环后模板正文被修改")
				}
				return
			}
			h.ok("POST", "/route_template/"+id+"/update", admin, map[string]any{"body": body})
			for _, path := range []string{"/v1/chat/completions", "/bypass/openai/v1/chat/completions"} {
				h.resetUpstream()
				before := h.moneyOf(t, owner)
				result := h.do("POST", path, owner.key, chatRequest("template-primary", tc.name+path))
				calls := strings.Join(h.upstreamSince(0), ",")
				if tc.target != "" {
					if result.status != 200 || calls != "template-primary,"+tc.target {
						t.Fatalf("模板覆盖错误: %s calls=%s", result.describe(), calls)
					}
				} else {
					want := 502
					if strings.HasPrefix(path, "/bypass") {
						want = 500
					}
					if result.status != want || calls != "template-primary" {
						t.Fatalf("显式空链未禁用: %s calls=%s", result.describe(), calls)
					}
				}
				h.flushSpend()
				after := h.moneyOf(t, owner)
				if tc.target == "" && !after.same(before) {
					t.Fatal("失败或循环仍扣费")
				}
				if tc.target != "" && after.key <= before.key {
					t.Fatal("成功回退未扣费")
				}
			}
			stored, err := h.store.ListConfig("model_fallbacks")
			if err != nil {
				t.Fatal(err)
			}
			raw, _ := json.Marshal(stored["template-primary"])
			if !strings.Contains(string(raw), "template-default") || strings.Contains(string(raw), "template-override") {
				t.Fatalf("模板修改了模型默认: %s", raw)
			}
		})
	}
}
