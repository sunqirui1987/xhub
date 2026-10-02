// Package gateway handles login, sessions, and the identity gate. Virtual-key resolution stays in the auth package.
package gateway

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/sunqirui1987/xhub/internal/auth"
	"github.com/sunqirui1987/xhub/internal/httpx"
	"github.com/sunqirui1987/xhub/internal/logx"
	"github.com/sunqirui1987/xhub/internal/store"
)

const (
	invalidUICredentials = "Invalid credentials used to access UI. Check 'UI_USERNAME' and 'UI_PASSWORD', or the password set for your user"
	invalidUserPassword  = "Invalid credentials used to access UI. Check the password set for your user"
	sessionTTL           = 7 * 24 * time.Hour
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
		"exp":          time.Now().Add(sessionTTL).Unix(),
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
	expiresAt := time.Now().Add(sessionTTL)
	s.mu.Lock()
	s.sessions[sess] = sessionRec{Role: role, UserID: userID, ExpiresAt: expiresAt}
	s.mu.Unlock()
	body, err := json.Marshal(map[string]any{"role": role, "user_id": userID, "expires_at": expiresAt.UTC().Format(time.RFC3339Nano)})
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
		if !rec.ExpiresAt.IsZero() && !time.Now().Before(rec.ExpiresAt) {
			s.mu.Lock()
			delete(s.sessions, tok)
			s.mu.Unlock()
			return sessionRec{}, false
		}
		return rec, true
	}
	if _, _, uid, exp, vok := verifySessionJWT(tok, s.Cfg.GeneralSettings.MasterKey); vok {
		return sessionRec{UserID: uid, ExpiresAt: exp}, true
	}
	m, err := s.Store.GetKV("ui_session", tok)
	if err != nil {
		return sessionRec{}, false
	}
	role, _ := m["role"].(string)
	uid, _ := m["user_id"].(string)
	expiresText, _ := m["expires_at"].(string)
	expiresAt, err := time.Parse(time.RFC3339Nano, expiresText)
	if role == "" || uid == "" || err != nil || !time.Now().Before(expiresAt) {
		if err == nil && !time.Now().Before(expiresAt) {
			_ = s.Store.DeleteKV("ui_session", tok)
		}
		return sessionRec{}, false
	}
	rec = sessionRec{Role: role, UserID: uid, ExpiresAt: expiresAt}
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
		role := ""
		if s.Store != nil {
			user, err := s.Store.GetUser(rec.UserID)
			if err != nil || user.ExtraBool("blocked") {
				return nil, auth.ErrSessionInvalid()
			}
			role = user.Role
		}
		if role == "" {
			role = rec.Role
		}
		if !validSessionRole(role) {
			return nil, auth.ErrSessionInvalid()
		}
		p := &auth.Principal{Kind: "session", Role: role, UserID: rec.UserID}
		if role == "proxy_admin_viewer" || role == "internal_user_viewer" {
			p.ViewOnly = true
		}
		logx.Debug("process %s %s step=identity source=session role=%s", r.Method, r.URL.Path, role)
		return p, nil
	}
	p, err := auth.Resolve(s.Cfg, s.Store, r)
	if err != nil {
		if tok == "" {
			logx.Debug("process %s %s step=identity source=none", r.Method, r.URL.Path)
		} else {
			logx.Debug("process %s %s step=identity source=reject reason=%s", r.Method, r.URL.Path, err.Error())
		}
		return nil, err
	}
	logx.Debug("process %s %s step=identity source=%s", r.Method, r.URL.Path, p.Kind)
	return p, nil
}

func validSessionRole(role string) bool {
	switch role {
	case "proxy_admin", "proxy_admin_viewer", "internal_user", "internal_user_viewer":
		return true
	default:
		return false
	}
}

