// Package identity serves HTTP for users, teams, organizations, and projects. The gateway still registers the routes. This package does not import gateway.
package identity

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/sunqirui1987/xhub/internal/auth"
	"github.com/sunqirui1987/xhub/internal/httpx"
	"github.com/sunqirui1987/xhub/internal/store"
)

// EmptyOK writes an empty success JSON body. It is used for a list that has no data yet or for an accepted no-op.
func EmptyOK(s Gate, w http.ResponseWriter, r *http.Request) {
	httpx.SetCallID(w, httpx.CallID())
	httpx.WriteJSON(w, 200, map[string]any{"object": "list", "data": []any{}})
}

// UiSettings returns the UI settings the dashboard needs. It does not include the master key or the database address.
func UiSettings(s Gate, w http.ResponseWriter, r *http.Request) {
	httpx.SetCallID(w, httpx.CallID())
	values := map[string]any{
		"enabled_ui_pages_internal_users":          nil,
		"enable_projects_ui":                       true,
		"enable_chat_ui":                           true,
		"disable_agents_for_internal_users":        false,
		"allow_agents_for_team_admins":             true,
		"disable_vector_stores_for_internal_users": false,
		"allow_vector_stores_for_team_admins":      true,
	}
	if saved, err := s.DB().GetKV("ui_settings", "values"); err == nil {
		for k, v := range saved {
			if k == "id" || k == "kind" {
				continue
			}
			values[k] = v
		}
	}
	httpx.WriteJSON(w, 200, map[string]any{
		"status":                          "ok",
		"values":                          values,
		"enabled_ui_pages_internal_users": values["enabled_ui_pages_internal_users"],
		"logo_url":                        "",
		"primary_color":                   "",
		"enabled_pages":                   []string{},
		"default_team_settings":           map[string]any{},
	})
}

// UpdateUISettings stores the dashboard page-visibility payload and returns it.
func UpdateUISettings(s Gate, w http.ResponseWriter, r *http.Request) {
	if s.RequireManage(w, r) == nil {
		return
	}
	body := readMap(r)
	raw, _ := json.Marshal(body)
	if err := s.DB().PutKV("ui_settings", "values", string(raw)); err != nil {
		httpx.WriteError(w, 500, "internal", err.Error())
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"status": "ok", "values": body})
}

// UserNew creates a user. It requires a management identity. A missing user_id is generated with a user_ prefix. The password is stored only as a hash. The default role is internal_user. When auto_create_key is true it also issues a key, and the plaintext appears only in this response.
func UserNew(s Gate, w http.ResponseWriter, r *http.Request) {
	if s.RequireManage(w, r) == nil {
		return
	}
	body := readMap(r)
	id := str(body["user_id"])
	if id == "" {
		id = "user_" + httpx.CallID()[:12]
	}
	hash, err := store.HashPassword(str(body["password"]))
	if err != nil {
		httpx.WriteError(w, 500, "internal", "failed to hash password")
		return
	}
	e := store.Entity{
		ID: id, Email: str(body["user_email"]), Role: str(body["user_role"]), Alias: str(body["user_alias"]),
		ModelsJSON: encodeModels(body["models"]), MaxBudget: parseNullFloat(body["max_budget"]),
		Password: hash, CreatedAt: time.Now().UTC(),
	}
	if e.Role == "" {
		e.Role = "internal_user"
	}
	applyEntityExtra(&e, body, "tpm_limit", "rpm_limit", "metadata", "blocked", "budget_duration", "teams", "organizations", "sso_user_id")
	if err := s.DB().InsertUser(e); err != nil {
		httpx.WriteError(w, 400, "invalid_request", err.Error())
		return
	}
	if teams, ok := body["teams"].([]any); ok {
		for _, t := range teams {
			tid := str(t)
			if tid == "" {
				if tm, ok := t.(map[string]any); ok {
					tid = str(tm["team_id"])
				}
			}
			if tid != "" {
				_, _ = addTeamMember(s, tid, map[string]any{"user_id": e.ID, "user_email": e.Email, "role": "user"})
			}
		}
	}
	pub := userPublic(e)
	if autoCreateKey(body) {
		plain := store.NewPlainKey()
		k, err := s.MakeKey(plain, map[string]any{
			"user_id": e.ID, "key_type": "default", "models": body["models"],
			"max_budget": body["max_budget"], "key_alias": body["key_alias"], "duration": body["duration"],
		})
		if err == nil && s.DB().InsertKey(k) == nil {
			kr := s.KeyJSON(k, plain, true)
			pub["key"] = kr["key"]
			pub["token"] = kr["token"]
			pub["expires"] = kr["expires"]
			pub["token_id"] = kr["token_id"]
			pub["key_name"] = kr["key_name"]
		}
	}
	httpx.WriteJSON(w, 200, pub)
}

// UserList lists users one page at a time and includes each user's key count. search, user_email, and role filter the rows. page defaults to 1 and page_size defaults to 25 with a maximum of 100. It requires a management identity. The response does not include password hashes.
func UserList(s Gate, w http.ResponseWriter, r *http.Request) {
	if s.RequireManage(w, r) == nil {
		return
	}
	list, _ := s.DB().ListUsers()
	counts := keyCountByUser(s)
	q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("search")))
	email := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("user_email")))
	role := r.URL.Query().Get("role")
	out := []map[string]any{}
	for _, e := range list {
		row := userPublic(e)
		row["key_count"] = counts[e.ID]
		if q != "" {
			hay := strings.ToLower(e.ID + " " + e.Email + " " + e.Alias)
			if !strings.Contains(hay, q) {
				continue
			}
		}
		if email != "" && !strings.Contains(strings.ToLower(e.Email), email) {
			continue
		}
		if role != "" && e.Role != role {
			continue
		}
		out = append(out, row)
	}
	page := queryInt(r, "page", 1, 0)
	size := queryInt(r, "page_size", 25, 100)
	total := len(out)
	pages := total / size
	if total%size != 0 {
		pages++
	}
	if pages < 1 {
		pages = 1
	}
	httpx.WriteJSON(w, 200, map[string]any{
		"data": sliceMaps(out, page, size), "users": sliceMaps(out, page, size), "total": total,
		"page": page, "page_size": size, "total_pages": pages,
	})
}

