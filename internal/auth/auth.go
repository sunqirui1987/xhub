package auth

import (
	"encoding/base64"
	"net/http"
	"strings"
	"time"

	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/store"
)

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

func stripBearer(v string) string {
	v = strings.TrimSpace(v)
	if len(v) >= 7 && strings.EqualFold(v[:7], "bearer ") {
		return strings.TrimSpace(v[7:])
	}
	return v
}

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
	if k.Blocked {
		return nil, errBlocked
	}
	if k.ExpiresAt.Valid && time.Now().After(k.ExpiresAt.Time) {
		return nil, errExpired
	}
	return &Principal{Kind: "virtual", Key: k, Plain: plain, Hash: hash}, nil
}

func (p *Principal) CanManage() bool {
	if p == nil {
		return false
	}
	if p.Master {
		return true
	}
	if p.Key != nil && (p.Key.KeyType == "management" || p.Key.KeyType == "default") {
		return true
	}
	return false
}

func (p *Principal) CanLLM(cfg *config.Config) bool {
	if p == nil {
		return false
	}
	if p.Kind == "session" {
		return !p.ViewOnly
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

func (e authErr) Error() string { return string(e) }

var (
	errNo      = authErr("invalid_api_key")
	errBlocked = authErr("key_blocked")
	errExpired = authErr("key_expired")
)

func IsAuthErr(err error) bool {
	_, ok := err.(authErr)
	return ok
}
