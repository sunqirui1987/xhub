# 上游请求构造与协议适配

[English](readme.md) · [全功能实现说明](../../docs/development/implementation.md)

## 职责与实现契约

对话协议转换、凭据填充、请求过滤和直通辅助函数为数据面提供支持。`dialogue.go` 通过统一结构转换受支持的对话协议；`credential.go` 填充部署参数；`allow.go` 与 `proxy_params.go` 限制请求；`passthrough.go` 拼接传输地址。`call.go` 只定义操作名常量。
本包不发送 HTTP、不执行重试、不访问数据库，也不记录费用。数据面选择部署后负责超时、流生命周期和结算；API 根地址和端点构造由选中的供应商实现负责。
代理专属参数不应泄漏给供应商。协议 usage、错误和流事件需要保留原始语义，尤其是缓存 token 与明确零值。新增适配器需提供成功样本、失败样本和真实模型请求证据。

## 子目录与协作边界

| 目录 | 职责 |
| --- | --- |
| [estimate](estimate/readme_cn.md) | Token 与费用估算 |

## 源码职责与入口

### allow.go

- [`func PathPrefixes() []string`](allow.go) — PathPrefixes returns the prefixes in the allowlist.py order so a test can compare them with the Python source.
- [`func Allow(path string, mount bool) bool`](allow.go) — Allow reports whether a path may stay on the gateway process. When mount is true only the mount table is checked, matching a FastAPI Mount. An ordinary route checks an exact path first, then a prefix. An empty path is not a data-plane path. The comparison uses the path from route registration, not a URL with a query string.
- [`func Filter(next http.Handler) http.Handler`](allow.go) — Filter drops management routes before the process serves them. The Python gateway trims the route table after startup, once registration is finished. Here the request has already reached a registeredhandler, and the same list decides whether to allow it or return 404. The effect matches: the data plane stays, and management routes such as /key/generate are not served by this filter.

### build.go

公开类型：`Upstream`，保存供应商调用使用的 URL、请求头和正文。具体构造由选中的供应商实现负责。

原生 Responses 的转发、usage 与 continuation 由 [provider/openai](../provider/openai/readme_cn.md) 和 [dataplane](../dataplane/readme_cn.md) 负责，不能用已经移除的 Responses-to-chat 转换函数作为验证入口。

### call.go

本文件定义路由和供应商实现共享的 `Op*` 操作常量，不负责选择默认 API 根地址、构造请求头或编码供应商请求。

### credential.go

- [`func Hydrate(params, credentialValues map[string]any) map[string]any`](credential.go) — Hydrate fills deployment parameters that are still empty from a named credential. LiteLLM load_credentials_from_list writes a value only when the key is entirely absent. Deployments in this gateway often store api_key as an empty string after a redacted response is read back, so an empty string also counts as unset. api_key is the exception. The dashboard masks a stored key as ***** and the update handlerkeeps the previous value, so a model can keep a stale key after the user points it at a credential. A non-blank api_key on the credential replaces that stale key. Every other credential fieldstill fills only a blank deployment field, and a non-empty value there still wins. os.environ/NAME inside a string is expanded on this call, not when the credential is saved. The same credential can fo

### dialogue.go

公开类型：`Turn`、`Dialogue`、`DialogueResult`。

- [`func ParseDialogue(protocol string, body map[string]any) (Dialogue, error)`](dialogue.go) — 把受支持协议的请求转换为统一对话结构，不支持的协议返回错误。
- [`func EncodeDialogue(d Dialogue, protocol, model string) (map[string]any, error)`](dialogue.go) — 把统一对话转换为选定的上游协议和模型。
- [`func ParseDialogueResult(protocol string, raw []byte) (DialogueResult, error)`](dialogue.go) — 解析受支持的上游响应，并保留协议用量事实。
- [`func EncodeDialogueResult(r DialogueResult, protocol, model string) ([]byte, error)`](dialogue.go) — 输出选定的公开响应协议和模型名。
- [`func NormalizeDialogueUsage(u map[string]any) map[string]any`](dialogue.go) — 规范用量字段，同时保留明确零值和缓存 token 数量。

