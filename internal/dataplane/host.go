// host.go is the gateway surface this package is allowed to call. The
// process implements Host in gateway/wire.go. Adding a method here forces
// that adapter and every test stub to implement it, which is intentional:
// the data plane must not import the process.

package dataplane

import (
	"net/http"
	"time"

	"github.com/sunqirui1987/xhub/internal/auth"
	"github.com/sunqirui1987/xhub/internal/cache"
	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/gateway/prefs"
	"github.com/sunqirui1987/xhub/internal/hooks"
	"github.com/sunqirui1987/xhub/internal/iam"
	"github.com/sunqirui1987/xhub/internal/live"
	"github.com/sunqirui1987/xhub/internal/logx"
	"github.com/sunqirui1987/xhub/internal/plugin"
	"github.com/sunqirui1987/xhub/internal/provider"
	"github.com/sunqirui1987/xhub/internal/router"
)

// SpendLog 是用量行。聊天循环和 Bypass 都调用它，两边都不自己写数据库。
// 网关实现在 gateway/spend.go。必须先 RememberExchange 和 AnnotateCall，再 RecordSpend，
// 因为 RecordSpend 会按 callID 把暂存的头、正文和备注取走并删掉。
type SpendLog interface {
	// RememberExchange 把这一次调用的请求头、请求体和响应体暂存到 callID 上。
	// 网关只在打开「在日志里保存请求内容」时才真正落库。
	//
	// 参数 callID：本次请求的 x-litellm-call-id。r：入站请求，用来抄方法和头。
	// 参数 reqBody：原始请求字节。respBody：准备写入日志的响应字节。查询没有正文时传 nil。
	// 返回：无。
	// 调用：Serve 的拒绝、缓存命中、流式成功、非流式成功；ServeBypass 的创建和查询。
	// 测试：gateway/prompt_log_test.go 覆盖 promptJSON 的落库形状。数据面夹具里的实现是空的。
	RememberExchange(callID string, r *http.Request, reqBody, respBody []byte)

	// AnnotateCall 记下首字时间、供应商、缓存和部署。RecordSpend 会按 callID 取走这条备注。
	// 参数 callID：与 RememberExchange 相同。note：这一次要写进日志的事实。TTFTMs 为 nil 时该列留空。
	// 返回：无。
	// 调用：Serve 的缓存命中和成功路径；ServeBypass 在转发结束后。
	// 测试：bypass_logic_test.go 断言创建笔记里有供应商、TTFT 和部署 id。
	AnnotateCall(callID string, note CallNote)

	// RecordSpend 写一条用量。它读取并清掉 callID 上的暂存，计算费用，更新响应头里的 cost。
	// 缓存命中或状态码大于等于 400 时，token 仍保留，扣费金额强制为 0。
	//
	// 参数 w：写 x-litellm-response-cost 等头。p：调用方，用来记用户、团队和密钥。
	// 参数 callID：取暂存。alias：对外模型名。op：call_type，聊天是 chat，Bypass 是「类型:动作」。
	// 参数 usage：prompt_tokens 和 completion_tokens。nil 当作空用量，费用为 0。
	// 参数 start：请求开始时间，用来算耗时。cacheHit：为 true 时不按 token 扣费。
	// 参数 status：写进日志的 HTTP 状态。depID：api_base|model，用来找部署上的单价。
	// 返回：无。有 Redis 时行进队列，否则当场写 PostgreSQL。
	// 调用：Serve、ServeBypass，以及 gateway WriteChatJSON、WriteCacheHit 内部。
	// 测试：prompt_log_test.go TestSpendLogRoundTripReturnsPromptPayload；bypass_logic_test.go 检查 call_type 和只扣一次。
	RecordSpend(w http.ResponseWriter, p *auth.Principal, callID, alias, op string, usage map[string]any, start time.Time, cacheHit bool, status int, depID string)
}

