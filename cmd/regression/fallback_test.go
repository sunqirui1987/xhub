package regression

import (
	"net/http"
	"os"
	"strings"
	"testing"
)

// TestFallbackChain 把同一对外名下的部署回退走完：
// 5xx 换下一条且只记成功那笔 → 4xx 直接返回、不再往下试 → 全部失败是 502 且不扣费 →
// 换部署前重新查额度，第二条不会被拨到 → 停用一条就走剩下那条，都停用则 model_disabled →
// 护栏在选部署之前拦住，两条都不拨。
func TestFallbackChain(t *testing.T) { runSimulated(t, fallbackSimulated) }

// fallbackSimulated 在隔离网关中验证部署回退、预算复核和护栏；参数 t 为测试上下文，无返回值。
// 显式成本策略保证候选顺序，关闭模板冷却避免前序失败影响断言；隔离 schema 清理全部数据。
func fallbackSimulated(t *testing.T) {
	const public = "regression-failover"
	h := newHarness(t)
	admin := h.adminSession()
	c := h.openScope(t, admin, "failover")
	h.addDBModel(t, admin, public, "fail-a", "dep-fail-a", map[string]any{"input_cost_per_token": testInputRate / 2})
	h.addDBModel(t, admin, public, "fail-b", "dep-fail-b", nil)
	// 默认权重采用随机顺序，不能据此断言先 fail-a；使用真实模板明确成本排序与冷却设置。
	template := routeTemplate(t, h, admin, "deterministic failover", routeTemplateBody([]any{map[string]any{"model": public, "strategy": "cost-based-routing"}}, 1, 60, 0, 0))
	h.ok(http.MethodPost, "/key/update", admin, map[string]any{"key": c.key, "route_template_id": template})

	h.scriptStatus("fail-a", http.StatusInternalServerError)
	before := h.moneyOf(t, c)
	mark := len(h.upstreamCalls())
	ok := h.ok(http.MethodPost, "/v1/chat/completions", c.key, chatRequest(public, "fail then succeed"))
	if got := h.upstreamSince(mark); strings.Join(got, ",") != "fail-a,fail-b" {
		t.Fatalf("failover reached %v, want fail-a,fail-b", got)
	}
	cost := parseFloatOrZero(ok.header("x-litellm-response-cost"))
	if after := h.moneyOf(t, c); !after.grewBy(before, cost) {
		t.Fatalf("failover billed the failed attempt too: before %+v after %+v cost %v", before, after, cost)
	}

	h.scriptStatus("fail-b", http.StatusBadRequest)
	before = h.moneyOf(t, c)
	mark = len(h.upstreamCalls())
	bad := h.do(http.MethodPost, "/v1/chat/completions", c.key, chatRequest(public, "4xx is terminal"))
	if bad.status != http.StatusBadRequest {
		t.Fatalf("upstream 400 was rewritten to %d: %s", bad.status, bad.describe())
	}
	if got := h.upstreamSince(mark); strings.Join(got, ",") != "fail-a,fail-b" {
		t.Fatalf("a 400 kept going: %v", got)
	}
	if after := h.moneyOf(t, c); !after.same(before) {
		t.Fatalf("a 400 was charged: %+v -> %+v", before, after)
	}

	h.scriptStatus("fail-b", http.StatusInternalServerError)
	before = h.moneyOf(t, c)
	failed := h.do(http.MethodPost, "/v1/chat/completions", c.key, chatRequest(public, "both fail"))
	if failed.status != http.StatusBadGateway {
		t.Fatalf("both deployments failing answered %d, want 502: %s", failed.status, failed.describe())
	}
	if after := h.moneyOf(t, c); !after.same(before) {
		t.Fatalf("a 502 was charged: %+v -> %+v", before, after)
	}
	h.flushSpend()
	logs := h.do(http.MethodGet, "/spend/logs/ui?page=1&page_size=200", admin, nil)
	var errorRow map[string]any
	for _, row := range rowsOf(logs, "data", "logs") {
		if stringField(row, "model") == public && stringField(row, "status") == "error" {
			errorRow = row
		}
	}
	if errorRow == nil {
		t.Fatal("the failed call was not logged")
	}
	if spend, _ := floatField(errorRow, "spend"); spend != 0 {
		t.Fatalf("the failed log was charged %v", spend)
	}

	h.scriptStatus("fail-a", http.StatusInternalServerError)
	h.scriptStatus("fail-b", 0)
	h.onUpstream("fail-a", func() {
		money := h.moneyOf(t, c)
		spent := money.project
		h.setProjectBudget(t, admin, c.projectID, spent)
	})
	mark = len(h.upstreamCalls())
	stopped := h.do(http.MethodPost, "/v1/chat/completions", c.key, chatRequest(public, "budget between deployments"))
	if stopped.status != http.StatusTooManyRequests || !strings.Contains(errorMessage(stopped), "Project") {
		t.Fatalf("the second deployment was not stopped by the project: %s", stopped.describe())
	}
	if got := h.upstreamSince(mark); strings.Join(got, ",") != "fail-a" {
		t.Fatalf("budget recheck still dialed %v", got)
	}
	h.onUpstream("fail-a", nil)

	h.setProjectBudget(t, admin, c.projectID, 1000)
	h.scriptStatus("fail-a", 0)
	h.scriptStatus("fail-b", 0)
	h.ok(http.MethodPost, "/model/disable", admin, map[string]any{"id": "dep-fail-a"})
	h.assertBilled(t, c, admin, public, "disabled deployment is skipped", []string{"fail-b"})
	h.ok(http.MethodPost, "/model/disable", admin, map[string]any{"id": "dep-fail-b"})
	mark = len(h.upstreamCalls())
	disabled := h.do(http.MethodPost, "/v1/chat/completions", c.key, chatRequest(public, "both disabled"))
	if disabled.status != http.StatusBadRequest || !strings.Contains(errorMessage(disabled), "disabled") {
		t.Fatalf("two disabled deployments answered %s", disabled.describe())
	}
	if got := h.upstreamSince(mark); len(got) != 0 {
		t.Fatalf("a disabled model reached the upstream: %v", got)
	}

	h.ok(http.MethodPost, "/model/enable", admin, map[string]any{"id": "dep-fail-b"})
	h.putGuardrail(t, "failover-block", guardrailRow("failover-block", kindBlockedWords, []string{"launch-codes"}, ""))
	mark = len(h.upstreamCalls())
	blocked := h.do(http.MethodPost, "/v1/chat/completions", c.key, chatRequest(public, "the launch-codes"))
	if blocked.status != http.StatusBadRequest {
		t.Fatalf("guardrail answered %d: %s", blocked.status, blocked.describe())
	}
	if got := h.upstreamSince(mark); len(got) != 0 {
		t.Fatalf("a guardrail block still failed over: %v", got)
	}
}

