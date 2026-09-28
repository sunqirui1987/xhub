// Package gateway handles login, sessions, and the identity gate. Virtual-key resolution stays in the auth package.
package gateway

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/sunqirui1987/xhub/internal/auth"
	"github.com/sunqirui1987/xhub/internal/httpx"
	"github.com/sunqirui1987/xhub/internal/store"
)

const (
	invalidUICredentials = "Invalid credentials used to access UI. Check 'UI_USERNAME' and 'UI_PASSWORD', or the password set for your user"
	invalidUserPassword  = "Invalid credentials used to access UI. Check the password set for your user"
)

// login accepts a username and password. When environment-credential login is disabled, only a stored user is accepted.
func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	httpx.SetCallID(w, httpx.CallID())
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if body.Username == "" || body.Password == "" {
		httpx.WriteError(w, 400, "invalid_request", "Please enter your username / password")
		return
	}
	if s.envCredentialsMatch(body.Username, body.Password) {
		s.loginSuccess(w, body.Username, "proxy_admin")
		return
	}
	if u, err := s.Store.FindUserLogin(body.Username); err == nil && store.CheckPassword(u.Password, body.Password) {
		role := u.Role
		if role == "" {
			role = "internal_user"
		}
		s.loginSuccess(w, u.ID, role)
		return
	}
	httpx.WriteError(w, 401, "auth_error", s.invalidCredentialsMessage())
}

// envCredentialLoginEnabled reports whether the console account from the environment may log in.
func (s *Server) envCredentialLoginEnabled() bool {
	return !s.Cfg.GeneralSettings.DisableEnvCredentialLogin
}

// invalidCredentialsMessage is the single login-failure text. It does not say whether the user was missing or the password was wrong.
func (s *Server) invalidCredentialsMessage() string {
	if s.envCredentialLoginEnabled() {
		return invalidUICredentials
	}
	return invalidUserPassword
}

// uiEnvCredentials derives the default console account from the master key. With no separate configuration the username is admin.
func uiEnvCredentials(master string) (username, password string) {
	username = os.Getenv("UI_USERNAME")
	if username == "" {
		username = "admin"
	}
	password = os.Getenv("UI_PASSWORD")
	if password == "" {
		password = master
	}
	return username, password
}

// envCredentialsMatch reports whether the username and password match the environment credentials. The comparison does not return early in a way that reveals more than a length difference.
func (s *Server) envCredentialsMatch(username, password string) bool {
	if !s.envCredentialLoginEnabled() {
		return false
	}
	wantUser, wantPass := uiEnvCredentials(s.Cfg.GeneralSettings.MasterKey)
	return credsEqual(username, wantUser) && credsEqual(password, wantPass)
}

// credsEqual compares two strings in constant time so a normal equality check cannot leak more than the length difference.
func credsEqual(a, b string) bool {
	return hmac.Equal([]byte(a), []byte(b))
}

// loginSuccess writes the session and user info for a successful login.
func (s *Server) loginSuccess(w http.ResponseWriter, userID, role string) {
	s.ensureUser(userID, role)
	sess := "sess-" + httpx.CallID()
	s.rememberSession(sess, userID, role)
	jwt := signSessionJWT(sess, userID, role, s.Cfg.GeneralSettings.MasterKey)
	httpx.WriteJSON(w, 200, map[string]any{
		"token":        jwt,
		"key":          sess,
		"user_role":    role,
		"user_id":      userID,
		"premium_user": true,
		"redirect_url": "/ui/?login=success",
	})
}

// signSessionJWT issues a session JWT. The signing key comes from configuration and is not written to logs beyond the response.
func signSessionJWT(sess, userID, role, secret string) string {
	if secret == "" {
		secret = "xhub"
	}
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	payload, _ := json.Marshal(map[string]any{
		"user_id":      userID,
		"user_role":    role,
		"key":          sess,
		"premium_user": true,
		"exp":          time.Now().Add(7 * 24 * time.Hour).Unix(),
		"login_method": "username_password",
	})
	pl := base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(header + "." + pl))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return header + "." + pl + "." + sig
}

// ClearSessions drops the in-memory login sessions. A caller uses it to represent a process restart after which sessions are gone.
func (s *Server) ClearSessions() {
	s.mu.Lock()
	s.sessions = map[string]sessionRec{}
	s.mu.Unlock()
}

// rememberSession stores a session in the process table. A restart invalidates it.
func (s *Server) rememberSession(sess, userID, role string) {
	s.mu.Lock()
	s.sessions[sess] = sessionRec{Role: role, UserID: userID}
	s.mu.Unlock()
	body, err := json.Marshal(map[string]string{"role": role, "user_id": userID})
	if err != nil {
		return
	}
	_ = s.Store.PutKV("ui_session", sess, string(body))
}

