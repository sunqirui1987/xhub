package regression

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// templateBody 从真实管理接口读配置，不能直接查库，否则漏掉响应丢字段的问题。
func templateBody(t *testing.T, h *harness, admin, id string) map[string]any {
	t.Helper()
	r := h.ok(http.MethodGet, "/route_template/"+id, admin, nil)
	body, ok := r.json()["body"].(map[string]any)
	if !ok {
		t.Fatalf("template has no object body: %s", r.describe())
	}
	return body
}

// assertRouterDocument 先统一 JSON 数字类型，再逐项比较；零、false、空数组和 null 都不能丢。
func assertRouterDocument(t *testing.T, got, want map[string]any) {
	t.Helper()
	var normalized map[string]any
	if err := json.Unmarshal(mustJSON(want), &normalized); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, normalized) {
		t.Fatalf("router document changed:\ngot  %s\nwant %s", mustJSON(got), mustJSON(normalized))
	}
}

// TestRouteTemplateConfigurationRoundTrip 覆盖整个编辑文档，不只挑策略或重试次数。
// 所有控制台字段均要有样例；新增字段却忘了补回归时，这条守卫会报出字段名。
// 回退、别名等字段这里仅证明保存和读取，不能把它当成请求期已实现的证据。
func TestRouteTemplateConfigurationRoundTrip(t *testing.T) {
	h := newHarness(t)
	admin := h.adminSession()
	body := map[string]any{
		"routing_strategy": "weighted-split",
		"routing_strategy_args": map[string]any{"weights": []any{
			map[string]any{"api_base": "https://example.invalid/v1", "model": "openai/primary", "weight": 70},
		}},
		"routing_groups": []any{map[string]any{"group_name": "cheap", "models": []any{"primary"}, "routing_strategy": "lowest-cost"}},
		"num_retries":    3, "timeout": 1.25, "stream_timeout": 2.5,
		"allowed_fails": 2, "cooldown_time": 4.5, "retry_after": 1, "max_fallbacks": 2,
		"fallbacks":                []any{map[string]any{"primary": []any{"backup"}}},
		"context_window_fallbacks": []any{map[string]any{"primary": []any{"long-context"}}},
		"content_policy_fallbacks": []any{map[string]any{"primary": []any{"safe"}}},
		"fallback_causes":          []any{"no_response", "status"},
		"retry_policy":             map[string]any{"RateLimitErrorRetries": 2},
		"model_group_retry_policy": map[string]any{"primary": map[string]any{"RateLimitErrorRetries": 1}},
		"model_group_alias":        map[string]any{"alias": "primary"},
		"enable_pre_call_checks":   true, "enable_tag_filtering": true,
	}
	page := h.ok(http.MethodGet, "/router/settings", admin, nil).json()
	for _, field := range listField(page, "fields") {
		name := stringField(field, "field_name")
		if _, covered := body[name]; !covered {
			t.Fatalf("router field %q has no round-trip regression value", name)
		}
	}
	id := routeTemplate(t, h, admin, "complete configuration", body)
	assertRouterDocument(t, templateBody(t, h, admin, id), body)
	rows := rowsOf(h.ok(http.MethodGet, "/route_template/list", admin, nil), "data")
	listed := findBy(rows, "id", id)
	listedBody, ok := listed["body"].(map[string]any)
	if !ok {
		t.Fatalf("template list has no object body for %s: %v", id, listed)
	}
	assertRouterDocument(t, listedBody, body)

	// 更新整份替换：删掉的高级字段不能从旧正文或平台默认补回来。
	replacement := map[string]any{
		"routing_strategy": "simple-shuffle", "routing_strategy_args": map[string]any{},
		"num_retries": 0, "timeout": 0, "allowed_fails": 0, "cooldown_time": 0,
		"max_fallbacks": 0, "fallbacks": []any{}, "retry_policy": nil, "enable_tag_filtering": false,
	}
	h.ok(http.MethodPost, "/route_template/"+id+"/update", admin, map[string]any{"name": "replaced configuration", "body": replacement})
	assertRouterDocument(t, templateBody(t, h, admin, id), replacement)
}

