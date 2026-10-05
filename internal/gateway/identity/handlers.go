// Package identity serves users, organizations, teams, members and projects.
// Every handler resolves the caller, asks Authorize for one decision per
// object, and only then touches identity data; listings are narrowed in SQL by
// the scope rather than filtered after the rows are read.
package identity

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/sunqirui1987/xhub/internal/auth"
	"github.com/sunqirui1987/xhub/internal/authz"
	"github.com/sunqirui1987/xhub/internal/httpx"
	"github.com/sunqirui1987/xhub/internal/iam"
	"github.com/sunqirui1987/xhub/internal/logx"
)

var logTraceOnceHandlers sync.Once

// ---------- users ----------

// UserNew creates an account. Only a platform administrator may, because the
// account's existence changes what every other decision can see.
func UserNew(g Gate, w http.ResponseWriter, r *http.Request) {
	logTraceOnceHandlers.Do(func() { logx.Trace("enter identity.UserNew") })

	httpx.SetCallID(w, httpx.CallID())
	p := g.RequireManage(w, r)
	if p == nil {
		return
	}
	body := readMap(r)
	in := iam.UserInput{
		Email:     str(body["user_email"]),
		Name:      str(body["user_alias"]),
		Password:  str(body["password"]),
		Role:      iam.StoreRole(str(body["user_role"])),
		MaxBudget: floatPtr(body["max_budget"]),
	}
	if in.Name == "" {
		in.Name = str(body["user_name"])
	}
	if err := g.Authorize(r, p, authz.ActionUserCreate, authz.Object{Type: authz.ObjectUser}); err != nil {
		g.WriteAuthz(w, r, err)
		return
	}
	u, err := g.Identity().CreateUser(r.Context(), actorOf(p), in)
	if err != nil {
		g.WriteIAMError(w, r, err)
		return
	}
	httpx.WriteJSON(w, 200, userPublic(u, nil))
}

// UserList lists accounts. Only a platform administrator reaches the store: a
// signed-in member reads other people through the member listing of a team,
// which returns public fields under the team scope.
func UserList(g Gate, w http.ResponseWriter, r *http.Request) {
	httpx.SetCallID(w, httpx.CallID())
	p := g.RequireManage(w, r)
	if p == nil {
		return
	}
	limit := queryInt(r, "page_size", 100)
	offset := pageOffset(r, limit)
	rows, err := g.Identity().ListUsers(r.Context(), r.URL.Query().Get("search"), limit, offset)
	if err != nil {
		g.WriteIAMError(w, r, err)
		return
	}
	roles, err := membershipsOf(g, r, rows)
	if err != nil {
		g.WriteIAMError(w, r, err)
		return
	}
	out := make([]map[string]any, 0, len(rows))
	for i := range rows {
		out = append(out, userPublic(&rows[i], roles[rows[i].ID]))
	}
	httpx.WriteJSON(w, 200, map[string]any{
		"users":        out,
		"total_count":  len(out),
		"current_page": 1,
		"total_pages":  1,
		"size":         len(out),
	})
}

// membershipsOf reads each user's team roles. A platform administrator's
// listing is the only caller, so a read failure is a real failure.
func membershipsOf(g Gate, r *http.Request, rows []iam.User) (map[string][]map[string]any, error) {
	out := make(map[string][]map[string]any, len(rows))
	for i := range rows {
		teams, err := g.Identity().ListTeams(r.Context(), rows[i].ID, "")
		if err != nil {
			return nil, err
		}
		list := make([]map[string]any, 0, len(teams))
		for _, t := range teams {
			list = append(list, map[string]any{
				"team_id":         t.ID,
				"team_alias":      t.Name,
				"user_role":       t.Role,
				"organization_id": t.OrganizationID,
			})
		}
		out[rows[i].ID] = list
	}
	return out, nil
}

// UserFilterUI is the account picker the console builds filters from. It
// returns names and ids only and is a platform administrator route.
func UserFilterUI(g Gate, w http.ResponseWriter, r *http.Request) {
	httpx.SetCallID(w, httpx.CallID())
	p := g.RequireManage(w, r)
	if p == nil {
		return
	}
	rows, err := g.Identity().ListUsers(r.Context(), "", 500, 0)
	if err != nil {
		g.WriteIAMError(w, r, err)
		return
	}
	emailQ := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("user_email")))
	idQ := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("user_id")))
	out := make([]map[string]any, 0, len(rows))
	for i := range rows {
		if emailQ != "" && !strings.Contains(strings.ToLower(rows[i].Email), emailQ) {
			continue
		}
		if idQ != "" && !strings.Contains(strings.ToLower(rows[i].ID), idQ) {
			continue
		}
		out = append(out, map[string]any{"user_id": rows[i].ID, "user_email": rows[i].Email, "user_alias": rows[i].Name})
	}
	httpx.WriteJSON(w, 200, map[string]any{"users": out})
}

// AvailableUsers is the account picker. It is the same listing as the filter
// picker: only a platform administrator may enumerate accounts.
func AvailableUsers(g Gate, w http.ResponseWriter, r *http.Request) {
	UserFilterUI(g, w, r)
}

