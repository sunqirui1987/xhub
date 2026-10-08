# HTTP 网关与模块组合

[English](readme.md) · [全功能实现说明](../../docs/development/implementation.md)

## 职责与实现契约

server.go 的 New 组合配置、框架 Store、IAM、Redis、缓存、hooks、插件与授权器。routes.go 按 health/session/keys/models/tokens/ingress/access/identity/usage/prefs/guard/compliance/family 顺序登记内置模块；wire.go 实现子模块 Host，子模块不反向导入父包。
Handle 接受 METHOD /path，重复路径保留第一次登记；Use 拒绝 nil、空名和重复模块名，服务就绪后立即挂载新模块。ModuleNames 返回副本。engine.go 和 catalog.go 负责 Gin 与目录接线；未知入口返回 JSON 404，不能意外回退成首页或通用成功。
session/onboarding 处理登录与一次性 bootstrap；limits 和 route_settings 建立模型、预算与模板链；ingress/bypass 委托 dataplane；spend 保存费用和日志。控制台管理写操作依赖会话角色，主密钥仅用于明确 bootstrap 边界。新增路由必须同时定义授权、输入校验、真实业务行为及测试，目录中存在路径不等于实现完成。

## 子目录与协作边界

| 目录 | 职责 |
| --- | --- |
| [family](family/readme_cn.md) | API 家族处理与资源隔离 |
| [guard](guard/readme_cn.md) | 正文护栏 |
| [identity](identity/readme_cn.md) | 身份资源、成员与模板管理 |
| [keys](keys/readme_cn.md) | 虚拟密钥与服务密钥生命周期 |
| [models](models/readme_cn.md) | 模型部署、能力与价格管理 |
| [prefs](prefs/readme_cn.md) | 平台设置与请求级路由文档 |
| [templateauth](templateauth/readme_cn.md) | 模板选择授权共享边界 |
| [usage](usage/readme_cn.md) | 用量日志、活动与费用报表 |

## 源码职责与入口

### access.go

内部实现和协议边界见 [access.go](access.go)。

### affinity.go

- [`func (s *Server) PlanRoute(r *http.Request, alias string, body map[string]any, p *auth.Principal) dataplane.RoutePlan`](affinity.go) — PlanRoute resolves the session and any deployment already pinned to it. previous_response_id wins, then an explicit session, then a stable prompt prefix.
- [`func (s *Server) CommitRoute(plan dataplane.RoutePlan, deploymentID, responseID string)`](affinity.go) — CommitRoute remembers which deployment served this session and, when the response has an id, which deployment produced it.
- [`func (s *Server) PinOfficial(taskID, deploymentID string)`](affinity.go) — 把官方任务 id 钉到创建时的部署，有效期七天。
- [`func (s *Server) OfficialDeployment(taskID string) string`](affinity.go) — 按官方任务 id 取回创建时的部署。没有钉时为空串。
- [`func (s *Server) OfficialBilled(taskID string) bool`](affinity.go) — 判断这个官方任务是否已经记过用量。
- [`func (s *Server) MarkOfficialBilled(taskID string)`](affinity.go) — 标记这个官方任务已经记过用量，避免创建和后续查询重复扣费。

### bypass.go

内部实现和协议边界见 [bypass.go](bypass.go)。

### catalog.go

内部实现和协议边界见 [catalog.go](catalog.go)。

### codec.go

内部实现和协议边界见 [codec.go](codec.go)。

### compliance.go

内部实现和协议边界见 [compliance.go](compliance.go)。

### doc.go

内部实现和协议边界见 [doc.go](doc.go)。

### engine.go

- [`func (s *Server) GinRoutes() gin.RoutesInfo`](engine.go) — GinRoutes returns the methods, paths, and handlers mounted on the engine, so a caller can confirm there is no "/" catch-all.
- [`func (s *Server) Handler() http.Handler`](engine.go) — Handler is the process HTTP entry. Tests mount this, not the Gin engine. Bypass that returns true has already written the response. Gin then either streams straight through or buffers a non-stream response so the idempotency key can replay it.

### health.go

- [`func PublicOrigin() string`](health.go) — PublicOrigin is the OpenAI-compatible API origin. XHUB_PUBLIC_ORIGIN overrides the local default. The value is an origin only: the console is not mounted under it.