// verifySessionJWT checks a session JWT. A bad signature or an expired token returns ok false.
func verifySessionJWT(tok, secret string) (role, sess, userID string, expiresAt time.Time, ok bool) {
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		return "", "", "", time.Time{}, false
	}
	if secret == "" {
		secret = "xhub"
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(parts[0] + "." + parts[1]))
	want := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(want), []byte(parts[2])) {
		return "", "", "", time.Time{}, false
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", "", "", time.Time{}, false
	}
	var claims map[string]any
	if json.Unmarshal(raw, &claims) != nil {
		return "", "", "", time.Time{}, false
	}
	role, _ = claims["user_role"].(string)
	sess, _ = claims["key"].(string)
	userID, _ = claims["user_id"].(string)
	exp, ok := claims["exp"].(float64)
	if role == "" || sess == "" || userID == "" || !ok || exp <= float64(time.Now().Unix()) {
		return "", "", "", time.Time{}, false
	}
	expiresAt = time.Unix(int64(exp), 0)
	return role, sess, userID, expiresAt, true
}

// requireManage requires a management identity. On failure it writes 401 and returns nil.
func (s *Server) requireManage(w http.ResponseWriter, r *http.Request) *auth.Principal {
	p, err := s.resolve(r)
	if err != nil {
		logx.Error("process %s %s step=auth gate=manage ok=false", r.Method, r.URL.Path)
		httpx.WriteTypedError(w, r.URL.Path, 401, "invalid_api_key", "Authentication Error, No api key passed in.")
		return nil
	}
	if !p.CanManage() {
		logx.Error("process %s %s step=auth gate=manage ok=false kind=%s", r.Method, r.URL.Path, p.Kind)
		httpx.WriteTypedError(w, r.URL.Path, 401, "invalid_api_key", "Not allowed to access management endpoints")
		return nil
	}
	if p.ViewOnly && r.Method != http.MethodGet && r.Method != http.MethodHead {
		logx.Error("process %s %s step=auth gate=manage ok=false reason=view-only", r.Method, r.URL.Path)
		httpx.WriteTypedError(w, r.URL.Path, 403, "forbidden", "view-only role cannot write")
		return nil
	}
	logx.Debug("process %s %s step=auth gate=manage ok=true kind=%s", r.Method, r.URL.Path, p.Kind)
	return p
}

// requireMixed accepts either a management identity or an inference identity. If it is neither, it writes 401.
func (s *Server) requireMixed(w http.ResponseWriter, r *http.Request) *auth.Principal {
	p, err := s.resolve(r)
	if err != nil {
		logx.Error("process %s %s step=auth gate=mixed ok=false", r.Method, r.URL.Path)
		httpx.WriteTypedError(w, r.URL.Path, 401, "invalid_api_key", "Authentication Error, No api key passed in.")
		return nil
	}
	if !p.CanLLM(s.Cfg) && !p.CanManage() {
		logx.Error("process %s %s step=auth gate=mixed ok=false kind=%s", r.Method, r.URL.Path, p.Kind)
		httpx.WriteTypedError(w, r.URL.Path, 401, "invalid_api_key", "Not allowed to access this endpoint")
		return nil
	}
	if p.ViewOnly && r.Method != http.MethodGet && r.Method != http.MethodHead {
		logx.Error("process %s %s step=auth gate=mixed ok=false reason=view-only", r.Method, r.URL.Path)
		httpx.WriteTypedError(w, r.URL.Path, 403, "forbidden", "view-only role cannot write")
		return nil
	}
	logx.Debug("process %s %s step=auth gate=mixed ok=true kind=%s", r.Method, r.URL.Path, p.Kind)
	return p
}

// requireLLMPrincipal requires an identity that may call inference. The master key may not by default.
func (s *Server) requireLLMPrincipal(w http.ResponseWriter, r *http.Request) *auth.Principal {
	p, err := s.resolve(r)
	if err != nil {
		logx.Error("auth %s %s no api key", r.Method, r.URL.Path)
		httpx.WriteTypedError(w, r.URL.Path, 401, "invalid_api_key", "Authentication Error, No api key passed in.")
		return nil
	}
	if !p.CanLLM(s.Cfg) {
		logx.Error("auth %s %s master key cannot call llm", r.Method, r.URL.Path)
		httpx.WriteTypedError(w, r.URL.Path, 401, "invalid_api_key", "Master key cannot call /v1/chat/completions")
		return nil
	}
	logx.Debug("process %s %s step=auth kind=%s", r.Method, r.URL.Path, p.Kind)
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
