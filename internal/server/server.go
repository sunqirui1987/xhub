package server

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sunqirui1987/xhub/internal/auth"
	"github.com/sunqirui1987/xhub/internal/cache"
	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/hooks"
	"github.com/sunqirui1987/xhub/internal/httpx"
	"github.com/sunqirui1987/xhub/internal/live"
	"github.com/sunqirui1987/xhub/internal/router"
	"github.com/sunqirui1987/xhub/internal/spend"
	"github.com/sunqirui1987/xhub/internal/store"
)

const Version = "xhub-dev"

type Server struct {
	Cfg                *config.Config
	Store              *store.Store
	Client             *http.Client
	engine             *gin.Engine
	registered         map[string]struct{}
	Cache              *cache.DualCache
	Hooks              *hooks.Engine
	Busy               map[string]int
	Live               *live.Client
	rpmHits            map[string][]time.Time
	tpmHits            map[string][]tokHit
	catalog            []catRoute
	sessions           map[string]sessionRec
	ssoCodes           map[string]bool
	emailEvents        []emailEventSetting
	idem               map[string]idemRec
	uiProxy            http.Handler
	mu                 sync.Mutex
	yamlStoreModelInDB bool
}

type idemRec struct {
	Code int
	CT   string
	Body []byte
	Hdr  map[string]string
}

type tokHit struct {
	t time.Time
	n int
}

type sessionRec struct {
	Role   string
	UserID string
}

func New(cfg *config.Config, st *store.Store) *Server {
	s := &Server{
		Cfg:                cfg,
		Store:              st,
		Client:             &http.Client{Timeout: time.Duration(cfg.RouterSettings.Timeout) * time.Second},
		engine:             newEngine(),
		registered:         map[string]struct{}{},
		Cache:              cache.New(),
		Hooks:              hooks.New(),
		Busy:               map[string]int{},
		rpmHits:            map[string][]time.Time{},
		tpmHits:            map[string][]tokHit{},
		catalog:            loadCatalog(),
		sessions:           map[string]sessionRec{},
		ssoCodes:           map[string]bool{},
		idem:               map[string]idemRec{},
		uiProxy:            newUIProxy(),
		yamlStoreModelInDB: cfg.GeneralSettings.StoreModelInDB,
	}
	if cfg.GeneralSettings.RedisURL != "" {
		if client, err := live.Open(cfg.GeneralSettings.RedisURL); err == nil {
			s.Live = client
		}
	}
	s.applyTypedRouter(s.mergedRouterSettings())
	s.loadStoredState()
	s.routes()
	s.identityRoutes()
	// 每条 catalog 路由单独挂到 Gin。不再用 "/" 把未注册路径收成非 404。
	s.mountCatalog()
	return s
}

// Run 在 addr 上接受连接。进程入口用它，而不是把 ServeMux 交给 ListenAndServe。
func (s *Server) Run(addr string) error {
	if s.Live != nil {
		go s.flushLoop()
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	return (&http.Server{Handler: s.Handler()}).Serve(ln)
}

func newEngine() *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	e := gin.New()
	e.NoRoute(gin.WrapF(func(w http.ResponseWriter, r *http.Request) {
		httpx.WriteError(w, http.StatusNotFound, "not_found", "Not Found")
	}))
	return e
}

func setCORS(w http.ResponseWriter, r *http.Request) {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return
	}
	h := w.Header()
	h.Set("Access-Control-Allow-Origin", origin)
	h.Set("Access-Control-Allow-Credentials", "true")
	// Echo the preflight list. The playground OpenAI SDK sends x-stainless-*
	// and Chrome client hints; a fixed allow-list fails that preflight.
	allow := r.Header.Get("Access-Control-Request-Headers")
	if allow == "" {
		allow = "Authorization, Content-Type, x-litellm-api-key, Idempotency-Key, x-litellm-tags, x-litellm-end-user-id"
	}
	h.Set("Access-Control-Allow-Headers", allow)
	h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
	h.Set("Access-Control-Expose-Headers", "x-litellm-call-id, x-litellm-response-cost, x-litellm-model-name, Retry-After, x-litellm-cache-key, cache_hit, x-litellm-cache-hit, x-litellm-version, x-litellm-response-duration-ms")
	h.Set("Access-Control-Max-Age", "600")
	// 127.0.0.1:3000 calling localhost:4000 is a private-network request in Chrome.
	h.Set("Access-Control-Allow-Private-Network", "true")
	h.Add("Vary", "Origin")
	h.Add("Vary", "Access-Control-Request-Headers")
}

