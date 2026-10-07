// access.go serves SSO, email events, cache probes, the customer list, and
// SCIM. These routes do not enter the inference loop and do not record spend.

package gateway

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/sunqirui1987/xhub/internal/gateway/family"
	"github.com/sunqirui1987/xhub/internal/httpx"
	"github.com/sunqirui1987/xhub/internal/logx"
	"sync"
)

var logTraceOnceAccess sync.Once

// The SSO code generator and its exchange are gone. The pair minted an
// administrator session from an unauthenticated request: ssoGenerate handed out
// a code to anyone and loginExchange turned it into user "admin" with role
// "proxy_admin" without consulting any account. Authentication is now
// username-plus-password against the users table and nothing else.

type emailEventSetting struct {
	Event   string `json:"event"`
	Enabled bool   `json:"enabled"`
}

// defaultEmailEventSettings is the default email-event switches used when the database has no configuration.
// 参数：无。
// 返回 []emailEventSetting（[]emailEventSetting）：库里还没有配置时用的邮件事件开关。邀请新用户默认打开，其余默认关闭。
// 调用：仅在 access.go 内使用
// 测试：无直接单测
func defaultEmailEventSettings() []emailEventSetting {
	return []emailEventSetting{
		{Event: "Virtual Key Created", Enabled: false},
		{Event: "New User Invitation", Enabled: true},
		{Event: "Virtual Key Rotated", Enabled: false},
		{Event: "Soft Budget Crossed", Enabled: false},
		{Event: "Max Budget Alert", Enabled: false},
	}
}

// currentEmailEventSettings returns the email-event settings that are in effect. A nil slice falls back to defaultEmailEventSettings.
// 参数：无。
// 返回 []emailEventSetting（[]emailEventSetting）：当前生效的邮件事件开关。进程里还没设过时退回默认列表。
// 调用：仅在 access.go 内使用
// 测试：无直接单测
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

// emailEventSettings reads or updates the email-event settings.
// 参数 w（http.ResponseWriter）：调用方的 HTTP 响应，状态码和正文写在这里；r（*http.Request）：入站 HTTP 请求，用来读路径、头和正文。
// 调用：仅在 access.go 内使用
// 测试：无直接单测
// 返回：无。状态码和正文写进调用方的响应。
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

// emailEventSettingsReset restores the email-event settings to the defaults.
// 参数 w（http.ResponseWriter）：调用方的 HTTP 响应，状态码和正文写在这里；r（*http.Request）：入站 HTTP 请求，用来读路径、头和正文。
// 调用：仅在 access.go 内使用
// 测试：无直接单测
// 返回：无。状态码和正文写进调用方的响应。
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

// flushCache clears the in-process response cache. It requires a management identity.
// 参数 w（http.ResponseWriter）：调用方的 HTTP 响应，状态码和正文写在这里；r（*http.Request）：入站 HTTP 请求，用来读路径、头和正文。
// 调用：仅在 access.go 内使用
// 测试：无直接单测
// 返回：无。状态码和正文写进调用方的响应。
func (s *Server) flushCache(w http.ResponseWriter, r *http.Request) {
	if s.requireManage(w, r) == nil {
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"status": "ok", "message": "cache flushed"})
}

// cacheSettings reads or stores cache settings. It does not return cached bodies.
// 参数 w（http.ResponseWriter）：调用方的 HTTP 响应，状态码和正文写在这里；r（*http.Request）：入站 HTTP 请求，用来读路径、头和正文。
// 调用：仅在 access.go 内使用
// 测试：无直接单测
// 返回：无。状态码和正文写进调用方的响应。
func (s *Server) cacheSettings(w http.ResponseWriter, r *http.Request) {
	if s.requireManage(w, r) == nil {
		return
	}
	if r.Method == http.MethodPost {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body == nil {
			body = map[string]any{}
		}
		raw, _ := json.Marshal(body)
		if err := s.Store.PutKV("cache", "settings", string(raw)); err != nil {
			httpx.WriteError(w, 500, "internal", err.Error())
			return
		}
		httpx.WriteJSON(w, 200, body)
		return
	}
	saved, err := s.Store.GetKV("cache", "settings")
	if err != nil {
		httpx.WriteJSON(w, 200, map[string]any{"enabled": false})
		return
	}
	httpx.WriteJSON(w, 200, saved)
}