// UserFilterUI returns a user array matched by a partial email or user id. The dashboard maps over the result when adding a member, so this is not a paged object.
// team_id is only a hint for the caller's organization scope. It does not exclude users who have not joined that team yet.
func UserFilterUI(s Gate, w http.ResponseWriter, r *http.Request) {
	if s.RequireManage(w, r) == nil {
		return
	}
	list, _ := s.DB().ListUsers()
	email := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("user_email")))
	idq := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("user_id")))
	out := []map[string]any{}
	for _, e := range list {
		if email != "" && !strings.Contains(strings.ToLower(e.Email), email) {
			continue
		}
		if idq != "" && !strings.Contains(strings.ToLower(e.ID), idq) {
			continue
		}
		out = append(out, userPublic(e))
	}
	httpx.WriteJSON(w, 200, out)
}

// AvailableUsers returns every user as public fields for the picker. It requires a management identity, is not paged, and does not include password hashes.
func AvailableUsers(s Gate, w http.ResponseWriter, r *http.Request) {
	if s.RequireManage(w, r) == nil {
		return
	}
	list, _ := s.DB().ListUsers()
	out := []map[string]any{}
	for _, e := range list {
		out = append(out, userPublic(e))
	}
	httpx.WriteJSON(w, 200, out)
}

// lookupUser looks up a user from the query or the current identity. A database miss still returns a public placeholder built from that id and role.
func lookupUser(s Gate, r *http.Request, p *auth.Principal) map[string]any {
	id := r.URL.Query().Get("user_id")
	if id == "" {
		if p != nil && p.Role != "" {
			id = p.Role
			if p.Role == "proxy_admin" || p.Role == "proxy_admin_viewer" {
				id = "admin"
			}
		}
	}
	if id != "" {
		if e, err := s.DB().GetUser(id); err == nil {
			return userPublic(*e)
		}
	}
	role := "internal_user"
	if p != nil && p.Role != "" {
		role = p.Role
	}
	if id == "" {
		id = "admin"
	}
	return userPublic(store.Entity{ID: id, Email: id, Role: role, Alias: id, CreatedAt: time.Now().UTC()})
}

// UserInfo reads one user and wraps it with user_info plus empty keys and teams. A user_id query that is not in the database returns 404. With no user_id it derives a placeholder from the current identity instead of returning 404.
func UserInfo(s Gate, w http.ResponseWriter, r *http.Request) {
	p := s.RequireManage(w, r)
	if p == nil {
		return
	}
	if id := r.URL.Query().Get("user_id"); id != "" {
		if _, err := s.DB().GetUser(id); err != nil {
			httpx.WriteError(w, 404, "not_found", "user not found")
			return
		}
	}
	pub := lookupUser(s, r, p)
	httpx.WriteJSON(w, 200, map[string]any{
		"user_id":   pub["user_id"],
		"user_info": pub,
		"keys":      []any{},
		"teams":     []any{},
	})
}

// UserInfoV2 reads the same user as UserInfo, but the response is the public user object without a user_info wrapper. A missing user_id is still 404.
func UserInfoV2(s Gate, w http.ResponseWriter, r *http.Request) {
	p := s.RequireManage(w, r)
	if p == nil {
		return
	}
	if id := r.URL.Query().Get("user_id"); id != "" {
		if _, err := s.DB().GetUser(id); err != nil {
			httpx.WriteError(w, 404, "not_found", "user not found")
			return
		}
	}
	httpx.WriteJSON(w, 200, lookupUser(s, r, p))
}

// UserUpdate updates a user by user_id. A missing user_id returns 400 and a missing user returns 404. Only fields present on the request change. A new password is hashed again. An empty password leaves the old hash.
func UserUpdate(s Gate, w http.ResponseWriter, r *http.Request) {
	if s.RequireManage(w, r) == nil {
		return
	}
	body := readMap(r)
	id := str(body["user_id"])
	if id == "" {
		httpx.WriteError(w, 400, "invalid_request", "user_id required")
		return
	}
	e, err := s.DB().GetUser(id)
	if err != nil {
		httpx.WriteError(w, 404, "not_found", "user not found")
		return
	}
	if v, ok := body["user_email"].(string); ok {
		e.Email = v
	}
	if v, ok := body["user_role"].(string); ok {
		e.Role = v
	}
	if v, ok := body["user_alias"].(string); ok {
		e.Alias = v
	}
	if _, ok := body["models"]; ok {
		e.ModelsJSON = encodeModels(body["models"])
	}
	if _, ok := body["max_budget"]; ok {
		e.MaxBudget = parseNullFloat(body["max_budget"])
	}
	applyEntityExtra(e, body, "tpm_limit", "rpm_limit", "metadata", "blocked", "budget_duration", "teams", "organizations", "sso_user_id")
	if pw, ok := body["password"].(string); ok && pw != "" {
		hash, err := store.HashPassword(pw)
		if err != nil {
			httpx.WriteError(w, 500, "internal", "failed to hash password")
			return
		}
		e.Password = hash
	}
	if err := s.DB().UpdateUser(*e); err != nil {
		httpx.WriteError(w, 400, "invalid_request", err.Error())
		return
	}
	httpx.WriteJSON(w, 200, userPublic(*e))
}

// UserDelete deletes users by user_ids or user_id. A missing id is skipped and the rest continue. It does not cascade-delete that user's keys. It requires a management identity.
func UserDelete(s Gate, w http.ResponseWriter, r *http.Request) {
	if s.RequireManage(w, r) == nil {
		return
	}
	body := readMap(r)
	ids := idsFrom(body, "user_ids", "user_id")
	deleted := 0
	for _, id := range ids {
		if err := s.DB().DeleteUser(id); err == nil {
			deleted++
		}
	}
	httpx.WriteJSON(w, 200, map[string]any{"deleted": deleted, "user_ids": ids})
}