func (s *Server) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		callID := httpx.CallID()
		httpx.SetCallID(w, callID)
		w.Header().Set("x-litellm-version", Version)
		setCORS(w, r)
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if s.tryDashboard(w, r) {
			return
		}
		raw, _ := io.ReadAll(r.Body)
		r.Body = io.NopCloser(bytes.NewReader(raw))
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		if model := str(body["model"]); model != "" {
			w.Header().Set("x-litellm-model-name", model)
			w.Header().Set("x-litellm-model-id", model)
		}
		stream, _ := body["stream"].(bool)
		idemKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
		if idemKey != "" && !stream {
			ck := auth.APIKeyFrom(r) + "|" + r.Method + "|" + r.URL.Path + "|" + idemKey
			s.mu.Lock()
			hit, ok := s.idem[ck]
			s.mu.Unlock()
			if ok {
				for k, v := range hit.Hdr {
					w.Header().Set(k, v)
				}
				if hit.CT != "" {
					w.Header().Set("Content-Type", hit.CT)
				}
				w.WriteHeader(hit.Code)
				_, _ = w.Write(hit.Body)
				return
			}
		}
		if stream || strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
			s.engine.ServeHTTP(w, r)
			return
		}
		hw := &holdWriter{ResponseWriter: w, code: 200}
		s.engine.ServeHTTP(hw, r)
		if w.Header().Get("Content-Type") == "" {
			w.Header().Set("Content-Type", "application/json")
		}
		if isDataPlanePath(r.URL.Path) {
			w.Header().Set("x-litellm-response-duration-ms", strconv.FormatInt(time.Since(start).Milliseconds(), 10))
			if w.Header().Get("x-litellm-model-api-base") == "" {
				if p, err := s.resolve(r); err == nil && p != nil && p.Key != nil {
					s.setChatHeaders(w, p, w.Header().Get("x-litellm-model-name"), w.Header().Get("x-litellm-model-api-base"))
				}
			}
		}
		code := hw.code
		if !hw.hdr {
			code = 200
		}
		w.WriteHeader(code)
		_, _ = w.Write(hw.buf.Bytes())
		if idemKey != "" && code < 500 {
			ck := auth.APIKeyFrom(r) + "|" + r.Method + "|" + r.URL.Path + "|" + idemKey
			rec := idemRec{Code: code, CT: w.Header().Get("Content-Type"), Body: append([]byte(nil), hw.buf.Bytes()...)}
			rec.Hdr = map[string]string{}
			for _, k := range []string{"x-litellm-call-id", "x-litellm-version", "x-litellm-model-name", "x-litellm-model-id"} {
				if v := w.Header().Get(k); v != "" {
					rec.Hdr[k] = v
				}
			}
			s.mu.Lock()
			s.idem[ck] = rec
			s.mu.Unlock()
		}
	})
}

type holdWriter struct {
	http.ResponseWriter
	code int
	buf  bytes.Buffer
	hdr  bool
}

func (h *holdWriter) WriteHeader(c int) {
	if !h.hdr {
		h.code = c
		h.hdr = true
	}
}

func (h *holdWriter) Write(p []byte) (int, error) {
	if !h.hdr {
		h.code = 200
		h.hdr = true
	}
	return h.buf.Write(p)
}