### ingress.go

内部实现和协议边界见 [ingress.go](ingress.go)。

### limits.go

内部实现和协议边界见 [limits.go](limits.go)。

### onboarding.go

内部实现和协议边界见 [onboarding.go](onboarding.go)。

### public_hub.go

内部实现和协议边界见 [public_hub.go](public_hub.go)。

### removed.go

- [`func IsRemovedColumn(path string) bool`](removed.go) — 判断这条路径是不是已经从产品里删除的控制台列。
- [`func IsRetiredPath(path string) bool`](removed.go) — IsRetiredPath reports whether a path belongs to the catalog's retired class. These are the "mixed" routes the catalog still lists but the product removed. They are answered with 410 rather than served , so a test that walks the route inventory has to expect that and not a 200. It asks the catalog for the classification rather than repeating the prefix list, so a path cannot be retired in one placeand served in the other.

### route_settings.go

- [`func (s *Server) RouteSettingsFor(p *auth.Principal) prefs.RouteSettings`](route_settings.go) — RouteSettingsFor resolves the router settings one request runs under. The narrow scope is already known: keyBudgetOK walked key, team and organization for the budget chain and recorded both the first scope that names a template and which scope that was. So this does not walk anything - it reads the one template that walk already chose, or the platform document when none was chosen. There is deliberately no process-wide cache. A cache would be fastest to build around the template row, and it would also be the thing that hands a stale template to the requests immediately after an operator edits one - the exact moment they are looking at the console to check that the edit took. One primary-key read per inference request is the cheaper mistake.

### routes.go

- [`func (s *Server) Handle(pattern string, h http.HandlerFunc)`](routes.go) — Handle implements httpx.Registrar. pattern is "METHOD /path". A pattern already mounted is not replaced by a later module.
- [`func (s *Server) Use(m httpx.Module) error`](routes.go) — Use mounts a module by name. Mounting the same name again fails, and paths already mounted stay as they are. If the process is already serving, the new module is mounted immediately. During New it isrecorded and mounted later with the others.
- [`func (s *Server) ModuleNames() []string`](routes.go) — ModuleNames returns a copy of the module names in mount order.

### server.go

公开类型：`Server`.

- [`func New(cfg *config.Config, st *store.Store, db *iam.DB) *Server`](server.go) — New assembles the gateway. It loads the catalog, merges router settings, and registers dedicated routes plus the remaining catalog routes. The identity store is required: a gateway without one could not tell who is calling.
- [`func (s *Server) Run(addr string) error`](server.go) — Run accepts connections on addr. The process entry uses it instead of handing a ServeMux to ListenAndServe.

### session.go

内部实现和协议边界见 [session.go](session.go)。

### spend.go

内部实现和协议边界见 [spend.go](spend.go)。

### theme_settings.go

内部实现和协议边界见 [theme_settings.go](theme_settings.go)。

### tokens.go

内部实现和协议边界见 [tokens.go](tokens.go)。

### wire.go

