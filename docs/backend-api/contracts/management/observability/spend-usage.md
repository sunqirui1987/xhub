# Spend, usage, logs

- Family: `mgmt.spend`
- Status: `specified`
- 参考: 下表定位列
- Console: /usage /logs /old-usage /chat/usage /chat/logs
- Auth: management

## 目的

本族是网关下列 HTTP 路径的完整分母。字段名与默认值以冻结契约及 该路由 handler 的请求体模型与上游 SDK 透传字段 为准，网关不得改名。

## HTTP 表面

实现必须注册每一行 method+path。规范化列用于覆盖校验（去掉 `:path` 与尾斜杠）。

| Method | Path | 规范化 path | 参考 |
|---|---|---|---|
| `POST` | `/cost/estimate` | `/cost/estimate` | `proxy/management_endpoints/cost_tracking_settings.py:569` |
| `POST` | `/cost/predict-cache` | `/cost/predict-cache` | `proxy/management_endpoints/prompt_cache_prediction.py:175` |
| `GET` | `/gateway/daily/activity` | `/gateway/daily/activity` | `proxy/management_endpoints/gateway_request_endpoints.py:94` |
| `GET` | `/global/activity` | `/global/activity` | `proxy/spend_tracking/spend_management_endpoints.py:623` |
| `GET` | `/global/activity/cache_hits` | `/global/activity/cache_hits` | `proxy/analytics_endpoints/analytics_endpoints.py:25` |
| `GET` | `/global/activity/exceptions` | `/global/activity/exceptions` | `proxy/spend_tracking/spend_management_endpoints.py:1064` |
| `GET` | `/global/activity/exceptions/deployment` | `/global/activity/exceptions/deployment` | `proxy/spend_tracking/spend_management_endpoints.py:917` |
| `GET` | `/global/activity/model` | `/global/activity/model` | `proxy/spend_tracking/spend_management_endpoints.py:765` |
| `GET` | `/global/all_end_users` | `/global/all_end_users` | `proxy/spend_tracking/spend_management_endpoints.py:4003` |
| `GET` | `/global/spend` | `/global/spend` | `proxy/spend_tracking/spend_management_endpoints.py:3775` |
| `GET` | `/global/spend/all_tag_names` | `/global/spend/all_tag_names` | `proxy/spend_tracking/spend_management_endpoints.py:1969` |
| `POST` | `/global/spend/end_users` | `/global/spend/end_users` | `proxy/spend_tracking/spend_management_endpoints.py:4035` |
| `GET` | `/global/spend/keys` | `/global/spend/keys` | `proxy/spend_tracking/spend_management_endpoints.py:3867` |
| `GET` | `/global/spend/logs` | `/global/spend/logs` | `proxy/spend_tracking/spend_management_endpoints.py:3685` |
| `GET` | `/global/spend/models` | `/global/spend/models` | `proxy/spend_tracking/spend_management_endpoints.py:4119` |
| `GET` | `/global/spend/provider` | `/global/spend/provider` | `proxy/spend_tracking/spend_management_endpoints.py:1172` |
| `POST` | `/global/spend/refresh` | `/global/spend/refresh` | `proxy/spend_tracking/spend_management_endpoints.py:3568` |
| `GET` | `/global/spend/report` | `/global/spend/report` | `proxy/spend_tracking/spend_management_endpoints.py:1301` |
| `POST` | `/global/spend/reset` | `/global/spend/reset` | `proxy/spend_tracking/spend_management_endpoints.py:3533` |
| `GET` | `/global/spend/tags` | `/global/spend/tags` | `proxy/spend_tracking/spend_management_endpoints.py:2021` |
| `GET` | `/global/spend/teams` | `/global/spend/teams` | `proxy/spend_tracking/spend_management_endpoints.py:3914` |
| `POST` | `/spend/calculate` | `/spend/calculate` | `proxy/spend_tracking/spend_management_endpoints.py:2165` |
| `GET` | `/spend/keys` | `/spend/keys` | `proxy/spend_tracking/spend_management_endpoints.py:384` |
| `GET` | `/spend/logs` | `/spend/logs` | `proxy/spend_tracking/spend_management_endpoints.py:3321` |
| `GET` | `/spend/logs/session/ui` | `/spend/logs/session/ui` | `proxy/spend_tracking/spend_management_endpoints.py:4350` |
| `GET` | `/spend/logs/ui` | `/spend/logs/ui` | `proxy/spend_tracking/spend_management_endpoints.py:2354` |
| `GET` | `/spend/logs/ui/{request_id}` | `/spend/logs/ui/{request_id}` | `proxy/spend_tracking/spend_management_endpoints.py:3228` |
| `GET` | `/spend/logs/v2` | `/spend/logs/v2` | `proxy/spend_tracking/spend_management_endpoints.py:2346` |
| `GET` | `/spend/tags` | `/spend/tags` | `proxy/spend_tracking/spend_management_endpoints.py:521` |
| `GET` | `/spend/users` | `/spend/users` | `proxy/spend_tracking/spend_management_endpoints.py:446` |
| `GET` | `/spend_logs/end_users` | `/spend_logs/end_users` | `proxy/management_endpoints/management_v1/spend_logs.py:172` |
| `GET` | `/spend_logs/users` | `/spend_logs/users` | `proxy/management_endpoints/management_v1/spend_logs.py:223` |
| `POST` | `/usage/ai/chat` | `/usage/ai/chat` | `proxy/management_endpoints/usage_endpoints/endpoints.py:34` |

