// Member routes. A member reads their own team's roster; only a team
// administrator, or a platform administrator, changes it. The list is public
// account fields and nothing else, and a removal revokes the personal keys the
// membership was carrying.
package identity

import (
	"net/http"
	"strings"
	"sync"

	"github.com/sunqirui1987/xhub/internal/authz"
	"github.com/sunqirui1987/xhub/internal/httpx"
	"github.com/sunqirui1987/xhub/internal/iam"
	"github.com/sunqirui1987/xhub/internal/logx"
)

var logTraceOnceMembers sync.Once

// TeamMembers lists a team's roster. Any member of the team may read it, which is what the member selector is built from; the rows carry no key, no budget and no per-member spend.
// 参数 g（Gate）：带当前操作者的鉴权守卫。允许时返回 nil，拒绝时返回禁止或未找到；w（http.ResponseWriter）：调用方的 HTTP 响应，状态和正文写在这里；r（*http.Request）：入站 HTTP 请求。
// 返回：无。状态码和正文写进调用方的响应。
// 调用：gateway/identity/mount.go
// 测试：无直接单测
func TeamMembers(g Gate, w http.ResponseWriter, r *http.Request) {
	logTraceOnceMembers.Do(func() { logx.Trace("enter identity.TeamMembers") })

	httpx.SetCallID(w, httpx.CallID())
	p := g.RequireUser(w, r)
	if p == nil {
		return
	}
	teamID := teamIDFrom(r)
	if teamID == "" {
		httpx.WriteError(w, 400, "invalid_request", "team_id required")
		return
	}
	if err := g.Authorize(r, p, authz.ActionMemberRead, authz.Object{Type: authz.ObjectMember, TeamID: teamID}); err != nil {
		g.WriteAuthz(w, r, err)
		return
	}
	rows, err := g.Identity().ListMembers(r.Context(), teamID)
	if err != nil {
		g.WriteIAMError(w, r, err)
		return
	}
	out := make([]map[string]any, 0, len(rows))
	for i := range rows {
		out = append(out, memberPublic(&rows[i]))
	}
	httpx.WriteJSON(w, 200, map[string]any{"team_id": teamID, "members": out, "team_memberships": out})
}

// memberPublic is the public JSON for one membership.
// 参数 m（*iam.Member）：正在累加或展示的成员关系。
// 返回 map[string]any（map[string]any）：成员公开的字段表。缺键表示上游或库里没有这个字段。
// 调用：gateway/identity/handlers.go
// 测试：无直接单测
func memberPublic(m *iam.Member) map[string]any {
	return map[string]any{
		"user_id":    m.UserID,
		"user_email": m.Email,
		"user_alias": m.Name,
		"user_name":  m.Name,
		"role":       m.Role,
		"user_role":  m.Role,
		"status":     m.Status,
	}
}

// OrgMemberAdd grants organization administration to an existing account. The only organization role is org_admin. People reach an organization by joining one of its teams; this route does not create ageneral membership.
// 参数 g（Gate）：带当前操作者的鉴权守卫。允许时返回 nil，拒绝时返回禁止或未找到；w（http.ResponseWriter）：调用方的 HTTP 响应，状态和正文写在这里；r（*http.Request）：入站 HTTP 请求。
// 返回：无。状态码和正文写进调用方的响应。
// 调用：gateway/identity/mount.go
// 测试：无直接单测
func OrgMemberAdd(g Gate, w http.ResponseWriter, r *http.Request) {
	httpx.SetCallID(w, httpx.CallID())
	p := g.RequireUser(w, r)
	if p == nil {
		return
	}
	body := readMap(r)
	orgID := str(body["organization_id"])
	email, _ := memberFromBody(body)
	if orgID == "" || email == "" {
		httpx.WriteError(w, 400, "invalid_request", "organization_id and the member's email are required")
		return
	}
	if err := g.Authorize(r, p, authz.ActionOrgAdmin, authz.Object{Type: authz.ObjectOrg, ID: orgID}); err != nil {
		g.WriteAuthz(w, r, err)
		return
	}
	m, err := g.Identity().AddOrgAdmin(r.Context(), actorOf(p), orgID, email)
	if err != nil {
		g.WriteIAMError(w, r, err)
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{
		"organization_id": orgID,
		"member": map[string]any{
			"user_id": m.UserID, "user_email": m.Email, "user_alias": m.Name, "user_role": m.Role,
		},
	})
}