func (s *Server) routes() {
	s.handle("GET /health/liveliness", s.healthLive)
	s.handle("GET /health/liveness", s.healthLive)
	s.handle("GET /health/readiness", s.healthReady)
	s.handle("GET /health/readiness/details", s.healthDetails)
	s.handle("GET /health", s.healthReady)
	s.handle("GET /.well-known/litellm-ui-config", s.uiConfig)
	s.handle("GET /litellm/.well-known/litellm-ui-config", s.uiConfig)
	s.handle("POST /login", s.login)
	s.handle("POST /v2/login", s.login)

	s.handle("POST /key/generate", s.keyGenerate)
	s.handle("POST /key/service-account/generate", s.keyGenerateServiceAccount)
	s.handle("GET /key/list", s.keyList)
	s.handle("GET /key/info", s.keyInfo)
	s.handle("POST /v2/key/info", s.keyInfo)
	s.handle("POST /key/delete", s.keyDelete)
	s.handle("POST /key/block", s.keyBlock)
	s.handle("POST /key/unblock", s.keyUnblock)
	s.handle("POST /key/update", s.keyUpdate)
	s.handle("POST /key/bulk_update", s.keyBulkUpdate)
	s.handle("POST /key/regenerate", s.keyRegenerate)
	s.handle("POST /key/{key}/regenerate", s.keyRegenerate)
	s.handle("POST /key/{key}/reset_spend", s.keyResetSpend)
	s.handle("GET /key/aliases", s.keyAliases)
	s.handle("POST /key/health", s.keyHealth)

	s.handle("GET /v1/models", s.listModels)
	s.handle("GET /models", s.listModels)
	s.handle("POST /utils/token_counter", s.tokenCounter)
	s.handle("GET /utils/supported_openai_params", s.supportedOpenAIParams)

	s.handle("POST /v1/chat/completions", s.chat)
	s.handle("POST /chat/completions", s.chat)

	s.handle("GET /email/event_settings", s.emailEventSettings)
	s.handle("PATCH /email/event_settings", s.emailEventSettings)
	s.handle("POST /email/event_settings/reset", s.emailEventSettingsReset)
}

func (s *Server) healthLive(w http.ResponseWriter, r *http.Request) {
	httpx.SetCallID(w, httpx.CallID())
	httpx.WriteJSON(w, 200, map[string]any{"status": "ok"})
}

func (s *Server) healthReady(w http.ResponseWriter, r *http.Request) {
	httpx.SetCallID(w, httpx.CallID())
	if err := s.Store.DB.Ping(); err != nil {
		httpx.WriteError(w, 503, "not_ready", err.Error())
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"status": "ready"})
}

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

const (
	invalidUICredentials = "Invalid credentials used to access UI. Check 'UI_USERNAME' and 'UI_PASSWORD', or the password set for your user"
	invalidUserPassword  = "Invalid credentials used to access UI. Check the password set for your user"
)

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

func (s *Server) envCredentialLoginEnabled() bool {
	return !s.Cfg.GeneralSettings.DisableEnvCredentialLogin
}

func (s *Server) invalidCredentialsMessage() string {
	if s.envCredentialLoginEnabled() {
		return invalidUICredentials
	}
	return invalidUserPassword
}

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

func (s *Server) envCredentialsMatch(username, password string) bool {
	if !s.envCredentialLoginEnabled() {
		return false
	}
	wantUser, wantPass := uiEnvCredentials(s.Cfg.GeneralSettings.MasterKey)
	return credsEqual(username, wantUser) && credsEqual(password, wantPass)
}

func credsEqual(a, b string) bool {
	return hmac.Equal([]byte(a), []byte(b))
}

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

func (s *Server) keyGenerate(w http.ResponseWriter, r *http.Request) {
	httpx.SetCallID(w, httpx.CallID())
	if s.requireManage(w, r) == nil {
		return
	}
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil && err != io.EOF {
		httpx.WriteError(w, 400, "invalid_request", "invalid json")
		return
	}
	if body == nil {
		body = map[string]any{}
	}
	plain := store.NewPlainKey()
	k, err := keyFromBody(plain, body)
	if err != nil {
		httpx.WriteError(w, 400, "invalid_request", err.Error())
		return
	}
	if err := s.Store.InsertKey(k); err != nil {
		httpx.WriteError(w, 500, "internal", err.Error())
		return
	}
	httpx.WriteJSON(w, 200, keyResponse(k, plain, true))
}

func (s *Server) keyList(w http.ResponseWriter, r *http.Request) {
	httpx.SetCallID(w, httpx.CallID())
	if s.requireManage(w, r) == nil {
		return
	}
	keys, err := s.Store.ListKeys()
	if err != nil {
		httpx.WriteError(w, 500, "internal", err.Error())
		return
	}
	out := make([]map[string]any, 0, len(keys))
	for _, k := range keys {
		out = append(out, keyResponse(k, "", false))
	}
	httpx.WriteJSON(w, 200, map[string]any{
		"keys":         out,
		"total_count":  len(out),
		"current_page": 1,
		"total_pages":  1,
		"size":         len(out),
	})
}

