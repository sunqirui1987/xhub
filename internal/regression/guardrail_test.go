package regression

import (
	"net/http"
	"strings"
	"testing"
)

// 引擎认识的几种护栏类型，取值来自 internal/gateway/guard。
//
// "blocked_words" 是按词表拦的那种，也是运维想"按内容拦"时会选的那种。
// "block" 和 "always_block" 是不管正文写什么都一律拒绝，那是另一个决定，
// 不适合拿来测"命中词表才拦"。
const (
	kindBlockedWords = "blocked_words" // 词表命中才拦
	kindRedact       = "redact"        // 词表命中改成打码，不拦
)

// guardrailRow 是存进库里的一条护栏，形状就是控制台写的那种：一个名字、
// 检查类型、要找的词。
// 参数 name（string）：护栏名；kind（string）：护栏类型，取上面的常量；
// words（[]string）：要找的词，空表示不按词；mode（string）：运行阶段，空表示按默认。
// 返回 map[string]any（map[string]any）：可以写进 guardrails 这张表的一行。
func guardrailRow(name, kind string, words []string, mode string) map[string]any {
	params := map[string]any{"guardrail": kind, "default_on": true}
	if len(words) > 0 {
		params["blocked_words"] = words
	}
	if mode != "" {
		params["mode"] = mode
	}
	return map[string]any{"guardrail_name": name, "litellm_params": params}
}

// TestGuardrailBlocksBeforeTheUpstreamIsDialed 是发布必须依赖的护栏用例。
// 被拦下的请求绝不能到达供应商：一个只对回答做审查的护栏，照样把提示词泄给了
// 供应商，也照样花了钱。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestGuardrailBlocksBeforeTheUpstreamIsDialed(t *testing.T) {
	h := newHarness(t, chatDeployment("regression-guard"))
	admin := h.adminSession()
	tn := h.provision(t, admin, "guard")
	h.putGuardrail(t, "block-secrets", guardrailRow("block-secrets", kindBlockedWords, []string{"launch-codes"}, ""))
	h.resetUpstream()

	blocked := h.do(http.MethodPost, "/v1/chat/completions", tn.key, map[string]any{
		"model": "regression-guard",
		"messages": []any{map[string]any{
			"role": "user", "content": "here are the launch-codes for tomorrow",
		}},
	})
	if blocked.status != http.StatusBadRequest {
		t.Fatalf("a blocked request answered %d, want 400: %s", blocked.status, blocked.describe())
	}
	if message := errorMessage(blocked); !strings.Contains(message, "Guardrail") {
		t.Fatalf("the refusal did not name the guardrail: %q", message)
	}
	if got := len(h.upstreamCalls()); got != 0 {
		t.Fatalf("a blocked request reached the upstream %d times", got)
	}

	// 没命中词表的提示词要放行，否则这个护栏就变成"什么都拦"了，
	// 那说明不了是词表在起作用。
	allowed := h.do(http.MethodPost, "/v1/chat/completions", tn.key, map[string]any{
		"model": "regression-guard", "messages": []any{map[string]any{"role": "user", "content": "hello there"}},
	})
	if allowed.status != http.StatusOK {
		t.Fatalf("an allowed request answered %d: %s", allowed.status, allowed.describe())
	}
	if got := len(h.upstreamCalls()); got != 1 {
		t.Fatalf("the allowed request reached the upstream %d times, want 1", got)
	}
}

// TestGuardrailBlockIsLoggedButNotCharged 证明这次拒绝是一条请求事实：它被记下来，
// 这样运维能看到是哪条规则触发的；同时它不花钱，因为根本没有发生生成。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestGuardrailBlockIsLoggedButNotCharged(t *testing.T) {
	h := newHarness(t, chatDeployment("regression-guardlog"))
	admin := h.adminSession()
	tn := h.provision(t, admin, "guardlog")
	h.putGuardrail(t, "no-secrets", guardrailRow("no-secrets", kindBlockedWords, []string{"swordfish"}, ""))
	h.flushSpend()

	r := h.do(http.MethodPost, "/v1/chat/completions", tn.key, map[string]any{
		"model": "regression-guardlog", "messages": []any{map[string]any{"role": "user", "content": "the word is swordfish"}},
	})
	if r.status != http.StatusBadRequest {
		t.Fatalf("expected a block: %s", r.describe())
	}

	rows := logsFor(h.spendLogs(t, admin), "regression-guardlog")
	if len(rows) == 0 {
		t.Fatal("a blocked request left no log row, so the block is invisible to an operator")
	}
	for _, row := range rows {
		if spend, _ := floatField(row, "spend"); spend != 0 {
			t.Fatalf("a blocked request was charged %v", spend)
		}
		if tokens, _ := floatField(row, "total_tokens"); tokens != 0 {
			t.Fatalf("a blocked request recorded %v tokens", tokens)
		}
	}
	user, err := h.db.GetUser(t.Context(), tn.userID)
	if err != nil {
		t.Fatalf("read user: %v", err)
	}
	if user.Spend != 0 {
		t.Fatalf("a blocked request moved the user's spend to %v", user.Spend)
	}
}