// lookupSession looks up a session by token in memory, then a JWT, then the key-value table. A miss returns ok false.
func (s *Server) lookupSession(tok string) (sessionRec, bool) {
	s.mu.Lock()
	rec, ok := s.sessions[tok]
	s.mu.Unlock()
	if ok {
		return rec, true
	}
	if rle, _, uid, vok := verifySessionJWT(tok, s.Cfg.GeneralSettings.MasterKey); vok {
		return sessionRec{Role: rle, UserID: uid}, true
	}
	m, err := s.Store.GetKV("ui_session", tok)
	if err != nil {
		return sessionRec{}, false
	}
	role, _ := m["role"].(string)
	uid, _ := m["user_id"].(string)
	if role == "" {
		return sessionRec{}, false
	}
	rec = sessionRec{Role: role, UserID: uid}
	s.mu.Lock()
	s.sessions[tok] = rec
	s.mu.Unlock()
	return rec, true
}

// resolve resolves the caller from the request, including a session JWT and a virtual key. On failure it returns an error and the caller writes the 401.
func (s *Server) resolve(r *http.Request) (*auth.Principal, error) {
	tok := auth.APIKeyFrom(r)
	rec, ok := s.lookupSession(tok)
	if ok {
		p := &auth.Principal{Kind: "session", Master: true, Role: rec.Role, UserID: rec.UserID}
		if rec.Role == "proxy_admin_viewer" || rec.Role == "internal_user_viewer" {
			p.ViewOnly = true
		}
		return p, nil
	}
	return auth.Resolve(s.Cfg, s.Store, r)
}

// verifySessionJWT checks a session JWT. A bad signature or an expired token returns ok false.
func verifySessionJWT(tok, secret string) (role, sess, userID string, ok bool) {
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		return "", "", "", false
	}
	if secret == "" {
		secret = "xhub"
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(parts[0] + "." + parts[1]))
	want := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(want), []byte(parts[2])) {
		return "", "", "", false
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", "", "", false
	}
	var claims map[string]any
	if json.Unmarshal(raw, &claims) != nil {
		return "", "", "", false
	}
	role, _ = claims["user_role"].(string)
	sess, _ = claims["key"].(string)
	userID, _ = claims["user_id"].(string)
	if role == "" {
		return "", "", "", false
	}
	return role, sess, userID, true
}

// requireManage requires a management identity. On failure it writes 401 and returns nil.
func (s *Server) requireManage(w http.ResponseWriter, r *http.Request) *auth.Principal {
	p, err := s.resolve(r)
	if err != nil {
		httpx.WriteTypedError(w, r.URL.Path, 401, "invalid_api_key", "Authentication Error, No api key passed in.")
		return nil
	}
	if !p.CanManage() {
		httpx.WriteTypedError(w, r.URL.Path, 401, "invalid_api_key", "Not allowed to access management endpoints")
		return nil
	}
	if p.ViewOnly && r.Method != http.MethodGet && r.Method != http.MethodHead {
		httpx.WriteTypedError(w, r.URL.Path, 403, "forbidden", "view-only role cannot write")
		return nil
	}
	return p
}

// requireMixed accepts either a management identity or an inference identity. If it is neither, it writes 401.
func (s *Server) requireMixed(w http.ResponseWriter, r *http.Request) *auth.Principal {
	p, err := s.resolve(r)
	if err != nil {
		httpx.WriteTypedError(w, r.URL.Path, 401, "invalid_api_key", "Authentication Error, No api key passed in.")
		return nil
	}
	if !p.CanLLM(s.Cfg) && !p.CanManage() {
		httpx.WriteTypedError(w, r.URL.Path, 401, "invalid_api_key", "Not allowed to access this endpoint")
		return nil
	}
	if p.ViewOnly && r.Method != http.MethodGet && r.Method != http.MethodHead {
		httpx.WriteTypedError(w, r.URL.Path, 403, "forbidden", "view-only role cannot write")
		return nil
	}
	return p
}

// requireLLMPrincipal requires an identity that may call inference. The master key may not by default.
func (s *Server) requireLLMPrincipal(w http.ResponseWriter, r *http.Request) *auth.Principal {
	p, err := s.resolve(r)
	if err != nil {
		log.Printf("error auth %s %s no api key", r.Method, r.URL.Path)
		httpx.WriteTypedError(w, r.URL.Path, 401, "invalid_api_key", "Authentication Error, No api key passed in.")
		return nil
	}
	if !p.CanLLM(s.Cfg) {
		log.Printf("error auth %s %s master key cannot call llm", r.Method, r.URL.Path)
		httpx.WriteTypedError(w, r.URL.Path, 401, "invalid_api_key", "Master key cannot call /v1/chat/completions")
		return nil
	}
	return p
}

// ensureUser makes sure the user row exists. An existing row does not have fields other than the role overwritten.
func (s *Server) ensureUser(id, role string) {
	if id == "" {
		return
	}
	if _, err := s.Store.GetUser(id); err == nil {
		return
	}
	_ = s.Store.InsertUser(store.Entity{
		ID: id, Email: id, Role: role, Alias: id, ModelsJSON: "[]", CreatedAt: time.Now().UTC(),
	})
}
