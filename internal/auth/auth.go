// Package auth answers one question: who is calling. Every question of what the
// caller may then do belongs to internal/authz, and no decision is made here.
//
// A caller proves itself with the master credential or a virtual key. UI
// sessions do not arrive through Resolve, because a session is gateway state —
// a signed token checked against a stored row — and the gateway resolves it
// before this package is reached.
package auth

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/sunqirui1987/xhub/internal/authz"
	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/iam"
	"github.com/sunqirui1987/xhub/internal/logx"
)

// Failure codes reported to the client.
const (
	CodeInvalidKey = "invalid_api_key"
	CodeBlocked    = "key_blocked"
	CodeRevoked    = "key_revoked"
	CodeExpired    = "key_expired"
)

// Error is an authentication failure carrying the code the response reports.
// Its wrapped error is an authz sentinel, so the gateway can decide the status
// from the authorization package's helpers alone.
type Error struct {
	Code string
	Err  error
}

// 实现 error 接口，返回写进日志或 HTTP 错误体的文本。
// 参数：无。
// 返回 string（string）：error 接口的文本，给日志和 HTTP 错误体使用。
// 调用：authz/authz.go、authz/decide.go、authz/scope.go、catalog/embed.go
// 测试：无直接单测
func (e *Error) Error() string { return e.Code + ": " + e.Err.Error() }

// 交出被包装的底层错误，供 errors.Is 和 errors.As 识别具体类型。
// 参数：无。
// 返回 error（error）：失败原因，nil 表示这一步成功。
// 调用：authz/authz.go、gateway/engine.go
// 测试：无直接单测
func (e *Error) Unwrap() error { return e.Err }

// Code reports the failure code to send, defaulting to invalid_api_key for an error this package did not raise. A dependency failure keeps its own code out of the response: it is a 500, not a rejected credential.
// 参数 err（error）：失败原因，nil 表示这一步成功。
// 返回 string（string）：要返回给调用方的错误码。认不出包装类型时用 invalid_api_key。
// 调用：gateway/session.go
// 测试：无直接单测
func Code(err error) string {
	var target *Error
	if errors.As(err, &target) {
		return target.Code
	}
	return CodeInvalidKey
}

// 把错误码和底层错误包成 *Error，供 Code 以后取回错误码。
// 参数 code（string）：错误码或业务码；err（error）：失败原因，nil 表示成功。
// 返回 error（error）：失败原因，nil 表示这一步成功。
// 调用：仅在 auth.go 内使用
// 测试：无直接单测
func fail(code string, err error) error { return &Error{Code: code, Err: err} }

// Principal is the caller of one request, in the form the gateway and the
// authorization layer both need: the actor to decide on, plus the key row the
// spend and rate-limit paths read their limits from.
type Principal struct {
	Kind authz.Kind
	// Role is the platform role and is meaningful for a session only. A key
	// never carries one: a key grants inference and the ability to read itself,
	// whatever role its owner happens to hold.
	Role   string
	UserID string
	// KeyID, Key and Hash are set for a virtual key. Hash is the token hash,
	// which is also the live-spend and rate-limit reference.
	KeyID string
	Key   *iam.Key
	Hash  string
	// TeamID, ProjectID and OwnerType describe a key's binding. Only a key has
	// them; a session reaches teams through its memberships.
	TeamID    string
	ProjectID string
	OwnerType string
	// Session and Version identify a UI session. Version is the user row's
	// session_version when the session was issued, and a request is refused
	// once the stored version has moved past it.
	Session string
	Version int
}

// Actor is the authorization-layer view of this caller.
// 参数：无。
// 调用：authz/authz.go、gateway/session.go
// 测试：无直接单测
// 返回：给授权层用的调用方视图。nil 接收者得到零值 Actor，不携带用户或密钥。
func (p *Principal) Actor() authz.Actor {
	if p == nil {
		return authz.Actor{}
	}
	return authz.Actor{
		Kind:         p.Kind,
		UserID:       p.UserID,
		Role:         p.Role,
		KeyID:        p.KeyID,
		KeyTeamID:    p.TeamID,
		KeyProjectID: p.ProjectID,
		OwnerType:    p.OwnerType,
	}
}

