# Responses API

- Family: `data.responses`
- Status: `specified`
- 参考: 下表定位列
- Console: /playground
- Auth: virtual-key

## 目的

本族是网关下列 HTTP 路径的完整分母。字段名与默认值以冻结契约及 该路由 handler 的请求体模型与上游 SDK 透传字段 为准，网关不得改名。

## HTTP 表面

实现必须注册每一行 method+path。规范化列用于覆盖校验（去掉 `:path` 与尾斜杠）。

| Method | Path | 规范化 path | 参考 |
|---|---|---|---|
| `POST` | `/openai/v1/responses` | `/openai/v1/responses` | `proxy/response_api_endpoints/endpoints.py:189` |
| `POST` | `/openai/v1/responses/compact` | `/openai/v1/responses/compact` | `proxy/response_api_endpoints/endpoints.py:957` |
| `POST` | `/openai/v1/responses/input_tokens` | `/openai/v1/responses/input_tokens` | `proxy/response_api_endpoints/endpoints.py:1112` |
| `DELETE` | `/openai/v1/responses/{response_id}` | `/openai/v1/responses/{response_id}` | `proxy/response_api_endpoints/endpoints.py:783` |
| `GET` | `/openai/v1/responses/{response_id}` | `/openai/v1/responses/{response_id}` | `proxy/response_api_endpoints/endpoints.py:670` |
| `POST` | `/openai/v1/responses/{response_id}/cancel` | `/openai/v1/responses/{response_id}/cancel` | `proxy/response_api_endpoints/endpoints.py:1185` |
| `GET` | `/openai/v1/responses/{response_id}/input_items` | `/openai/v1/responses/{response_id}/input_items` | `proxy/response_api_endpoints/endpoints.py:889` |
| `POST` | `/responses` | `/responses` | `proxy/response_api_endpoints/endpoints.py:184` |
| `WEBSOCKET` | `/responses` | `/responses` | `proxy/response_api_endpoints/endpoints.py:1416` |
| `POST` | `/responses/compact` | `/responses/compact` | `proxy/response_api_endpoints/endpoints.py:952` |
| `POST` | `/responses/input_tokens` | `/responses/input_tokens` | `proxy/response_api_endpoints/endpoints.py:1107` |
| `DELETE` | `/responses/{response_id}` | `/responses/{response_id}` | `proxy/response_api_endpoints/endpoints.py:778` |
| `GET` | `/responses/{response_id}` | `/responses/{response_id}` | `proxy/response_api_endpoints/endpoints.py:665` |
| `POST` | `/responses/{response_id}/cancel` | `/responses/{response_id}/cancel` | `proxy/response_api_endpoints/endpoints.py:1180` |
| `GET` | `/responses/{response_id}/input_items` | `/responses/{response_id}/input_items` | `proxy/response_api_endpoints/endpoints.py:884` |
| `POST` | `/v1/responses` | `/v1/responses` | `proxy/response_api_endpoints/endpoints.py:179` |
| `WEBSOCKET` | `/v1/responses` | `/v1/responses` | `proxy/response_api_endpoints/endpoints.py:1415` |
| `POST` | `/v1/responses/compact` | `/v1/responses/compact` | `proxy/response_api_endpoints/endpoints.py:947` |
| `POST` | `/v1/responses/input_tokens` | `/v1/responses/input_tokens` | `proxy/response_api_endpoints/endpoints.py:1102` |
| `DELETE` | `/v1/responses/{response_id}` | `/v1/responses/{response_id}` | `proxy/response_api_endpoints/endpoints.py:773` |
| `GET` | `/v1/responses/{response_id}` | `/v1/responses/{response_id}` | `proxy/response_api_endpoints/endpoints.py:660` |
| `POST` | `/v1/responses/{response_id}/cancel` | `/v1/responses/{response_id}/cancel` | `proxy/response_api_endpoints/endpoints.py:1175` |
| `GET` | `/v1/responses/{response_id}/input_items` | `/v1/responses/{response_id}/input_items` | `proxy/response_api_endpoints/endpoints.py:879` |

## 请求

`Authorization: Bearer sk-...` 或 `x-litellm-api-key`。`key_type` 为 `llm_api` 或 `default`。master key 默认不能调本族，除非 `general_settings` 显式允许。

冻结字段（不得改名）：`model`、`input`、`instructions`、`tools`、`tool_choice`、`previous_response_id`、`store`、`stream`、`max_output_tokens`、`metadata`。

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
{"model": "gpt-4o-mini", "input": "Hello", "store": true, "stream": false}
```

## 响应

冻结响应字段：`id`、`object`、`status`、`output`、`usage`、`model`、`created_at`。

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
{"id": "resp_01", "object": "response", "status": "completed", "model": "gpt-4o-mini", "output": [], "usage": {"input_tokens": 8, "output_tokens": 2}}
```

## 错误

OpenAI 兼容路径使用 `{"error":{"message","type","code","param"}}`。Anthropic 原生用 Anthropic 错误形状。Gemini 用 Google 错误形状。鉴权失败 401，模型不允许 401/403，预算/RPM/TPM 429 + `Retry-After`，参数 400。未实现 Provider 明确错误。共性见 [错误与响应头](../../../architecture/errors-headers.md)。

## 副作用

数据面调用记 spend、占 RPM/TPM/并发、写 SpendLogs，可走 Guardrail 与响应缓存。见 [生命周期](../../../architecture/request-lifecycle.md) 与 [计量](../../../architecture/spend-limits.md)。

## 控制台绑定

Console: /playground。动作对照 [前端绑定](../../../../frontend/binding.md) 与 [页面规格](../../../../frontend/pages/README.md)。无控制台入口的路径仍须对 SDK/curl 可用。

## 验收

- 本表每一行 method+path 均已注册；未实现时返回 JSON 错误信封，不是 HTML。
- 鉴权失败与成功主路径合同测试。
- 标准客户端只改 base_url + api_key（数据面）。
