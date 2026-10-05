// Package gateway handles login, sessions, and the identity gate. Virtual-key
// resolution stays in the auth package.
package gateway

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/sunqirui1987/xhub/internal/auth"
	"github.com/sunqirui1987/xhub/internal/authz"
	"github.com/sunqirui1987/xhub/internal/httpx"
	"github.com/sunqirui1987/xhub/internal/iam"
	"github.com/sunqirui1987/xhub/internal/logx"
)

const invalidCredentials = "Invalid credentials. Check the email and password of your account."

const sessionTTL = 7 * 24 * time.Hour

// sessionRec is a live UI session. It stores the session_version the session
// was issued at and never a role: the role is read from the user row on every
// request, so a role change takes effect immediately rather than at the next
// login. A version bump — a disable, a deletion, a password reset — invalidates
// every session a user has open.
type sessionRec struct {
	UserID    string
	Version   int
	ExpiresAt time.Time
}

// login accepts an email and password and starts a session. There is no
// environment credential and no unauthenticated path to an administrator: the
// only account that exists before anybody signs in is the one /bootstrap made.
func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	httpx.SetCallID(w, httpx.CallID())
	var body struct {
		Username string `json:"username"`
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	email := body.Email
	if email == "" {
		email = body.Username
	}
	if email == "" || body.Password == "" {
		httpx.WriteError(w, 400, "invalid_request", "Please enter your username / password")
		return
	}
	if s.IAM == nil {
		httpx.WriteError(w, 503, "internal", "identity store unavailable")
		return
	}
	u, err := s.IAM.Login(r.Context(), email, body.Password)
	if err != nil {
		// A missing account and a wrong password answer the same way, so the
		// response cannot be used to enumerate accounts.
		logx.Debug("login rejected user=%s", email)
		httpx.WriteError(w, 401, "auth_error", invalidCredentials)
		return
	}
	s.loginSuccess(w, u)
}

// loginSuccess starts a session for a user that has already been authenticated.
func (s *Server) loginSuccess(w http.ResponseWriter, u *iam.User) {
	sess := "sess-" + httpx.CallID()
	if !s.rememberSession(sess, u.ID, u.SessionVersion) {
		httpx.WriteError(w, 500, "internal", "could not start a session")
		return
	}
	jwt := signSessionJWT(sess, u.ID, u.SessionVersion, u.Role, u.Email, s.Cfg.GeneralSettings.MasterKey)
	logx.Info("login ok user=%s role=%s", u.ID, u.Role)
	httpx.WriteJSON(w, 200, map[string]any{
		"token":        jwt,
		"key":          sess,
		"user_role":    iam.ConsoleRole(u.Role),
		"user_id":      u.ID,
		"premium_user": true,
		"redirect_url": "/ui/?login=success",
	})
}

// logout ends one session. It is idempotent: a session that is already gone is
// still a successful logout.
func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	httpx.SetCallID(w, httpx.CallID())
	tok := auth.APIKeyFrom(r)
	s.dropSession(tok)
	// A JWT names its session row, so a logout with a token rather than an id
	// still revokes the row the token would otherwise be accepted against.
	if sess, _, _, _, ok := verifySessionJWT(tok, s.Cfg.GeneralSettings.MasterKey); ok {
		s.dropSession(sess)
	}
	httpx.WriteJSON(w, 200, map[string]any{"status": "ok"})
}

// me reports the caller's identity and capabilities. The capability list comes
// from the same decision matrix the server enforces, so the console cannot
// offer a page the server would refuse.
func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	p, err := s.resolve(r)
	if err != nil {
		s.writeAuthError(w, r, err)
		return
	}
	out := map[string]any{
		"user_id":      p.UserID,
		"user_role":    iam.ConsoleRole(p.Role),
		"kind":         string(p.Kind),
		"capabilities": []string{},
		"teams":        []any{},
	}
	if p.Kind == authz.KindSession {
		g, err := s.guard(r.Context(), p)
		if err != nil {
			s.writeAuthError(w, r, err)
			return
		}
		out["capabilities"] = g.Capabilities()
		out["teams"] = g.TeamRoles()
		out["admin_organization_ids"] = g.AdminOrgIDs()
	}
	httpx.WriteJSON(w, 200, out)
}

