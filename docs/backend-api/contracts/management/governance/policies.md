# Policies

- Family: `mgmt.policies`
- Status: `specified`
- 参考: 下表定位列
- Console: /policies /tool-policies
- Auth: management

## 目的

本族是网关下列 HTTP 路径的完整分母。字段名与默认值以冻结契约及 该路由 handler 的请求体模型与上游 SDK 透传字段 为准，网关不得改名。

## HTTP 表面

实现必须注册每一行 method+path。规范化列用于覆盖校验（去掉 `:path` 与尾斜杠）。

| Method | Path | 规范化 path | 参考 |
|---|---|---|---|
| `POST` | `/policies` | `/policies` | `proxy/policy_engine/policy_endpoints.py:151` |
| `POST` | `/policies/attachments` | `/policies/attachments` | `proxy/policy_engine/policy_endpoints.py:708` |
| `POST` | `/policies/attachments/estimate-impact` | `/policies/attachments/estimate-impact` | `proxy/policy_engine/policy_resolve_endpoints.py:321` |
| `GET` | `/policies/attachments/list` | `/policies/attachments/list` | `proxy/policy_engine/policy_endpoints.py:654` |
| `DELETE` | `/policies/attachments/{attachment_id}` | `/policies/attachments/{attachment_id}` | `proxy/policy_engine/policy_endpoints.py:839` |
| `GET` | `/policies/attachments/{attachment_id}` | `/policies/attachments/{attachment_id}` | `proxy/policy_engine/policy_endpoints.py:800` |
| `GET` | `/policies/compare` | `/policies/compare` | `proxy/policy_engine/policy_endpoints.py:321` |
| `GET` | `/policies/list` | `/policies/list` | `proxy/policy_engine/policy_endpoints.py:72` |
| `DELETE` | `/policies/name/{policy_name}/all-versions` | `/policies/name/{policy_name}/all-versions` | `proxy/policy_engine/policy_endpoints.py:352` |
| `GET` | `/policies/name/{policy_name}/versions` | `/policies/name/{policy_name}/versions` | `proxy/policy_engine/policy_endpoints.py:220` |
| `POST` | `/policies/name/{policy_name}/versions` | `/policies/name/{policy_name}/versions` | `proxy/policy_engine/policy_endpoints.py:245` |
| `POST` | `/policies/resolve` | `/policies/resolve` | `proxy/policy_engine/policy_resolve_endpoints.py:215` |
| `POST` | `/policies/test-pipeline` | `/policies/test-pipeline` | `proxy/policy_engine/policy_endpoints.py:591` |
| `GET` | `/policies/usage/overview` | `/policies/usage/overview` | `proxy/guardrails/usage_endpoints.py:934` |
| `DELETE` | `/policies/{policy_id}` | `/policies/{policy_id}` | `proxy/policy_engine/policy_endpoints.py:476` |
| `GET` | `/policies/{policy_id}` | `/policies/{policy_id}` | `proxy/policy_engine/policy_endpoints.py:381` |
| `PUT` | `/policies/{policy_id}` | `/policies/{policy_id}` | `proxy/policy_engine/policy_endpoints.py:417` |
| `GET` | `/policies/{policy_id}/resolved-guardrails` | `/policies/{policy_id}/resolved-guardrails` | `proxy/policy_engine/policy_endpoints.py:525` |
| `PUT` | `/policies/{policy_id}/status` | `/policies/{policy_id}/status` | `proxy/policy_engine/policy_endpoints.py:280` |
| `GET` | `/policy/info/{policy_name}` | `/policy/info/{policy_name}` | `proxy/management_endpoints/policy_endpoints/endpoints.py:491` |
| `GET` | `/policy/list` | `/policy/list` | `proxy/management_endpoints/policy_endpoints/endpoints.py:452` |
| `GET` | `/policy/templates` | `/policy/templates` | `proxy/management_endpoints/policy_endpoints/endpoints.py:626` |
| `POST` | `/policy/templates/enrich` | `/policy/templates/enrich` | `proxy/management_endpoints/policy_endpoints/endpoints.py:714` |
| `POST` | `/policy/templates/enrich/stream` | `/policy/templates/enrich/stream` | `proxy/management_endpoints/policy_endpoints/endpoints.py:870` |
| `POST` | `/policy/templates/suggest` | `/policy/templates/suggest` | `proxy/management_endpoints/policy_endpoints/endpoints.py:1093` |
| `POST` | `/policy/templates/test` | `/policy/templates/test` | `proxy/management_endpoints/policy_endpoints/endpoints.py:1140` |
| `POST` | `/policy/test` | `/policy/test` | `proxy/management_endpoints/policy_endpoints/endpoints.py:548` |
| `POST` | `/policy/validate` | `/policy/validate` | `proxy/management_endpoints/policy_endpoints/endpoints.py:383` |

## 请求

master key、SSO session，或 `key_type=management` / 具备对应角色的虚拟 Key。按 `user_api_key_auth`。

冻结字段（不得改名）：`policy_name`、`description`、`statements`、`status`。

### 请求头

| Header | 必填 | 说明 |
|---|---|---|
| `Authorization` | 是 | `Bearer` master key 或 session |
| `Content-Type` | JSON 时是 | `application/json` 或 multipart |

### 请求体

```json
{"policy_name": "prod-block-pii", "status": "active"}
```

## 响应

冻结响应字段：`policy_id`、`policy_name`、`version`、`status`、`created_at`。

### 响应头

| Header | 说明 |
|---|---|
| `x-litellm-call-id` | 见 [错误与响应头](../../../architecture/errors-headers.md) |
| `Content-Type` | 见 [错误与响应头](../../../architecture/errors-headers.md) |
| `Retry-After` | 429 时 |

### 响应体

```json
{"policy_id": "pol_01", "policy_name": "prod-block-pii", "version": 1, "status": "active"}
```

## 错误

OpenAI 兼容路径使用 `{"error":{"message","type","code","param"}}`。Anthropic 原生用 Anthropic 错误形状。Gemini 用 Google 错误形状。鉴权失败 401，模型不允许 401/403，预算/RPM/TPM 429 + `Retry-After`，参数 400。未实现 Provider 明确错误。共性见 [错误与响应头](../../../architecture/errors-headers.md)。

## 副作用

管理写操作记 AuditLog 并失效 Auth cache。测试调用（如 `/prompts/test`、`/health/test_connection`）仍记 spend。

## 控制台绑定

Console: /policies /tool-policies。动作对照 [前端绑定](../../../../frontend/binding.md) 与 [页面规格](../../../../frontend/pages/README.md)。无控制台入口的路径仍须对 SDK/curl 可用。

## 验收

- 本表每一行 method+path 均已注册；未实现时返回 JSON 错误信封，不是 HTML。
- 鉴权失败与成功主路径合同测试。
- 标准客户端只改 base_url + api_key（数据面）。
