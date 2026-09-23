package server

import (
	"net/http"

	"github.com/sunqirui1987/xhub/internal/httpx"
)

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
		freezeFamily("customer", row)
		out = append(out, map[string]any{
			"user_id":    row["user_id"],
			"spend":      row["spend"],
			"blocked":    row["blocked"],
			"max_budget": row["max_budget"],
		})
	}
	httpx.WriteJSON(w, 200, map[string]any{"object": "list", "data": out, "customers": out, "end_users": out})
}

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
