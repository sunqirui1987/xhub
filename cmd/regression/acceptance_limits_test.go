package regression

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// acceptanceDaily 是用户 daily 聚合接口中与计费原子性有关的总计。
// 它保存可观察的金额、真实响应 token 和成功/失败请求数；拒绝新增失败请求，但金额和 token 不变。
type acceptanceDaily struct {
	spend, prompt, completion, total, requests, success, failed float64
}

// acceptanceSnapshot 保存一次限制请求之前的持久化可观察状态。
// 五级金额、事件行和 daily 汇总来自真实 PostgreSQL，upstream 是本地供应商收到的调用数；速率限制自身的 Redis 桶不在快照中，因为拒绝也会合法地占用窗口。
type acceptanceSnapshot struct {
	money        scopeMoney
	daily        acceptanceDaily
	events       int
	upstreamMark int
}

// TestAcceptanceLimitsAndAccountingContract 串起发布验收所需的限制与计费契约。
// 前置条件是 harness 提供真实网关、隔离 PostgreSQL schema、独立 Redis 和本地上游；验证真实响应 token 在事件、daily 和五级金额中一致。预算、RPM、TPM 与模型名单拒绝无外发、不扣费，但保留可关联的零费用失败日志并增加 daily 失败数；放宽后原密钥恢复。
// 测试资源由 harness 的 Cleanup 清理；本函数不依赖外部供应商凭据，也不修改已有租户数据。
func TestAcceptanceLimitsAndAccountingContract(t *testing.T) {
	const (
		modelA = "regression-acceptance-limits"
		modelB = "regression-acceptance-other"
		room   = 1000.0
	)
	h := newHarness(t, chatDeployment(modelA), chatDeployment(modelB))
	// 子测试顺序执行并暂时切换断言上下文；不能复制包含互斥锁的 harness，服务器闭包必须继续使用原对象。
	admin := h.adminSession()
	c := h.openScope(t, admin, "acceptance-limits")
	h.usageOverride(modelA, map[string]any{
		"prompt_tokens": 37, "completion_tokens": 13, "total_tokens": 50,
	})
	h.usageOverride(modelB, map[string]any{
		"prompt_tokens": 19, "completion_tokens": 7, "total_tokens": 26,
	})
	setAcceptanceBudgets(t, h, admin, c, room)

	t.Run("真实响应用量贯穿事件日报和五级金额", func(t *testing.T) {
		previousT := h.t
		h.t = t
		t.Cleanup(func() { h.t = previousT })
		beforeMoney := h.moneyOf(t, c)
		beforeDaily := acceptanceDailyOf(t, h, admin, c.userID)
		beforeEvents := acceptanceEventCount(t, h, admin, modelA)
		mark := len(h.upstreamCalls())

		r := h.ok(http.MethodPost, "/v1/chat/completions", c.key, chatRequest(modelA, "accounting-contract"))
		usage, ok := r.json()["usage"].(map[string]any)
		if !ok {
			t.Fatalf("成功响应缺少 usage: %s", r.describe())
		}
		assertAcceptanceNumber(t, usage, "prompt_tokens", 37)
		assertAcceptanceNumber(t, usage, "completion_tokens", 13)
		assertAcceptanceNumber(t, usage, "total_tokens", 50)
		if got := h.upstreamSince(mark); len(got) != 1 || got[0] != modelA {
			t.Fatalf("上游调用为 %v，期望仅调用 %s 一次", got, modelA)
		}

		cost := parseFloatOrZero(r.header("x-litellm-response-cost"))
		if cost <= 0 {
			t.Fatalf("成功响应金额无效: %s", r.describe())
		}
		callID := r.header("x-litellm-call-id")
		row := findLogByRequestID(h.spendLogs(t, admin), callID)
		if row == nil {
			t.Fatalf("事件列表中找不到 call_id=%q", callID)
		}
		assertAcceptanceNumber(t, row, "prompt_tokens", 37)
		assertAcceptanceNumber(t, row, "completion_tokens", 13)
		assertAcceptanceNumber(t, row, "spend", cost)
		if got := stringField(row, "project_id"); got != c.projectID {
			t.Fatalf("事件 project_id=%q，期望 %q", got, c.projectID)
		}
		if got := acceptanceEventCount(t, h, admin, modelA); got != beforeEvents+1 {
			t.Fatalf("成功事件数从 %d 变为 %d，期望仅新增一条", beforeEvents, got)
		}
		if after := h.moneyOf(t, c); !after.grewBy(beforeMoney, cost) {
			t.Fatalf("五级金额增量为 user=%v key=%v project=%v team=%v org=%v，期望均为 %v",
				after.user-beforeMoney.user, after.key-beforeMoney.key, after.project-beforeMoney.project,
				after.team-beforeMoney.team, after.org-beforeMoney.org, cost)
		}
		afterDaily := acceptanceDailyOf(t, h, admin, c.userID)
		assertAcceptanceDailyDelta(t, beforeDaily, afterDaily, acceptanceDaily{
			spend: cost, prompt: 37, completion: 13, total: 50, requests: 1, success: 1,
		})
	})

	t.Run("五级预算共同到顶时拒绝原子且放宽后恢复", func(t *testing.T) {
		previousT := h.t
		h.t = t
		t.Cleanup(func() { h.t = previousT })
		// 即使断言终止子测试也恢复共享归属链预算，让后续限流场景独立执行。
		t.Cleanup(func() { setAcceptanceBudgets(t, h, admin, c, room) })
		spent := h.moneyOf(t, c)
		setAcceptanceBudgetsAtSpend(t, h, admin, c, spent)
		assertAcceptanceRejected(t, h, admin, c, modelA, "budget-refused", http.StatusTooManyRequests, "User budget has been exceeded")
		setAcceptanceBudgets(t, h, admin, c, room)
		h.assertBilled(t, c, admin, modelA, "budget-restored", []string{modelA})
	})

	t.Run("RPM拒绝不外发不计费且提高窗口后恢复", func(t *testing.T) {
		previousT := h.t
		h.t = t
		t.Cleanup(func() { h.t = previousT })
		rpm := acceptanceKey(t, h, c, "acceptance-rpm")
		h.ok(http.MethodPost, "/key/update", admin, map[string]any{"key": rpm.key, "rpm_limit": 1})
		h.assertBilled(t, rpm, admin, modelA, "rpm-first", []string{modelA})
		assertAcceptanceRejected(t, h, admin, rpm, modelA, "rpm-refused", http.StatusTooManyRequests, "rpm_limit")
		h.ok(http.MethodPost, "/key/update", admin, map[string]any{"key": rpm.key, "rpm_limit": 3})
		h.assertBilled(t, rpm, admin, modelA, "rpm-restored", []string{modelA})
	})

	t.Run("TPM按请求估算拒绝但账单仍取真实响应且放宽后恢复", func(t *testing.T) {
		previousT := h.t
		h.t = t
		t.Cleanup(func() { h.t = previousT })
		tpm := acceptanceKey(t, h, c, "acceptance-tpm")
		h.ok(http.MethodPost, "/key/update", admin, map[string]any{"key": tpm.key, "tpm_limit": 40})
		h.assertBilled(t, tpm, admin, modelA, "tpm-first", []string{modelA})
		assertAcceptanceRejected(t, h, admin, tpm, modelA, "tpm-refused", http.StatusTooManyRequests, "tpm_limit")
		h.ok(http.MethodPost, "/key/update", admin, map[string]any{"key": tpm.key, "tpm_limit": 200})
		h.assertBilled(t, tpm, admin, modelA, "tpm-restored", []string{modelA})
	})

	t.Run("模型白名单拒绝原子且原密钥放宽后恢复", func(t *testing.T) {
		previousT := h.t
		h.t = t
		t.Cleanup(func() { h.t = previousT })
		h.ok(http.MethodPost, "/team/update", admin, map[string]any{
			"team_id": c.teamID, "models": []string{modelA, modelB},
		})
		h.ok(http.MethodPost, "/project/update", admin, map[string]any{
			"project_id": c.projectID, "models": []string{modelA},
		})
		h.ok(http.MethodPost, "/key/update", admin, map[string]any{
			"key": c.key, "models": []string{modelA},
		})
		assertAcceptanceRejected(t, h, admin, c, modelB, "allowlist-refused", http.StatusUnauthorized, "allowed model")
		h.ok(http.MethodPost, "/project/update", admin, map[string]any{
			"project_id": c.projectID, "models": []string{modelA, modelB},
		})
		h.ok(http.MethodPost, "/key/update", admin, map[string]any{
			"key": c.key, "models": []string{modelA, modelB},
		})
		h.assertBilled(t, c, admin, modelB, "allowlist-restored", []string{modelB})
	})
}