// UserInfo reads one account. A caller reads themselves; a platform
// administrator reads anybody.
func UserInfo(g Gate, w http.ResponseWriter, r *http.Request) {
	httpx.SetCallID(w, httpx.CallID())
	p := g.RequireUser(w, r)
	if p == nil {
		return
	}
	body := readMap(r)
	target := r.URL.Query().Get("user_id")
	if v := str(body["user_id"]); v != "" {
		target = v
	}
	if target == "" {
		target = p.UserID
	}
	if err := g.Authorize(r, p, authz.ActionUserRead, authz.Object{Type: authz.ObjectUser, ID: target}); err != nil {
		g.WriteAuthz(w, r, err)
		return
	}
	u, err := g.Identity().GetUser(r.Context(), target)
	if err != nil {
		g.WriteIAMError(w, r, err)
		return
	}
	roles, err := membershipsOf(g, r, []iam.User{*u})
	if err != nil {
		g.WriteIAMError(w, r, err)
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"user_info": userPublic(u, roles[u.ID])})
}

// UserUpdate changes a role, a status, a budget or an email. Only a platform
// administrator may, and only their own name and password are reachable through
// the profile routes.
func UserUpdate(g Gate, w http.ResponseWriter, r *http.Request) {
	httpx.SetCallID(w, httpx.CallID())
	p := g.RequireUser(w, r)
	if p == nil {
		return
	}
	body := readMap(r)
	target := r.PathValue("user_id")
	if target == "" {
		target = str(body["user_id"])
	}
	if target == "" {
		target = p.UserID
	}
	self := target == p.UserID && p.Kind == authz.KindSession
	if err := g.Authorize(r, p, authz.ActionUserWrite, authz.Object{Type: authz.ObjectUser, ID: target}); err != nil {
		g.WriteAuthz(w, r, err)
		return
	}
	if self && !p.PlatformAdmin() {
		// A person edits their own profile and password, never their own role,
		// status or budget: those would be self-elevation.
		if err := updateProfile(g, r, p, target, body); err != nil {
			g.WriteIAMError(w, r, err)
			return
		}
	} else {
		in := iam.UserUpdate{
			Role:      storedRolePtr(body),
			Status:    statusPtr(body),
			Email:     stringPtr(body, "user_email"),
			MaxBudget: optionalFloat(body, "max_budget"),
		}
		if in.Role == nil && in.Status == nil && in.Email == nil && in.MaxBudget == nil {
			httpx.WriteError(w, 400, "invalid_request", "no supported field to update")
			return
		}
		if _, err := g.Identity().AdminUpdateUser(r.Context(), actorOf(p), target, in); err != nil {
			g.WriteIAMError(w, r, err)
			return
		}
	}
	u, err := g.Identity().GetUser(r.Context(), target)
	if err != nil {
		g.WriteIAMError(w, r, err)
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"user_info": userPublic(u, nil)})
}

// storedRolePtr reads the console role and maps it onto the stored spelling.
// A missing field stays nil so an update that does not mention the role leaves
// it alone.
func storedRolePtr(body map[string]any) *string {
	raw := stringPtr(body, "user_role")
	if raw == nil {
		return nil
	}
	mapped := iam.StoreRole(*raw)
	return &mapped
}

// statusPtr reads the block flag the console sends as blocked.
func statusPtr(body map[string]any) *string {
	if v, ok := body["blocked"].(bool); ok {
		s := iam.StatusActive
		if v {
			s = iam.StatusDisabled
		}
		return &s
	}
	return stringPtr(body, "status")
}

// updateProfile applies the two fields a person owns.
func updateProfile(g Gate, r *http.Request, p *auth.Principal, id string, body map[string]any) error {
	ctx := r.Context()
	if name := str(body["user_alias"]); name != "" {
		if _, err := g.Identity().UpdateProfile(ctx, actorOf(p), id, name); err != nil {
			return err
		}
	}
	if pw := str(body["password"]); pw != "" {
		if err := g.Identity().SetPassword(ctx, actorOf(p), id, pw); err != nil {
			return err
		}
	}
	return nil
}

// UserDelete removes an account. Its memberships go with it and its personal
// keys are revoked; service keys belong to teams and are untouched.
func UserDelete(g Gate, w http.ResponseWriter, r *http.Request) {
	httpx.SetCallID(w, httpx.CallID())
	p := g.RequireUser(w, r)
	if p == nil {
		return
	}
	body := readMap(r)
	target := r.PathValue("user_id")
	if target == "" {
		target = str(body["user_id"])
	}
	if target == "" {
		httpx.WriteError(w, 400, "invalid_request", "user_id required")
		return
	}
	if target == p.UserID {
		httpx.WriteError(w, 400, "invalid_request", "an account cannot delete itself")
		return
	}
	if err := g.Authorize(r, p, authz.ActionUserDelete, authz.Object{Type: authz.ObjectUser, ID: target}); err != nil {
		g.WriteAuthz(w, r, err)
		return
	}
	if err := g.Identity().DeleteUser(r.Context(), actorOf(p), target); err != nil {
		g.WriteIAMError(w, r, err)
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"deleted": true})
}

// ---------- organizations ----------

// OrgNew creates an organization. Only a platform administrator may: an
// organization is a grouping of teams, not something a tenant owns.
func OrgNew(g Gate, w http.ResponseWriter, r *http.Request) {
	httpx.SetCallID(w, httpx.CallID())
	p := g.RequireManage(w, r)
	if p == nil {
		return
	}
	body := readMap(r)
	if err := g.Authorize(r, p, authz.ActionOrgWrite, authz.Object{Type: authz.ObjectOrg}); err != nil {
		g.WriteAuthz(w, r, err)
		return
	}
	o, err := g.Identity().CreateOrg(r.Context(), actorOf(p), str(body["organization_alias"]), floatPtr(body["max_budget"]))
	if err != nil {
		g.WriteIAMError(w, r, err)
		return
	}
	httpx.WriteJSON(w, 200, orgPublic(o))
}

