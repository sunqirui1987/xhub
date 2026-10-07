package regression

import (
	"net/http"
	"strings"
	"testing"
)

// TestRouterSettingsChain 一条链路覆盖设置的读写和立刻生效：
// 非管理员改不了，调用仍走原策略 → 改成最低成本后下一次就打到便宜部署 →
// 重试次数改成 2 后，失败部署被拨两次才换成功的那条，只记一笔 →
// fallbacks 能存能读，但请求仍打原来的部署池。
func TestRouterSettingsChain(t *testing.T) { runSimulated(t, routerSettingsSimulated) }

func routerSettingsSimulated(t *testing.T) {
	const public = "routed-settings"
	h := newHarness(t,
		deployment(public, "openai/heavy", map[string]any{"weight": 10, "input_cost_per_token": 0.01}),
		deployment(public, "openai/light", map[string]any{"weight": 1, "input_cost_per_token": 0.0000001}),
		deployment("other-model", "openai/other-model", nil),
	)
	admin := h.adminSession()
	c := h.openScope(t, admin, "settings")

	refused := h.setRouter(c.session, map[string]any{"routing_strategy": "lowest-cost"})
	if refused.status < 300 {
		t.Fatalf("a member updated router settings: %s", refused.describe())
	}
	h.assertBilled(t, c, admin, public, "still the heavier deployment", []string{"heavy"})

	if r := h.setRouter(admin, map[string]any{"routing_strategy": "lowest-cost"}); r.status >= 300 {
		t.Fatalf("admin router update: %s", r.text())
	}
	h.assertBilled(t, c, admin, public, "lowest cost takes effect immediately", []string{"light"})

	h.scriptStatus("light", http.StatusInternalServerError)
	if r := h.setRouter(admin, map[string]any{"num_retries": 2}); r.status >= 300 {
		t.Fatalf("retry update: %s", r.text())
	}
	before := h.moneyOf(t, c)
	mark := len(h.upstreamCalls())
	retried := h.ok(http.MethodPost, "/v1/chat/completions", c.key, chatRequest(public, "retry then succeed"))
	if got := h.upstreamSince(mark); strings.Join(got, ",") != "light,light,heavy" {
		t.Fatalf("retries reached %v, want light,light,heavy", got)
	}
	cost := parseFloatOrZero(retried.header("x-litellm-response-cost"))
	after := h.moneyOf(t, c)
	if !after.grewBy(before, cost) {
		t.Fatalf("retries billed more than the successful deployment: before %+v after %+v cost %v", before, after, cost)
	}

	h.scriptStatus("light", 0)
	fallbacks := []any{map[string]any{public: []any{"other-model"}}}
	if r := h.setRouter(admin, map[string]any{"fallbacks": fallbacks}); r.status >= 300 {
		t.Fatalf("fallback update: %s", r.text())
	}
	page := h.ok(http.MethodGet, "/router/settings", admin, nil).json()
	raw := string(mustJSON(page))
	if !strings.Contains(raw, "other-model") {
		t.Fatalf("stored fallbacks were not returned: %s", raw)
	}
	h.assertBilled(t, c, admin, public, "fallbacks are stored but not followed", []string{"light"})
}
