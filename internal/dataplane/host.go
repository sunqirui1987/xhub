// 数据面依赖的网关能力。本包不引用 server，避免和 HTTP 注册循环依赖。
package dataplane

import (
	"net/http"
	"time"

	"github.com/sunqirui1987/xhub/internal/auth"
	"github.com/sunqirui1987/xhub/internal/cache"
	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/hooks"
	"github.com/sunqirui1987/xhub/internal/live"
	"github.com/sunqirui1987/xhub/internal/plugin"
	"github.com/sunqirui1987/xhub/internal/router"
	"github.com/sunqirui1987/xhub/internal/store"
)

// Host 是数据面需要的网关能力。实现留在 HTTP 进程里，本包不反向引用 server，避免循环依赖。
// 预算、限流和花费落库仍由 Host 完成；本包只决定这次请求打到哪个部署、报文怎么送出去。
type Host interface {
	RequireLLMPrincipal(w http.ResponseWriter, r *http.Request) *auth.Principal
	ResolveRequest(r *http.Request) (*auth.Principal, error)
	GatewayConfig() *config.Config
	HTTPClient() *http.Client
	ResponseCache() *cache.DualCache
	HookEngine() *hooks.Engine
	// Extensions 是推理发出前的扩展表。没有注册项时数据面照常访问上游。
	Extensions() *plugin.Registry
	RouteState() router.State
	RouterDocument() map[string]any
	GuardrailBlocks(body map[string]any) (bool, string)
	AttachCredential(dep config.ModelEntry) config.ModelEntry
	IncBusy(id string)
	DecBusy(id string)
	NoteFailure(id string)
	NoteLatency(id string, ms float64)
	SetChatHeaders(w http.ResponseWriter, p *auth.Principal, alias, apiBase string)
	RecordSpend(w http.ResponseWriter, p *auth.Principal, callID, alias string, usage map[string]any, start time.Time, cacheHit bool, depID string)
	WriteCacheHit(w http.ResponseWriter, p *auth.Principal, callID, alias, ck string, hit []byte, start time.Time)
	WriteChatJSON(w http.ResponseWriter, p *auth.Principal, callID, alias, ck, op, provider string, respBody []byte, status int, start time.Time, depID string)
	EnforceIdentityLimits(w http.ResponseWriter, path string, p *auth.Principal, alias string, est int) bool
	Redis() *live.Client
	Models() []config.ModelEntry
	BusyMap() map[string]int
	SpendStore() *store.Store
}