// TestRouteTemplateSeedsOnceFromPlatformDefaults 新建时复制平台值，后续平台编辑不能混入模板。
func TestRouteTemplateSeedsOnceFromPlatformDefaults(t *testing.T) {
	h := newHarness(t, chatDeployment("template-seed"))
	admin := h.adminSession()
	c := h.openScope(t, admin, "template-seed")
	h.ok(http.MethodPost, "/config/update", admin, map[string]any{"router_settings": map[string]any{
		"num_retries": 3, "timeout": 1.5, "allowed_fails": 0,
	}})
	baseline := h.ok(http.MethodGet, "/router/settings", admin, nil).json()["router_settings"].(map[string]any)
	id := routeTemplate(t, h, admin, "seed snapshot", map[string]any{"enable_tag_filtering": false})
	assertRouterDocument(t, templateBody(t, h, admin, id), baseline)
	bindTemplate(t, h, admin, "team", c.teamID, id)
	h.ok(http.MethodPost, "/config/update", admin, map[string]any{"router_settings": map[string]any{"num_retries": 1, "timeout": 5}})
	assertRouterDocument(t, templateBody(t, h, admin, id), baseline)
	h.scriptStatus("template-seed", http.StatusInternalServerError)
	r := h.do(http.MethodPost, "/v1/chat/completions", c.key, chatRequest("template-seed", "template keeps the snapshot"))
	if r.status != http.StatusBadGateway || h.upstreamAttempts("template-seed") != 3 {
		t.Fatalf("platform edit leaked into selected template: %s, attempts=%d", r.describe(), h.upstreamAttempts("template-seed"))
	}
	bindTemplate(t, h, admin, "team", c.teamID, "")
	h.resetUpstream()
	r = h.do(http.MethodPost, "/v1/chat/completions", c.key, chatRequest("template-seed", "inherit the edited platform"))
	if r.status != http.StatusBadGateway || h.upstreamAttempts("template-seed") != 1 {
		t.Fatalf("cleared template did not inherit current platform: %s, attempts=%d", r.describe(), h.upstreamAttempts("template-seed"))
	}
}

