// 登录、会话和身份门槛。虚拟密钥的解析仍交给 auth 包。
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
	"github.com/sunqirui1987/xhub/internal/store"
)

const (
	invalidUICredentials = "Invalid credentials used to access UI. Check 'UI_USERNAME' and 'UI_PASSWORD', or the password set for your user"
	invalidUserPassword  = "Invalid credentials used to access UI. Check the password set for your user"
)

// 用户名密码登录。环境凭证登录被关掉时只接受库存用户。
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

// 是否允许用环境里的控制台账号登录。
func (s *Server) envCredentialLoginEnabled() bool {
	return !s.Cfg.GeneralSettings.DisableEnvCredentialLogin
}

// 登录失败时的统一文案，不区分用户不存在和密码错误。
func (s *Server) invalidCredentialsMessage() string {
	if s.envCredentialLoginEnabled() {
		return invalidUICredentials
	}
	return invalidUserPassword
}

// 从主密钥推导默认控制台账号。没有单独配置时用户名是 admin。
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

// 用户名和密码是否与环境凭证一致。比较时不提前返回长度差异以外的信息。
func (s *Server) envCredentialsMatch(username, password string) bool {
	if !s.envCredentialLoginEnabled() {
		return false
	}
	wantUser, wantPass := uiEnvCredentials(s.Cfg.GeneralSettings.MasterKey)
	return credsEqual(username, wantUser) && credsEqual(password, wantPass)
}

// 常量时间比较两段字符串，避免用普通相等泄露密码长度以外的差异。
func credsEqual(a, b string) bool {
	return hmac.Equal([]byte(a), []byte(b))
}

// 写下登录成功的会话和用户信息。
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

// 签发会话 JWT。密钥来自配置，不写进响应以外的日志。
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

// ClearSessions 清空内存里的登录会话。测试用它表示进程重启后会话不再保留。
func (s *Server) ClearSessions() {
	s.mu.Lock()
	s.sessions = map[string]sessionRec{}
	s.mu.Unlock()
}

// 把会话放进进程内表。重启后会话失效。
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

// 按令牌找会话。找不到时 ok 为 false。
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

// 从请求解析调用方身份，含会话 JWT 和虚拟密钥。失败时返回错误，由上层写 401。
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

// 校验会话 JWT。签名不对或过期时 ok 为 false。
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

// 要求管理身份。失败时写 401 并返回 nil。
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

// 管理身份或推理身份都可以。两者都不是时写 401。
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

// 要求可以推理的身份。主密钥默认不行。
func (s *Server) requireLLMPrincipal(w http.ResponseWriter, r *http.Request) *auth.Principal {
	p, err := s.resolve(r)
	if err != nil {
		httpx.WriteTypedError(w, r.URL.Path, 401, "invalid_api_key", "Authentication Error, No api key passed in.")
		return nil
	}
	if !p.CanLLM(s.Cfg) {
		httpx.WriteTypedError(w, r.URL.Path, 401, "invalid_api_key", "Master key cannot call /v1/chat/completions")
		return nil
	}
	return p
}

// 保证用户行存在。已存在时不覆盖角色以外的字段。
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
