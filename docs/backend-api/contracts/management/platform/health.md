# Health and diagnostics

- Family: `mgmt.health`
- Status: `specified`
- 参考: 下表定位列
- Console: shell
- Auth: mixed

## 目的

本族是网关下列 HTTP 路径的完整分母。字段名与默认值以冻结契约及 该路由 handler 的请求体模型与上游 SDK 透传字段 为准，网关不得改名。

## HTTP 表面

实现必须注册每一行 method+path。规范化列用于覆盖校验（去掉 `:path` 与尾斜杠）。

| Method | Path | 规范化 path | 参考 |
|---|---|---|---|
| `GET` | `/` | `/` |
| `GET` | `//` | `/` | `proxy/proxy_server.py:2316` |
| `GET` | `/health` | `/health` | `proxy/health_endpoints/_health_endpoints.py:1118` |
| `GET` | `/health/backlog` | `/health/backlog` | `proxy/health_endpoints/_health_endpoints.py:1874` |
| `GET` | `/health/drain` | `/health/drain` | `proxy/health_endpoints/_health_endpoints.py:1898` |
| `GET` | `/health/history` | `/health/history` | `proxy/health_endpoints/_health_endpoints.py:1292` |
| `GET` | `/health/latest` | `/health/latest` | `proxy/health_endpoints/_health_endpoints.py:1332` |
| `GET` | `/health/license` | `/health/license` | `proxy/health_endpoints/_health_endpoints.py:1441` |
| `GET` | `/health/liveliness` | `/health/liveliness` | `proxy/health_endpoints/_health_endpoints.py:1946` |
| `OPTIONS` | `/health/liveliness` | `/health/liveliness` | `proxy/health_endpoints/_health_endpoints.py:1983` |
| `GET` | `/health/liveness` | `/health/liveness` | `proxy/health_endpoints/_health_endpoints.py:1950` |
| `OPTIONS` | `/health/liveness` | `/health/liveness` | `proxy/health_endpoints/_health_endpoints.py:1987` |
| `GET` | `/health/readiness` | `/health/readiness` | `proxy/health_endpoints/_health_endpoints.py:1839` |
| `OPTIONS` | `/health/readiness` | `/health/readiness` | `proxy/health_endpoints/_health_endpoints.py:1967` |
| `GET` | `/health/readiness/details` | `/health/readiness/details` | `proxy/health_endpoints/_health_endpoints.py:1862` |
| `GET` | `/health/services` | `/health/services` | `proxy/health_endpoints/_health_endpoints.py:262` |
| `GET` | `/health/shared-status` | `/health/shared-status` | `proxy/health_endpoints/_health_endpoints.py:1364` |
| `POST` | `/health/test_connection` | `/health/test_connection` | `proxy/health_endpoints/_health_endpoints.py:2003` |
| `GET` | `/settings` | `/settings` | `proxy/health_endpoints/_health_endpoints.py:1551` |
| `GET` | `/test` | `/test` | `proxy/health_endpoints/_health_endpoints.py:238` |
| `POST` | `/test/connection` | `/test/connection` | `proxy/_experimental/mcp_server/rest_endpoints.py:1554` |
| `POST` | `/test/tools/list` | `/test/tools/list` | `proxy/_experimental/mcp_server/rest_endpoints.py:1588` |

## 请求

数据面虚拟 Key，管理面 session/master key。OAuth 回调走浏览器 session。见 [鉴权决策](../../../architecture/auth-decision.md)。

冻结字段（不得改名）：`test_connection: model`、`mode`、`api_base`、`api_key`。

### 请求头

| Header | 必填 | 说明 |
|---|---|---|
| `Authorization` | 是 | `Bearer` master key 或 session |
| `Content-Type` | JSON 时是 | `application/json` 或 multipart |

### 请求体

```json
{"model": "gpt-4o-mini", "mode": "chat"}
```

## 响应

冻结响应字段：`status`、`healthy_count`、`unhealthy_count`、`details`。

### 响应头

| Header | 说明 |
|---|---|
| `x-litellm-call-id` | 见 [错误与响应头](../../../architecture/errors-headers.md) |
| `Content-Type` | 见 [错误与响应头](../../../architecture/errors-headers.md) |
| `Retry-After` | 429 时 |

### 响应体

```json
{"status": "healthy"}
```

## 错误

OpenAI 兼容路径使用 `{"error":{"message","type","code","param"}}`。Anthropic 原生用 Anthropic 错误形状。Gemini 用 Google 错误形状。鉴权失败 401，模型不允许 401/403，预算/RPM/TPM 429 + `Retry-After`，参数 400。未实现 Provider 明确错误。共性见 [错误与响应头](../../../architecture/errors-headers.md)。

## 副作用

管理写操作记 AuditLog 并失效 Auth cache。测试调用（如 `/prompts/test`、`/health/test_connection`）仍记 spend。

## 控制台绑定

Console: shell。动作对照 [前端绑定](../../../../frontend/binding.md) 与 [页面规格](../../../../frontend/pages/README.md)。无控制台入口的路径仍须对 SDK/curl 可用。

## 验收

- 本表每一行 method+path 均已注册；未实现时返回 JSON 错误信封，不是 HTML。
- 鉴权失败与成功主路径合同测试。
- 标准客户端只改 base_url + api_key（数据面）。
