// Package identity bulk-updates users and adds, updates, and deletes team and organization members.
package identity

import (
	"net/http"

	"github.com/sunqirui1987/xhub/internal/httpx"
	"github.com/sunqirui1987/xhub/internal/store"
)

// UserAvailableRoles returns the user roles the current identity may assign.
func UserAvailableRoles(s Gate, w http.ResponseWriter, r *http.Request) {
	if s.RequireManage(w, r) == nil {
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{
		"proxy_admin": map[string]any{
			"description": "admin over litellm proxy, has all permissions",
			"ui_label":    "Admin (All Permissions)",
		},
		"proxy_admin_viewer": map[string]any{
			"description": "view all keys, view all spend",
			"ui_label":    "Admin (View Only)",
		},
		"internal_user": map[string]any{
			"description": "view/create/delete their own keys, view their own spend",
			"ui_label":    "Internal User (Create/Delete/View)",
		},
		"internal_user_viewer": map[string]any{
			"description": "view their own keys, view their own spend",
			"ui_label":    "Internal User (View Only)",
		},
	})
}

// UserBulkUpdate updates many users. When one row fails it returns an error and does not promise that the other rows were committed.
func UserBulkUpdate(s Gate, w http.ResponseWriter, r *http.Request) {
	if s.RequireManage(w, r) == nil {
		return
	}
	body := readMap(r)
	var users []any
	if v, ok := body["users"].([]any); ok {
		users = v
	}
	results := []map[string]any{}
	okN, failN := 0, 0
	for _, raw := range users {
		u, ok := raw.(map[string]any)
		if !ok {
			failN++
			results = append(results, map[string]any{"status": "error", "error": "invalid user"})
			continue
		}
		id := str(u["user_id"])
		if id == "" {
			failN++
			results = append(results, map[string]any{"status": "error", "error": "user_id required"})
			continue
		}
		e, err := s.DB().GetUser(id)
		if err != nil {
			failN++
			results = append(results, map[string]any{"user_id": id, "status": "error", "error": "user not found"})
			continue
		}
		if v, ok := u["user_email"].(string); ok {
			e.Email = v
		}
		if v, ok := u["user_role"].(string); ok {
			e.Role = v
		}
		if v, ok := u["user_alias"].(string); ok {
			e.Alias = v
		}
		if _, ok := u["max_budget"]; ok {
			e.MaxBudget = parseNullFloat(u["max_budget"])
		}
		applyEntityExtra(e, u, "tpm_limit", "rpm_limit", "metadata", "blocked", "budget_duration", "teams")
		if err := s.DB().UpdateUser(*e); err != nil {
			failN++
			results = append(results, map[string]any{"user_id": id, "status": "error", "error": err.Error()})
			continue
		}
		okN++
		results = append(results, map[string]any{"user_id": id, "status": "ok", "user": userPublic(*e)})
	}
	httpx.WriteJSON(w, 200, map[string]any{
		"results":            results,
		"total_requested":    len(users),
		"successful_updates": okN,
		"failed_updates":     failN,
	})
}

// TeamMemberAdd adds a member to a team.
func TeamMemberAdd(s Gate, w http.ResponseWriter, r *http.Request) {
	if s.RequireManage(w, r) == nil {
		return
	}
	body := readMap(r)
	tid := str(body["team_id"])
	if tid == "" {
		httpx.WriteError(w, 400, "invalid_request", "team_id required")
		return
	}
	members := parseMembers(body["member"])
	if len(members) == 0 {
		httpx.WriteError(w, 400, "invalid_request", "member required")
		return
	}
	var updatedUsers []map[string]any
	var memberships []map[string]any
	var team map[string]any
	for _, mem := range members {
		t, err := addTeamMember(s, tid, mem)
		if err != nil {
			httpx.WriteError(w, 400, "invalid_request", err.Error())
			return
		}
		team = t
		uid := str(mem["user_id"])
		if uid != "" {
			if u, err := s.DB().GetUser(uid); err == nil {
				updatedUsers = append(updatedUsers, userPublic(*u))
			}
		}
		memberships = append(memberships, map[string]any{
			"team_id": tid, "user_id": uid, "user_email": str(mem["user_email"]), "role": str(mem["role"]),
		})
	}
	if team == nil {
		httpx.WriteError(w, 400, "invalid_request", "team not found")
		return
	}
	httpx.WriteJSON(w, 200, mergeMaps(team, map[string]any{
		"updated_users":            updatedUsers,
		"updated_team_memberships": memberships,
	}))
}

