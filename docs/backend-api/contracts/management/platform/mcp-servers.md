# MCP server admin

- Family: `mgmt.mcp_servers`
- Status: `specified`
- 参考: 下表定位列
- Console: /mcp-servers
- Auth: management

## 目的

本族是网关下列 HTTP 路径的完整分母。字段名与默认值以冻结契约及 该路由 handler 的请求体模型与上游 SDK 透传字段 为准，网关不得改名。

## HTTP 表面

实现必须注册每一行 method+path。规范化列用于覆盖校验（去掉 `:path` 与尾斜杠）。

| Method | Path | 规范化 path | 参考 |
|---|---|---|---|
| `GET` | `/openapi-registry` | `/openapi-registry` | `proxy/management_endpoints/mcp_management_endpoints.py:2988` |
| `GET` | `/registry.json` | `/registry.json` | `proxy/management_endpoints/mcp_management_endpoints.py:987` |
| `GET` | `/server` | `/server` | `proxy/management_endpoints/mcp_management_endpoints.py:1106` |
| `POST` | `/server` | `/server` | `proxy/management_endpoints/mcp_management_endpoints.py:1569` |
| `PUT` | `/server` | `/server` | `proxy/management_endpoints/mcp_management_endpoints.py:2679` |
| `GET` | `/server/health` | `/server/health` | `proxy/management_endpoints/mcp_management_endpoints.py:1223` |
| `POST` | `/server/import` | `/server/import` | `proxy/management_endpoints/mcp_management_endpoints.py:1662` |
| `POST` | `/server/oauth/session` | `/server/oauth/session` | `proxy/management_endpoints/mcp_management_endpoints.py:1772` |
| `GET` | `/server/oauth/{server_id}/authorize` | `/server/oauth/{server_id}/authorize` | `proxy/management_endpoints/mcp_management_endpoints.py:1985` |
| `POST` | `/server/oauth/{server_id}/register` | `/server/oauth/{server_id}/register` | `proxy/management_endpoints/mcp_management_endpoints.py:2104` |
| `POST` | `/server/oauth/{server_id}/token` | `/server/oauth/{server_id}/token` | `proxy/management_endpoints/mcp_management_endpoints.py:2044` |
| `POST` | `/server/register` | `/server/register` | `proxy/management_endpoints/mcp_management_endpoints.py:1274` |
| `GET` | `/server/submissions` | `/server/submissions` | `proxy/management_endpoints/mcp_management_endpoints.py:1348` |
| `DELETE` | `/server/{server_id}` | `/server/{server_id}` | `proxy/management_endpoints/mcp_management_endpoints.py:2131` |
| `GET` | `/server/{server_id}` | `/server/{server_id}` | `proxy/management_endpoints/mcp_management_endpoints.py:1469` |
| `PUT` | `/server/{server_id}/approve` | `/server/{server_id}/approve` | `proxy/management_endpoints/mcp_management_endpoints.py:1378` |
| `DELETE` | `/server/{server_id}/oauth-user-credential` | `/server/{server_id}/oauth-user-credential` | `proxy/management_endpoints/mcp_management_endpoints.py:2335` |
| `POST` | `/server/{server_id}/oauth-user-credential` | `/server/{server_id}/oauth-user-credential` | `proxy/management_endpoints/mcp_management_endpoints.py:2266` |
| `GET` | `/server/{server_id}/oauth-user-credential/status` | `/server/{server_id}/oauth-user-credential/status` | `proxy/management_endpoints/mcp_management_endpoints.py:2374` |
| `PUT` | `/server/{server_id}/reject` | `/server/{server_id}/reject` | `proxy/management_endpoints/mcp_management_endpoints.py:1422` |
| `DELETE` | `/server/{server_id}/user-credential` | `/server/{server_id}/user-credential` | `proxy/management_endpoints/mcp_management_endpoints.py:2234` |
| `POST` | `/server/{server_id}/user-credential` | `/server/{server_id}/user-credential` | `proxy/management_endpoints/mcp_management_endpoints.py:2197` |
| `DELETE` | `/server/{server_id}/user-env-vars` | `/server/{server_id}/user-env-vars` | `proxy/management_endpoints/mcp_management_endpoints.py:2624` |
| `GET` | `/server/{server_id}/user-env-vars` | `/server/{server_id}/user-env-vars` | `proxy/management_endpoints/mcp_management_endpoints.py:2560` |
| `POST` | `/server/{server_id}/user-env-vars` | `/server/{server_id}/user-env-vars` | `proxy/management_endpoints/mcp_management_endpoints.py:2582` |
| `GET` | `/tools` | `/tools` | `proxy/management_endpoints/mcp_management_endpoints.py:912` |
| `POST` | `/tools/call` | `/tools/call` | `proxy/_experimental/mcp_server/rest_endpoints.py:1056` |
| `GET` | `/tools/list` | `/tools/list` | `proxy/_experimental/mcp_server/rest_endpoints.py:834` |
| `GET` | `/toolset` | `/toolset` | `proxy/management_endpoints/mcp_management_endpoints.py:3065` |
| `POST` | `/toolset` | `/toolset` | `proxy/management_endpoints/mcp_management_endpoints.py:3025` |
| `PUT` | `/toolset` | `/toolset` | `proxy/management_endpoints/mcp_management_endpoints.py:3115` |
| `DELETE` | `/toolset/{toolset_id}` | `/toolset/{toolset_id}` | `proxy/management_endpoints/mcp_management_endpoints.py:3166` |
| `GET` | `/toolset/{toolset_id}` | `/toolset/{toolset_id}` | `proxy/management_endpoints/mcp_management_endpoints.py:3088` |
| `GET` | `/user-credentials` | `/user-credentials` | `proxy/management_endpoints/mcp_management_endpoints.py:2412` |
| `GET` | `/user-env-vars/status` | `/user-env-vars/status` | `proxy/management_endpoints/mcp_management_endpoints.py:2651` |

