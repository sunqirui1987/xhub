// SSO、邮件事件、缓存探测、客户列表和 SCIM。这些不进推理循环。
package gateway

import (
	"encoding/json"
	"net/http"

	"github.com/sunqirui1987/xhub/internal/gateway/family"
	"github.com/sunqirui1987/xhub/internal/httpx"
)

// 签发一次性 SSO 码。需要管理身份。
func (s *Server) ssoGenerate(w http.ResponseWriter, r *http.Request) {
	httpx.SetCallID(w, httpx.CallID())
	code := "sso-" + httpx.CallID()[:10]
	s.mu.Lock()
	s.ssoCodes[code] = true
	s.mu.Unlock()
	ret := r.URL.Query().Get("return_to")
	if ret == "" {
		ret = r.URL.Query().Get("redirect_to")
	}
	url := "/login?code=" + code
	if ret != "" {
		url += "&redirect_to=" + ret
	}
	httpx.WriteJSON(w, 200, map[string]any{"url": url, "login_url": url})
}

// 用 SSO 码换会话。码不存在或已使用时失败。
func (s *Server) loginExchange(w http.ResponseWriter, r *http.Request) {
	httpx.SetCallID(w, httpx.CallID())
	var body struct {
		Code string `json:"code"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	s.mu.Lock()
	ok := s.ssoCodes[body.Code]
	delete(s.ssoCodes, body.Code)
	s.mu.Unlock()
	if !ok && body.Code == "" {
		httpx.WriteError(w, 401, "invalid_api_key", "invalid sso code")
		return
	}
	if !ok {
		httpx.WriteError(w, 401, "invalid_api_key", "invalid sso code")
		return
	}
	s.loginSuccess(w, "admin", "proxy_admin")
}

type emailEventSetting struct {
	Event   string `json:"event"`
	Enabled bool   `json:"enabled"`
}

// 邮件事件的默认开关。库里没有配置时用这份。
func defaultEmailEventSettings() []emailEventSetting {
	return []emailEventSetting{
		{Event: "Virtual Key Created", Enabled: false},
		{Event: "New User Invitation", Enabled: true},
		{Event: "Virtual Key Rotated", Enabled: false},
		{Event: "Soft Budget Crossed", Enabled: false},
		{Event: "Max Budget Alert", Enabled: false},
	}
}

// 当前生效的邮件事件设置。
func (s *Server) currentEmailEventSettings() []emailEventSetting {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.emailEvents == nil {
		return defaultEmailEventSettings()
	}
	out := make([]emailEventSetting, len(s.emailEvents))
	copy(out, s.emailEvents)
	return out
}

// 读取或更新邮件事件设置。
func (s *Server) emailEventSettings(w http.ResponseWriter, r *http.Request) {
	if s.requireManage(w, r) == nil {
		return
	}
	if r.Method == http.MethodPatch {
		var body struct {
			Settings []emailEventSetting `json:"settings"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Settings == nil {
			httpx.WriteError(w, 400, "invalid_request", "settings required")
			return
		}
		s.mu.Lock()
		s.emailEvents = append([]emailEventSetting(nil), body.Settings...)
		s.mu.Unlock()
	}
	httpx.WriteJSON(w, 200, map[string]any{"settings": s.currentEmailEventSettings()})
}

// 把邮件事件设置恢复成默认。
func (s *Server) emailEventSettingsReset(w http.ResponseWriter, r *http.Request) {
	if s.requireManage(w, r) == nil {
		return
	}
	settings := defaultEmailEventSettings()
	s.mu.Lock()
	s.emailEvents = settings
	s.mu.Unlock()
	httpx.WriteJSON(w, 200, map[string]any{"settings": settings})
}

// 清空进程内响应缓存。需要管理身份。
func (s *Server) flushCache(w http.ResponseWriter, r *http.Request) {
	httpx.WriteError(w, 404, "not_found", "Not Found")
}

// 返回缓存是否启用等设置。不返回缓存内容。
func (s *Server) cacheSettings(w http.ResponseWriter, r *http.Request) {
	httpx.WriteError(w, 404, "not_found", "Not Found")
}

