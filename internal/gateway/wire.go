// wire.go is the adapter surface. dataplane.Host, and the Host interfaces in
// the subpackages, are implemented by forwarding to functions that already
// live next to the policy. A method here must not grow a second copy of the
// check. New process capabilities belong beside the code they call, and then
// get a one-line forwarder in this file.

package gateway

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/sunqirui1987/xhub/internal/auth"
	"github.com/sunqirui1987/xhub/internal/authz"
	"github.com/sunqirui1987/xhub/internal/cache"
	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/dataplane"
	"github.com/sunqirui1987/xhub/internal/gateway/guard"
	"github.com/sunqirui1987/xhub/internal/gateway/models"
	"github.com/sunqirui1987/xhub/internal/gateway/prefs"
	"github.com/sunqirui1987/xhub/internal/hooks"
	"github.com/sunqirui1987/xhub/internal/iam"
	"github.com/sunqirui1987/xhub/internal/live"
	"github.com/sunqirui1987/xhub/internal/logx"
	"github.com/sunqirui1987/xhub/internal/plugin"
	"github.com/sunqirui1987/xhub/internal/router"
	"github.com/sunqirui1987/xhub/internal/store"
	"sync"
)

var logTraceOnceWire sync.Once

// RequireLLMPrincipal resolves an identity that may call inference. On failure it has already written the response and returns nil.
// 参数 w（http.ResponseWriter）：调用方的 HTTP 响应，状态码和正文写在这里；r（*http.Request）：入站 HTTP 请求，用来读路径、头和正文。
// 返回 *auth.Principal（*auth.Principal）：已经解析的调用方，含用户、团队和密钥。
// 调用：dataplane/host.go、dataplane/official.go、dataplane/serve.go
// 测试：bypass_logic_test.go、failure_log_test.go
func (s *Server) RequireLLMPrincipal(w http.ResponseWriter, r *http.Request) *auth.Principal {
	logTraceOnceWire.Do(func() { logx.Trace("enter gateway.RequireLLMPrincipal") })

	return s.requireLLMPrincipal(w, r)
}

// ResolveRequest resolves the request identity again so a retry can confirm the key is still valid.
// 参数 r（*http.Request）：入站 HTTP 请求，用来读路径、头和正文。
// 返回 *auth.Principal（*auth.Principal）：已经解析的调用方，含用户、团队和密钥；error（error）：失败原因，nil 表示这一步成功。
// 调用：dataplane/host.go、dataplane/serve.go
// 测试：bypass_logic_test.go、failure_log_test.go
func (s *Server) ResolveRequest(r *http.Request) (*auth.Principal, error) {
	return s.resolve(r)
}

// GatewayConfig 返回当前进程配置。空模型表表示没有部署，不是「不做限制」。
// 参数 s：网关进程。
// 返回：当前配置，不会复制。
// 调用：dataplane Serve 与 ServeBypass。测试：failure_log_test.go、bypass_logic_test.go 的夹具实现同一方法。
func (s *Server) GatewayConfig() *config.Config { return s.Cfg }

// HTTPClient returns the HTTP client used for upstream calls. The timeout comes from the router settings.
// 参数：无。
// 调用：dataplane/host.go、dataplane/official.go、dataplane/serve.go
// 测试：bypass_logic_test.go、failure_log_test.go
// 返回 *http.Client（*http.Client）：访问上游的 HTTP 客户端，超时来自路由设置。
func (s *Server) HTTPClient() *http.Client { return s.Client }

// ResponseCache returns the in-process cache for non-streaming responses.
// 参数：无。
// 调用：dataplane/host.go、dataplane/serve.go
// 测试：bypass_logic_test.go、failure_log_test.go
// 返回 *cache.DualCache（*cache.DualCache）：响应缓存。未命中时 Get 返回空。
func (s *Server) ResponseCache() *cache.DualCache { return s.Cache }

// HookEngine returns the budget and concurrency gate.
// 参数：无。
// 调用：dataplane/host.go、dataplane/serve.go
// 测试：bypass_logic_test.go、failure_log_test.go
// 返回 *hooks.Engine（*hooks.Engine）：并发钩子。Begin 占用一个名额，返回的函数在请求结束时释放。
func (s *Server) HookEngine() *hooks.Engine { return s.Hooks }

// Extensions is the empty extension registry created at process start. Tests and startup code register implementations here, and the data plane calls them in registration order.
// 参数：无。
// 调用：dataplane/host.go、dataplane/serve.go
// 测试：bypass_logic_test.go、failure_log_test.go
// 返回 *plugin.Registry（*plugin.Registry）：推理前扩展。没有注册项时不会拒绝调用。
func (s *Server) Extensions() *plugin.Registry { return s.extensions }

