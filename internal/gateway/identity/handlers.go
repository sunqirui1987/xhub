// 用户、团队、组织和项目的 HTTP 接口。路由仍由 gateway 注册，本包不引用 gateway。
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

// 返回空的成功 JSON。用于还没有数据的列表或已接受的空操作。
func EmptyOK(s Gate, w http.ResponseWriter, r *http.Request) {
	httpx.SetCallID(w, httpx.CallID())
	httpx.WriteJSON(w, 200, map[string]any{"object": "list", "data": []any{}})
}

// 返回控制台需要的 UI 设置。不包含主密钥和数据库地址。
func UiSettings(s Gate, w http.ResponseWriter, r *http.Request) {
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

// userNew 创建用户。需要管理身份。缺 user_id 时生成 user_ 前缀。密码只存哈希。角色缺省是 internal_user。auto_create_key 为真时顺便发一把密钥，明文只在这次响应里。
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

// userList 分页列出用户，并带上每人的密钥数量。可用 search、user_email、role 过滤。page 缺省 1，page_size 缺省 25、最大 100。需要管理身份。响应不含密码哈希。
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

// availableUsers 列出全部用户的公开字段，供下拉选择。需要管理身份。不分页，也不含密码哈希。
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

// 按查询参数或当前身份找用户。找不到时返回 nil，由调用方写 404。
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

// userInfo 读取一个用户，并套上 user_info、空的 keys 和 teams。查询参数 user_id 在库里不存在时返回 404。没传 user_id 时用当前身份推导一个占位用户，而不是 404。
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

// userInfoV2 与 userInfo 同一用户，但响应就是用户公开对象，不包 user_info。user_id 不存在时同样 404。
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

// userUpdate 按 user_id 更新用户。缺 user_id 返回 400，用户不存在返回 404。只改请求里出现的字段。新密码会重新哈希，空密码不改原哈希。
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

// userDelete 按 user_ids 或 user_id 删除用户。某个 id 不存在时跳过并继续。不级联删除该用户的密钥。需要管理身份。
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

// teamNew 创建团队。需要管理身份。缺 team_id 时生成。创建者会写进成员列表。
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

// teamList 列出当前身份能看到的团队。管理员看全部，其它身份只看自己所属的。
func TeamList(s Gate, w http.ResponseWriter, r *http.Request) {
	if s.RequireManage(w, r) == nil {
		return
	}
	httpx.WriteJSON(w, 200, listTeamsFiltered(s, r))
}

// teamListV2 分页列出团队。page 从 1 开始。过滤规则与 teamList 相同。
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

// teamAvailable 列出可以分配给密钥或用户的团队。非管理员不会看到无关团队。
func TeamAvailable(s Gate, w http.ResponseWriter, r *http.Request) {
	if s.RequireManage(w, r) == nil {
		return
	}
	httpx.WriteJSON(w, 200, listTeamsFiltered(s, r))
}

// teamInfo 按 team_id 读取团队。找不到返回 404。需要管理身份。
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
	httpx.WriteJSON(w, 200, mergeMaps(pub, map[string]any{"info": pub}))
}

// teamUpdate 按 team_id 更新团队。没出现的预算、模型和成员保持原值。团队不存在返回 404。
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

// teamDelete 按 team_ids 或 team_id 删除团队。不存在的 id 跳过。不自动删除成员密钥。
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

// orgNew 创建组织。需要管理身份。缺 organization_id 时生成。
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

// orgList 列出组织。需要管理身份。
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

// orgInfo 按 organization_id 读取组织。找不到返回 404。
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

// orgUpdate 按 organization_id 更新组织。只改请求里出现的字段。不存在返回 404。
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

// orgDelete 按 organization_ids 或 organization_id 删除组织。不存在的 id 跳过。不级联删除其团队。
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

// projectNew 创建项目并挂到请求里的团队或组织。需要管理身份。缺 project_id 时生成。
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

