# Realtime

- Family: `data.realtime`
- Status: `specified`
- 参考: 下表定位列
- Console: —
- Auth: virtual-key

## 目的

本族是网关下列 HTTP 路径的完整分母。字段名与默认值以冻结契约及 该路由 handler 的请求体模型与上游 SDK 透传字段 为准，网关不得改名。

## HTTP 表面

实现必须注册每一行 method+path。规范化列用于覆盖校验（去掉 `:path` 与尾斜杠）。

| Method | Path | 规范化 path | 参考 |
|---|---|---|---|
| `WEBSOCKET` | `/openai/v1/realtime` | `/openai/v1/realtime` | `proxy/proxy_server.py:11933` |
| `POST` | `/openai/v1/realtime/calls` | `/openai/v1/realtime/calls` | `proxy/realtime_endpoints/endpoints.py:370` |
| `POST` | `/openai/v1/realtime/client_secrets` | `/openai/v1/realtime/client_secrets` | `proxy/realtime_endpoints/endpoints.py:226` |
| `POST` | `/openai/v1/realtime/transcription_sessions` | `/openai/v1/realtime/transcription_sessions` | `proxy/realtime_endpoints/endpoints.py:531` |
| `WEBSOCKET` | `/realtime` | `/realtime` | `proxy/proxy_server.py:11935` |
| `POST` | `/realtime/calls` | `/realtime/calls` | `proxy/realtime_endpoints/endpoints.py:366` |
| `POST` | `/realtime/client_secrets` | `/realtime/client_secrets` | `proxy/realtime_endpoints/endpoints.py:221` |
| `POST` | `/realtime/transcription_sessions` | `/realtime/transcription_sessions` | `proxy/realtime_endpoints/endpoints.py:526` |
| `WEBSOCKET` | `/v1/realtime` | `/v1/realtime` | `proxy/proxy_server.py:11934` |
| `POST` | `/v1/realtime/calls` | `/v1/realtime/calls` | `proxy/realtime_endpoints/endpoints.py:362` |
| `POST` | `/v1/realtime/client_secrets` | `/v1/realtime/client_secrets` | `proxy/realtime_endpoints/endpoints.py:216` |
| `POST` | `/v1/realtime/transcription_sessions` | `/v1/realtime/transcription_sessions` | `proxy/realtime_endpoints/endpoints.py:521` |
| `WEBSOCKET` | `/vertex_ai/live` | `/vertex_ai/live` | `proxy/proxy_server.py:11838` |

## 请求

`Authorization: Bearer sk-...` 或 `x-litellm-api-key`。`key_type` 为 `llm_api` 或 `default`。master key 默认不能调本族，除非 `general_settings` 显式允许。

冻结字段（不得改名）：`session: model`、`modalities`、`voice`、`instructions; client_secrets: expires_after`。

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
{"model": "gpt-4o-realtime-preview", "voice": "alloy"}
```

## 响应

冻结响应字段：`WebSocket session events type+event_id; HTTP client_secret`。

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
{"client_secret": {"value": "ek_...", "expires_at": 1710000000}}
```

## 错误

OpenAI 兼容路径使用 `{"error":{"message","type","code","param"}}`。Anthropic 原生用 Anthropic 错误形状。Gemini 用 Google 错误形状。鉴权失败 401，模型不允许 401/403，预算/RPM/TPM 429 + `Retry-After`，参数 400。未实现 Provider 明确错误。共性见 [错误与响应头](../../../architecture/errors-headers.md)。

## 副作用

数据面调用记 spend、占 RPM/TPM/并发、写 SpendLogs，可走 Guardrail 与响应缓存。见 [生命周期](../../../architecture/request-lifecycle.md) 与 [计量](../../../architecture/spend-limits.md)。

## 控制台绑定

Console: —。动作对照 [前端绑定](../../../../frontend/binding.md) 与 [页面规格](../../../../frontend/pages/README.md)。无控制台入口的路径仍须对 SDK/curl 可用。

## 验收

- 本表每一行 method+path 均已注册；未实现时返回 JSON 错误信封，不是 HTML。
- 鉴权失败与成功主路径合同测试。
- 标准客户端只改 base_url + api_key（数据面）。
