package regression

import (
	"net/http"
	"testing"
	"time"
)

// 每一层额度都是同一条链路，只是被设上限的那一层不同：
// 额度内调用成功，五层各记一笔，日志对得上 → 这一层到顶后 429 且点名这一层，
// 不拨上游、不加花费 → 抬高上限后下一次又成功。其它层不设上限，避免点名被抢走。

func TestPersonalBudgetChain(t *testing.T) {
	runBoth(t, func(t *testing.T) { personalBudgetChain(t, false) }, func(t *testing.T) { personalBudgetChain(t, true) })
}

func personalBudgetChain(t *testing.T, live bool) {
	h, admin, c, model := budgetSubject(t, "personal", live)
	runBudgetChain(t, h, admin, c, model, "User", func(ceiling float64) {
		h.setUserBudget(t, admin, c.userID, ceiling)
	})

	// 停用个人会收回他的个人密钥，所以恢复账号之后旧密钥仍然不能用，要重新发一把。
	h.ok(http.MethodPost, "/user/update", admin, map[string]any{"user_id": c.userID, "blocked": true})
	h.assertStopped(t, c, model, "user disabled")
	h.ok(http.MethodPost, "/user/update", admin, map[string]any{"user_id": c.userID, "blocked": false})
	h.assertStopped(t, c, model, "revoked key stays dead")
	// 停用账号会让当时的会话失效，重新发密钥之前要再登录一次。
	c.session = h.login(c.email, c.password)
	secret, keyID := h.issueKey(t, c.session, c.teamID, c.projectID, "personal-new-key")
	c.key, c.keyID = secret, keyID
	h.assertBilled(t, c, admin, model, "new key after user enabled", []string{model})
}

func TestTeamBudgetChain(t *testing.T) {
	runBoth(t, func(t *testing.T) { teamBudgetChain(t, false) }, func(t *testing.T) { teamBudgetChain(t, true) })
}

func teamBudgetChain(t *testing.T, live bool) {
	h, admin, c, model := budgetSubject(t, "team", live)
	runBudgetChain(t, h, admin, c, model, "Team", func(ceiling float64) {
		h.setTeamBudget(t, admin, c.teamID, ceiling)
	})
	h.ok(http.MethodPost, "/team/update", admin, map[string]any{"team_id": c.teamID, "blocked": true})
	stopped := h.assertStopped(t, c, model, "team blocked")
	if stopped.status >= 500 {
		t.Fatalf("a blocked team answered %d: %s", stopped.status, stopped.describe())
	}
	h.ok(http.MethodPost, "/team/update", admin, map[string]any{"team_id": c.teamID, "blocked": false})
	h.assertBilled(t, c, admin, model, "team unblocked", []string{model})
}

func TestProjectBudgetChain(t *testing.T) {
	runBoth(t, func(t *testing.T) { projectBudgetChain(t, false) }, func(t *testing.T) { projectBudgetChain(t, true) })
}

