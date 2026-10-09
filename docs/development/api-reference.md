# HTTP 接口与处理器参考

[功能实现](implementation.md) · [权限](permissions.md) · [协议与运行边界](runtime.md)

## 认证、授权与错误约定

管理操作通常要求当前用户会话和对应动作权限；推理可以用当前会话或有效虚拟密钥。主密钥仅接受明确 bootstrap/应急动作。公开接口不等于公开修改权限。处理器是最终授权来源，表格列出实际入口，不把相同前缀下的全部接口推断为同一权限。

JSON 请求字段由处理器解析；模型定义使用 model_name/litellm_params/model_info，模板创建使用 name/body，模板绑定使用 scope/scope_id/route_template_id。非法 JSON 或字段返回请求错误，失效凭据返回认证错误，越权返回 403 或隐藏存在性的 404。依赖故障保留 5xx。429 使用 Retry-After；入口错误体保持对应协议。

x-litellm-call-id 关联调用与日志，x-litellm-response-cost 表示这次本地计算的美元费用，不是数据库 commit 回执。SSE 发出后无法改变 HTTP 状态，需查看日志失败证据。

## internal/gateway

[目录契约](../../internal/gateway/readme_cn.md)

| 方法与路径 | 处理器 | 注册源码 |
| --- | --- | --- |
| `POST /compliance/eu-ai-act` | `complianceEU` | [compliance.go](../../internal/gateway/compliance.go) |
| `POST /compliance/gdpr` | `complianceGDPR` | [compliance.go](../../internal/gateway/compliance.go) |
| `GET /health/liveliness` | `healthLive` | [routes.go](../../internal/gateway/routes.go) |
| `GET /health/liveness` | `healthLive` | [routes.go](../../internal/gateway/routes.go) |
| `GET /health/readiness` | `healthReady` | [routes.go](../../internal/gateway/routes.go) |
| `GET /health/readiness/details` | `healthDetails` | [routes.go](../../internal/gateway/routes.go) |
| `GET /health` | `healthReady` | [routes.go](../../internal/gateway/routes.go) |
| `GET /.well-known/litellm-ui-config` | `uiConfig` | [routes.go](../../internal/gateway/routes.go) |
| `GET /litellm/.well-known/litellm-ui-config` | `uiConfig` | [routes.go](../../internal/gateway/routes.go) |
| `POST /login` | `login` | [routes.go](../../internal/gateway/routes.go) |
| `POST /v2/login` | `login` | [routes.go](../../internal/gateway/routes.go) |
| `POST /v3/login` | `login` | [routes.go](../../internal/gateway/routes.go) |
| `POST /logout` | `logout` | [routes.go](../../internal/gateway/routes.go) |
| `POST /v2/logout` | `logout` | [routes.go](../../internal/gateway/routes.go) |
| `POST /v3/logout` | `logout` | [routes.go](../../internal/gateway/routes.go) |
| `GET /auth/me` | `me` | [routes.go](../../internal/gateway/routes.go) |
| `POST /auth/logout` | `logout` | [routes.go](../../internal/gateway/routes.go) |
| `POST /bootstrap` | `bootstrap` | [routes.go](../../internal/gateway/routes.go) |
| `GET /bootstrap/status` | `bootstrapStatus` | [routes.go](../../internal/gateway/routes.go) |
| `GET /authorize/flow` | `authorizeFlow` | [routes.go](../../internal/gateway/routes.go) |
| `POST /authorize/complete` | `authorizeComplete` | [routes.go](../../internal/gateway/routes.go) |
| `POST /v1/mcp/server/oauth/{server_id}/token` | `mcpOAuthToken` | [routes.go](../../internal/gateway/routes.go) |
| `GET /public/v1/model_hub` | `publicModelHub` | [routes.go](../../internal/gateway/routes.go) |
| `GET /public/v1/model_hub/{facet}` | `publicModelHubFacet` | [routes.go](../../internal/gateway/routes.go) |
| `GET /public/model_hub` | `publicModelHub` | [routes.go](../../internal/gateway/routes.go) |
| `GET /public/model_hub/info` | `publicModelHubInfo` | [routes.go](../../internal/gateway/routes.go) |
| `GET /public/endpoints` | `publicEndpoints` | [routes.go](../../internal/gateway/routes.go) |
| `GET /model_hub` | `publicModelHub` | [routes.go](../../internal/gateway/routes.go) |
| `GET /model_hub/{facet}` | `publicModelHubFacet` | [routes.go](../../internal/gateway/routes.go) |
| `POST /model_hub/update_useful_links` | `updateUsefulLinks` | [routes.go](../../internal/gateway/routes.go) |
| `POST /utils/token_counter` | `tokenCounter` | [routes.go](../../internal/gateway/routes.go) |
| `GET /utils/supported_openai_params` | `supportedOpenAIParams` | [routes.go](../../internal/gateway/routes.go) |
| `POST /utils/transform_request` | `transformRequest` | [routes.go](../../internal/gateway/routes.go) |
| `POST /v1/chat/completions` | `chat` | [routes.go](../../internal/gateway/routes.go) |
| `POST /chat/completions` | `chat` | [routes.go](../../internal/gateway/routes.go) |
| `POST /v1/embeddings` | `embeddings` | [routes.go](../../internal/gateway/routes.go) |
| `POST /embeddings` | `embeddings` | [routes.go](../../internal/gateway/routes.go) |
| `POST /v1/completions` | `completions` | [routes.go](../../internal/gateway/routes.go) |
| `POST /completions` | `completions` | [routes.go](../../internal/gateway/routes.go) |
| `POST /v1/messages` | `messages` | [routes.go](../../internal/gateway/routes.go) |
| `POST /v1/audio/translations` | `audioTranslations` | [routes.go](../../internal/gateway/routes.go) |
| `POST /audio/translations` | `audioTranslations` | [routes.go](../../internal/gateway/routes.go) |
| `POST /flushall` | `flushCache` | [routes.go](../../internal/gateway/routes.go) |
| `GET /cache/settings` | `cacheSettings` | [routes.go](../../internal/gateway/routes.go) |
| `POST /cache/settings` | `cacheSettings` | [routes.go](../../internal/gateway/routes.go) |
| `GET /get/ui_theme_settings` | `uiTheme` | [routes.go](../../internal/gateway/routes.go) |
| `PATCH /update/ui_theme_settings` | `uiTheme` | [routes.go](../../internal/gateway/routes.go) |
| `POST /upload/logo` | `uiTheme` | [routes.go](../../internal/gateway/routes.go) |
| `POST /prompts/test` | `promptTest` | [routes.go](../../internal/gateway/routes.go) |
| `POST /search_tools/test_connection` | `searchToolTest` | [routes.go](../../internal/gateway/routes.go) |
| `GET /cache/ping` | `cachePing` | [routes.go](../../internal/gateway/routes.go) |
| `GET /ping` | `cachePing` | [routes.go](../../internal/gateway/routes.go) |
| `GET /customer/list` | `customerList` | [routes.go](../../internal/gateway/routes.go) |
| `GET /end_user/list` | `customerList` | [routes.go](../../internal/gateway/routes.go) |