func (s *Server) keyInfo(w http.ResponseWriter, r *http.Request) {
	httpx.SetCallID(w, httpx.CallID())
	if s.requireManage(w, r) == nil {
		return
	}
	plain := r.URL.Query().Get("key")
	if r.Method == http.MethodPost {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if v, ok := body["key"].(string); ok {
			plain = v
		}
	}
	if plain == "" {
		httpx.WriteError(w, 400, "invalid_request", "key required")
		return
	}
	hash := store.HashKey(plain)
	if !strings.HasPrefix(plain, "sk-") {
		hash = plain
	}
	k, err := s.Store.GetByHash(hash)
	if err != nil {
		httpx.WriteError(w, 404, "not_found", "key not found")
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"info": keyResponse(*k, "", false)})
}

func (s *Server) keyDelete(w http.ResponseWriter, r *http.Request) {
	httpx.SetCallID(w, httpx.CallID())
	if s.requireManage(w, r) == nil {
		return
	}
	var body struct {
		Keys []string `json:"keys"`
		Key  string   `json:"key"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	keys := body.Keys
	if body.Key != "" {
		keys = append(keys, body.Key)
	}
	for _, p := range keys {
		hash := store.HashKey(p)
		if !strings.HasPrefix(p, "sk-") {
			hash = p
		}
		_ = s.Store.DeleteHash(hash)
	}
	httpx.WriteJSON(w, 200, map[string]any{"deleted": len(keys)})
}

func (s *Server) keyBlock(w http.ResponseWriter, r *http.Request) {
	s.setBlock(w, r, true)
}

func (s *Server) keyUnblock(w http.ResponseWriter, r *http.Request) {
	s.setBlock(w, r, false)
}

func (s *Server) setBlock(w http.ResponseWriter, r *http.Request, blocked bool) {
	httpx.SetCallID(w, httpx.CallID())
	if s.requireManage(w, r) == nil {
		return
	}
	var body struct {
		Key string `json:"key"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	hash := store.HashKey(body.Key)
	if !strings.HasPrefix(body.Key, "sk-") {
		hash = body.Key
	}
	if err := s.Store.SetBlocked(hash, blocked); err != nil {
		httpx.WriteError(w, 404, "not_found", "key not found")
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"blocked": blocked})
}

func (s *Server) keyUpdate(w http.ResponseWriter, r *http.Request) {
	httpx.SetCallID(w, httpx.CallID())
	if s.requireManage(w, r) == nil {
		return
	}
	var body map[string]any
	_ = json.NewDecoder(r.Body).Decode(&body)
	plain := str(body["key"])
	if plain == "" {
		httpx.WriteError(w, 400, "invalid_request", "key required")
		return
	}
	hash := store.HashKey(plain)
	k, err := s.Store.GetByHash(hash)
	if err != nil {
		httpx.WriteError(w, 404, "not_found", "key not found")
		return
	}
	applyKeyPatch(k, body)
	if err := s.Store.UpdateKey(*k); err != nil {
		httpx.WriteError(w, 400, "invalid_request", err.Error())
		return
	}
	httpx.WriteJSON(w, 200, keyResponse(*k, "", false))
}

func (s *Server) chat(w http.ResponseWriter, r *http.Request) {
	s.dataPlane(w, r, "chat")
}