// IsMaster reports the emergency master credential, which only reaches the bootstrap and emergency routes.
// 参数：无。
// 返回 bool（bool）：当前身份是紧急主密钥时返回真。它只用于引导和应急路由。
// 调用：gateway/models/access.go、gateway/models/list.go、gateway/session.go
// 测试：无直接单测
func (p *Principal) IsMaster() bool { return p != nil && p.Kind == authz.KindMaster }

// PlatformAdmin reports a platform administrator session. A key never confers it, whatever role its owner holds: a key is a credential for inference and for reading itself, and it does not become an administration credential by belonging to an administrator.
// 参数：无。
// 返回 bool（bool）：当前会话是平台管理员时返回真。密钥不会因为主人是管理员而变成管理凭证。
// 调用：authz/authz.go、authz/decide.go、gateway/identity/handlers.go、gateway/models/list.go
// 测试：无直接单测
func (p *Principal) PlatformAdmin() bool {
	return p != nil && p.Kind == authz.KindSession && p.Role == iam.RoleAdmin
}

// CanInfer reports whether this credential may send inference. Any active session may; a key may once the gateway has revalidated it for this request.
// 参数：无。
// 返回 bool（bool）：这个凭证可以发起推理时返回真。会话通过后即可；密钥要等本次请求重新校验通过。
// 调用：dataplane/serve.go、gateway/wire.go
// 测试：无直接单测
func (p *Principal) CanInfer() bool {
	return p != nil && (p.Kind == authz.KindSession || p.Kind == authz.KindKey)
}

// stripBearer 去掉凭证头里的 Bearer 前缀和首尾空白。没有这个前缀时原样返回。
// 参数 v（string）：Authorization 或 x-api-key 头的原文。
// 返回：去掉 Bearer 前缀和两端空白后的密钥。没有前缀时就是去掉空白的原文。空串表示头是空的。
// 调用：APIKeyFrom，用来从各个凭证头里取出明文。
// 测试：无直接单测
func stripBearer(v string) string {
	v = strings.TrimSpace(v)
	if len(v) >= 7 && strings.EqualFold(v[:7], "bearer ") {
		return strings.TrimSpace(v[7:])
	}
	return v
}

// APIKeyFrom reads the credential in LiteLLM order: x-litellm-api-key, then Authorization, then api-key and x-api-key.
// 参数 r（*http.Request）：入站 HTTP 请求，用来读路径、头和正文。
// 调用：gateway/engine.go、gateway/session.go
// 测试：无直接单测
// 返回：按 x-litellm-api-key、Authorization、api-key、x-api-key 的顺序找到的第一把密钥。都没有时为空串。
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

// Resolve 从请求里识别主密钥或 API 密钥，并组装调用方。
// 参数 ctx（context.Context）：取消时停止密钥查询；cfg（*config.Config）：读取主密钥；db（*iam.DB）：按哈希查密钥行，没有库时无法识别普通密钥；r（*http.Request）：从 Authorization 或 x-api-key 取明文。
// 返回 *Principal（*Principal）：主密钥或已通过校验的密钥调用方。失败时为 nil；error（error）：没有密钥、密钥不可用或查库失败。nil 表示识别成功。
// 调用：gateway 的 resolve。
// 测试：无直接单测
func Resolve(ctx context.Context, cfg *config.Config, db *iam.DB, r *http.Request) (*Principal, error) {
	plain := APIKeyFrom(r)
	if plain == "" {
		return nil, fail(CodeInvalidKey, authz.ErrUnauthenticated)
	}
	if master := cfg.GeneralSettings.MasterKey; master != "" && plain == master {
		logx.Debug("auth master credential used path=%s", r.URL.Path)
		return &Principal{Kind: authz.KindMaster}, nil
	}
	if db == nil {
		return nil, fail(CodeInvalidKey, authz.ErrUnauthenticated)
	}
	hash := iam.HashKey(plain)
	k, err := db.KeyByHash(ctx, hash)
	if err != nil {
		if errors.Is(err, iam.ErrNotFound) {
			logx.Debug("auth unknown key path=%s", r.URL.Path)
			return nil, fail(CodeInvalidKey, authz.ErrUnauthenticated)
		}
		logx.Error("auth key lookup failed path=%s err=%v", r.URL.Path, err)
		return nil, &authz.InternalError{Err: err}
	}
	if code := unusable(k); code != "" {
		logx.Debug("auth rejected key id=%s code=%s", k.ID, code)
		return nil, fail(code, authz.ErrForbidden)
	}
	logx.Debug("auth key accepted id=%s owner=%s", k.ID, k.OwnerType)
	return keyPrincipal(k, hash), nil
}

