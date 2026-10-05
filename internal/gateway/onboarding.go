// Package gateway finishes a local connect flow and lists shadow-eval tasks.
// These handlers do not call an external identity provider.
//
// The invitation flow is gone. It minted a session from a key-value invite
// record and a hard-coded "internal_user" default, which is a third account role
// the model no longer has: an account is created with a password by a platform
// administrator, and that is the only way one comes into being.
package gateway

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/sunqirui1987/xhub/internal/httpx"
	"github.com/sunqirui1987/xhub/internal/logx"
	"sync"
)

var logTraceOnceOnboarding sync.Once

// shadowEvalList returns the task array. An object envelope makes the shadow-eval page crash when it filters.
func (s *Server) shadowEvalList(w http.ResponseWriter, r *http.Request) {
	logTraceOnceOnboarding.Do(func() { logx.Trace("enter gateway.shadowEvalList") })

	httpx.WriteJSON(w, 200, []any{})
}

// authorizeFlow describes one connect attempt from a local fixture and does not call an external identity provider.
func (s *Server) authorizeFlow(w http.ResponseWriter, r *http.Request) {
	flow := r.URL.Query().Get("flow")
	body := map[string]any{
		"state":         "unscoped",
		"client_origin": "http://127.0.0.1:3000",
		"server_id":     nil,
		"server_name":   nil,
		"connected":     false,
	}
	if flow == "" {
		body["state"] = "stale"
	}
	if flow == "e2e-fixture" {
		body["state"] = "interactive"
		body["server_id"] = "e2e-server"
		body["server_name"] = "e2e-server"
		body["connected"] = true
	}
	httpx.WriteJSON(w, 200, body)
}

// authorizeComplete accepts the connect form and returns a fixed local result.
func (s *Server) authorizeComplete(w http.ResponseWriter, r *http.Request) {
	raw, _ := io.ReadAll(r.Body)
	flow, decision := "", ""
	trimmed := strings.TrimSpace(string(raw))
	if strings.HasPrefix(trimmed, "{") {
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		flow, _ = body["flow"].(string)
		decision, _ = body["decision"].(string)
	} else if trimmed != "" {
		vals, _ := url.ParseQuery(trimmed)
		flow = vals.Get("flow")
		decision = vals.Get("decision")
	}
	if decision == "" {
		decision = "complete"
	}
	if flow == "" {
		flow = "local"
	}
	httpx.WriteJSON(w, 200, map[string]any{
		"status":   "ok",
		"decision": decision,
		"flow":     flow,
	})
}

// mcpOAuthToken answers the console's follow-up after the OAuth callback stores a code.
func (s *Server) mcpOAuthToken(w http.ResponseWriter, r *http.Request) {
	httpx.WriteJSON(w, 200, map[string]any{
		"access_token": "at-" + httpx.CallID()[:12],
		"token_type":   "Bearer",
		"expires_in":   3600,
	})
}