## internal/gateway/family

[目录契约](../../internal/gateway/family/readme_cn.md)

| 方法与路径 | 处理器 | 注册源码 |
| --- | --- | --- |
| `POST /v1/responses` | `Responses` | [mount.go](../../internal/gateway/family/mount.go) |
| `POST /responses` | `Responses` | [mount.go](../../internal/gateway/family/mount.go) |

## internal/gateway/guard

[目录契约](../../internal/gateway/guard/readme_cn.md)

| 方法与路径 | 处理器 | 注册源码 |
| --- | --- | --- |
| `POST /apply_guardrail` | `Apply` | [mount.go](../../internal/gateway/guard/mount.go) |
| `POST /guardrails/apply_guardrail` | `Apply` | [mount.go](../../internal/gateway/guard/mount.go) |

## internal/gateway/identity

[目录契约](../../internal/gateway/identity/readme_cn.md)

| 方法与路径 | 处理器 | 注册源码 |
| --- | --- | --- |
| `POST /user/new` | `UserNew` | [mount.go](../../internal/gateway/identity/mount.go) |
| `GET /user/list` | `UserList` | [mount.go](../../internal/gateway/identity/mount.go) |
| `GET /user/filter/ui` | `UserFilterUI` | [mount.go](../../internal/gateway/identity/mount.go) |
| `GET /user/available_users` | `AvailableUsers` | [mount.go](../../internal/gateway/identity/mount.go) |
| `GET /user/info` | `UserInfo` | [mount.go](../../internal/gateway/identity/mount.go) |
| `GET /v2/user/info` | `UserInfo` | [mount.go](../../internal/gateway/identity/mount.go) |
| `POST /user/update` | `UserUpdate` | [mount.go](../../internal/gateway/identity/mount.go) |
| `POST /user/delete` | `UserDelete` | [mount.go](../../internal/gateway/identity/mount.go) |
| `POST /user/set_password` | `UserSetPassword` | [mount.go](../../internal/gateway/identity/mount.go) |
| `POST /user/{user_id}/password` | `UserSetPassword` | [mount.go](../../internal/gateway/identity/mount.go) |
| `POST /organization/new` | `OrgNew` | [mount.go](../../internal/gateway/identity/mount.go) |
| `GET /organization/list` | `OrgList` | [mount.go](../../internal/gateway/identity/mount.go) |
| `GET /organization/info` | `OrgInfo` | [mount.go](../../internal/gateway/identity/mount.go) |
| `PATCH /organization/update` | `OrgUpdate` | [mount.go](../../internal/gateway/identity/mount.go) |
| `DELETE /organization/delete` | `OrgDelete` | [mount.go](../../internal/gateway/identity/mount.go) |
| `POST /organization/member_add` | `OrgMemberAdd` | [mount.go](../../internal/gateway/identity/mount.go) |
| `POST /organization/member_delete` | `OrgMemberRemove` | [mount.go](../../internal/gateway/identity/mount.go) |
| `DELETE /organization/member_delete` | `OrgMemberRemove` | [mount.go](../../internal/gateway/identity/mount.go) |
| `POST /team/new` | `TeamNew` | [mount.go](../../internal/gateway/identity/mount.go) |
| `GET /team/list` | `TeamList` | [mount.go](../../internal/gateway/identity/mount.go) |
| `GET /v2/team/list` | `TeamListV2` | [mount.go](../../internal/gateway/identity/mount.go) |
| `GET /team/available` | `TeamAvailable` | [mount.go](../../internal/gateway/identity/mount.go) |
| `GET /team/info` | `TeamInfo` | [mount.go](../../internal/gateway/identity/mount.go) |
| `POST /team/update` | `TeamUpdate` | [mount.go](../../internal/gateway/identity/mount.go) |
| `POST /team/delete` | `TeamDelete` | [mount.go](../../internal/gateway/identity/mount.go) |
| `POST /team/move` | `TeamMove` | [mount.go](../../internal/gateway/identity/mount.go) |
| `GET /team/models` | `TeamModels` | [mount.go](../../internal/gateway/identity/mount.go) |
| `GET /team/member_list` | `TeamMembers` | [mount.go](../../internal/gateway/identity/mount.go) |
| `POST /team/member_add` | `TeamMemberAdd` | [mount.go](../../internal/gateway/identity/mount.go) |
| `POST /team/member_update` | `TeamMemberUpdate` | [mount.go](../../internal/gateway/identity/mount.go) |
| `POST /team/member_delete` | `TeamMemberRemove` | [mount.go](../../internal/gateway/identity/mount.go) |
| `POST /project/new` | `ProjectNew` | [mount.go](../../internal/gateway/identity/mount.go) |
| `GET /project/list` | `ProjectList` | [mount.go](../../internal/gateway/identity/mount.go) |
| `POST /project/list` | `ProjectList` | [mount.go](../../internal/gateway/identity/mount.go) |
| `GET /project/info` | `ProjectInfo` | [mount.go](../../internal/gateway/identity/mount.go) |
| `POST /project/info` | `ProjectInfo` | [mount.go](../../internal/gateway/identity/mount.go) |
| `POST /project/update` | `ProjectUpdate` | [mount.go](../../internal/gateway/identity/mount.go) |
| `POST /project/delete` | `ProjectDelete` | [mount.go](../../internal/gateway/identity/mount.go) |
| `GET /route_template/list` | `RouteTemplateList` | [mount.go](../../internal/gateway/identity/mount.go) |
| `POST /route_template/new` | `RouteTemplateCreate` | [mount.go](../../internal/gateway/identity/mount.go) |
| `GET /route_template/{template_id}` | `RouteTemplateGet` | [mount.go](../../internal/gateway/identity/mount.go) |
| `POST /route_template/{template_id}/update` | `RouteTemplateUpdate` | [mount.go](../../internal/gateway/identity/mount.go) |
| `POST /route_template/{template_id}/delete` | `RouteTemplateDelete` | [mount.go](../../internal/gateway/identity/mount.go) |
| `GET /route_template/{template_id}/usage` | `RouteTemplateUsage` | [mount.go](../../internal/gateway/identity/mount.go) |
| `GET /route_template/binding` | `RouteTemplateBinding` | [mount.go](../../internal/gateway/identity/mount.go) |
| `POST /route_template/binding` | `RouteTemplateBinding` | [mount.go](../../internal/gateway/identity/mount.go) |
| `GET /audit/logs` | `AuditLog` | [mount.go](../../internal/gateway/identity/mount.go) |