// RouteState returns concurrency, cooldown, latency, and usage for the router. Without Redis only in-process Busy is present.
// 参数：无。
// 调用：dataplane/host.go、dataplane/official.go、dataplane/serve.go
// 测试：bypass_logic_test.go、failure_log_test.go
// 返回 router.State（router.State）：这一刻的冷却、延迟、用量和并发，交给路由器排序。
func (s *Server) RouteState() router.State { return s.routerState() }

// RouterDocument returns the merged router settings. A database key overrides YAML.
// 参数：无。
// 返回 map[string]any（map[string]any）：路由文档的字段表。缺键表示上游或库里没有这个字段。
// 调用：dataplane/host.go、dataplane/live.go
// 测试：bypass_logic_test.go、failure_log_test.go
func (s *Server) RouterDocument() map[string]any { return prefs.MergedRouter(s) }

// GuardrailBlocks runs guardrails before a chat request is sent. When blocked is true, msg is the reason shown to the caller. The findings are kept against callID until the spend row is written.
// 参数 callID（string）：这一次调用的 id，用来把请求、响应和用量记在同一行；body（map[string]any）：已经解析的 JSON 对象。缺字段表示上游或调用方没有给这项。
// 返回 bool（bool）：护栏拦截了这次聊天时返回真。未拦截时返回假；string（string）：给调用方看的拦截说明。未拦截时为空串。
// 调用：dataplane/host.go、dataplane/serve.go
// 测试：bypass_logic_test.go、failure_log_test.go
func (s *Server) GuardrailBlocks(callID string, body map[string]any) (bool, string) {
	blocked, message, findings := guard.Evaluate(s, body)
	if len(findings) > 0 && callID != "" {
		if raw, err := json.Marshal(findings); err == nil {
			s.noteGuardrail(callID, string(raw))
		}
	}
	return blocked, message
}

// AttachCredential 按部署上的凭据名从库里补上 api_key 和 api_base。
// 参数 dep（config.ModelEntry）：要补密钥的部署。没有凭证名时原样返回。
// 返回：填好密钥的副本，或凭证错误。数据面据此决定跳过该部署还是写 401。
// 调用：dataplane Serve 与 ServeBypass。
// 测试：bypass_logic_test.go
func (s *Server) AttachCredential(dep config.ModelEntry) (config.ModelEntry, error) {
	return s.withCredential(dep)
}

// IncBusy increments the in-process concurrency count for a deployment.
// 参数 id（string）：部署 id，形状是 api_base|model。空串时计数没有对应的部署。
// 返回：无。该部署的进程内在途数加一。
// 调用：dataplane/host.go、dataplane/serve.go
// 测试：bypass_logic_test.go、failure_log_test.go
func (s *Server) IncBusy(id string) { s.incBusy(id) }

// DecBusy decrements the in-process concurrency count for a deployment. The caller must pair it with IncBusy so the count is not driven below what that caller added.
// 参数 id（string）：部署 id，形状是 api_base|model。必须和 IncBusy 用同一个 id。
// 返回：无。该部署的进程内在途数减一。必须和 IncBusy 成对，否则计数会少减。
// 调用：dataplane/host.go、dataplane/serve.go
// 测试：bypass_logic_test.go、failure_log_test.go
func (s *Server) DecBusy(id string) { s.decBusy(id) }

// NoteFailure records a deployment failure. An allowed_fails below 1 does not write Redis.
// 参数 id（string）：记下失败使用的主键。空串表示调用方没有指定记录。
// 返回：无。这个部署的一次失败已记下。allowed_fails 小于 1 时不写 Redis。
// 调用：dataplane/host.go、dataplane/serve.go
// 测试：bypass_logic_test.go、failure_log_test.go
func (s *Server) NoteFailure(id string, settings prefs.RouteSettings) {
	s.noteFailure(id, settings)
}

// NoteLatency records the milliseconds of one successful call. Zero is recorded too, and it means the call finished.
// 参数 id（string）：记下延迟使用的主键。空串表示调用方没有指定记录；ms（float64）：记下延迟使用的小数。0 表示没有费用或尚未计价。
// 返回：无。这次成功调用的毫秒数已记下。0 也会记，表示调用已经结束。
// 调用：dataplane/host.go、dataplane/serve.go
// 测试：bypass_logic_test.go、failure_log_test.go
func (s *Server) NoteLatency(id string, ms float64) { s.noteLatency(id, ms) }

// SetChatHeaders writes response headers such as the model name, spend, and call ID.
// 参数 w（http.ResponseWriter）：调用方的 HTTP 响应，状态和正文写在这里；p（*auth.Principal）：已经鉴权的调用方，含用户、团队、密钥哈希和别名；alias（string）：对外模型名；apiBase（string）：上游根地址，末尾斜杠会被去掉再拼路径。
// 返回：无。状态码和正文写进调用方的响应。
// 调用：dataplane/host.go、dataplane/serve.go
// 测试：bypass_logic_test.go、failure_log_test.go
func (s *Server) SetChatHeaders(w http.ResponseWriter, p *auth.Principal, alias, apiBase string) {
	s.setChatHeaders(w, p, alias, apiBase)
}