// acceptanceKey 在同一用户、项目、团队和组织下签发一把新的限流测试密钥。
// 参数 c 提供完整归属链，alias 是稳定别名；返回的新 chained 只替换密钥明文和 ID，调用方用它隔离 RPM/TPM Redis 桶，其余资源仍由 harness 统一清理。
func acceptanceKey(t *testing.T, h *harness, c chained, alias string) chained {
	t.Helper()
	secret, id := h.issueKey(t, c.session, c.teamID, c.projectID, alias)
	c.key = secret
	c.keySecret = secret
	c.keyID = id
	return c
}

// setAcceptanceBudgets 按父级到子级顺序给五层设置相同的宽松预算。
// 参数 budget 是美元上限；函数无返回值，供初始化和解除预算拒绝时调用，任一真实更新接口失败会终止测试。
func setAcceptanceBudgets(t *testing.T, h *harness, admin string, c chained, budget float64) {
	t.Helper()
	h.setOrgBudget(t, admin, c.orgID, budget)
	h.setTeamBudget(t, admin, c.teamID, budget)
	h.setProjectBudget(t, admin, c.projectID, budget)
	h.setKeyBudget(t, admin, c.key, budget)
	h.setUserBudget(t, admin, c.userID, budget)
}

// setAcceptanceBudgetsAtSpend 把五层预算分别收紧到其当前已花金额。
// 参数 spent 来自 flush 后的真实计数；函数按子级到父级写入，保证父级仍有空间容纳子级上限，随后下一次请求应在上游前被拒绝。
func setAcceptanceBudgetsAtSpend(t *testing.T, h *harness, admin string, c chained, spent scopeMoney) {
	t.Helper()
	h.setUserBudget(t, admin, c.userID, spent.user)
	h.setKeyBudget(t, admin, c.key, spent.key)
	h.setProjectBudget(t, admin, c.projectID, spent.project)
	h.setTeamBudget(t, admin, c.teamID, spent.team)
	h.setOrgBudget(t, admin, c.orgID, spent.org)
}

