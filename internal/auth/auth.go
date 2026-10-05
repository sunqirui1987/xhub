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

func (e *Error) Error() string { return e.Code + ": " + e.Err.Error() }
func (e *Error) Unwrap() error { return e.Err }

// Code reports the failure code to send, defaulting to invalid_api_key for an
// error this package did not raise. A dependency failure keeps its own code
// out of the response: it is a 500, not a rejected credential.
func Code(err error) string {
	var target *Error
	if errors.As(err, &target) {
		return target.Code
	}
	return CodeInvalidKey
}

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

// IsMaster reports the emergency master credential, which only reaches the
// bootstrap and emergency routes.
func (p *Principal) IsMaster() bool { return p != nil && p.Kind == authz.KindMaster }

// PlatformAdmin reports a platform administrator session. A key never confers
// it, whatever role its owner holds: a key is a credential for inference and
// for reading itself, and it does not become an administration credential by
// belonging to an administrator.
func (p *Principal) PlatformAdmin() bool {
	return p != nil && p.Kind == authz.KindSession && p.Role == iam.RoleAdmin
}

// CanInfer reports whether this credential may send inference. Any active
// session may; a key may once the gateway has revalidated it for this request.
func (p *Principal) CanInfer() bool {
	return p != nil && (p.Kind == authz.KindSession || p.Kind == authz.KindKey)
}

// stripBearer removes a Bearer prefix and surrounding space. Without that prefix it returns the trimmed text.
func stripBearer(v string) string {
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

// Resolve identifies the caller from the request.
//
// A missing credential, an unknown key and a key that may no longer be used all
// answer 401 with a code the client can read. A failed key lookup is different:
// it is returned as an authz internal error so the caller answers 500 instead
// of telling the holder their valid key is bad because the database blinked.
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

// unusable names why a key may not be used, or "" when it may. The lifecycle
// status is checked before the expiry so a revoked key is never reported as
// merely expired.
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

// SessionPrincipal builds the actor for a signed-in user. The stored row is the
// authority, not the token: a session is refused once the account is no longer
// active or its session_version has moved, which is how a role change, a
// disable and a password reset end every session the user had open.
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

// ErrSessionInvalid reports a session that no longer maps to a usable account:
// the user is gone, disabled, or has moved past the session's version.
func ErrSessionInvalid() error { return fail(CodeInvalidKey, authz.ErrUnauthenticated) }
