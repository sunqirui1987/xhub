# MCP data plane

- Family: `data.mcp`
- Status: `specified`
- 参考: 下表定位列
- Console: /mcp-servers /mcp/oauth/callback
- Auth: mixed

## 目的

本族是网关下列 HTTP 路径的完整分母。字段名与默认值以冻结契约及 该路由 handler 的请求体模型与上游 SDK 透传字段 为准，网关不得改名。

## HTTP 表面

实现必须注册每一行 method+path。规范化列用于覆盖校验（去掉 `:path` 与尾斜杠）。

| Method | Path | 规范化 path | 参考 |
|---|---|---|---|
| `GET` | `/.well-known/oauth-authorization-server` | `/.well-known/oauth-authorization-server` | `proxy/_experimental/mcp_server/byok_oauth_endpoints.py:612` |
| `GET` | `/.well-known/oauth-authorization-server/{root_path:path}/v1/mcp/oauth` | `/.well-known/oauth-authorization-server/{root_path}/v1/mcp/oauth` | `proxy/_experimental/mcp_server/byok_oauth_endpoints.py:624` |
| `GET` | `/.well-known/oauth-protected-resource` | `/.well-known/oauth-protected-resource` | `proxy/_experimental/mcp_server/discoverable_endpoints.py:2564` |
| `GET` | `/authorize` | `/authorize` | `proxy/_experimental/mcp_server/discoverable_endpoints.py:1870` |
| `POST` | `/authorize/complete` | `/authorize/complete` | `proxy/_experimental/mcp_server/discoverable_endpoints.py:2041` |
| `GET` | `/authorize/flow` | `/authorize/flow` | `proxy/_experimental/mcp_server/discoverable_endpoints.py:2030` |
| `GET` | `/authorize/mcp-session` | `/authorize/mcp-session` | `proxy/_experimental/mcp_server/discoverable_endpoints.py:1845` |
| `GET` | `/callback` | `/callback` | `proxy/_experimental/mcp_server/discoverable_endpoints.py:2142` |
| `GET` | `/discover` | `/discover` | `proxy/management_endpoints/mcp_management_endpoints.py:2924` |
| `GET` | `/enabled` | `/enabled` | `proxy/_experimental/mcp_server/server.py:4839` |
| `POST` | `/introspect` | `/introspect` | `proxy/_experimental/mcp_server/discoverable_endpoints.py:2083` |
| `GET` | `/mcp` | `/mcp` | `catalog.families:0` |
| `POST` | `/mcp` | `/mcp` | `catalog.families:0` |
| `GET` | `/mcp/proxy` | `/mcp/proxy` | `catalog.families:0` |
| `POST` | `/mcp/proxy` | `/mcp/proxy` | `catalog.families:0` |
| `POST` | `/register` | `/register` | `proxy/_experimental/mcp_server/discoverable_endpoints.py:2856` |
| `POST` | `/revoke` | `/revoke` | `proxy/_experimental/mcp_server/discoverable_endpoints.py:2070` |
| `POST` | `/token` | `/token` | `proxy/_experimental/mcp_server/discoverable_endpoints.py:1944` |
| `GET` | `/token/generate` | `/token/generate` | `proxy/proxy_server.py:18744` |
| `GET` | `/v1/mcp/oauth/authorize` | `/v1/mcp/oauth/authorize` | `proxy/_experimental/mcp_server/byok_oauth_endpoints.py:651` |
| `POST` | `/v1/mcp/oauth/authorize` | `/v1/mcp/oauth/authorize` | `proxy/_experimental/mcp_server/byok_oauth_endpoints.py:718` |
| `POST` | `/v1/mcp/oauth/token` | `/v1/mcp/oauth/token` | `proxy/_experimental/mcp_server/byok_oauth_endpoints.py:781` |
| `GET` | `/{mcp_server_name}/authorize` | `/{mcp_server_name}/authorize` | `proxy/_experimental/mcp_server/discoverable_endpoints.py:1869` |
| `GET` | `/{mcp_server_name}/mcp` | `/{mcp_server_name}/mcp` | `catalog.families:0` |
| `POST` | `/{mcp_server_name}/mcp` | `/{mcp_server_name}/mcp` | `catalog.families:0` |
| `POST` | `/{mcp_server_name}/register` | `/{mcp_server_name}/register` | `proxy/_experimental/mcp_server/discoverable_endpoints.py:2855` |
| `POST` | `/{mcp_server_name}/token` | `/{mcp_server_name}/token` | `proxy/_experimental/mcp_server/discoverable_endpoints.py:1943` |