// TestRouteTemplatePrecedenceUsesOneWholeDocument 策略和重试一起验证，不能把各层字段拼起来。
// 同时核对控制台来源与上游请求一致，清空选择后一级一级恢复继承。
func TestRouteTemplatePrecedenceUsesOneWholeDocument(t *testing.T) {
	const public = "template-precedence"
	h := newHarness(t,
		deployment(public, "openai/template-heavy", map[string]any{"weight": 10, "input_cost_per_token": 0.01}),
		deployment(public, "openai/template-light", map[string]any{"weight": 1, "input_cost_per_token": 0.0000001}),
	)
	admin := h.adminSession()
	c := h.openScope(t, admin, "template-precedence")
	h.ok(http.MethodPost, "/config/update", admin, map[string]any{"router_settings": map[string]any{"num_retries": 1, "allowed_fails": 0}})
	h.scriptStatus("template-heavy", http.StatusInternalServerError)
	h.scriptStatus("template-light", http.StatusInternalServerError)
	org := routeTemplate(t, h, admin, "organization document", map[string]any{"routing_strategy": "lowest-cost", "num_retries": 2})
	team := routeTemplate(t, h, admin, "team document", map[string]any{"routing_strategy": "simple-shuffle", "num_retries": 3})
	key := routeTemplate(t, h, admin, "key document", map[string]any{"routing_strategy": "lowest-cost", "num_retries": 4})
	check := func(label, templateID, source, sourceID, first, second string, attempts int) {
		t.Helper()
		effective := effectiveTemplate(t, h, admin, "key", c.keyID)
		if stringField(effective, "template_id") != templateID || effective["scope_type"] != source || stringField(effective, "scope_id") != sourceID {
			t.Fatalf("%s: wrong effective template: %v", label, effective)
		}
		h.resetUpstream()
		r := h.do(http.MethodPost, "/v1/chat/completions", c.key, chatRequest(public, label))
		want := make([]string, 0, attempts*2)
		for _, model := range []string{first, second} {
			for i := 0; i < attempts; i++ {
				want = append(want, model)
			}
		}
		if r.status != http.StatusBadGateway || !reflect.DeepEqual(h.upstreamSince(0), want) {
			t.Fatalf("%s: %s, calls=%v, want %v", label, r.describe(), h.upstreamSince(0), want)
		}
	}
	check("platform", "", "platform", "", "template-heavy", "template-light", 1)
	bindTemplate(t, h, admin, "organization", c.orgID, org)
	check("organization", org, "organization", c.orgID, "template-light", "template-heavy", 2)
	bindTemplate(t, h, admin, "team", c.teamID, team)
	check("team", team, "team", c.teamID, "template-heavy", "template-light", 3)
	bindTemplate(t, h, admin, "key", c.keyID, key)
	check("key", key, "key", c.keyID, "template-light", "template-heavy", 4)
	bindTemplate(t, h, admin, "key", c.keyID, "")
	check("clear key", team, "team", c.teamID, "template-heavy", "template-light", 3)
	bindTemplate(t, h, admin, "team", c.teamID, "")
	check("clear team", org, "organization", c.orgID, "template-light", "template-heavy", 2)
	bindTemplate(t, h, admin, "organization", c.orgID, "")
	check("clear organization", "", "platform", "", "template-heavy", "template-light", 1)
}

// TestRouteTemplateEditsApplyOnlyToSelectedScopes 修改模板正文后下一次请求就读新值。
func TestRouteTemplateEditsApplyOnlyToSelectedScopes(t *testing.T) {
	h := newHarness(t, chatDeployment("template-edit"))
	admin := h.adminSession()
	a := h.provision(t, admin, "template-edit-a")
	b := h.provision(t, admin, "template-edit-b")
	h.scriptStatus("template-edit", http.StatusInternalServerError)
	id := routeTemplate(t, h, admin, "edited live", map[string]any{"num_retries": 2, "allowed_fails": 0})
	bindTemplate(t, h, admin, "team", a.teamID, id)
	for i, tries := range []int{2, 4} {
		if i > 0 {
			h.ok(http.MethodPost, "/route_template/"+id+"/update", admin, map[string]any{"body": map[string]any{"num_retries": tries, "allowed_fails": 0}})
		}
		for j, tn := range []provisionedTenant{a, b} {
			want := 1
			if j == 0 {
				want = tries
			}
			h.resetUpstream()
			r := h.do(http.MethodPost, "/v1/chat/completions", tn.key, chatRequest("template-edit", fmt.Sprintf("edit %d tenant %d", i, j)))
			if r.status != http.StatusBadGateway || h.upstreamAttempts("template-edit") != want {
				t.Fatalf("edit %d tenant %d: %s, attempts=%d want %d", i, j, r.describe(), h.upstreamAttempts("template-edit"), want)
			}
		}
	}
}