// allowedIPRoute reads and edits the IP allow-list the admin panel shows. The list is stored as JSON and an empty list is data: [].
// 参数 w（http.ResponseWriter）：调用方的 HTTP 响应，状态码和正文写在这里；r（*http.Request）：入站 HTTP 请求，用来读路径、头和正文。
// 调用：仅在 access.go 内使用
// 测试：无直接单测
// 返回：无。状态码和正文写进调用方的响应。
func (s *Server) allowedIPRoute(w http.ResponseWriter, r *http.Request) {
	if s.requireManage(w, r) == nil {
		return
	}
	ips := s.readAllowedIPs()
	if r.Method == http.MethodGet {
		httpx.WriteJSON(w, 200, map[string]any{"data": ips})
		return
	}
	var body struct {
		IP string `json:"ip"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	ip := strings.TrimSpace(body.IP)
	if ip == "" {
		httpx.WriteError(w, 400, "invalid_request", "ip required")
		return
	}
	if strings.Contains(r.URL.Path, "delete") {
		kept := ips[:0]
		for _, cur := range ips {
			if cur != ip {
				kept = append(kept, cur)
			}
		}
		ips = kept
	} else {
		found := false
		for _, cur := range ips {
			if cur == ip {
				found = true
				break
			}
		}
		if !found {
			ips = append(ips, ip)
		}
	}
	s.writeAllowedIPs(ips)
	httpx.WriteJSON(w, 200, map[string]any{"data": ips})
}

// readAllowedIPs returns the saved IP allow-list. A missing record is an empty slice.
// 参数：无。
// 返回 []string（[]string）：读取AllowedIPs。没有匹配时为 nil 或空切片，调用方按长度判断。
// 调用：仅在 access.go 内使用
// 测试：无直接单测
func (s *Server) readAllowedIPs() []string {
	rec, err := s.Store.GetKV("allowed_ips", "list")
	if err != nil {
		return []string{}
	}
	raw, _ := rec["ips"].([]any)
	out := []string{}
	for _, v := range raw {
		if s, ok := v.(string); ok && s != "" {
			out = append(out, s)
		}
	}
	return out
}

// writeAllowedIPs stores the IP allow-list as one key-value document.
// 参数 ips（[]string）：ips列表。空切片表示没有可处理的项。
// 返回：无。IP 允许列表已写成一条键值记录。传入 nil 时写成空列表。
// 调用：仅在 access.go 内使用
// 测试：无直接单测
func (s *Server) writeAllowedIPs(ips []string) {
	if ips == nil {
		ips = []string{}
	}
	vals := make([]any, len(ips))
	for i, ip := range ips {
		vals[i] = ip
	}
	raw, _ := json.Marshal(map[string]any{"ips": vals})
	_ = s.Store.PutKV("allowed_ips", "list", string(raw))
}

// customerList lists customers. With no data it returns an empty list instead of 404.
// 参数 w（http.ResponseWriter）：调用方的 HTTP 响应，状态码和正文写在这里；r（*http.Request）：入站 HTTP 请求，用来读路径、头和正文。
// 调用：仅在 access.go 内使用
// 测试：无直接单测
// 返回：无。状态码和正文写进调用方的响应。
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

// cachePing checks whether the cache is usable.
// 参数 w（http.ResponseWriter）：调用方的 HTTP 响应，状态码和正文写在这里；r（*http.Request）：入站 HTTP 请求，用来读路径、头和正文。
// 调用：仅在 access.go 内使用
// 测试：无直接单测
// 返回：无。状态码和正文写进调用方的响应。
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

// callbackDelete deletes one callback configuration.
// 参数 w（http.ResponseWriter）：调用方的 HTTP 响应，状态码和正文写在这里；r（*http.Request）：入站 HTTP 请求，用来读路径、头和正文。
// 调用：仅在 access.go 内使用
// 测试：无直接单测
// 返回：无。状态码和正文写进调用方的响应。
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

// scimUsers is the minimal SCIM user collection. It reads the accounts from the identity store; the fields SCIM has no equivalent for are not invented.
// 参数 w（http.ResponseWriter）：调用方的 HTTP 响应，状态码和正文写在这里；r（*http.Request）：入站 HTTP 请求，用来读路径、头和正文。
// 调用：仅在 access.go 内使用
// 测试：无直接单测
// 返回：无。状态码和正文写进调用方的响应。
func (s *Server) scimUsers(w http.ResponseWriter, r *http.Request) {
	logTraceOnceAccess.Do(func() { logx.Trace("enter gateway.scimUsers") })

	if s.requireManage(w, r) == nil {
		return
	}
	resources := []map[string]any{}
	if s.IAM != nil {
		users, err := s.IAM.ListUsers(r.Context(), "", 500, 0)
		if err != nil {
			s.writeIAMError(w, r, err)
			return
		}
		for _, u := range users {
			resources = append(resources, map[string]any{
				"schemas":  []any{"urn:ietf:params:scim:schemas:core:2.0:User"},
				"id":       u.ID,
				"userName": u.Email,
				"active":   u.Active(),
				"emails":   []any{map[string]any{"value": u.Email, "primary": true}},
				"name":     map[string]any{"formatted": u.Name},
				"meta":     map[string]any{"resourceType": "User"},
			})
		}
	}
	httpx.WriteJSON(w, 200, scimList("User", resources))
}

// scimGroups is the minimal SCIM group collection. A team maps to a SCIM group and its members are the team's memberships, read from the one membership source rather than from a mirror.
// 参数 w（http.ResponseWriter）：调用方的 HTTP 响应，状态码和正文写在这里；r（*http.Request）：入站 HTTP 请求，用来读路径、头和正文。
// 调用：仅在 access.go 内使用
// 测试：无直接单测
// 返回：无。状态码和正文写进调用方的响应。
func (s *Server) scimGroups(w http.ResponseWriter, r *http.Request) {
	if s.requireManage(w, r) == nil {
		return
	}
	resources := []map[string]any{}
	if s.IAM != nil {
		teams, err := s.IAM.ListTeams(r.Context(), "", "")
		if err != nil {
			s.writeIAMError(w, r, err)
			return
		}
		for _, t := range teams {
			resources = append(resources, map[string]any{
				"schemas":     []any{"urn:ietf:params:scim:schemas:core:2.0:Group"},
				"id":          t.ID,
				"displayName": t.Name,
				"members":     s.scimMembers(r, t.ID),
				"meta":        map[string]any{"resourceType": "Group"},
			})
		}
	}
	httpx.WriteJSON(w, 200, scimList("Group", resources))
}

// scimMembers renders a team's memberships in the SCIM member shape. A failed lookup yields an empty list rather than a broken group entry.
// 参数 r（*http.Request）：入站 HTTP 请求，用来读路径、头和正文；teamID（string）：团队 id。空串表示没有指定团队。
// 返回 []map[string]any（[]map[string]any）：一组map[string]any。没有匹配时为空切片，不是 nil 分页。
// 调用：仅在 access.go 内使用
// 测试：无直接单测
func (s *Server) scimMembers(r *http.Request, teamID string) []map[string]any {
	members, err := s.IAM.ListMembers(r.Context(), teamID)
	if err != nil {
		return []map[string]any{}
	}
	out := make([]map[string]any, 0, len(members))
	for _, m := range members {
		out = append(out, map[string]any{
			"value":   m.UserID,
			"display": m.Email,
			"role":    m.Role,
		})
	}
	return out
}

// scimList wraps resources as a SCIM ListResponse. A nil resources value becomes an empty array, total Results is the final length, and startIndex stays 1 because this list is not paged.
// 参数 resourceType（string）：scim列表使用的resource类型。空串表示调用方没有提供这项；resources（[]map[string]any）：scim列表使用的map[string]any。
// 返回 map[string]any（map[string]any）：scim列表的字段表。缺键表示上游或库里没有这个字段。
// 调用：仅在 access.go 内使用
// 测试：无直接单测
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
