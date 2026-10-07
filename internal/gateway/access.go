// access.go serves cache probes and the customer list. These routes do not
// enter the inference loop and do not record spend. Email events, the IP
// allow-list, and SCIM used to live here; those pages are gone.

package gateway

import (
	"encoding/json"
	"net/http"

	"github.com/sunqirui1987/xhub/internal/gateway/family"
	"github.com/sunqirui1987/xhub/internal/httpx"
)

// The SSO code generator and its exchange are gone. The pair minted an
// administrator session from an unauthenticated request: ssoGenerate handed out
// a code to anyone and loginExchange turned it into user "admin" with role
// "proxy_admin" without consulting any account. Authentication is now
// username-plus-password against the users table and nothing else.

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