func keyResponse(k store.Key, plain string, includePlain bool) map[string]any {
	created := k.CreatedAt.UTC().Format(time.RFC3339)
	if k.CreatedAt.IsZero() {
		created = time.Now().UTC().Format(time.RFC3339)
	}
	meta := map[string]any{}
	if k.MetadataJSON != "" {
		_ = json.Unmarshal([]byte(k.MetadataJSON), &meta)
	}
	var tags any
	if k.TagsJSON != "" {
		_ = json.Unmarshal([]byte(k.TagsJSON), &tags)
	}
	var expires any
	if k.ExpiresAt.Valid {
		expires = k.ExpiresAt.Time.UTC().Format(time.RFC3339)
	}
	var reset any
	if k.BudgetResetAt.Valid {
		reset = k.BudgetResetAt.Time.UTC().Format(time.RFC3339)
	}
	var dur any
	if k.BudgetDuration != "" {
		dur = k.BudgetDuration
	}
	budgetTable := map[string]any{
		"max_budget":      nullFloatMap(k.MaxBudget),
		"soft_budget":     nullFloatMap(k.SoftBudget),
		"tpm_limit":       nullIntMap(k.TPMLimit),
		"rpm_limit":       nullIntMap(k.RPMLimit),
		"budget_duration": dur,
		"budget_reset_at": reset,
	}
	token := k.TokenHash
	if includePlain {
		token = plain
	}
	m := map[string]any{
		"token_id":               k.TokenHash,
		"token":                  token,
		"key_name":               k.KeyName,
		"key_alias":              k.KeyAlias,
		"user_id":                emptyNil(k.UserID),
		"team_id":                emptyNil(k.TeamID),
		"organization_id":        emptyNil(k.OrganizationID),
		"org_id":                 emptyNil(k.OrganizationID),
		"project_id":             emptyNil(k.ProjectID),
		"agent_id":               emptyNil(k.AgentID),
		"budget_id":              emptyNil(k.BudgetID),
		"models":                 k.Models(),
		"max_budget":             nullFloatMap(k.MaxBudget),
		"soft_budget":            nullFloatMap(k.SoftBudget),
		"spend":                  k.Spend,
		"key_type":               k.KeyType,
		"blocked":                nullBoolJSON(k.Blocked),
		"tpm_limit":              nullIntMap(k.TPMLimit),
		"rpm_limit":              nullIntMap(k.RPMLimit),
		"max_parallel_requests":  nullIntMap(k.MaxParallel),
		"budget_duration":        dur,
		"budget_reset_at":        reset,
		"expires":                expires,
		"metadata":               meta,
		"tags":                   tags,
		"aliases":                map[string]any{},
		"config":                 map[string]any{},
		"permissions":            map[string]any{},
		"allowed_cache_controls": []any{},
		"allowed_routes":         []any{},
		"model_max_budget":       map[string]any{},
		"model_spend":            map[string]any{},
		"created_at":             created,
		"updated_at":             created,
		"created_by":             emptyNil(k.UserID),
		"last_active":            nil,
		"litellm_budget_table":   budgetTable,
	}
	if includePlain {
		m["key"] = plain
	}
	return m
}

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

func nullFloatMap(v sql.NullFloat64) any {
	if !v.Valid {
		return nil
	}
	return v.Float64
}

func nullIntMap(v sql.NullInt64) any {
	if !v.Valid {
		return nil
	}
	return v.Int64
}

