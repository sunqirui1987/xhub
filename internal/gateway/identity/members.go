// 用户批量更新，以及团队和组织成员的增删改。
package identity

import (
	"net/http"

	"github.com/sunqirui1987/xhub/internal/httpx"
	"github.com/sunqirui1987/xhub/internal/store"
)

// 返回当前身份可以分配的用户角色。
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

// 批量更新用户。某一行失败时返回错误，不承诺其余行已提交。
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

// 向团队添加成员。
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

// 从团队移除成员。
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

// 更新团队成员的角色。
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

// 屏蔽整个团队。成员密钥随后不能推理。
func TeamBlock(s Gate, w http.ResponseWriter, r *http.Request) {
	setTeamBlocked(s, w, r, true)
}

// 取消团队屏蔽。
func TeamUnblock(s Gate, w http.ResponseWriter, r *http.Request) {
	setTeamBlocked(s, w, r, false)
}

// 设置团队屏蔽标志。
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

// 向组织添加成员。
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

// 从组织移除成员。
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

// 更新组织成员角色。
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

// 在团队的成员列表里追加一条。已存在的用户改为更新角色。
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

// 把请求里的成员字段收成对象列表。
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

// 把正文中列出的键写进实体 extra，其它 extra 键保留。
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

// 创建用户时是否同时生成一把密钥。缺省不生成。
func autoCreateKey(body map[string]any) bool {
	if v, ok := body["auto_create_key"].(bool); ok {
		return v
	}
	return true
}