## internal/gateway/keys

[目录契约](../../internal/gateway/keys/readme_cn.md)

| 方法与路径 | 处理器 | 注册源码 |
| --- | --- | --- |
| `POST /key/generate` | `Generate` | [mount.go](../../internal/gateway/keys/mount.go) |
| `POST /key/service-account/generate` | `ServiceAccount` | [mount.go](../../internal/gateway/keys/mount.go) |
| `GET /key/list` | `List` | [mount.go](../../internal/gateway/keys/mount.go) |
| `GET /key/info` | `Info` | [mount.go](../../internal/gateway/keys/mount.go) |
| `POST /v2/key/info` | `Info` | [mount.go](../../internal/gateway/keys/mount.go) |
| `POST /key/delete` | `Delete` | [mount.go](../../internal/gateway/keys/mount.go) |
| `POST /key/block` | `Block` | [mount.go](../../internal/gateway/keys/mount.go) |
| `POST /key/unblock` | `Unblock` | [mount.go](../../internal/gateway/keys/mount.go) |
| `POST /key/update` | `Update` | [mount.go](../../internal/gateway/keys/mount.go) |
| `POST /key/bulk_update` | `BulkUpdate` | [mount.go](../../internal/gateway/keys/mount.go) |
| `POST /key/regenerate` | `Regenerate` | [mount.go](../../internal/gateway/keys/mount.go) |
| `POST /key/{key}/regenerate` | `Regenerate` | [mount.go](../../internal/gateway/keys/mount.go) |
| `POST /key/{key}/reset_spend` | `ResetSpend` | [mount.go](../../internal/gateway/keys/mount.go) |
| `GET /key/aliases` | `Aliases` | [mount.go](../../internal/gateway/keys/mount.go) |
| `POST /key/health` | `Health` | [mount.go](../../internal/gateway/keys/mount.go) |

