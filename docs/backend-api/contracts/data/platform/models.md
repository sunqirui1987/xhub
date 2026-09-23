# Models list and Model Hub

- Family: `data.model_hub`
- Status: `specified`
- 参考: 下表定位列
- Console: /model-hub-table /model_hub
- Auth: virtual-key

## 目的

本族是网关下列 HTTP 路径的完整分母。字段名与默认值以冻结契约及 该路由 handler 的请求体模型与上游 SDK 透传字段 为准，网关不得改名。

## HTTP 表面

实现必须注册每一行 method+path。规范化列用于覆盖校验（去掉 `:path` 与尾斜杠）。

| Method | Path | 规范化 path | 参考 |
|---|---|---|---|
| `GET` | `/cursor/models` | `/cursor/models` | `proxy/response_api_endpoints/endpoints.py:442` |
| `GET` | `/model/deprecations` | `/model/deprecations` | `proxy/proxy_server.py:15277` |
| `GET` | `/model_hub` | `/model_hub` | `proxy/public_endpoints/public_v1/model_hub.py:226` |
| `POST` | `/model_hub/update_useful_links` | `/model_hub/update_useful_links` | `proxy/management_endpoints/model_management_endpoints.py:2484` |
| `GET` | `/model_hub/{facet}` | `/model_hub/{facet}` | `proxy/public_endpoints/public_v1/model_hub.py:276` |
| `GET` | `/models` | `/models` | `proxy/proxy_server.py:10606` |
| `GET` | `/models/{model_id}` | `/models/{model_id}` | `proxy/proxy_server.py:10842` |
| `GET` | `/public/model_hub` | `/public/model_hub` | `proxy/public_endpoints/public_endpoints.py:211` |
| `GET` | `/public/model_hub/info` | `/public/model_hub/info` | `proxy/public_endpoints/public_endpoints.py:345` |
| `GET` | `/v1/model/deprecations` | `/v1/model/deprecations` | `proxy/proxy_server.py:15283` |
| `GET` | `/v1/models` | `/v1/models` | `proxy/proxy_server.py:10605` |
| `GET` | `/v1/models/{model_id}` | `/v1/models/{model_id}` | `proxy/proxy_server.py:10837` |

## 请求

`Authorization: Bearer sk-...` 或 `x-litellm-api-key`。`key_type` 为 `llm_api` 或 `default`。master key 默认不能调本族，除非 `general_settings` 显式允许。

冻结字段（不得改名）：`无 body（GET）；update_useful_links: useful_links`。

### 请求头

| Header | 必填 | 说明 |
|---|---|---|
| `Authorization` | 是 | `Bearer` master key 或 session |
| `Content-Type` | JSON 时是 | `application/json` 或 multipart |

### 请求体

```json
{"useful_links": {"docs": "https://example.com"}}
```

## 响应

冻结响应字段：`object`、`data[].id`、`object`、`created`、`owned_by`。

### 响应头

| Header | 说明 |
|---|---|
| `x-litellm-call-id` | 见 [错误与响应头](../../../architecture/errors-headers.md) |
| `Content-Type` | 见 [错误与响应头](../../../architecture/errors-headers.md) |
| `Retry-After` | 429 时 |

### 响应体

```json
{"object": "list", "data": [{"id": "gpt-4o-mini", "object": "model", "owned_by": "openai"}]}
```

## 错误

OpenAI 兼容路径使用 `{"error":{"message","type","code","param"}}`。Anthropic 原生用 Anthropic 错误形状。Gemini 用 Google 错误形状。鉴权失败 401，模型不允许 401/403，预算/RPM/TPM 429 + `Retry-After`，参数 400。未实现 Provider 明确错误。共性见 [错误与响应头](../../../architecture/errors-headers.md)。

## 副作用

管理写操作记 AuditLog 并失效 Auth cache。测试调用（如 `/prompts/test`、`/health/test_connection`）仍记 spend。

## 控制台绑定

Console: /model-hub-table /model_hub。动作对照 [前端绑定](../../../../frontend/binding.md) 与 [页面规格](../../../../frontend/pages/README.md)。无控制台入口的路径仍须对 SDK/curl 可用。

## 验收

- 本表每一行 method+path 均已注册；未实现时返回 JSON 错误信封，不是 HTML。
- 鉴权失败与成功主路径合同测试。
- 标准客户端只改 base_url + api_key（数据面）。
