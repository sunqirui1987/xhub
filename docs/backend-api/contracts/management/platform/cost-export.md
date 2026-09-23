# CloudZero and Vantage export

- Family: `mgmt.cost_export`
- Status: `specified`
- 参考: 下表定位列
- Console: /cost-tracking
- Auth: management

## 目的

本族是网关下列 HTTP 路径的完整分母。字段名与默认值以冻结契约及 该路由 handler 的请求体模型与上游 SDK 透传字段 为准，网关不得改名。

## HTTP 表面

实现必须注册每一行 method+path。规范化列用于覆盖校验（去掉 `:path` 与尾斜杠）。

| Method | Path | 规范化 path | 参考 |
|---|---|---|---|
| `DELETE` | `/cloudzero/delete` | `/cloudzero/delete` | `proxy/spend_tracking/cloudzero_endpoints.py:518` |
| `POST` | `/cloudzero/dry-run` | `/cloudzero/dry-run` | `proxy/spend_tracking/cloudzero_endpoints.py:395` |
| `POST` | `/cloudzero/export` | `/cloudzero/export` | `proxy/spend_tracking/cloudzero_endpoints.py:453` |
| `POST` | `/cloudzero/init` | `/cloudzero/init` | `proxy/spend_tracking/cloudzero_endpoints.py:345` |
| `GET` | `/cloudzero/settings` | `/cloudzero/settings` | `proxy/spend_tracking/cloudzero_endpoints.py:133` |
| `PUT` | `/cloudzero/settings` | `/cloudzero/settings` | `proxy/spend_tracking/cloudzero_endpoints.py:192` |
| `DELETE` | `/vantage/delete` | `/vantage/delete` | `proxy/spend_tracking/vantage_endpoints.py:521` |
| `POST` | `/vantage/dry-run` | `/vantage/dry-run` | `proxy/spend_tracking/vantage_endpoints.py:357` |
| `POST` | `/vantage/export` | `/vantage/export` | `proxy/spend_tracking/vantage_endpoints.py:448` |
| `POST` | `/vantage/init` | `/vantage/init` | `proxy/spend_tracking/vantage_endpoints.py:310` |
| `GET` | `/vantage/settings` | `/vantage/settings` | `proxy/spend_tracking/vantage_endpoints.py:147` |
| `PUT` | `/vantage/settings` | `/vantage/settings` | `proxy/spend_tracking/vantage_endpoints.py:199` |

## 请求

master key、SSO session，或 `key_type=management` / 具备对应角色的虚拟 Key。按 `user_api_key_auth`。

冻结字段（不得改名）：`api_key`、`connection settings; export: start_date`、`end_date`。

### 请求头

| Header | 必填 | 说明 |
|---|---|---|
| `Authorization` | 是 | `Bearer` master key 或 session |
| `Content-Type` | JSON 时是 | `application/json` 或 multipart |

### 请求体

```json
{"api_key": "..."}
```

## 响应

冻结响应字段：`status`、`exported_count`。

### 响应头

| Header | 说明 |
|---|---|
| `x-litellm-call-id` | 见 [错误与响应头](../../../architecture/errors-headers.md) |
| `Content-Type` | 见 [错误与响应头](../../../architecture/errors-headers.md) |
| `Retry-After` | 429 时 |

### 响应体

```json
{"status": "ok"}
```

## 错误

OpenAI 兼容路径使用 `{"error":{"message","type","code","param"}}`。Anthropic 原生用 Anthropic 错误形状。Gemini 用 Google 错误形状。鉴权失败 401，模型不允许 401/403，预算/RPM/TPM 429 + `Retry-After`，参数 400。未实现 Provider 明确错误。共性见 [错误与响应头](../../../architecture/errors-headers.md)。

## 副作用

管理写操作记 AuditLog 并失效 Auth cache。测试调用（如 `/prompts/test`、`/health/test_connection`）仍记 spend。

## 控制台绑定

Console: /cost-tracking。动作对照 [前端绑定](../../../../frontend/binding.md) 与 [页面规格](../../../../frontend/pages/README.md)。无控制台入口的路径仍须对 SDK/curl 可用。

## 验收

- 本表每一行 method+path 均已注册；未实现时返回 JSON 错误信封，不是 HTML。
- 鉴权失败与成功主路径合同测试。
- 标准客户端只改 base_url + api_key（数据面）。