// projectList 列出项目。需要管理身份。
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

// projectInfo 按 project_id 读取项目。找不到返回 404。
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

// projectUpdate 按 project_id 更新项目。没出现的字段保持原值。不存在返回 404。
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

// projectDelete 按 project_ids 或 project_id 删除项目。不存在的 id 跳过。
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

// 项目的对外 JSON。
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

// budgetNew 创建命名预算。需要管理身份。金额缺省保持无效，不写成 0。缺 budget_id 时生成。
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

// budgetList 列出全部预算的公开 JSON。需要管理身份。
func BudgetList(s Gate, w http.ResponseWriter, r *http.Request) {
	if s.RequireManage(w, r) == nil {
		return
	}
	list, _ := s.DB().ListBudgets()
	httpx.WriteJSON(w, 200, map[string]any{"data": list})
}

// budgetListPaged 分页列出预算。page 缺省 1。需要管理身份。
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

// spendLogEndUsers 花费日志里出现过的终端用户 id。需要管理身份。没有日志时为空列表。
func SpendLogEndUsers(s Gate, w http.ResponseWriter, r *http.Request) {
	spendLogFacet(s, w, r, "end_user")
}

// spendLogUsers 花费日志里出现过的用户 id。需要管理身份。
func SpendLogUsers(s Gate, w http.ResponseWriter, r *http.Request) {
	spendLogFacet(s, w, r, "user")
}

// 按用户或终端用户聚合花费日志的一个维度。
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

// budgetInfo 按 budget_id 读取预算。找不到返回 404。需要管理身份。
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

// budgetUpdate 按 budget_id 更新预算。只改请求里出现的限额、时长和模型预算。不存在返回 404。
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

// budgetSettings 返回预算表单的字段定义。传了 budget_id 且能找到时带上当前值，否则字段值为空。需要管理身份。
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

// 用请求体组装预算。缺少的金额保持无效，不写成 0。
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

// budgetDelete 按 budget_ids、budget_id 或 id 删除预算。某个 id 失败时继续删其余的，响应里的 deleted 是成功条数。
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

// spendLogs 分页花费日志。可用 start_date、end_date、model、request_id 过滤。page 缺省 1，page_size 缺省 50。需要管理身份。
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

// globalSpend 全部花费日志及其合计。合计只加类型为 float64 的 spend。需要管理身份。
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

// ModelInfo 返回当前进程里的全部模型。litellm 参数里的密钥已遮罩。需要管理身份。这里不按查询参数过滤，空列表也是 200。
func ModelInfo(s Gate, w http.ResponseWriter, r *http.Request) {
	if s.RequireManage(w, r) == nil {
		return
	}
	var data []map[string]any
	list := s.ModelList()
	for _, m := range list {
		data = append(data, s.ModelPublic(m))
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

// 读取 JSON 对象。空正文或解析失败时返回空表。
func readMap(r *http.Request) map[string]any {
	var body map[string]any
	b, _ := io.ReadAll(r.Body)
	_ = json.Unmarshal(b, &body)
	if body == nil {
		body = map[string]any{}
	}
	return body
}

// 用户的对外 JSON。密码哈希不会出现。
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

// 团队的对外 JSON。
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

// 组织的对外 JSON。
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

// 从正文读取 id 列表。复数键优先，否则用单数字段。
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

// 用 extra 覆盖 base 的同名键。extra 没有的键保留。
func mergeMaps(base, extra map[string]any) map[string]any {
	out := cloneMap(base)
	for k, v := range extra {
		out[k] = v
	}
	return out
}

// 读取整数查询参数。非法或超过上限时用 def 或上限。
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

// 按页切片。页码从 1 开始，越界得到空切片。
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

// 按查询条件列出团队。非管理员只能看到自己相关的团队。
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

// 给团队 JSON 补上成员数等派生字段。
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

// 每个用户拥有的虚拟密钥数量。
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
