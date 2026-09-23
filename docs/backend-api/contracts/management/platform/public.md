# Public discovery hubs

- Family: `mgmt.public`
- Status: `specified`
- 参考: 下表定位列
- Console: /model_hub /connect
- Auth: public-or-key

## 目的

本族是网关下列 HTTP 路径的完整分母。字段名与默认值以冻结契约及 该路由 handler 的请求体模型与上游 SDK 透传字段 为准，网关不得改名。

## HTTP 表面

实现必须注册每一行 method+path。规范化列用于覆盖校验（去掉 `:path` 与尾斜杠）。

| Method | Path | 规范化 path | 参考 |
|---|---|---|---|
| `GET` | `/public/agent_hub` | `/public/agent_hub` | `proxy/public_endpoints/public_endpoints.py:263` |
| `GET` | `/public/agents/fields` | `/public/agents/fields` | `proxy/public_endpoints/public_endpoints.py:568` |
| `GET` | `/public/autorouter_presets` | `/public/autorouter_presets` | `proxy/public_endpoints/public_endpoints.py:533` |
| `GET` | `/public/complexity_router/scorer_defaults` | `/public/complexity_router/scorer_defaults` | `proxy/public_endpoints/public_endpoints.py:405` |
| `GET` | `/public/endpoints` | `/public/endpoints` | `proxy/public_endpoints/public_endpoints.py:550` |
| `GET` | `/public/litellm_blog_posts` | `/public/litellm_blog_posts` | `proxy/public_endpoints/public_endpoints.py:448` |
| `GET` | `/public/litellm_model_cost_map` | `/public/litellm_model_cost_map` | `proxy/public_endpoints/public_endpoints.py:427` |
| `GET` | `/public/mcp_hub` | `/public/mcp_hub` | `proxy/public_endpoints/public_endpoints.py:287` |
| `GET` | `/public/providers` | `/public/providers` | `proxy/public_endpoints/public_endpoints.py:369` |
| `GET` | `/public/providers/fields` | `/public/providers/fields` | `proxy/public_endpoints/public_endpoints.py:382` |
| `GET` | `/public/skill_hub` | `/public/skill_hub` | `proxy/public_endpoints/public_endpoints.py:301` |

## 请求

公开发现路径可无 Key；写操作需要虚拟 Key 或 session。

冻结字段（不得改名）：`GET 无 body；部分 facet 用 query `facet``。

### 请求头

| Header | 必填 | 说明 |
|---|---|---|
| `Authorization` | 是 | `Bearer` master key 或 session |
| `Content-Type` | JSON 时是 | `application/json` 或 multipart |

### 请求体

```json
{"facet": "providers"}
```

## 响应

冻结响应字段：`hub lists: name`、`description`、`provider fields`。

### 响应头

| Header | 说明 |
|---|---|
| `x-litellm-call-id` | 见 [错误与响应头](../../../architecture/errors-headers.md) |
| `Content-Type` | 见 [错误与响应头](../../../architecture/errors-headers.md) |
| `Retry-After` | 429 时 |

### 响应体

```json
{"data": [{"name": "gpt-4o-mini", "provider": "openai"}]}
```

## 错误

OpenAI 兼容路径使用 `{"error":{"message","type","code","param"}}`。Anthropic 原生用 Anthropic 错误形状。Gemini 用 Google 错误形状。鉴权失败 401，模型不允许 401/403，预算/RPM/TPM 429 + `Retry-After`，参数 400。未实现 Provider 明确错误。共性见 [错误与响应头](../../../architecture/errors-headers.md)。

## 副作用

管理写操作记 AuditLog 并失效 Auth cache。测试调用（如 `/prompts/test`、`/health/test_connection`）仍记 spend。

## 控制台绑定

Console: /model_hub /connect。动作对照 [前端绑定](../../../../frontend/binding.md) 与 [页面规格](../../../../frontend/pages/README.md)。无控制台入口的路径仍须对 SDK/curl 可用。

## 验收

- 本表每一行 method+path 均已注册；未实现时返回 JSON 错误信封，不是 HTML。
- 鉴权失败与成功主路径合同测试。
- 标准客户端只改 base_url + api_key（数据面）。
