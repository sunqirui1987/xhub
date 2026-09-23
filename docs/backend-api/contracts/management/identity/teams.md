# Teams

- Family: `mgmt.teams`
- Status: `specified`
- 参考: 下表定位列
- Console: /teams
- Auth: management

## 目的

本族是网关下列 HTTP 路径的完整分母。字段名与默认值以冻结契约及 该路由 handler 的请求体模型与上游 SDK 透传字段 为准，网关不得改名。

## HTTP 表面

实现必须注册每一行 method+path。规范化列用于覆盖校验（去掉 `:path` 与尾斜杠）。

| Method | Path | 规范化 path | 参考 |
|---|---|---|---|
| `GET` | `/team/available` | `/team/available` | `proxy/management_endpoints/team_endpoints.py:4808` |
| `POST` | `/team/block` | `/team/block` | `proxy/management_endpoints/team_endpoints.py:4686` |
| `POST` | `/team/bulk_member_add` | `/team/bulk_member_add` | `proxy/management_endpoints/team_endpoints.py:3792` |
| `GET` | `/team/daily/activity` | `/team/daily/activity` | `proxy/management_endpoints/team_endpoints.py:6182` |
| `GET` | `/team/daily/activity/aggregated` | `/team/daily/activity/aggregated` | `proxy/management_endpoints/team_endpoints.py:6268` |
| `POST` | `/team/delete` | `/team/delete` | `proxy/management_endpoints/team_endpoints.py:3938` |
| `GET` | `/team/filter/ui` | `/team/filter/ui` | `proxy/management_endpoints/team_endpoints.py:5528` |
| `GET` | `/team/info` | `/team/info` | `proxy/management_endpoints/team_endpoints.py:4418` |
| `POST` | `/team/key/bulk_update` | `/team/key/bulk_update` | `proxy/management_endpoints/key_management_endpoints.py:3397` |
| `GET` | `/team/list` | `/team/list` | `proxy/management_endpoints/team_endpoints.py:5417` |
| `POST` | `/team/member_add` | `/team/member_add` | `proxy/management_endpoints/team_endpoints.py:3113` |
| `POST` | `/team/member_delete` | `/team/member_delete` | `proxy/management_endpoints/team_endpoints.py:3283` |
| `POST` | `/team/member_update` | `/team/member_update` | `proxy/management_endpoints/team_endpoints.py:3476` |
| `GET` | `/team/metadata_schema` | `/team/metadata_schema` | `proxy/management_endpoints/team_endpoints.py:4790` |
| `POST` | `/team/model/add` | `/team/model/add` | `proxy/management_endpoints/team_endpoints.py:5611` |
| `POST` | `/team/model/delete` | `/team/model/delete` | `proxy/management_endpoints/team_endpoints.py:5716` |
| `POST` | `/team/new` | `/team/new` | `proxy/management_endpoints/team_endpoints.py:1184` |
| `POST` | `/team/permissions_bulk_update` | `/team/permissions_bulk_update` | `proxy/management_endpoints/team_endpoints.py:5936` |
| `GET` | `/team/permissions_list` | `/team/permissions_list` | `proxy/management_endpoints/team_endpoints.py:5803` |
| `POST` | `/team/permissions_update` | `/team/permissions_update` | `proxy/management_endpoints/team_endpoints.py:5870` |
| `GET` | `/team/spend/by_user` | `/team/spend/by_user` | `proxy/management_endpoints/team_endpoints.py:6380` |
| `GET` | `/team/spend/report` | `/team/spend/report` | `proxy/spend_tracking/spend_management_endpoints.py:1884` |
| `POST` | `/team/unblock` | `/team/unblock` | `proxy/management_endpoints/team_endpoints.py:4741` |
| `POST` | `/team/update` | `/team/update` | `proxy/management_endpoints/team_endpoints.py:1940` |
| `GET` | `/team/{team_id:path}/callback` | `/team/{team_id}/callback` | `proxy/management_endpoints/team_callback_endpoints.py:718` |
| `POST` | `/team/{team_id:path}/callback` | `/team/{team_id}/callback` | `proxy/management_endpoints/team_callback_endpoints.py:253` |
| `DELETE` | `/team/{team_id:path}/callback/{callback_name}` | `/team/{team_id}/callback/{callback_name}` | `proxy/management_endpoints/team_callback_endpoints.py:434` |
| `PATCH` | `/team/{team_id}` | `/team/{team_id}` | `proxy/management_endpoints/team_endpoints.py:2369` |
| `POST` | `/team/{team_id}/disable_logging` | `/team/{team_id}/disable_logging` | `proxy/management_endpoints/team_callback_endpoints.py:579` |
| `POST` | `/team/{team_id}/member/{user_id}/reset_spend` | `/team/{team_id}/member/{user_id}/reset_spend` | `proxy/management_endpoints/team_endpoints.py:3680` |
| `GET` | `/team/{team_id}/members/me` | `/team/{team_id}/members/me` | `proxy/management_endpoints/team_endpoints.py:4572` |
| `GET` | `/v2/team/list` | `/v2/team/list` | `proxy/management_endpoints/team_endpoints.py:5151` |

## 请求

master key、SSO session，或 `key_type=management` / 具备对应角色的虚拟 Key。按 `user_api_key_auth`。

冻结字段（不得改名）：`team_alias`、`organization_id`、`models`、`max_budget`、`tpm_limit`、`rpm_limit`、`members_with_roles`、`guardrails`、`object_permission`、`metadata`、`blocked`。

### 请求头

| Header | 必填 | 说明 |
|---|---|---|
| `Authorization` | 是 | `Bearer` master key 或 session |
| `Content-Type` | JSON 时是 | `application/json` 或 multipart |

### 请求体

```json
{"team_alias": "platform", "models": ["gpt-4o-mini"], "max_budget": 100, "members_with_roles": [{"user_id": "user_01", "role": "admin"}]}
```

## 响应

冻结响应字段：`team_id`、`team_alias`、`organization_id`、`models`、`spend`、`max_budget`、`members_with_roles`。

### 响应头

| Header | 说明 |
|---|---|
| `x-litellm-call-id` | 见 [错误与响应头](../../../architecture/errors-headers.md) |
| `Content-Type` | 见 [错误与响应头](../../../architecture/errors-headers.md) |
| `Retry-After` | 429 时 |

### 响应体

```json
{"team_id": "team_01", "team_alias": "platform", "spend": 0, "max_budget": 100}
```

## 错误

OpenAI 兼容路径使用 `{"error":{"message","type","code","param"}}`。Anthropic 原生用 Anthropic 错误形状。Gemini 用 Google 错误形状。鉴权失败 401，模型不允许 401/403，预算/RPM/TPM 429 + `Retry-After`，参数 400。未实现 Provider 明确错误。共性见 [错误与响应头](../../../architecture/errors-headers.md)。

## 副作用

管理写操作记 AuditLog 并失效 Auth cache。测试调用（如 `/prompts/test`、`/health/test_connection`）仍记 spend。

## 控制台绑定

Console: /teams。动作对照 [前端绑定](../../../../frontend/binding.md) 与 [页面规格](../../../../frontend/pages/README.md)。无控制台入口的路径仍须对 SDK/curl 可用。

## 验收

- 本表每一行 method+path 均已注册；未实现时返回 JSON 错误信封，不是 HTML。
- 鉴权失败与成功主路径合同测试。
- 标准客户端只改 base_url + api_key（数据面）。