- [`func (s *Server) RequireLLMPrincipal(w http.ResponseWriter, r *http.Request) *auth.Principal`](wire.go) — RequireLLMPrincipal resolves an identity that may call inference. On failure it has already written the response and returns nil.
- [`func (s *Server) ResolveRequest(r *http.Request) (*auth.Principal, error)`](wire.go) — ResolveRequest resolves the request identity again so a retry can confirm the key is still valid.
- [`func (s *Server) GatewayConfig() *config.Config`](wire.go) — GatewayConfig 返回当前进程配置。空模型表表示没有部署，不是「不做限制」。
- [`func (s *Server) HTTPClient() *http.Client`](wire.go) — HTTPClient returns the HTTP client used for upstream calls. The timeout comes from the router settings.
- [`func (s *Server) ResponseCache() *cache.DualCache`](wire.go) — ResponseCache returns the in-process cache for non-streaming responses.
- [`func (s *Server) HookEngine() *hooks.Engine`](wire.go) — HookEngine returns the budget and concurrency gate.
- [`func (s *Server) Extensions() *plugin.Registry`](wire.go) — Extensions is the empty extension registry created at process start. Tests and startup code register implementations here, and the data plane calls them in registration order.
- [`func (s *Server) RouteState() router.State`](wire.go) — RouteState returns concurrency, cooldown, latency, and usage for the router. Without Redis only in-process Busy is present.
- [`func (s *Server) RouterDocument() map[string]any`](wire.go) — RouterDocument returns the merged router settings. A database key overrides YAML.
- [`func (s *Server) GuardrailBlocks(callID string, body map[string]any) (bool, string)`](wire.go) — GuardrailBlocks runs guardrails before a chat request is sent. When blocked is true, msg is the reason shown to the caller. The findings are kept against callID until the spend row is written.
- [`func (s *Server) AttachCredential(dep config.ModelEntry) (config.ModelEntry, error)`](wire.go) — AttachCredential 按部署上的凭据名从库里补上 api_key 和 api_base。
- [`func (s *Server) IncBusy(id string)`](wire.go) — IncBusy increments the in-process concurrency count for a deployment.
- [`func (s *Server) DecBusy(id string)`](wire.go) — DecBusy decrements the in-process concurrency count for a deployment. The caller must pair it with IncBusy so the count is not driven below what that caller added.
- [`func (s *Server) NoteFailure(id string, settings prefs.RouteSettings)`](wire.go) — NoteFailure records a deployment failure. An allowed_fails below 1 does not write Redis.
- [`func (s *Server) NoteLatency(id string, ms float64)`](wire.go) — NoteLatency records the milliseconds of one successful call. Zero is recorded too, and it means the call finished.
- [`func (s *Server) SetChatHeaders(w http.ResponseWriter, p *auth.Principal, alias, apiBase string)`](wire.go) — SetChatHeaders writes response headers such as the model name, spend, and call ID.
- [`func (s *Server) RecordSpend(w http.ResponseWriter, p *auth.Principal, callID, alias, op string, usage map[string]any, start time.Time, cacheHit bool, status int, depID string)`](wire.go) — RecordSpend records this call's tokens on the hot path and, when Redis is configured, queues the log instead of writing PostgreSQL immediately.
- [`func (s *Server) RememberExchange(callID string, r *http.Request, reqBody, respBody []byte)`](wire.go) — RememberExchange holds the request and response until this call's spend row is written.
- [`func (s *Server) PinnedDeployment(taskID string) string`](wire.go) — 按聊天响应 id 取回钉住的部署。这是对话粘滞，不是官方任务钉。
- [`func (s *Server) FindDeployment(id string) (config.ModelEntry, bool)`](wire.go) — 按部署 id 在当前模型表里查找。找不到时 ok 为假。
- [`func (s *Server) AnnotateCall(callID string, note dataplane.CallNote)`](wire.go) — 记下这次调用的首字时间、供应商、缓存和部署，等记用量时取走。
- [`func (s *Server) WriteCacheHit(w http.ResponseWriter, p *auth.Principal, callID, alias, ck, op string, hit []byte, start time.Time)`](wire.go) — WriteCacheHit writes a cached body back and records a cache-hit spend row with a zero delta.
- [`func (s *Server) WriteChatJSON(w http.ResponseWriter, p *auth.Principal, callID, alias, ck, op, provider string, respBody []byte, status int, start time.Time, depID string)`](wire.go) — WriteChatJSON writes the upstream JSON to the caller and records spend. A non-success status is not cached as a successful body.
- [`func (s *Server) EnforceIdentityLimits(w http.ResponseWriter, path string, p *auth.Principal, alias string, est int) bool`](wire.go) — EnforceIdentityLimits checks the model allow-list, budget, and RPM or TPM. On rejection it has already written the response.
- [`func (s *Server) Redis() *live.Client`](wire.go) — Redis returns the hot-path Redis client. It is nil when Redis is not configured, and spend then uses the synchronous database path.
- [`func (s *Server) Models() []config.ModelEntry`](wire.go) — Models returns the deployments loaded into this process. It is the model table, not an allow-list of names.
- [`func (s *Server) BusyMap() map[string]int`](wire.go) — BusyMap returns how many requests each deployment is handling inside this process.
- [`func (s *Server) FlushSpend()`](wire.go) — FlushSpend writes spend and logs that are still in Redis into PostgreSQL.
- [`func (s *Server) RequireUser(w http.ResponseWriter, r *http.Request) *auth.Principal`](wire.go) — RequireUser accepts any signed-in session or virtual key. On failure it has already written 401.
- [`func (s *Server) RequireManage(w http.ResponseWriter, r *http.Request) *auth.Principal`](wire.go) — RequireManage requires a management identity. On failure it has already written 401 and returns nil.
- [`func (s *Server) ModelList() []config.ModelEntry`](wire.go) — ModelList returns a copy of the current model list. The lock is held while copying, and the caller may read the result freely afterward.
- [`func (s *Server) ModelPublic(m config.ModelEntry) map[string]any`](wire.go) — ModelPublic is the public JSON for a model. Secrets inside the parameters are masked.
- [`func (s *Server) LockModels()`](wire.go) — LockModels locks the model table. Pair it with UnlockModels. The lock also covers database writes during an update.
- [`func (s *Server) UnlockModels()`](wire.go) — UnlockModels releases the model-table lock.
- [`func (s *Server) ModelTable() *[]config.ModelEntry`](wire.go) — ModelTable returns a pointer to the in-process model slice. On a request path the caller must LockModels first.
- [`func (s *Server) Resolve(r *http.Request) (*auth.Principal, error)`](wire.go) — Resolve resolves the identity of the current request. On failure it does not write a response.
- [`func (s *Server) AllowLLM(p *auth.Principal) bool`](wire.go) — AllowLLM reports whether this identity may call inference.
- [`func (s *Server) RequireLLM(w http.ResponseWriter, r *http.Request) *auth.Principal`](wire.go) — RequireLLM requires an identity that may call inference. The master key may not by default. On failure it has already written 401.
- [`func (s *Server) RequireMixed(w http.ResponseWriter, r *http.Request) *auth.Principal`](wire.go) — RequireMixed accepts either a management identity or an inference identity. If it is neither, it has already written 401.
- [`func (s *Server) Identity() *iam.DB`](wire.go) — Identity returns the identity store. A handler reads its rows from here, but only after Authorize has permitted the action that reads them.
- [`func (s *Server) RecordStore() *store.Store`](wire.go) — RecordStore returns the framework record store: proxy models, the price-map reload plan, provider credentials and general settings. It answers no authorization question, and every identity read goes through Identity instead.
- [`func (s *Server) Authorize(r *http.Request, p *auth.Principal, action authz.Action, obj authz.Object) error`](wire.go) — Authorize decides one action for an already resolved caller and returns nil when it is permitted. It is the only way a handler may reach identity data: the object names what is being acted on and theguard reads its real ownership from the database, so a handler cannot pass a team the row does not belong to. A refusal is returned rather than written, because a handler that makes several decisionsneeds to know which one failed. WriteAuthz turns it into the response.
- [`func (s *Server) KeysScope(r *http.Request, p *auth.Principal) (*authz.Scope, error)`](wire.go) — KeysScope returns the key-listing scope for an already resolved caller. The listing is narrowed in SQL by the scope rather than filtered after the rows are read, so a large table never widens what a handler returns.
- [`func (s *Server) UsageScope(r *http.Request, p *auth.Principal, teamID string) (*authz.Scope, error)`](wire.go) — UsageScope returns the usage-listing scope for an already resolved caller, optionally narrowed to one team. The scope is applied as a WHERE fragment, so the narrowing happens in SQL and the roll-up is never read in full.
- [`func (s *Server) LogsScope(r *http.Request, p *auth.Principal) (*authz.Scope, error)`](wire.go) — LogsScope returns the request-log scope for an already resolved caller: their own personal logs, plus the service-key logs of the teams they administer. A platform administrator receives every row, which the handler audits on read.
- [`func (s *Server) AuditLogRead(r *http.Request, p *auth.Principal, requestID string, e iam.UsageEvent) error`](wire.go) — AuditLogRead records that a platform administrator read a request log they do not own. The evidence row is the access, not the content: it names the request id and the ownership snapshot that was read
- [`func (s *Server) TeamFilter(r *http.Request, p *auth.Principal) ([]string, error)`](wire.go) — TeamFilter 返回这个调用方可以出现在列表里的团队 id。平台管理员得到 nil，表示不限制。
- [`func (s *Server) WriteAuthz(w http.ResponseWriter, r *http.Request, err error) bool`](wire.go) — WriteAuthz turns a refused authorization into the response: 404 when the object does not exist or is not visible, 403 when it is visible but not permitted, 500 when the ownership could not be read. It reports whether a response was written.
- [`func (s *Server) WriteAuthError(w http.ResponseWriter, r *http.Request, err error) bool`](wire.go) — WriteAuthError turns a failed identification or a failed identity write into the response. It reports whether a response was written.
- [`func (s *Server) WriteIAMError(w http.ResponseWriter, r *http.Request, err error) bool`](wire.go) — WriteIAMError turns a store failure into the response. It reports whether a response was written.
- [`func (s *Server) DataPlane(w http.ResponseWriter, r *http.Request, op string)`](wire.go) — DataPlane hands this inference call to dataplane.Serve. op is the operation name that was already recognized.
- [`func (s *Server) Config() *config.Config`](wire.go) — Config returns the in-process config. A router-settings write changes fields that are already present on RouterSettings.

