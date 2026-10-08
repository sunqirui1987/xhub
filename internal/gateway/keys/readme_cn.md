# 虚拟密钥与服务密钥生命周期

[English](readme.md) · [全功能实现说明](../../../docs/development/implementation.md)

## 职责与实现契约

generate.go 创建个人密钥和服务账号密钥，admin.go 实现列表、详情、编辑、批量更新、禁用/恢复、删除、轮换及费用重置。Host 依赖认证、授权、IAM、配置和模板可见性；HTTP 接口见 mount.go。
创建和再生成是 plaintext 唯一正常展示边界；Response 的 includePlain 参数控制返回，普通列表详情不能泄漏原始凭据。服务密钥的 owner_type、组织、团队、项目归属必须校验，不能伪装成个人密钥规避作用域。
密钥管理员身份与可用模型不互相替代。编辑路由模板前读取并授权，空模板表示继承。禁用、过期、轮换和删除后推理请求要重新读取状态，旧密钥不能继续调用。批量操作需逐个校验对象，而不是只对整个 HTTP 请求做一次宽泛授权。

## 源码职责与入口

### admin.go

- [`func ServiceAccount(s Host, w http.ResponseWriter, r *http.Request)`](admin.go) — ServiceAccount generates a service key for a team or one of its projects. It has no owner, so it requires the team's administration rather than mere membership; Authorize makes that decision.
- [`func Regenerate(s Host, w http.ResponseWriter, r *http.Request)`](admin.go) — Regenerate rotates the key plaintext. The old plaintext stops working immediately, and the key's own limits and narrowing are untouched.
- [`func ResetSpend(s Host, w http.ResponseWriter, r *http.Request)`](admin.go) — ResetSpend sets the key spend back to zero, or to reset_to when the body carries one. Historical usage rows are not deleted, so the figures that produced the old total remain.
- [`func Aliases(s Host, w http.ResponseWriter, r *http.Request)`](admin.go) — Aliases lists the key names the caller may see, for the dashboard pickers.
- [`func Health(s Host, w http.ResponseWriter, r *http.Request)`](admin.go) — Health checks that a credential still passes identification. It is the liveness probe the LiteLLM clients call before their first request.
- [`func BulkUpdate(s Host, w http.ResponseWriter, r *http.Request)`](admin.go) — BulkUpdate applies one patch to many keys. A key the caller may not write is skipped rather than failing the batch, and updated is the count that landed.

### codec.go

内部实现和协议边界见 [codec.go](codec.go)。

### generate.go

- [`func Generate(s Host, w http.ResponseWriter, r *http.Request)`](generate.go) — Generate creates a virtual key and returns the plaintext only in this response. A member may mint a personal key for themselves inside a team they belong to; a service key needs the team's administration, and the decision is made by Authorize rather than here.
- [`func List(s Host, w http.ResponseWriter, r *http.Request)`](generate.go) — List lists the keys the caller may see and never returns plaintext. The rows are narrowed in SQL by the scope, so a handler bug cannot widen the listing.
- [`func Info(s Host, w http.ResponseWriter, r *http.Request)`](generate.go) — Info reads one virtual key.
- [`func Delete(s Host, w http.ResponseWriter, r *http.Request)`](generate.go) — Delete removes virtual keys. The plaintext can no longer call inference.
- [`func Block(s Host, w http.ResponseWriter, r *http.Request)`](generate.go) — Block marks a virtual key blocked.
- [`func Unblock(s Host, w http.ResponseWriter, r *http.Request)`](generate.go) — Unblock clears the blocked flag.
- [`func Update(s Host, w http.ResponseWriter, r *http.Request)`](generate.go) — Update changes a virtual key's name, narrowing, and limits. The plaintext stays the same, and a field the caller left out keeps its value.
- [`func Response(k iam.Key, plain string, includePlain bool) map[string]any`](generate.go) — Response is the public JSON for a virtual key. The plaintext is omitted when includePlain is false; the stored hash never appears, because the hash is what authentication compares and publishing it would leak the credential.

### host.go

公开类型：`Host`.

内部实现和协议边界见 [host.go](host.go)。

### mount.go

- [`func Module(h Host) httpx.Module`](mount.go) — Module creates, lists, updates, and rotates virtual keys.

## 对外 HTTP 边界

入口由下表所列注册文件安装。别名共享处理器；路径存在不替代权限和业务断言，授权说明见模块契约及接口参考。

| Method / path | 注册文件 |
| --- | --- |
| `POST /key/generate` | [mount.go](mount.go) |
| `POST /key/service-account/generate` | [mount.go](mount.go) |
| `GET /key/list` | [mount.go](mount.go) |
| `GET /key/info` | [mount.go](mount.go) |
| `POST /v2/key/info` | [mount.go](mount.go) |
| `POST /key/delete` | [mount.go](mount.go) |
| `POST /key/block` | [mount.go](mount.go) |
| `POST /key/unblock` | [mount.go](mount.go) |
| `POST /key/update` | [mount.go](mount.go) |
| `POST /key/bulk_update` | [mount.go](mount.go) |
| `POST /key/regenerate` | [mount.go](mount.go) |
| `POST /key/{key}/regenerate` | [mount.go](mount.go) |
| `POST /key/{key}/reset_spend` | [mount.go](mount.go) |
| `GET /key/aliases` | [mount.go](mount.go) |
| `POST /key/health` | [mount.go](mount.go) |

## 依赖关系

[internal/auth](../../auth/readme_cn.md), [internal/authz](../../authz/readme_cn.md), [internal/gateway/templateauth](../templateauth/readme_cn.md), [internal/httpx](../../httpx/readme_cn.md), [internal/iam](../../iam/readme_cn.md), [internal/logx](../../logx/readme_cn.md).

## 验证与维护入口

当前目录没有直接测试文件；上层集成测试仅证明被执行的链路，不代表所有内部失败分支均已覆盖。

```bash
go test ./internal/gateway/keys -count=1
```

数据库验收设置 XHUB_REGRESSION_STRICT=1 并检查跳过项；Redis 和真实供应商需单独配置。接口、字段或行为改变后同步本说明及相关功能文档。