// acceptanceDailyOf 通过真实 daily 聚合接口读取指定用户的计费总计。
// 参数 admin 用于管理员可见范围，userID 将查询收窄到测试用户；返回金额、token 和请求数，接口失败或 metadata 形状缺失时直接终止测试，无写入副作用。
func acceptanceDailyOf(t *testing.T, h *harness, admin, userID string) acceptanceDaily {
	t.Helper()
	h.flushSpend()
	path := "/user/daily/activity/aggregated?user_id=" + url.QueryEscape(userID)
	body := h.ok(http.MethodGet, path, admin, nil).json()
	meta, ok := body["metadata"].(map[string]any)
	if !ok {
		t.Fatalf("daily 聚合缺少 metadata: %v", body)
	}
	return acceptanceDaily{
		spend:      firstFloat(meta, "total_spend"),
		prompt:     firstFloat(meta, "total_prompt_tokens"),
		completion: firstFloat(meta, "total_completion_tokens"),
		total:      firstFloat(meta, "total_tokens"),
		requests:   firstFloat(meta, "total_api_requests"),
		success:    firstFloat(meta, "total_successful_requests"),
		failed:     firstFloat(meta, "total_failed_requests"),
	}
}

// acceptanceEventCount 读取控制台事件接口并统计指定模型的全部事件行。
// 参数 model 将共享 schema 中的其他模型排除；返回事件数，验证拒绝留下零费用失败事件，读取前 flush 热数据。
func acceptanceEventCount(t *testing.T, h *harness, admin, model string) int {
	t.Helper()
	total := 0
	for _, row := range h.spendLogs(t, admin) {
		if stringField(row, "model") == model {
			total++
		}
	}
	return total
}