// OrgList lists the organizations the caller may see. A platform administrator
// sees every one; everybody else sees only the organizations that own one of
// their teams, which is what the console shows as the team's grouping.
func OrgList(g Gate, w http.ResponseWriter, r *http.Request) {
	httpx.SetCallID(w, httpx.CallID())
	p := g.RequireUser(w, r)
	if p == nil {
		return
	}
	// The scope is decided by who is asking, not by whether the user ID happens
	// to be empty. A master principal carries no user ID, so deriving the answer
	// from the ID alone turned it into "every organization" — the store reads an
	// empty ID as an unfiltered query. The master credential reaches bootstrap
	// and emergency routes only, so it sees nothing here.
	if p.Kind == authz.KindMaster {
		httpx.WriteJSON(w, 200, []any{})
		return
	}
	mine := p.UserID
	if p.PlatformAdmin() {
		mine = ""
	}
	rows, err := g.Identity().ListOrgs(r.Context(), mine)
	if err != nil {
		g.WriteIAMError(w, r, err)
		return
	}
	out := make([]map[string]any, 0, len(rows))
	for i := range rows {
		view, err := orgView(g, r, &rows[i])
		if err != nil {
			g.WriteIAMError(w, r, err)
			return
		}
		out = append(out, view)
	}
	httpx.WriteJSON(w, 200, out)
}

// OrgInfo reads one organization.
func OrgInfo(g Gate, w http.ResponseWriter, r *http.Request) {
	httpx.SetCallID(w, httpx.CallID())
	p := g.RequireUser(w, r)
	if p == nil {
		return
	}
	id := orgIDFrom(g, r, p)
	if id == "" {
		httpx.WriteError(w, 400, "invalid_request", "organization_id required")
		return
	}
	if err := g.Authorize(r, p, authz.ActionOrgRead, authz.Object{Type: authz.ObjectOrg, ID: id}); err != nil {
		g.WriteAuthz(w, r, err)
		return
	}
	o, err := g.Identity().GetOrg(r.Context(), id)
	if err != nil {
		g.WriteIAMError(w, r, err)
		return
	}
	view, err := orgView(g, r, o)
	if err != nil {
		g.WriteIAMError(w, r, err)
		return
	}
	httpx.WriteJSON(w, 200, view)
}

// orgIDFrom resolves the organization a request names, as a query parameter or
// in the body.
func orgIDFrom(_ Gate, r *http.Request, _ *auth.Principal) string {
	if id := r.PathValue("organization_id"); id != "" {
		return id
	}
	if id := r.URL.Query().Get("organization_id"); id != "" {
		return id
	}
	return ""
}

// OrgUpdate changes an organization's name, status or budget.
func OrgUpdate(g Gate, w http.ResponseWriter, r *http.Request) {
	httpx.SetCallID(w, httpx.CallID())
	p := g.RequireUser(w, r)
	if p == nil {
		return
	}
	body := readMap(r)
	id := str(body["organization_id"])
	if id == "" {
		httpx.WriteError(w, 400, "invalid_request", "organization_id required")
		return
	}
	// A budget or a status change is the platform administrator's. An
	// organization administrator may rename the organization only.
	action := authz.ActionOrgAdmin
	if _, budget := body["max_budget"]; budget || stringPtr(body, "status") != nil {
		action = authz.ActionOrgWrite
	}
	if err := g.Authorize(r, p, action, authz.Object{Type: authz.ObjectOrg, ID: id}); err != nil {
		g.WriteAuthz(w, r, err)
		return
	}
	in := iam.OrgUpdate{
		Name:      orgNamePtr(body),
		Status:    stringPtr(body, "status"),
		MaxBudget: optionalFloat(body, "max_budget"),
	}
	o, err := g.Identity().UpdateOrg(r.Context(), actorOf(p), id, in)
	if err != nil {
		g.WriteIAMError(w, r, err)
		return
	}
	httpx.WriteJSON(w, 200, orgPublic(o))
}

// orgNamePtr reads the name, which the console may send under either key.
func orgNamePtr(body map[string]any) *string {
	if v := stringPtr(body, "organization_alias"); v != nil {
		return v
	}
	return stringPtr(body, "organization_name")
}

// OrgDelete removes an organization. It refuses while teams remain, because
// deleting the teams underneath as a side effect of a rename-shaped request is
// not what the caller asked for.
func OrgDelete(g Gate, w http.ResponseWriter, r *http.Request) {
	httpx.SetCallID(w, httpx.CallID())
	p := g.RequireManage(w, r)
	if p == nil {
		return
	}
	id := str(readMap(r)["organization_id"])
	if id == "" {
		httpx.WriteError(w, 400, "invalid_request", "organization_id required")
		return
	}
	if err := g.Authorize(r, p, authz.ActionOrgWrite, authz.Object{Type: authz.ObjectOrg, ID: id}); err != nil {
		g.WriteAuthz(w, r, err)
		return
	}
	if err := g.Identity().DeleteOrg(r.Context(), actorOf(p), id); err != nil {
		g.WriteIAMError(w, r, err)
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"deleted": true})
}

// ---------- teams ----------

