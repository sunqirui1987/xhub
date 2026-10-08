# Caller authentication

[简体中文](readme_cn.md) · [Feature implementation reference](../../docs/development/implementation.md)

## Responsibilities and behavior

auth.go turns credentials into Principal, carrying identity kind, role, owner, key, organization, team, project, and template metadata. Header precedence is x-litellm-api-key, Authorization, api-key, then x-api-key. Resolve handles master credentials and virtual keys; the gateway validates login tokens before SessionPrincipal.
Authentication reloads key status, ownership, membership, and parent resources. Blocked, revoked, and expired keys have distinct stable codes. SessionPrincipal checks account state and session_version, invalidating old sessions after password reset or version changes.
Actor supplies authorization input. PlatformAdmin recognizes administrator sessions, never an inference key owned by an administrator. Dependency failures remain internal errors. This package resolves identity; action and query permissions belong to authz.

## Source responsibilities and entry points

### auth.go

Exported types: `Error`, `Principal`.

- [`func (e *Error) Error() string`](auth.go) — 实现 error 接口，返回写进日志或 HTTP 错误体的文本。
- [`func (e *Error) Unwrap() error`](auth.go) — 交出被包装的底层错误，供 errors.Is 和 errors.As 识别具体类型。
- [`func Code(err error) string`](auth.go) — Code reports the failure code to send, defaulting to invalid_api_key for an error this package did not raise. A dependency failure keeps its own code out of the response: it is a 500, not a rejected credential.
- [`func (p *Principal) Actor() authz.Actor`](auth.go) — Actor is the authorization-layer view of this caller.
- [`func (p *Principal) IsMaster() bool`](auth.go) — IsMaster reports the emergency master credential, which only reaches the bootstrap and emergency routes.
- [`func (p *Principal) PlatformAdmin() bool`](auth.go) — PlatformAdmin reports a platform administrator session. A key never confers it, whatever role its owner holds: a key is a credential for inference and for reading itself, and it does not become an administration credential by belonging to an administrator.
- [`func (p *Principal) CanInfer() bool`](auth.go) — CanInfer reports whether this credential may send inference. Any active session may; a key may once the gateway has revalidated it for this request.
- [`func APIKeyFrom(r *http.Request) string`](auth.go) — APIKeyFrom reads the credential in LiteLLM order: x-litellm-api-key, then Authorization, then api-key and x-api-key.
- [`func Resolve(ctx context.Context, cfg *config.Config, db *iam.DB, r *http.Request) (*Principal, error)`](auth.go) — Resolve 从请求里识别主密钥或 API 密钥，并组装调用方。
- [`func SessionPrincipal(u *iam.User, version int, session string) (*Principal, error)`](auth.go) — SessionPrincipal builds the actor for a signed-in user. The stored row is the authority, not the token: a session is refused once the account is no longer active or its session_version has moved, which is how a role change, a disable and a password reset end every session the user had open.
- [`func ErrSessionInvalid() error`](auth.go) — ErrSessionInvalid reports a session that no longer maps to a usable account: the user is gone, disabled, or has moved past the session's version.

## External HTTP boundary

This directory registers no direct HTTP route. Higher layers call its Go API; trace catalog dispatch or host calls through the dependency chain.

## Dependencies

[internal/authz](../authz/readme.md), [internal/config](../config/readme.md), [internal/iam](../iam/readme.md), [internal/logx](../logx/readme.md).

## Verification and maintenance

| Test file | Scenario entry points |
| --- | --- |
| [auth_test.go](auth_test.go) | `TestBasicCredentialTrimsHeaderBeforeSlicing` |

```bash
go test ./internal/auth -count=1
```

Use XHUB_REGRESSION_STRICT=1 for database acceptance and inspect skips. Redis and live providers require separate configuration. Update this reference and feature documentation after contract changes.