// bootstrap creates the first platform administrator. It is the only route the
// master credential reaches besides emergency administration, and it can run
// exactly once: iam.Bootstrap refuses a second call.
func (s *Server) bootstrap(w http.ResponseWriter, r *http.Request) {
	httpx.SetCallID(w, httpx.CallID())
	if s.Cfg.GeneralSettings.MasterKey == "" || auth.APIKeyFrom(r) != s.Cfg.GeneralSettings.MasterKey {
		httpx.WriteError(w, 401, "invalid_api_key", "the master key is required to bootstrap")
		return
	}
	if s.IAM == nil {
		httpx.WriteError(w, 503, "internal", "identity store unavailable")
		return
	}
	var body struct {
		Email    string `json:"email"`
		Name     string `json:"name"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.WriteError(w, 400, "invalid_request", "email, name and password are required")
		return
	}
	if body.Email == "" || body.Password == "" {
		httpx.WriteError(w, 400, "invalid_request", "email and password are required")
		return
	}
	u, err := s.IAM.Bootstrap(r.Context(), body.Email, body.Name, body.Password)
	if err != nil {
		s.writeIAMError(w, r, err)
		return
	}
	s.loginSuccess(w, u)
}

// seedAdmin creates the platform administrator named by general_settings at
// startup, so a deployment does not have to call POST /bootstrap by hand before
// anybody can sign in.
//
// It runs once per process start and is safe to run every time: the account is
// created only when no account with the configured address exists, and an
// existing account is never rewritten. The configured password is therefore an
// initial password, not a managed one.
//
// Three configuration states all leave seeding off, and each is a deliberate
// operator choice rather than an error:
//
//   - no admin_email or no admin_password: seeding was never configured
//   - disable_env_credential_login: the operator refuses a config-supplied account
//   - a short password: iam reports it, and this logs the refusal instead of
//     starting with an account nobody can log into
//
// A failure here is logged and never fatal. A process that cannot seed can still
// serve, and POST /bootstrap remains available with the master key.
func (s *Server) seedAdmin() {
	g := s.Cfg.GeneralSettings
	if s.IAM == nil || g.DisableEnvCredentialLogin {
		return
	}
	if g.AdminEmail == "" || g.AdminPassword == "" {
		return
	}
	created, err := s.IAM.EnsureAdmin(context.Background(), g.AdminEmail, g.AdminName, g.AdminPassword)
	if err != nil {
		logx.Error("configured administrator not created email=%s err=%v", g.AdminEmail, err)
		return
	}
	if !created {
		logx.Info("configured administrator left as it is email=%s", g.AdminEmail)
	}
}

// bootstrapStatus reports whether the first administrator exists, so the login
// page can show the setup form instead of a form nobody can use yet.
func (s *Server) bootstrapStatus(w http.ResponseWriter, r *http.Request) {
	httpx.SetCallID(w, httpx.CallID())
	done := false
	if s.IAM != nil {
		var err error
		if done, err = s.IAM.Bootstrapped(r.Context()); err != nil {
			s.writeIAMError(w, r, err)
			return
		}
	}
	httpx.WriteJSON(w, 200, map[string]any{"bootstrap_required": !done, "bootstrapped": done})
}

// signSessionJWT issues a session JWT. The signing key comes from configuration
// and is not written to logs beyond the response.
//
// user_role is the console spelling, not the stored one. The admin UI decides
// who is a platform administrator by reading this claim, and it only recognises
// proxy_admin. A token that omitted the claim, or that carried the stored
// "admin", was treated as an ordinary user.
func signSessionJWT(sess, userID string, version int, role, email, secret string) string {
	if secret == "" {
		secret = "xhub"
	}
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	payload, _ := json.Marshal(map[string]any{
		"user_id":         userID,
		"user_role":       iam.ConsoleRole(role),
		"user_email":      email,
		"session_version": version,
		"key":             sess,
		"premium_user":    true,
		"exp":             time.Now().Add(sessionTTL).Unix(),
		"login_method":    "username_password",
	})
	pl := base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(header + "." + pl))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return header + "." + pl + "." + sig
}

// rememberSession stores a session in the process table and the key-value
// table. The key-value row is what keeps the session revocable before the JWT
// expires, so a failure to write it fails the login rather than issuing a token
// nothing can revoke.
func (s *Server) rememberSession(sess, userID string, version int) bool {
	expiresAt := time.Now().Add(sessionTTL)
	s.mu.Lock()
	s.sessions[sess] = sessionRec{UserID: userID, Version: version, ExpiresAt: expiresAt}
	s.mu.Unlock()
	if s.Store == nil {
		return true
	}
	body, err := json.Marshal(map[string]any{
		"user_id":    userID,
		"version":    version,
		"expires_at": expiresAt.UTC().Format(time.RFC3339Nano),
	})
	if err != nil {
		return false
	}
	if err := s.Store.PutKV("ui_session", sess, string(body)); err != nil {
		logx.Error("session persist failed user=%s err=%v", userID, err)
		s.dropMemorySession(sess)
		return false
	}
	return true
}

// dropSession forgets a session in both the process table and the key-value table.
func (s *Server) dropSession(sess string) {
	s.dropMemorySession(sess)
	if s.Store != nil && sess != "" {
		_ = s.Store.DeleteKV("ui_session", sess)
	}
}

// lookupSession resolves a session id or a session JWT. A JWT is accepted only
// while its session row still exists and has not expired. A process with no
// store, such as a dial test, keeps the in-memory table as the only record.
func (s *Server) lookupSession(tok string) (sessionRec, bool) {
	if tok == "" {
		return sessionRec{}, false
	}
	if rec, ok := s.storedSession(tok); ok {
		return rec, true
	}
	if s.Store == nil {
		return s.memorySession(tok)
	}
	sess, uid, version, exp, vok := verifySessionJWT(tok, s.Cfg.GeneralSettings.MasterKey)
	if !vok || sess == "" {
		return sessionRec{}, false
	}
	rec, ok := s.storedSession(sess)
	if !ok || rec.UserID != uid || rec.Version != version {
		return sessionRec{}, false
	}
	if !exp.IsZero() && !time.Now().Before(exp) {
		return sessionRec{}, false
	}
	return rec, true
}

func (s *Server) memorySession(id string) (sessionRec, bool) {
	s.mu.Lock()
	rec, ok := s.sessions[id]
	s.mu.Unlock()
	if !ok {
		return sessionRec{}, false
	}
	if !rec.ExpiresAt.IsZero() && !time.Now().Before(rec.ExpiresAt) {
		s.dropMemorySession(id)
		return sessionRec{}, false
	}
	return rec, true
}

// storedSession reads the key-value session. A missing or expired row is not a
// session, even if this process still remembers it.
func (s *Server) storedSession(id string) (sessionRec, bool) {
	if s.Store == nil || id == "" {
		return sessionRec{}, false
	}
	m, err := s.Store.GetKV("ui_session", id)
	if err != nil {
		s.dropMemorySession(id)
		return sessionRec{}, false
	}
	uid, _ := m["user_id"].(string)
	version, _ := m["version"].(float64)
	expiresText, _ := m["expires_at"].(string)
	expiresAt, err := time.Parse(time.RFC3339Nano, expiresText)
	if uid == "" || err != nil || !time.Now().Before(expiresAt) {
		if err == nil && !time.Now().Before(expiresAt) {
			_ = s.Store.DeleteKV("ui_session", id)
		}
		s.dropMemorySession(id)
		return sessionRec{}, false
	}
	rec := sessionRec{UserID: uid, Version: int(version), ExpiresAt: expiresAt}
	s.mu.Lock()
	s.sessions[id] = rec
	s.mu.Unlock()
	return rec, true
}

func (s *Server) dropMemorySession(id string) {
	s.mu.Lock()
	delete(s.sessions, id)
	s.mu.Unlock()
}

// resolve identifies the caller. A session is read from its stored row and
// checked against the live user, and a virtual key is revalidated in full on
// every request: owner active, still a member of the key's team, and neither
// the team, the organization nor the project blocked. A key that passed at
// login is not trusted later.
func (s *Server) resolve(r *http.Request) (*auth.Principal, error) {
	ctx := r.Context()
	tok := auth.APIKeyFrom(r)
	if rec, ok := s.lookupSession(tok); ok {
		if s.IAM == nil {
			return nil, auth.ErrSessionInvalid()
		}
		u, err := s.IAM.GetUser(ctx, rec.UserID)
		if err != nil {
			logx.Debug("session user unreadable user=%s err=%v", rec.UserID, err)
			return nil, auth.ErrSessionInvalid()
		}
		p, err := auth.SessionPrincipal(u, rec.Version, tok)
		if err != nil {
			return nil, err
		}
		logx.Debug("process %s %s step=identity source=session role=%s", r.Method, r.URL.Path, p.Role)
		return p, nil
	}
	p, err := auth.Resolve(ctx, s.Cfg, s.IAM, r)
	if err != nil {
		if tok == "" {
			logx.Debug("process %s %s step=identity source=none", r.Method, r.URL.Path)
		} else {
			logx.Debug("process %s %s step=identity source=reject reason=%s", r.Method, r.URL.Path, err.Error())
		}
		return nil, err
	}
	if p.Kind == authz.KindKey {
		if err := s.checkKey(ctx, p); err != nil {
			logx.Debug("process %s %s step=identity source=key reason=stale key=%s", r.Method, r.URL.Path, p.KeyID)
			return nil, auth.ErrSessionInvalid()
		}
	}
	logx.Debug("process %s %s step=identity source=%s", r.Method, r.URL.Path, p.Kind)
	return p, nil
}

// checkKey revalidates a key's whole ownership chain. It runs on every request
// so removing a member, disabling an owner, or blocking a team takes effect
// immediately rather than at key expiry.
func (s *Server) checkKey(ctx context.Context, p *auth.Principal) error {
	g, err := s.guard(ctx, p)
	if err != nil {
		return err
	}
	return g.CheckKey(ctx)
}

// guard builds the per-request authorization context.
func (s *Server) guard(ctx context.Context, p *auth.Principal) (*authz.Guard, error) {
	if s.Authz == nil {
		return nil, &authz.InternalError{Err: errNoAuthz}
	}
	return s.Authz.Guard(ctx, p.Actor())
}

var errNoAuthz = errorString("gateway: authorization layer unavailable")

type errorString string

func (e errorString) Error() string { return string(e) }

// verifySessionJWT checks a session JWT. A bad signature or an expired token returns ok false.
func verifySessionJWT(tok, secret string) (sess, userID string, version int, expiresAt time.Time, ok bool) {
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		return "", "", 0, time.Time{}, false
	}
	if secret == "" {
		secret = "xhub"
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(parts[0] + "." + parts[1]))
	want := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(want), []byte(parts[2])) {
		return "", "", 0, time.Time{}, false
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", "", 0, time.Time{}, false
	}
	var claims map[string]any
	if json.Unmarshal(raw, &claims) != nil {
		return "", "", 0, time.Time{}, false
	}
	sess, _ = claims["key"].(string)
	userID, _ = claims["user_id"].(string)
	v, _ := claims["session_version"].(float64)
	exp, hasExp := claims["exp"].(float64)
	if sess == "" || userID == "" || !hasExp || exp <= float64(time.Now().Unix()) {
		return "", "", 0, time.Time{}, false
	}
	return sess, userID, int(v), time.Unix(int64(exp), 0), true
}

// requireMaster accepts only the master credential. It reaches the bootstrap and
// emergency routes and nothing else.
func (s *Server) requireMaster(w http.ResponseWriter, r *http.Request) *auth.Principal {
	p, err := s.resolve(r)
	if err != nil || !p.IsMaster() {
		logx.Error("process %s %s step=auth gate=master ok=false", r.Method, r.URL.Path)
		httpx.WriteTypedError(w, r.URL.Path, 401, "invalid_api_key", "Authentication Error, No api key passed in.")
		return nil
	}
	logx.Debug("process %s %s step=auth gate=master ok=true", r.Method, r.URL.Path)
	return p
}

// requireUser accepts any signed-in session or virtual key. It does not grant
// global management. Personal keys, team membership, usage, and logs use this
// gate and then narrow the rows.
func (s *Server) requireUser(w http.ResponseWriter, r *http.Request) *auth.Principal {
	p, err := s.resolve(r)
	if err != nil {
		logx.Error("process %s %s step=auth gate=user ok=false", r.Method, r.URL.Path)
		s.writeAuthError(w, r, err)
		return nil
	}
	logx.Debug("process %s %s step=auth gate=user ok=true kind=%s", r.Method, r.URL.Path, p.Kind)
	return p
}

// requireManage requires a platform administrator session. A team administrator
// is not a platform administrator: management routes that a team admin may use
// resolve the team themselves and are gated by requireUser plus an explicit
// authorization decision.
func (s *Server) requireManage(w http.ResponseWriter, r *http.Request) *auth.Principal {
	p, err := s.resolve(r)
	if err != nil {
		logx.Error("process %s %s step=auth gate=manage ok=false", r.Method, r.URL.Path)
		s.writeAuthError(w, r, err)
		return nil
	}
	if !s.canManage(p) {
		logx.Error("process %s %s step=auth gate=manage ok=false kind=%s", r.Method, r.URL.Path, p.Kind)
		httpx.WriteTypedError(w, r.URL.Path, 403, "forbidden", "Not allowed to access management endpoints")
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
		s.writeAuthError(w, r, err)
		return nil
	}
	if !s.canManage(p) && !s.canLLM(p) {
		logx.Error("process %s %s step=auth gate=mixed ok=false kind=%s", r.Method, r.URL.Path, p.Kind)
		httpx.WriteTypedError(w, r.URL.Path, 403, "forbidden", "Not allowed to access this endpoint")
		return nil
	}
	logx.Debug("process %s %s step=auth gate=mixed ok=true kind=%s", r.Method, r.URL.Path, p.Kind)
	return p
}

// requireLLMPrincipal requires an identity that may call inference. The master
// key may not: it is an emergency credential for administration.
func (s *Server) requireLLMPrincipal(w http.ResponseWriter, r *http.Request) *auth.Principal {
	p, err := s.resolve(r)
	if err != nil {
		logx.Error("auth %s %s no api key", r.Method, r.URL.Path)
		s.writeAuthError(w, r, err)
		return nil
	}
	if !s.canLLM(p) {
		logx.Error("auth %s %s cannot call llm kind=%s", r.Method, r.URL.Path, p.Kind)
		httpx.WriteTypedError(w, r.URL.Path, 403, "forbidden", "this credential cannot call inference")
		return nil
	}
	logx.Debug("process %s %s step=auth kind=%s", r.Method, r.URL.Path, p.Kind)
	return p
}

// canManage reports platform administration. A key never confers it, whatever
// role its owner holds: a key is a credential for inference and for reading
// itself, and nothing else.
func (s *Server) canManage(p *auth.Principal) bool {
	return p != nil && p.Kind == authz.KindSession && p.Role == iam.RoleAdmin
}

// canLLM reports whether the caller may send inference. Any active session may;
// a key may once it has passed its per-request revalidation in resolve.
func (s *Server) canLLM(p *auth.Principal) bool {
	if p == nil {
		return false
	}
	return p.Kind == authz.KindSession || p.Kind == authz.KindKey
}

// writeAuthError answers a failed identification with the status the authz
// sentinel implies: 401 for a missing or unusable credential, 500 for a
// dependency that could not be read, so a database failure is never reported as
// the client's bad key.
func (s *Server) writeAuthError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case authz.IsInternal(err):
		httpx.WriteTypedError(w, r.URL.Path, 500, "internal", "identity lookup failed")
	case authz.IsUnauthenticated(err):
		httpx.WriteTypedError(w, r.URL.Path, 401, auth.Code(err), authFailureMessage(r))
	case authz.IsNotFound(err):
		httpx.WriteTypedError(w, r.URL.Path, 401, auth.Code(err), authFailureMessage(r))
	default:
		httpx.WriteTypedError(w, r.URL.Path, 401, auth.Code(err), authFailureMessage(r))
	}
}

// authFailureMessage distinguishes a request that brought no credential from
// one whose credential is no longer accepted. The console treats the second
// as a finished session and sends the person to sign in again.
func authFailureMessage(r *http.Request) string {
	if auth.APIKeyFrom(r) != "" {
		return "Your session has ended. Sign in again."
	}
	return "Authentication Error, No api key passed in."
}

// writeAuthzError maps an authorization decision onto a response. It never
// reports a distinction between "does not exist" and "not yours".
func (s *Server) writeAuthzError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case err == nil:
		return
	case authz.IsInternal(err):
		httpx.WriteTypedError(w, r.URL.Path, 500, "internal", "authorization lookup failed")
	case authz.IsUnauthenticated(err):
		httpx.WriteTypedError(w, r.URL.Path, 401, "invalid_api_key", "Authentication Error, No api key passed in.")
	case authz.IsForbidden(err):
		httpx.WriteTypedError(w, r.URL.Path, 403, "forbidden", "Not allowed to access this resource")
	default:
		httpx.WriteTypedError(w, r.URL.Path, 404, "not_found", "Not found")
	}
}

// writeIAMError maps an iam failure onto a response. A conflict is a 409, a
// rejected value or a protected last administrator is a 400, and a dependency
// failure is a 500.
func (s *Server) writeIAMError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case err == nil:
		return
	case err == iam.ErrNotFound:
		httpx.WriteTypedError(w, r.URL.Path, 404, "not_found", "Not found")
	case err == iam.ErrConflict:
		httpx.WriteTypedError(w, r.URL.Path, 409, "conflict", err.Error())
	case err == iam.ErrInvalid, err == iam.ErrLastAdmin, err == iam.ErrLastPlatformAdmin, err == iam.ErrInactive:
		httpx.WriteTypedError(w, r.URL.Path, 400, "invalid_request", err.Error())
	default:
		logx.Error("iam error %s %s: %v", r.Method, r.URL.Path, err)
		httpx.WriteTypedError(w, r.URL.Path, 500, "internal", "request could not be completed")
	}
}