// RecordSpend records this call's tokens on the hot path and, when Redis is configured, queues the log instead of writing PostgreSQL immediately.
// 参数 w（http.ResponseWriter）：调用方的 HTTP 响应，状态和正文写在这里；p（*auth.Principal）：已经鉴权的调用方，含用户、团队、密钥哈希和别名；callID（string）：这一次调用的 id，用来把请求、响应和用量记在同一行；alias（string）：对外模型名；op（string）：操作名，例如 chat；usage（map[string]any）：用量对象。字段可能是 prompt_tokens，也可能是 input_tokens；start（time.Time）：时间范围的起点。零值表示不限制开始；cacheHit（bool）：为真时走缓存命中这一支。为假时保持原来的路径；status（int）：HTTP 状态码；depID（string）：部署 id。空串表示当前没有钉住的部署。
// 返回：无。状态码和正文写进调用方的响应。
// 调用：dataplane/host.go、dataplane/official.go、dataplane/serve.go
// 测试：bypass_logic_test.go、failure_log_test.go
func (s *Server) RecordSpend(w http.ResponseWriter, p *auth.Principal, callID, alias, op string, usage map[string]any, start time.Time, cacheHit bool, status int, depID string) {
	s.recordSpend(w, p, callID, alias, op, usage, start, cacheHit, status, depID)
}

// RememberExchange holds the request and response until this call's spend row is written.
// 参数 callID（string）：这一次调用的 id，用来把请求、响应和用量记在同一行；r（*http.Request）：入站 HTTP 请求，用来读路径、头和正文；reqBody（[]byte）：Remember交换要读的原始字节；respBody（[]byte）：Remember交换要读的原始字节。
// 返回：无。请求和响应已留到这条花费写入时再用。
// 调用：dataplane/host.go、dataplane/official.go、dataplane/serve.go
// 测试：bypass_logic_test.go、failure_log_test.go
func (s *Server) RememberExchange(callID string, r *http.Request, reqBody, respBody []byte) {
	s.rememberExchange(callID, r, reqBody, respBody)
}

// 按聊天响应 id 取回钉住的部署。这是对话粘滞，不是官方任务钉。
// 参数 taskID（string）：官方任务 id。后续查询和计费靠它找回创建时的部署。
// 返回 string（string）：读到的字符串。键不存在时为空串，调用方要把空串当成没有钉住。
// 调用：仅在 wire.go 内使用
// 测试：bypass_logic_test.go、failure_log_test.go
func (s *Server) PinnedDeployment(taskID string) string {
	return s.affinityGet("deployment_affinity:v1:response:" + taskID)
}

// 按部署 id 在当前模型表里查找。找不到时 ok 为假。
// 参数 id（string）：部署、模型或凭据 id。空串表示没有选定。
// 返回：模型行，以及是否在当前模型表里。没有时查询官方任务会得到 404。
// 调用：dataplane/host.go、dataplane/official.go、gateway/spend.go
// 测试：bypass_logic_test.go、failure_log_test.go
func (s *Server) FindDeployment(id string) (config.ModelEntry, bool) {
	for _, m := range s.Models() {
		if router.CooldownID(m) == id {
			return m, true
		}
	}
	return config.ModelEntry{}, false
}

// 记下这次调用的首字时间、供应商、缓存、部署和原始上游诊断，等记用量时取走。
// 参数 callID（string）：这一次调用的 id，用来把请求、响应和用量记在同一行；note（dataplane.CallNote）：这一次调用的附注，含首字时间、供应商、缓存和部署。
// 返回：无。业务附注保留已有诊断，重试新诊断替换旧响应；callID 为空时不记。
// 调用：dataplane/host.go、dataplane/official.go、dataplane/serve.go
// 测试：bypass_logic_test.go、failure_log_test.go
func (s *Server) AnnotateCall(callID string, note dataplane.CallNote) {
	if s == nil || callID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.callNotes == nil {
		s.callNotes = map[string]dataplane.CallNote{}
	}
	// 业务附注不得覆盖传输诊断；重试的非空诊断替换旧响应。
	if note.Upstream == nil {
		note.Upstream = s.callNotes[callID].Upstream
	}
	s.callNotes[callID] = note
}

// 取出并删除这次调用的附注。
// 参数 callID（string）：这一次调用的 id，用来把请求、响应和用量记在同一行。
// 返回 CallNote（dataplane.CallNote）：取出并删除这次调用的附注。
// 调用：gateway/spend.go
// 测试：无直接单测
func (s *Server) takeNote(callID string) dataplane.CallNote {
	s.mu.Lock()
	defer s.mu.Unlock()
	note := s.callNotes[callID]
	delete(s.callNotes, callID)
	return note
}

