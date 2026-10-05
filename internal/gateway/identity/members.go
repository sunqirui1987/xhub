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

// TeamMembers lists a team's roster. Any member of the team may read it, which
// is what the member selector is built from; the rows carry no key, no budget
// and no per-member spend.
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

// TeamMemberAdd adds an existing account to a team by exact email. An unknown
// address and a disabled account give the same answer, so the route cannot be
// used to enumerate accounts.
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

// TeamMemberUpdate changes a team role. The store refuses to leave a team
// without an administrator, and the refusal is a 400 rather than a silent
// demotion.
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

// TeamMemberRemove removes a membership. The store revokes the personal keys
// bound to that team in the same transaction, so a removed member's keys stop
// working on their next request.
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
	target := str(body["user_id"])
	if teamID == "" || target == "" {
		httpx.WriteError(w, 400, "invalid_request", "team_id and user_id required")
		return
	}
	if err := g.Authorize(r, p, authz.ActionMemberRemove, authz.Object{Type: authz.ObjectMember, TeamID: teamID}); err != nil {
		g.WriteAuthz(w, r, err)
		return
	}
	if err := g.Identity().RemoveMember(r.Context(), actorOf(p), teamID, target); err != nil {
		g.WriteIAMError(w, r, err)
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"removed": true})
}

// teamIDFrom resolves the team a request names, as a path parameter or a query
// parameter.
func teamIDFrom(r *http.Request) string {
	if id := r.PathValue("team_id"); id != "" {
		return id
	}
	return r.URL.Query().Get("team_id")
}

// memberFromBody reads one membership out of a request. The console nests it
// under member and names the account user_email; the LiteLLM clients put the
// address in user_id instead, so both are accepted for that field.
//
// The team roles a caller may hand out are team_admin and member. Anything else,
// including a platform role, is read as member: a team administrator cannot
// promote anybody past their own team.
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

// teamRole maps a requested role onto the two roles a team has.
func teamRole(requested string) string {
	switch strings.ToLower(strings.TrimSpace(requested)) {
	case "team_admin", "admin":
		return iam.TeamAdmin
	default:
		return iam.TeamMember
	}
}
