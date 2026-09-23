# Model admin

- Family: `mgmt.models`
- Status: `specified`
- 参考: 下表定位列
- Console: /models-and-endpoints
- Auth: management

## 目的

本族是网关下列 HTTP 路径的完整分母。字段名与默认值以冻结契约及 该路由 handler 的请求体模型与上游 SDK 透传字段 为准，网关不得改名。

## HTTP 表面

实现必须注册每一行 method+path。规范化列用于覆盖校验（去掉 `:path` 与尾斜杠）。

| Method | Path | 规范化 path | 参考 |
|---|---|---|---|
| `POST` | `/model/block` | `/model/block` | `proxy/management_endpoints/model_management_endpoints.py:1119` |
| `GET` | `/model/cost_map/source` | `/model/cost_map/source` | `proxy/proxy_server.py:18353` |
| `POST` | `/model/delete` | `/model/delete` | `proxy/management_endpoints/model_management_endpoints.py:1834` |
| `GET` | `/model/info` | `/model/info` | `proxy/proxy_server.py:15060` |
| `GET` | `/model/metrics` | `/model/metrics` | `proxy/proxy_server.py:14638` |
| `GET` | `/model/metrics/exceptions` | `/model/metrics/exceptions` | `proxy/proxy_server.py:14842` |
| `GET` | `/model/metrics/slow_responses` | `/model/metrics/slow_responses` | `proxy/proxy_server.py:14753` |
| `POST` | `/model/new` | `/model/new` | `proxy/management_endpoints/model_management_endpoints.py:2007` |
| `GET` | `/model/settings` | `/model/settings` | `proxy/proxy_server.py:15551` |
| `GET` | `/model/streaming_metrics` | `/model/streaming_metrics` | `proxy/proxy_server.py:14506` |
| `POST` | `/model/unblock` | `/model/unblock` | `proxy/management_endpoints/model_management_endpoints.py:1148` |
| `POST` | `/model/update` | `/model/update` | `proxy/management_endpoints/model_management_endpoints.py:2209` |
| `PATCH` | `/model/{model_id}/update` | `/model/{model_id}/update` | `proxy/management_endpoints/model_management_endpoints.py:816` |
| `GET` | `/model_group/info` | `/model_group/info` | `proxy/proxy_server.py:15353` |
| `POST` | `/model_group/make_public` | `/model_group/make_public` | `proxy/management_endpoints/model_management_endpoints.py:2396` |
| `GET` | `/v1/model/info` | `/v1/model/info` | `proxy/proxy_server.py:15065` |
| `GET` | `/v2/model/info` | `/v2/model/info` | `proxy/proxy_server.py:14269` |

## 请求

master key、SSO session，或 `key_type=management` / 具备对应角色的虚拟 Key。按 `user_api_key_auth`。

冻结字段（不得改名）：`model_name`、`litellm_params (model`、`api_base`、`api_key`、`custom_llm_provider`、`rpm`、`tpm`、`timeout`、`stream_timeout)`、`model_info (id`、`mode`、`base_model)`。

### 请求头

| Header | 必填 | 说明 |
|---|---|---|
| `Authorization` | 是 | `Bearer` master key 或 session |
| `Content-Type` | JSON 时是 | `application/json` 或 multipart |

### 请求体

```json
{"model_name": "gpt-4o-mini", "litellm_params": {"model": "openai/gpt-4o-mini", "api_key": "os.environ/OPENAI_API_KEY", "rpm": 480, "timeout": 60}}
```

## 响应

冻结响应字段：`model_name`、`litellm_params`、`model_info`、`blocked`。

### 响应头

| Header | 说明 |
|---|---|
| `x-litellm-call-id` | 见 [错误与响应头](../../../architecture/errors-headers.md) |
| `Content-Type` | 见 [错误与响应头](../../../architecture/errors-headers.md) |
| `Retry-After` | 429 时 |

### 响应体

```json
{"model_name": "gpt-4o-mini", "model_info": {"id": "model_01", "mode": "chat"}}
```

## 错误

OpenAI 兼容路径使用 `{"error":{"message","type","code","param"}}`。Anthropic 原生用 Anthropic 错误形状。Gemini 用 Google 错误形状。鉴权失败 401，模型不允许 401/403，预算/RPM/TPM 429 + `Retry-After`，参数 400。未实现 Provider 明确错误。共性见 [错误与响应头](../../../architecture/errors-headers.md)。

## 副作用

管理写操作记 AuditLog 并失效 Auth cache。测试调用（如 `/prompts/test`、`/health/test_connection`）仍记 spend。

## 控制台绑定

Console: /models-and-endpoints。动作对照 [前端绑定](../../../../frontend/binding.md) 与 [页面规格](../../../../frontend/pages/README.md)。无控制台入口的路径仍须对 SDK/curl 可用。

## 验收

- 本表每一行 method+path 均已注册；未实现时返回 JSON 错误信封，不是 HTML。
- 鉴权失败与成功主路径合同测试。
- 标准客户端只改 base_url + api_key（数据面）。
