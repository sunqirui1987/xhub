# Guardrails admin

- Family: `mgmt.guardrails`
- Status: `specified`
- 参考: 下表定位列
- Console: /guardrails /guardrails-monitor
- Auth: management

## 目的

本族是网关下列 HTTP 路径的完整分母。字段名与默认值以冻结契约及 该路由 handler 的请求体模型与上游 SDK 透传字段 为准，网关不得改名。

## HTTP 表面

实现必须注册每一行 method+path。规范化列用于覆盖校验（去掉 `:path` 与尾斜杠）。

| Method | Path | 规范化 path | 参考 |
|---|---|---|---|
| `POST` | `/apply_guardrail` | `/apply_guardrail` | `proxy/guardrails/guardrail_endpoints.py:2315` |
| `POST` | `/guardrails` | `/guardrails` | `proxy/guardrails/guardrail_endpoints.py:325` |
| `POST` | `/guardrails/apply_guardrail` | `/guardrails/apply_guardrail` | `proxy/guardrails/guardrail_endpoints.py:2314` |
| `GET` | `/guardrails/list` | `/guardrails/list` | `proxy/guardrails/guardrail_endpoints.py:126` |
| `POST` | `/guardrails/register` | `/guardrails/register` | `proxy/guardrails/guardrail_endpoints.py:684` |
| `GET` | `/guardrails/submissions` | `/guardrails/submissions` | `proxy/guardrails/guardrail_endpoints.py:860` |
| `GET` | `/guardrails/submissions/{guardrail_id}` | `/guardrails/submissions/{guardrail_id}` | `proxy/guardrails/guardrail_endpoints.py:956` |
| `POST` | `/guardrails/submissions/{guardrail_id}/approve` | `/guardrails/submissions/{guardrail_id}/approve` | `proxy/guardrails/guardrail_endpoints.py:992` |
| `POST` | `/guardrails/submissions/{guardrail_id}/reject` | `/guardrails/submissions/{guardrail_id}/reject` | `proxy/guardrails/guardrail_endpoints.py:1073` |
| `POST` | `/guardrails/test_custom_code` | `/guardrails/test_custom_code` | `proxy/guardrails/guardrail_endpoints.py:2030` |
| `GET` | `/guardrails/ui/add_guardrail_settings` | `/guardrails/ui/add_guardrail_settings` | `proxy/guardrails/guardrail_endpoints.py:1379` |
| `GET` | `/guardrails/ui/category_yaml/{category_name}` | `/guardrails/ui/category_yaml/{category_name}` | `proxy/guardrails/guardrail_endpoints.py:1434` |
| `GET` | `/guardrails/ui/major_airlines` | `/guardrails/ui/major_airlines` | `proxy/guardrails/guardrail_endpoints.py:1493` |
| `GET` | `/guardrails/ui/provider_specific_params` | `/guardrails/ui/provider_specific_params` | `proxy/guardrails/guardrail_endpoints.py:1905` |
| `GET` | `/guardrails/usage/detail/{guardrail_id}` | `/guardrails/usage/detail/{guardrail_id}` | `proxy/guardrails/usage_endpoints.py:621` |
| `GET` | `/guardrails/usage/logs` | `/guardrails/usage/logs` | `proxy/guardrails/usage_endpoints.py:859` |
| `GET` | `/guardrails/usage/overview` | `/guardrails/usage/overview` | `proxy/guardrails/usage_endpoints.py:548` |
| `POST` | `/guardrails/validate_blocked_words_file` | `/guardrails/validate_blocked_words_file` | `proxy/guardrails/guardrail_endpoints.py:1525` |
| `DELETE` | `/guardrails/{guardrail_id}` | `/guardrails/{guardrail_id}` | `proxy/guardrails/guardrail_endpoints.py:555` |
| `GET` | `/guardrails/{guardrail_id}` | `/guardrails/{guardrail_id}` | `proxy/guardrails/guardrail_endpoints.py:1284` |
| `PATCH` | `/guardrails/{guardrail_id}` | `/guardrails/{guardrail_id}` | `proxy/guardrails/guardrail_endpoints.py:1117` |
| `PUT` | `/guardrails/{guardrail_id}` | `/guardrails/{guardrail_id}` | `proxy/guardrails/guardrail_endpoints.py:432` |
| `GET` | `/guardrails/{guardrail_id}/info` | `/guardrails/{guardrail_id}/info` | `proxy/guardrails/guardrail_endpoints.py:1289` |
| `GET` | `/v2/guardrails/list` | `/v2/guardrails/list` | `proxy/guardrails/guardrail_endpoints.py:179` |

## 请求

master key、SSO session，或 `key_type=management` / 具备对应角色的虚拟 Key。按 `user_api_key_auth`。

冻结字段（不得改名）：`guardrail_name`、`litellm_params.guardrail`、`mode`、`default_on`、`guardrail_info`。

### 请求头

| Header | 必填 | 说明 |
|---|---|---|
| `Authorization` | 是 | `Bearer` master key 或 session |
| `Content-Type` | JSON 时是 | `application/json` 或 multipart |

### 请求体

```json
{"guardrail_name": "pii", "litellm_params": {"guardrail": "presidio", "mode": "pre_call", "default_on": true}}
```

## 响应

冻结响应字段：`guardrail_id`、`guardrail_name`、`litellm_params`、`created_at`。

### 响应头

| Header | 说明 |
|---|---|
| `x-litellm-call-id` | 见 [错误与响应头](../../../architecture/errors-headers.md) |
| `Content-Type` | 见 [错误与响应头](../../../architecture/errors-headers.md) |
| `Retry-After` | 429 时 |

### 响应体

```json
{"guardrail_id": "gr_01", "guardrail_name": "pii"}
```

## 错误

OpenAI 兼容路径使用 `{"error":{"message","type","code","param"}}`。Anthropic 原生用 Anthropic 错误形状。Gemini 用 Google 错误形状。鉴权失败 401，模型不允许 401/403，预算/RPM/TPM 429 + `Retry-After`，参数 400。未实现 Provider 明确错误。共性见 [错误与响应头](../../../architecture/errors-headers.md)。

## 副作用

管理写操作记 AuditLog 并失效 Auth cache。测试调用（如 `/prompts/test`、`/health/test_connection`）仍记 spend。

## 控制台绑定

Console: /guardrails /guardrails-monitor。动作对照 [前端绑定](../../../../frontend/binding.md) 与 [页面规格](../../../../frontend/pages/README.md)。无控制台入口的路径仍须对 SDK/curl 可用。

## 验收

- 本表每一行 method+path 均已注册；未实现时返回 JSON 错误信封，不是 HTML。
- 鉴权失败与成功主路径合同测试。
- 标准客户端只改 base_url + api_key（数据面）。
