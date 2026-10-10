package regression

import (
	"net/http"
	"testing"
)

// 这一组测几个子系统之间的交互：单看每一个都是对的，但两个凑在一起时可能互相
// 抵消。这类问题在只测单点的套件里几乎抓不到。
//
// 主要是三组：
//
//   - 护栏和额度：被护栏拦下的调用不该扣钱，也不该让额度少一块。
//   - 缓存和额度：缓存命中不该推进额度，否则一个反复问同一个问题的租户会被
//     无谓地拦下。
//   - 审计与真实发生的操作：管理动作要留痕。

// TestGuardrailBlockDoesNotConsumeBudget 证明被护栏拦下的调用不扣额度。
//
// 单测护栏时看到的是"拦住了"，单测额度时看到的是"超了会拒"。两个凑在一起时
// 多了一个问题：一次没真正发生的生成，该不该占额度？答案是不该——否则一个
// 反复触发护栏的调用方会把自己的额度耗光。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestGuardrailBlockDoesNotConsumeBudget(t *testing.T) {
	h := newHarness(t, chatDeployment("regression-guard-budget"))
	admin := h.adminSession()
	tn := h.provision(t, admin, "guard-budget")
	h.putGuardrail(t, "no-secrets", guardrailRow("no-secrets", kindBlockedWords, []string{"swordfish"}, ""))
	h.flushSpend()

	// 连撞几次护栏。
	for i := 0; i < 3; i++ {
		r := h.do(http.MethodPost, "/v1/chat/completions", tn.key, map[string]any{
			"model": "regression-guard-budget",
			"messages": []any{map[string]any{
				"role": "user", "content": "the word is swordfish " + string(rune('a'+i)),
			}},
		})
		if r.status != http.StatusBadRequest {
			t.Fatalf("the guardrail did not block: %s", r.describe())
		}
	}

	h.flushSpend()
	user, err := h.db.GetUser(t.Context(), tn.userID)
	if err != nil {
		t.Fatalf("read user: %v", err)
	}
	if user.Spend != 0 {
		t.Fatalf("three blocked calls moved the user's spend to %v", user.Spend)
	}

	// 额度还完好：把上限设成一个很小的数，一次真实调用仍然能过。
	// 如果被拦下的那几次占了额度，这里就会被拒。
	h.setUserBudget(t, admin, tn.userID, expectedCost())
	h.ok(http.MethodPost, "/v1/chat/completions", tn.key, map[string]any{
		"model": "regression-guard-budget", "messages": []any{map[string]any{"role": "user", "content": "clean prompt"}},
	})
}

// TestCacheHitDoesNotConsumeBudget 证明缓存命中不推进额度。
//
// 缓存命中不花钱，所以也不该占额度。如果它占了，一个反复问同一个问题的调用方
// 会因为一些免费请求被拦下——用户看到的是"我明明没花钱却没额度了"。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestCacheHitDoesNotConsumeBudget(t *testing.T) {
	h := newHarness(t, chatDeployment("regression-cache-budget"))
	admin := h.adminSession()
	tn := h.provision(t, admin, "cache-budget")
	h.flushSpend()

	request := map[string]any{
		"model": "regression-cache-budget", "messages": []any{map[string]any{"role": "user", "content": "same question"}},
	}

	// 第一次：未命中，真花钱。
	first := h.ok(http.MethodPost, "/v1/chat/completions", tn.key, request)
	charged := parseFloatOrZero(first.header("x-litellm-response-cost"))
	if charged <= 0 {
		t.Fatalf("the first call was not priced: %s", first.describe())
	}

	// 再问几次同一个问题，全部命中缓存。
	for i := 0; i < 3; i++ {
		hit := h.ok(http.MethodPost, "/v1/chat/completions", tn.key, request)
		if !isCacheHit(hit) {
			t.Fatalf("call %d was not a cache hit: %s", i+2, hit.describe())
		}
	}

	// 额度只扣了第一次那一次。
	h.flushSpend()
	user, err := h.db.GetUser(t.Context(), tn.userID)
	if err != nil {
		t.Fatalf("read user: %v", err)
	}
	if !nearlyEqual(user.Spend, charged) {
		t.Fatalf("four calls (one miss, three hits) spent %v, want only the miss at %v", user.Spend, charged)
	}
}