// Adapted 是聊天、向量、图像、音频、重排这条循环。它不钉官方任务，也不刷 Redis。
// 网关在 limits.go 的 dataPlane 里把 *Server 传给 Serve。
type Adapted interface {
	// RequireLLMPrincipal 解析可以发起推理的身份。失败时响应已经写成 401，并返回 nil。
	// 参数 w：写 401。r：从 Authorization 或会话 cookie 取身份。
	// 返回：调用方。nil 表示未授权。
	// 调用：Serve 开头。测试：failure_log_test.go 的 logHost 固定返回一个会话身份。
	RequireLLMPrincipal(w http.ResponseWriter, r *http.Request) *auth.Principal

	// ResolveRequest 在重试前再解析一次身份，确认密钥没有在这次请求中途失效。
	// 参数 r：同一入站请求。
	// 返回：新的身份。密钥被撤销或过期时返回错误，Serve 中止重试。
	// 调用：Serve 在 num_retries 大于 0 且第一次尝试之后。测试夹具返回固定身份。
	ResolveRequest(r *http.Request) (*auth.Principal, error)

	// EnforceIdentityLimits 检查模型允许名单、预算，以及 RPM 或 TPM。拒绝时响应已写好。
	//
	// 参数 w、path：写 403 或 429，path 决定错误包络。p：调用方。alias：对外模型名。
	// 参数 est：EstimateTokens 给出的上界，用来扣 TPM。
	// 返回：true 表示可以继续。false 表示调用方必须 return。
	// 调用：Serve 在读完模型名之后，以及每次重试之前。测试夹具恒返回 true。
	EnforceIdentityLimits(w http.ResponseWriter, path string, p *auth.Principal, alias string, est int) bool

	// GuardrailBlocks 在聊天发往上游之前跑护栏。拦截时响应已经写好。
	// 参数 callID：本次调用 id。body：请求 JSON。
	// 返回：blocked 为 true 时 msg 是给调用方的说明。未命中时 blocked 为 false，msg 为空。
	// 调用：Serve，且只在 op 为 chat 或空时。测试：gateway/guardrail_block_test.go。
	GuardrailBlocks(callID string, body map[string]any) (bool, string)

	// GatewayConfig 返回进程配置。Serve 用它的模型表；路由策略和重试次数改读 RouteSettingsFor。
	// 参数：无。
	// 返回 *config.Config（*config.Config）：当前进程配置，含模型表和路由策略，不会复制。
	// 调用：dataplane/official.go、dataplane/serve.go、gateway/wire.go
	// 测试：bypass_logic_test.go、failure_log_test.go
	GatewayConfig() *config.Config

	// RouteSettingsFor 返回这一次请求生效的路由设置：策略、重试次数、超时和冷却阈值。
	//
	// 调用方选中的模板由身份链在预算检查时解析出来，所以这里不遍历、不额外查询。
	// 没有任何一层选中模板时返回平台默认那一份，行为和这个功能存在之前一样。
	//
	// 参数 p（*auth.Principal）：已经解析的调用方，含预算链填入的模板选择。
	// 返回 prefs.RouteSettings（prefs.RouteSettings）：这次请求生效的设置和来源。
	// 调用：dataplane/serve.go
	// 测试：route_settings_test.go
	RouteSettingsFor(p *auth.Principal) prefs.RouteSettings

	// RouteState 返回这一刻的冷却、延迟、用量和正在处理的请求数。没有 Redis 时只有 Busy。
	// 参数：无。
	// 返回 router.State（router.State）：这一刻的冷却、延迟、用量和并发，交给路由器排序。
	// 调用：dataplane/official.go、dataplane/serve.go、gateway/wire.go
	// 测试：bypass_logic_test.go、failure_log_test.go
	RouteState() router.State

	// PlanRoute 在选部署之前解析会话，并取回已经钉在这个会话上的部署。
	// 参数 r：读会话头。alias：对外模型名。body：读 metadata.user_id 和消息。p：用来隔离租户。
	// 返回：RoutePlan。Pinned 非空时 Serve 把该部署放在候选第一位。
	// 调用：Serve。测试：gateway/affinity_test.go。
	PlanRoute(r *http.Request, alias string, body map[string]any, p *auth.Principal) RoutePlan

	// CommitRoute 把这次真正用到的部署记到会话上，供下一轮继续打到同一上游。
	// 参数 plan：PlanRoute 的结果。deploymentID：api_base|model。responseID：响应里的 id，可空。
	// 返回：无。
	// 调用：Serve 在流式或非流式成功之后。测试：affinity_test.go TestAffinityPinIsReused。
	CommitRoute(plan RoutePlan, deploymentID, responseID string)

	// ResponseCache 返回进程内和 Redis 的响应缓存。缓存关闭时 Get 仍然可调用，只是没有命中。
	// 参数：无。
	// 返回 *cache.DualCache（*cache.DualCache）：响应缓存。未命中时 Get 返回空。
	// 调用：dataplane/serve.go、gateway/wire.go
	// 测试：bypass_logic_test.go、failure_log_test.go
	ResponseCache() *cache.DualCache

	// HookEngine 返回并发钩子。Begin 占用一个名额，返回的函数在请求结束时释放。 超过并行上限时 Begin 的结果由网关侧写成 429，这里只负责调用 Begin。
	// 参数：无。
	// 返回 *hooks.Engine（*hooks.Engine）：并发钩子。Begin 占用一个名额，返回的函数在请求结束时释放。
	// 调用：dataplane/serve.go、gateway/wire.go
	// 测试：bypass_logic_test.go、failure_log_test.go
	HookEngine() *hooks.Engine

	// Extensions 返回推理前的扩展注册表。没有注册项时 Run 不拒绝，调用继续走向上游。
	// 参数：无。
	// 返回 *plugin.Registry（*plugin.Registry）：推理前扩展。没有注册项时不会拒绝调用。
	// 调用：dataplane/serve.go、gateway/wire.go
	// 测试：bypass_logic_test.go、failure_log_test.go
	Extensions() *plugin.Registry

	// AttachCredential 按部署上的凭据名补上 api_key 和 api_base。没有凭据名时原样返回。
	// 参数 dep：路由选出的一行模型。
	// 返回：填好密钥的副本。凭证库不可用或凭证无效时返回错误，Serve 跳过该部署。
	// 调用：Serve 每个部署一次；ServeBypass 在创建和查询时。测试夹具原样返回 dep。
	AttachCredential(dep config.ModelEntry) (config.ModelEntry, error)

	// HTTPClient 返回访问上游的客户端。超时来自路由配置。
	// 参数：无。
	// 返回 *http.Client（*http.Client）：访问上游的 HTTP 客户端，超时来自路由设置。
	// 调用：dataplane/official.go、dataplane/serve.go、gateway/wire.go
	// 测试：bypass_logic_test.go、failure_log_test.go
	HTTPClient() *http.Client

	// IncBusy 把该部署正在处理的请求数加一。id 是 api_base|model。RouteState 的 Busy 读这个计数。
	// 参数 id（string）：部署 id，形状是 api_base|model。空串时计数没有对应的部署。
	// 返回：无。该部署的在途请求数加一。RouteState 的 Busy 读这个数。
	// 调用：dataplane/serve.go、gateway/wire.go
	// 测试：bypass_logic_test.go、failure_log_test.go
	IncBusy(id string)

	// DecBusy 把该部署正在处理的请求数减一。必须和 IncBusy 成对，包含失败路径。
	// 参数 id：与 IncBusy 相同。返回：无。
	// 调用：Serve 在请求结束时，包括失败路径，必须和 IncBusy 成对。
	// 测试：无直接单测
	DecBusy(id string)

	// NoteFailure 记下这个部署的一次失败，供下次路由决定是否冷却。
	//
	// 阈值跟着这次请求的设置走，所以调用方把那设置一并传进来：一个租户把
	// allowed_fails 调低之后，它的失败按它自己的阈值计数，而不是按全局的值。
	//
	// 参数 id：部署 id。settings：这次请求生效的路由设置。返回：无。
	// 调用：Serve 在 5xx 或 429 之后。实现转到 dataplane.RecordFailure。
	// 测试：无直接单测
	NoteFailure(id string, settings prefs.RouteSettings)

	// NoteLatency 记下从这个请求开始到响应头的毫秒数。
	// 参数 id：部署 id。ms：从请求开始到响应头的毫秒数。返回：无。
	// 调用：Serve 在收到上游响应头之后。
	// 测试：无直接单测
	NoteLatency(id string, ms float64)

	// SetChatHeaders 给聊天响应写上调用 id、对外模型名和本次上游地址。
	// 参数 w：响应。p：调用方，用来决定哪些头可以暴露。alias：对外模型名。apiBase：本次上游根地址。
	// 返回：无。
	// 调用：Serve 非流式成功，以及流式开始写出之前。
	// 测试：无直接单测
	SetChatHeaders(w http.ResponseWriter, p *auth.Principal, alias, apiBase string)

	// WriteCacheHit 把缓存正文写回调用方，并记一条扣费为 0 的用量。token 数仍保留。
	//
	// 参数 w、p、callID、alias：与 RecordSpend 相同。ck：缓存键。op：操作名。
	// 参数 hit：缓存的响应字节。start：请求开始时间。
	// 返回：无。响应已写完。
	// 调用：Serve 在 ResponseCache.Get 命中时。测试：TestServeLogsCacheHitAndStreamMetrics。
	WriteCacheHit(w http.ResponseWriter, p *auth.Principal, callID, alias, ck, op string, hit []byte, start time.Time)

	// WriteChatJSON 把上游 JSON 写给调用方并记用量。非成功状态不会被当成成功正文放进缓存。
	//
	// 参数 provider：custom_llm_provider 或从模型 id 拆出的供应商。respBody：上游正文。
	// 参数 status：上游状态码。depID：部署 id。其余参数与 WriteCacheHit 相同。
	// 返回：无。
	// 调用：Serve 的非流式成功路径。流式路径自己写字节并直接 RecordSpend。
	// 测试：failure_log_test.go
	WriteChatJSON(w http.ResponseWriter, p *auth.Principal, callID, alias, ck, op, provider string, respBody []byte, status int, start time.Time, depID string)

	SpendLog
}