// TeamNew creates a team inside an organization, together with its first
// administrator. Only a platform administrator may create or move a team.
func TeamNew(g Gate, w http.ResponseWriter, r *http.Request) {
	httpx.SetCallID(w, httpx.CallID())
	p := g.RequireManage(w, r)
	if p == nil {
		return
	}
	body := readMap(r)
	if err := g.Authorize(r, p, authz.ActionTeamCreate, authz.Object{Type: authz.ObjectTeam}); err != nil {
		g.WriteAuthz(w, r, err)
		return
	}
	in := iam.TeamInput{
		OrganizationID: str(body["organization_id"]),
		Name:           teamName(body),
		Description:    str(body["team_description"]),
		Models:         stringList(body["models"]),
		MaxBudget:      floatPtr(body["max_budget"]),
		AdminUserID:    teamAdminID(body),
	}
	if in.AdminUserID == "" {
		// The console's create-team form names no administrator, and the route's
		// contract is that the caller becomes the team's first team_admin. A team
		// with no administrator could never be managed again, so this is a
		// default rather than an omission.
		in.AdminUserID = p.UserID
	}
	if in.OrganizationID == "" || in.Name == "" || in.AdminUserID == "" {
		httpx.WriteError(w, 400, "invalid_request", "organization_id, team_alias and the first team admin are required")
		return
	}
	t, err := g.Identity().CreateTeam(r.Context(), actorOf(p), in)
	if err != nil {
		g.WriteIAMError(w, r, err)
		return
	}
	httpx.WriteJSON(w, 200, teamPublic(t, iam.TeamAdmin))
}

// teamName reads the team's display name, which the console sends under
// team_alias.
func teamName(body map[string]any) string {
	if v := str(body["team_alias"]); v != "" {
		return v
	}
	return str(body["team_name"])
}

// teamAdminID reads the first administrator. The console sends an email.
func teamAdminID(body map[string]any) string {
	if v := str(body["admin_user_id"]); v != "" {
		return v
	}
	return str(body["user_id"])
}

// TeamList lists the teams the caller belongs to, or every team for a platform
// administrator. The rows carry the caller's own role. The body is an array,
// which is what GET /team/list has always returned.
func TeamList(g Gate, w http.ResponseWriter, r *http.Request) {
	out, ok := loadTeamList(g, w, r)
	if !ok {
		return
	}
	httpx.WriteJSON(w, 200, out)
}

// TeamListV2 is GET /v2/team/list. The console's paged query reads teams,
// total, page, page_size and total_pages. A bare array makes teams undefined,
// and React Query rejects a query function that returns undefined.
func TeamListV2(g Gate, w http.ResponseWriter, r *http.Request) {
	out, ok := loadTeamList(g, w, r)
	if !ok {
		return
	}
	page := queryInt(r, "page", 1)
	if page < 1 {
		page = 1
	}
	size := queryInt(r, "page_size", 100)
	if size < 1 {
		size = 100
	}
	if size > 500 {
		size = 500
	}
	total := len(out)
	start := (page - 1) * size
	if start > total {
		start = total
	}
	end := start + size
	if end > total {
		end = total
	}
	pageRows := out[start:end]
	if pageRows == nil {
		pageRows = []map[string]any{}
	}
	pages := 1
	if total > 0 {
		pages = (total + size - 1) / size
	}
	httpx.WriteJSON(w, 200, map[string]any{
		"teams":       pageRows,
		"total":       total,
		"page":        page,
		"page_size":   size,
		"total_pages": pages,
	})
}

// loadTeamList reads the teams this caller may see and applies the query
// filters on top of that scope. A platform administrator who passes user_id
// narrows to that account; omitting it lists every team. Anyone else is always
// limited to their own memberships, so a query parameter cannot widen the list.
// status=deleted is an empty list: teams are removed, not archived.
func loadTeamList(g Gate, w http.ResponseWriter, r *http.Request) ([]map[string]any, bool) {
	httpx.SetCallID(w, httpx.CallID())
	p := g.RequireUser(w, r)
	if p == nil {
		return nil, false
	}
	if !p.PlatformAdmin() && p.Kind != authz.KindSession {
		// A virtual key is a credential for inference; it does not browse the
		// tenant's teams.
		httpx.WriteError(w, 403, "forbidden", "this credential cannot list teams")
		return nil, false
	}
	if r.URL.Query().Get("status") == "deleted" {
		return []map[string]any{}, true
	}
	orgFilter := r.URL.Query().Get("organization_id")
	var rows []iam.TeamWithRole
	var err error
	if p.PlatformAdmin() {
		rows, err = g.Identity().ListTeams(r.Context(), r.URL.Query().Get("user_id"), orgFilter)
	} else {
		rows, err = g.Identity().ListVisibleTeams(r.Context(), p.UserID, orgFilter)
	}
	if err != nil {
		g.WriteIAMError(w, r, err)
		return nil, false
	}
	wantID := r.URL.Query().Get("team_id")
	alias := strings.ToLower(r.URL.Query().Get("team_alias"))
	out := make([]map[string]any, 0, len(rows))
	for i := range rows {
		if wantID != "" && rows[i].ID != wantID {
			continue
		}
		if alias != "" && !strings.Contains(strings.ToLower(rows[i].Name), alias) {
			continue
		}
		pub := teamPublic(&rows[i].Team, rows[i].Role)
		attachRoster(g, r, p, pub)
		out = append(out, pub)
	}
	return out, true
}