// TeamNew creates a team. It requires a management identity. A missing team_id is generated. The creator is written into the member list.
func TeamNew(s Gate, w http.ResponseWriter, r *http.Request) {
	p := s.RequireManage(w, r)
	if p == nil {
		return
	}
	body := readMap(r)
	id := str(body["team_id"])
	if id == "" {
		id = "team_" + httpx.CallID()[:12]
	}
	e := store.Entity{
		ID: id, Alias: str(body["team_alias"]), TeamID: str(body["organization_id"]),
		ModelsJSON: encodeModels(body["models"]), MaxBudget: parseNullFloat(body["max_budget"]), CreatedAt: time.Now().UTC(),
	}
	applyEntityExtra(&e, body, "tpm_limit", "rpm_limit", "metadata", "blocked", "budget_duration", "members_with_roles", "guardrails", "object_permission")
	if _, ok := e.Extra()["members_with_roles"]; !ok {
		e.PutExtra("members_with_roles", []any{})
	}
	if err := s.DB().InsertTeam(e); err != nil {
		httpx.WriteError(w, 400, "invalid_request", err.Error())
		return
	}
	if p.UserID != "" {
		if pub, err := addTeamMember(s, id, map[string]any{"user_id": p.UserID, "role": "admin"}); err == nil {
			httpx.WriteJSON(w, 200, decorateTeam(s, pub))
			return
		}
	}
	httpx.WriteJSON(w, 200, decorateTeam(s, teamPublic(e)))
}

// TeamList lists the teams the current identity can see. An admin sees all of them. Any other identity sees only teams they belong to.
func TeamList(s Gate, w http.ResponseWriter, r *http.Request) {
	if s.RequireManage(w, r) == nil {
		return
	}
	httpx.WriteJSON(w, 200, listTeamsFiltered(s, r))
}

// TeamListV2 lists teams one page at a time. page starts at 1. The filter rules match TeamList.
func TeamListV2(s Gate, w http.ResponseWriter, r *http.Request) {
	if s.RequireManage(w, r) == nil {
		return
	}
	out := listTeamsFiltered(s, r)
	page := queryInt(r, "page", 1, 0)
	size := queryInt(r, "page_size", 50, 100)
	total := len(out)
	pages := total / size
	if total%size != 0 {
		pages++
	}
	if pages < 1 {
		pages = 1
	}
	httpx.WriteJSON(w, 200, map[string]any{
		"teams":       sliceMaps(out, page, size),
		"total":       total,
		"page":        page,
		"page_size":   size,
		"total_pages": pages,
	})
}

// TeamAvailable lists teams that can be assigned to a key or a user. A non-admin does not see unrelated teams.
func TeamAvailable(s Gate, w http.ResponseWriter, r *http.Request) {
	if s.RequireManage(w, r) == nil {
		return
	}
	httpx.WriteJSON(w, 200, listTeamsFiltered(s, r))
}

// TeamInfo reads a team by team_id. The response includes team_info and team_memberships, which the dashboard detail page renders. A miss returns 404. It requires a management identity.
func TeamInfo(s Gate, w http.ResponseWriter, r *http.Request) {
	if s.RequireManage(w, r) == nil {
		return
	}
	id := r.URL.Query().Get("team_id")
	if id == "" {
		httpx.WriteJSON(w, 200, map[string]any{"status": "ok", "info": nil})
		return
	}
	e, err := s.DB().GetTeam(id)
	if err != nil {
		httpx.WriteError(w, 404, "not_found", "team not found")
		return
	}
	pub := decorateTeam(s, teamPublic(*e))
	// The dashboard reads team_info and the members page reads team_memberships. info is kept for callers that already read that field.
	httpx.WriteJSON(w, 200, mergeMaps(pub, map[string]any{
		"info":             pub,
		"team_info":        pub,
		"team_memberships": teamMemberships(pub),
	}))
}

// TeamUpdate updates a team by team_id. Budget, models, and members that are absent keep their previous values. A missing team returns 404.
func TeamUpdate(s Gate, w http.ResponseWriter, r *http.Request) {
	if s.RequireManage(w, r) == nil {
		return
	}
	body := readMap(r)
	id := str(body["team_id"])
	if id == "" {
		httpx.WriteError(w, 400, "invalid_request", "team_id required")
		return
	}
	e, err := s.DB().GetTeam(id)
	if err != nil {
		httpx.WriteError(w, 404, "not_found", "team not found")
		return
	}
	if v, ok := body["team_alias"].(string); ok {
		e.Alias = v
	}
	if v, ok := body["organization_id"].(string); ok {
		e.TeamID = v
	}
	if _, ok := body["models"]; ok {
		e.ModelsJSON = encodeModels(body["models"])
	}
	if _, ok := body["max_budget"]; ok {
		e.MaxBudget = parseNullFloat(body["max_budget"])
	}
	applyEntityExtra(e, body, "tpm_limit", "rpm_limit", "metadata", "blocked", "budget_duration", "members_with_roles", "guardrails")
	if err := s.DB().UpdateTeam(*e); err != nil {
		httpx.WriteError(w, 400, "invalid_request", err.Error())
		return
	}
	httpx.WriteJSON(w, 200, decorateTeam(s, teamPublic(*e)))
}

// TeamDelete deletes teams by team_ids or team_id. A missing id is skipped. Member keys are not deleted automatically.
func TeamDelete(s Gate, w http.ResponseWriter, r *http.Request) {
	if s.RequireManage(w, r) == nil {
		return
	}
	body := readMap(r)
	ids := idsFrom(body, "team_ids", "team_id")
	deleted := 0
	for _, id := range ids {
		if err := s.DB().DeleteTeam(id); err == nil {
			deleted++
		}
	}
	httpx.WriteJSON(w, 200, map[string]any{"deleted": deleted, "team_ids": ids})
}