// Bypass 是官方接口转发。它不把正文编成聊天补全，不读响应缓存，不跑护栏。
// 网关在 bypass.go 的 serveBypass 里，路径已经匹配到端点类型之后调用 ServeBypass。
// 和 Adapted 同名的方法语义相同，这里再写一遍，读这个接口时不用跳回去。
type Bypass interface {
	// RouteSettingsFor 读取此次请求继承的模板。
	// 参数 p：调用方。返回：路由配置。
	// 调用：ServeBypass。测试：official_template_test.go。
	RouteSettingsFor(p *auth.Principal) prefs.RouteSettings
	// NoteFailure 更新当前凭据的失败计数。
	// 参数 id、settings：凭据部署 id 和模板。返回：无。
	// 调用：ServeBypass。测试：official_template_test.go。
	NoteFailure(id string, settings prefs.RouteSettings)
	// ResolveRequest 在重试前重新检查身份。
	// 参数 r：请求。返回：最新身份或错误。
	// 调用：ServeBypass。测试：official_template_test.go。
	ResolveRequest(r *http.Request) (*auth.Principal, error)

	// RequireLLMPrincipal 解析可以发起官方接口调用的身份。失败时响应已经写成 401，并返回 nil。
	// 参数 w：写 401。r：入站请求。
	// 返回：调用方。nil 表示停止。
	// 调用：ServeBypass 开头。测试：bypass_logic_test.go 的 logicHost 返回固定会话。
	RequireLLMPrincipal(w http.ResponseWriter, r *http.Request) *auth.Principal

	// EnforceIdentityLimits 检查模型允许名单、预算和速率。拒绝时响应已写好。
	//
	// 参数 path：官方路径，例如 /api/v3/contents/generations/tasks。alias：正文模型字段里的名字。
	// 参数 est：EstimateTokens 的上界。
	// 返回：true 才继续选部署。
	// 调用：serveBypassCreate。测试夹具恒返回 true。
	EnforceIdentityLimits(w http.ResponseWriter, path string, p *auth.Principal, alias string, est int) bool

	// GatewayConfig 返回进程配置。Bypass 只用路由策略给 router.Order。
	// 参数：无。
	// 返回 *config.Config（*config.Config）：当前进程配置，含模型表和路由策略，不会复制。
	// 调用：dataplane/official.go、dataplane/serve.go、gateway/wire.go
	// 测试：bypass_logic_test.go、failure_log_test.go
	GatewayConfig() *config.Config

	// RouteState 返回冷却和并发快照，供创建请求在多部署之间排序。
	// 参数：无。
	// 返回 router.State（router.State）：这一刻的冷却、延迟、用量和并发，交给路由器排序。
	// 调用：dataplane/official.go、dataplane/serve.go、gateway/wire.go
	// 测试：bypass_logic_test.go、failure_log_test.go
	RouteState() router.State

	// Models 返回当前进程里的全部部署。Bypass 再用端点类型过滤。
	// 参数：无。
	// 参数：无。
	// 调用：dataplane/live.go、dataplane/official.go、gateway/bypass.go、gateway/wire.go
	// 测试：bypass_logic_test.go、failure_log_test.go
	Models() []config.ModelEntry

	// AttachCredential 给选中的官方部署补上上游密钥。凭证失败时转发会写成 401。
	// 参数 dep：选中的部署。
	// 返回：带密钥的副本。凭证失败时 ServeBypass 写 401。
	// 调用：创建、查询，以及列表在按密钥分组时。
	// 测试：bypass_logic_test.go
	AttachCredential(dep config.ModelEntry) (config.ModelEntry, error)

	// FindDeployment 按钉住的部署 id 找回那条模型配置。进程里已经没有这行时返回假。
	// 参数 id：PinOfficial 存下来的值。
	// 返回：找到时 ok 为 true。进程里已经没有这行时 ok 为 false，查询回 404。
	// 调用：serveBypassFollow。测试：bypass_logic_test.go。
	FindDeployment(id string) (config.ModelEntry, bool)

	// HTTPClient 返回访问上游的客户端。
	// 参数：无。
	// 返回 *http.Client（*http.Client）：访问上游的 HTTP 客户端，超时来自路由设置。
	// 调用：dataplane/official.go、dataplane/serve.go、gateway/wire.go
	// 测试：bypass_logic_test.go、failure_log_test.go
	HTTPClient() *http.Client

	// PlanRoute 为官方请求解析会话 id，只用于记用量，不改变这次选中的部署。
	// 参数 alias：对外模型名。body：创建请求的 JSON；查询时为 nil。
	// 返回：RoutePlan。只用 SessionID 写入 CallNote。
	// 调用：创建和查询在记用量之前。
	// 测试：bypass_logic_test.go
	PlanRoute(r *http.Request, alias string, body map[string]any, p *auth.Principal) RoutePlan

	// PinOfficial 把官方任务 id 钉到创建时的部署，有效期七天。
	// 参数 taskID：从创建响应按端点类型的 task_id 字段取出。deploymentID：api_base|model。
	// 返回：无。
	// 调用：serveBypassCreate 在上游返回任务 id 之后。测试：gateway/affinity_pin_test.go 固定 7 天。
	PinOfficial(taskID, deploymentID string)

	// OfficialDeployment 按任务 id 取回创建时的部署。没有钉时返回空串。
	// 参数 taskID：路径参数或查询参数里的任务 id。
	// 返回：部署 id，或空串。
	// 调用：serveBypassFollow。测试：bypass_logic_test.go 清掉钉之后再查得到 404。
	OfficialDeployment(taskID string) string
	// PinOfficialContext stores creation-time billing facts alongside a task pin.
	// 参数 taskID（string）：任务标识；facts（provider.TaskContext）：请求中的计费事实。
	// 返回：无。
	// 调用：官方任务创建成功后。
	// 测试：official_settlement_test.go。
	PinOfficialContext(taskID string, facts provider.TaskContext)
	// OfficialContext returns the facts recorded when a task was created.
	// 参数 taskID（string）：任务标识。
	// 返回 provider.TaskContext：已保存的计费事实；缺失时为零值。
	// 调用：官方任务查询结算时。
	// 测试：official_settlement_test.go。
	OfficialContext(taskID string) provider.TaskContext

	SpendLog
}

