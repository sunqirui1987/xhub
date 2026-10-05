// Package dataplane depends on gateway capabilities through Host. It does not import the server type, so HTTP registration and this package do not import each other.
package dataplane

import (
	"net/http"
	"time"

	"github.com/sunqirui1987/xhub/internal/auth"
	"github.com/sunqirui1987/xhub/internal/cache"
	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/hooks"
	"github.com/sunqirui1987/xhub/internal/iam"
	"github.com/sunqirui1987/xhub/internal/live"
	"github.com/sunqirui1987/xhub/internal/logx"
	"github.com/sunqirui1987/xhub/internal/plugin"
	"github.com/sunqirui1987/xhub/internal/router"
)

// Host is the gateway capability the data plane needs. The HTTP process implements it. This package does not import the server, which avoids an import cycle.
// Budget, rate limits, and spend persistence stay on Host. This package only chooses the deployment and how the bytes are sent.
type Host interface {
	RequireLLMPrincipal(w http.ResponseWriter, r *http.Request) *auth.Principal
	ResolveRequest(r *http.Request) (*auth.Principal, error)
	GatewayConfig() *config.Config
	HTTPClient() *http.Client
	ResponseCache() *cache.DualCache
	HookEngine() *hooks.Engine
	// Extensions is the registry that runs before inference. With nothing registered, the data plane still calls the upstream.
	Extensions() *plugin.Registry
	RouteState() router.State
	RouterDocument() map[string]any
	GuardrailBlocks(body map[string]any) (bool, string)
	AttachCredential(dep config.ModelEntry) (config.ModelEntry, error)
	IncBusy(id string)
	DecBusy(id string)
	NoteFailure(id string)
	NoteLatency(id string, ms float64)
	SetChatHeaders(w http.ResponseWriter, p *auth.Principal, alias, apiBase string)
	RecordSpend(w http.ResponseWriter, p *auth.Principal, callID, alias, op string, usage map[string]any, start time.Time, cacheHit bool, status int, depID string)
	// RememberExchange keeps this call's headers and bodies until spend is recorded. The gateway stores them only when prompt storage is on.
	RememberExchange(callID string, r *http.Request, reqBody, respBody []byte)
	WriteCacheHit(w http.ResponseWriter, p *auth.Principal, callID, alias, ck, op string, hit []byte, start time.Time)
	WriteChatJSON(w http.ResponseWriter, p *auth.Principal, callID, alias, ck, op, provider string, respBody []byte, status int, start time.Time, depID string)
	EnforceIdentityLimits(w http.ResponseWriter, path string, p *auth.Principal, alias string, est int) bool
	Redis() *live.Client
	Models() []config.ModelEntry
	BusyMap() map[string]int
	// Identity is the store the spend flusher writes usage into. It is nil when
	// PostgreSQL is not configured, and the flusher then does nothing.
	Identity() *iam.DB
}

// traceHop records that one inference call entered the data plane. The path is enough; the body and the key stay out.
func traceHop(path string) {
	logx.Trace("dataplane hop path=%s", path)
}