// noteGuardrail keeps the monitoring rows for one call until its spend log is written.
// 参数 callID（string）：这一次调用的 id，用来把请求、响应和用量记在同一行；info（string）：记下护栏使用的信息。空串表示调用方没有提供这项。
// 返回：无。这次调用的护栏监控行已留到写花费日志时再用。callID 或内容为空时不留。
// 调用：仅在 wire.go 内使用
// 测试：无直接单测
func (s *Server) noteGuardrail(callID, info string) {
	if s == nil || callID == "" || info == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.guardrailNotes == nil {
		s.guardrailNotes = map[string]string{}
	}
	s.guardrailNotes[callID] = info
}

// 取出并删除这次调用的护栏说明。没有时为空串。
// 参数 callID（string）：这一次调用的 id，用来把请求、响应和用量记在同一行。
// 返回 string（string）：取出并删除这次调用的护栏说明。没有可用值时返回空串。
// 调用：gateway/spend.go
// 测试：无直接单测
func (s *Server) takeGuardrail(callID string) string {
	if s == nil || callID == "" {
		return ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	info := s.guardrailNotes[callID]
	delete(s.guardrailNotes, callID)
	return info
}

// WriteCacheHit writes a cached body back and records a cache-hit spend row with a zero delta.
// 参数 w（http.ResponseWriter）：调用方的 HTTP 响应，状态和正文写在这里；p（*auth.Principal）：已经鉴权的调用方，含用户、团队、密钥哈希和别名；callID（string）：这一次调用的 id，用来把请求、响应和用量记在同一行；alias（string）：对外模型名；ck（string）：写入缓存命中使用的ck。空串表示调用方没有提供这项；op（string）：操作名，例如 chat；hit（[]byte）：缓存命中的响应正文；start（time.Time）：时间范围的起点。零值表示不限制开始。
// 返回：无。状态码和正文写进调用方的响应。
// 调用：dataplane/host.go、dataplane/serve.go
// 测试：bypass_logic_test.go、failure_log_test.go
func (s *Server) WriteCacheHit(w http.ResponseWriter, p *auth.Principal, callID, alias, ck, op string, hit []byte, start time.Time) {
	s.writeCacheHit(w, p, callID, alias, ck, op, hit, start)
}

// WriteChatJSON writes the upstream JSON to the caller and records spend. A non-success status is not cached as a successful body.
// 参数 w（http.ResponseWriter）：调用方的 HTTP 响应，状态和正文写在这里；p（*auth.Principal）：已经鉴权的调用方，含用户、团队、密钥哈希和别名；callID（string）：这一次调用的 id，用来把请求、响应和用量记在同一行；alias（string）：对外模型名；ck（string）：写入对话JSON使用的ck。空串表示调用方没有提供这项；op（string）：操作名，例如 chat；provider（string）：供应商标识，例如 openai 或 volcengine；respBody（[]byte）：写入对话JSON要读的原始字节；status（int）：HTTP 状态码；start（time.Time）：时间范围的起点。零值表示不限制开始；depID（string）：部署 id。空串表示当前没有钉住的部署。
// 返回：无。状态码和正文写进调用方的响应。
// 调用：dataplane/host.go、dataplane/serve.go
// 测试：bypass_logic_test.go、failure_log_test.go
func (s *Server) WriteChatJSON(w http.ResponseWriter, p *auth.Principal, callID, alias, ck, op, provider string, respBody []byte, status int, start time.Time, depID string) {
	s.writeChatJSON(w, p, callID, alias, ck, op, provider, respBody, status, start, depID)
}

// EnforceIdentityLimits checks the model allow-list, budget, and RPM or TPM. On rejection it has already written the response.
// 参数 w（http.ResponseWriter）：拒绝时把 403 或 429 写在这里；path（string）：决定错误包络的 URL 路径；p（*auth.Principal）：已经解析的调用方；alias（string）：要调用的对外模型名；est（int）：预估 token，用来扣 TPM。
// 返回 bool（bool）：模型允许名单、预算和 RPM 或 TPM 都通过时返回真。拒绝时响应已经写好。
// 调用：dataplane/host.go、dataplane/official.go、dataplane/serve.go、gateway/family/handlers.go
// 测试：bypass_logic_test.go、failure_log_test.go
func (s *Server) EnforceIdentityLimits(w http.ResponseWriter, path string, p *auth.Principal, alias string, est int) bool {
	return s.enforceIdentityLimits(w, path, p, alias, est)
}

// Redis returns the hot-path Redis client. It is nil when Redis is not configured, and spend then uses the synchronous database path.
// 参数 s：网关进程。
// 返回：热路径 Redis。nil 表示没配 Redis，花费在请求里直接写 PostgreSQL，任务钉只留在本进程内存。
// 调用：dataplane State、RecordFailure、RecordLatency、RecordUsage、Flush。
// 测试：bypass_logic_test.go、failure_log_test.go 的夹具返回 nil。
func (s *Server) Redis() *live.Client { return s.Live }

// Models returns the deployments loaded into this process. It is the model table, not an allow-list of names.
// 参数 s：网关进程。
// 返回：Cfg.ModelList。空切片表示当前没有部署，官方 Bypass 会因此找不到模型。
// 调用：dataplane Bypass 的 eligible、Runtime 的 State、gateway bypass 匹配。
// 测试：bypass_logic_test.go 用夹具模型表断言选中的密钥。
func (s *Server) Models() []config.ModelEntry { return s.Cfg.ModelList }

// BusyMap returns how many requests each deployment is handling inside this process.
// 参数 s：网关进程。
// 返回：部署 id 到正在处理的请求数。没有该 id 时视为 0。State 把它放进路由器的并发快照。
// 调用：dataplane State。
// 测试：failure_log_test.go 的夹具返回空表。
func (s *Server) BusyMap() map[string]int { return s.Busy }

// routerState returns the runtime state handed to the router. The implementation is in dataplane.
// 参数：无。
// 调用：仅在 wire.go 内使用
// 测试：无直接单测
// 返回 router.State（router.State）：这一刻的冷却、延迟、用量和并发，交给路由器排序。
func (s *Server) routerState() router.State { return dataplane.State(s) }

// noteFailure records a deployment failure and may start cooldown.
// 参数 id（string）：记下失败使用的主键。空串表示调用方没有指定记录；settings（prefs.RouteSettings）：这次请求生效的路由设置，冷却阈值从它来。
// 返回：无。这个部署的失败已记下，并可能开始冷却。
// 调用：仅在 wire.go 内使用
// 测试：无直接单测
func (s *Server) noteFailure(id string, settings prefs.RouteSettings) {
	dataplane.RecordFailure(s, id, settings)
}

// noteLatency records the latency of a successful call.
// 参数 id（string）：记下延迟使用的主键。空串表示调用方没有指定记录；ms（float64）：记下延迟使用的小数。0 表示没有费用或尚未计价。
// 返回：无。这次成功调用的延迟已记下。
// 调用：仅在 wire.go 内使用
// 测试：无直接单测
func (s *Server) noteLatency(id string, ms float64) { dataplane.RecordLatency(s, id, ms) }

// noteUsage adds tokens to the deployment usage.
// 参数 id（string）：记下用量使用的主键。空串表示调用方没有指定记录；tokens（int）：记下用量使用的整数。零表示没有这项或尚未计数。
// 返回：无。这些 token 已加进该部署的用量。
// 调用：gateway/spend.go
// 测试：无直接单测
func (s *Server) noteUsage(id string, tokens int) { dataplane.RecordUsage(s, id, tokens) }

// flushLoop flushes the spend queue every 60 seconds.
// 参数：无。
// 调用：gateway/server.go
// 测试：无直接单测
// 返回：无。每 60 秒把花费队列刷进库的循环已启动。
func (s *Server) flushLoop() { dataplane.FlushLoop(s) }

// FlushSpend writes spend and logs that are still in Redis into PostgreSQL.
// 参数：无。
// 调用：仅在 wire.go 内使用
// 测试：无直接单测
// 返回：无。还在 Redis 里的花费和日志已写入 PostgreSQL。
func (s *Server) FlushSpend() { dataplane.Flush(s) }

// RequireUser accepts any signed-in session or virtual key. On failure it has already written 401.
// 参数 w（http.ResponseWriter）：调用方的 HTTP 响应，状态码和正文写在这里；r（*http.Request）：入站 HTTP 请求，用来读路径、头和正文。
// 返回 *auth.Principal（*auth.Principal）：已经解析的调用方，含用户、团队和密钥。
// 调用：gateway/identity/gate.go、gateway/identity/handlers.go、gateway/identity/members.go、gateway/keys/admin.go
// 测试：无直接单测
func (s *Server) RequireUser(w http.ResponseWriter, r *http.Request) *auth.Principal {
	return s.requireUser(w, r)
}

// RequireManage requires a management identity. On failure it has already written 401 and returns nil.
// 参数 w（http.ResponseWriter）：调用方的 HTTP 响应，状态码和正文写在这里；r（*http.Request）：入站 HTTP 请求，用来读路径、头和正文。
// 返回 *auth.Principal（*auth.Principal）：已经解析的调用方，含用户、团队和密钥。
// 调用：gateway/family/handlers.go、gateway/family/host.go、gateway/guard/guard.go、gateway/guard/host.go
// 测试：guard_test.go
func (s *Server) RequireManage(w http.ResponseWriter, r *http.Request) *auth.Principal {
	return s.requireManage(w, r)
}

// ModelList returns a copy of the current model list. The lock is held while copying, and the caller may read the result freely afterward.
// 参数：无。
// 调用：gateway/usage/benchmarks.go、gateway/usage/chat.go、gateway/usage/host.go
// 测试：无直接单测
// 返回：模型表的副本。锁只在复制期间持有，调用方之后可以自由读。
func (s *Server) ModelList() []config.ModelEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]config.ModelEntry(nil), s.Cfg.ModelList...)
}