// TestRouteTemplateRetriesAndFailoverBillOnce 模板重试要接入原来的部署回退与计费链。
// 500/429 重试耗尽才换部署；400 直接透传且不计费。模型级 fallbacks 当前仅存储。
func TestRouteTemplateRetriesAndFailoverBillOnce(t *testing.T) {
	const public = "template-failover"
	h := newHarness(t,
		deployment(public, "openai/template-fail-a", map[string]any{"weight": 10}),
		deployment(public, "openai/template-fail-b", map[string]any{"weight": 1}),
		chatDeployment("template-other-model"),
	)
	admin := h.adminSession()
	c := h.openScope(t, admin, "template-failover")
	id := routeTemplate(t, h, admin, "two tries before failover", map[string]any{
		"num_retries": 2, "allowed_fails": 0,
		"fallbacks": []any{map[string]any{public: []any{"template-other-model"}}},
	})
	bindTemplate(t, h, admin, "team", c.teamID, id)
	for _, status := range []int{http.StatusInternalServerError, http.StatusTooManyRequests} {
		h.scriptStatus("template-fail-a", status)
		h.assertBilled(t, c, admin, public, fmt.Sprintf("template failover after %d", status), []string{"template-fail-a", "template-fail-a", "template-fail-b"})
	}
	h.scriptStatus("template-fail-a", http.StatusBadRequest)
	before := h.moneyOf(t, c)
	h.resetUpstream()
	r := h.do(http.MethodPost, "/v1/chat/completions", c.key, chatRequest(public, "400 is terminal"))
	if r.status != http.StatusBadRequest || strings.Join(h.upstreamSince(0), ",") != "template-fail-a" {
		t.Fatalf("template retried or rewrote a 400: %s, calls=%v", r.describe(), h.upstreamSince(0))
	}
	if after := h.moneyOf(t, c); !after.same(before) {
		t.Fatalf("template charged a 400: before=%+v after=%+v", before, after)
	}
	// 已存的模型级回退不能被误报成可执行的链，失败后当前仍然返回 502。
	h.scriptStatus("template-fail-a", http.StatusInternalServerError)
	h.scriptStatus("template-fail-b", http.StatusInternalServerError)
	h.resetUpstream()
	r = h.do(http.MethodPost, "/v1/chat/completions", c.key, chatRequest(public, "model fallback is only stored"))
	if r.status != http.StatusBadGateway || strings.Join(h.upstreamSince(0), ",") != "template-fail-a,template-fail-a,template-fail-b,template-fail-b" {
		t.Fatalf("stored model fallback changed current behavior: %s, calls=%v", r.describe(), h.upstreamSince(0))
	}
	if after := h.moneyOf(t, c); !after.same(before) {
		t.Fatalf("template charged failed attempts: before=%+v after=%+v", before, after)
	}
}

// TestRouteTemplateWeightsDriveTraffic 验证编辑器列表和手写映射两种权重形状。
// 部署自身是 9:1，模板改成 3:7；只测模板字段存下了会漏掉数据面仍用部署权重。
func TestRouteTemplateWeightsDriveTraffic(t *testing.T) {
	for _, shape := range []string{"list", "map"} {
		t.Run(shape, func(t *testing.T) {
			const public = "template-split"
			h := newHarness(t, splitDeployment(public, "openai/template-split-a", 9), splitDeployment(public, "openai/template-split-b", 1))
			admin := h.adminSession()
			c := h.openScope(t, admin, "template-split")
			base := h.prices.URL + "/v1"
			var weights any = map[string]any{base + "|openai/template-split-a": 3, base + "|openai/template-split-b": 7}
			if shape == "list" {
				weights = []any{
					map[string]any{"api_base": base, "model": "openai/template-split-a", "weight": 3},
					map[string]any{"api_base": base, "model": "openai/template-split-b", "weight": 7},
				}
			}
			id := routeTemplate(t, h, admin, "custom shares", map[string]any{"routing_strategy": "weighted-split", "routing_strategy_args": map[string]any{"weights": weights}})
			bindTemplate(t, h, admin, "team", c.teamID, id)
			before := h.moneyOf(t, c)
			for i := 0; i < 10; i++ {
				r := h.ok(http.MethodPost, "/v1/chat/completions", c.key, chatRequest(public, fmt.Sprintf("share %d", i)))
				if cost := parseFloatOrZero(r.header("x-litellm-response-cost")); !nearlyEqual(cost, expectedCost()) {
					t.Fatalf("split call charged %v, want %v", cost, expectedCost())
				}
			}
			if a, b := h.upstreamAttempts("template-split-a"), h.upstreamAttempts("template-split-b"); a != 3 || b != 7 {
				t.Fatalf("template shares produced %d:%d, want 3:7", a, b)
			}
			if after := h.moneyOf(t, c); !after.grewBy(before, 10*expectedCost()) {
				t.Fatalf("template split did not bill all scopes: before=%+v after=%+v", before, after)
			}
			// 清掉模板回到 simple-shuffle，仍取部署自身权重最大的那条。
			bindTemplate(t, h, admin, "team", c.teamID, "")
			h.resetUpstream()
			h.ok(http.MethodPost, "/v1/chat/completions", c.key, chatRequest(public, "back to deployment weights"))
			if got := h.upstreamSince(0); strings.Join(got, ",") != "template-split-a" {
				t.Fatalf("template weights leaked after clearing: %v", got)
			}
		})
	}
}

