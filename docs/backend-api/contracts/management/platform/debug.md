# Debug and memory

- Family: `mgmt.debug`
- Status: `specified`
- 参考: 下表定位列
- Console: —
- Auth: master-key

## 目的

本族是网关下列 HTTP 路径的完整分母。字段名与默认值以冻结契约及 该路由 handler 的请求体模型与上游 SDK 透传字段 为准，网关不得改名。

## HTTP 表面

实现必须注册每一行 method+path。规范化列用于覆盖校验（去掉 `:path` 与尾斜杠）。

| Method | Path | 规范化 path | 参考 |
|---|---|---|---|
| `GET` | `/debug/asyncio-tasks` | `/debug/asyncio-tasks` | `proxy/common_utils/debug_utils.py:57` |
| `GET` | `/debug/memory/details` | `/debug/memory/details` | `proxy/common_utils/debug_utils.py:669` |
| `POST` | `/debug/memory/gc/configure` | `/debug/memory/gc/configure` | `proxy/common_utils/debug_utils.py:728` |
| `GET` | `/debug/memory/summary` | `/debug/memory/summary` | `proxy/common_utils/debug_utils.py:315` |
| `POST` | `/lazy/warm/{name}` | `/lazy/warm/{name}` | `proxy/_lazy_features.py:471` |
| `GET` | `/memory-usage` | `/memory-usage` | `proxy/common_utils/debug_utils.py:107` |
| `GET` | `/memory-usage-in-mem-cache` | `/memory-usage-in-mem-cache` | `proxy/common_utils/debug_utils.py:127` |
| `GET` | `/memory-usage-in-mem-cache-items` | `/memory-usage-in-mem-cache-items` | `proxy/common_utils/debug_utils.py:169` |
| `GET` | `/otel-spans` | `/otel-spans` | `proxy/common_utils/debug_utils.py:786` |

## 请求

仅 master key / proxy_admin。

冻结字段（不得改名）：`gc configure: enabled`。

### 请求头

| Header | 必填 | 说明 |
|---|---|---|
| `Authorization` | 是 | `Bearer` master key 或 session |
| `Content-Type` | JSON 时是 | `application/json` 或 multipart |

### 请求体

```json
{"enabled": true}
```

## 响应

冻结响应字段：`rss_mb`、`heap`、`asyncio_tasks`。

### 响应头

| Header | 说明 |
|---|---|
| `x-litellm-call-id` | 见 [错误与响应头](../../../architecture/errors-headers.md) |
| `Content-Type` | 见 [错误与响应头](../../../architecture/errors-headers.md) |
| `Retry-After` | 429 时 |

### 响应体

```json
{"rss_mb": 256}
```

## 错误

OpenAI 兼容路径使用 `{"error":{"message","type","code","param"}}`。Anthropic 原生用 Anthropic 错误形状。Gemini 用 Google 错误形状。鉴权失败 401，模型不允许 401/403，预算/RPM/TPM 429 + `Retry-After`，参数 400。未实现 Provider 明确错误。共性见 [错误与响应头](../../../architecture/errors-headers.md)。

## 副作用

管理写操作记 AuditLog 并失效 Auth cache。测试调用（如 `/prompts/test`、`/health/test_connection`）仍记 spend。

## 控制台绑定

Console: —。动作对照 [前端绑定](../../../../frontend/binding.md) 与 [页面规格](../../../../frontend/pages/README.md)。无控制台入口的路径仍须对 SDK/curl 可用。

## 验收

- 本表每一行 method+path 均已注册；未实现时返回 JSON 错误信封，不是 HTML。
- 鉴权失败与成功主路径合同测试。
- 标准客户端只改 base_url + api_key（数据面）。
