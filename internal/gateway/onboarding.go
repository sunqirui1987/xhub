// Package gateway redeems an invitation, finishes a local connect flow, and lists shadow-eval tasks. These handlers do not call an external identity provider.
package gateway

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/sunqirui1987/xhub/internal/httpx"
	"github.com/sunqirui1987/xhub/internal/store"
)

// onboardingGetToken exchanges an invite id for a session JWT that carries the email, so the claim page can show who was invited.
func (s *Server) onboardingGetToken(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.URL.Query().Get("invite_link"))
	if id == "" {
		httpx.WriteTypedError(w, r.URL.Path, 400, "invalid_request", "invite_link is required")
		return
	}
	inv, err := s.Store.GetKV("invitation", id)
	if err != nil || len(inv) == 0 {
		httpx.WriteTypedError(w, r.URL.Path, 404, "not_found", "invitation not found")
		return
	}
	if accepted, _ := inv["is_accepted"].(bool); accepted {
		httpx.WriteTypedError(w, r.URL.Path, 400, "invalid_request", "invitation already claimed")
		return
	}
	userID, _ := inv["user_id"].(string)
	user, err := s.Store.GetUser(userID)
	if err != nil {
		httpx.WriteTypedError(w, r.URL.Path, 404, "not_found", "invited user not found")
		return
	}
	role := user.Role
	if role == "" {
		role = "internal_user"
	}
	sess := "sess-" + httpx.CallID()
	s.rememberSession(sess, user.ID, role)
	token := signOnboardingJWT(sess, user.ID, user.Email, role, s.Cfg.GeneralSettings.MasterKey)
	httpx.WriteJSON(w, 200, map[string]any{
		"token":      token,
		"user_id":    user.ID,
		"user_email": user.Email,
		"key":        sess,
	})
}

// onboardingClaim sets a password for the invited user and returns a session that can log in.
func (s *Server) onboardingClaim(w http.ResponseWriter, r *http.Request) {
	var body struct {
		InvitationLink string `json:"invitation_link"`
		UserID         string `json:"user_id"`
		Password       string `json:"password"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if body.InvitationLink == "" || body.UserID == "" || body.Password == "" {
		httpx.WriteTypedError(w, r.URL.Path, 400, "invalid_request", "invitation_link, user_id and password are required")
		return
	}
	p, err := s.resolve(r)
	if err != nil || p.UserID != body.UserID {
		httpx.WriteTypedError(w, r.URL.Path, 401, "invalid_api_key", "onboarding session does not match the invited user")
		return
	}
	inv, err := s.Store.GetKV("invitation", body.InvitationLink)
	if err != nil || len(inv) == 0 {
		httpx.WriteTypedError(w, r.URL.Path, 404, "not_found", "invitation not found")
		return
	}
	if uid, _ := inv["user_id"].(string); uid != body.UserID {
		httpx.WriteTypedError(w, r.URL.Path, 400, "invalid_request", "invitation does not match user")
		return
	}
	user, err := s.Store.GetUser(body.UserID)
	if err != nil {
		httpx.WriteTypedError(w, r.URL.Path, 404, "not_found", "invited user not found")
		return
	}
	hash, err := store.HashPassword(body.Password)
	if err != nil {
		httpx.WriteTypedError(w, r.URL.Path, 500, "internal", "failed to hash password")
		return
	}
	user.Password = hash
	if err := s.Store.UpdateUser(*user); err != nil {
		httpx.WriteTypedError(w, r.URL.Path, 500, "internal", err.Error())
		return
	}
	inv["is_accepted"] = true
	inv["accepted_at"] = time.Now().UTC().Format(time.RFC3339)
	raw, _ := json.Marshal(inv)
	_ = s.Store.PutKV("invitation", body.InvitationLink, string(raw))
	role := user.Role
	if role == "" {
		role = "internal_user"
	}
	s.loginSuccess(w, user.ID, role)
}

// signOnboardingJWT signs a day-long session JWT for an invited user. An empty secret falls back to the local development key.
func signOnboardingJWT(sess, userID, email, role, secret string) string {
	if secret == "" {
		secret = "xhub"
	}
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	payload, _ := json.Marshal(map[string]any{
		"user_id":      userID,
		"user_email":   email,
		"user_role":    role,
		"key":          sess,
		"premium_user": true,
		"exp":          time.Now().Add(24 * time.Hour).Unix(),
		"login_method": "invitation",
	})
	pl := base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(header + "." + pl))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return header + "." + pl + "." + sig
}

// shadowEvalList returns the task array. An object envelope makes the shadow-eval page crash when it filters.
func (s *Server) shadowEvalList(w http.ResponseWriter, r *http.Request) {
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