// acceptanceState 在限制请求前收集五级金额、daily、事件数与本地上游游标。
// 参数 c 指向当前密钥及其归属链，model 用于事件筛选；返回只读快照，供拒绝原子性断言使用，flush 是唯一副作用。
func acceptanceState(t *testing.T, h *harness, admin string, c chained, model string) acceptanceSnapshot {
	t.Helper()
	return acceptanceSnapshot{
		money:        h.moneyOf(t, c),
		daily:        acceptanceDailyOf(t, h, admin, c.userID),
		events:       acceptanceEventCount(t, h, admin, model),
		upstreamMark: len(h.upstreamCalls()),
	}
}

// assertAcceptanceRejected 验证一次限制拒绝的完整原子性契约。
// 参数给出归属、模型、唯一内容、状态码及错误片段；拒绝不外发、不计费，记录一条零 token 失败事件。
// daily 请求与失败各增加一，成功、token 和金额不变；RPM/TPM 拒绝允许递增速率窗口。
func assertAcceptanceRejected(t *testing.T, h *harness, admin string, c chained, model, content string, status int, messagePart string) {
	t.Helper()
	before := acceptanceState(t, h, admin, c, model)
	r := h.do(http.MethodPost, "/v1/chat/completions", c.key, chatRequest(model, content))
	if r.status != status || !strings.Contains(errorMessage(r), messagePart) {
		t.Fatalf("限制拒绝为 %s，期望状态 %d 且消息包含 %q", r.describe(), status, messagePart)
	}
	if got := h.upstreamSince(before.upstreamMark); len(got) != 0 {
		t.Fatalf("被拒绝请求仍外发到上游: %v", got)
	}
	after := acceptanceState(t, h, admin, c, model)
	if !after.money.same(before.money) {
		t.Fatalf("拒绝后五级金额变化: before=%+v after=%+v", before.money, after.money)
	}
	if after.events != before.events+1 {
		t.Fatalf("拒绝后事件数从 %d 变为 %d", before.events, after.events)
	}
	row := findLogByRequestID(h.spendLogs(t, admin), r.header("x-litellm-call-id"))
	if row == nil || stringField(row, "status") != "error" {
		t.Fatalf("拒绝未记录可关联失败事件: %v", row)
	}
	for _, field := range []string{"spend", "prompt_tokens", "completion_tokens"} {
		assertAcceptanceNumber(t, row, field, 0)
	}
	assertAcceptanceDailyDelta(t, before.daily, after.daily, acceptanceDaily{requests: 1, failed: 1})
}

// same 判断两份 daily 总计在金额浮点误差范围内且 token、请求数完全一致。
// 参数 other 是比较对象；返回 true 表示拒绝没有改变报表，函数无副作用。
func (d acceptanceDaily) same(other acceptanceDaily) bool {
	return nearlyEqual(d.spend, other.spend) && d.prompt == other.prompt &&
		d.completion == other.completion && d.total == other.total && d.requests == other.requests && d.success == other.success && d.failed == other.failed
}

// assertAcceptanceDailyDelta 断言一次成功调用给 daily 带来的精确增量。
// 参数 before/after 是调用前后汇总，want 是响应 token、金额和一次请求；函数无返回值，任一字段不符时给出完整增量帮助定位事件聚合问题。
func assertAcceptanceDailyDelta(t *testing.T, before, after, want acceptanceDaily) {
	t.Helper()
	got := acceptanceDaily{
		spend:      after.spend - before.spend,
		prompt:     after.prompt - before.prompt,
		completion: after.completion - before.completion,
		total:      after.total - before.total,
		requests:   after.requests - before.requests,
		success:    after.success - before.success,
		failed:     after.failed - before.failed,
	}
	if !got.same(want) {
		t.Fatalf("daily 增量为 %+v，期望 %+v", got, want)
	}
}

// assertAcceptanceNumber 断言 JSON 对象中的数字字段符合计费契约。
// 参数 body/field 指定响应或事件字段，want 是期望值；函数无返回值，缺字段与数值不符都会报告，适用于整数 token 和浮点金额。
func assertAcceptanceNumber(t *testing.T, body map[string]any, field string, want float64) {
	t.Helper()
	got, ok := floatField(body, field)
	if !ok || !nearlyEqual(got, want) {
		t.Fatalf("字段 %s=%s，期望 %v", field, fmt.Sprint(body[field]), want)
	}
}