// TestManagerActionsAreAudited 证明管理动作会留下审计记录。
//
// 审计是"谁把额度改了"这类问题的唯一答案。它出问题时不会报错，只会让记录少几条，
// 所以只能在事后对比。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestManagerActionsAreAudited(t *testing.T) {
	h := newHarness(t, chatDeployment("regression-audit"))
	admin := h.adminSession()
	tn := h.provision(t, admin, "audit")

	// 做几个会改变状态的管理动作。
	h.setUserBudget(t, admin, tn.userID, 42.0)
	h.ok(http.MethodPost, "/team/update", admin, map[string]any{
		"team_id": tn.teamID, "max_budget": 43.0,
	})

	r := h.do(http.MethodGet, "/audit/logs?page=1&page_size=200", admin, nil)
	if r.status != http.StatusOK {
		t.Fatalf("the audit log is not readable by a platform administrator: %s", r.describe())
	}
	body := r.json()
	rows := rowsOfFrom(body, "logs", "data", "audit_logs")
	if len(rows) == 0 {
		t.Fatalf("no audit records were written for two administrative changes: %s",
			truncate(string(mustJSON(body)), 400))
	}

	// 至少要有记录指向这次操作涉及的对象。字段名在各版本里不一样，所以只要
	// 有任何一条记录提到了这个用户或这个团队就算数。
	found := false
	for _, row := range rows {
		for _, field := range []string{"object_id", "target_id", "user_id", "team_id", "resource_id", "entity_id"} {
			if got := stringField(row, field); got == tn.userID || got == tn.teamID {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("no audit record points at the objects that changed: %s",
			truncate(string(mustJSON(rows)), 500))
	}
}

// TestChangingABudgetTakesEffectImmediately 证明改额度是立刻生效的，不靠重启也不靠
// 缓存过期。
//
// 额度是应急手段：有人跑飞了，运维把它压下去，期待的是"现在就停"。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestChangingABudgetTakesEffectImmediately(t *testing.T) {
	h := newHarness(t, chatDeployment("regression-immediate"))
	admin := h.adminSession()
	tn := h.provision(t, admin, "immediate")
	h.flushSpend()

	request := map[string]any{
		"model": "regression-immediate", "messages": []any{map[string]any{"role": "user", "content": "one"}},
	}

	// 先确认能调。
	h.ok(http.MethodPost, "/v1/chat/completions", tn.key, request)

	// 前置已消费；将上限压到累计消费，下一次立刻被拒；harness 清理账单与账号。
	h.flushSpend()
	user, err := h.gw.Identity().GetUser(t.Context(), tn.userID)
	if err != nil {
		t.Fatal(err)
	}
	h.setUserBudget(t, admin, tn.userID, user.Spend)
	r := h.do(http.MethodPost, "/v1/chat/completions", tn.key, request)
	if r.status != http.StatusTooManyRequests {
		t.Fatalf("a budget set to cumulative spend did not stop the next call: %s", r.describe())
	}

	// 再抬回去，也必须立刻恢复。
	h.setUserBudget(t, admin, tn.userID, 1000)
	h.ok(http.MethodPost, "/v1/chat/completions", tn.key, map[string]any{
		"model": "regression-immediate", "messages": []any{map[string]any{"role": "user", "content": "two"}},
	})
}