// TestRouteTemplateTimeoutChangesWithoutRestart 卡住上游，必须由当前请求的超时解除等待。
// 不靠睡眠制造成功路径：收到失败响应后释放上游，抬高模板超时后同一密钥可以成功。
func TestRouteTemplateTimeoutChangesWithoutRestart(t *testing.T) {
	for _, scope := range []string{"platform", "team"} {
		t.Run(scope, func(t *testing.T) {
			h := newHarness(t, chatDeployment("template-timeout"))
			admin := h.adminSession()
			c := h.openScope(t, admin, "template-timeout")
			id := ""
			if scope == "team" {
				id = routeTemplate(t, h, admin, "request timeout", map[string]any{"timeout": 5})
				bindTemplate(t, h, admin, "team", c.teamID, id)
			}
			update := func(seconds float64) {
				t.Helper()
				body := map[string]any{"timeout": seconds, "num_retries": 1, "allowed_fails": 0}
				if id == "" {
					h.ok(http.MethodPost, "/config/update", admin, map[string]any{"router_settings": body})
				} else {
					h.ok(http.MethodPost, "/route_template/"+id+"/update", admin, map[string]any{"body": body})
				}
			}
			update(0.1)
			release := h.holdUpstream("template-timeout")
			defer release()
			result := make(chan reply, 1)
			go func() {
				result <- h.do(http.MethodPost, "/v1/chat/completions", c.key, chatRequest("template-timeout", "must time out"))
			}()
			select {
			case r := <-result:
				if r.status != http.StatusBadGateway || h.upstreamAttempts("template-timeout") != 1 {
					t.Fatalf("timeout: %s, attempts=%d", r.describe(), h.upstreamAttempts("template-timeout"))
				}
			case <-time.After(3 * time.Second):
				release()
				<-result
				t.Fatal("runtime timeout was ignored; blocked upstream did not fail within 3 seconds")
			}
			release()
			if got := h.moneyOf(t, c); !got.same(scopeMoney{}) {
				t.Fatalf("timeout was charged: %+v", got)
			}
			update(5)
			h.assertBilled(t, c, admin, "template-timeout", "timeout edited without restart", []string{"template-timeout"})
		})
	}
}