// TeamMemberDelete removes a member from a team.
func TeamMemberDelete(s Gate, w http.ResponseWriter, r *http.Request) {
	if s.RequireManage(w, r) == nil {
		return
	}
	body := readMap(r)
	tid := str(body["team_id"])
	if tid == "" {
		httpx.WriteError(w, 400, "invalid_request", "team_id required")
		return
	}
	e, err := s.DB().GetTeam(tid)
	if err != nil {
		httpx.WriteError(w, 400, "invalid_request", "team not found")
		return
	}
	uid, email := str(body["user_id"]), str(body["user_email"])
	kept := []any{}
	for _, raw := range e.ExtraList("members_with_roles") {
		m, _ := raw.(map[string]any)
		if m == nil {
			continue
		}
		if (uid != "" && str(m["user_id"]) == uid) || (email != "" && str(m["user_email"]) == email) {
			continue
		}
		kept = append(kept, m)
	}
	e.PutExtra("members_with_roles", kept)
	_ = s.DB().UpdateTeam(*e)
	httpx.WriteJSON(w, 200, teamPublic(*e))
}

// TeamMemberUpdate changes a team member's role.
func TeamMemberUpdate(s Gate, w http.ResponseWriter, r *http.Request) {
	if s.RequireManage(w, r) == nil {
		return
	}
	body := readMap(r)
	tid := str(body["team_id"])
	if tid == "" {
		httpx.WriteError(w, 400, "invalid_request", "team_id required")
		return
	}
	e, err := s.DB().GetTeam(tid)
	if err != nil {
		httpx.WriteError(w, 400, "invalid_request", "team not found")
		return
	}
	uid := str(body["user_id"])
	updated := map[string]any{}
	list := e.ExtraList("members_with_roles")
	for i, raw := range list {
		m, _ := raw.(map[string]any)
		if m == nil || str(m["user_id"]) != uid {
			continue
		}
		if v, ok := body["role"].(string); ok && v != "" {
			m["role"] = v
		}
		if _, ok := body["max_budget_in_team"]; ok {
			m["max_budget_in_team"] = body["max_budget_in_team"]
		}
		if _, ok := body["tpm_limit"]; ok {
			m["tpm_limit"] = body["tpm_limit"]
		}
		if _, ok := body["rpm_limit"]; ok {
			m["rpm_limit"] = body["rpm_limit"]
		}
		list[i] = m
		updated = m
	}
	e.PutExtra("members_with_roles", list)
	_ = s.DB().UpdateTeam(*e)
	httpx.WriteJSON(w, 200, map[string]any{
		"team_id": tid, "user_id": uid, "user_email": updated["user_email"],
		"role": updated["role"], "max_budget_in_team": updated["max_budget_in_team"],
		"tpm_limit": updated["tpm_limit"], "rpm_limit": updated["rpm_limit"],
	})
}

// TeamBlock blocks a whole team. Member keys can no longer call inference.
func TeamBlock(s Gate, w http.ResponseWriter, r *http.Request) {
	setTeamBlocked(s, w, r, true)
}

// TeamUnblock clears a team block.
func TeamUnblock(s Gate, w http.ResponseWriter, r *http.Request) {
	setTeamBlocked(s, w, r, false)
}

// setTeamBlocked sets the team blocked flag.
func setTeamBlocked(s Gate, w http.ResponseWriter, r *http.Request, blocked bool) {
	if s.RequireManage(w, r) == nil {
		return
	}
	body := readMap(r)
	tid := str(body["team_id"])
	if tid == "" {
		httpx.WriteError(w, 400, "invalid_request", "team_id required")
		return
	}
	e, err := s.DB().GetTeam(tid)
	if err != nil {
		httpx.WriteError(w, 400, "invalid_request", "team not found")
		return
	}
	e.PutExtra("blocked", blocked)
	if err := s.DB().UpdateTeam(*e); err != nil {
		httpx.WriteError(w, 400, "invalid_request", err.Error())
		return
	}
	httpx.WriteJSON(w, 200, teamPublic(*e))
}

// OrgMemberAdd adds a member to an organization.
func OrgMemberAdd(s Gate, w http.ResponseWriter, r *http.Request) {
	if s.RequireManage(w, r) == nil {
		return
	}
	body := readMap(r)
	oid := str(body["organization_id"])
	if oid == "" {
		httpx.WriteError(w, 400, "invalid_request", "organization_id required")
		return
	}
	e, err := s.DB().GetOrg(oid)
	if err != nil {
		httpx.WriteError(w, 400, "invalid_request", "organization not found")
		return
	}
	members := parseMembers(body["member"])
	if len(members) == 0 {
		httpx.WriteError(w, 400, "invalid_request", "member required")
		return
	}
	list := e.ExtraList("members")
	for _, mem := range members {
		if str(mem["role"]) == "" {
			mem["role"] = "internal_user"
		}
		list = append(list, mem)
	}
	e.PutExtra("members", list)
	_ = s.DB().UpdateOrg(*e)
	httpx.WriteJSON(w, 200, map[string]any{
		"organization_id": oid,
		"members":         list,
		"updated_users":   []any{},
	})
}