// OrgNew creates an organization. It requires a management identity. A missing organization_id is generated.
func OrgNew(s Gate, w http.ResponseWriter, r *http.Request) {
	if s.RequireManage(w, r) == nil {
		return
	}
	body := readMap(r)
	id := str(body["organization_id"])
	if id == "" {
		id = "org_" + httpx.CallID()[:12]
	}
	e := store.Entity{
		ID: id, Alias: str(body["organization_alias"]), ModelsJSON: encodeModels(body["models"]),
		MaxBudget: parseNullFloat(body["max_budget"]), CreatedAt: time.Now().UTC(),
	}
	applyEntityExtra(&e, body, "budget_id", "metadata", "members")
	if _, ok := e.Extra()["members"]; !ok {
		e.PutExtra("members", []any{})
	}
	if err := s.DB().InsertOrg(e); err != nil {
		httpx.WriteError(w, 400, "invalid_request", err.Error())
		return
	}
	httpx.WriteJSON(w, 200, orgPublic(e))
}

// OrgList lists organizations. It requires a management identity.
func OrgList(s Gate, w http.ResponseWriter, r *http.Request) {
	if s.RequireManage(w, r) == nil {
		return
	}
	list, _ := s.DB().ListOrgs()
	out := []map[string]any{}
	for _, e := range list {
		out = append(out, orgPublic(e))
	}
	httpx.WriteJSON(w, 200, out)
}

// OrgInfo reads an organization by organization_id. A miss returns 404.
func OrgInfo(s Gate, w http.ResponseWriter, r *http.Request) {
	if s.RequireManage(w, r) == nil {
		return
	}
	id := r.URL.Query().Get("organization_id")
	if id == "" {
		httpx.WriteJSON(w, 200, map[string]any{"status": "ok", "info": nil})
		return
	}
	e, err := s.DB().GetOrg(id)
	if err != nil {
		httpx.WriteError(w, 404, "not_found", "organization not found")
		return
	}
	pub := orgPublic(*e)
	httpx.WriteJSON(w, 200, mergeMaps(pub, map[string]any{"info": pub}))
}

// OrgUpdate updates an organization by organization_id. Only fields present on the request change. A missing organization returns 404.
func OrgUpdate(s Gate, w http.ResponseWriter, r *http.Request) {
	if s.RequireManage(w, r) == nil {
		return
	}
	body := readMap(r)
	id := str(body["organization_id"])
	if id == "" {
		httpx.WriteError(w, 400, "invalid_request", "organization_id required")
		return
	}
	e, err := s.DB().GetOrg(id)
	if err != nil {
		httpx.WriteError(w, 404, "not_found", "organization not found")
		return
	}
	if v, ok := body["organization_alias"].(string); ok {
		e.Alias = v
	}
	if _, ok := body["models"]; ok {
		e.ModelsJSON = encodeModels(body["models"])
	}
	if _, ok := body["max_budget"]; ok {
		e.MaxBudget = parseNullFloat(body["max_budget"])
	}
	applyEntityExtra(e, body, "budget_id", "metadata", "members")
	if err := s.DB().UpdateOrg(*e); err != nil {
		httpx.WriteError(w, 400, "invalid_request", err.Error())
		return
	}
	httpx.WriteJSON(w, 200, orgPublic(*e))
}

// OrgDelete deletes organizations by organization_ids or organization_id. A missing id is skipped. Teams are not cascade-deleted.
func OrgDelete(s Gate, w http.ResponseWriter, r *http.Request) {
	if s.RequireManage(w, r) == nil {
		return
	}
	body := readMap(r)
	ids := idsFrom(body, "organization_ids", "organization_id")
	deleted := 0
	for _, id := range ids {
		err := s.DB().DeleteOrg(id)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			httpx.WriteError(w, 400, "invalid_request", err.Error())
			return
		}
		deleted++
	}
	httpx.WriteJSON(w, 200, map[string]any{"deleted": deleted, "organization_ids": ids})
}

// ProjectNew creates a project and attaches it to the team or organization on the request. It requires a management identity. A missing project_id is generated.
func ProjectNew(s Gate, w http.ResponseWriter, r *http.Request) {
	if s.RequireManage(w, r) == nil {
		return
	}
	body := readMap(r)
	id := str(body["project_id"])
	if id == "" {
		id = "proj_" + httpx.CallID()[:12]
	}
	e := store.Entity{
		ID: id, Alias: str(body["project_alias"]), TeamID: str(body["team_id"]),
		ModelsJSON: encodeModels(body["models"]), MaxBudget: parseNullFloat(body["max_budget"]),
		Blocked: boolOf(body["blocked"]), CreatedAt: time.Now().UTC(),
	}
	if err := s.DB().InsertProject(e); err != nil {
		httpx.WriteError(w, 400, "invalid_request", err.Error())
		return
	}
	httpx.WriteJSON(w, 200, projectPublic(e))
}

// ProjectList lists projects. It requires a management identity.
func ProjectList(s Gate, w http.ResponseWriter, r *http.Request) {
	if s.RequireManage(w, r) == nil {
		return
	}
	list, _ := s.DB().ListProjects()
	out := []map[string]any{}
	for _, e := range list {
		out = append(out, projectPublic(e))
	}
	if out == nil {
		out = []map[string]any{}
	}
	httpx.WriteJSON(w, 200, out)
}

// ProjectInfo reads a project by project_id. A miss returns 404.
func ProjectInfo(s Gate, w http.ResponseWriter, r *http.Request) {
	if s.RequireManage(w, r) == nil {
		return
	}
	id := r.URL.Query().Get("project_id")
	if id == "" {
		body := readMap(r)
		id = str(body["project_id"])
	}
	if id == "" {
		httpx.WriteJSON(w, 200, map[string]any{"object": "list", "data": []any{}})
		return
	}
	e, err := s.DB().GetProject(id)
	if err != nil {
		httpx.WriteError(w, 404, "not_found", "project not found")
		return
	}
	httpx.WriteJSON(w, 200, projectPublic(*e))
}

