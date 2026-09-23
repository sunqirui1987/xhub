# Files

- Family: `data.files`
- Status: `specified`
- 参考: 下表定位列
- Console: /vector-stores
- Auth: virtual-key

## 目的

本族是网关下列 HTTP 路径的完整分母。字段名与默认值以冻结契约及 该路由 handler 的请求体模型与上游 SDK 透传字段 为准，网关不得改名。

## HTTP 表面

实现必须注册每一行 method+path。规范化列用于覆盖校验（去掉 `:path` 与尾斜杠）。

| Method | Path | 规范化 path | 参考 |
|---|---|---|---|
| `GET` | `/files` | `/files` | `proxy/openai_files_endpoints/files_endpoints.py:1479` |
| `POST` | `/files` | `/files` | `proxy/openai_files_endpoints/files_endpoints.py:370` |
| `DELETE` | `/files/{file_id:path}` | `/files/{file_id}` | `proxy/openai_files_endpoints/files_endpoints.py:1256` |
| `GET` | `/files/{file_id:path}` | `/files/{file_id}` | `proxy/openai_files_endpoints/files_endpoints.py:1062` |
| `GET` | `/files/{file_id:path}/content` | `/files/{file_id}/content` | `proxy/openai_files_endpoints/files_endpoints.py:757` |
| `GET` | `/v1/files` | `/v1/files` | `proxy/openai_files_endpoints/files_endpoints.py:1474` |
| `POST` | `/v1/files` | `/v1/files` | `proxy/openai_files_endpoints/files_endpoints.py:365` |
| `DELETE` | `/v1/files/{file_id:path}` | `/v1/files/{file_id}` | `proxy/openai_files_endpoints/files_endpoints.py:1251` |
| `GET` | `/v1/files/{file_id:path}` | `/v1/files/{file_id}` | `proxy/openai_files_endpoints/files_endpoints.py:1057` |
| `GET` | `/v1/files/{file_id:path}/content` | `/v1/files/{file_id}/content` | `proxy/openai_files_endpoints/files_endpoints.py:752` |
| `GET` | `/v1/vector_stores/{vector_store_id}/files` | `/v1/vector_stores/{vector_store_id}/files` | `proxy/vector_store_files_endpoints/endpoints.py:589` |
| `POST` | `/v1/vector_stores/{vector_store_id}/files` | `/v1/vector_stores/{vector_store_id}/files` | `proxy/vector_store_files_endpoints/endpoints.py:482` |
| `DELETE` | `/v1/vector_stores/{vector_store_id}/files/{file_id}` | `/v1/vector_stores/{vector_store_id}/files/{file_id}` | `proxy/vector_store_files_endpoints/endpoints.py:1009` |
| `GET` | `/v1/vector_stores/{vector_store_id}/files/{file_id}` | `/v1/vector_stores/{vector_store_id}/files/{file_id}` | `proxy/vector_store_files_endpoints/endpoints.py:685` |
| `POST` | `/v1/vector_stores/{vector_store_id}/files/{file_id}` | `/v1/vector_stores/{vector_store_id}/files/{file_id}` | `proxy/vector_store_files_endpoints/endpoints.py:902` |
| `GET` | `/v1/vector_stores/{vector_store_id}/files/{file_id}/content` | `/v1/vector_stores/{vector_store_id}/files/{file_id}/content` | `proxy/vector_store_files_endpoints/endpoints.py:792` |
| `GET` | `/vector_stores/{vector_store_id}/files` | `/vector_stores/{vector_store_id}/files` | `proxy/vector_store_files_endpoints/endpoints.py:595` |
| `POST` | `/vector_stores/{vector_store_id}/files` | `/vector_stores/{vector_store_id}/files` | `proxy/vector_store_files_endpoints/endpoints.py:488` |
| `DELETE` | `/vector_stores/{vector_store_id}/files/{file_id}` | `/vector_stores/{vector_store_id}/files/{file_id}` | `proxy/vector_store_files_endpoints/endpoints.py:1015` |
| `GET` | `/vector_stores/{vector_store_id}/files/{file_id}` | `/vector_stores/{vector_store_id}/files/{file_id}` | `proxy/vector_store_files_endpoints/endpoints.py:691` |
| `POST` | `/vector_stores/{vector_store_id}/files/{file_id}` | `/vector_stores/{vector_store_id}/files/{file_id}` | `proxy/vector_store_files_endpoints/endpoints.py:908` |
| `GET` | `/vector_stores/{vector_store_id}/files/{file_id}/content` | `/vector_stores/{vector_store_id}/files/{file_id}/content` | `proxy/vector_store_files_endpoints/endpoints.py:798` |
| `GET` | `/{provider}/v1/files` | `/{provider}/v1/files` | `proxy/openai_files_endpoints/files_endpoints.py:1469` |
| `POST` | `/{provider}/v1/files` | `/{provider}/v1/files` | `proxy/openai_files_endpoints/files_endpoints.py:360` |
| `DELETE` | `/{provider}/v1/files/{file_id:path}` | `/{provider}/v1/files/{file_id}` | `proxy/openai_files_endpoints/files_endpoints.py:1246` |
| `GET` | `/{provider}/v1/files/{file_id:path}` | `/{provider}/v1/files/{file_id}` | `proxy/openai_files_endpoints/files_endpoints.py:1052` |
| `GET` | `/{provider}/v1/files/{file_id:path}/content` | `/{provider}/v1/files/{file_id}/content` | `proxy/openai_files_endpoints/files_endpoints.py:747` |

## 请求

`Authorization: Bearer sk-...` 或 `x-litellm-api-key`。`key_type` 为 `llm_api` 或 `default`。master key 默认不能调本族，除非 `general_settings` 显式允许。

冻结字段（不得改名）：`file (multipart)`、`purpose`、`expires_after`。

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
{"purpose": "batch"}
```

## 响应

冻结响应字段：`id`、`object`、`bytes`、`created_at`、`filename`、`purpose`、`status`。

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
{"id": "file_01", "object": "file", "bytes": 12, "filename": "in.jsonl", "purpose": "batch", "status": "processed"}
```

## 错误

OpenAI 兼容路径使用 `{"error":{"message","type","code","param"}}`。Anthropic 原生用 Anthropic 错误形状。Gemini 用 Google 错误形状。鉴权失败 401，模型不允许 401/403，预算/RPM/TPM 429 + `Retry-After`，参数 400。未实现 Provider 明确错误。共性见 [错误与响应头](../../../architecture/errors-headers.md)。

## 副作用

数据面调用记 spend、占 RPM/TPM/并发、写 SpendLogs，可走 Guardrail 与响应缓存。见 [生命周期](../../../architecture/request-lifecycle.md) 与 [计量](../../../architecture/spend-limits.md)。

## 控制台绑定

Console: /vector-stores。动作对照 [前端绑定](../../../../frontend/binding.md) 与 [页面规格](../../../../frontend/pages/README.md)。无控制台入口的路径仍须对 SDK/curl 可用。

## 验收

- 本表每一行 method+path 均已注册；未实现时返回 JSON 错误信封，不是 HTML。
- 鉴权失败与成功主路径合同测试。
- 标准客户端只改 base_url + api_key（数据面）。