// TestRouteTemplateCooldownUsesSelectedThresholds Redis 冷却阈值也必须读模板。
// 平台禁用冷却、模板允许一次失败；取消选择不会解除已经进入的共享部署冷却。
func TestRouteTemplateCooldownUsesSelectedThresholds(t *testing.T) {
	if os.Getenv("XHUB_REGRESSION_REDIS_URL") == "" {
		t.Skip("cooldown needs XHUB_REGRESSION_REDIS_URL")
	}
	const public = "template-cooldown"
	h := newHarness(t, deployment(public, "openai/template-cool-a", map[string]any{"weight": 10}), deployment(public, "openai/template-cool-b", map[string]any{"weight": 1}))
	admin := h.adminSession()
	c := h.openScope(t, admin, "template-cooldown")
	h.ok(http.MethodPost, "/config/update", admin, map[string]any{"router_settings": map[string]any{"allowed_fails": 0, "cooldown_time": 120}})
	id := routeTemplate(t, h, admin, "cool on first failure", map[string]any{"allowed_fails": 1, "cooldown_time": 7})
	bindTemplate(t, h, admin, "team", c.teamID, id)
	h.scriptStatus("template-cool-a", http.StatusInternalServerError)
	h.assertBilled(t, c, admin, public, "trip template cooldown", []string{"template-cool-a", "template-cool-b"})
	// 平台是 120 秒，模板是 7 秒；既核对阈值，也核对冷却时长来自同一份模板。
	opt, err := redis.ParseURL(os.Getenv("XHUB_REGRESSION_REDIS_URL"))
	if err != nil {
		t.Fatal(err)
	}
	rdb := redis.NewClient(opt)
	defer rdb.Close()
	ttl, err := rdb.PTTL(t.Context(), "xhub:cooldown:"+h.prices.URL+"/v1|openai/template-cool-a").Result()
	if err != nil || ttl <= 0 || ttl > 7*time.Second {
		t.Fatalf("template cooldown duration=%v err=%v, want a positive TTL <= 7 seconds", ttl, err)
	}
	h.scriptStatus("template-cool-a", 0)
	h.assertBilled(t, c, admin, public, "template skips cooled deployment", []string{"template-cool-b"})
	bindTemplate(t, h, admin, "team", c.teamID, "")
	h.assertBilled(t, c, admin, public, "shared cooldown survives clearing template", []string{"template-cool-b"})
}

