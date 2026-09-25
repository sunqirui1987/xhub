// 各子包需要的进程能力都在这里实现。方法只转发到本包已有函数，不另写策略。
package gateway

import (
	"net/http"
	"time"

	"github.com/sunqirui1987/xhub/internal/auth"
	"github.com/sunqirui1987/xhub/internal/cache"
	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/dataplane"
	"github.com/sunqirui1987/xhub/internal/gateway/guard"
	"github.com/sunqirui1987/xhub/internal/gateway/keys"
	"github.com/sunqirui1987/xhub/internal/gateway/models"
	"github.com/sunqirui1987/xhub/internal/gateway/prefs"
	"github.com/sunqirui1987/xhub/internal/hooks"
	"github.com/sunqirui1987/xhub/internal/live"
	"github.com/sunqirui1987/xhub/internal/plugin"
	"github.com/sunqirui1987/xhub/internal/router"
	"github.com/sunqirui1987/xhub/internal/store"
)

// 解析可推理的身份。失败时已经写了响应，返回 nil。
func (s *Server) RequireLLMPrincipal(w http.ResponseWriter, r *http.Request) *auth.Principal {
	return s.requireLLMPrincipal(w, r)
}

// 重新解析请求身份，供重试前确认密钥仍然有效。
func (s *Server) ResolveRequest(r *http.Request) (*auth.Principal, error) {
	return s.resolve(r)
}

// 当前进程配置，含模型列表和路由参数。
func (s *Server) GatewayConfig() *config.Config { return s.Cfg }

// 访问上游的 HTTP 客户端。超时来自路由设置。
func (s *Server) HTTPClient() *http.Client { return s.Client }

// 非流式响应的进程内缓存。
func (s *Server) ResponseCache() *cache.DualCache { return s.Cache }

// 预算和并发闸门。
func (s *Server) HookEngine() *hooks.Engine { return s.Hooks }

// Extensions 是进程启动时建的空扩展表。测试和启动代码往这里注册实现，数据面按注册顺序调用。
func (s *Server) Extensions() *plugin.Registry { return s.extensions }

// 路由器要用的并发、冷却、延迟和用量。没有 Redis 时只有进程内 Busy。
func (s *Server) RouteState() router.State { return s.routerState() }

// 合并后的路由设置。数据库键覆盖 YAML。
func (s *Server) RouterDocument() map[string]any { return prefs.MergedRouter(s) }

// 聊天请求发出前跑护栏。blocked 为真时 msg 是给调用方的原因。
func (s *Server) GuardrailBlocks(body map[string]any) (bool, string) {
	return guard.PreCall(s, body)
}

// 按 litellm_credential_name 把密钥库中的值填进部署。没有名字时原样返回。
func (s *Server) AttachCredential(dep config.ModelEntry) config.ModelEntry {
	return s.withCredential(dep)
}

// 把部署的进程内并发加一。
func (s *Server) IncBusy(id string) { s.incBusy(id) }

// 把部署的进程内并发减一。不会减到调用方没加过的次数以下由调用方保证成对。
func (s *Server) DecBusy(id string) { s.decBusy(id) }

// 记录部署失败。allowed_fails 小于 1 时不写 Redis。
func (s *Server) NoteFailure(id string) { s.noteFailure(id) }

// 记录一次成功调用的毫秒数。0 也记录，表示调用完成。
func (s *Server) NoteLatency(id string, ms float64) { s.noteLatency(id, ms) }

// 写下模型名、花费和调用 ID 等响应头。
func (s *Server) SetChatHeaders(w http.ResponseWriter, p *auth.Principal, alias, apiBase string) {
	s.setChatHeaders(w, p, alias, apiBase)
}

// 把本次 token 记入热路径，并在配置了 Redis 时把日志排队，而不是当场写 PostgreSQL。
func (s *Server) RecordSpend(w http.ResponseWriter, p *auth.Principal, callID, alias string, usage map[string]any, start time.Time, cacheHit bool, depID string) {
	s.recordSpend(w, p, callID, alias, usage, start, cacheHit, depID)
}

// 把缓存命中的正文写回，并记一笔零增量的缓存命中花费。
func (s *Server) WriteCacheHit(w http.ResponseWriter, p *auth.Principal, callID, alias, ck string, hit []byte, start time.Time) {
	s.writeCacheHit(w, p, callID, alias, ck, hit, start)
}

// 把上游 JSON 写给调用方，并记花费。状态码不是成功时不把正文当成功缓存。
func (s *Server) WriteChatJSON(w http.ResponseWriter, p *auth.Principal, callID, alias, ck, op, provider string, respBody []byte, status int, start time.Time, depID string) {
	s.writeChatJSON(w, p, callID, alias, ck, op, provider, respBody, status, start, depID)
}