## internal/gateway/models

[目录契约](../../internal/gateway/models/readme_cn.md)

| 方法与路径 | 处理器 | 注册源码 |
| --- | --- | --- |
| `GET /v1/models` | `List` | [mount.go](../../internal/gateway/models/mount.go) |
| `GET /models` | `List` | [mount.go](../../internal/gateway/models/mount.go) |
| `GET /model/available` | `Available` | [mount.go](../../internal/gateway/models/mount.go) |
| `GET /v2/model/info` | `Info` | [mount.go](../../internal/gateway/models/mount.go) |
| `GET /model/cost_map/source` | `CostMapSource` | [mount.go](../../internal/gateway/models/mount.go) |
| `POST /reload/model_cost_map` | `ReloadCostMap` | [mount.go](../../internal/gateway/models/mount.go) |
| `POST /schedule/model_cost_map_reload` | `ScheduleCostMapReload` | [mount.go](../../internal/gateway/models/mount.go) |
| `DELETE /schedule/model_cost_map_reload` | `CancelCostMapReload` | [mount.go](../../internal/gateway/models/mount.go) |
| `GET /schedule/model_cost_map_reload/status` | `CostMapReloadStatus` | [mount.go](../../internal/gateway/models/mount.go) |
| `GET /price/catalog` | `PriceList` | [mount.go](../../internal/gateway/models/mount.go) |
| `POST /price/model` | `UpsertPriceModel` | [mount.go](../../internal/gateway/models/mount.go) |
| `DELETE /price/model` | `DeletePriceModel` | [mount.go](../../internal/gateway/models/mount.go) |
| `POST /price/model/reset` | `ResetPriceModel` | [mount.go](../../internal/gateway/models/mount.go) |
| `POST /price/provider` | `UpsertPriceProvider` | [mount.go](../../internal/gateway/models/mount.go) |
| `DELETE /price/provider` | `DeletePriceProvider` | [mount.go](../../internal/gateway/models/mount.go) |
| `POST /model/new` | `New` | [mount.go](../../internal/gateway/models/mount.go) |
| `POST /model/builtin/refresh` | `RefreshBuiltin` | [mount.go](../../internal/gateway/models/mount.go) |
| `POST /model/builtin/models` | `ListBuiltin` | [mount.go](../../internal/gateway/models/mount.go) |
| `POST /model/builtin/add` | `AddBuiltinModels` | [mount.go](../../internal/gateway/models/mount.go) |
| `POST /model/update` | `Update` | [mount.go](../../internal/gateway/models/mount.go) |
| `PATCH /model/{model_id}/update` | `Update` | [mount.go](../../internal/gateway/models/mount.go) |
| `POST /model/delete` | `Delete` | [mount.go](../../internal/gateway/models/mount.go) |
| `POST /model/disable` | `Disable` | [mount.go](../../internal/gateway/models/mount.go) |
| `POST /model/enable` | `Enable` | [mount.go](../../internal/gateway/models/mount.go) |
| `GET /model_group/info` | `GroupInfo` | [mount.go](../../internal/gateway/models/mount.go) |