// TeamAvailable lists the teams the caller could put a resource in, which is
// the same set as their own teams.
func TeamAvailable(g Gate, w http.ResponseWriter, r *http.Request) {
	TeamList(g, w, r)
}

// TeamInfo reads one team.
func TeamInfo(g Gate, w http.ResponseWriter, r *http.Request) {
	httpx.SetCallID(w, httpx.CallID())
	p := g.RequireUser(w, r)
	if p == nil {
		return
	}
	body := readMap(r)
	id := r.PathValue("team_id")
	if id == "" {
		id = r.URL.Query().Get("team_id")
	}
	if id == "" {
		id = str(body["team_id"])
	}
	if id == "" {
		httpx.WriteError(w, 400, "invalid_request", "team_id required")
		return
	}
	if err := g.Authorize(r, p, authz.ActionTeamRead, authz.Object{Type: authz.ObjectTeam, ID: id}); err != nil {
		g.WriteAuthz(w, r, err)
		return
	}
	t, err := g.Identity().GetTeam(r.Context(), id)
	if err != nil {
		g.WriteIAMError(w, r, err)
		return
	}
	info := teamPublic(t, myTeamRole(g, r, p, id))
	attachRoster(g, r, p, info)
	httpx.WriteJSON(w, 200, map[string]any{
		"team_id":          id,
		"team_info":        info,
		"team_memberships": info["members_with_roles"],
		"keys":             []any{},
	})
}

// myTeamRole reads the caller's own role in a team, for the console's
// role-dependent buttons. A platform administrator who is not a member of the
// team has no team role.
func myTeamRole(g Gate, r *http.Request, p *auth.Principal, teamID string) string {
	roles, err := g.Identity().Memberships(r.Context(), p.UserID)
	if err != nil {
		return ""
	}
	return roles[teamID]
}

// attachRoster puts the roster on the team object the console reads. The
// detail page lists team_info.members_with_roles, so an empty placeholder
// there hides everyone who was added.
func attachRoster(g Gate, r *http.Request, p *auth.Principal, team map[string]any) {
	id, _ := team["team_id"].(string)
	roster := teamRoster(g, r, p, id)
	team["members_with_roles"] = roster
	team["members_count"] = len(roster)
}

// teamRoster reads the team's members. A caller who may not read them, and a
// store failure, both produce an empty list rather than a second error after
// the team itself was already served.
func teamRoster(g Gate, r *http.Request, p *auth.Principal, teamID string) []map[string]any {
	out := []map[string]any{}
	if err := g.Authorize(r, p, authz.ActionMemberRead, authz.Object{Type: authz.ObjectMember, TeamID: teamID}); err != nil {
		return out
	}
	rows, err := g.Identity().ListMembers(r.Context(), teamID)
	if err != nil {
		return out
	}
	for i := range rows {
		out = append(out, memberPublic(&rows[i]))
	}
	return out
}

// TeamUpdate changes a team. A team administrator may change the name and the
// description of their own team; the status and the budget ceiling are a
// platform administrator's decision, and the two are separate actions because
// the matrix separates them.
func TeamUpdate(g Gate, w http.ResponseWriter, r *http.Request) {
	httpx.SetCallID(w, httpx.CallID())
	p := g.RequireUser(w, r)
	if p == nil {
		return
	}
	body := readMap(r)
	id := r.PathValue("team_id")
	if id == "" {
		id = str(body["team_id"])
	}
	if id == "" {
		httpx.WriteError(w, 400, "invalid_request", "team_id required")
		return
	}
	if err := g.Authorize(r, p, authz.ActionTeamWrite, authz.Object{Type: authz.ObjectTeam, ID: id}); err != nil {
		g.WriteAuthz(w, r, err)
		return
	}
	name, description := profilePatch(body)
	if name != nil || description != nil {
		if _, err := g.Identity().UpdateTeamProfile(r.Context(), actorOf(p), id, name, description); err != nil {
			g.WriteIAMError(w, r, err)
			return
		}
	}
	status, budget := adminTeamPatch(body)
	// The model list sits with the budget rather than with the profile: it is
	// the team's ceiling, and a team administrator who could widen it would be
	// granting their own team reach the platform did not give them.
	var models *[]string
	if _, ok := body["models"]; ok {
		v := stringList(body["models"])
		models = &v
	}
	if status != nil || budget != nil || models != nil {
		if err := g.Authorize(r, p, authz.ActionTeamBudget, authz.Object{Type: authz.ObjectTeam, ID: id}); err != nil {
			g.WriteAuthz(w, r, err)
			return
		}
		if _, err := g.Identity().AdminUpdateTeam(r.Context(), actorOf(p), id, status, budget, models); err != nil {
			g.WriteIAMError(w, r, err)
			return
		}
	}
	t, err := g.Identity().GetTeam(r.Context(), id)
	if err != nil {
		g.WriteIAMError(w, r, err)
		return
	}
	httpx.WriteJSON(w, 200, teamPublic(t, ""))
}

// profilePatch reads the two fields a team administrator owns.
func profilePatch(body map[string]any) (*string, *string) {
	var name *string
	if has(body, "team_alias") {
		v := str(body["team_alias"])
		name = &v
	} else if has(body, "team_name") {
		v := str(body["team_name"])
		name = &v
	}
	var description *string
	if has(body, "team_description") {
		v := str(body["team_description"])
		description = &v
	}
	return name, description
}

