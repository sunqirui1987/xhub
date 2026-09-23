package server

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/sunqirui1987/xhub/internal/auth"
	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/httpx"
	"github.com/sunqirui1987/xhub/internal/store"
)

func (s *Server) identityRoutes() {
	s.Mux.HandleFunc("POST /user/new", s.userNew)
	s.Mux.HandleFunc("GET /user/list", s.userList)
	s.Mux.HandleFunc("GET /user/available_users", s.availableUsers)
	s.Mux.HandleFunc("GET /user/available_roles", s.userAvailableRoles)
	s.Mux.HandleFunc("POST /user/bulk_update", s.userBulkUpdate)
	s.Mux.HandleFunc("GET /user/info", s.userInfo)
	s.Mux.HandleFunc("GET /v2/user/info", s.userInfoV2)
	s.Mux.HandleFunc("GET /sso/get/ui_settings", s.uiSettings)
	s.Mux.HandleFunc("GET /global/spend/teams", s.globalSpendTeams)
	s.Mux.HandleFunc("POST /user/update", s.userUpdate)
	s.Mux.HandleFunc("POST /user/delete", s.userDelete)
	s.Mux.HandleFunc("POST /team/new", s.teamNew)
	s.Mux.HandleFunc("GET /team/list", s.teamList)
	s.Mux.HandleFunc("GET /v2/team/list", s.teamListV2)
	s.Mux.HandleFunc("GET /team/available", s.teamAvailable)
	s.Mux.HandleFunc("GET /team/info", s.teamInfo)
	s.Mux.HandleFunc("POST /team/update", s.teamUpdate)
	s.Mux.HandleFunc("POST /team/delete", s.teamDelete)
	s.Mux.HandleFunc("POST /team/member_add", s.teamMemberAdd)
	s.Mux.HandleFunc("POST /team/bulk_member_add", s.teamMemberAdd)
	s.Mux.HandleFunc("POST /team/member_delete", s.teamMemberDelete)
	s.Mux.HandleFunc("POST /team/member_update", s.teamMemberUpdate)
	s.Mux.HandleFunc("POST /team/block", s.teamBlock)
	s.Mux.HandleFunc("POST /team/unblock", s.teamUnblock)
	s.Mux.HandleFunc("POST /organization/new", s.orgNew)
	s.Mux.HandleFunc("GET /organization/list", s.orgList)
	s.Mux.HandleFunc("GET /organization/info", s.orgInfo)
	s.Mux.HandleFunc("PATCH /organization/update", s.orgUpdate)
	s.Mux.HandleFunc("DELETE /organization/delete", s.orgDelete)
	s.Mux.HandleFunc("POST /organization/member_add", s.orgMemberAdd)
	s.Mux.HandleFunc("DELETE /organization/member_delete", s.orgMemberDelete)
	s.Mux.HandleFunc("PATCH /organization/member_update", s.orgMemberUpdate)
	s.Mux.HandleFunc("POST /project/new", s.projectNew)
	s.Mux.HandleFunc("GET /project/new", s.projectNew)
	s.Mux.HandleFunc("GET /project/list", s.projectList)
	s.Mux.HandleFunc("POST /project/list", s.projectList)
	s.Mux.HandleFunc("GET /project/info", s.projectInfo)
	s.Mux.HandleFunc("POST /project/info", s.projectInfo)
	s.Mux.HandleFunc("POST /project/update", s.projectUpdate)
	s.Mux.HandleFunc("GET /project/update", s.projectUpdate)
	s.Mux.HandleFunc("POST /project/delete", s.projectDelete)
	s.Mux.HandleFunc("GET /project/delete", s.projectDelete)
	s.Mux.HandleFunc("POST /budget/new", s.budgetNew)
	s.Mux.HandleFunc("GET /budget/list", s.budgetList)
	s.Mux.HandleFunc("GET /budgets", s.budgetList)
	s.Mux.HandleFunc("GET /management/v1/budgets", s.budgetListPaged)
	s.Mux.HandleFunc("GET /management/v1/spend_logs/end_users", s.spendLogEndUsers)
	s.Mux.HandleFunc("GET /management/v1/spend_logs/users", s.spendLogUsers)
	s.Mux.HandleFunc("POST /budget/info", s.budgetInfo)
	s.Mux.HandleFunc("POST /budget/update", s.budgetUpdate)
	s.Mux.HandleFunc("POST /budget/delete", s.budgetDelete)
	s.Mux.HandleFunc("GET /budget/settings", s.budgetSettings)
	s.Mux.HandleFunc("GET /spend/logs", s.spendLogs)
	s.Mux.HandleFunc("GET /spend/logs/ui", s.spendLogs)
	s.Mux.HandleFunc("GET /global/spend", s.globalSpend)
	s.Mux.HandleFunc("GET /v2/model/info", s.modelInfo)
	s.Mux.HandleFunc("GET /v1/model/info", s.modelInfo)
	s.Mux.HandleFunc("GET /model/info", s.modelInfo)
	s.Mux.HandleFunc("POST /model/new", s.modelNew)
	s.Mux.HandleFunc("POST /model/update", s.modelUpdate)
	s.Mux.HandleFunc("PATCH /model/{model_id}/update", s.modelUpdate)
	s.Mux.HandleFunc("POST /model/delete", s.modelDelete)
	s.Mux.HandleFunc("POST /model/block", s.modelBlock)
	s.Mux.HandleFunc("POST /model/unblock", s.modelUnblock)
	s.Mux.HandleFunc("GET /model_group/info", s.modelGroupInfo)
	s.Mux.HandleFunc("GET /spend/logs/v2", s.spendLogsV2)
	s.Mux.HandleFunc("GET /spend/logs/ui/{request_id}", s.spendLogByID)
	s.Mux.HandleFunc("GET /global/spend/logs", s.globalSpendLogs)
	s.Mux.HandleFunc("GET /global/spend/keys", s.globalSpendKeys)
	s.Mux.HandleFunc("GET /global/spend/models", s.globalSpendModels)
	s.Mux.HandleFunc("GET /global/spend/provider", s.globalSpendProvider)
	s.Mux.HandleFunc("POST /global/spend/end_users", s.globalSpendEndUsers)
	s.Mux.HandleFunc("GET /global/activity", s.globalActivity)
	s.Mux.HandleFunc("GET /global/activity/model", s.globalActivityModel)
	s.Mux.HandleFunc("GET /global/activity/cache_hits", s.globalActivityCacheHits)
	s.Mux.HandleFunc("POST /spend/calculate", s.spendCalculate)
	s.Mux.HandleFunc("GET /spend/keys", s.spendKeys)
	s.Mux.HandleFunc("GET /spend/users", s.spendUsers)
	s.Mux.HandleFunc("GET /spend/tags", s.spendTags)
	s.Mux.HandleFunc("POST /health/test_connection", s.healthTestConnection)
	s.Mux.HandleFunc("GET /health/services", s.healthServices)
	s.Mux.HandleFunc("GET /test", s.healthTest)
	s.Mux.HandleFunc("GET /get/ui_settings", s.uiSettings)
	s.Mux.HandleFunc("GET /get/user_banner", s.emptyOK)
	s.Mux.HandleFunc("GET /get_image", s.emptyOK)
	s.Mux.HandleFunc("GET /get_logo_url", s.emptyOK)
	s.Mux.HandleFunc("GET /get/ui_theme_settings", s.uiSettings)
	s.Mux.HandleFunc("GET /sso/key/generate", s.ssoGenerate)
	s.Mux.HandleFunc("POST /v3/login", s.login)
	s.Mux.HandleFunc("POST /v3/login/exchange", s.loginExchange)
	s.Mux.HandleFunc("POST /flushall", s.flushCache)
	s.Mux.HandleFunc("GET /cache/settings", s.cacheSettings)
	s.Mux.HandleFunc("POST /cache/settings", s.cacheSettings)
	s.Mux.HandleFunc("GET /cache/ping", s.cachePing)
	s.Mux.HandleFunc("GET /ping", s.cachePing)
	s.Mux.HandleFunc("GET /router/settings", s.routerSettings)
	s.Mux.HandleFunc("GET /router/fields", s.routerSettings)
	s.Mux.HandleFunc("GET /get/config/callbacks", s.configCallbacks)
	s.Mux.HandleFunc("POST /config/callback/delete", s.callbackDelete)
	s.Mux.HandleFunc("GET /config/list", s.configList)
	s.Mux.HandleFunc("GET /customer/list", s.customerList)
	s.Mux.HandleFunc("GET /end_user/list", s.customerList)
	s.Mux.HandleFunc("POST /apply_guardrail", s.applyGuardrailHTTP)
	s.Mux.HandleFunc("POST /guardrails/apply_guardrail", s.applyGuardrailHTTP)
	s.Mux.HandleFunc("GET /Users", s.scimUsers)
	s.Mux.HandleFunc("GET /Groups", s.scimGroups)
	s.Mux.HandleFunc("GET /global/spend/tags", s.globalSpendTags)
	s.Mux.HandleFunc("GET /global/spend/all_tag_names", s.globalSpendTagNames)
	s.Mux.HandleFunc("POST /v1/embeddings", s.embeddings)
	s.Mux.HandleFunc("POST /embeddings", s.embeddings)
	s.Mux.HandleFunc("POST /v1/completions", s.completions)
	s.Mux.HandleFunc("POST /completions", s.completions)
	s.Mux.HandleFunc("POST /v1/messages", s.messages)
	s.Mux.HandleFunc("POST /v1/responses", s.responsesAPI)
	s.Mux.HandleFunc("POST /responses", s.responsesAPI)
	s.Mux.HandleFunc("POST /v1/audio/translations", s.audioTranslations)
	s.Mux.HandleFunc("POST /audio/translations", s.audioTranslations)
}