// Runtime 是 Redis 快照和花费刷写。它不处理 HTTP，也不看请求正文。
// 网关在 routerState、noteFailure、noteLatency、noteUsage 和 flushLoop 里调用 live.go。
type Runtime interface {
	// BusyMap 返回本进程内每个部署 id 正在处理的请求数。
	// 参数：无。接收者是实现 Runtime 的进程。
	// 返回：部署 id 到正在处理的请求数。没有该 id 表示当前是 0，不是一份解析出来的 JSON。
	// 调用：State，放进 router.State.Busy。
	// 测试：failure_log_test.go 的夹具返回空表。
	BusyMap() map[string]int

	// Redis 返回热路径客户端。未配置 Redis 时返回 nil，State 只填 Busy，Flush 直接返回。
	// 参数：无。
	// 返回 *live.Client（*live.Client）：热路径 Redis 客户端。未配置 Redis 时为 nil，花费改为当场写数据库。
	// 调用：dataplane/live.go、gateway/wire.go
	// 测试：bypass_logic_test.go、failure_log_test.go
	Redis() *live.Client

	// Models 返回全部部署，用来把模型行换成部署 id，再向 Redis 批量问冷却、延迟和用量。
	// 参数：无。
	// 返回：当前模型表。空表表示没有部署。
	// 调用：State 和 Bypass 的 eligible。
	// 测试：bypass_logic_test.go 用夹具模型表选部署。
	Models() []config.ModelEntry

	// RouterDocument 返回路由设置的原始键值，读 allowed_fails 和 cooldown_time。
	// 参数：无。
	// 返回：路由设置原表。RecordFailure 从中读 allowed_fails 和 cooldown_time。缺这两个键时用默认：不允许失败就不写 Redis，冷却一分钟。
	// 调用：RecordFailure。
	// 测试：无直接单测。
	RouterDocument() map[string]any

	// Identity 是 Flush 写入用量的库。没配 PostgreSQL 时为 nil，Flush 直接返回。
	// 参数：无。
	// 返回 *iam.DB（*iam.DB）：Flush 写入用量的库。没有 PostgreSQL 时为 nil，Flush 直接返回。
	// 调用：dataplane/live.go、gateway/identity/gate.go、gateway/identity/handlers.go、gateway/identity/members.go
	// 测试：bypass_logic_test.go、failure_log_test.go
	Identity() *iam.DB
}