// ModelPublic is the public JSON for a model. Secrets inside the parameters are masked.
// 参数 m（config.ModelEntry）：一条模型部署，含对外名、供应商参数和价格覆盖。
// 返回 map[string]any（map[string]any）：模型公开的字段表。缺键表示上游或库里没有这个字段。
// 调用：仅在 wire.go 内使用
// 测试：无直接单测
func (s *Server) ModelPublic(m config.ModelEntry) map[string]any { return models.Public(m) }

// LockModels locks the model table. Pair it with UnlockModels. The lock also covers database writes during an update.
// 参数：无。
// 调用：gateway/models/admin.go、gateway/models/available.go、gateway/models/builtin.go、gateway/models/host.go
// 测试：无直接单测
// 返回：无。模型表已锁上，更新时的数据库写也盖在这把锁里。必须配 UnlockModels。
func (s *Server) LockModels() { s.mu.Lock() }

// UnlockModels releases the model-table lock.
// 参数：无。
// 调用：gateway/models/admin.go、gateway/models/available.go、gateway/models/builtin.go、gateway/models/host.go
// 测试：无直接单测
// 返回：无。模型表的锁已放开。
func (s *Server) UnlockModels() { s.mu.Unlock() }

// ModelTable returns a pointer to the in-process model slice. On a request path the caller must LockModels first.
// 参数：无。
// 调用：gateway/models/admin.go、gateway/models/available.go、gateway/models/builtin.go、gateway/models/host.go
// 测试：无直接单测
// 返回：进程内模型切片的指针。请求路径上必须先 LockModels。
func (s *Server) ModelTable() *[]config.ModelEntry { return &s.Cfg.ModelList }