## 请求

master key、SSO session，或 `key_type=management` / 具备对应角色的虚拟 Key。按 `user_api_key_auth`。

冻结字段（不得改名）：`query: start_date`、`end_date`、`api_key`、`user_id`、`team_id`、`request_id`、`page`、`page_size`。

### 请求头

| Header | 必填 | 说明 |
|---|---|---|
| `Authorization` | 是 | `Bearer` master key 或 session |
| `Content-Type` | JSON 时是 | `application/json` 或 multipart |

### 请求体

```json
{"start_date": "2026-01-01", "end_date": "2026-01-31"}
```

## 响应

冻结响应字段：`request_id`、`model`、`spend`、`prompt_tokens`、`completion_tokens`、`startTime`、`api_key (hashed)`。

### 响应头

| Header | 说明 |
|---|---|
| `x-litellm-call-id` | 见 [错误与响应头](../../../architecture/errors-headers.md) |
| `Content-Type` | 见 [错误与响应头](../../../architecture/errors-headers.md) |
| `Retry-After` | 429 时 |

### 响应体

```json
{"data": [{"request_id": "req_01", "model": "gpt-4o-mini", "spend": 0.0001, "prompt_tokens": 8, "completion_tokens": 2}]}
```

## 错误

OpenAI 兼容路径使用 `{"error":{"message","type","code","param"}}`。Anthropic 原生用 Anthropic 错误形状。Gemini 用 Google 错误形状。鉴权失败 401，模型不允许 401/403，预算/RPM/TPM 429 + `Retry-After`，参数 400。未实现 Provider 明确错误。共性见 [错误与响应头](../../../architecture/errors-headers.md)。

## 副作用

管理写操作记 AuditLog 并失效 Auth cache。测试调用（如 `/prompts/test`、`/health/test_connection`）仍记 spend。

## 控制台绑定

Console: /usage /logs /old-usage /chat/usage /chat/logs。动作对照 [前端绑定](../../../../frontend/binding.md) 与 [页面规格](../../../../frontend/pages/README.md)。无控制台入口的路径仍须对 SDK/curl 可用。

## 验收

- 本表每一行 method+path 均已注册；未实现时返回 JSON 错误信封，不是 HTML。
- 鉴权失败与成功主路径合同测试。
- 标准客户端只改 base_url + api_key（数据面）。