// 检查模型允许列表、预算和 RPM/TPM。拒绝时已经写了响应。
func (s *Server) EnforceIdentityLimits(w http.ResponseWriter, path string, p *auth.Principal, alias string, est int) bool {
	return s.enforceIdentityLimits(w, path, p, alias, est)
}

// 热路径 Redis。未配置时为 nil，花费改走同步数据库。
func (s *Server) Redis() *live.Client { return s.Live }

// 允许的模型名。空列表表示不额外限制。
func (s *Server) Models() []config.ModelEntry { return s.Cfg.ModelList }

// 进程内每个部署正在处理的请求数。
func (s *Server) BusyMap() map[string]int { return s.Busy }

// 花费和日志要写入的 PostgreSQL 存储。
func (s *Server) SpendStore() *store.Store { return s.Store }

// 交给路由器的运行时状态。实现在 dataplane。
func (s *Server) routerState() router.State { return dataplane.State(s) }

// 记录部署失败并可能进入冷却。
func (s *Server) noteFailure(id string) { dataplane.RecordFailure(s, id) }

// 记录成功调用的延迟。
func (s *Server) noteLatency(id string, ms float64) { dataplane.RecordLatency(s, id, ms) }

// 把 token 记入部署用量。
func (s *Server) noteUsage(id string, tokens int) { dataplane.RecordUsage(s, id, tokens) }

// 每 60 秒刷一次花费队列。
func (s *Server) flushLoop() { dataplane.FlushLoop(s) }

// FlushSpend 把 Redis 中尚未落库的花费和日志写入 PostgreSQL。
func (s *Server) FlushSpend() { dataplane.Flush(s) }

// RequireManage 要求管理身份。失败时已经写了 401，返回 nil。
func (s *Server) RequireManage(w http.ResponseWriter, r *http.Request) *auth.Principal {
	return s.requireManage(w, r)
}

// DB 是管理接口使用的 PostgreSQL 存储。
func (s *Server) DB() *store.Store { return s.Store }

// MakeKey 用明文和请求体组装要入库的密钥。模型列表非法时返回错误。
func (s *Server) MakeKey(plain string, body map[string]any) (store.Key, error) {
	return keys.FromBody(plain, body)
}

// KeyJSON 是虚拟密钥的对外 JSON。includePlain 为假时不含明文。
func (s *Server) KeyJSON(k store.Key, plain string, includePlain bool) map[string]any {
	return keys.Response(k, plain, includePlain)
}

// ModelList 返回当前模型列表的副本。调用期间持有锁，返回后调用方可以随意读。
func (s *Server) ModelList() []config.ModelEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]config.ModelEntry(nil), s.Cfg.ModelList...)
}

// ModelPublic 是模型的对外 JSON。参数里的密钥会被遮罩。
func (s *Server) ModelPublic(m config.ModelEntry) map[string]any { return models.Public(m) }

// LockModels 锁住模型表。和 UnlockModels 成对，更新期间也覆盖数据库写入。
func (s *Server) LockModels() { s.mu.Lock() }

// UnlockModels 放开模型表的锁。
func (s *Server) UnlockModels() { s.mu.Unlock() }

// ModelTable 返回进程内模型切片的指针。调用方在请求路径上要先 LockModels。
func (s *Server) ModelTable() *[]config.ModelEntry { return &s.Cfg.ModelList }

// Resolve 解析当前请求的身份。失败时不写响应。
func (s *Server) Resolve(r *http.Request) (*auth.Principal, error) { return s.resolve(r) }

// AllowLLM 报告这个身份能否调用推理。判断用的是进程配置，不是模型表。
func (s *Server) AllowLLM(p *auth.Principal) bool { return p.CanLLM(s.Cfg) }

// RequireLLM 要求可以推理的身份。主密钥默认不行，失败时已经写了 401。
func (s *Server) RequireLLM(w http.ResponseWriter, r *http.Request) *auth.Principal {
	return s.requireLLMPrincipal(w, r)
}

// RequireMixed 管理身份或推理身份都可以。两者都不是时已经写了 401。
func (s *Server) RequireMixed(w http.ResponseWriter, r *http.Request) *auth.Principal {
	return s.requireMixed(w, r)
}

// DataPlane 把这次推理交给 dataplane.Serve。op 是已经识别的操作名。
func (s *Server) DataPlane(w http.ResponseWriter, r *http.Request, op string) {
	s.dataPlane(w, r, op)
}

// Config 返回进程内配置。路由设置写入会改 RouterSettings 里已经出现的字段。
func (s *Server) Config() *config.Config { return s.Cfg }