// OrgMemberRemove revokes organization administration.
// 参数 g（Gate）：带当前操作者的鉴权守卫。允许时返回 nil，拒绝时返回禁止或未找到；w（http.ResponseWriter）：调用方的 HTTP 响应，状态和正文写在这里；r（*http.Request）：入站 HTTP 请求。
// 返回：无。状态码和正文写进调用方的响应。
// 调用：gateway/identity/mount.go
// 测试：无直接单测
func OrgMemberRemove(g Gate, w http.ResponseWriter, r *http.Request) {
	httpx.SetCallID(w, httpx.CallID())
	p := g.RequireUser(w, r)
	if p == nil {
		return
	}
	body := readMap(r)
	orgID := str(body["organization_id"])
	target := str(body["user_id"])
	if target == "" {
		member, _ := body["member"].(map[string]any)
		target = str(member["user_id"])
	}
	if orgID == "" || target == "" {
		httpx.WriteError(w, 400, "invalid_request", "organization_id and user_id required")
		return
	}
	if err := g.Authorize(r, p, authz.ActionOrgAdmin, authz.Object{Type: authz.ObjectOrg, ID: orgID}); err != nil {
		g.WriteAuthz(w, r, err)
		return
	}
	if err := g.Identity().RemoveOrgAdmin(r.Context(), actorOf(p), orgID, target); err != nil {
		g.WriteIAMError(w, r, err)
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"organization_id": orgID, "user_id": target, "deleted": true})
}

// TeamMemberAdd adds an existing account to a team by exact email. An unknown address and a disabled account give the same answer, so the route cannot be used to enumerate accounts.
// 参数 g（Gate）：带当前操作者的鉴权守卫。允许时返回 nil，拒绝时返回禁止或未找到；w（http.ResponseWriter）：调用方的 HTTP 响应，状态和正文写在这里；r（*http.Request）：入站 HTTP 请求。
// 返回：无。状态码和正文写进调用方的响应。
// 调用：gateway/identity/mount.go
// 测试：无直接单测
func TeamMemberAdd(g Gate, w http.ResponseWriter, r *http.Request) {
	httpx.SetCallID(w, httpx.CallID())
	p := g.RequireUser(w, r)
	if p == nil {
		return
	}
	body := readMap(r)
	teamID := teamIDFrom(r)
	if teamID == "" {
		teamID = str(body["team_id"])
	}
	email, role := memberFromBody(body)
	if teamID == "" || email == "" {
		httpx.WriteError(w, 400, "invalid_request", "team_id and the member's email are required")
		return
	}
	if err := g.Authorize(r, p, authz.ActionMemberWrite, authz.Object{Type: authz.ObjectMember, TeamID: teamID}); err != nil {
		g.WriteAuthz(w, r, err)
		return
	}
	m, err := g.Identity().AddMember(r.Context(), actorOf(p), teamID, email, role)
	if err != nil {
		g.WriteIAMError(w, r, err)
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"team_id": teamID, "member": memberPublic(m)})
}

// TeamMemberUpdate changes a team role. The store refuses to leave a team without an administrator, and the refusal is a 400 rather than a silent demotion.
// 参数 g（Gate）：带当前操作者的鉴权守卫。允许时返回 nil，拒绝时返回禁止或未找到；w（http.ResponseWriter）：调用方的 HTTP 响应，状态和正文写在这里；r（*http.Request）：入站 HTTP 请求。
// 返回：无。状态码和正文写进调用方的响应。
// 调用：gateway/identity/mount.go
// 测试：无直接单测
func TeamMemberUpdate(g Gate, w http.ResponseWriter, r *http.Request) {
	httpx.SetCallID(w, httpx.CallID())
	p := g.RequireUser(w, r)
	if p == nil {
		return
	}
	body := readMap(r)
	teamID := teamIDFrom(r)
	if teamID == "" {
		teamID = str(body["team_id"])
	}
	target := str(body["user_id"])
	if target == "" {
		member, _ := body["member"].(map[string]any)
		target = str(member["user_id"])
	}
	_, role := memberFromBody(body)
	if teamID == "" || target == "" {
		httpx.WriteError(w, 400, "invalid_request", "team_id and user_id required")
		return
	}
	if err := g.Authorize(r, p, authz.ActionMemberWrite, authz.Object{Type: authz.ObjectMember, TeamID: teamID}); err != nil {
		g.WriteAuthz(w, r, err)
		return
	}
	if err := g.Identity().SetMemberRole(r.Context(), actorOf(p), teamID, target, role); err != nil {
		g.WriteIAMError(w, r, err)
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"team_id": teamID, "user_id": target, "role": role})
}