## 对外 HTTP 边界

入口由下表所列注册文件安装。别名共享处理器；路径存在不替代权限和业务断言，授权说明见模块契约及接口参考。

| Method / path | 注册文件 |
| --- | --- |
| `POST /compliance/eu-ai-act` | [compliance.go](compliance.go) |
| `POST /compliance/gdpr` | [compliance.go](compliance.go) |
| `GET /health/liveliness` | [routes.go](routes.go) |
| `GET /health/liveness` | [routes.go](routes.go) |
| `GET /health/readiness` | [routes.go](routes.go) |
| `GET /health/readiness/details` | [routes.go](routes.go) |
| `GET /health` | [routes.go](routes.go) |
| `GET /.well-known/litellm-ui-config` | [routes.go](routes.go) |
| `GET /litellm/.well-known/litellm-ui-config` | [routes.go](routes.go) |
| `POST /login` | [routes.go](routes.go) |
| `POST /v2/login` | [routes.go](routes.go) |
| `POST /v3/login` | [routes.go](routes.go) |
| `POST /logout` | [routes.go](routes.go) |
| `POST /v2/logout` | [routes.go](routes.go) |
| `POST /v3/logout` | [routes.go](routes.go) |
| `GET /auth/me` | [routes.go](routes.go) |
| `POST /auth/logout` | [routes.go](routes.go) |
| `POST /bootstrap` | [routes.go](routes.go) |
| `GET /bootstrap/status` | [routes.go](routes.go) |
| `GET /authorize/flow` | [routes.go](routes.go) |
| `POST /authorize/complete` | [routes.go](routes.go) |
| `POST /v1/mcp/server/oauth/{server_id}/token` | [routes.go](routes.go) |
| `GET /public/v1/model_hub` | [routes.go](routes.go) |
| `GET /public/v1/model_hub/{facet}` | [routes.go](routes.go) |
| `GET /public/model_hub` | [routes.go](routes.go) |
| `GET /public/model_hub/info` | [routes.go](routes.go) |
| `GET /public/endpoints` | [routes.go](routes.go) |
| `GET /model_hub` | [routes.go](routes.go) |
| `GET /model_hub/{facet}` | [routes.go](routes.go) |
| `POST /model_hub/update_useful_links` | [routes.go](routes.go) |
| `POST /utils/token_counter` | [routes.go](routes.go) |
| `GET /utils/supported_openai_params` | [routes.go](routes.go) |
| `POST /utils/transform_request` | [routes.go](routes.go) |
| `POST /v1/chat/completions` | [routes.go](routes.go) |
| `POST /chat/completions` | [routes.go](routes.go) |
| `POST /v1/embeddings` | [routes.go](routes.go) |
| `POST /embeddings` | [routes.go](routes.go) |
| `POST /v1/completions` | [routes.go](routes.go) |
| `POST /completions` | [routes.go](routes.go) |
| `POST /v1/messages` | [routes.go](routes.go) |
| `POST /v1/audio/translations` | [routes.go](routes.go) |
| `POST /audio/translations` | [routes.go](routes.go) |
| `POST /flushall` | [routes.go](routes.go) |
| `GET /cache/settings` | [routes.go](routes.go) |
| `POST /cache/settings` | [routes.go](routes.go) |
| `GET /get/ui_theme_settings` | [routes.go](routes.go) |
| `PATCH /update/ui_theme_settings` | [routes.go](routes.go) |
| `POST /upload/logo` | [routes.go](routes.go) |
| `POST /prompts/test` | [routes.go](routes.go) |
| `POST /search_tools/test_connection` | [routes.go](routes.go) |
| `GET /cache/ping` | [routes.go](routes.go) |
| `GET /ping` | [routes.go](routes.go) |
| `GET /customer/list` | [routes.go](routes.go) |
| `GET /end_user/list` | [routes.go](routes.go) |