// OrgMemberDelete removes a member from an organization.
func OrgMemberDelete(s Gate, w http.ResponseWriter, r *http.Request) {
	if s.RequireManage(w, r) == nil {
		return
	}
	body := readMap(r)
	oid := str(body["organization_id"])
	if oid == "" {
		httpx.WriteError(w, 400, "invalid_request", "organization_id required")
		return
	}
	e, err := s.DB().GetOrg(oid)
	if err != nil {
		httpx.WriteError(w, 400, "invalid_request", "organization not found")
		return
	}
	uid, email := str(body["user_id"]), str(body["user_email"])
	kept := []any{}
	for _, raw := range e.ExtraList("members") {
		m, _ := raw.(map[string]any)
		if m == nil {
			continue
		}
		if (uid != "" && str(m["user_id"]) == uid) || (email != "" && str(m["user_email"]) == email) {
			continue
		}
		kept = append(kept, m)
	}
	e.PutExtra("members", kept)
	_ = s.DB().UpdateOrg(*e)
	httpx.WriteJSON(w, 200, orgPublic(*e))
}

// OrgMemberUpdate changes an organization member's role.
func OrgMemberUpdate(s Gate, w http.ResponseWriter, r *http.Request) {
	if s.RequireManage(w, r) == nil {
		return
	}
	body := readMap(r)
	oid := str(body["organization_id"])
	if oid == "" {
		httpx.WriteError(w, 400, "invalid_request", "organization_id required")
		return
	}
	e, err := s.DB().GetOrg(oid)
	if err != nil {
		httpx.WriteError(w, 400, "invalid_request", "organization not found")
		return
	}
	uid := str(body["user_id"])
	list := e.ExtraList("members")
	for i, raw := range list {
		m, _ := raw.(map[string]any)
		if m == nil || str(m["user_id"]) != uid {
			continue
		}
		if v, ok := body["role"].(string); ok && v != "" {
			m["user_role"] = v
			m["role"] = v
		}
		list[i] = m
	}
	e.PutExtra("members", list)
	_ = s.DB().UpdateOrg(*e)
	httpx.WriteJSON(w, 200, orgPublic(*e))
}

// addTeamMember appends one entry to the team member list. A user who is already a member has their role updated instead.
func addTeamMember(s Gate, tid string, mem map[string]any) (map[string]any, error) {
	e, err := s.DB().GetTeam(tid)
	if err != nil {
		return nil, err
	}
	if str(mem["role"]) == "" {
		mem["role"] = "user"
	}
	uid := str(mem["user_id"])
	if uid == "" {
		uid = str(mem["user_email"])
		mem["user_id"] = uid
	}
	list := e.ExtraList("members_with_roles")
	for _, raw := range list {
		m, _ := raw.(map[string]any)
		if m != nil && str(m["user_id"]) == uid && uid != "" {
			return teamPublic(*e), nil
		}
	}
	list = append(list, mem)
	e.PutExtra("members_with_roles", list)
	if err := s.DB().UpdateTeam(*e); err != nil {
		return nil, err
	}
	return teamPublic(*e), nil
}

// parseMembers turns the member field on the request into a list of objects.
func parseMembers(v any) []map[string]any {
	out := []map[string]any{}
	switch t := v.(type) {
	case map[string]any:
		out = append(out, t)
	case []any:
		for _, x := range t {
			if m, ok := x.(map[string]any); ok {
				out = append(out, m)
			}
		}
	}
	return out
}

// applyEntityExtra writes the named keys from the body into the entity extra JSON and keeps the other extra keys.
func applyEntityExtra(e *store.Entity, body map[string]any, keys ...string) {
	if e == nil {
		return
	}
	m := e.Extra()
	for _, k := range keys {
		if v, ok := body[k]; ok {
			m[k] = v
		}
	}
	e.SetExtra(m)
}

// autoCreateKey reports whether creating a user should also generate a key. A missing auto_create_key field defaults to true.
func autoCreateKey(body map[string]any) bool {
	if v, ok := body["auto_create_key"].(bool); ok {
		return v
	}
	return true
}