func (s *Server) emptyOK(w http.ResponseWriter, r *http.Request) {
	httpx.SetCallID(w, httpx.CallID())
	httpx.WriteJSON(w, 200, map[string]any{"object": "list", "data": []any{}})
}

func (s *Server) uiSettings(w http.ResponseWriter, r *http.Request) {
	httpx.SetCallID(w, httpx.CallID())
	httpx.WriteJSON(w, 200, map[string]any{
		"status": "ok",
		"values": map[string]any{
			"enabled_ui_pages_internal_users":          nil,
			"enable_projects_ui":                       true,
			"enable_chat_ui":                           true,
			"disable_agents_for_internal_users":        false,
			"allow_agents_for_team_admins":             true,
			"disable_vector_stores_for_internal_users": false,
			"allow_vector_stores_for_team_admins":      true,
		},
		"enabled_ui_pages_internal_users": nil,
		"logo_url":                        "",
		"primary_color":                   "",
		"enabled_pages":                   []string{},
		"default_team_settings":           map[string]any{},
	})
}

func (s *Server) userNew(w http.ResponseWriter, r *http.Request) {
	if s.requireManage(w, r) == nil {
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
	if err := s.Store.InsertUser(e); err != nil {
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
				_, _ = s.addTeamMember(tid, map[string]any{"user_id": e.ID, "user_email": e.Email, "role": "user"})
			}
		}
	}
	pub := userPublic(e)
	if autoCreateKey(body) {
		plain := store.NewPlainKey()
		k, err := keyFromBody(plain, map[string]any{
			"user_id": e.ID, "key_type": "default", "models": body["models"],
			"max_budget": body["max_budget"], "key_alias": body["key_alias"], "duration": body["duration"],
		})
		if err == nil && s.Store.InsertKey(k) == nil {
			kr := keyResponse(k, plain, true)
			pub["key"] = kr["key"]
			pub["token"] = kr["token"]
			pub["expires"] = kr["expires"]
			pub["token_id"] = kr["token_id"]
			pub["key_name"] = kr["key_name"]
		}
	}
	httpx.WriteJSON(w, 200, pub)
}