// ProjectUpdate updates a project by project_id. Fields that are absent keep their previous values. A missing project returns 404.
func ProjectUpdate(s Gate, w http.ResponseWriter, r *http.Request) {
	if s.RequireManage(w, r) == nil {
		return
	}
	body := readMap(r)
	id := str(body["project_id"])
	if id == "" {
		httpx.WriteError(w, 400, "invalid_request", "project_id required")
		return
	}
	e, err := s.DB().GetProject(id)
	if err != nil {
		httpx.WriteError(w, 404, "not_found", "project not found")
		return
	}
	if v, ok := body["project_alias"].(string); ok {
		e.Alias = v
	}
	if v, ok := body["team_id"].(string); ok {
		e.TeamID = v
	}
	if _, ok := body["models"]; ok {
		e.ModelsJSON = encodeModels(body["models"])
	}
	if _, ok := body["max_budget"]; ok {
		e.MaxBudget = parseNullFloat(body["max_budget"])
	}
	if _, ok := body["blocked"]; ok {
		e.Blocked = boolOf(body["blocked"])
	}
	if err := s.DB().UpdateProject(*e); err != nil {
		httpx.WriteError(w, 400, "invalid_request", err.Error())
		return
	}
	httpx.WriteJSON(w, 200, projectPublic(*e))
}

// ProjectDelete deletes projects by project_ids or project_id. A missing id is skipped.
func ProjectDelete(s Gate, w http.ResponseWriter, r *http.Request) {
	if s.RequireManage(w, r) == nil {
		return
	}
	body := readMap(r)
	ids := idsFrom(body, "project_ids", "project_id")
	deleted := 0
	for _, id := range ids {
		if err := s.DB().DeleteProject(id); err == nil {
			deleted++
		}
	}
	httpx.WriteJSON(w, 200, map[string]any{"deleted": deleted, "project_ids": ids})
}

// projectPublic is the project JSON shown to callers.
func projectPublic(e store.Entity) map[string]any {
	created := e.CreatedAt.UTC().Format(time.RFC3339)
	if e.CreatedAt.IsZero() {
		created = time.Now().UTC().Format(time.RFC3339)
	}
	m := map[string]any{
		"project_id":           e.ID,
		"project_alias":        e.Alias,
		"team_id":              e.TeamID,
		"models":               e.Models(),
		"spend":                e.Spend,
		"blocked":              e.Blocked,
		"created_at":           created,
		"updated_at":           created,
		"created_by":           "",
		"updated_by":           "",
		"description":          nil,
		"budget_id":            nil,
		"metadata":             map[string]any{},
		"model_spend":          map[string]any{},
		"object_permission_id": nil,
		"litellm_budget_table": nil,
	}
	if e.MaxBudget.Valid {
		m["max_budget"] = e.MaxBudget.Float64
		m["litellm_budget_table"] = map[string]any{"max_budget": e.MaxBudget.Float64}
	} else {
		m["max_budget"] = nil
	}
	return m
}

// BudgetNew creates a named budget. It requires a management identity. A missing amount stays invalid and is not written as 0. A missing budget_id is generated.
func BudgetNew(s Gate, w http.ResponseWriter, r *http.Request) {
	if s.RequireManage(w, r) == nil {
		return
	}
	body := readMap(r)
	id := str(body["budget_id"])
	if id == "" {
		id = "budget_" + httpx.CallID()[:12]
	}
	b := budgetFromBody(id, body)
	_ = s.DB().InsertBudget(b)
	got, err := s.DB().GetBudget(id)
	if err != nil {
		httpx.WriteJSON(w, 200, b.Public())
		return
	}
	httpx.WriteJSON(w, 200, got.Public())
}

// BudgetList lists the public JSON of every budget. It requires a management identity.
func BudgetList(s Gate, w http.ResponseWriter, r *http.Request) {
	if s.RequireManage(w, r) == nil {
		return
	}
	list, _ := s.DB().ListBudgets()
	httpx.WriteJSON(w, 200, map[string]any{"data": list})
}

// BudgetListPaged lists budgets one page at a time. page defaults to 1. It requires a management identity.
func BudgetListPaged(s Gate, w http.ResponseWriter, r *http.Request) {
	if s.RequireManage(w, r) == nil {
		return
	}
	list, _ := s.DB().ListBudgets()
	if list == nil {
		list = []map[string]any{}
	}
	page := queryInt(r, "page", 1, 0)
	size := queryInt(r, "page_size", 50, 100)
	httpx.WriteJSON(w, 200, pagedListBody(r.URL.Path, list, page, size))
}

// SpendLogEndUsers returns end-user ids that have appeared in spend logs. It requires a management identity. With no logs it returns an empty list.
func SpendLogEndUsers(s Gate, w http.ResponseWriter, r *http.Request) {
	spendLogFacet(s, w, r, "end_user")
}

// SpendLogUsers returns user ids that have appeared in spend logs. It requires a management identity.
func SpendLogUsers(s Gate, w http.ResponseWriter, r *http.Request) {
	spendLogFacet(s, w, r, "user")
}

// spendLogFacet lists distinct user or end-user ids from spend logs and returns a paged body.
func spendLogFacet(s Gate, w http.ResponseWriter, r *http.Request, kind string) {
	if s.RequireManage(w, r) == nil {
		return
	}
	logs, _ := s.DB().ListSpendLogs()
	seen := map[string]bool{}
	out := []map[string]any{}
	q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	for _, row := range logs {
		id := str(row["user"])
		if kind == "end_user" {
			id = str(row["end_user"])
		}
		if id == "" || seen[id] {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(id), q) {
			continue
		}
		seen[id] = true
		if kind == "end_user" {
			out = append(out, map[string]any{"end_user": id, "user_id": id})
		} else {
			out = append(out, map[string]any{"user_id": id})
		}
	}
	page := queryInt(r, "page", 1, 0)
	size := queryInt(r, "page_size", 50, 100)
	httpx.WriteJSON(w, 200, pagedListBody(r.URL.Path, out, page, size))
}