## 依赖关系

[internal/auth](../auth/readme_cn.md), [internal/authz](../authz/readme_cn.md), [internal/cache](../cache/readme_cn.md), [internal/catalog](../catalog/readme_cn.md), [internal/config](../config/readme_cn.md), [internal/dataplane](../dataplane/readme_cn.md), [internal/gateway/family](family/readme_cn.md), [internal/gateway/guard](guard/readme_cn.md), [internal/gateway/identity](identity/readme_cn.md), [internal/gateway/keys](keys/readme_cn.md), [internal/gateway/models](models/readme_cn.md), [internal/gateway/prefs](prefs/readme_cn.md), [internal/gateway/usage](usage/readme_cn.md), [internal/hooks](../hooks/readme_cn.md), [internal/httpx](../httpx/readme_cn.md), [internal/iam](../iam/readme_cn.md), [internal/live](../live/readme_cn.md), [internal/llm](../llm/readme_cn.md), [internal/llm/estimate](../llm/estimate/readme_cn.md), [internal/logx](../logx/readme_cn.md), [internal/plugin](../plugin/readme_cn.md), [internal/provider](../provider/readme_cn.md), [internal/provider/all](../provider/all/readme_cn.md), [internal/router](../router/readme_cn.md), [internal/store](../store/readme_cn.md).