// Resolve resolves the identity of the current request. On failure it does not write a response.
// 参数 r（*http.Request）：入站 HTTP 请求，用来读路径、头和正文。
// 返回 *auth.Principal（*auth.Principal）：已经解析的调用方，含用户、团队和密钥；error（error）：失败原因，nil 表示这一步成功。
// 调用：auth/auth.go、gateway/models/available.go、gateway/models/host.go、gateway/models/list.go
// 测试：无直接单测
func (s *Server) Resolve(r *http.Request) (*auth.Principal, error) { return s.resolve(r) }

// AllowLLM reports whether this identity may call inference.
// 参数 p（*auth.Principal）：已经解析的调用方，含用户、团队和密钥。
// 返回 bool（bool）：这个身份可以发起推理时返回真。主密钥默认不可以，除非打开了 allow_master_key_llm。
// 调用：gateway/models/host.go、gateway/models/list.go
// 测试：无直接单测
func (s *Server) AllowLLM(p *auth.Principal) bool { return p.CanInfer() }

// RequireLLM requires an identity that may call inference. The master key may not by default. On failure it has already written 401.
// 参数 w（http.ResponseWriter）：调用方的 HTTP 响应，状态码和正文写在这里；r（*http.Request）：入站 HTTP 请求，用来读路径、头和正文。
// 返回 *auth.Principal（*auth.Principal）：已经解析的调用方，含用户、团队和密钥。
// 调用：gateway/family/handlers.go、gateway/family/host.go
// 测试：无直接单测
func (s *Server) RequireLLM(w http.ResponseWriter, r *http.Request) *auth.Principal {
	return s.requireLLMPrincipal(w, r)
}

// RequireMixed accepts either a management identity or an inference identity. If it is neither, it has already written 401.
// 参数 w（http.ResponseWriter）：调用方的 HTTP 响应，状态码和正文写在这里；r（*http.Request）：入站 HTTP 请求，用来读路径、头和正文。
// 返回 *auth.Principal（*auth.Principal）：已经解析的调用方，含用户、团队和密钥。
// 调用：gateway/family/handlers.go、gateway/family/host.go、gateway/keys/admin.go、gateway/keys/host.go
// 测试：无直接单测
func (s *Server) RequireMixed(w http.ResponseWriter, r *http.Request) *auth.Principal {
	return s.requireMixed(w, r)
}

// Identity returns the identity store. A handler reads its rows from here, but only after Authorize has permitted the action that reads them.
// 参数：无。
// 返回 *iam.DB（*iam.DB）：身份与用量共用的库。没有 PostgreSQL 时为 nil：管理接口不能读用户和密钥，Flush 也不会把热花费落库。
// 调用：dataplane/host.go、dataplane/live.go、gateway/identity/gate.go、gateway/identity/handlers.go
// 测试：bypass_logic_test.go、failure_log_test.go
func (s *Server) Identity() *iam.DB { return s.IAM }