// BudgetInfo reads a budget by budget_id. A miss returns 404. It requires a management identity.
func BudgetInfo(s Gate, w http.ResponseWriter, r *http.Request) {
	if s.RequireManage(w, r) == nil {
		return
	}
	body := readMap(r)
	ids := idsFrom(body, "budgets", "budget_id")
	out := []map[string]any{}
	for _, id := range ids {
		if b, err := s.DB().GetBudget(id); err == nil {
			out = append(out, b.Public())
		}
	}
	httpx.WriteJSON(w, 200, map[string]any{"data": out})
}

// BudgetUpdate updates a budget by budget_id. Only limits, duration, and model budgets present on the request change. A missing budget returns 404.
func BudgetUpdate(s Gate, w http.ResponseWriter, r *http.Request) {
	if s.RequireManage(w, r) == nil {
		return
	}
	body := readMap(r)
	id := str(body["budget_id"])
	if id == "" {
		id = str(body["id"])
	}
	if id == "" {
		httpx.WriteError(w, 400, "invalid_request", "budget_id required")
		return
	}
	b, err := s.DB().GetBudget(id)
	if err != nil {
		httpx.WriteError(w, 404, "not_found", "budget not found")
		return
	}
	if _, ok := body["max_budget"]; ok {
		b.MaxBudget = parseNullFloat(body["max_budget"])
	}
	if _, ok := body["soft_budget"]; ok {
		b.SoftBudget = parseNullFloat(body["soft_budget"])
	}
	if _, ok := body["tpm_limit"]; ok {
		b.TPM = parseNullInt(body["tpm_limit"])
	}
	if _, ok := body["rpm_limit"]; ok {
		b.RPM = parseNullInt(body["rpm_limit"])
	}
	if _, ok := body["max_parallel_requests"]; ok {
		b.MaxParallel = parseNullInt(body["max_parallel_requests"])
	}
	if v, ok := body["budget_duration"].(string); ok {
		b.Duration = v
	}
	if _, ok := body["budget_reset_at"]; ok || body["budget_duration"] != nil {
		b.ResetAt = store.ResetAtFrom(str(body["budget_reset_at"]), b.Duration)
	}
	if mm, ok := body["model_max_budget"]; ok && mm != nil {
		raw, _ := json.Marshal(mm)
		b.ModelMaxJSON = string(raw)
	}
	if err := s.DB().UpdateBudget(*b); err != nil {
		httpx.WriteError(w, 400, "invalid_request", err.Error())
		return
	}
	httpx.WriteJSON(w, 200, b.Public())
}

// BudgetSettings returns the field definitions for the budget form. When budget_id is present and found, the current values are included. Otherwise the field values are empty. It requires a management identity.
func BudgetSettings(s Gate, w http.ResponseWriter, r *http.Request) {
	if s.RequireManage(w, r) == nil {
		return
	}
	id := r.URL.Query().Get("budget_id")
	row := map[string]any{}
	if id != "" {
		if b, err := s.DB().GetBudget(id); err == nil {
			row = b.Public()
		}
	}
	fields := []struct{ name, typ, desc string }{
		{"max_budget", "Float", "The max budget for the budget."},
		{"soft_budget", "Float", "The soft budget for the budget."},
		{"tpm_limit", "Integer", "The tokens per minute limit for the budget."},
		{"rpm_limit", "Integer", "The requests per minute limit for the budget."},
		{"budget_duration", "String", "Budget reset period."},
		{"max_parallel_requests", "Integer", "The max number of parallel requests for the budget."},
		{"model_max_budget", "Object", "Specify max budget for a given model."},
	}
	out := make([]map[string]any, 0, len(fields))
	for _, f := range fields {
		out = append(out, map[string]any{
			"field_name":          f.name,
			"field_type":          f.typ,
			"field_description":   f.desc,
			"field_value":         row[f.name],
			"stored_in_db":        true,
			"field_default_value": nil,
		})
	}
	httpx.WriteJSON(w, 200, out)
}

// budgetFromBody builds a budget from the request body. A missing amount stays invalid and is not written as 0.
func budgetFromBody(id string, body map[string]any) store.Budget {
	dur := str(body["budget_duration"])
	b := store.Budget{
		ID:          id,
		MaxBudget:   parseNullFloat(body["max_budget"]),
		SoftBudget:  parseNullFloat(body["soft_budget"]),
		TPM:         parseNullInt(body["tpm_limit"]),
		RPM:         parseNullInt(body["rpm_limit"]),
		MaxParallel: parseNullInt(body["max_parallel_requests"]),
		Duration:    dur,
		ResetAt:     store.ResetAtFrom(str(body["budget_reset_at"]), dur),
	}
	if mm, ok := body["model_max_budget"]; ok && mm != nil {
		raw, _ := json.Marshal(mm)
		b.ModelMaxJSON = string(raw)
	}
	return b
}

// BudgetDelete deletes budgets by budget_ids, budget_id, or id. A failed id does not stop the rest, and deleted in the response is the success count.
func BudgetDelete(s Gate, w http.ResponseWriter, r *http.Request) {
	if s.RequireManage(w, r) == nil {
		return
	}
	body := readMap(r)
	ids := idsFrom(body, "budget_ids", "budget_id")
	if id := str(body["id"]); id != "" {
		ids = append(ids, id)
	}
	deleted := 0
	for _, id := range ids {
		if err := s.DB().DeleteBudget(id); err == nil {
			deleted++
		}
	}
	httpx.WriteJSON(w, 200, map[string]any{"deleted": deleted})
}

// SpendLogs lists spend logs one page at a time. start_date, end_date, model, and request_id filter the rows. page defaults to 1 and page_size defaults to 50. It requires a management identity.
func SpendLogs(s Gate, w http.ResponseWriter, r *http.Request) {
	if s.RequireManage(w, r) == nil {
		return
	}
	list, _ := s.DB().ListSpendLogs()
	filtered := make([]map[string]any, 0, len(list))
	for _, row := range list {
		if !spendTimeInRange(str(row["startTime"]), r) {
			continue
		}
		if model := r.URL.Query().Get("model"); model != "" && str(row["model"]) != model {
			continue
		}
		if id := r.URL.Query().Get("request_id"); id != "" && str(row["request_id"]) != id {
			continue
		}
		filtered = append(filtered, row)
	}
	sortSpendLogs(filtered)
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	if pageSize < 1 {
		pageSize = 50
	}
	total := len(filtered)
	totalPages := 0
	if total > 0 {
		totalPages = (total + pageSize - 1) / pageSize
	}
	start := (page - 1) * pageSize
	if start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	httpx.WriteJSON(w, 200, map[string]any{
		"data":        filtered[start:end],
		"total":       total,
		"page":        page,
		"page_size":   pageSize,
		"total_pages": totalPages,
	})
}

