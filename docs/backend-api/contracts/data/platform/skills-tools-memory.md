# Skills, Tools, Memory

- Family: `data.skills_tools_memory`
- Status: `specified`
- 参考: 下表定位列
- Console: /skills /memory /tool-policies
- Auth: mixed

## 目的

本族是网关下列 HTTP 路径的完整分母。字段名与默认值以冻结契约及 该路由 handler 的请求体模型与上游 SDK 透传字段 为准，网关不得改名。

## HTTP 表面

实现必须注册每一行 method+path。规范化列用于覆盖校验（去掉 `:path` 与尾斜杠）。

| Method | Path | 规范化 path | 参考 |
|---|---|---|---|
| `GET` | `/.well-known/agent-skills/index.json` | `/.well-known/agent-skills/index.json` | `proxy/discovery_endpoints/agent_skills_endpoints.py:74` |
| `GET` | `/.well-known/skills/index.json` | `/.well-known/skills/index.json` | `proxy/discovery_endpoints/agent_skills_endpoints.py:79` |
| `GET` | `/v1/memory` | `/v1/memory` | `proxy/memory/memory_endpoints.py:345` |
| `POST` | `/v1/memory` | `/v1/memory` | `proxy/memory/memory_endpoints.py:299` |
| `DELETE` | `/v1/memory/{key:path}` | `/v1/memory/{key}` | `proxy/memory/memory_endpoints.py:554` |
| `GET` | `/v1/memory/{key:path}` | `/v1/memory/{key}` | `proxy/memory/memory_endpoints.py:427` |
| `PUT` | `/v1/memory/{key:path}` | `/v1/memory/{key}` | `proxy/memory/memory_endpoints.py:443` |
| `GET` | `/v1/skills` | `/v1/skills` | `proxy/anthropic_endpoints/skills_endpoints.py:181` |
| `POST` | `/v1/skills` | `/v1/skills` | `proxy/anthropic_endpoints/skills_endpoints.py:83` |
| `DELETE` | `/v1/skills/{skill_id}` | `/v1/skills/{skill_id}` | `proxy/anthropic_endpoints/skills_endpoints.py:408` |
| `GET` | `/v1/skills/{skill_id}` | `/v1/skills/{skill_id}` | `proxy/anthropic_endpoints/skills_endpoints.py:311` |
| `GET` | `/v1/skills/{skill_id}/archive` | `/v1/skills/{skill_id}/archive` | `proxy/discovery_endpoints/agent_skills_endpoints.py:112` |
| `GET` | `/v1/tool` | `/v1/tool` | `catalog.families:0` |
| `POST` | `/v1/tool` | `/v1/tool` | `catalog.families:0` |
| `GET` | `/v1/tool/list` | `/v1/tool/list` | `proxy/management_endpoints/tool_management_endpoints.py:177` |
| `POST` | `/v1/tool/policy` | `/v1/tool/policy` | `proxy/management_endpoints/tool_management_endpoints.py:585` |
| `GET` | `/v1/tool/policy/options` | `/v1/tool/policy/options` | `proxy/management_endpoints/tool_management_endpoints.py:161` |
| `GET` | `/v1/tool/spend` | `/v1/tool/spend` | `proxy/management_endpoints/tool_management_endpoints.py:233` |
| `GET` | `/v1/tool/{tool_name:path}` | `/v1/tool/{tool_name}` | `proxy/management_endpoints/tool_management_endpoints.py:494` |
| `GET` | `/v1/tool/{tool_name:path}/detail` | `/v1/tool/{tool_name}/detail` | `proxy/management_endpoints/tool_management_endpoints.py:314` |
| `GET` | `/v1/tool/{tool_name:path}/logs` | `/v1/tool/{tool_name}/logs` | `proxy/management_endpoints/tool_management_endpoints.py:404` |
| `DELETE` | `/v1/tool/{tool_name:path}/overrides` | `/v1/tool/{tool_name}/overrides` | `proxy/management_endpoints/tool_management_endpoints.py:698` |

## 请求

数据面虚拟 Key，管理面 session/master key。OAuth 回调走浏览器 session。见 [鉴权决策](../../../architecture/auth-decision.md)。

冻结字段（不得改名）：`skills: name`、`source; memory: key`、`value; tool policy: tool_name`、`allowed`。

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
{"key": "pref", "value": "dark"}
```

## 响应

冻结响应字段：`id / key / value / skill objects`。

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
{"key": "pref", "value": "dark"}
```

## 错误

OpenAI 兼容路径使用 `{"error":{"message","type","code","param"}}`。Anthropic 原生用 Anthropic 错误形状。Gemini 用 Google 错误形状。鉴权失败 401，模型不允许 401/403，预算/RPM/TPM 429 + `Retry-After`，参数 400。未实现 Provider 明确错误。共性见 [错误与响应头](../../../architecture/errors-headers.md)。

## 副作用

数据面调用记 spend、占 RPM/TPM/并发、写 SpendLogs，可走 Guardrail 与响应缓存。见 [生命周期](../../../architecture/request-lifecycle.md) 与 [计量](../../../architecture/spend-limits.md)。

## 控制台绑定

Console: /skills /memory /tool-policies。动作对照 [前端绑定](../../../../frontend/binding.md) 与 [页面规格](../../../../frontend/pages/README.md)。无控制台入口的路径仍须对 SDK/curl 可用。

## 验收

- 本表每一行 method+path 均已注册；未实现时返回 JSON 错误信封，不是 HTML。
- 鉴权失败与成功主路径合同测试。
- 标准客户端只改 base_url + api_key（数据面）。