## 请求

数据面虚拟 Key，管理面 session/master key。OAuth 回调走浏览器 session。见 [鉴权决策](../../../architecture/auth-decision.md)。

冻结字段（不得改名）：`JSON-RPC method/params; OAuth: client_id`、`redirect_uri`、`code`、`code_verifier`。

### 请求头

| Header | 必填 | 说明 |
|---|---|---|
| `Authorization` | 是* | `Bearer sk-...` |
| `x-litellm-api-key` | 否 | 若出现则优先于 Authorization |
| `Content-Type` | 是 | `application/json` 或 `multipart/form-data` |
| `Idempotency-Key` | 否 | 有则重放 |
| `x-litellm-tags` | 否 | 标签 |
| `x-litellm-end-user-id` | 否 | 终端用户 |

### 请求体

```json
{"jsonrpc": "2.0", "id": 1, "method": "tools/list", "params": {}}
```

## 响应

冻结响应字段：`JSON-RPC result/error; token: access_token`、`token_type`、`expires_in`。

### 响应头

| Header | 说明 |
|---|---|
| `x-litellm-call-id` | 见 [错误与响应头](../../../architecture/errors-headers.md) |
| `x-litellm-model-id` | 见 [错误与响应头](../../../architecture/errors-headers.md) |
| `x-litellm-model-name` | 见 [错误与响应头](../../../architecture/errors-headers.md) |
| `x-litellm-model-api-base` | 见 [错误与响应头](../../../architecture/errors-headers.md) |
| `x-litellm-version` | 见 [错误与响应头](../../../architecture/errors-headers.md) |
| `x-litellm-response-cost` | 见 [错误与响应头](../../../architecture/errors-headers.md) |
| `x-litellm-response-cost-original` | 见 [错误与响应头](../../../architecture/errors-headers.md) |
| `x-litellm-response-cost-input` | 见 [错误与响应头](../../../architecture/errors-headers.md) |
| `x-litellm-response-cost-output` | 见 [错误与响应头](../../../architecture/errors-headers.md) |
| `x-litellm-key-tpm-limit` | 见 [错误与响应头](../../../architecture/errors-headers.md) |
| `x-litellm-key-rpm-limit` | 见 [错误与响应头](../../../architecture/errors-headers.md) |
| `x-litellm-key-max-budget` | 见 [错误与响应头](../../../architecture/errors-headers.md) |
| `x-litellm-key-spend` | 见 [错误与响应头](../../../architecture/errors-headers.md) |
| `x-litellm-cache-key` | 见 [错误与响应头](../../../architecture/errors-headers.md) |
| `x-litellm-response-duration-ms` | 见 [错误与响应头](../../../architecture/errors-headers.md) |
| `Retry-After` | 429 时 |

### 响应体

```json
{"jsonrpc": "2.0", "id": 1, "result": {"tools": []}}
```

## 错误

OpenAI 兼容路径使用 `{"error":{"message","type","code","param"}}`。Anthropic 原生用 Anthropic 错误形状。Gemini 用 Google 错误形状。鉴权失败 401，模型不允许 401/403，预算/RPM/TPM 429 + `Retry-After`，参数 400。未实现 Provider 明确错误。共性见 [错误与响应头](../../../architecture/errors-headers.md)。

## 副作用

数据面调用记 spend、占 RPM/TPM/并发、写 SpendLogs，可走 Guardrail 与响应缓存。见 [生命周期](../../../architecture/request-lifecycle.md) 与 [计量](../../../architecture/spend-limits.md)。

## 控制台绑定

Console: /mcp-servers /mcp/oauth/callback。动作对照 [前端绑定](../../../../frontend/binding.md) 与 [页面规格](../../../../frontend/pages/README.md)。无控制台入口的路径仍须对 SDK/curl 可用。

## 验收

- 本表每一行 method+path 均已注册；未实现时返回 JSON 错误信封，不是 HTML。
- 鉴权失败与成功主路径合同测试。
- 标准客户端只改 base_url + api_key（数据面）。