// GlobalSpend returns every spend log and their total. The total adds only spend values that are float64. It requires a management identity.
func GlobalSpend(s Gate, w http.ResponseWriter, r *http.Request) {
	if s.RequireManage(w, r) == nil {
		return
	}
	list, _ := s.DB().ListSpendLogs()
	var total float64
	for _, row := range list {
		if f, ok := row["spend"].(float64); ok {
			total += f
		}
	}
	httpx.WriteJSON(w, 200, map[string]any{"spend": total, "data": list})
}

// ModelInfo returns models in the current process. Secrets inside litellm parameters are masked. It requires a management identity.
// modelId, model, and search narrow the list. page and size paginate when either is present. An empty list is still 200.
func ModelInfo(s Gate, w http.ResponseWriter, r *http.Request) {
	if s.RequireManage(w, r) == nil {
		return
	}
	q := r.URL.Query()
	wantID := q.Get("modelId")
	wantName := q.Get("model")
	search := strings.ToLower(strings.TrimSpace(q.Get("search")))
	excludeAuto := q.Get("exclude_auto_routers") == "true"
	var data []map[string]any
	for _, m := range s.ModelList() {
		pub := s.ModelPublic(m)
		info, _ := pub["model_info"].(map[string]any)
		params, _ := pub["litellm_params"].(map[string]any)
		if wantID != "" && str(info["id"]) != wantID {
			continue
		}
		if wantName != "" && m.ModelName != wantName {
			continue
		}
		if search != "" && !strings.Contains(strings.ToLower(m.ModelName), search) {
			continue
		}
		if excludeAuto {
			model, _ := params["model"].(string)
			if strings.HasPrefix(model, "auto_router/") {
				continue
			}
		}
		data = append(data, pub)
	}
	total := len(data)
	page, size := 1, total
	if q.Get("page") != "" || q.Get("size") != "" {
		page = queryInt(r, "page", 1, 0)
		size = queryInt(r, "size", 50, 1000)
		data = sliceMaps(data, page, size)
	}
	if data == nil {
		data = []map[string]any{}
	}
	pages := 1
	if size > 0 {
		pages = total / size
		if total%size != 0 {
			pages++
		}
		if pages < 1 {
			pages = 1
		}
	}
	httpx.WriteJSON(w, 200, map[string]any{
		"data":         data,
		"total_count":  total,
		"current_page": page,
		"total_pages":  pages,
		"size":         size,
	})
}

// readMap reads a JSON object. An empty body or a parse failure returns an empty map.
func readMap(r *http.Request) map[string]any {
	var body map[string]any
	b, _ := io.ReadAll(r.Body)
	_ = json.Unmarshal(b, &body)
	if body == nil {
		body = map[string]any{}
	}
	return body
}

// userPublic is the user JSON shown to callers. The password hash is not included.
func userPublic(e store.Entity) map[string]any {
	created := e.CreatedAt.UTC().Format(time.RFC3339)
	if e.CreatedAt.IsZero() {
		created = time.Now().UTC().Format(time.RFC3339)
	}
	x := e.Extra()
	teams := x["teams"]
	if teams == nil {
		teams = []any{}
	}
	meta := x["metadata"]
	if meta == nil {
		meta = map[string]any{}
	}
	orgs := x["organizations"]
	if orgs == nil {
		orgs = []any{}
	}
	m := map[string]any{
		"user_id":         e.ID,
		"user_email":      e.Email,
		"user_role":       e.Role,
		"user_alias":      e.Alias,
		"models":          e.Models(),
		"spend":           e.Spend,
		"teams":           teams,
		"organizations":   orgs,
		"created_at":      created,
		"updated_at":      created,
		"key_count":       0,
		"sso_user_id":     x["sso_user_id"],
		"tpm_limit":       x["tpm_limit"],
		"rpm_limit":       x["rpm_limit"],
		"metadata":        meta,
		"blocked":         e.ExtraBool("blocked"),
		"budget_duration": x["budget_duration"],
	}
	if e.MaxBudget.Valid {
		m["max_budget"] = e.MaxBudget.Float64
	} else {
		m["max_budget"] = nil
	}
	return m
}

// teamPublic is the team JSON shown to callers.
func teamPublic(e store.Entity) map[string]any {
	created := e.CreatedAt.UTC().Format(time.RFC3339)
	if e.CreatedAt.IsZero() {
		created = time.Now().UTC().Format(time.RFC3339)
	}
	x := e.Extra()
	members := x["members_with_roles"]
	if members == nil {
		members = []any{}
	}
	meta := x["metadata"]
	if meta == nil {
		meta = map[string]any{}
	}
	perm := x["object_permission"]
	if perm == nil {
		perm = map[string]any{}
	}
	m := map[string]any{
		"team_id":            e.ID,
		"team_alias":         e.Alias,
		"organization_id":    e.TeamID,
		"models":             e.Models(),
		"spend":              e.Spend,
		"members_with_roles": members,
		"members":            members,
		"keys":               []any{},
		"created_at":         created,
		"updated_at":         created,
		"tpm_limit":          x["tpm_limit"],
		"rpm_limit":          x["rpm_limit"],
		"metadata":           meta,
		"blocked":            e.ExtraBool("blocked"),
		"budget_duration":    x["budget_duration"],
		"object_permission":  perm,
	}
	if e.MaxBudget.Valid {
		m["max_budget"] = e.MaxBudget.Float64
	} else {
		m["max_budget"] = nil
	}
	return m
}