// TestBlockingAKeyTakesEffectImmediately 证明停用密钥立刻生效。
//
// 停用是应急手段里最急的那个：密钥泄漏时，多生效一分钟就多一分钟的风险。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestBlockingAKeyTakesEffectImmediately(t *testing.T) {
	h := newHarness(t, chatDeployment("regression-blockkey"))
	admin := h.adminSession()
	a := h.provision(t, admin, "blockkey")

	request := map[string]any{
		"model": "regression-blockkey", "messages": []any{map[string]any{"role": "user", "content": "one"}},
	}
	h.ok(http.MethodPost, "/v1/chat/completions", a.key, request)

	h.ok(http.MethodPost, "/key/block", a.session, map[string]any{"key": a.key})

	// 下一次就调不动，中间不留窗口。
	r := h.do(http.MethodPost, "/v1/chat/completions", a.key, request)
	if r.status < 300 {
		t.Fatalf("a blocked key still worked: %s", r.describe())
	}
}

// TestDeletingATeamStopsItsKeys 证明删掉团队之后，它下面的密钥也跟着失效。
//
// 删团队时如果只删了团队行，密钥会挂在一个已经不存在的归属上继续能用——
// 这是权限系统里最难发现的一种残留。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestDeletingATeamStopsItsKeys(t *testing.T) {
	h := newHarness(t, chatDeployment("regression-delteam"))
	admin := h.adminSession()
	a := h.provision(t, admin, "delteam")

	request := map[string]any{
		"model": "regression-delteam", "messages": []any{map[string]any{"role": "user", "content": "one"}},
	}
	// 删之前能用。
	h.ok(http.MethodPost, "/v1/chat/completions", a.key, request)

	h.ok(http.MethodPost, "/team/delete", admin, map[string]any{"team_id": a.teamID})

	// 删之后这把密钥不该还能调。
	r := h.do(http.MethodPost, "/v1/chat/completions", a.key, request)
	if r.status < 300 {
		t.Fatalf("a key under a deleted team still worked: %s", r.describe())
	}
}

// TestModelRestrictionAndBudgetAreIndependent 证明模型名单和额度是两件独立的事，
// 而且分别报出各自的理由。
//
// 两者都用 401 或 429 拒绝，所以只有错误信息能把它们区分开。混在一起会让运维
// 不知道该去改哪个。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestModelRestrictionAndBudgetAreIndependent(t *testing.T) {
	h := newHarness(t, chatDeployment("regression-allowed"), chatDeployment("regression-denied"))
	admin := h.adminSession()
	tn := h.provision(t, admin, "independent")

	// 一把限定模型的密钥，额度充裕。
	key := h.keyWith(t, tn.session, map[string]any{
		"key_alias": "limited-key", "team_id": tn.teamID, "models": []string{"regression-allowed"},
	})

	// 名单外：报的是模型不在名单里。
	outside := h.do(http.MethodPost, "/v1/chat/completions", key, map[string]any{
		"model": "regression-denied", "messages": []any{map[string]any{"role": "user", "content": "x"}},
	})
	if outside.status != http.StatusUnauthorized {
		t.Fatalf("a model outside the allow-list answered %d: %s", outside.status, outside.describe())
	}
	if message := errorMessage(outside); !containsSubstring(message, "allowed model list") {
		t.Fatalf("the refusal did not name the allow-list: %q", message)
	}

	// 名单内但额度用尽：报的是额度，不是模型。
	h.spendOn(t, tn.tenant, "user", 1000)
	h.setUserBudget(t, admin, tn.userID, 1000)
	overBudget := h.do(http.MethodPost, "/v1/chat/completions", key, map[string]any{
		"model": "regression-allowed", "messages": []any{map[string]any{"role": "user", "content": "y"}},
	})
	if overBudget.status != http.StatusTooManyRequests {
		t.Fatalf("an over-budget call answered %d: %s", overBudget.status, overBudget.describe())
	}
	if message := errorMessage(overBudget); !containsSubstring(message, "budget") {
		t.Fatalf("the refusal did not name the budget: %q", message)
	}
}