## internal/gateway/prefs

[目录契约](../../internal/gateway/prefs/readme_cn.md)

| 方法与路径 | 处理器 | 注册源码 |
| --- | --- | --- |
| `GET /router/settings` | `Page` | [mount.go](../../internal/gateway/prefs/mount.go) |
| `GET /router/fields` | `Page` | [mount.go](../../internal/gateway/prefs/mount.go) |
| `GET /get/config/callbacks` | `Callbacks` | [mount.go](../../internal/gateway/prefs/mount.go) |
| `GET /config/list` | `List` | [mount.go](../../internal/gateway/prefs/mount.go) |
| `POST /config/update` | `Update` | [mount.go](../../internal/gateway/prefs/mount.go) |
| `POST /config/field/update` | `FieldUpdate` | [mount.go](../../internal/gateway/prefs/mount.go) |
| `POST /config/field/delete` | `FieldDelete` | [mount.go](../../internal/gateway/prefs/mount.go) |

## internal/gateway/usage

[目录契约](../../internal/gateway/usage/readme_cn.md)

| 方法与路径 | 处理器 | 注册源码 |
| --- | --- | --- |
| `GET /global/spend/teams` | `SpendTeams` | [mount.go](../../internal/gateway/usage/mount.go) |
| `GET /spend/logs/v2` | `LogsV2` | [mount.go](../../internal/gateway/usage/mount.go) |
| `GET /spend/logs/ui` | `LogsV2` | [mount.go](../../internal/gateway/usage/mount.go) |
| `GET /spend/logs/ui/{request_id}` | `LogByID` | [mount.go](../../internal/gateway/usage/mount.go) |
| `GET /spend/logs/session/ui` | `SessionLogs` | [mount.go](../../internal/gateway/usage/mount.go) |
| `GET /global/spend/logs` | `SpendLogs` | [mount.go](../../internal/gateway/usage/mount.go) |
| `GET /global/spend/keys` | `SpendKeys` | [mount.go](../../internal/gateway/usage/mount.go) |
| `GET /global/spend/models` | `SpendModels` | [mount.go](../../internal/gateway/usage/mount.go) |
| `GET /global/spend/provider` | `SpendProvider` | [mount.go](../../internal/gateway/usage/mount.go) |
| `POST /global/spend/end_users` | `SpendEndUsers` | [mount.go](../../internal/gateway/usage/mount.go) |
| `GET /user/daily/activity` | `UserDailyActivity` | [mount.go](../../internal/gateway/usage/mount.go) |
| `GET /user/daily/activity/aggregated` | `UserDailyActivityAggregated` | [mount.go](../../internal/gateway/usage/mount.go) |
| `GET /team/daily/activity` | `TeamDailyActivity` | [mount.go](../../internal/gateway/usage/mount.go) |
| `GET /team/daily/activity/aggregated` | `TeamDailyActivityAggregated` | [mount.go](../../internal/gateway/usage/mount.go) |
| `GET /team/spend/by_user` | `TeamSpendByUser` | [mount.go](../../internal/gateway/usage/mount.go) |
| `GET /organization/daily/activity` | `OrganizationDailyActivity` | [mount.go](../../internal/gateway/usage/mount.go) |
| `GET /gateway/daily/activity` | `GatewayDailyActivity` | [mount.go](../../internal/gateway/usage/mount.go) |
| `GET /global/activity` | `Activity` | [mount.go](../../internal/gateway/usage/mount.go) |
| `GET /global/activity/model` | `ActivityModel` | [mount.go](../../internal/gateway/usage/mount.go) |
| `GET /global/activity/cache_hits` | `ActivityCacheHits` | [mount.go](../../internal/gateway/usage/mount.go) |
| `POST /spend/calculate` | `Calculate` | [mount.go](../../internal/gateway/usage/mount.go) |
| `GET /spend/keys` | `Keys` | [mount.go](../../internal/gateway/usage/mount.go) |
| `GET /spend/users` | `Users` | [mount.go](../../internal/gateway/usage/mount.go) |
| `GET /tag/list` | `TagList` | [mount.go](../../internal/gateway/usage/mount.go) |
| `GET /spend/tags` | `Tags` | [mount.go](../../internal/gateway/usage/mount.go) |
| `GET /global/spend/tags` | `SpendTags` | [mount.go](../../internal/gateway/usage/mount.go) |
| `GET /global/spend/all_tag_names` | `SpendTagNames` | [mount.go](../../internal/gateway/usage/mount.go) |
| `POST /health/test_connection` | `HealthTestConnection` | [mount.go](../../internal/gateway/usage/mount.go) |
| `GET /health/services` | `HealthServices` | [mount.go](../../internal/gateway/usage/mount.go) |
| `GET /test` | `HealthTest` | [mount.go](../../internal/gateway/usage/mount.go) |

