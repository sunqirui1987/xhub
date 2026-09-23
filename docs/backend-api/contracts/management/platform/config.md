# Config and YAML

- Family: `mgmt.config`
- Status: `specified`
- 参考: 下表定位列
- Console: /models-and-endpoints /router-settings
- Auth: management

## 目的

本族是网关下列 HTTP 路径的完整分母。字段名与默认值以冻结契约及 该路由 handler 的请求体模型与上游 SDK 透传字段 为准，网关不得改名。

## HTTP 表面

实现必须注册每一行 method+path。规范化列用于覆盖校验（去掉 `:path` 与尾斜杠）。

| Method | Path | 规范化 path | 参考 |
|---|---|---|---|
| `GET` | `/config/block_requests_for_models_without_pricing` | `/config/block_requests_for_models_without_pricing` | `proxy/management_endpoints/cost_tracking_settings.py:507` |
| `PATCH` | `/config/block_requests_for_models_without_pricing` | `/config/block_requests_for_models_without_pricing` | `proxy/management_endpoints/cost_tracking_settings.py:517` |
| `POST` | `/config/callback/delete` | `/config/callback/delete` | `proxy/proxy_server.py:17810` |
| `GET` | `/config/cost_discount_config` | `/config/cost_discount_config` | `proxy/management_endpoints/cost_tracking_settings.py:192` |
| `PATCH` | `/config/cost_discount_config` | `/config/cost_discount_config` | `proxy/management_endpoints/cost_tracking_settings.py:227` |
| `GET` | `/config/cost_margin_config` | `/config/cost_margin_config` | `proxy/management_endpoints/cost_tracking_settings.py:325` |
| `PATCH` | `/config/cost_margin_config` | `/config/cost_margin_config` | `proxy/management_endpoints/cost_tracking_settings.py:360` |
| `POST` | `/config/field/delete` | `/config/field/delete` | `proxy/proxy_server.py:17729` |
| `GET` | `/config/field/info` | `/config/field/info` | `proxy/proxy_server.py:17364` |
| `POST` | `/config/field/update` | `/config/field/update` | `proxy/proxy_server.py:17120` |
| `GET` | `/config/list` | `/config/list` | `proxy/proxy_server.py:17562` |
| `DELETE` | `/config/pass_through_endpoint` | `/config/pass_through_endpoint` | `proxy/pass_through_endpoints/pass_through_endpoints.py:3683` |
| `GET` | `/config/pass_through_endpoint` | `/config/pass_through_endpoint` | `proxy/pass_through_endpoints/pass_through_endpoints.py:3396` |
| `POST` | `/config/pass_through_endpoint` | `/config/pass_through_endpoint` | `proxy/pass_through_endpoints/pass_through_endpoints.py:3591` |
| `GET` | `/config/pass_through_endpoint/team/{team_id}` | `/config/pass_through_endpoint/team/{team_id}` | `proxy/pass_through_endpoints/pass_through_endpoints.py:3401` |
| `POST` | `/config/pass_through_endpoint/{endpoint_id}` | `/config/pass_through_endpoint/{endpoint_id}` | `proxy/pass_through_endpoints/pass_through_endpoints.py:3452` |
| `GET` | `/config/pass_through_endpoints/settings` | `/config/pass_through_endpoints/settings` | `proxy/config_management_endpoints/pass_through_endpoints.py:17` |
| `POST` | `/config/update` | `/config/update` | `proxy/proxy_server.py:16852` |
| `GET` | `/config/yaml` | `/config/yaml` | `proxy/proxy_server.py:18128` |
| `DELETE` | `/config_overrides/cyberark` | `/config_overrides/cyberark` | `proxy/management_endpoints/config_override_endpoints.py:822` |
| `GET` | `/config_overrides/cyberark` | `/config_overrides/cyberark` | `proxy/management_endpoints/config_override_endpoints.py:764` |
| `POST` | `/config_overrides/cyberark` | `/config_overrides/cyberark` | `proxy/management_endpoints/config_override_endpoints.py:649` |
| `POST` | `/config_overrides/cyberark/test_connection` | `/config_overrides/cyberark/test_connection` | `proxy/management_endpoints/config_override_endpoints.py:886` |
| `DELETE` | `/config_overrides/hashicorp_vault` | `/config_overrides/hashicorp_vault` | `proxy/management_endpoints/config_override_endpoints.py:521` |
| `GET` | `/config_overrides/hashicorp_vault` | `/config_overrides/hashicorp_vault` | `proxy/management_endpoints/config_override_endpoints.py:461` |
| `POST` | `/config_overrides/hashicorp_vault` | `/config_overrides/hashicorp_vault` | `proxy/management_endpoints/config_override_endpoints.py:325` |
| `POST` | `/config_overrides/hashicorp_vault/test_connection` | `/config_overrides/hashicorp_vault/test_connection` | `proxy/management_endpoints/config_override_endpoints.py:588` |
| `POST` | `/reload/anthropic_beta_headers` | `/reload/anthropic_beta_headers` | `proxy/proxy_server.py:18407` |
| `POST` | `/reload/model_cost_map` | `/reload/model_cost_map` | `proxy/proxy_server.py:18154` |

## 请求

master key、SSO session，或 `key_type=management` / 具备对应角色的虚拟 Key。按 `user_api_key_auth`。

冻结字段（不得改名）：`config YAML / JSON: model_list`、`router_settings`、`litellm_settings`、`general_settings`。

### 请求头

| Header | 必填 | 说明 |
|---|---|---|
| `Authorization` | 是 | `Bearer` master key 或 session |
| `Content-Type` | JSON 时是 | `application/json` 或 multipart |

### 请求体

```json
{"general_settings": {"master_key": "os.environ/LITELLM_MASTER_KEY"}}
```

## 响应

冻结响应字段：`status`、`version`。

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

Console: /models-and-endpoints /router-settings。动作对照 [前端绑定](../../../../frontend/binding.md) 与 [页面规格](../../../../frontend/pages/README.md)。无控制台入口的路径仍须对 SDK/curl 可用。

## 验收

- 本表每一行 method+path 均已注册；未实现时返回 JSON 错误信封，不是 HTML。
- 鉴权失败与成功主路径合同测试。
- 标准客户端只改 base_url + api_key（数据面）。