// TeamMemberRemove removes a membership. The store revokes the personal keys bound to that team in the same transaction, so a removed member's keys stop working on their next request.
// 参数 g（Gate）：带当前操作者的鉴权守卫。允许时返回 nil，拒绝时返回禁止或未找到；w（http.ResponseWriter）：调用方的 HTTP 响应，状态和正文写在这里；r（*http.Request）：入站 HTTP 请求。
// 返回：无。状态码和正文写进调用方的响应。
// 调用：gateway/identity/mount.go
// 测试：无直接单测
func TeamMemberRemove(g Gate, w http.ResponseWriter, r *http.Request) {
	httpx.SetCallID(w, httpx.CallID())
	p := g.RequireUser(w, r)
	if p == nil {
		return
	}
	body := readMap(r)
	teamID := teamIDFrom(r)
	if teamID == "" {
		teamID = str(body["team_id"])
	}
	if teamID == "" {
		httpx.WriteError(w, 400, "invalid_request", "team_id and user_id required")
		return
	}
	if err := g.Authorize(r, p, authz.ActionMemberRemove, authz.Object{Type: authz.ObjectMember, TeamID: teamID}); err != nil {
		g.WriteAuthz(w, r, err)
		return
	}
	target, email := memberDeleteTarget(body)
	if target == "" && email != "" {
		rows, err := g.Identity().ListMembers(r.Context(), teamID)
		if err != nil {
			g.WriteIAMError(w, r, err)
			return
		}
		for i := range rows {
			if strings.EqualFold(rows[i].Email, email) {
				target = rows[i].UserID
				break
			}
		}
	}
	if target == "" {
		httpx.WriteError(w, 400, "invalid_request", "team_id and user_id required")
		return
	}
	if err := g.Identity().RemoveMember(r.Context(), actorOf(p), teamID, target); err != nil {
		g.WriteIAMError(w, r, err)
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"removed": true})
}

// teamIDFrom resolves the team a request names, as a path parameter or a query parameter.
// 参数 r（*http.Request）：入站 HTTP 请求，用来读路径、头和正文。
// 返回 string（string）：路径参数 team_id。路径上没有时用查询参数 team_id。
// 调用：仅在 members.go 内使用
// 测试：无直接单测
func teamIDFrom(r *http.Request) string {
	if id := r.PathValue("team_id"); id != "" {
		return id
	}
	return r.URL.Query().Get("team_id")
}

// memberFromBody reads one membership out of a request. The console nests it under member and names the account user_email; the LiteLLM clients put the address in user_id instead, so both are accepted for that field. The team roles a caller may hand out are team_admin and member. Anything else, including a platform role, is read as member: a team administrator cannot promote anybody past their own team.
// 参数 body（map[string]any）：已经解析的 JSON 对象。缺字段表示上游或调用方没有给这项。
// 返回 string（string）：成员邮箱，依次取 user_email、user_id、email；string（string）：团队角色，只可能是 team_admin 或 member。
// 调用：仅在 members.go 内使用
// 测试：无直接单测
func memberFromBody(body map[string]any) (string, string) {
	member, _ := body["member"].(map[string]any)
	pick := func(key string) string {
		if v := str(member[key]); v != "" {
			return v
		}
		return str(body[key])
	}
	email := pick("user_email")
	if email == "" {
		email = pick("user_id")
	}
	if email == "" {
		email = pick("email")
	}
	return email, teamRole(pick("role"))
}

// memberDeleteTarget reads who to remove. The console sends user_id, and sometimes only the email. An email stuffed into user_id is treated as an email, because account ids are not addresses.
// 参数 body（map[string]any）：已经解析的 JSON 对象。缺字段表示上游或调用方没有给这项。
// 返回 string（string）：要移除的用户 id。user_id 里是邮箱时这里为空串；string（string）：要移除的邮箱。塞进 user_id 的邮箱也算邮箱。
// 调用：仅在 members.go 内使用
// 测试：无直接单测
func memberDeleteTarget(body map[string]any) (string, string) {
	member, _ := body["member"].(map[string]any)
	pick := func(key string) string {
		if v := str(member[key]); v != "" {
			return v
		}
		return str(body[key])
	}
	id := pick("user_id")
	email := pick("user_email")
	if email == "" {
		email = pick("email")
	}
	if strings.Contains(id, "@") {
		if email == "" {
			email = id
		}
		id = ""
	}
	return id, email
}

// teamRole maps a requested role onto the two roles a team has.
// 参数 requested（string）：调用方要求的角色名。空串按普通成员处理。
// 返回 string（string）：team_admin 或 admin 收成 iam.TeamAdmin，其余包括平台角色都收成 iam.TeamMember。
// 调用：gateway/identity/handlers.go
// 测试：无直接单测
func teamRole(requested string) string {
	switch strings.ToLower(strings.TrimSpace(requested)) {
	case "team_admin", "admin":
		return iam.TeamAdmin
	default:
		return iam.TeamMember
	}
}
