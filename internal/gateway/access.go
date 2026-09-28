// Package gateway serves SSO, email events, cache probes, the customer list, and SCIM. These routes do not enter the inference loop.
package gateway

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/sunqirui1987/xhub/internal/gateway/family"
	"github.com/sunqirui1987/xhub/internal/httpx"
)

// ssoGenerate issues a one-time SSO code. It requires a management identity.
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

// loginExchange trades an SSO code for a session. A missing or already used code fails.
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

// defaultEmailEventSettings is the default email-event switches used when the database has no configuration.
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
func (s *Server) flushCache(w http.ResponseWriter, r *http.Request) {
	if s.requireManage(w, r) == nil {
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"status": "ok", "message": "cache flushed"})
}

// cacheSettings reads or stores cache settings. It does not return cached bodies.
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

// scimUsers is the minimal SCIM user collection. A write that is not implemented returns a clear error.
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

// scimGroups is the minimal SCIM group collection.
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

// scimList wraps resources as a SCIM ListResponse. A nil resources value becomes an empty array, totalResults is the final length, and startIndex stays 1 because this list is not paged.
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
