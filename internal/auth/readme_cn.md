# 调用身份解析

[English](readme.md) · [全功能实现说明](../../docs/development/implementation.md)

## 职责与实现契约

auth.go 把 HTTP 凭据转为 Principal，统一记录身份类型、角色、用户、密钥、组织、团队、项目以及模板来源。APIKeyFrom 依次读取 x-litellm-api-key、Authorization、api-key、x-api-key。Resolve 识别 master 或数据库中的虚拟密钥；网页登录的 token 验证由网关完成，再调用 SessionPrincipal。
密钥验证重新读取当前密钥、所有者、成员关系和父级资源，不能用陈旧缓存继续授权。禁用、撤销、过期分别有稳定错误码。SessionPrincipal 对比账号状态和 session_version，密码重置、禁用和版本变更使旧会话失效。
Principal.Actor 为 authz 提供动作判断输入；PlatformAdmin 只认管理员会话，管理员拥有的 API key 不因此获得管理权限。主密钥是应急凭据，不能当作日常登录或推理密钥。依赖读取失败是内部错误，不能伪装成凭据无效。

## 源码职责与入口

### auth.go

公开类型：`Error`, `Principal`.

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

## 对外 HTTP 边界

无本目录直接登记的 HTTP 路由。导出的 Go API 由上层调用；运行时目录调度或调用宿主的入口应沿依赖链追踪。

## 依赖关系

[internal/authz](../authz/readme_cn.md), [internal/config](../config/readme_cn.md), [internal/iam](../iam/readme_cn.md), [internal/logx](../logx/readme_cn.md).

## 验证与维护入口

| 测试文件 | 场景入口 |
| --- | --- |
| [auth_test.go](auth_test.go) | `TestBasicCredentialTrimsHeaderBeforeSlicing` |

```bash
go test ./internal/auth -count=1
```

数据库验收设置 XHUB_REGRESSION_STRICT=1 并检查跳过项；Redis 和真实供应商需单独配置。接口、字段或行为改变后同步本说明及相关功能文档。