// TestRouteTemplateScopeFormsKeepAndClearSelections 控制台使用实体的创建/更新接口，
// 不能只验证专用 binding 接口。省略字段表示保持，null 或空串表示恢复继承。
func TestRouteTemplateScopeFormsKeepAndClearSelections(t *testing.T) {
	h := newHarness(t, chatDeployment("template-forms"))
	admin := h.adminSession()
	c := h.openScope(t, admin, "template-forms")
	id := routeTemplate(t, h, admin, "scope forms", map[string]any{"num_retries": 2, "allowed_fails": 0})
	h.scriptStatus("template-forms", http.StatusInternalServerError)
	for _, tc := range []struct {
		scope, scopeID, method, path, idField, aliasField string
	}{
		{"organization", c.orgID, http.MethodPatch, "/organization/update", "organization_id", "organization_alias"},
		{"team", c.teamID, http.MethodPost, "/team/update", "team_id", "team_alias"},
		{"key", c.keyID, http.MethodPost, "/key/update", "key", "key_alias"},
	} {
		t.Run(tc.scope, func(t *testing.T) {
			// 请求失败归到当前子测试，避免夹具在父测试上调用 FailNow。
			previous := h.t
			h.t = t
			defer func() { h.t = previous }()
			check := func(label, wantID, wantSource string, attempts int) {
				t.Helper()
				selected := h.ok(http.MethodGet, "/route_template/binding?scope="+tc.scope+"&scope_id="+tc.scopeID, admin, nil).json()
				if stringField(selected, "route_template_id") != wantID {
					t.Fatalf("%s selection=%v, want %q", label, selected, wantID)
				}
				if effective := effectiveTemplate(t, h, admin, "key", c.keyID); effective["scope_type"] != wantSource {
					t.Fatalf("%s source=%v, want %s", label, effective, wantSource)
				}
				h.resetUpstream()
				r := h.do(http.MethodPost, "/v1/chat/completions", c.key, chatRequest("template-forms", tc.scope+" "+label))
				if r.status != http.StatusBadGateway || h.upstreamAttempts("template-forms") != attempts {
					t.Fatalf("%s: %s, attempts=%d want %d", label, r.describe(), h.upstreamAttempts("template-forms"), attempts)
				}
			}
			h.ok(tc.method, tc.path, admin, map[string]any{tc.idField: tc.scopeID, "route_template_id": id})
			check("selected", id, tc.scope, 2)
			h.ok(tc.method, tc.path, admin, map[string]any{tc.idField: tc.scopeID, tc.aliasField: "renamed " + tc.scope})
			check("unrelated edit", id, tc.scope, 2)
			h.ok(tc.method, tc.path, admin, map[string]any{tc.idField: tc.scopeID, "route_template_id": nil})
			check("cleared with null", "", "platform", 1)
			h.ok(tc.method, tc.path, admin, map[string]any{tc.idField: tc.scopeID, "route_template_id": id})
			h.ok(tc.method, tc.path, admin, map[string]any{tc.idField: tc.scopeID, "route_template_id": ""})
			check("cleared with empty string", "", "platform", 1)
		})
	}

	// 三种实体的创建接口也必须保留选择；读绑定并真正发请求验证。
	org := h.ok(http.MethodPost, "/organization/new", admin, map[string]any{"organization_alias": "template-on-create", "route_template_id": id})
	orgID := firstString(org.json(), "organization_id", "id")
	team := h.ok(http.MethodPost, "/team/new", admin, map[string]any{"team_alias": "template-on-create", "organization_id": orgID, "route_template_id": id})
	teamID := firstString(team.json(), "team_id", "id")
	user := h.ok(http.MethodPost, "/user/new", admin, map[string]any{
		"user_email": "template-on-create@example.com", "password": "template-on-create-password",
		"user_role": "internal_user", "team_id": teamID, "organization_id": orgID,
	})
	userID := firstString(user.json(), "user_id", "id")
	key := h.ok(http.MethodPost, "/key/generate", admin, map[string]any{"team_id": teamID, "user_id": userID, "route_template_id": id})
	keyID := firstString(key.json(), "token_id", "key_id")
	secret := firstString(key.json(), "key", "token")
	for scope, scopeID := range map[string]string{"organization": orgID, "team": teamID, "key": keyID} {
		selected := h.ok(http.MethodGet, "/route_template/binding?scope="+scope+"&scope_id="+scopeID, admin, nil).json()
		if selected["route_template_id"] != id {
			t.Fatalf("%s creation dropped selection: %v", scope, selected)
		}
	}
	h.resetUpstream()
	r := h.do(http.MethodPost, "/v1/chat/completions", secret, chatRequest("template-forms", "created with selection"))
	if r.status != http.StatusBadGateway || h.upstreamAttempts("template-forms") != 2 {
		t.Fatalf("created key did not use template: %s, attempts=%d", r.describe(), h.upstreamAttempts("template-forms"))
	}
}

// TestRouteTemplateUsageAndDeletionCoverEveryScope 用量计数和删除拒绝必须包括三种范围。
// 删失败后配置仍可读，解除所有引用后才允许删除，之后读取返回 404。
func TestRouteTemplateUsageAndDeletionCoverEveryScope(t *testing.T) {
	h := newHarness(t)
	admin := h.adminSession()
	c := h.openScope(t, admin, "template-usage")
	id := routeTemplate(t, h, admin, "used at every level", map[string]any{"num_retries": 2})
	scopes := map[string]string{"organization": c.orgID, "team": c.teamID, "key": c.keyID}
	for scope, scopeID := range scopes {
		bindTemplate(t, h, admin, scope, scopeID, id)
	}
	checkUsage := func(rows []map[string]any) {
		t.Helper()
		if len(rows) != len(scopes) {
			t.Fatalf("usage count=%d want %d: %v", len(rows), len(scopes), rows)
		}
		for scope, scopeID := range scopes {
			row := findBy(rows, "scope_type", scope)
			if row["scope_id"] != scopeID {
				t.Fatalf("%s usage missing %s: %v", scope, scopeID, rows)
			}
		}
	}
	checkUsage(rowsOf(h.ok(http.MethodGet, "/route_template/"+id+"/usage", admin, nil), "data"))
	listed := findBy(rowsOf(h.ok(http.MethodGet, "/route_template/list", admin, nil), "data"), "id", id)
	if listed["used_by"] != float64(3) {
		t.Fatalf("list usage count=%v want 3", listed["used_by"])
	}
	for _, scope := range []string{"key", "team", "organization"} {
		r := h.do(http.MethodPost, "/route_template/"+id+"/delete", admin, nil)
		err, _ := r.json()["error"].(map[string]any)
		if r.status != http.StatusConflict || err["code"] != "template_in_use" {
			t.Fatalf("referenced template deletion: %s", r.describe())
		}
		checkUsage(listField(err, "used_by"))
		h.ok(http.MethodGet, "/route_template/"+id, admin, nil)
		bindTemplate(t, h, admin, scope, scopes[scope], "")
		delete(scopes, scope)
	}
	h.ok(http.MethodPost, "/route_template/"+id+"/delete", admin, nil)
	if r := h.do(http.MethodGet, "/route_template/"+id, admin, nil); r.status != http.StatusNotFound {
		t.Fatalf("deleted template still readable: %s", r.describe())
	}
}