// unusable names why a key may not be used, or "" when it may. The lifecycle status is checked before the expiry so a revoked key is never reported as merely expired.
// 参数 k（*iam.Key）：密钥行，含哈希、限额和归属，不含明文。
// 返回 string（string）：密钥不能使用的原因。可以使用时为空串。
// 调用：仅在 auth.go 内使用
// 测试：无直接单测
func unusable(k *iam.Key) string {
	if k == nil {
		return CodeInvalidKey
	}
	if k.Status != iam.StatusActive {
		switch k.Status {
		case iam.StatusRevoked:
			return CodeRevoked
		default:
			return CodeBlocked
		}
	}
	if k.ExpiresAt != nil && !time.Now().Before(*k.ExpiresAt) {
		return CodeExpired
	}
	return ""
}

// 用密钥行组装推理调用方。用户和项目只在列非空时填上。
// 参数 k（*iam.Key）：密钥行，含哈希、限额和归属，不含明文；hash（string）：密钥哈希，用来对齐热花费和日志。
// 返回 *Principal（*Principal）：已经解析的调用方，含用户、团队和密钥。
// 调用：仅在 auth.go 内使用
// 测试：无直接单测
func keyPrincipal(k *iam.Key, hash string) *Principal {
	p := &Principal{Kind: authz.KindKey, KeyID: k.ID, Key: k, Hash: hash, TeamID: k.TeamID, OwnerType: k.OwnerType}
	if k.UserID != nil {
		p.UserID = *k.UserID
	}
	if k.ProjectID != nil {
		p.ProjectID = *k.ProjectID
	}
	return p
}

// SessionPrincipal builds the actor for a signed-in user. The stored row is the authority, not the token: a session is refused once the account is no longer active or its session_version has moved, which is how a role change, a disable and a password reset end every session the user had open.
// 参数 u（*iam.User）：用户行，含邮箱、角色和状态；version（int）：会话版本，改密后旧会话失效；session（string）：会话 id。
// 返回 *Principal（*Principal）：已经解析的调用方，含用户、团队和密钥；error（error）：失败原因，nil 表示这一步成功。
// 调用：gateway/session.go
// 测试：无直接单测
func SessionPrincipal(u *iam.User, version int, session string) (*Principal, error) {
	if u == nil || !u.Active() {
		return nil, fail(CodeInvalidKey, authz.ErrUnauthenticated)
	}
	if u.SessionVersion != version {
		logx.Debug("auth session stale user=%s want=%d got=%d", u.ID, u.SessionVersion, version)
		return nil, fail(CodeInvalidKey, authz.ErrUnauthenticated)
	}
	logx.Debug("auth session accepted user=%s role=%s", u.ID, u.Role)
	return &Principal{Kind: authz.KindSession, UserID: u.ID, Role: u.Role, Session: session, Version: version}, nil
}

// ErrSessionInvalid reports a session that no longer maps to a usable account: the user is gone, disabled, or has moved past the session's version.
// 参数：无。
// 返回 error（error）：失败原因，nil 表示这一步成功。
// 调用：gateway/session.go
// 测试：无直接单测
func ErrSessionInvalid() error { return fail(CodeInvalidKey, authz.ErrUnauthenticated) }