// adminTeamPatch reads the two fields only a platform administrator owns.
func adminTeamPatch(body map[string]any) (*string, **float64) {
	var status *string
	if v, ok := body["blocked"].(bool); ok {
		s := iam.StatusActive
		if v {
			s = iam.StatusBlocked
		}
		status = &s
	} else {
		status = stringPtr(body, "status")
	}
	return status, optionalFloat(body, "max_budget")
}

// TeamDelete removes a team. Its projects, memberships and key assignments go
// with it, and its keys stop working on their next request.
func TeamDelete(g Gate, w http.ResponseWriter, r *http.Request) {
	httpx.SetCallID(w, httpx.CallID())
	p := g.RequireManage(w, r)
	if p == nil {
		return
	}
	id := r.PathValue("team_id")
	if id == "" {
		id = str(readMap(r)["team_id"])
	}
	if id == "" {
		httpx.WriteError(w, 400, "invalid_request", "team_id required")
		return
	}
	if err := g.Authorize(r, p, authz.ActionTeamDelete, authz.Object{Type: authz.ObjectTeam, ID: id}); err != nil {
		g.WriteAuthz(w, r, err)
		return
	}
	if err := g.Identity().DeleteTeam(r.Context(), actorOf(p), id); err != nil {
		g.WriteIAMError(w, r, err)
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"deleted": true})
}

// TeamMove changes the organization a team belongs to. It is a platform
// administrator route because it re-scopes every access group the team holds.
func TeamMove(g Gate, w http.ResponseWriter, r *http.Request) {
	httpx.SetCallID(w, httpx.CallID())
	p := g.RequireManage(w, r)
	if p == nil {
		return
	}
	body := readMap(r)
	id := r.PathValue("team_id")
	if id == "" {
		id = str(body["team_id"])
	}
	org := str(body["organization_id"])
	if id == "" || org == "" {
		httpx.WriteError(w, 400, "invalid_request", "team_id and organization_id required")
		return
	}
	if err := g.Authorize(r, p, authz.ActionTeamMove, authz.Object{Type: authz.ObjectTeam, ID: id}); err != nil {
		g.WriteAuthz(w, r, err)
		return
	}
	t, err := g.Identity().MoveTeam(r.Context(), actorOf(p), id, org)
	if err != nil {
		g.WriteIAMError(w, r, err)
		return
	}
	httpx.WriteJSON(w, 200, teamPublic(t, ""))
}

// ---------- projects ----------

// ProjectNew creates a project inside a team. A team administrator may.
func ProjectNew(g Gate, w http.ResponseWriter, r *http.Request) {
	httpx.SetCallID(w, httpx.CallID())
	p := g.RequireUser(w, r)
	if p == nil {
		return
	}
	body := readMap(r)
	teamID := str(body["team_id"])
	if err := g.Authorize(r, p, authz.ActionProjectWrite, authz.Object{Type: authz.ObjectProject, TeamID: teamID}); err != nil {
		g.WriteAuthz(w, r, err)
		return
	}
	in := iam.ProjectInput{
		TeamID:    teamID,
		Name:      projectName(body),
		Models:    stringList(body["models"]),
		MaxBudget: floatPtr(body["max_budget"]),
	}
	if in.Name == "" {
		httpx.WriteError(w, 400, "invalid_request", "project_alias required")
		return
	}
	proj, err := g.Identity().CreateProject(r.Context(), actorOf(p), in)
	if err != nil {
		g.WriteIAMError(w, r, err)
		return
	}
	httpx.WriteJSON(w, 200, projectPublic(proj))
}

// projectName reads the project's display name.
func projectName(body map[string]any) string {
	if v := str(body["project_alias"]); v != "" {
		return v
	}
	return str(body["project_name"])
}

// ProjectList lists projects. A platform administrator sees every project; a
// member sees the projects of the teams they belong to; nobody sees a project
// of a team they are not in. The narrowing happens in SQL.
func ProjectList(g Gate, w http.ResponseWriter, r *http.Request) {
	httpx.SetCallID(w, httpx.CallID())
	p := g.RequireUser(w, r)
	if p == nil {
		return
	}
	teams, err := g.TeamFilter(r, p)
	if err != nil {
		g.WriteAuthz(w, r, err)
		return
	}
	// TeamFilter for a non-admin is only direct membership. An organization
	// administrator also sees the projects of every team in that organization.
	if teams != nil && !p.PlatformAdmin() {
		visible, err := g.Identity().ListVisibleTeams(r.Context(), p.UserID, "")
		if err != nil {
			g.WriteIAMError(w, r, err)
			return
		}
		teams = make([]string, 0, len(visible))
		for _, t := range visible {
			teams = append(teams, t.ID)
		}
	}
	if teamID := str(readMap(r)["team_id"]); teamID != "" {
		allowed := teams == nil
		for _, id := range teams {
			if id == teamID {
				allowed = true
				break
			}
		}
		if !allowed {
			httpx.WriteJSON(w, 200, map[string]any{"projects": []any{}})
			return
		}
		teams = []string{teamID}
	}
	rows, err := g.Identity().ListProjects(r.Context(), teams)
	if err != nil {
		g.WriteIAMError(w, r, err)
		return
	}
	out := make([]map[string]any, 0, len(rows))
	for i := range rows {
		out = append(out, projectPublic(&rows[i]))
	}
	httpx.WriteJSON(w, 200, map[string]any{"projects": out})
}