### error.go

- [`func ExceptionForStatus(status int) (name string, ok bool)`](error.go) — ExceptionForStatus maps an upstream HTTP status to a LiteLLM exception class name. Status 400 and 422 are BadRequestError. Status 408 and 504 are Timeout. A status without its own branch returns ok false so the caller treats it as an unknown upstream error.

### gemini.go

内部实现和协议边界见 [gemini.go](gemini.go)。

供应商协议分类由 [provider/capability.go](../provider/capability.go) 的能力定义和 [build.go](build.go) 的分派处理，未知协议必须沿真实失败路径验证。

### message.go

- [`func IsWildcardModel(model string) bool`](message.go) — IsWildcardModel reports whether a model name is a wildcard route.  LiteLLM uses * in a name to mean a group of models, so openai/* can catch openai/gpt-4o. An exact name wins. The router looks at these patterns only when no exact deployment exists.
- [`func NormalizeStreamOptions(opts map[string]any) (includeObfuscation bool, ok bool)`](message.go) — NormalizeStreamOptions reads include_obfuscation from Responses stream options. The field must be a boolean. A missing field, a wrong type, or options that are not an object all mean the caller did not give a usable switch, so ok is false. Do not treat the default as false and write it back onto the request.
- [`func ShapeResponsesMessage(message map[string]any) map[string]any`](message.go) — ShapeResponsesMessage reshapes one chat message into a form the Responses API accepts. An assistant message stays as it is. A user message whose content is a list of blocks, with a type of text, rewrites those blocks to input_text. Responses does not accept the chat-completion text type. Without such a block the message is returned unchanged, so a message that is already input_text is not wrappedagain.

### passthrough.go

- [`func PassthroughURL(base, endpoint, provider string) string`](passthrough.go) — PassthroughURL is the outbound address produced by LiteLLM _join_url_paths. It first joins the subpath onto the api_base path and rejects "..". If an OpenAI result still has no /v1/, v1 is inserted after api.openai.com/.
- [`func PassthroughSubpath(base, subpath string, include bool) string`](passthrough.go) — PassthroughSubpath matches HttpPassThroughEndpointHelpers.construct_target_url_with_subpath. When include is false or the subpath is empty, base is returned unchanged. Otherwise the subpath is normalized and joined.

### proxy_params.go

- [`func StripProxyParams(body map[string]any)`](proxy_params.go) — StripProxyParams deletes LiteLLM proxy fields and leaves the body the provider understands.

## 对外 HTTP 边界

无本目录直接登记的 HTTP 路由。导出的 Go API 由上层调用；运行时目录调度或调用宿主的入口应沿依赖链追踪。

## 依赖关系

[internal/logx](../logx/readme_cn.md).

## 验证与维护入口

| 测试文件 | 场景入口 |
| --- | --- |
| [allow_test.go](allow_test.go) | `TestAllowWithoutPublicSchema`, `TestFilterRejectsPublicSchema` |
| [call_test.go](call_test.go) | `TestHydrateCredentialAPIKeyReplacesStaleDeploymentKey`, `TestHydrateKeepsDeploymentKeyWhenCredentialHasNone` |
| [dialogue_test.go](dialogue_test.go) | `TestDialogueProtocolMatrix`, `TestDialogueRejectsUnsupported`, `TestDialogueResultMatrix` |
| [dialogue_google_test.go](dialogue_google_test.go) | `TestGoogleDialogueBoundaries`, `TestGoogleResultBoundaries` |

```bash
go test ./internal/llm -count=1
```

数据库验收设置 XHUB_REGRESSION_STRICT=1 并检查跳过项；Redis 和真实供应商需单独配置。接口、字段或行为改变后同步本说明及相关功能文档。