## 验证与维护入口

| 测试文件 | 场景入口 |
| --- | --- |
| [access_log_test.go](access_log_test.go) | `TestHandlerLogsMethodPathStatusAndDuration`, `TestHandlerLogsErrorLineForFailure` |
| [activity_http_test.go](activity_http_test.go) | `TestDailyActivityHTTPReadsRecordedUsage`, `TestUsageViewsFollowTheCallerScope` |
| [affinity_pin_test.go](affinity_pin_test.go) | `TestOfficialTaskPinLastsSevenDays` |
| [affinity_test.go](affinity_test.go) | `TestSessionIDSticksToThePromptPrefix`, `TestContinuationPinsDoNotCrossCallerOrModel`, `TestSessionIDPrefersAnExplicitHeader`, `TestSessionIDReadsClaudeMetadataAndClientHeaders`, `TestContentSessionStaysStableAsTheTranscriptGrows`, `TestAffinityPinIsReused` |
| [builtin_providers_test.go](builtin_providers_test.go) | `TestBuiltinProvidersInstallOnStartup`, `TestBuiltinProvidersStayOutWhenDisabled` |
| [call_cost_test.go](call_cost_test.go) | `TestDeploymentCostUsesWindowsCacheImagesAndSeconds`, `TestCallCostUsesCatalogBaseModelAndManualDoesNotFallThrough` |
| [catalog_reads_test.go](catalog_reads_test.go) | `TestCatalogReads` |
| [chains_test.go](chains_test.go) | `TestMain`, `TestLiveChains`, `TestPlaygroundCompletionReachesUpstream` |
| [compliance_test.go](compliance_test.go) | `TestEUAIActWithoutGuardrailsFailsClosed`, `TestCompliancePassesWhenPreCallSucceeds`, `TestMixedGuardrailModeDoesNotCountAsPreCall`, `TestSingleGuardrailObjectIsAccepted`, `TestComplianceEndpointsReturnChecks` |
| [console_split_test.go](console_split_test.go) | `TestGatewayDoesNotHostTheConsole`, `TestPublicOriginOverride` |
| [dial_log_test.go](dial_log_test.go) | `TestHandlerDialFailureLogKeepsHostWithoutURL` |
| [dropped_alerts_test.go](dropped_alerts_test.go) | `TestDroppedAlertsAreGone`, `TestUnusedAdminSurfacesStayGone` |
| [guardrail_block_test.go](guardrail_block_test.go) | `TestChatStopsBeforeUpstreamWhenADefaultGuardrailBlocks` |
| [idempotency_test.go](idempotency_test.go) | `TestIdempotencyScopesHashesAndExpires`, `TestIdempotencyConcurrentRequestWaitsForOwner` |
| [log_completeness_test.go](log_completeness_test.go) | `TestLogRecordRoundTripKeepsTheFactsTheConsoleReads`, `TestPlanRoutePinsTheSameDeploymentForOnePrompt` |
| [official_settlement_test.go](official_settlement_test.go) | `TestOfficialSettlementSkipsRouteTPMAndConsumesOwnMetadata` |
| [permission_test.go](permission_test.go) | `TestMasterCannotEnumerateTenants`, `TestMasterCannotWrite`, `TestListingsRespectTeamScope`, `TestTeamReadRequiresMembership`, `TestTeamAdminBoundary`, `TestMemberAdministrationNeedsTeamAdmin`, `TestMemberMustBeInTheTeam` |
| [prompt_log_test.go](prompt_log_test.go) | `TestPromptJSONKeepsHeadersBodyAndResponse`, `TestSpendLogRoundTripReturnsPromptPayload` |
| [prompt_response_test.go](prompt_response_test.go) | `TestAssembleLoggedResponseReadsCompletedEvent`, `TestAssembleLoggedResponseKeepsStreamedToolCalls`, `TestAssembleLoggedResponseJoinsChatDeltas` |
| [removed_test.go](removed_test.go) | `TestRemovedColumnsStayUnregistered`, `TestRetiredMoreToolsRoutesAreRemoved`, `TestRetiredPathsAreRefused`, `TestRemovedColumnWinsOverRetired` |
| [retired_preferences_test.go](retired_preferences_test.go) | `TestRetiredRouterPreferencesReturnBadRequest` |
| [route_template_test.go](route_template_test.go) | `TestTemplatesAreThePlatformAdministratorsToEdit`, `TestSelectingATemplateRespectsTheScope`, `TestAnOrganizationAdministratorPointsTheirOwnOrganization`, `TestDeletingATemplateStillInUseIsRefusedWithTheList`, `TestClearingASelectionIsNotTheSameAsLeavingItOut`, `TestTheEffectiveTemplateNamesWhereItCameFrom`, `TestAnUnknownScopeIsRefused`, `TestATeamAdministratorSelectsThroughTheTeamRoute`, `TestTheTeamReadExposesTheSelection`, `TestANewTemplateIsSeededFromThePlatformDefault`, `TestASuppliedBodyIsNotSecondGuessed`, `TestASingleTeamSessionPicksUpItsTeamsTemplate` |
| [spend_session_test.go](spend_session_test.go) | `TestSessionLogBindingFilesASingleMembership`, `TestSessionLogBindingDoesNotGuessAmongTeams` |
| [template_selection_regression_test.go](template_selection_regression_test.go) | `TestTemplateVisibilityOrdinaryWrites` |
| [testdb_test.go](testdb_test.go) | fixture/辅助函数，无顶层 Test |
| [visibility_chain_test.go](visibility_chain_test.go) | `TestVisibilityChainPlatformAdminSeesEverything`, `TestVisibilityChainOrgAdminSeesOwnOrganization`, `TestVisibilityChainTeamAdminSeesOwnTeam`, `TestVisibilityChainMemberSeesOnlySelf`, `TestVisibilityChainOrgAdminCannotCrossOrganizations`, `TestVisibilityChainTeamAdminCannotCrossTeams`, `TestVisibilityChainPasswordResetFollowsTheChain`, `TestVisibilityChainCreateAccountCarriesItsScope`, `TestVisibilityChainRejectsScopeTheCallerMayNotGrant`, `TestVisibilityChainUsageAndLogsFollowTheChain`, `TestVisibilityChainNothingLeaksThroughTheAccountPicker`, `TestVisibilityChainBlockTakesEffectImmediately` |

```bash
go test ./internal/gateway -count=1
```

数据库验收设置 XHUB_REGRESSION_STRICT=1 并检查跳过项；Redis 和真实供应商需单独配置。接口、字段或行为改变后同步本说明及相关功能文档。