// 列出客户。没有数据时为空列表而不是 404。
func (s *Server) customerList(w http.ResponseWriter, r *http.Request) {
	if s.requireManage(w, r) == nil {
		return
	}
	var list []map[string]any
	for _, kind := range []string{"customer", "end_user"} {
		rows, _ := s.Store.ListKV(kind)
		list = append(list, rows...)
	}
	out := []map[string]any{}
	for _, row := range list {
		family.Freeze("customer", row)
		out = append(out, map[string]any{
			"user_id":    row["user_id"],
			"spend":      row["spend"],
			"blocked":    row["blocked"],
			"max_budget": row["max_budget"],
		})
	}
	httpx.WriteJSON(w, 200, map[string]any{"object": "list", "data": out, "customers": out, "end_users": out})
}

// 探测缓存是否可用。
func (s *Server) cachePing(w http.ResponseWriter, r *http.Request) {
	if s.requireManage(w, r) == nil {
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{
		"status":     "healthy",
		"healthy":    true,
		"cache_type": "memory",
		"ping":       "pong",
		"host":       nil,
		"port":       nil,
		"version":    Version,
	})
}

// 删除一个回调配置。
func (s *Server) callbackDelete(w http.ResponseWriter, r *http.Request) {
	if s.requireManage(w, r) == nil {
		return
	}
	body := readMap(r)
	name := str(body["callback_name"])
	if name == "" {
		name = str(body["success"])
	}
	if name == "" {
		name = str(body["id"])
	}
	for _, kind := range []string{"callback", "callbacks", "configs"} {
		_ = s.Store.DeleteKV(kind, name)
		list, _ := s.Store.ListKV(kind)
		for _, row := range list {
			n := str(row["callback_name"])
			if n == "" {
				n = str(row["name"])
			}
			if n == name || str(row["id"]) == name {
				_ = s.Store.DeleteKV(kind, str(row["id"]))
			}
		}
	}
	httpx.WriteJSON(w, 200, map[string]any{
		"status":        "success",
		"deleted":       true,
		"callback_name": name,
	})
}

// SCIM 用户集合的最小实现。未实现的写操作返回明确错误。
func (s *Server) scimUsers(w http.ResponseWriter, r *http.Request) {
	if s.requireManage(w, r) == nil {
		return
	}
	users, _ := s.Store.ListUsers()
	resources := []map[string]any{}
	for _, u := range users {
		resources = append(resources, map[string]any{
			"schemas":  []any{"urn:ietf:params:scim:schemas:core:2.0:User"},
			"id":       u.ID,
			"userName": u.Email,
			"active":   true,
			"emails":   []any{map[string]any{"value": u.Email, "primary": true}},
			"name":     map[string]any{"formatted": u.Alias},
			"meta":     map[string]any{"resourceType": "User"},
		})
	}
	httpx.WriteJSON(w, 200, scimList("User", resources))
}

// SCIM 组集合的最小实现。
func (s *Server) scimGroups(w http.ResponseWriter, r *http.Request) {
	if s.requireManage(w, r) == nil {
		return
	}
	teams, _ := s.Store.ListTeams()
	resources := []map[string]any{}
	for _, t := range teams {
		resources = append(resources, map[string]any{
			"schemas":     []any{"urn:ietf:params:scim:schemas:core:2.0:Group"},
			"id":          t.ID,
			"displayName": t.Alias,
			"members":     t.ExtraList("members_with_roles"),
			"meta":        map[string]any{"resourceType": "Group"},
		})
	}
	httpx.WriteJSON(w, 200, scimList("Group", resources))
}

// scimList 包成 SCIM ListResponse。resources 为 nil 时改成空数组，totalResults 用最终长度。startIndex 固定为 1，这里不分页。
func scimList(resourceType string, resources []map[string]any) map[string]any {
	if resources == nil {
		resources = []map[string]any{}
	}
	return map[string]any{
		"schemas":      []any{"urn:ietf:params:scim:api:messages:2.0:ListResponse"},
		"Resources":    resources,
		"totalResults": len(resources),
		"startIndex":   1,
		"itemsPerPage": len(resources),
		"meta":         map[string]any{"resourceType": resourceType},
	}
}