// TestCooldownSkipsADeployment 配了 Redis 时，失败次数到了 allowed_fails 就跳过这条部署。
// 没配 Redis 就跳过这条，不把没跑当成通过。
func TestCooldownSkipsADeployment(t *testing.T) { runSimulated(t, cooldownSimulated) }

func cooldownSimulated(t *testing.T) {
	if os.Getenv("XHUB_REGRESSION_REDIS_URL") == "" {
		t.Skip("cooldown needs XHUB_REGRESSION_REDIS_URL")
	}
	const public = "regression-cooldown"
	h := newHarness(t,
		deployment(public, "cool-a", map[string]any{"input_cost_per_token": 0.0000001}),
		deployment(public, "cool-b", map[string]any{"input_cost_per_token": 0.01}),
	)
	admin := h.adminSession()
	c := h.openScope(t, admin, "cooldown")
	// 成本策略确定先尝试 cool-a，同时保留 cool-b 作为失败后的候选；这样冷却断言不依赖随机首选。
	h.modelRouteTemplate(t, admin, c, "cooldown route", public, "cost-based-routing", nil, 1, 1, 60)
	h.scriptStatus("cool-a", http.StatusInternalServerError)
	h.ok(http.MethodPost, "/v1/chat/completions", c.key, chatRequest(public, "trip the cooldown"))
	h.scriptStatus("cool-a", 0)
	h.resetUpstream()
	h.ok(http.MethodPost, "/v1/chat/completions", c.key, chatRequest(public, "skip the cooled deployment"))
	if got := h.upstreamSince(0); strings.Join(got, ",") != "cool-b" {
		t.Fatalf("cooled deployment was still dialed: %v", got)
	}
}

// addDBModel 从控制台加一条部署，这样它才能被单独停用。配置里的模型停不了。
func (h *harness) addDBModel(t *testing.T, admin, public, upstream, id string, extra map[string]any) {
	t.Helper()
	params := map[string]any{
		"model":                 upstream,
		"api_key":               "sk-fake-upstream",
		"api_base":              h.prices.URL + "/v1",
		"custom_llm_provider":   "openai",
		"input_cost_per_token":  testInputRate,
		"output_cost_per_token": testOutputRate,
	}
	for k, v := range extra {
		params[k] = v
	}
	h.ok(http.MethodPost, "/model/new", admin, map[string]any{
		"model_name":     public,
		"litellm_params": params,
		"model_info":     map[string]any{"transport": "bypass_openai_chat", "endpoint_types": []string{"chat"}, "id": id},
	})
}
