// Package gateway serves liveness, readiness, and the startup config the dashboard reads. These handlers do not change models or record spend.
package gateway

import (
	"net/http"

	"github.com/sunqirui1987/xhub/internal/httpx"
)

// healthLive answers the liveness probe. It does not touch the database and returns status ok.
func (s *Server) healthLive(w http.ResponseWriter, r *http.Request) {
	httpx.SetCallID(w, httpx.CallID())
	httpx.WriteJSON(w, 200, map[string]any{"status": "ok"})
}

// healthReady answers the readiness probe. A failed database ping returns 503.
func (s *Server) healthReady(w http.ResponseWriter, r *http.Request) {
	httpx.SetCallID(w, httpx.CallID())
	if err := s.Store.DB.Ping(); err != nil {
		httpx.WriteError(w, 503, "not_ready", err.Error())
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"status": "ready"})
}

// healthDetails returns readiness details, including litellm_version, so the dashboard can tell which gateway build is running.
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

// uiConfig returns the startup config the dashboard needs, such as the proxy base URL. It does not include secrets.
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