// RecordStore returns the framework record store: proxy models, the price-map reload plan, provider credentials and general settings. It answers no authorization question, and every identity read goes through Identity instead.
// 参数：无。
// 返回 *store.Store（*store.Store）：代理模型、价格重载计划、凭据和通用设置所在的库。它不回答鉴权。未配置时为 nil。
// 调用：gateway/family/handlers.go、gateway/family/host.go、gateway/guard/guard.go、gateway/guard/host.go
// 测试：guard_test.go
func (s *Server) RecordStore() *store.Store { return s.Store }

// Authorize decides one action for an already resolved caller and returns nil when it is permitted. It is the only way a handler may reach identity data: the object names what is being acted on and theguard reads its real ownership from the database, so a handler cannot pass a team the row does not belong to. A refusal is returned rather than written, because a handler that makes several decisionsneeds to know which one failed. WriteAuthz turns it into the response.
// 参数 r（*http.Request）：入站 HTTP 请求；p（*auth.Principal）：已经鉴权的调用方，含用户、团队、密钥哈希和别名；action（authz.Action）：要判定的动作，例如读日志、管理密钥或发起推理；obj（authz.Object）：要判定的对象，含类型、团队、组织和归属用户。
// 返回 error（error）：失败原因，nil 表示这一步成功。
// 调用：authz/decide.go、gateway/identity/gate.go、gateway/identity/handlers.go、gateway/identity/members.go 测试：authz_test.go
func (s *Server) Authorize(r *http.Request, p *auth.Principal, action authz.Action, obj authz.Object) error {
	g, err := s.guard(r.Context(), p)
	if err != nil {
		return err
	}
	return s.Authz.Authorize(r.Context(), g, action, obj)
}

// KeysScope returns the key-listing scope for an already resolved caller. The listing is narrowed in SQL by the scope rather than filtered after the rows are read, so a large table never widens what a handler returns.
// 参数 r（*http.Request）：入站 HTTP 请求，用来读路径、头和正文；p（*auth.Principal）：已经解析的调用方，含用户、团队和密钥。
// 返回 *authz.Scope（*authz.Scope）：已解析调用方的密钥列表范围，列表在 SQL 里收窄；error（error）：鉴权层没装上或成员关系读失败。nil 表示范围已经定好。
// 调用：authz/decide.go、gateway/keys/admin.go、gateway/keys/generate.go、gateway/keys/host.go
// 测试：authz_test.go
func (s *Server) KeysScope(r *http.Request, p *auth.Principal) (*authz.Scope, error) {
	g, err := s.guard(r.Context(), p)
	if err != nil {
		return nil, err
	}
	return g.KeysScope(r.Context())
}

// UsageScope returns the usage-listing scope for an already resolved caller, optionally narrowed to one team. The scope is applied as a WHERE fragment, so the narrowing happens in SQL and the roll-up is
//
//	never read in full.
//
// 参数 r（*http.Request）：入站 HTTP 请求，用来读路径、头和正文；p（*auth.Principal）：已经解析的调用方，含用户、团队和密钥；teamID（string）：团队 id。空串表示没有指定团队。
// 返回 *authz.Scope（*authz.Scope）：已解析调用方的用量列表范围，可以再收窄到一个团队；error（error）：鉴权层没装上或无权看这个团队。nil 表示范围已经定好。
// 调用：authz/decide.go、gateway/usage/activity.go、gateway/usage/entity_activity.go、gateway/usage/host.go
// 测试：authz_test.go
func (s *Server) UsageScope(r *http.Request, p *auth.Principal, teamID string) (*authz.Scope, error) {
	g, err := s.guard(r.Context(), p)
	if err != nil {
		return nil, err
	}
	return g.UsageScope(r.Context(), teamID)
}

// LogsScope returns the request-log scope for an already resolved caller: their own personal logs, plus the service-key logs of the teams they administer. A platform administrator receives every row, which the handler audits on read.
// 参数 r（*http.Request）：入站 HTTP 请求，用来读路径、头和正文；p（*auth.Principal）：已经解析的调用方，含用户、团队和密钥。
// 返回 *authz.Scope（*authz.Scope）：已解析调用方的请求日志范围。本人的个人日志，加上所管理团队的服务密钥日志；error（error）：鉴权层没装上。nil 表示范围已经定好。
// 调用：authz/decide.go、gateway/usage/host.go、gateway/usage/reports.go
// 测试：authz_test.go
func (s *Server) LogsScope(r *http.Request, p *auth.Principal) (*authz.Scope, error) {
	g, err := s.guard(r.Context(), p)
	if err != nil {
		return nil, err
	}
	return g.LogsScope(r.Context())
}

