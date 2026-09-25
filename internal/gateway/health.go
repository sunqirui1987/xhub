// 存活、就绪和给控制台的启动配置。不改模型，也不记花费。
package gateway

import (
	"net/http"

	"github.com/sunqirui1987/xhub/internal/httpx"
)

// 存活探针。不访问数据库，成功时 status 为 ok。
func (s *Server) healthLive(w http.ResponseWriter, r *http.Request) {
	httpx.SetCallID(w, httpx.CallID())
	httpx.WriteJSON(w, 200, map[string]any{"status": "ok"})
}

// 就绪探针。数据库 Ping 失败时 503。
func (s *Server) healthReady(w http.ResponseWriter, r *http.Request) {
	httpx.SetCallID(w, httpx.CallID())
	if err := s.Store.DB.Ping(); err != nil {
		httpx.WriteError(w, 503, "not_ready", err.Error())
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"status": "ready"})
}

// 就绪详情。包含 litellm_version，供控制台判断网关版本。
func (s *Server) healthDetails(w http.ResponseWriter, r *http.Request) {
	httpx.SetCallID(w, httpx.CallID())
	httpx.WriteJSON(w, 200, map[string]any{
		"status":          "ready",
		"litellm_version": Version,
		"healthy_count":   1,
		"unhealthy_count": 0,
		"details":         []any{},
	})
}

// 控制台启动配置，例如代理基址。不含秘密。
func (s *Server) uiConfig(w http.ResponseWriter, r *http.Request) {
	httpx.SetCallID(w, httpx.CallID())
	httpx.WriteJSON(w, 200, map[string]any{
		"admin_ui_disabled":             false,
		"auto_redirect_to_sso":          false,
		"sso_configured":                true,
		"hide_default_credentials_hint": false,
		"is_control_plane":              false,
		"proxy_base_url":                "",
		"server_root_path":              "/",
	})
}