// ProjectInfo reads one project.
func ProjectInfo(g Gate, w http.ResponseWriter, r *http.Request) {
	httpx.SetCallID(w, httpx.CallID())
	p := g.RequireUser(w, r)
	if p == nil {
		return
	}
	body := readMap(r)
	id := r.PathValue("project_id")
	if id == "" {
		id = r.URL.Query().Get("project_id")
	}
	if id == "" {
		id = str(body["project_id"])
	}
	if id == "" {
		httpx.WriteError(w, 400, "invalid_request", "project_id required")
		return
	}
	proj, err := g.Identity().GetProject(r.Context(), id)
	if err != nil {
		g.WriteIAMError(w, r, err)
		return
	}
	if err := g.Authorize(r, p, authz.ActionProjectRead, authz.Object{Type: authz.ObjectProject, ID: proj.ID}); err != nil {
		g.WriteAuthz(w, r, err)
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"project": projectPublic(proj)})
}

// ProjectUpdate changes a project's name, status, narrowing and budget. The
// store refuses a narrowing that is not inside the team's grants.
func ProjectUpdate(g Gate, w http.ResponseWriter, r *http.Request) {
	httpx.SetCallID(w, httpx.CallID())
	p := g.RequireUser(w, r)
	if p == nil {
		return
	}
	body := readMap(r)
	id := r.PathValue("project_id")
	if id == "" {
		id = str(body["project_id"])
	}
	if id == "" {
		httpx.WriteError(w, 400, "invalid_request", "project_id required")
		return
	}
	cur, err := g.Identity().GetProject(r.Context(), id)
	if err != nil {
		g.WriteIAMError(w, r, err)
		return
	}
	if err := g.Authorize(r, p, authz.ActionProjectWrite, authz.Object{Type: authz.ObjectProject, ID: id}); err != nil {
		g.WriteAuthz(w, r, err)
		return
	}
	in := iam.ProjectInput{
		TeamID:    cur.TeamID,
		Name:      projectName(body),
		Status:    str(body["status"]),
		Models:    cur.Models,
		MaxBudget: cur.MaxBudget,
	}
	if has(body, "models") {
		in.Models = stringList(body["models"])
	}
	if has(body, "max_budget") {
		in.MaxBudget = floatPtr(body["max_budget"])
	}
	out, err := g.Identity().UpdateProject(r.Context(), actorOf(p), id, in)
	if err != nil {
		g.WriteIAMError(w, r, err)
		return
	}
	httpx.WriteJSON(w, 200, projectPublic(out))
}

// ProjectDelete removes a project and its keys.
func ProjectDelete(g Gate, w http.ResponseWriter, r *http.Request) {
	httpx.SetCallID(w, httpx.CallID())
	p := g.RequireUser(w, r)
	if p == nil {
		return
	}
	id := str(readMap(r)["project_id"])
	if id == "" {
		httpx.WriteError(w, 400, "invalid_request", "project_id required")
		return
	}
	proj, err := g.Identity().GetProject(r.Context(), id)
	if err != nil {
		g.WriteIAMError(w, r, err)
		return
	}
	if err := g.Authorize(r, p, authz.ActionProjectWrite, authz.Object{Type: authz.ObjectProject, ID: proj.ID}); err != nil {
		g.WriteAuthz(w, r, err)
		return
	}
	if err := g.Identity().DeleteProject(r.Context(), actorOf(p), id); err != nil {
		g.WriteIAMError(w, r, err)
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"deleted": true})
}

// TeamModels reports the models a team may reach. A member may read it for
// their own team.
func TeamModels(g Gate, w http.ResponseWriter, r *http.Request) {
	httpx.SetCallID(w, httpx.CallID())
	p := g.RequireUser(w, r)
	if p == nil {
		return
	}
	teamID := r.PathValue("team_id")
	if teamID == "" {
		teamID = r.URL.Query().Get("team_id")
	}
	if teamID == "" {
		httpx.WriteError(w, 400, "invalid_request", "team_id required")
		return
	}
	if err := g.Authorize(r, p, authz.ActionTeamRead, authz.Object{Type: authz.ObjectTeam, ID: teamID}); err != nil {
		g.WriteAuthz(w, r, err)
		return
	}
	models, err := g.Identity().AllowedModelsForTeam(r.Context(), teamID)
	if err != nil {
		g.WriteIAMError(w, r, err)
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"team_id": teamID, "models": nonNilStrings(models)})
}

// ---------- audit ----------

// AuditLog lists the audit trail. Only a platform administrator may read it.
func AuditLog(g Gate, w http.ResponseWriter, r *http.Request) {
	httpx.SetCallID(w, httpx.CallID())
	p := g.RequireManage(w, r)
	if p == nil {
		return
	}
	if err := g.Authorize(r, p, authz.ActionAuditRead, authz.Object{Type: authz.ObjectAudit}); err != nil {
		g.WriteAuthz(w, r, err)
		return
	}
	limit := queryInt(r, "page_size", 100)
	rows, err := g.Identity().ListAudit(r.Context(), limit, pageOffset(r, limit))
	if err != nil {
		g.WriteIAMError(w, r, err)
		return
	}
	out := make([]map[string]any, 0, len(rows))
	for i := range rows {
		out = append(out, map[string]any{
			"id":          rows[i].ID,
			"ts":          rows[i].TS.UTC().Format(time.RFC3339),
			"actor_id":    rows[i].ActorID,
			"actor_kind":  rows[i].ActorKind,
			"action":      rows[i].Action,
			"object_type": rows[i].ObjectType,
			"object_id":   rows[i].ObjectID,
			"team_id":     emptyNil(rows[i].TeamID),
			"detail":      rows[i].Detail,
		})
	}
	httpx.WriteJSON(w, 200, map[string]any{"audit_logs": out})
}