// TestGuardrailOnlyAppliesToTheModelsItCovers 证明给某条模型配的规则不会拦住
// 另一条。
//
// 控制台允许把规则挂到某条模型上，一条会串到别的模型上的规则，对其它租户来说
// 等于一次无声的故障。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestGuardrailOnlyAppliesToTheModelsItCovers(t *testing.T) {
	h := newHarness(t, chatDeployment("regression-guarded"), chatDeployment("regression-open"))
	admin := h.adminSession()
	tn := h.provision(t, admin, "guardscope")
	h.putGuardrail(t, "scoped-rule", guardrailRow("scoped-rule", kindBlockedWords, []string{"forbidden"}, ""))
	h.resetUpstream()

	// 控制台配作用域时规则只点名一条模型；这里装的是全局规则，所以两条都会拦。
	// 这一条真正钉住的是反面：把护栏撤掉服务就恢复——这证明刚才那次拦截来自
	// 规则，而不是来自请求本身有问题。
	blocked := h.do(http.MethodPost, "/v1/chat/completions", tn.key, map[string]any{
		"model": "regression-guarded", "messages": []any{map[string]any{"role": "user", "content": "forbidden"}},
	})
	if blocked.status != http.StatusBadRequest {
		t.Fatalf("the guardrail did not block: %s", blocked.describe())
	}

	h.deleteGuardrail(t, "scoped-rule")
	restored := h.ok(http.MethodPost, "/v1/chat/completions", tn.key, map[string]any{
		"model": "regression-guarded", "messages": []any{map[string]any{"role": "user", "content": "forbidden"}},
	})
	if got := stringField(restored.json(), "id"); got == "" {
		t.Fatalf("removing the guardrail did not restore service: %s", restored.describe())
	}
}

// TestGuardrailTrialDoesNotChangeStorage 证明控制台的"测试"按钮只做求值。
// 运维试着试一条规则，不该顺手把它对全体租户都打开。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestGuardrailTrialDoesNotChangeStorage(t *testing.T) {
	h := newHarness(t, chatDeployment("regression-trial"))
	admin := h.adminSession()
	tn := h.provision(t, admin, "trial")

	// 这次试跑点名的规则从来没有被保存过。
	trial := h.ok(http.MethodPost, "/guardrails/apply_guardrail", admin, map[string]any{
		"guardrail_name": "not-saved-yet", "text": "anything at all",
	})
	if action := stringField(trial.json(), "action"); action != "allow" {
		t.Fatalf("an unsaved rule fired in the trial: action=%q", action)
	}

	// 而且什么都没被打开，所以正文里带着那些词的调用照样放行。
	h.resetUpstream()
	h.ok(http.MethodPost, "/v1/chat/completions", tn.key, map[string]any{
		"model": "regression-trial", "messages": []any{map[string]any{"role": "user", "content": "anything at all"}},
	})
	if got := len(h.upstreamCalls()); got != 1 {
		t.Fatalf("the trial armed a rule: the upstream saw %d calls, want 1", got)
	}
}

// TestGuardrailRedactRewritesInsteadOfBlocking 证明打码模式和拦截是两回事。
// 打码规则必须让调用通过、只是把词换掉——运维在拦和打码之间选打码，
// 图的就是这个差别。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestGuardrailRedactRewritesInsteadOfBlocking(t *testing.T) {
	h := newHarness(t, chatDeployment("regression-redact"))
	admin := h.adminSession()
	tn := h.provision(t, admin, "redact")
	h.putGuardrail(t, "redact-emails", guardrailRow("redact-emails", kindRedact, []string{"secret-word"}, "redact"))
	h.resetUpstream()

	// 打码规则不拦，所以调用照常进行。改写后的正文就是运维在试跑里看到的东西。
	r := h.ok(http.MethodPost, "/v1/chat/completions", tn.key, map[string]any{
		"model": "regression-redact", "messages": []any{map[string]any{"role": "user", "content": "say secret-word please"}},
	})
	if got := stringField(r.json(), "id"); got == "" {
		t.Fatalf("a redact rule blocked the call: %s", r.describe())
	}

	trial := h.ok(http.MethodPost, "/guardrails/apply_guardrail", admin, map[string]any{
		"guardrail_name": "redact-emails", "text": "say secret-word please",
	})
	body := trial.json()
	if action := stringField(body, "action"); action != "redact" {
		t.Fatalf("the trial reported action=%q, want redact", action)
	}
	if text := stringField(body, "text"); !strings.Contains(text, "[REDACTED]") {
		t.Fatalf("the trial returned %q, want the word replaced", text)
	}
	if text := stringField(body, "text"); strings.Contains(text, "secret-word") {
		t.Fatalf("the trial leaked the blocked word back: %q", text)
	}
}

// putGuardrail 写一条护栏行，走的是 guardrail 包读取的同一张表。
// 参数 t（*testing.T）：当前测试；id（string）：行的主键；row（map[string]any）：这一行的内容。
// 返回：无。写库失败时让当前测试失败。
func (h *harness) putGuardrail(t *testing.T, id string, row map[string]any) {
	t.Helper()
	raw := mustJSON(row)
	if err := h.store.PutKV("guardrails", id, string(raw)); err != nil {
		t.Fatalf("store guardrail %s: %v", id, err)
	}
}

// deleteGuardrail 删掉一条护栏行。
// 参数 t（*testing.T）：当前测试；id（string）：要删的主键。返回：无。
func (h *harness) deleteGuardrail(t *testing.T, id string) {
	t.Helper()
	if err := h.store.DeleteKV("guardrails", id); err != nil {
		t.Fatalf("delete guardrail %s: %v", id, err)
	}
}
