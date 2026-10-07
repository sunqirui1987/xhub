// health.go serves liveness, readiness, and the startup config the dashboard
// reads. These handlers do not change models and do not record spend.

package gateway

import (
	"net/http"
	"os"
	"strings"

	"github.com/sunqirui1987/xhub/internal/httpx"
	"github.com/sunqirui1987/xhub/internal/logx"
	"sync"
)

// LocalGatewayOrigin is the API clients and the console use when no public base is configured.
const LocalGatewayOrigin = "http://localhost:4000"

// PublicOrigin is the OpenAI-compatible API origin. XHUB_PUBLIC_ORIGIN overrides the local default. The value is an origin only: the console is not mounted under it.
// 参数：无。
// 返回 string（string）：OpenAI 兼容接口的源。XHUB_PUBLIC_ORIGIN 优先，否则 http://localhost:4000。
// 调用：仅在 health.go 内使用
// 测试：无直接单测
func PublicOrigin() string {
	if v := strings.TrimSpace(os.Getenv("XHUB_PUBLIC_ORIGIN")); v != "" {
		return strings.TrimRight(v, "/")
	}
	return LocalGatewayOrigin
}

var logTraceOnceHealth sync.Once

// healthLive answers the liveness probe. It does not touch the database and returns status ok.
// 参数 w（http.ResponseWriter）：调用方的 HTTP 响应，状态码和正文写在这里；r（*http.Request）：入站 HTTP 请求，用来读路径、头和正文。
// 调用：仅在 health.go 内使用
// 测试：无直接单测
// 返回：无。状态码和正文写进调用方的响应。
func (s *Server) healthLive(w http.ResponseWriter, r *http.Request) {
	logTraceOnceHealth.Do(func() { logx.Trace("enter gateway.healthLive") })
	logx.Debug("process %s %s step=health check=liveliness", r.Method, r.URL.Path)
	httpx.SetCallID(w, httpx.CallID())
	httpx.WriteJSON(w, 200, map[string]any{"status": "ok"})
}

// healthReady answers the readiness probe. A failed database ping returns 503.
// 参数 w（http.ResponseWriter）：调用方的 HTTP 响应，状态码和正文写在这里；r（*http.Request）：入站 HTTP 请求，用来读路径、头和正文。
// 调用：仅在 health.go 内使用
// 测试：无直接单测
// 返回：无。状态码和正文写进调用方的响应。
func (s *Server) healthReady(w http.ResponseWriter, r *http.Request) {
	httpx.SetCallID(w, httpx.CallID())
	if err := s.Store.DB.Ping(); err != nil {
		logx.Error("process %s %s step=health check=readiness ok=false", r.Method, r.URL.Path)
		httpx.WriteError(w, 503, "not_ready", err.Error())
		return
	}
	logx.Debug("process %s %s step=health check=readiness ok=true", r.Method, r.URL.Path)
	httpx.WriteJSON(w, 200, map[string]any{"status": "ready"})
}

// healthDetails returns readiness details, including litellm_version, so the dashboard can tell which gateway build is running.
// 参数 w（http.ResponseWriter）：调用方的 HTTP 响应，状态码和正文写在这里；r（*http.Request）：入站 HTTP 请求，用来读路径、头和正文。
// 调用：仅在 health.go 内使用
// 测试：无直接单测
// 返回：无。状态码和正文写进调用方的响应。
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
// 参数 w（http.ResponseWriter）：调用方的 HTTP 响应，状态码和正文写在这里；r（*http.Request）：入站 HTTP 请求，用来读路径、头和正文。
// 调用：仅在 health.go 内使用
// 测试：无直接单测
// 返回：无。状态码和正文写进调用方的响应。
func (s *Server) uiConfig(w http.ResponseWriter, r *http.Request) {
	httpx.SetCallID(w, httpx.CallID())
	httpx.WriteJSON(w, 200, map[string]any{
		"admin_ui_disabled":             false,
		"auto_redirect_to_sso":          false,
		"sso_configured":                true,
		"hide_default_credentials_hint": false,
		"is_control_plane":              false,
		"proxy_base_url":                PublicOrigin(),
		"server_root_path":              "/",
	})
}
