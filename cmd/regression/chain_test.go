package regression

import (
	"net/http"
	"strings"
	"testing"
)

// TestRequestChain 用同一个租户把路由、回退、项目额度、密钥重置和模型名单串起来。
// 分开的链路各自绿，凑在一起仍可能互相抵消，所以这里再走一遍主路径。
func TestRequestChain(t *testing.T) { runSimulated(t, requestChainSimulated) }

func requestChainSimulated(t *testing.T) {
	const (
		public = "regression-chain"
		side   = "regression-chain-side"
	)
	h := newHarness(t,
		deployment(public, "openai/chain-cheap", map[string]any{"weight": 1, "input_cost_per_token": 0.0000001}),
		deployment(public, "openai/chain-dear", map[string]any{"weight": 1, "input_cost_per_token": 0.01}),
		chatDeployment(side),
	)
	admin := h.adminSession()
	c := h.openScope(t, admin, "chain")
	const ceiling = 1000.0
	h.setUserBudget(t, admin, c.userID, ceiling)
	h.setKeyBudget(t, admin, c.key, ceiling)
	h.setProjectBudget(t, admin, c.projectID, ceiling)
	h.setTeamBudget(t, admin, c.teamID, ceiling)
	h.setOrgBudget(t, admin, c.orgID, ceiling)
	h.modelRouteTemplate(t, admin, c, "chain cost route", public, "cost-based-routing", nil, 1, 0, 0)

	h.assertBilled(t, c, admin, public, "cheap deployment", []string{"chain-cheap"})

	h.scriptStatus("chain-cheap", http.StatusInternalServerError)
	before := h.moneyOf(t, c)
	mark := len(h.upstreamCalls())
	fell := h.ok(http.MethodPost, "/v1/chat/completions", c.key, chatRequest(public, "fall back to the dear one"))
	if got := h.upstreamSince(mark); strings.Join(got, ",") != "chain-cheap,chain-dear" {
		t.Fatalf("fallback reached %v", got)
	}
	if after := h.moneyOf(t, c); !after.grewBy(before, parseFloatOrZero(fell.header("x-litellm-response-cost"))) {
		t.Fatalf("fallback billed both deployments: %+v -> %+v", before, after)
	}

	h.scriptStatus("chain-cheap", 0)
	spent := h.moneyOf(t, c).project
	h.setProjectBudget(t, admin, c.projectID, spent)
	h.assertBudgetStop(t, c, admin, public, "project is the reason", "Project")

	h.ok(http.MethodPost, "/key/"+c.keyID+"/reset_spend", admin, map[string]any{})
	h.assertBudgetStop(t, c, admin, public, "resetting the key leaves the project", "Project")
	h.setProjectBudget(t, admin, c.projectID, ceiling)
	h.assertBilled(t, c, admin, public, "project raised", []string{"chain-cheap"})

	h.ok(http.MethodPost, "/team/update", admin, map[string]any{
		"team_id": c.teamID, "models": []string{public, side},
	})
	h.ok(http.MethodPost, "/project/update", admin, map[string]any{
		"project_id": c.projectID, "models": []string{public},
	})
	h.assertStopped(t, c, side, "outside the project list")
	h.assertBilled(t, c, admin, public, "inside the project list", []string{"chain-cheap"})

	rows := h.successRows(t, admin, public)
	if len(rows) < 3 {
		t.Fatalf("the chain logged %d successes, want at least 3", len(rows))
	}
	last := rows[len(rows)-1]
	if stringField(last, "project_id") != c.projectID {
		t.Fatalf("last log project %q, want %s", stringField(last, "project_id"), c.projectID)
	}
}
