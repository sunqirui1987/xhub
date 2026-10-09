package regression

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// TestTemplateRoutingUnified 验证完整模板保存、身份继承、两类数据面组回退、计费、无效更新及删除。
// 前置隔离数据库与本地上游；清除绑定后删模板、部署，harness最终销毁schema，无外部服务凭据。
func TestTemplateRoutingUnified(t *testing.T) {
	h := newHarness(t)
	admin := h.adminSession()
	owner := h.openScope(t, admin, "unified-template")
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
		if model == "fail-primary" {
			w.WriteHeader(500)
			writeJSON(w, map[string]any{"error": map[string]any{"code": "api_error"}})
			return
		}
		writeJSON(w, map[string]any{"id": "template-chat", "object": "chat.completion", "model": model, "choices": []any{map[string]any{"index": 0, "message": map[string]any{"role": "assistant", "content": "ok"}, "finish_reason": "stop"}}, "usage": map[string]any{"prompt_tokens": 11, "completion_tokens": 5, "total_tokens": 16}})
	}))
	defer upstream.Close()
	for i, name := range []string{"primary", "backup"} {
		model := "fail-primary"
		if i == 1 {
			model = "good-backup"
		}
		h.ok("POST", "/model/new", admin, map[string]any{"model_name": name, "litellm_params": map[string]any{"model": model, "api_base": upstream.URL, "api_key": "local", "deployment_id": "unified-" + name, "custom_llm_provider": "custom", "input_cost_per_token": 0.000001, "output_cost_per_token": 0.000002}, "model_info": map[string]any{"id": "unified-" + name, "transport": "bypass_openai_chat", "endpoint_types": []string{"chat", "bypass:openai-chat"}, "pricing_source": "manual"}})
	}
	body := routeTemplateBody([]any{map[string]any{"model": "backup", "strategy": "least-busy"}}, 1, 60, 0, 0)
	body["routing_groups"] = []any{map[string]any{"group_name": "chat-group", "models": []string{"primary"}, "routing_strategy": "traffic-split", "routing_strategy_args": map[string]any{"allocations": []any{map[string]any{"deployment_id": "unified-primary", "weight": 1}}}}}
	body["fallbacks"] = []any{map[string]any{"chat-group": []string{"backup"}}}
	body["context_window_fallbacks"] = []any{}
	body["content_policy_fallbacks"] = []any{}
	if r := h.do("POST", "/route_template/new", "", map[string]any{"name": "unauth", "body": body}); r.status != 401 {
		t.Fatalf("未认证创建: %s", r.describe())
	}
	id := routeTemplate(t, h, admin, "complete template", body)
	defer func() {
		for _, scope := range []struct{ kind, id string }{{"key", owner.keyID}, {"team", owner.teamID}, {"organization", owner.orgID}} {
			bindTemplate(t, h, admin, scope.kind, scope.id, "")
		}
		h.ok("POST", "/route_template/"+id+"/delete", admin, map[string]any{})
		for _, name := range []string{"primary", "backup"} {
			h.ok("POST", "/model/delete", admin, map[string]any{"id": "unified-" + name})
		}
	}()
	for _, scope := range []struct{ kind, id string }{{"organization", owner.orgID}, {"team", owner.teamID}, {"key", owner.keyID}} {
		bindTemplate(t, h, admin, scope.kind, scope.id, id)
		if got := effectiveTemplate(t, h, admin, "key", owner.keyID); got["scope_type"] != scope.kind {
			t.Fatalf("完整模板继承: %v", got)
		}
		for _, path := range []string{"/v1/chat/completions", "/bypass/openai/v1/chat/completions"} {
			mu.Lock()
			calls = nil
			mu.Unlock()
			before := h.moneyOf(t, owner)
			h.ok("POST", path, owner.key, chatRequest("chat-group", "unified"+scope.kind+path))
			mu.Lock()
			got := strings.Join(calls, ",")
			mu.Unlock()
			if got != "fail-primary,good-backup" {
				t.Fatalf("%s %s 组回退顺序: %s", scope.kind, path, got)
			}
			h.flushSpend()
			if h.moneyOf(t, owner).key <= before.key {
				t.Fatal("回退实际部署未计费")
			}
		}
	}
	if r := h.do("POST", "/route_template/"+id+"/delete", admin, map[string]any{}); r.status != 409 {
		t.Fatalf("绑定删除保护: %s", r.describe())
	}
	invalid := routeTemplateBody([]any{}, 1, 60, 0, 0)
	invalid["fallbacks"] = []any{map[string]any{"primary": []string{"missing"}}}
	if r := h.do("POST", "/route_template/"+id+"/update", admin, map[string]any{"body": invalid}); r.status != 400 {
		t.Fatalf("悬空引用保存: %s", r.describe())
	}
	// 默认 primary→backup 与模板 backup→primary 合并才形成环，保存边界必须拒绝。
	h.ok("PUT", "/model/fallback", admin, map[string]any{"model_name": "primary", "policy": map[string]any{"fallbacks": []string{"backup"}}})
	invalid["fallbacks"] = []any{map[string]any{"backup": []string{"primary"}}}
	if r := h.do("POST", "/route_template/"+id+"/update", admin, map[string]any{"body": invalid}); r.status != 400 {
		t.Fatalf("合并循环保存: %s", r.describe())
	}
	h.ok("PUT", "/model/fallback", admin, map[string]any{"model_name": "primary", "policy": map[string]any{}})
	got := templateBody(t, h, admin, id)
	raw, _ := json.Marshal(got)
	if !strings.Contains(string(raw), "chat-group") {
		t.Fatalf("失败更新覆盖了正文: %s", raw)
	}
	// 显式空目标只禁用该组通用链，正文更新原子生效，两类数据面均返回原始上游失败。
	body["fallbacks"] = []any{map[string]any{"chat-group": []string{}}}
	h.ok("POST", "/route_template/"+id+"/update", admin, map[string]any{"body": body})
	for _, path := range []string{"/v1/chat/completions", "/bypass/openai/v1/chat/completions"} {
		mu.Lock()
		calls = nil
		mu.Unlock()
		r := h.do("POST", path, owner.key, chatRequest("chat-group", "disabled"))
		if r.status < 400 {
			t.Fatalf("禁用回退仍成功: %s", r.describe())
		}
		mu.Lock()
		got := strings.Join(calls, ",")
		mu.Unlock()
		if got != "fail-primary" {
			t.Fatalf("禁用后仍回退: %s", got)
		}
	}
}
