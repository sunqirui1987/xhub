# Access Groups (data plane)

- Family: `data.access_groups`
- Status: `specified`
- 参考: 下表定位列
- Console: /access-groups
- Auth: mixed

## 目的

本族是网关下列 HTTP 路径的完整分母。字段名与默认值以冻结契约及 该路由 handler 的请求体模型与上游 SDK 透传字段 为准，网关不得改名。

## HTTP 表面

实现必须注册每一行 method+path。规范化列用于覆盖校验（去掉 `:path` 与尾斜杠）。

| Method | Path | 规范化 path | 参考 |
|---|---|---|---|
| `GET` | `/access_group` | `/access_group` | `catalog.families:0` |
| `POST` | `/access_group` | `/access_group` | `catalog.families:0` |
| `GET` | `/access_group/list` | `/access_group/list` | `proxy/management_endpoints/model_access_group_management_endpoints.py:720` |
| `POST` | `/access_group/new` | `/access_group/new` | `proxy/management_endpoints/model_access_group_management_endpoints.py:574` |
| `DELETE` | `/access_group/{access_group}/budget` | `/access_group/{access_group}/budget` | `proxy/management_endpoints/model_access_group_management_endpoints.py:1206` |
| `GET` | `/access_group/{access_group}/budget` | `/access_group/{access_group}/budget` | `proxy/management_endpoints/model_access_group_management_endpoints.py:1093` |
| `PUT` | `/access_group/{access_group}/budget` | `/access_group/{access_group}/budget` | `proxy/management_endpoints/model_access_group_management_endpoints.py:1128` |
| `DELETE` | `/access_group/{access_group}/delete` | `/access_group/{access_group}/delete` | `proxy/management_endpoints/model_access_group_management_endpoints.py:983` |
| `GET` | `/access_group/{access_group}/info` | `/access_group/{access_group}/info` | `proxy/management_endpoints/model_access_group_management_endpoints.py:770` |
| `PUT` | `/access_group/{access_group}/update` | `/access_group/{access_group}/update` | `proxy/management_endpoints/model_access_group_management_endpoints.py:829` |
| `GET` | `/access_groups` | `/access_groups` | `proxy/management_endpoints/mcp_management_endpoints.py:936` |
| `GET` | `/v1/access_group` | `/v1/access_group` | `proxy/management_endpoints/access_group_endpoints.py:511` |
| `POST` | `/v1/access_group` | `/v1/access_group` | `proxy/management_endpoints/access_group_endpoints.py:439` |
| `DELETE` | `/v1/access_group/{access_group_id}` | `/v1/access_group/{access_group_id}` | `proxy/management_endpoints/access_group_endpoints.py:644` |
| `GET` | `/v1/access_group/{access_group_id}` | `/v1/access_group/{access_group_id}` | `proxy/management_endpoints/access_group_endpoints.py:526` |
| `PUT` | `/v1/access_group/{access_group_id}` | `/v1/access_group/{access_group_id}` | `proxy/management_endpoints/access_group_endpoints.py:547` |

## 请求

数据面虚拟 Key，管理面 session/master key。OAuth 回调走浏览器 session。见 [鉴权决策](../../../architecture/auth-decision.md)。

冻结字段（不得改名）：`access_group_id`、`models`、`budget_id`。

### 请求头

| Header | 必填 | 说明 |
|---|---|---|
| `Authorization` | 是 | `Bearer` master key 或 session |
| `Content-Type` | JSON 时是 | `application/json` 或 multipart |

### 请求体

```json
{"access_group_id": "prod-chat", "models": ["gpt-4o-mini"]}
```

## 响应

冻结响应字段：`access_group_id`、`models`、`budget_id`、`created_at`。

### 响应头

| Header | 说明 |
|---|---|
| `x-litellm-call-id` | 见 [错误与响应头](../../../architecture/errors-headers.md) |
| `Content-Type` | 见 [错误与响应头](../../../architecture/errors-headers.md) |
| `Retry-After` | 429 时 |

### 响应体

```json
{"access_group_id": "prod-chat", "models": ["gpt-4o-mini"]}
```

## 错误

OpenAI 兼容路径使用 `{"error":{"message","type","code","param"}}`。Anthropic 原生用 Anthropic 错误形状。Gemini 用 Google 错误形状。鉴权失败 401，模型不允许 401/403，预算/RPM/TPM 429 + `Retry-After`，参数 400。未实现 Provider 明确错误。共性见 [错误与响应头](../../../architecture/errors-headers.md)。

## 副作用

管理写操作记 AuditLog 并失效 Auth cache。测试调用（如 `/prompts/test`、`/health/test_connection`）仍记 spend。

## 控制台绑定

Console: /access-groups。动作对照 [前端绑定](../../../../frontend/binding.md) 与 [页面规格](../../../../frontend/pages/README.md)。无控制台入口的路径仍须对 SDK/curl 可用。

## 验收

- 本表每一行 method+path 均已注册；未实现时返回 JSON 错误信封，不是 HTML。
- 鉴权失败与成功主路径合同测试。
- 标准客户端只改 base_url + api_key（数据面）。
