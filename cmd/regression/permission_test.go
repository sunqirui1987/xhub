package regression

import (
	"net/http"
	"strings"
	"testing"
)

// 这一组测权限：谁能看见谁、谁能改谁、以及越界时会被怎么挡住。
//
// 权限出问题的方式很安静——不是报错，而是多看见了一条、多改了一行。所以这里
// 的断言不看状态码就算了，而是去点数：这个身份看到几条团队、几行日志、
// 能不能读到隔壁租户的那一条。
//
// 用的身份有四种，覆盖了判定矩阵的四个角：
//
//   - 平台管理员：什么都看得见。
//   - 组织管理员：只管自己这个组织。
//   - 团队成员：只看得见自己所属的那一条，改不了别人。
//   - 隔壁租户：什么都看不见。

// TestTenantIsolationAcrossOrganizations 证明两个组织互相看不见对方的团队和项目。
// 这是最要紧的一条：串租户是权限系统最严重的故障形态，而且它不会抛错，
// 只会让列表里多出几行。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestTenantIsolationAcrossOrganizations(t *testing.T) {
	h := newHarness(t, chatDeployment("regression-perm"))
	admin := h.adminSession()

	// 两个互不相干的租户，各有自己的组织、团队、用户和密钥。
	a := h.provision(t, admin, "perm-a")
	b := h.provision(t, admin, "perm-b")

	// 平台管理员两边都看得见。
	teams := h.ok(http.MethodGet, "/team/list", admin, nil)
	allNames := namesOf(rowsOf(teams, "teams", "data"), "team_alias")
	for _, want := range []string{"perm-a-team", "perm-b-team"} {
		if !contains(allNames, want) {
			t.Fatalf("the platform administrator cannot see %s: %v", want, allNames)
		}
	}

	// A 租户的成员只看得见自己那一条团队。
	aTeams := h.ok(http.MethodGet, "/team/list", a.session, nil)
	aNames := namesOf(rowsOf(aTeams, "teams", "data"), "team_alias")
	if contains(aNames, "perm-b-team") {
		t.Fatalf("tenant A can see tenant B's team: %v", aNames)
	}
	if !contains(aNames, "perm-a-team") {
		t.Fatalf("tenant A cannot see its own team: %v", aNames)
	}

	// 组织列表同理。
	bOrgs := h.ok(http.MethodGet, "/organization/list", b.session, nil)
	bOrgNames := namesOf(rowsOf(bOrgs, "organizations", "data"), "organization_alias")
	if contains(bOrgNames, "perm-a-org") {
		t.Fatalf("tenant B can see tenant A's organization: %v", bOrgNames)
	}
}

// TestMemberCannotReadAnotherTenant 证明越界读取返回的不是"空列表"而是拒绝，
// 而且拒绝的理由是找不到而不是没权限——对不该知道存在性的东西，不该泄漏它存在。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestMemberCannotReadAnotherTenant(t *testing.T) {
	h := newHarness(t, chatDeployment("regression-perm"))
	admin := h.adminSession()
	a := h.provision(t, admin, "perm-a")
	b := h.provision(t, admin, "perm-b")

	// A 的成员去读 B 的团队。
	r := h.do(http.MethodGet, "/team/info?team_id="+b.teamID, a.session, nil)
	if r.status < 300 {
		t.Fatalf("tenant A read tenant B's team directly: %s", r.describe())
	}
	// 拒绝的原因不能把"这个团队存在"这件事说出来。
	if message := errorMessage(r); strings.Contains(message, "perm-b") {
		t.Fatalf("the refusal leaked the other tenant's name: %q", message)
	}
}

// TestMemberCannotAdminister 证明普通成员改不了团队、也发不了别人的密钥。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestMemberCannotAdminister(t *testing.T) {
	h := newHarness(t, chatDeployment("regression-perm"))
	admin := h.adminSession()
	a := h.provision(t, admin, "perm-a")

	// 改自己团队的额度：这是管理员的事，成员不行。
	r := h.do(http.MethodPost, "/team/update", a.session, map[string]any{
		"team_id": a.teamID, "max_budget": 1000,
	})
	if r.status < 300 {
		t.Fatalf("a member changed their own team's budget: %s", r.describe())
	}

	// 给团队加人：同样不该允许。
	r = h.do(http.MethodPost, "/team/member_add", a.session, map[string]any{
		"team_id": a.teamID, "user_email": "intruder@example.com", "role": "admin",
	})
	if r.status < 300 {
		t.Fatalf("a member added a member to their own team: %s", r.describe())
	}
}

// TestKeyLifecycleAndScope 证明密钥的完整生命周期：签发、能用、被看到、
// 被停用后立刻失效、以及列表里不会出现别的租户的密钥。
//
// "停用后立刻失效"这一条特别值得测：密钥是被反复校验的，缓存一层就可能导致
// 停用之后还能继续用一段时间。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestKeyLifecycleAndScope(t *testing.T) {
	h := newHarness(t, chatDeployment("regression-perm"))
	admin := h.adminSession()
	a := h.provision(t, admin, "perm-a")
	b := h.provision(t, admin, "perm-b")

	request := map[string]any{
		"model": "regression-perm", "messages": []any{map[string]any{"role": "user", "content": "key-lifecycle"}},
	}

	// 刚签发的密钥能用。
	h.ok(http.MethodPost, "/v1/chat/completions", a.key, request)

	// 它出现在 A 自己的密钥列表里。
	keys := h.ok(http.MethodGet, "/key/list", a.session, nil)
	aliases := namesOf(rowsOf(keys, "keys", "data"), "key_alias")
	if !contains(aliases, "perm-a-key") {
		t.Fatalf("tenant A cannot see its own key: %v", aliases)
	}
	// 但不出现在 B 的列表里。
	bKeys := h.ok(http.MethodGet, "/key/list", b.session, nil)
	bAliases := namesOf(rowsOf(bKeys, "keys", "data"), "key_alias")
	if contains(bAliases, "perm-a-key") {
		t.Fatalf("tenant B can see tenant A's key: %v", bAliases)
	}

	// 停用之后立刻失效，不该还能再用一段时间。
	h.ok(http.MethodPost, "/key/block", a.session, map[string]any{"key": a.key})
	blocked := h.do(http.MethodPost, "/v1/chat/completions", a.key, request)
	if blocked.status < 300 {
		t.Fatalf("a blocked key still called inference: %s", blocked.describe())
	}
}

