package regression

import (
	"net/http"
	"strings"
	"testing"
)

// TestModelLimitChain 把团队、项目、密钥三层名单串成一条链路。
// 空名单是继承，不是拒绝全部；写进团队名单之外的名字会被更新接口拒绝，已经收窄的调用不受影响。
func TestModelLimitChain(t *testing.T) {
	runBoth(t, modelLimitSimulated, modelLimitLive)
}

func modelLimitLive(t *testing.T) {
	h, admin, c, model := openLiveChat(t, "limit-live")
	other := "not-a-vendor-model"
	h.ok(http.MethodPost, "/team/update", admin, map[string]any{
		"team_id": c.teamID, "models": []string{model, other},
	})
	h.ok(http.MethodPost, "/project/update", admin, map[string]any{
		"project_id": c.projectID, "models": []string{model},
	})
	h.assertBilled(t, c, admin, model, "real model is inside the project list", nil)
	refused := h.assertStopped(t, c, other, "name outside the project list")
	if message := errorMessage(refused); !strings.Contains(message, "allowed model") {
		t.Fatalf("the model refusal said %q", message)
	}
}

func modelLimitSimulated(t *testing.T) {
	const (
		modelA = "regression-limit-a"
		modelB = "regression-limit-b"
	)
	h := newHarness(t, chatDeployment(modelA), chatDeployment(modelB))
	admin := h.adminSession()
	c := h.openScope(t, admin, "limit")

	h.ok(http.MethodPost, "/team/update", admin, map[string]any{
		"team_id": c.teamID, "models": []string{modelA, modelB},
	})
	h.ok(http.MethodPost, "/project/update", admin, map[string]any{
		"project_id": c.projectID, "models": []string{modelA},
	})

	h.assertBilled(t, c, admin, modelA, "allowed by the project", []string{modelA})
	refused := h.assertStopped(t, c, modelB, "narrowed out by the project")
	if message := errorMessage(refused); !strings.Contains(message, "allowed model") {
		t.Fatalf("the model refusal said %q, want it to name the allow list", message)
	}
	if got := len(h.successRows(t, admin, modelB)); got != 0 {
		t.Fatalf("the refused model wrote %d success logs", got)
	}

	// 项目名单清空之后继承团队，B 立刻能调，不用换密钥。
	h.ok(http.MethodPost, "/project/update", admin, map[string]any{
		"project_id": c.projectID, "models": []string{},
	})
	h.assertBilled(t, c, admin, modelB, "project list cleared", []string{modelB})

	h.ok(http.MethodPost, "/key/update", admin, map[string]any{
		"key": c.key, "models": []string{modelA},
	})
	h.assertStopped(t, c, modelB, "key list drops B")
	h.assertBilled(t, c, admin, modelA, "key list keeps A", []string{modelA})

	outside := h.do(http.MethodPost, "/project/update", admin, map[string]any{
		"project_id": c.projectID, "models": []string{"not-on-the-team"},
	})
	if outside.status < 300 {
		t.Fatalf("a project accepted a model outside its team: %s", outside.describe())
	}
	h.assertStopped(t, c, modelB, "still narrowed after the rejected write")
	h.assertBilled(t, c, admin, modelA, "A still allowed after the rejected write", []string{modelA})
}