## 请求

master key、SSO session，或 `key_type=management` / 具备对应角色的虚拟 Key。按 `user_api_key_auth`。

冻结字段（不得改名）：`server_name`、`url`、`transport`、`auth_type`、`credentials`、`mcp_info`。

### 请求头

| Header | 必填 | 说明 |
|---|---|---|
| `Authorization` | 是 | `Bearer` master key 或 session |
| `Content-Type` | JSON 时是 | `application/json` 或 multipart |

### 请求体

```json
{"server_name": "github", "url": "https://mcp.example/sse", "transport": "sse", "auth_type": "none"}
```

## 响应

冻结响应字段：`server_id`、`server_name`、`url`、`transport`、`status`、`tools`。

### 响应头

| Header | 说明 |
|---|---|
| `x-litellm-call-id` | 见 [错误与响应头](../../../architecture/errors-headers.md) |
| `Content-Type` | 见 [错误与响应头](../../../architecture/errors-headers.md) |
| `Retry-After` | 429 时 |

### 响应体

```json
{"server_id": "mcp_01", "server_name": "github", "status": "active"}
```

## 错误

OpenAI 兼容路径使用 `{"error":{"message","type","code","param"}}`。Anthropic 原生用 Anthropic 错误形状。Gemini 用 Google 错误形状。鉴权失败 401，模型不允许 401/403，预算/RPM/TPM 429 + `Retry-After`，参数 400。未实现 Provider 明确错误。共性见 [错误与响应头](../../../architecture/errors-headers.md)。

## 副作用

管理写操作记 AuditLog 并失效 Auth cache。测试调用（如 `/prompts/test`、`/health/test_connection`）仍记 spend。

## 控制台绑定

Console: /mcp-servers。动作对照 [前端绑定](../../../../frontend/binding.md) 与 [页面规格](../../../../frontend/pages/README.md)。无控制台入口的路径仍须对 SDK/curl 可用。

## 验收

- 本表每一行 method+path 均已注册；未实现时返回 JSON 错误信封，不是 HTML。
- 鉴权失败与成功主路径合同测试。
- 标准客户端只改 base_url + api_key（数据面）。