func projectBudgetChain(t *testing.T, live bool) {
	h, admin, c, model := budgetSubject(t, "project", live)
	outsider := h.provision(t, admin, "project-outsider")
	memberRows := rowsOf(h.ok(http.MethodPost, "/project/list", c.session, map[string]any{}), "projects")
	if findBy(memberRows, "project_id", c.projectID) == nil {
		t.Fatalf("the member cannot see their own project: %v", namesOf(memberRows, "project_id"))
	}
	foreignRows := rowsOf(h.ok(http.MethodPost, "/project/list", outsider.session, map[string]any{}), "projects")
	if findBy(foreignRows, "project_id", c.projectID) != nil {
		t.Fatal("another organization can see this project")
	}

	runBudgetChain(t, h, admin, c, model, "Project", func(ceiling float64) {
		h.setProjectBudget(t, admin, c.projectID, ceiling)
	})

	h.ok(http.MethodPost, "/project/update", admin, map[string]any{"project_id": c.projectID, "blocked": true})
	h.assertStopped(t, c, model, "project blocked")
	h.ok(http.MethodPost, "/project/update", admin, map[string]any{"project_id": c.projectID, "blocked": false})
	h.assertBilled(t, c, admin, model, "project unblocked", []string{model})

	// 项目上限不能宽过团队。写失败之后，推理仍按刚才抬高的项目上限放行。
	h.setTeamBudget(t, admin, c.teamID, 5)
	refused := h.do(http.MethodPost, "/project/update", admin, map[string]any{
		"project_id": c.projectID, "max_budget": 6,
	})
	if refused.status < 300 {
		t.Fatalf("a project ceiling above its team was stored: %s", refused.describe())
	}
	h.assertBilled(t, c, admin, model, "project ceiling unchanged", []string{model})

	h.ok(http.MethodPost, "/project/delete", admin, map[string]any{"project_id": c.projectID})
	mark := len(h.upstreamCalls())
	gone := h.do(http.MethodPost, "/v1/chat/completions", c.key, chatRequest(model, "project deleted"))
	if gone.status < 300 {
		t.Fatalf("a key of a deleted project still answered: %s", gone.describe())
	}
	if !h.live {
		if got := h.upstreamSince(mark); len(got) != 0 {
			t.Fatalf("a deleted project's key reached the upstream: %v", got)
		}
	}
}

func TestOrganizationBudgetChain(t *testing.T) {
	runBoth(t, func(t *testing.T) { organizationBudgetChain(t, false) }, func(t *testing.T) { organizationBudgetChain(t, true) })
}

func organizationBudgetChain(t *testing.T, live bool) {
	h, admin, c, model := budgetSubject(t, "org", live)
	runBudgetChain(t, h, admin, c, model, "Organization", func(ceiling float64) {
		h.setOrgBudget(t, admin, c.orgID, ceiling)
	})
	h.ok(http.MethodPatch, "/organization/update", admin, map[string]any{
		"organization_id": c.orgID, "status": "blocked",
	})
	h.assertStopped(t, c, model, "org blocked")
	h.ok(http.MethodPatch, "/organization/update", admin, map[string]any{
		"organization_id": c.orgID, "status": "active",
	})
	h.assertBilled(t, c, admin, model, "org unblocked", []string{model})
}

func TestKeyBudgetChain(t *testing.T) {
	runBoth(t, func(t *testing.T) { keyBudgetChain(t, false) }, func(t *testing.T) { keyBudgetChain(t, true) })
}

func keyBudgetChain(t *testing.T, live bool) {
	h, admin, c, model := budgetSubject(t, "key", live)
	runBudgetChain(t, h, admin, c, model, "Key", func(ceiling float64) {
		h.setKeyBudget(t, admin, c.key, ceiling)
	})

	h.ok(http.MethodPost, "/key/block", admin, map[string]any{"key": c.key})
	h.assertStopped(t, c, model, "key blocked")
	h.ok(http.MethodPost, "/key/unblock", admin, map[string]any{"key": c.key})
	h.assertBilled(t, c, admin, model, "key unblocked", []string{model})

	h.ok(http.MethodPost, "/key/update", admin, map[string]any{"key": c.key, "duration": "1s"})
	time.Sleep(1500 * time.Millisecond)
	h.assertStopped(t, c, model, "key expired")
	h.ok(http.MethodPost, "/key/update", admin, map[string]any{"key": c.key, "duration": "24h"})
	h.assertBilled(t, c, admin, model, "key expiry extended", []string{model})
}

// TestBudgetNamesTheNarrowestExhaustedScope 把五层一起顶到同一条线上。
// 前置隔离 schema、本地上游；先释放子级保留额再收紧父级，断言额度拦截不调用供应商；harness 自动清理。
func TestBudgetNamesTheNarrowestExhaustedScope(t *testing.T) {
	runBoth(t, func(t *testing.T) { narrowBudgetChain(t, false) }, func(t *testing.T) { narrowBudgetChain(t, true) })
}