// Host is the process surface. The gateway implements it once. Callers take
// Adapted, Bypass, or Runtime so a change in one loop does not touch the others.
type Host interface {
	Adapted
	Bypass
	Runtime
}

// RoutePlan 是选部署之前决定的会话钉。
// Alias 是对外模型名。SessionID 写入用量行，把同一会话的请求收成一组。
// Caller 是密钥哈希或用户 id，避免不同租户共用一根钉。
// Pinned 是上一次部署 id。非空时 Serve 把该部署放在候选第一位。Bypass 不使用 Pinned。
type RoutePlan struct {
	Alias     string
	SessionID string
	Caller    string
	Pinned    string
}

// CallNote 是写进用量行的事实。由 AnnotateCall 暂存，RecordSpend 取走。
// TTFTMs 是首字节或同步整次的毫秒数，nil 表示这次没量到。
// Provider 是 custom_llm_provider，或 Bypass 的供应商前缀。
// CacheKey 是响应缓存键。CacheHit 为 true 时扣费金额为 0。
// SessionID 与 RoutePlan.SessionID 相同。部署身份由 RecordSpend 的 depID 参数传递。
type CallNote struct {
	// SettlementID overrides persisted RequestID, never the metadata callID.
	SettlementID string
	// SkipRouteUsage excludes async task queries from synchronous route TPM.
	SkipRouteUsage bool
	TTFTMs         *int
	Provider       string
	CacheKey       string
	CacheHit       bool
	SessionID      string
}

// traceHop 在数据面入口记一条日志，只含路径，不含正文和密钥。
// 参数 path：入站 URL 路径。返回：无。
// 调用：Serve 第一行。无单独测试。
func traceHop(path string) {
	logx.Trace("dataplane hop path=%s", path)
}
