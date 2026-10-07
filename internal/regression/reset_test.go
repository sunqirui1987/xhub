package regression

import (
	"net/http"
	"strings"
	"testing"
)

// TestResetSpendChain 密钥花费重置只清密钥这一层。
// 用尽后归零可以再调，历史日志还在；reset_to 仍顶着上限时继续拒绝；
// 项目还到顶时，密钥归零也没用。没有写权限的人改不了别人的花费。
func TestResetSpendChain(t *testing.T) {
	runBoth(t, func(t *testing.T) { resetSpendChain(t, false) }, func(t *testing.T) { resetSpendChain(t, true) })
}

func resetSpendChain(t *testing.T, live bool) {
	var h *harness
	var admin string
	var c chained
	var model string
	if live {
		h, admin, c, model = openLiveChat(t, "reset-live")
	} else {
		model = "regression-reset"
		h = newHarness(t, chatDeployment(model))
		admin = h.adminSession()
		c = h.openScope(t, admin, "reset")
	}
	h.setKeyBudget(t, admin, c.key, 1000)
	h.assertBilled(t, c, admin, model, "fills the key", []string{model})
	spent := h.moneyOf(t, c).key
	h.setKeyBudget(t, admin, c.key, spent)
	h.assertBudgetStop(t, c, admin, model, "key is spent", "Key")

	firstLogs := len(h.successRows(t, admin, model))
	h.ok(http.MethodPost, "/key/"+c.keyID+"/reset_spend", admin, map[string]any{})
	if got := h.moneyOf(t, c).key; got != 0 {
		t.Fatalf("reset left the key spend at %v", got)
	}
	if got := len(h.successRows(t, admin, model)); got != firstLogs {
		t.Fatalf("reset rewrote history: success logs %d -> %d", firstLogs, got)
	}
	audit := h.ok(http.MethodGet, "/audit/logs", admin, nil).json()
	raw := string(mustJSON(audit))
	if !strings.Contains(raw, "key.reset_spend") {
		t.Fatalf("reset was not audited: %s", raw)
	}
	h.assertBilled(t, c, admin, model, "after reset", []string{model})

	// 真实供应商两次的 token 数可能不同。把上限收到这次的实际花费上，
	// reset_to 写成同一个数，等于没放开。
	again := h.moneyOf(t, c).key
	h.setKeyBudget(t, admin, c.key, again)
	h.ok(http.MethodPost, "/key/"+c.keyID+"/reset_spend", admin, map[string]any{"reset_to": again})
	h.assertBudgetStop(t, c, admin, model, "reset_to still at the ceiling", "Key")
	h.ok(http.MethodPost, "/key/"+c.keyID+"/reset_spend", admin, map[string]any{"reset_to": 0})

	projectSpent := h.moneyOf(t, c).project
	h.setProjectBudget(t, admin, c.projectID, projectSpent)
	h.assertBudgetStop(t, c, admin, model, "project still exhausted", "Project")
	h.setProjectBudget(t, admin, c.projectID, 1000)
	h.assertBilled(t, c, admin, model, "project ceiling raised", []string{model})

	stranger := h.provision(t, admin, "reset-stranger")
	keyBefore := h.moneyOf(t, c).key
	denied := h.do(http.MethodPost, "/key/"+c.keyID+"/reset_spend", stranger.session, map[string]any{})
	if denied.status < 300 {
		t.Fatalf("a stranger reset the key: %s", denied.describe())
	}
	if got := h.moneyOf(t, c).key; !nearlyEqual(got, keyBefore) {
		t.Fatalf("the denied reset moved key spend from %v to %v", keyBefore, got)
	}
}

// TestPasswordResetChain 重置密码让旧会话失效，新密码登录后可以继续推理。密钥本身还在。
func TestPasswordResetChain(t *testing.T) {
	runBoth(t, func(t *testing.T) { passwordResetChain(t, false) }, func(t *testing.T) { passwordResetChain(t, true) })
}

func passwordResetChain(t *testing.T, live bool) {
	var h *harness
	var admin string
	var c chained
	var model string
	if live {
		h, admin, c, model = openLiveChat(t, "password-live")
	} else {
		model = "regression-password"
		h = newHarness(t, chatDeployment(model))
		admin = h.adminSession()
		c = h.openScope(t, admin, "password")
	}
	h.assertBilled(t, c, admin, model, "before the reset", []string{model})

	const next = "password-reset-2"
	h.ok(http.MethodPost, "/user/set_password", admin, map[string]any{
		"user_id": c.userID, "password": next,
	})
	old := h.do(http.MethodGet, "/auth/me", c.session, nil)
	if old.status < 300 {
		t.Fatalf("the old session survived the password reset: %s", old.describe())
	}
	c.session = h.login(c.email, next)
	c.password = next
	h.assertBilled(t, c, admin, model, "after signing in again", []string{model})
}