func narrowBudgetChain(t *testing.T, live bool) {
	h, admin, c, model := budgetSubject(t, "narrow", live)
	// 父级先留出余地，子级的上限才写得上去。一次真实调用之后再把要验的那几层收到实际花费上，
	// 这样模拟上游和真实供应商的 token 数不一样也不影响断言。
	const room = 1000.0
	h.setTeamBudget(t, admin, c.teamID, room)
	h.setProjectBudget(t, admin, c.projectID, room)
	h.setKeyBudget(t, admin, c.key, room)
	h.setUserBudget(t, admin, c.userID, room)
	h.setOrgBudget(t, admin, c.orgID, room)

	h.assertBilled(t, c, admin, model, "fills the tight scopes", []string{model})
	spent := h.moneyOf(t, c)
	// 先收回密钥未用保留额，才能降低个人；同时到顶时维持旧入口的个人优先错误契约。
	h.setKeyBudget(t, admin, c.key, spent.key)
	h.setUserBudget(t, admin, c.userID, spent.user)
	h.assertBudgetStop(t, c, admin, model, "user and key are both spent", "User")
	h.setUserBudget(t, admin, c.userID, room)
	h.assertBudgetStop(t, c, admin, model, "key is the narrow one left", "Key")
	// 将个人额度改为共享，避免未消费的个人保留额阻止降低团队额度。
	h.ok(http.MethodPost, "/user/update", admin, map[string]any{"user_id": c.userID, "max_budget": nil})
	h.ok(http.MethodPost, "/key/update", admin, map[string]any{"key": c.key, "max_budget": nil})
	h.setProjectBudget(t, admin, c.projectID, spent.project)
	h.setTeamBudget(t, admin, c.teamID, spent.team)
	h.assertBudgetStop(t, c, admin, model, "project and team are both spent", "Project")
	h.setTeamBudget(t, admin, c.teamID, room)
	h.setProjectBudget(t, admin, c.projectID, room)
	h.setTeamBudget(t, admin, c.teamID, spent.team)
	h.assertBudgetStop(t, c, admin, model, "only the team is spent", "Team")
	// 根组织收紧前释放团队保留额，个人/密钥继续共享根组织的金额上限。
	h.ok(http.MethodPost, "/team/update", admin, map[string]any{"team_id": c.teamID, "max_budget": nil})
	h.setOrgBudget(t, admin, c.orgID, spent.org)
	h.assertBudgetStop(t, c, admin, model, "only the organization is spent", "Organization")
	h.setOrgBudget(t, admin, c.orgID, room)
	h.assertBilled(t, c, admin, model, "every ceiling is open", []string{model})
}

func budgetFixture(t *testing.T, name string) (*harness, string, chained, string) {
	t.Helper()
	model := "regression-" + name
	h := newHarness(t, chatDeployment(model))
	admin := h.adminSession()
	return h, admin, h.openScope(t, admin, name), model
}

func budgetSubject(t *testing.T, name string, live bool) (*harness, string, chained, string) {
	t.Helper()
	if live {
		return openLiveChat(t, name+"-live")
	}
	return budgetFixture(t, name)
}

func runBudgetChain(t *testing.T, h *harness, admin string, c chained, model, scope string, set func(float64)) {
	t.Helper()
	const room = 1000.0
	set(room)
	h.assertBilled(t, c, admin, model, scope+" under ceiling", []string{model})
	set(scopeSpend(h.moneyOf(t, c), scope))
	h.assertBudgetStop(t, c, admin, model, scope+" at ceiling", scope)
	set(room)
	h.assertBilled(t, c, admin, model, scope+" ceiling raised", []string{model})
}

func scopeSpend(m scopeMoney, scope string) float64 {
	switch scope {
	case "User":
		return m.user
	case "Key":
		return m.key
	case "Project":
		return m.project
	case "Team":
		return m.team
	default:
		return m.org
	}
}
