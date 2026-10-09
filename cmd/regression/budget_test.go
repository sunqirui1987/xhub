package regression

import (
	"net/http"
	"strings"
	"testing"
)

// 个人、密钥、项目、团队、组织的完整额度链路在 budget_chain_test.go。
// 这里留下两件那条链路没有单独展开的事：频率上限不是额度，以及没设上限不是零。

// TestBudgetAccumulatesAcrossCalls 证明上限是拿"租户已经花了多少"来比的，不是拿
// 单次调用金额来比。一个刚好够一次调用的上限，会放行这一次、拒绝下一次，
// 中间没有任何人工操作。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestBudgetAccumulatesAcrossCalls(t *testing.T) {
	h := newHarness(t, chatDeployment("regression-accumulate"))
	admin := h.adminSession()
	tn := h.provision(t, admin, "accumulate")
	h.flushSpend()

	// 上限刚好是一次调用的钱，所以第一次调用就把它顶到线。
	oneCall := expectedCost()
	h.setUserBudget(t, admin, tn.userID, oneCall)

	first := h.ok(http.MethodPost, "/v1/chat/completions", tn.key, map[string]any{
		"model": "regression-accumulate", "messages": []any{map[string]any{"role": "user", "content": "one"}},
	})
	charged := parseFloatOrZero(first.header("x-litellm-response-cost"))
	if !nearlyEqual(charged, oneCall) {
		t.Fatalf("the call was charged %v, want the expected %v", charged, oneCall)
	}

	// 换个提示词，让第二次不可能被缓存答上——否则它会因为错误的原因通过。
	second := h.do(http.MethodPost, "/v1/chat/completions", tn.key, map[string]any{
		"model": "regression-accumulate", "messages": []any{map[string]any{"role": "user", "content": "two"}},
	})
	if second.status != http.StatusTooManyRequests {
		t.Fatalf("the second call was not refused after the ceiling was reached: %s", second.describe())
	}
}

// TestRateLimitIsSeparateFromBudget 证明频率上限拒绝时给的是限流错误，不是额度错误。
// 两者到达调用方时状态码一样，所以只有错误信息能把它们区分开。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestRateLimitIsSeparateFromBudget(t *testing.T) {
	h := newHarness(t, chatDeployment("regression-rate"))
	admin := h.adminSession()
	tn := h.provision(t, admin, "rate")

	key := h.keyWith(t, tn.session, map[string]any{
		"key_alias": "rpm-key", "team_id": tn.teamID, "rpm_limit": 1,
	})
	request := map[string]any{
		"model": "regression-rate", "messages": []any{map[string]any{"role": "user", "content": "rate"}},
	}
	h.ok(http.MethodPost, "/v1/chat/completions", key, request)

	r := h.do(http.MethodPost, "/v1/chat/completions", key, request)
	if r.status != http.StatusTooManyRequests {
		t.Fatalf("the second call under rpm_limit=1 was not refused: %s", r.describe())
	}
	message := errorMessage(r)
	if !strings.Contains(message, "rpm_limit") {
		t.Fatalf("the refusal named %q, want rpm_limit", message)
	}
	if strings.Contains(message, "budget") {
		t.Fatalf("a rate limit was reported as a budget problem: %q", message)
	}
}

// TestUnlimitedScopeIsUnlimited 证明"没设上限"不会被读成"上限为零"。
// 没设额度就是不限量；把它当成 0 的话，这个租户就什么都调不了了。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestUnlimitedScopeIsUnlimited(t *testing.T) {
	h := newHarness(t, chatDeployment("regression-unlimited"))
	admin := h.adminSession()
	tn := h.provision(t, admin, "unlimited")
	h.flushSpend()

	// 真的花了钱，但三层都没装上限。
	h.spendOn(t, tn.tenant, "user", 1000.0)
	h.spendOn(t, tn.tenant, "team", 1000.0)
	h.spendOn(t, tn.tenant, "org", 1000.0)

	r := h.ok(http.MethodPost, "/v1/chat/completions", tn.key, map[string]any{
		"model": "regression-unlimited", "messages": []any{map[string]any{"role": "user", "content": "unlimited"}},
	})
	if got := stringField(r.json(), "id"); got == "" {
		t.Fatalf("a scope with no ceiling was refused: %s", r.describe())
	}
}

// errorMessage 从网关的强类型错误信封里取出 message。
// 参数 r（reply）：要读的响应。返回 string（string）：错误信息；取不到时返回正文原文。
func errorMessage(r reply) string {
	errObj, ok := r.json()["error"].(map[string]any)
	if !ok {
		return r.text()
	}
	return stringField(errObj, "message")
}