func (s *Server) userList(w http.ResponseWriter, r *http.Request) {
	if s.requireManage(w, r) == nil {
		return
	}
	list, _ := s.Store.ListUsers()
	counts := s.keyCountByUser()
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

func (s *Server) availableUsers(w http.ResponseWriter, r *http.Request) {
	if s.requireManage(w, r) == nil {
		return
	}
	list, _ := s.Store.ListUsers()
	out := []map[string]any{}
	for _, e := range list {
		out = append(out, userPublic(e))
	}
	httpx.WriteJSON(w, 200, out)
}

func (s *Server) lookupUser(r *http.Request, p *auth.Principal) map[string]any {
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
		if e, err := s.Store.GetUser(id); err == nil {
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

func (s *Server) userInfo(w http.ResponseWriter, r *http.Request) {
	p := s.requireManage(w, r)
	if p == nil {
		return
	}
	if id := r.URL.Query().Get("user_id"); id != "" {
		if _, err := s.Store.GetUser(id); err != nil {
			httpx.WriteError(w, 404, "not_found", "user not found")
			return
		}
	}
	pub := s.lookupUser(r, p)
	httpx.WriteJSON(w, 200, map[string]any{
		"user_id":   pub["user_id"],
		"user_info": pub,
		"keys":      []any{},
		"teams":     []any{},
	})
}

func (s *Server) userInfoV2(w http.ResponseWriter, r *http.Request) {
	p := s.requireManage(w, r)
	if p == nil {
		return
	}
	if id := r.URL.Query().Get("user_id"); id != "" {
		if _, err := s.Store.GetUser(id); err != nil {
			httpx.WriteError(w, 404, "not_found", "user not found")
			return
		}
	}
	httpx.WriteJSON(w, 200, s.lookupUser(r, p))
}

func (s *Server) userUpdate(w http.ResponseWriter, r *http.Request) {
	if s.requireManage(w, r) == nil {
		return
	}
	body := readMap(r)
	id := str(body["user_id"])
	if id == "" {
		httpx.WriteError(w, 400, "invalid_request", "user_id required")
		return
	}
	e, err := s.Store.GetUser(id)
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
	if err := s.Store.UpdateUser(*e); err != nil {
		httpx.WriteError(w, 400, "invalid_request", err.Error())
		return
	}
	httpx.WriteJSON(w, 200, userPublic(*e))
}

func (s *Server) userDelete(w http.ResponseWriter, r *http.Request) {
	if s.requireManage(w, r) == nil {
		return
	}
	body := readMap(r)
	ids := idsFrom(body, "user_ids", "user_id")
	deleted := 0
	for _, id := range ids {
		if err := s.Store.DeleteUser(id); err == nil {
			deleted++
		}
	}
	httpx.WriteJSON(w, 200, map[string]any{"deleted": deleted, "user_ids": ids})
}

func (s *Server) teamNew(w http.ResponseWriter, r *http.Request) {
	p := s.requireManage(w, r)
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
	if err := s.Store.InsertTeam(e); err != nil {
		httpx.WriteError(w, 400, "invalid_request", err.Error())
		return
	}
	if p.UserID != "" {
		if pub, err := s.addTeamMember(id, map[string]any{"user_id": p.UserID, "role": "admin"}); err == nil {
			httpx.WriteJSON(w, 200, s.decorateTeam(pub))
			return
		}
	}
	httpx.WriteJSON(w, 200, s.decorateTeam(teamPublic(e)))
}

func (s *Server) teamList(w http.ResponseWriter, r *http.Request) {
	if s.requireManage(w, r) == nil {
		return
	}
	httpx.WriteJSON(w, 200, s.listTeamsFiltered(r))
}

func (s *Server) teamListV2(w http.ResponseWriter, r *http.Request) {
	if s.requireManage(w, r) == nil {
		return
	}
	out := s.listTeamsFiltered(r)
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

func (s *Server) teamAvailable(w http.ResponseWriter, r *http.Request) {
	if s.requireManage(w, r) == nil {
		return
	}
	httpx.WriteJSON(w, 200, s.listTeamsFiltered(r))
}

func (s *Server) teamInfo(w http.ResponseWriter, r *http.Request) {
	if s.requireManage(w, r) == nil {
		return
	}
	id := r.URL.Query().Get("team_id")
	if id == "" {
		httpx.WriteJSON(w, 200, map[string]any{"status": "ok", "info": nil})
		return
	}
	e, err := s.Store.GetTeam(id)
	if err != nil {
		httpx.WriteError(w, 404, "not_found", "team not found")
		return
	}
	pub := s.decorateTeam(teamPublic(*e))
	httpx.WriteJSON(w, 200, mergeMaps(pub, map[string]any{"info": pub}))
}

func (s *Server) teamUpdate(w http.ResponseWriter, r *http.Request) {
	if s.requireManage(w, r) == nil {
		return
	}
	body := readMap(r)
	id := str(body["team_id"])
	if id == "" {
		httpx.WriteError(w, 400, "invalid_request", "team_id required")
		return
	}
	e, err := s.Store.GetTeam(id)
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
	if err := s.Store.UpdateTeam(*e); err != nil {
		httpx.WriteError(w, 400, "invalid_request", err.Error())
		return
	}
	httpx.WriteJSON(w, 200, s.decorateTeam(teamPublic(*e)))
}

func (s *Server) teamDelete(w http.ResponseWriter, r *http.Request) {
	if s.requireManage(w, r) == nil {
		return
	}
	body := readMap(r)
	ids := idsFrom(body, "team_ids", "team_id")
	deleted := 0
	for _, id := range ids {
		if err := s.Store.DeleteTeam(id); err == nil {
			deleted++
		}
	}
	httpx.WriteJSON(w, 200, map[string]any{"deleted": deleted, "team_ids": ids})
}

func (s *Server) orgNew(w http.ResponseWriter, r *http.Request) {
	if s.requireManage(w, r) == nil {
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
	if err := s.Store.InsertOrg(e); err != nil {
		httpx.WriteError(w, 400, "invalid_request", err.Error())
		return
	}
	httpx.WriteJSON(w, 200, orgPublic(e))
}

func (s *Server) orgList(w http.ResponseWriter, r *http.Request) {
	if s.requireManage(w, r) == nil {
		return
	}
	list, _ := s.Store.ListOrgs()
	out := []map[string]any{}
	for _, e := range list {
		out = append(out, orgPublic(e))
	}
	httpx.WriteJSON(w, 200, out)
}

func (s *Server) orgInfo(w http.ResponseWriter, r *http.Request) {
	if s.requireManage(w, r) == nil {
		return
	}
	id := r.URL.Query().Get("organization_id")
	if id == "" {
		httpx.WriteJSON(w, 200, map[string]any{"status": "ok", "info": nil})
		return
	}
	e, err := s.Store.GetOrg(id)
	if err != nil {
		httpx.WriteError(w, 404, "not_found", "organization not found")
		return
	}
	pub := orgPublic(*e)
	httpx.WriteJSON(w, 200, mergeMaps(pub, map[string]any{"info": pub}))
}

func (s *Server) orgUpdate(w http.ResponseWriter, r *http.Request) {
	if s.requireManage(w, r) == nil {
		return
	}
	body := readMap(r)
	id := str(body["organization_id"])
	if id == "" {
		httpx.WriteError(w, 400, "invalid_request", "organization_id required")
		return
	}
	e, err := s.Store.GetOrg(id)
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
	if err := s.Store.UpdateOrg(*e); err != nil {
		httpx.WriteError(w, 400, "invalid_request", err.Error())
		return
	}
	httpx.WriteJSON(w, 200, orgPublic(*e))
}

func (s *Server) orgDelete(w http.ResponseWriter, r *http.Request) {
	if s.requireManage(w, r) == nil {
		return
	}
	body := readMap(r)
	ids := idsFrom(body, "organization_ids", "organization_id")
	deleted := 0
	for _, id := range ids {
		if err := s.Store.DeleteOrg(id); err == nil {
			deleted++
		}
	}
	httpx.WriteJSON(w, 200, map[string]any{"deleted": deleted, "organization_ids": ids})
}

func (s *Server) projectNew(w http.ResponseWriter, r *http.Request) {
	if s.requireManage(w, r) == nil {
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
	if err := s.Store.InsertProject(e); err != nil {
		httpx.WriteError(w, 400, "invalid_request", err.Error())
		return
	}
	httpx.WriteJSON(w, 200, projectPublic(e))
}

func (s *Server) projectList(w http.ResponseWriter, r *http.Request) {
	if s.requireManage(w, r) == nil {
		return
	}
	list, _ := s.Store.ListProjects()
	out := []map[string]any{}
	for _, e := range list {
		out = append(out, projectPublic(e))
	}
	if out == nil {
		out = []map[string]any{}
	}
	httpx.WriteJSON(w, 200, out)
}

func (s *Server) projectInfo(w http.ResponseWriter, r *http.Request) {
	if s.requireManage(w, r) == nil {
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
	e, err := s.Store.GetProject(id)
	if err != nil {
		httpx.WriteError(w, 404, "not_found", "project not found")
		return
	}
	httpx.WriteJSON(w, 200, projectPublic(*e))
}

func (s *Server) projectUpdate(w http.ResponseWriter, r *http.Request) {
	if s.requireManage(w, r) == nil {
		return
	}
	body := readMap(r)
	id := str(body["project_id"])
	if id == "" {
		httpx.WriteError(w, 400, "invalid_request", "project_id required")
		return
	}
	e, err := s.Store.GetProject(id)
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
	if err := s.Store.UpdateProject(*e); err != nil {
		httpx.WriteError(w, 400, "invalid_request", err.Error())
		return
	}
	httpx.WriteJSON(w, 200, projectPublic(*e))
}

func (s *Server) projectDelete(w http.ResponseWriter, r *http.Request) {
	if s.requireManage(w, r) == nil {
		return
	}
	body := readMap(r)
	ids := idsFrom(body, "project_ids", "project_id")
	deleted := 0
	for _, id := range ids {
		if err := s.Store.DeleteProject(id); err == nil {
			deleted++
		}
	}
	httpx.WriteJSON(w, 200, map[string]any{"deleted": deleted, "project_ids": ids})
}

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

func (s *Server) budgetNew(w http.ResponseWriter, r *http.Request) {
	if s.requireManage(w, r) == nil {
		return
	}
	body := readMap(r)
	id := str(body["budget_id"])
	if id == "" {
		id = "budget_" + httpx.CallID()[:12]
	}
	b := budgetFromBody(id, body)
	_ = s.Store.InsertBudget(b)
	got, err := s.Store.GetBudget(id)
	if err != nil {
		httpx.WriteJSON(w, 200, b.Public())
		return
	}
	httpx.WriteJSON(w, 200, got.Public())
}

func (s *Server) budgetList(w http.ResponseWriter, r *http.Request) {
	if s.requireManage(w, r) == nil {
		return
	}
	list, _ := s.Store.ListBudgets()
	httpx.WriteJSON(w, 200, map[string]any{"data": list})
}

func (s *Server) budgetListPaged(w http.ResponseWriter, r *http.Request) {
	if s.requireManage(w, r) == nil {
		return
	}
	list, _ := s.Store.ListBudgets()
	if list == nil {
		list = []map[string]any{}
	}
	page := queryInt(r, "page", 1, 0)
	size := queryInt(r, "page_size", 50, 100)
	httpx.WriteJSON(w, 200, pagedListBody(r.URL.Path, list, page, size))
}

func (s *Server) spendLogEndUsers(w http.ResponseWriter, r *http.Request) {
	s.spendLogFacet(w, r, "end_user")
}

func (s *Server) spendLogUsers(w http.ResponseWriter, r *http.Request) {
	s.spendLogFacet(w, r, "user")
}

func (s *Server) spendLogFacet(w http.ResponseWriter, r *http.Request, kind string) {
	if s.requireManage(w, r) == nil {
		return
	}
	logs, _ := s.Store.ListSpendLogs()
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

func (s *Server) budgetInfo(w http.ResponseWriter, r *http.Request) {
	if s.requireManage(w, r) == nil {
		return
	}
	body := readMap(r)
	ids := idsFrom(body, "budgets", "budget_id")
	out := []map[string]any{}
	for _, id := range ids {
		if b, err := s.Store.GetBudget(id); err == nil {
			out = append(out, b.Public())
		}
	}
	httpx.WriteJSON(w, 200, map[string]any{"data": out})
}

func (s *Server) budgetUpdate(w http.ResponseWriter, r *http.Request) {
	if s.requireManage(w, r) == nil {
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
	b, err := s.Store.GetBudget(id)
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
	if err := s.Store.UpdateBudget(*b); err != nil {
		httpx.WriteError(w, 400, "invalid_request", err.Error())
		return
	}
	httpx.WriteJSON(w, 200, b.Public())
}

func (s *Server) budgetSettings(w http.ResponseWriter, r *http.Request) {
	if s.requireManage(w, r) == nil {
		return
	}
	id := r.URL.Query().Get("budget_id")
	row := map[string]any{}
	if id != "" {
		if b, err := s.Store.GetBudget(id); err == nil {
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

func (s *Server) budgetDelete(w http.ResponseWriter, r *http.Request) {
	if s.requireManage(w, r) == nil {
		return
	}
	body := readMap(r)
	ids := idsFrom(body, "budget_ids", "budget_id")
	if id := str(body["id"]); id != "" {
		ids = append(ids, id)
	}
	deleted := 0
	for _, id := range ids {
		if err := s.Store.DeleteBudget(id); err == nil {
			deleted++
		}
	}
	httpx.WriteJSON(w, 200, map[string]any{"deleted": deleted})
}

func (s *Server) spendLogs(w http.ResponseWriter, r *http.Request) {
	if s.requireManage(w, r) == nil {
		return
	}
	list, _ := s.Store.ListSpendLogs()
	httpx.WriteJSON(w, 200, map[string]any{"data": list})
}

func (s *Server) globalSpend(w http.ResponseWriter, r *http.Request) {
	if s.requireManage(w, r) == nil {
		return
	}
	list, _ := s.Store.ListSpendLogs()
	var total float64
	for _, row := range list {
		if f, ok := row["spend"].(float64); ok {
			total += f
		}
	}
	httpx.WriteJSON(w, 200, map[string]any{"spend": total, "data": list})
}

func (s *Server) modelInfo(w http.ResponseWriter, r *http.Request) {
	if s.requireManage(w, r) == nil {
		return
	}
	var data []map[string]any
	s.mu.Lock()
	list := append([]config.ModelEntry(nil), s.Cfg.ModelList...)
	s.mu.Unlock()
	for _, m := range list {
		data = append(data, s.modelPublic(m))
	}
	if data == nil {
		data = []map[string]any{}
	}
	httpx.WriteJSON(w, 200, map[string]any{
		"data":         data,
		"total_count":  len(data),
		"current_page": 1,
		"total_pages":  1,
		"size":         len(data),
	})
}

func readMap(r *http.Request) map[string]any {
	var body map[string]any
	b, _ := io.ReadAll(r.Body)
	_ = json.Unmarshal(b, &body)
	if body == nil {
		body = map[string]any{}
	}
	return body
}

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

func mergeMaps(base, extra map[string]any) map[string]any {
	out := cloneMap(base)
	for k, v := range extra {
		out[k] = v
	}
	return out
}

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

func (s *Server) listTeamsFiltered(r *http.Request) []map[string]any {
	list, _ := s.Store.ListTeams()
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
		out = append(out, s.decorateTeam(teamPublic(e)))
	}
	if out == nil {
		out = []map[string]any{}
	}
	return out
}

func (s *Server) decorateTeam(row map[string]any) map[string]any {
	tid := str(row["team_id"])
	keys, _ := s.Store.ListKeys()
	attached := []map[string]any{}
	for _, k := range keys {
		if k.TeamID == tid && tid != "" {
			attached = append(attached, keyResponse(k, "", false))
		}
	}
	row["keys"] = attached
	return row
}

func (s *Server) keyCountByUser() map[string]int {
	out := map[string]int{}
	keys, _ := s.Store.ListKeys()
	for _, k := range keys {
		if k.UserID != "" {
			out[k.UserID]++
		}
	}
	return out
}