## 目录调度与官方传输

上表覆盖 Go 源码直接注册的字面量入口。catalog/routes.json 另有兼容协议调度；provider 注册 transport 另有固定官方路径。它们应与 handler 和端点能力一起阅读：

- [catalog 路由目录](../../internal/catalog/routes.json)：请求分类/兼容入口；部分明确不支持，需核对 family 和 removed handlers。
- [接线测试基线](../testdata/catalog.json)：测试登记表面积，不能作为供应商功能承诺。
- [供应商传输](../../internal/provider/readme_cn.md)：官方 create/get/list 方法和路径映射。

## 核心管理请求示例

以下为字段示意，身份 token 与资源 ID 由调用环境提供；示例不含真实凭据。

```json
{"model_name":"public-chat","litellm_params":{"model":"provider-model","litellm_credential_name":"provider-account","custom_llm_provider":"openai"},"model_info":{"endpoint_types":["chat"],"pricing_source":"catalog","base_model":"catalog-model"}}
```

目录模型必须确实存在且有价格；manual 使用显式 rates 或扁平单价，0 表示明确零价。公开名和上游名不能为空。

```json
{"name":"team-policy","body":{"routing_strategy":"simple-shuffle","num_retries":2,"timeout":60,"allowed_fails":3,"cooldown_time":60}}
```

```json
{"scope":"team","scope_id":"team-id","route_template_id":"template-id"}
```

route_template_id 空字符串恢复继承。模板更新整份替换；高级字段是否执行见[路由规则](routing.md)。API 类型以当前后端和生成 schema 为依据，变更后运行前端 gen:api。
