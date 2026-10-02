// Package auth resolves the caller of an HTTP request. The master key, a virtual key, and a session do not grant the same rights.
package auth

import (
	"encoding/base64"
	"net/http"
	"strings"
	"time"

	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/logx"
	"github.com/sunqirui1987/xhub/internal/store"
	"sync"
)

// Principal is the caller of one request. Kind is master, virtual, or session. The master key cannot call inference unless configuration allows it.
type Principal struct {
	Kind     string // master | virtual | session
	Master   bool
	ViewOnly bool
	Role     string
	UserID   string
	Key      *store.Key
	Plain    string
	Hash     string
}

var logTraceOnceAuth sync.Once

// stripBearer removes a Bearer prefix and surrounding space. Without that prefix it returns the trimmed text.
func stripBearer(v string) string {
	logTraceOnceAuth.Do(func() { logx.Trace("enter auth.stripBearer") })

	v = strings.TrimSpace(v)
	if len(v) >= 7 && strings.EqualFold(v[:7], "bearer ") {
		return strings.TrimSpace(v[7:])
	}
	return v
}

// APIKeyFrom reads the credential in LiteLLM order: x-litellm-api-key, then Authorization, then api-key and x-api-key.
func APIKeyFrom(r *http.Request) string {
	// Same precedence as LiteLLM user_api_key_auth: x-litellm-api-key wins,
	// then Authorization, then the Azure/OpenAI api-key headers.
	if v := stripBearer(r.Header.Get("x-litellm-api-key")); v != "" {
		return v
	}
	h := r.Header.Get("Authorization")
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(h)), "bearer ") {
		return stripBearer(h)
	}
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(h)), "basic ") {
		raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(h[6:]))
		if err == nil {
			parts := strings.SplitN(string(raw), ":", 2)
			if len(parts) == 2 {
				return parts[1]
			}
			return string(raw)
		}
	}
	if v := stripBearer(r.Header.Get("api-key")); v != "" {
		return v
	}
	if v := stripBearer(r.Header.Get("x-api-key")); v != "" {
		return v
	}
	return ""
}

// Resolve turns the request into a Principal. A blocked or expired key returns an error instead of a Principal that cannot be used.
func Resolve(cfg *config.Config, st *store.Store, r *http.Request) (*Principal, error) {
	plain := APIKeyFrom(r)
	if plain == "" {
		return nil, errNo
	}
	if plain == cfg.GeneralSettings.MasterKey {
		return &Principal{Kind: "master", Master: true, Plain: plain}, nil
	}
	hash := store.HashKey(plain)
	k, err := st.GetByHash(hash)
	if err != nil {
		return nil, errNo
	}
	if k.Blocked.Valid && k.Blocked.Bool {
		return nil, errBlocked
	}
	if k.ExpiresAt.Valid && time.Now().After(k.ExpiresAt.Time) {
		return nil, errExpired
	}
	return &Principal{Kind: "virtual", Key: k, Plain: plain, Hash: hash}, nil
}

// CanManage reports whether this caller may use management routes. The master key may. A virtual key may when its type is management or default.
func (p *Principal) CanManage() bool {
	if p == nil {
		return false
	}
	if p.Master {
		return true
	}
	if p.Kind == "session" {
		return p.Role == "proxy_admin" || p.Role == "proxy_admin_viewer"
	}
	if p.Key != nil && (p.Key.KeyType == "management" || p.Key.KeyType == "default") {
		return true
	}
	return false
}

// CanLLM reports whether this caller may send inference. An authenticated
// session may use inference; ViewOnly limits management writes, not the
// models granted to the session. The master key may only when
// AllowMasterKeyLLM is set. A management key may not.
func (p *Principal) CanLLM(cfg *config.Config) bool {
	if p == nil {
		return false
	}
	if p.Kind == "session" {
		return true
	}
	if p.Master {
		return cfg.GeneralSettings.AllowMasterKeyLLM
	}
	if p.Key == nil {
		return false
	}
	switch p.Key.KeyType {
	case "llm_api", "default", "":
		return true
	default:
		return false
	}
}

type authErr string

// Error returns the authentication failure text, such as invalid_api_key, key_blocked, or key_expired.
func (e authErr) Error() string { return string(e) }

// ErrSessionInvalid is returned when a signed UI session no longer maps to a live user.
// It is kept in this package so the gateway can reject a stale session without exposing store details to auth callers.
func ErrSessionInvalid() error { return authErr("invalid_session") }

var (
	errNo      = authErr("invalid_api_key")
	errBlocked = authErr("key_blocked")
	errExpired = authErr("key_expired")
)

// IsAuthErr reports a missing, blocked, or expired key. Other errors must not be treated as a 401 authentication failure.
func IsAuthErr(err error) bool {
	_, ok := err.(authErr)
	return ok
}