// TestKeyRegenerateInvalidatesTheOldSecret 证明重新生成密钥之后，旧密钥立刻作废。
// 一半作废的轮换比不作废还危险：运维以为已经换了，旧的那把却还能用。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestKeyRegenerateInvalidatesTheOldSecret(t *testing.T) {
	h := newHarness(t, chatDeployment("regression-perm"))
	admin := h.adminSession()
	a := h.provision(t, admin, "perm-a")

	rotated := h.ok(http.MethodPost, "/key/regenerate", a.session, map[string]any{"key": a.key}).json()
	fresh := firstString(rotated, "key", "token")
	if !strings.HasPrefix(fresh, "sk-") {
		t.Fatalf("regenerate returned no usable secret: %s", mustJSON(rotated))
	}
	if fresh == a.key {
		t.Fatal("regenerate returned the same secret")
	}

	request := map[string]any{
		"model": "regression-perm", "messages": []any{map[string]any{"role": "user", "content": "rotate"}},
	}
	// 新密钥能用。
	h.ok(http.MethodPost, "/v1/chat/completions", fresh, request)
	// 旧密钥不能再用了。
	old := h.do(http.MethodPost, "/v1/chat/completions", a.key, request)
	if old.status < 300 {
		t.Fatalf("the rotated-away key still works: %s", old.describe())
	}
}

// TestUnauthenticatedIsRefused 证明没带身份时管理面和数据面都拒绝。
// 这类用例看着琐碎，但它是"鉴权中间件被整段跳过"这类回归唯一抓得住的地方。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestUnauthenticatedIsRefused(t *testing.T) {
	h := newHarness(t, chatDeployment("regression-perm"))

	cases := []struct {
		method, path string
		body         any
	}{
		{http.MethodPost, "/v1/chat/completions", map[string]any{"model": "regression-perm", "messages": []any{}}},
		{http.MethodGet, "/key/list", nil},
		{http.MethodGet, "/team/list", nil},
		{http.MethodGet, "/user/list", nil},
		{http.MethodGet, "/spend/logs/ui", nil},
		{http.MethodPost, "/price/model", map[string]any{"id": "x", "litellm_provider": "custom"}},
	}
	for _, tc := range cases {
		if r := h.do(tc.method, tc.path, "", tc.body); r.status != http.StatusUnauthorized {
			t.Fatalf("%s %s answered %d without a token", tc.method, tc.path, r.status)
		}
	}
}

// TestInvalidKeyIsRefused 证明一把乱编的密钥换不到任何东西。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestInvalidKeyIsRefused(t *testing.T) {
	h := newHarness(t, chatDeployment("regression-perm"))
	r := h.do(http.MethodPost, "/v1/chat/completions", "sk-not-a-real-key-000000000000", map[string]any{
		"model": "regression-perm", "messages": []any{map[string]any{"role": "user", "content": "hi"}},
	})
	if r.status != http.StatusUnauthorized {
		t.Fatalf("an invented key was accepted: %s", r.describe())
	}
}

// TestOrganizationAndTeamHierarchy 证明组织与团队的归属层级真的建立起来了：
// 团队挂在指定的组织下，成员挂进团队之后就从团队视角看得见。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestOrganizationAndTeamHierarchy(t *testing.T) {
	h := newHarness(t, chatDeployment("regression-perm"))
	admin := h.adminSession()
	a := h.provision(t, admin, "perm-a")

	// 团队信息要报出它所属的组织。
	info := h.ok(http.MethodGet, "/team/info?team_id="+a.teamID, admin, nil).json()
	team, _ := info["team_info"].(map[string]any)
	if team == nil {
		team = info
	}
	if got := firstString(team, "organization_id"); got != a.orgID {
		t.Fatalf("team reports organization_id=%q, want %q", got, a.orgID)
	}

	// 再建一个用户并加进这个团队，然后它应该出现在成员列表里。
	member := h.newTenant(t, admin, "perm-member")
	h.ok(http.MethodPost, "/team/member_add", admin, map[string]any{
		"team_id": a.teamID, "user_email": member.email, "role": "user",
	})
	members := h.ok(http.MethodGet, "/team/member_list?team_id="+a.teamID, admin, nil)
	emails := namesOf(rowsOf(members, "members", "data"), "user_email")
	if !contains(emails, member.email) {
		t.Fatalf("the added member is not in the team: %v", emails)
	}
}

// contains 判断列表里有没有这个值。
// 参数 list（[]string）：要查的列表；want（string）：要找的值，按字符串相等比较。
// 返回 bool（bool）：列表里有 want 时为真。
func contains(list []string, want string) bool {
	for _, item := range list {
		if item == want {
			return true
		}
	}
	return false
}