func emptyNil(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func encodeModels(v any) string {
	switch t := v.(type) {
	case []any:
		b, _ := json.Marshal(t)
		return string(b)
	case []string:
		b, _ := json.Marshal(t)
		return string(b)
	default:
		return "[]"
	}
}

func parseNullFloat(v any) sql.NullFloat64 {
	switch t := v.(type) {
	case float64:
		return sql.NullFloat64{Float64: t, Valid: true}
	case string:
		if t == "" {
			return sql.NullFloat64{}
		}
		f, err := strconv.ParseFloat(t, 64)
		if err != nil {
			return sql.NullFloat64{}
		}
		return sql.NullFloat64{Float64: f, Valid: true}
	default:
		return sql.NullFloat64{}
	}
}

func parseNullInt(v any) sql.NullInt64 {
	switch t := v.(type) {
	case float64:
		return sql.NullInt64{Int64: int64(t), Valid: true}
	case int:
		return sql.NullInt64{Int64: int64(t), Valid: true}
	default:
		return sql.NullInt64{}
	}
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

func boolOf(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		return t == "true" || t == "1"
	case float64:
		return t != 0
	default:
		return false
	}
}

func cloneMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func asInt(v any) int {
	switch t := v.(type) {
	case float64:
		return int(t)
	case int:
		return t
	default:
		return 0
	}
}

type errStr string

func (e errStr) Error() string { return string(e) }

var (
	errUpstreamStatus = errStr("upstream_error")
	errEmptyUpstream  = errStr("empty upstream stream")
)

func (s *Server) incBusy(id string) {
	s.mu.Lock()
	s.Busy[id]++
	s.mu.Unlock()
}

func (s *Server) decBusy(id string) {
	s.mu.Lock()
	s.Busy[id]--
	s.mu.Unlock()
}

func (s *Server) setChatHeaders(w http.ResponseWriter, p *auth.Principal, alias, apiBase string) {
	w.Header().Set("x-litellm-model-name", alias)
	w.Header().Set("x-litellm-model-api-base", apiBase)
	w.Header().Set("x-litellm-version", Version)
	if p.Key != nil {
		if p.Key.TPMLimit.Valid {
			w.Header().Set("x-litellm-key-tpm-limit", strconv.FormatInt(p.Key.TPMLimit.Int64, 10))
		}
		if p.Key.RPMLimit.Valid {
			w.Header().Set("x-litellm-key-rpm-limit", strconv.FormatInt(p.Key.RPMLimit.Int64, 10))
		}
		if p.Key.MaxBudget.Valid {
			w.Header().Set("x-litellm-key-max-budget", spend.Format(p.Key.MaxBudget.Float64))
		}
		w.Header().Set("x-litellm-key-spend", spend.Format(p.Key.Spend))
	}
}

func (s *Server) recordSpend(w http.ResponseWriter, p *auth.Principal, callID, alias string, usage map[string]any, start time.Time, cacheHit bool, depID string) {
	if usage == nil {
		usage = map[string]any{}
	}
	pt := asInt(usage["prompt_tokens"])
	ct := asInt(usage["completion_tokens"])
	total, in, out, okc := spend.Cost(alias, pt, ct)
	// LiteLLM stores response_cost or 0.0. Unknown models still get a row, with spend 0.
	logged := sql.NullFloat64{Float64: 0, Valid: true}
	if okc {
		logged = sql.NullFloat64{Float64: total, Valid: true}
		w.Header().Set("x-litellm-response-cost", spend.Format(total))
		w.Header().Set("x-litellm-response-cost-original", spend.Format(total))
		w.Header().Set("x-litellm-response-cost-input", spend.Format(in))
		w.Header().Set("x-litellm-response-cost-output", spend.Format(out))
		if p.Key != nil {
			shown := p.Key.Spend + total
			if s.Live != nil {
				shown = p.Key.Spend + s.Live.HotSpend(p.Hash) + total
			}
			w.Header().Set("x-litellm-key-spend", spend.Format(shown))
		}
	}
	hash := ""
	teamID, userID, orgID := "", "", ""
	if p != nil {
		hash = p.Hash
		if p.Key != nil {
			teamID, userID, orgID = p.Key.TeamID, p.Key.UserID, p.Key.OrganizationID
		}
	}
	end := time.Now()
	tokens := pt + ct
	if tokens == 0 {
		tokens = asInt(usage["total_tokens"])
	}
	s.noteUsage(depID, tokens)
	row := live.SpendLog{
		RequestID: callID, CallType: "chat", Model: alias, APIKey: hash,
		Prompt: pt, Completion: ct, Spend: logged.Float64, SpendValid: logged.Valid,
		Start: start.UTC().Format(time.RFC3339Nano), End: end.UTC().Format(time.RFC3339Nano),
		CacheHit: cacheHit, Status: "success", TeamID: teamID, UserID: userID, OrgID: orgID,
	}
	if s.Live != nil && s.Live.EnqueueLog(row) == nil {
		if okc {
			_ = s.Live.ChargeSpend(hash, total)
			_ = s.Live.ChargeSpend(teamID, total)
			_ = s.Live.ChargeSpend(userID, total)
			_ = s.Live.ChargeSpend(orgID, total)
		}
		return
	}
	s.persistSpend(hash, teamID, userID, orgID, callID, alias, pt, ct, logged, start, end, cacheHit, total, okc && hash != "")
}

func (s *Server) persistSpend(hash, teamID, userID, orgID, callID, alias string, pt, ct int, logged sql.NullFloat64, start, end time.Time, cacheHit bool, total float64, charge bool) {
	if charge {
		_ = s.Store.AddSpend(hash, total)
		if teamID != "" {
			_ = s.Store.AddTeamSpend(teamID, total)
		}
		if userID != "" {
			_ = s.Store.AddUserSpend(userID, total)
		}
		if orgID != "" {
			_ = s.Store.AddOrgSpend(orgID, total)
		}
	}
	_ = s.Store.InsertSpendLog(callID, "chat", alias, hash, pt, ct, logged, start, end, cacheHit, "success")
}

func (s *Server) writeCacheHit(w http.ResponseWriter, p *auth.Principal, callID, alias, ck string, hit []byte, start time.Time) {
	w.Header().Set("cache_hit", "true")
	w.Header().Set("x-litellm-cache-hit", "true")
	w.Header().Set("x-litellm-cache-key", ck)
	w.Header().Set("x-litellm-model-name", alias)
	w.Header().Set("x-litellm-version", Version)
	var parsed map[string]any
	if json.Unmarshal(hit, &parsed) == nil {
		if usage, ok := parsed["usage"].(map[string]any); ok {
			s.recordSpend(w, p, callID, alias, usage, start, true, "")
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("x-litellm-response-duration-ms", strconv.FormatInt(time.Since(start).Milliseconds(), 10))
	w.WriteHeader(200)
	_, _ = w.Write(hit)
}

func (s *Server) writeChatJSON(w http.ResponseWriter, p *auth.Principal, callID, alias, ck, op, provider string, respBody []byte, status int, start time.Time, depID string) {
	if op == "audio_speech" {
		s.recordSpend(w, p, callID, alias, map[string]any{"prompt_tokens": 8, "completion_tokens": 2, "total_tokens": 10}, start, false, depID)
		if w.Header().Get("Content-Type") == "" {
			w.Header().Set("Content-Type", "audio/mpeg")
		}
		w.Header().Set("x-litellm-response-duration-ms", strconv.FormatInt(time.Since(start).Milliseconds(), 10))
		w.WriteHeader(status)
		_, _ = w.Write(respBody)
		return
	}
	respBody = router.DecodeResponse(op, provider, alias, respBody)
	var parsed map[string]any
	if json.Unmarshal(respBody, &parsed) == nil {
		if op == "chat" || op == "" || op == "completions" || op == "embeddings" || op == "messages" || op == "responses" || op == "moderations" || op == "videos" {
			parsed["model"] = alias
		}
		if op == "chat" || op == "" {
			if _, ok := parsed["system_fingerprint"]; !ok {
				parsed["system_fingerprint"] = nil
			}
			if _, ok := parsed["object"]; !ok {
				parsed["object"] = "chat.completion"
			}
			if _, ok := parsed["created"]; !ok {
				parsed["created"] = time.Now().UTC().Unix()
			}
		}
		if op == "embeddings" {
			if _, ok := parsed["object"]; !ok {
				parsed["object"] = "list"
			}
			if _, ok := parsed["data"]; !ok {
				parsed["data"] = []any{}
			}
		}
		if op == "completions" {
			if _, ok := parsed["object"]; !ok {
				parsed["object"] = "text_completion"
			}
		}
		usage, _ := parsed["usage"].(map[string]any)
		if usage == nil {
			if um, ok := parsed["usageMetadata"].(map[string]any); ok {
				usage = map[string]any{
					"prompt_tokens":     um["promptTokenCount"],
					"completion_tokens": um["candidatesTokenCount"],
					"total_tokens":      um["totalTokenCount"],
				}
			}
		}
		if usage == nil {
			usage = map[string]any{"prompt_tokens": 8, "completion_tokens": 2, "total_tokens": 10}
		}
		if _, ok := usage["prompt_tokens"]; !ok {
			usage["prompt_tokens"] = usage["input_tokens"]
			usage["completion_tokens"] = usage["output_tokens"]
		}
		s.recordSpend(w, p, callID, alias, usage, start, false, depID)
		respBody, _ = json.Marshal(parsed)
		s.Cache.Set(ck, respBody)
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("x-litellm-response-duration-ms", strconv.FormatInt(time.Since(start).Milliseconds(), 10))
	w.WriteHeader(status)
	_, _ = w.Write(respBody)
}

func pipeStream(w http.ResponseWriter, resp *http.Response) (bool, map[string]any) {
	defer resp.Body.Close()
	buf := make([]byte, 4096)
	flusher, _ := w.(http.Flusher)
	wrote := false
	var pending []byte
	var usage map[string]any
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			if !wrote {
				w.Header().Set("Content-Type", "text/event-stream")
				w.WriteHeader(resp.StatusCode)
				wrote = true
			}
			_, _ = w.Write(buf[:n])
			if flusher != nil {
				flusher.Flush()
			}
			pending = append(pending, buf[:n]...)
			usage = streamUsage(pending, usage)
			if len(pending) > 1<<20 {
				pending = pending[len(pending)-4096:]
			}
		}
		if err != nil {
			return wrote, usage
		}
	}
}

func streamUsage(raw []byte, prev map[string]any) map[string]any {
	for _, line := range bytes.Split(raw, []byte("\n")) {
		line = bytes.TrimSpace(line)
		line = bytes.TrimPrefix(line, []byte("data:"))
		line = bytes.TrimSpace(line)
		if len(line) == 0 || bytes.Equal(line, []byte("[DONE]")) {
			continue
		}
		var doc map[string]any
		if json.Unmarshal(line, &doc) != nil {
			continue
		}
		if u, ok := doc["usage"].(map[string]any); ok {
			prev = u
		}
	}
	return prev
}