// ---------- public shapes ----------

// userPublic is the public JSON for an account. The password hash never
// appears; the field is not even carried on the bean the handler holds.
func userPublic(u *iam.User, teams []map[string]any) map[string]any {
	if teams == nil {
		teams = []map[string]any{}
	}
	return map[string]any{
		"user_id":         u.ID,
		"user_email":      u.Email,
		"user_alias":      u.Name,
		"user_name":       u.Name,
		"user_role":       iam.ConsoleRole(u.Role),
		"role":            iam.ConsoleRole(u.Role),
		"status":          u.Status,
		"blocked":         u.Status != iam.StatusActive,
		"max_budget":      floatJSON(u.MaxBudget),
		"spend":           u.Spend,
		"teams":           teams,
		"organizations":   []any{},
		"models":          []string{},
		"created_at":      u.CreatedAt.UTC().Format(time.RFC3339),
		"updated_at":      u.UpdatedAt.UTC().Format(time.RFC3339),
		"key_count":       0,
		"sso_user_id":     nil,
		"tpm_limit":       nil,
		"rpm_limit":       nil,
		"budget_duration": nil,
		"metadata":        map[string]any{},
	}
}

// teamPublic is the public JSON for a team. The role is the caller's own.
func teamPublic(t *iam.Team, role string) map[string]any {
	var budgetDuration any
	return map[string]any{
		"team_id":            t.ID,
		"team_alias":         t.Name,
		"team_name":          t.Name,
		"organization_id":    t.OrganizationID,
		"description":        t.Description,
		"status":             t.Status,
		"blocked":            t.Status == iam.StatusBlocked,
		"max_budget":         floatJSON(t.MaxBudget),
		"spend":              t.Spend,
		"budget_duration":    budgetDuration,
		"user_role":          emptyNil(role),
		"members_with_roles": []any{},
		"members_count":      0,
		"models":             nonNilStrings(t.Models),
		"keys":               []any{},
		"created_at":         t.CreatedAt.UTC().Format(time.RFC3339),
		"updated_at":         t.UpdatedAt.UTC().Format(time.RFC3339),
		"metadata":           map[string]any{},
	}
}

// orgPublic is the public JSON for an organization.
func orgPublic(o *iam.Organization) map[string]any {
	budget := floatJSON(o.MaxBudget)
	return map[string]any{
		"organization_id":    o.ID,
		"organization_alias": o.Name,
		"organization_name":  o.Name,
		"status":             o.Status,
		"max_budget":         budget,
		// The console used to read the budget from a nested table. The budget
		// lives on the organization; this object keeps that one field so an
		// older screen does not crash looking it up.
		"litellm_budget_table": map[string]any{"max_budget": budget},
		"spend":                o.Spend,
		"models":               []string{},
		"members":              []any{},
		"teams":                []any{},
		"created_at":           o.CreatedAt.UTC().Format(time.RFC3339),
		"updated_at":           o.UpdatedAt.UTC().Format(time.RFC3339),
		"metadata":             map[string]any{},
	}
}

// orgView adds the administrators and the teams. Both are properties of the
// organization, not of the account that is reading it.
func orgView(g Gate, r *http.Request, o *iam.Organization) (map[string]any, error) {
	pub := orgPublic(o)
	admins, err := g.Identity().ListOrgAdmins(r.Context(), o.ID)
	if err != nil {
		return nil, err
	}
	members := make([]any, 0, len(admins))
	for _, m := range admins {
		members = append(members, map[string]any{
			"user_id":    m.UserID,
			"user_email": m.Email,
			"user_alias": m.Name,
			"user_role":  m.Role,
			"user":       map[string]any{"user_alias": m.Name, "user_email": m.Email},
		})
	}
	pub["members"] = members
	teams, err := g.Identity().ListTeams(r.Context(), "", o.ID)
	if err != nil {
		return nil, err
	}
	listed := make([]any, 0, len(teams))
	for _, t := range teams {
		listed = append(listed, map[string]any{"team_id": t.ID, "team_alias": t.Name})
	}
	pub["teams"] = listed
	return pub, nil
}

// projectPublic is the public JSON for a project.
func projectPublic(p *iam.Project) map[string]any {
	return map[string]any{
		"project_id":      p.ID,
		"project_alias":   p.Name,
		"project_name":    p.Name,
		"team_id":         p.TeamID,
		"organization_id": emptyNil(p.OrganizationID),
		"status":          p.Status,
		"blocked":         p.Status != iam.StatusActive,
		"models":          nonNilStrings(p.Models),
		"max_budget":      floatJSON(p.MaxBudget),
		"spend":           p.Spend,
		"created_at":      p.CreatedAt.UTC().Format(time.RFC3339),
		"updated_at":      p.UpdatedAt.UTC().Format(time.RFC3339),
		"metadata":        map[string]any{},
	}
}

// actorOf is the audit actor for a write.
func actorOf(p *auth.Principal) iam.Actor {
	return iam.Actor{ID: p.UserID, Kind: string(p.Kind)}
}