// orgPublic is the organization JSON shown to callers.
func orgPublic(e store.Entity) map[string]any {
	created := e.CreatedAt.UTC().Format(time.RFC3339)
	if e.CreatedAt.IsZero() {
		created = time.Now().UTC().Format(time.RFC3339)
	}
	x := e.Extra()
	members := x["members"]
	if members == nil {
		members = []any{}
	}
	meta := x["metadata"]
	if meta == nil {
		meta = map[string]any{}
	}
	budgetTable := map[string]any{
		"max_budget":      nil,
		"tpm_limit":       x["tpm_limit"],
		"rpm_limit":       x["rpm_limit"],
		"budget_duration": x["budget_duration"],
	}
	m := map[string]any{
		"organization_id":      e.ID,
		"organization_alias":   e.Alias,
		"models":               e.Models(),
		"spend":                e.Spend,
		"model_spend":          map[string]any{},
		"created_at":           created,
		"updated_at":           created,
		"created_by":           "",
		"updated_by":           "",
		"members":              members,
		"members_with_roles":   members,
		"teams":                []any{},
		"users":                []any{},
		"budget_id":            x["budget_id"],
		"metadata":             meta,
		"tpm_limit":            x["tpm_limit"],
		"rpm_limit":            x["rpm_limit"],
		"litellm_budget_table": budgetTable,
	}
	if e.MaxBudget.Valid {
		m["max_budget"] = e.MaxBudget.Float64
		budgetTable["max_budget"] = e.MaxBudget.Float64
	} else {
		m["max_budget"] = nil
	}
	return m
}

// idsFrom reads an id list from the body. The plural key wins, otherwise the singular field is used.
func idsFrom(body map[string]any, plural, singular string) []string {
	var out []string
	switch v := body[plural].(type) {
	case []any:
		for _, x := range v {
			if s, ok := x.(string); ok && s != "" {
				out = append(out, s)
			}
		}
	case []string:
		out = append(out, v...)
	}
	if s := str(body[singular]); s != "" {
		out = append(out, s)
	}
	return out
}

// mergeMaps overwrites keys in base that also appear in extra. Keys absent from extra stay.
func mergeMaps(base, extra map[string]any) map[string]any {
	out := cloneMap(base)
	for k, v := range extra {
		out[k] = v
	}
	return out
}

// queryInt reads an integer query parameter. An illegal value uses def, and a value above the cap uses the cap.
func queryInt(r *http.Request, key string, def, max int) int {
	n, err := strconv.Atoi(r.URL.Query().Get(key))
	if err != nil || n < 1 {
		return def
	}
	if max > 0 && n > max {
		return max
	}
	return n
}

// sliceMaps returns one page. Page numbers start at 1. A page past the end returns an empty slice.
func sliceMaps(list []map[string]any, page, size int) []map[string]any {
	if size < 1 {
		size = 50
	}
	if page < 1 {
		page = 1
	}
	start := (page - 1) * size
	if start >= len(list) {
		return []map[string]any{}
	}
	end := start + size
	if end > len(list) {
		end = len(list)
	}
	return list[start:end]
}

// listTeamsFiltered lists teams that match the query. A non-admin sees only teams related to themselves.
func listTeamsFiltered(s Gate, r *http.Request) []map[string]any {
	list, _ := s.DB().ListTeams()
	org := r.URL.Query().Get("organization_id")
	tid := r.URL.Query().Get("team_id")
	alias := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("team_alias")))
	search := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("search")))
	out := []map[string]any{}
	for _, e := range list {
		if org != "" && e.TeamID != org {
			continue
		}
		if tid != "" && e.ID != tid {
			continue
		}
		if alias != "" && !strings.Contains(strings.ToLower(e.Alias), alias) {
			continue
		}
		if search != "" {
			hay := strings.ToLower(e.ID + " " + e.Alias)
			if !strings.Contains(hay, search) {
				continue
			}
		}
		out = append(out, decorateTeam(s, teamPublic(e)))
	}
	if out == nil {
		out = []map[string]any{}
	}
	return out
}

// teamMemberships turns the member list into the team_memberships the dashboard expects. Without a separate budget table the limits stay empty.
func teamMemberships(pub map[string]any) []any {
	raw, _ := pub["members_with_roles"].([]any)
	out := make([]any, 0, len(raw))
	tid := str(pub["team_id"])
	for _, item := range raw {
		member, ok := item.(map[string]any)
		if !ok {
			continue
		}
		out = append(out, map[string]any{
			"user_id":     str(member["user_id"]),
			"team_id":     tid,
			"budget_id":   str(member["budget_id"]),
			"spend":       member["spend"],
			"total_spend": member["total_spend"],
			"litellm_budget_table": map[string]any{
				"budget_id":             str(member["budget_id"]),
				"soft_budget":           nil,
				"max_budget":            member["max_budget_in_team"],
				"max_parallel_requests": nil,
				"tpm_limit":             member["tpm_limit"],
				"rpm_limit":             member["rpm_limit"],
				"model_max_budget":      nil,
				"budget_duration":       member["budget_duration"],
				"budget_reset_at":       member["budget_reset_at"],
				"allowed_models":        member["allowed_models"],
			},
		})
	}
	return out
}

// decorateTeam adds derived fields such as the member count to the team JSON.
func decorateTeam(s Gate, row map[string]any) map[string]any {
	tid := str(row["team_id"])
	keys, _ := s.DB().ListKeys()
	attached := []map[string]any{}
	for _, k := range keys {
		if k.TeamID == tid && tid != "" {
			attached = append(attached, s.KeyJSON(k, "", false))
		}
	}
	row["keys"] = attached
	return row
}

// keyCountByUser counts the virtual keys each user owns.
func keyCountByUser(s Gate) map[string]int {
	out := map[string]int{}
	keys, _ := s.DB().ListKeys()
	for _, k := range keys {
		if k.UserID != "" {
			out[k.UserID]++
		}
	}
	return out
}
