# Gemini v1beta

- Family: `data.gemini_v1beta`
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
| `POST` | `/models/{model_name:path}:countTokens` | `/models/{model_name}:countTokens` | `proxy/google_endpoints/endpoints.py:148` |
| `POST` | `/models/{model_name:path}:generateContent` | `/models/{model_name}:generateContent` | `proxy/google_endpoints/endpoints.py:21` |
| `POST` | `/models/{model_name:path}:streamGenerateContent` | `/models/{model_name}:streamGenerateContent` | `proxy/google_endpoints/endpoints.py:82` |
| `GET` | `/models/{model}:generateContent` | `/models/{model}:generateContent` | `catalog.families:0` |
| `POST` | `/models/{model}:generateContent` | `/models/{model}:generateContent` | `catalog.families:0` |
| `POST` | `/v1beta/models/{model_name:path}:countTokens` | `/v1beta/models/{model_name}:countTokens` | `proxy/google_endpoints/endpoints.py:143` |
| `POST` | `/v1beta/models/{model_name:path}:generateContent` | `/v1beta/models/{model_name}:generateContent` | `proxy/google_endpoints/endpoints.py:17` |
| `POST` | `/v1beta/models/{model_name:path}:streamGenerateContent` | `/v1beta/models/{model_name}:streamGenerateContent` | `proxy/google_endpoints/endpoints.py:78` |
| `GET` | `/v1beta/models/{model}:countTokens` | `/v1beta/models/{model}:countTokens` | `catalog.families:0` |
| `POST` | `/v1beta/models/{model}:countTokens` | `/v1beta/models/{model}:countTokens` | `catalog.families:0` |
| `GET` | `/v1beta/models/{model}:generateContent` | `/v1beta/models/{model}:generateContent` | `catalog.families:0` |
| `POST` | `/v1beta/models/{model}:generateContent` | `/v1beta/models/{model}:generateContent` | `catalog.families:0` |
| `GET` | `/v1beta/models/{model}:streamGenerateContent` | `/v1beta/models/{model}:streamGenerateContent` | `catalog.families:0` |
| `POST` | `/v1beta/models/{model}:streamGenerateContent` | `/v1beta/models/{model}:streamGenerateContent` | `catalog.families:0` |

## 请求

`Authorization: Bearer sk-...` 或 `x-litellm-api-key`。`key_type` 为 `llm_api` 或 `default`。master key 默认不能调本族，除非 `general_settings` 显式允许。

冻结字段（不得改名）：`contents`、`generationConfig`、`tools`、`safetySettings`、`systemInstruction`。

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
{"contents": [{"role": "user", "parts": [{"text": "Hi"}]}]}
```

## 响应

冻结响应字段：`candidates`、`usageMetadata`。

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
{"candidates": [{"content": {"parts": [{"text": "Hello"}]}}], "usageMetadata": {"promptTokenCount": 1, "candidatesTokenCount": 1}}
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
