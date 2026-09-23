# Virtual Keys

- Family: `mgmt.keys`
- Status: `specified`
- 参考: 下表定位列
- Console: /api-keys /chat/api-keys
- Auth: management

## 目的

本族是网关下列 HTTP 路径的完整分母。字段名与默认值以冻结契约及 `GenerateKeyRequest` / `UpdateKeyRequest` 字段集 为准，网关不得改名。

## HTTP 表面

实现必须注册每一行 method+path。规范化列用于覆盖校验（去掉 `:path` 与尾斜杠）。

| Method | Path | 规范化 path | 参考 |
|---|---|---|---|
| `GET` | `/key/aliases` | `/key/aliases` | `proxy/management_endpoints/key_management_endpoints.py:6185` |
| `POST` | `/key/block` | `/key/block` | `proxy/management_endpoints/key_management_endpoints.py:6738` |
| `POST` | `/key/bulk_update` | `/key/bulk_update` | `proxy/management_endpoints/key_management_endpoints.py:3201` |
| `POST` | `/key/delete` | `/key/delete` | `proxy/management_endpoints/key_management_endpoints.py:3653` |
| `POST` | `/key/generate` | `/key/generate` | `proxy/management_endpoints/key_management_endpoints.py:1728` |
| `POST` | `/key/health` | `/key/health` | `proxy/management_endpoints/key_management_endpoints.py:6966` |
| `GET` | `/key/info` | `/key/info` | `proxy/management_endpoints/key_management_endpoints.py:3930` |
| `GET` | `/key/list` | `/key/list` | `proxy/management_endpoints/key_management_endpoints.py:5954` |
| `POST` | `/key/regenerate` | `/key/regenerate` | `proxy/management_endpoints/key_management_endpoints.py:5202` |
| `POST` | `/key/service-account/generate` | `/key/service-account/generate` | `proxy/management_endpoints/key_management_endpoints.py:1939` |
| `GET` | `/key/spend/report` | `/key/spend/report` | `proxy/spend_tracking/spend_management_endpoints.py:1795` |
| `POST` | `/key/unblock` | `/key/unblock` | `proxy/management_endpoints/key_management_endpoints.py:6852` |
| `POST` | `/key/update` | `/key/update` | `proxy/management_endpoints/key_management_endpoints.py:2950` |
| `POST` | `/key/{key:path}/regenerate` | `/key/{key}/regenerate` | `proxy/management_endpoints/key_management_endpoints.py:5197` |
| `POST` | `/key/{key:path}/reset_spend` | `/key/{key}/reset_spend` | `proxy/management_endpoints/key_management_endpoints.py:5659` |
| `POST` | `/v2/key/info` | `/v2/key/info` | `proxy/management_endpoints/key_management_endpoints.py:3832` |

## 请求

master key、SSO session，或 `key_type=management` / 具备对应角色的虚拟 Key。按 `user_api_key_auth`。

冻结字段（不得改名）：`key_alias`、`duration`、`models`、`max_budget`、`soft_budget`、`user_id`、`team_id`、`organization_id`、`project_id`、`agent_id`、`tpm_limit`、`rpm_limit`、`max_parallel_requests`、`budget_duration`、`metadata`、`permissions`、`guardrails`、`policies`、`object_permission`、`allowed_routes`、`key_type`、`auto_rotate`、`rotation_interval`、`tags`、`model_rpm_limit`、`model_tpm_limit`、`budget_id`、`blocked`、`send_invite_email`。

### 请求头

| Header | 必填 | 说明 |
|---|---|---|
| `Authorization` | 是 | `Bearer` master key 或 session |
| `Content-Type` | JSON 时是 | `application/json` 或 multipart |

### 请求体

```json
{"key_alias": "checkout-prod", "models": ["gpt-4o-mini"], "max_budget": 10.0, "tpm_limit": 100000, "rpm_limit": 60, "duration": "30d", "key_type": "llm_api"}
```

## 响应

冻结响应字段：`key`、`key_name`、`key_alias`、`expires`、`token_id`、`user_id`、`team_id`、`models`、`max_budget`、`spend`。

### 响应头

| Header | 说明 |
|---|---|
| `x-litellm-call-id` | 见 [错误与响应头](../../../architecture/errors-headers.md) |
| `Content-Type` | 见 [错误与响应头](../../../architecture/errors-headers.md) |
| `Retry-After` | 429 时 |

### 响应体

```json
{"key": "sk-live-...", "key_name": "sk-...xxxx", "key_alias": "checkout-prod", "expires": null, "token_id": "hash", "spend": 0}
```

## 错误

OpenAI 兼容路径使用 `{"error":{"message","type","code","param"}}`。Anthropic 原生用 Anthropic 错误形状。Gemini 用 Google 错误形状。鉴权失败 401，模型不允许 401/403，预算/RPM/TPM 429 + `Retry-After`，参数 400。未实现 Provider 明确错误。共性见 [错误与响应头](../../../architecture/errors-headers.md)。

## 副作用

管理写操作记 AuditLog 并失效 Auth cache。测试调用（如 `/prompts/test`、`/health/test_connection`）仍记 spend。

## 控制台绑定

Console: /api-keys /chat/api-keys。动作对照 [前端绑定](../../../../frontend/binding.md) 与 [页面规格](../../../../frontend/pages/README.md)。无控制台入口的路径仍须对 SDK/curl 可用。

## 验收

- 本表每一行 method+path 均已注册；未实现时返回 JSON 错误信封，不是 HTML。
- 鉴权失败与成功主路径合同测试。
- 标准客户端只改 base_url + api_key（数据面）。