// AuditLogRead records that a platform administrator read a request log they do not own. The evidence row is the access, not the content: it names the request id and the ownership snapshot that was read
// .
// 参数 r（*http.Request）：入站 HTTP 请求；p（*auth.Principal）：已经鉴权的调用方，含用户、团队、密钥哈希和别名；requestID（string）：审计日志Read使用的请求标识。空串表示调用方没有提供这项；e（iam.UsageEvent）：审计日志Read使用的用量事件。
// 返回 error（error）：失败原因，nil 表示这一步成功。
// 调用：gateway/usage/host.go、gateway/usage/reports.go、iam/usage_read.go
// 测试：无直接单测
func (s *Server) AuditLogRead(r *http.Request, p *auth.Principal, requestID string, e iam.UsageEvent) error {
	if s.IAM == nil {
		return nil
	}
	return s.IAM.AuditLogRead(r.Context(), iam.Actor{ID: p.UserID, Kind: string(p.Kind)}, requestID, e)
}

// TeamFilter 返回这个调用方可以出现在列表里的团队 id。平台管理员得到 nil，表示不限制。
// 参数 r（*http.Request）：入站 HTTP 请求，用来读路径、头和正文；p（*auth.Principal）：已经解析的调用方，含用户、团队和密钥。
// 返回 []string（[]string）：团队过滤。没有匹配时为 nil 或空切片，调用方按长度判断；error（error）：失败原因。nil 表示这一步成功。
// 调用：authz/decide.go、gateway/identity/gate.go、gateway/identity/handlers.go
// 测试：authz_test.go
func (s *Server) TeamFilter(r *http.Request, p *auth.Principal) ([]string, error) {
	g, err := s.guard(r.Context(), p)
	if err != nil {
		return nil, err
	}
	return g.TeamFilter(), nil
}

// WriteAuthz turns a refused authorization into the response: 404 when the object does not exist or is not visible, 403 when it is visible but not permitted, 500 when the ownership could not be read. It
//
//	reports whether a response was written.
//
// 参数 w（http.ResponseWriter）：调用方的 HTTP 响应，状态码和正文写在这里；r（*http.Request）：入站 HTTP 请求，用来读路径、头和正文；err（error）：失败原因，nil 表示这一步成功。
// 返回 bool（bool）：已经把拒绝写进响应时返回真。对象不可见是 404，可见但不允许是 403，读归属失败是 500。
// 调用：gateway/identity/gate.go、gateway/identity/handlers.go、gateway/identity/members.go、gateway/keys/admin.go
// 测试：无直接单测
func (s *Server) WriteAuthz(w http.ResponseWriter, r *http.Request, err error) bool {
	if err == nil {
		return false
	}
	s.writeAuthzError(w, r, err)
	return true
}

// WriteAuthError turns a failed identification or a failed identity write into the response. It reports whether a response was written.
// 参数 w（http.ResponseWriter）：调用方的 HTTP 响应，状态码和正文写在这里；r（*http.Request）：入站 HTTP 请求，用来读路径、头和正文；err（error）：失败原因，nil 表示这一步成功。
// 返回 bool（bool）：已经把身份识别或身份写入的失败写进响应时返回真。
// 调用：仅在 wire.go 内使用
// 测试：无直接单测
func (s *Server) WriteAuthError(w http.ResponseWriter, r *http.Request, err error) bool {
	if err == nil {
		return false
	}
	s.writeAuthError(w, r, err)
	return true
}

// WriteIAMError turns a store failure into the response. It reports whether a response was written.
// 参数 w（http.ResponseWriter）：调用方的 HTTP 响应，状态码和正文写在这里；r（*http.Request）：入站 HTTP 请求，用来读路径、头和正文；err（error）：失败原因，nil 表示这一步成功。
// 返回 bool（bool）：已经把存储失败写进响应时返回真。
// 调用：gateway/identity/gate.go、gateway/identity/handlers.go、gateway/identity/members.go、gateway/keys/admin.go
// 测试：无直接单测
func (s *Server) WriteIAMError(w http.ResponseWriter, r *http.Request, err error) bool {
	if err == nil {
		return false
	}
	s.writeIAMError(w, r, err)
	return true
}

// DataPlane hands this inference call to dataplane.Serve. op is the operation name that was already recognized.
// 参数 w（http.ResponseWriter）：调用方的 HTTP 响应，状态码和正文写在这里；r（*http.Request）：入站 HTTP 请求，用来读路径、头和正文；op（string）：操作名或 call_type，写入用量行并选择协议。
// 调用：gateway/family/handlers.go、gateway/family/host.go
// 测试：无直接单测
// 返回：无。状态码和正文写进调用方的响应。
func (s *Server) DataPlane(w http.ResponseWriter, r *http.Request, op string) {
	s.dataPlane(w, r, op)
}

// Config returns the in-process config. A router-settings write changes fields that are already present on RouterSettings.
// 参数：无。
// 调用：gateway/prefs/host.go、gateway/prefs/settings.go
// 测试：无直接单测
// 返回 *config.Config（*config.Config）：当前进程配置，含模型表和路由策略，不会复制。
func (s *Server) Config() *config.Config { return s.Cfg }
