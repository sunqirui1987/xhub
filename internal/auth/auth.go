// 从请求里解析调用方身份。主密钥、虚拟密钥和会话的权限不同。
package auth

import (
	"encoding/base64"
	"net/http"
	"strings"
	"time"

	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/store"
)

// 一次请求的调用方。Kind 为 master、virtual 或 session。主密钥默认不能打推理，除非配置允许。
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

// 去掉 Bearer 前缀和空白。没有前缀时返回去掉空白后的原文。
func stripBearer(v string) string {
	v = strings.TrimSpace(v)
	if len(v) >= 7 && strings.EqualFold(v[:7], "bearer ") {
		return strings.TrimSpace(v[7:])
	}
	return v
}

// 按 LiteLLM 的顺序取密钥：x-litellm-api-key，其次 Authorization，再是 api-key 和 x-api-key。
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

// 把请求解析成身份。密钥被屏蔽或过期时返回错误，不返回一个不能用的 Principal。
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

// 主密钥，以及 key_type 为 management 或 default 的虚拟密钥，可以调用管理接口。
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

// 会话在非只读时可以推理。主密钥只有 AllowMasterKeyLLM 为真时可以。管理密钥不行。
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

// 身份错误的文本，例如 invalid_api_key、key_blocked 或 key_expired。
func (e authErr) Error() string { return string(e) }

var (
	errNo      = authErr("invalid_api_key")
	errBlocked = authErr("key_blocked")
	errExpired = authErr("key_expired")
)

// 是否为密钥缺失、屏蔽或过期。其它错误不能当成 401 的身份失败。
func IsAuthErr(err error) bool {
	_, ok := err.(authErr)
	return ok
}