// TestRouteTemplatePermissionsRejectCrossOrganizationChanges 名单可见性和绑定权限要一起测。
// 成员能选自己的团队/密钥，但不能改平台正文，也不能选另一个组织的模板。
func TestRouteTemplatePermissionsRejectCrossOrganizationChanges(t *testing.T) {
	h := newHarness(t)
	admin := h.adminSession()
	a := h.openScope(t, admin, "template-permissions-a")
	b := h.openScope(t, admin, "template-permissions-b")
	public := routeTemplate(t, h, admin, "published template", map[string]any{"num_retries": 2})
	private := firstString(h.ok(http.MethodPost, "/route_template/new", admin, map[string]any{
		"name": "other organization template", "organization_id": b.orgID, "body": map[string]any{"num_retries": 3},
	}).json(), "id")
	rows := rowsOf(h.ok(http.MethodGet, "/route_template/list", a.session, nil), "data")
	if len(findBy(rows, "id", private)) != 0 || len(findBy(rows, "id", public)) == 0 {
		t.Fatalf("template visibility leaked across organizations: %v", namesOf(rows, "name"))
	}
	for scope, scopeID := range map[string]string{"team": a.teamID, "key": a.keyID} {
		bindTemplate(t, h, a.session, scope, scopeID, public)
		r := h.do(http.MethodPost, "/route_template/binding", a.session, map[string]any{"scope": scope, "scope_id": scopeID, "route_template_id": private})
		if r.status != http.StatusNotFound {
			t.Fatalf("invisible template selection must be 404: %s", r.describe())
		}
		if got := effectiveTemplate(t, h, admin, scope, scopeID); got["template_id"] != public {
			t.Fatalf("refused selection changed %s: %v", scope, got)
		}
	}
	for scope, scopeID := range map[string]string{"organization": b.orgID, "team": b.teamID, "key": b.keyID} {
		r := h.do(http.MethodPost, "/route_template/binding", a.session, map[string]any{"scope": scope, "scope_id": scopeID, "route_template_id": public})
		if r.status != http.StatusNotFound {
			t.Fatalf("foreign %s selection must be 404: %s", scope, r.describe())
		}
		if got := effectiveTemplate(t, h, admin, scope, scopeID); got["scope_type"] != "platform" {
			t.Fatalf("refused selection changed foreign scope: %v", got)
		}
	}
	for _, action := range []string{"update", "delete"} {
		r := h.do(http.MethodPost, "/route_template/"+public+"/"+action, a.session, map[string]any{"body": map[string]any{"num_retries": 9}})
		if r.status != http.StatusForbidden {
			t.Fatalf("member changed platform template via %s: %s", action, r.describe())
		}
	}
	if got := templateBody(t, h, admin, public)["num_retries"]; got != float64(2) {
		t.Fatalf("refused template edit changed retries: %v", got)
	}
}
