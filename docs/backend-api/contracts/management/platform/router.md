# Router admin

- Family: `mgmt.router`
- Status: `specified`
- 参考: 下表定位列
- Console: /router-settings /cost-optimization
- Auth: management

## 目的

本族是网关下列 HTTP 路径的完整分母。字段名与默认值以冻结契约及 该路由 handler 的请求体模型与上游 SDK 透传字段 为准，网关不得改名。

## HTTP 表面

实现必须注册每一行 method+path。规范化列用于覆盖校验（去掉 `:path` 与尾斜杠）。

| Method | Path | 规范化 path | 参考 |
|---|---|---|---|
| `GET` | `/adaptive_router/state` | `/adaptive_router/state` | `proxy/proxy_server.py:18684` |
| `GET` | `/auto_router/benchmarks` | `/auto_router/benchmarks` | `proxy/management_endpoints/auto_router_endpoints.py:647` |
| `GET` | `/auto_router/classifier/default_prompt` | `/auto_router/classifier/default_prompt` | `proxy/management_endpoints/model_management_endpoints.py:2633` |
| `POST` | `/auto_router/classifier/default_prompt` | `/auto_router/classifier/default_prompt` | `proxy/management_endpoints/model_management_endpoints.py:2598` |
| `GET` | `/auto_router/session` | `/auto_router/session` | `proxy/management_endpoints/auto_router_endpoints.py:712` |
| `GET` | `/auto_router/shadow_eval` | `/auto_router/shadow_eval` | `proxy/management_endpoints/auto_router_endpoints.py:1615` |
| `POST` | `/auto_router/shadow_eval/start` | `/auto_router/shadow_eval/start` | `proxy/management_endpoints/auto_router_endpoints.py:1386` |
| `GET` | `/auto_router/shadow_eval/{job_id}` | `/auto_router/shadow_eval/{job_id}` | `proxy/management_endpoints/auto_router_endpoints.py:1665` |
| `POST` | `/auto_router/shadow_eval/{job_id}/stop` | `/auto_router/shadow_eval/{job_id}/stop` | `proxy/management_endpoints/auto_router_endpoints.py:1720` |
| `POST` | `/auto_router/test_routing` | `/auto_router/test_routing` | `proxy/management_endpoints/auto_router_endpoints.py:342` |
| `POST` | `/auto_router/validate_complexity_router_config` | `/auto_router/validate_complexity_router_config` | `proxy/management_endpoints/auto_router_endpoints.py:313` |
| `POST` | `/fallback` | `/fallback` | `proxy/management_endpoints/fallback_management_endpoints.py:41` |
| `DELETE` | `/fallback/{model}` | `/fallback/{model}` | `proxy/management_endpoints/fallback_management_endpoints.py:249` |
| `GET` | `/fallback/{model}` | `/fallback/{model}` | `proxy/management_endpoints/fallback_management_endpoints.py:192` |
| `GET` | `/router/fields` | `/router/fields` | `proxy/management_endpoints/router_settings_endpoints.py:127` |
| `GET` | `/router/settings` | `/router/settings` | `proxy/management_endpoints/router_settings_endpoints.py:61` |
| `GET` | `/routes` | `/routes` | `proxy/proxy_server.py:18719` |

## 请求

master key、SSO session，或 `key_type=management` / 具备对应角色的虚拟 Key。按 `user_api_key_auth`。

冻结字段（不得改名）：`routing_strategy`、`num_retries`、`timeout`、`allowed_fails`、`fallbacks`、`context_window_fallbacks`、`enable_pre_call_checks`。

### 请求头

| Header | 必填 | 说明 |
|---|---|---|
| `Authorization` | 是 | `Bearer` master key 或 session |
| `Content-Type` | JSON 时是 | `application/json` 或 multipart |

### 请求体

```json
{"routing_strategy": "simple-shuffle", "num_retries": 2, "timeout": 60}
```

## 响应

冻结响应字段：`router_settings object`。

### 响应头

| Header | 说明 |
|---|---|
| `x-litellm-call-id` | 见 [错误与响应头](../../../architecture/errors-headers.md) |
| `Content-Type` | 见 [错误与响应头](../../../architecture/errors-headers.md) |
| `Retry-After` | 429 时 |

### 响应体

```json
{"routing_strategy": "simple-shuffle", "num_retries": 2}
```

## 错误

OpenAI 兼容路径使用 `{"error":{"message","type","code","param"}}`。Anthropic 原生用 Anthropic 错误形状。Gemini 用 Google 错误形状。鉴权失败 401，模型不允许 401/403，预算/RPM/TPM 429 + `Retry-After`，参数 400。未实现 Provider 明确错误。共性见 [错误与响应头](../../../architecture/errors-headers.md)。

## 副作用

管理写操作记 AuditLog 并失效 Auth cache。测试调用（如 `/prompts/test`、`/health/test_connection`）仍记 spend。

## 控制台绑定

Console: /router-settings /cost-optimization。动作对照 [前端绑定](../../../../frontend/binding.md) 与 [页面规格](../../../../frontend/pages/README.md)。无控制台入口的路径仍须对 SDK/curl 可用。

## 验收

- 本表每一行 method+path 均已注册；未实现时返回 JSON 错误信封，不是 HTML。
- 鉴权失败与成功主路径合同测试。
- 标准客户端只改 base_url + api_key（数据面）。
