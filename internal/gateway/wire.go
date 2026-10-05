// Package gateway implements the process capabilities the subpackages need. Each method forwards to a function already in this package and does not add a new policy.
package gateway

import (
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
func (s *Server) RequireLLMPrincipal(w http.ResponseWriter, r *http.Request) *auth.Principal {
	logTraceOnceWire.Do(func() { logx.Trace("enter gateway.RequireLLMPrincipal") })

	return s.requireLLMPrincipal(w, r)
}

// ResolveRequest resolves the request identity again so a retry can confirm the key is still valid.
func (s *Server) ResolveRequest(r *http.Request) (*auth.Principal, error) {
	return s.resolve(r)
}

// GatewayConfig returns the current process config, including the model list and router parameters.
func (s *Server) GatewayConfig() *config.Config { return s.Cfg }

// HTTPClient returns the HTTP client used for upstream calls. The timeout comes from the router settings.
func (s *Server) HTTPClient() *http.Client { return s.Client }

// ResponseCache returns the in-process cache for non-streaming responses.
func (s *Server) ResponseCache() *cache.DualCache { return s.Cache }

// HookEngine returns the budget and concurrency gate.
func (s *Server) HookEngine() *hooks.Engine { return s.Hooks }

// Extensions is the empty extension registry created at process start. Tests and startup code register implementations here, and the data plane calls them in registration order.
func (s *Server) Extensions() *plugin.Registry { return s.extensions }

// RouteState returns concurrency, cooldown, latency, and usage for the router. Without Redis only in-process Busy is present.
func (s *Server) RouteState() router.State { return s.routerState() }

// RouterDocument returns the merged router settings. A database key overrides YAML.
func (s *Server) RouterDocument() map[string]any { return prefs.MergedRouter(s) }

// GuardrailBlocks runs guardrails before a chat request is sent. When blocked is true, msg is the reason shown to the caller.
func (s *Server) GuardrailBlocks(body map[string]any) (bool, string) {
	return guard.PreCall(s, body)
}

// AttachCredential fills a deployment from the secret store using litellm_credential_name. With no name the deployment is returned unchanged.
func (s *Server) AttachCredential(dep config.ModelEntry) (config.ModelEntry, error) {
	return s.withCredential(dep)
}

// IncBusy increments the in-process concurrency count for a deployment.
func (s *Server) IncBusy(id string) { s.incBusy(id) }

// DecBusy decrements the in-process concurrency count for a deployment. The caller must pair it with IncBusy so the count is not driven below what that caller added.
func (s *Server) DecBusy(id string) { s.decBusy(id) }

// NoteFailure records a deployment failure. An allowed_fails below 1 does not write Redis.
func (s *Server) NoteFailure(id string) { s.noteFailure(id) }

// NoteLatency records the milliseconds of one successful call. Zero is recorded too, and it means the call finished.
func (s *Server) NoteLatency(id string, ms float64) { s.noteLatency(id, ms) }

// SetChatHeaders writes response headers such as the model name, spend, and call ID.
func (s *Server) SetChatHeaders(w http.ResponseWriter, p *auth.Principal, alias, apiBase string) {
	s.setChatHeaders(w, p, alias, apiBase)
}

// RecordSpend records this call's tokens on the hot path and, when Redis is configured, queues the log instead of writing PostgreSQL immediately.
func (s *Server) RecordSpend(w http.ResponseWriter, p *auth.Principal, callID, alias, op string, usage map[string]any, start time.Time, cacheHit bool, status int, depID string) {
	s.recordSpend(w, p, callID, alias, op, usage, start, cacheHit, status, depID)
}

// RememberExchange holds the request and response until this call's spend row is written.
func (s *Server) RememberExchange(callID string, r *http.Request, reqBody, respBody []byte) {
	s.rememberExchange(callID, r, reqBody, respBody)
}

// WriteCacheHit writes a cached body back and records a cache-hit spend row with a zero delta.
func (s *Server) WriteCacheHit(w http.ResponseWriter, p *auth.Principal, callID, alias, ck, op string, hit []byte, start time.Time) {
	s.writeCacheHit(w, p, callID, alias, ck, op, hit, start)
}

// WriteChatJSON writes the upstream JSON to the caller and records spend. A non-success status is not cached as a successful body.
func (s *Server) WriteChatJSON(w http.ResponseWriter, p *auth.Principal, callID, alias, ck, op, provider string, respBody []byte, status int, start time.Time, depID string) {
	s.writeChatJSON(w, p, callID, alias, ck, op, provider, respBody, status, start, depID)
}

// EnforceIdentityLimits checks the model allow-list, budget, and RPM or TPM. On rejection it has already written the response.
func (s *Server) EnforceIdentityLimits(w http.ResponseWriter, path string, p *auth.Principal, alias string, est int) bool {
	return s.enforceIdentityLimits(w, path, p, alias, est)
}

// Redis returns the hot-path Redis client. It is nil when Redis is not configured, and spend then uses the synchronous database path.
func (s *Server) Redis() *live.Client { return s.Live }

// Models returns the allowed model names. An empty list means no extra restriction.
func (s *Server) Models() []config.ModelEntry { return s.Cfg.ModelList }

// BusyMap returns how many requests each deployment is handling inside this process.
func (s *Server) BusyMap() map[string]int { return s.Busy }

// routerState returns the runtime state handed to the router. The implementation is in dataplane.
func (s *Server) routerState() router.State { return dataplane.State(s) }

// noteFailure records a deployment failure and may start cooldown.
func (s *Server) noteFailure(id string) { dataplane.RecordFailure(s, id) }

// noteLatency records the latency of a successful call.
func (s *Server) noteLatency(id string, ms float64) { dataplane.RecordLatency(s, id, ms) }

// noteUsage adds tokens to the deployment usage.
func (s *Server) noteUsage(id string, tokens int) { dataplane.RecordUsage(s, id, tokens) }

// flushLoop flushes the spend queue every 60 seconds.
func (s *Server) flushLoop() { dataplane.FlushLoop(s) }

// FlushSpend writes spend and logs that are still in Redis into PostgreSQL.
func (s *Server) FlushSpend() { dataplane.Flush(s) }

// RequireUser accepts any signed-in session or virtual key. On failure it has already written 401.
func (s *Server) RequireUser(w http.ResponseWriter, r *http.Request) *auth.Principal {
	return s.requireUser(w, r)
}

// RequireManage requires a management identity. On failure it has already written 401 and returns nil.
func (s *Server) RequireManage(w http.ResponseWriter, r *http.Request) *auth.Principal {
	return s.requireManage(w, r)
}

// ModelList returns a copy of the current model list. The lock is held while copying, and the caller may read the result freely afterward.
func (s *Server) ModelList() []config.ModelEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]config.ModelEntry(nil), s.Cfg.ModelList...)
}

// ModelPublic is the public JSON for a model. Secrets inside the parameters are masked.
func (s *Server) ModelPublic(m config.ModelEntry) map[string]any { return models.Public(m) }

// LockModels locks the model table. Pair it with UnlockModels. The lock also covers database writes during an update.
func (s *Server) LockModels() { s.mu.Lock() }

// UnlockModels releases the model-table lock.
func (s *Server) UnlockModels() { s.mu.Unlock() }

// ModelTable returns a pointer to the in-process model slice. On a request path the caller must LockModels first.
func (s *Server) ModelTable() *[]config.ModelEntry { return &s.Cfg.ModelList }

// Resolve resolves the identity of the current request. On failure it does not write a response.
func (s *Server) Resolve(r *http.Request) (*auth.Principal, error) { return s.resolve(r) }

// AllowLLM reports whether this identity may call inference.
func (s *Server) AllowLLM(p *auth.Principal) bool { return p.CanInfer() }

// RequireLLM requires an identity that may call inference. The master key may not by default. On failure it has already written 401.
func (s *Server) RequireLLM(w http.ResponseWriter, r *http.Request) *auth.Principal {
	return s.requireLLMPrincipal(w, r)
}

// RequireMixed accepts either a management identity or an inference identity. If it is neither, it has already written 401.
func (s *Server) RequireMixed(w http.ResponseWriter, r *http.Request) *auth.Principal {
	return s.requireMixed(w, r)
}

// Identity returns the identity store. A handler reads its rows from here, but
// only after Authorize has permitted the action that reads them.
func (s *Server) Identity() *iam.DB { return s.IAM }

// RecordStore returns the framework record store: proxy models, the price-map
// reload plan, provider credentials and general settings. It answers no
// authorization question, and every identity read goes through Identity instead.
func (s *Server) RecordStore() *store.Store { return s.Store }

// Authorize decides one action for an already resolved caller and returns nil
// when it is permitted. It is the only way a handler may reach identity data:
// the object names what is being acted on and the guard reads its real
// ownership from the database, so a handler cannot pass a team the row does not
// belong to.
//
// A refusal is returned rather than written, because a handler that makes
// several decisions needs to know which one failed. WriteAuthz turns it into the
// response.
func (s *Server) Authorize(r *http.Request, p *auth.Principal, action authz.Action, obj authz.Object) error {
	g, err := s.guard(r.Context(), p)
	if err != nil {
		return err
	}
	return s.Authz.Authorize(r.Context(), g, action, obj)
}

// KeysScope returns the key-listing scope for an already resolved caller. The
// listing is narrowed in SQL by the scope rather than filtered after the rows
// are read, so a large table never widens what a handler returns.
func (s *Server) KeysScope(r *http.Request, p *auth.Principal) (*authz.Scope, error) {
	g, err := s.guard(r.Context(), p)
	if err != nil {
		return nil, err
	}
	return g.KeysScope(r.Context())
}

// UsageScope returns the usage-listing scope for an already resolved caller,
// optionally narrowed to one team. The scope is applied as a WHERE fragment, so
// the narrowing happens in SQL and the roll-up is never read in full.
func (s *Server) UsageScope(r *http.Request, p *auth.Principal, teamID string) (*authz.Scope, error) {
	g, err := s.guard(r.Context(), p)
	if err != nil {
		return nil, err
	}
	return g.UsageScope(r.Context(), teamID)
}

// LogsScope returns the request-log scope for an already resolved caller: their
// own personal logs, plus the service-key logs of the teams they administer. A
// platform administrator receives every row, which the handler audits on read.
func (s *Server) LogsScope(r *http.Request, p *auth.Principal) (*authz.Scope, error) {
	g, err := s.guard(r.Context(), p)
	if err != nil {
		return nil, err
	}
	return g.LogsScope(r.Context())
}

// AuditLogRead records that a platform administrator read a request log they do
// not own. The evidence row is the access, not the content: it names the request
// id and the ownership snapshot that was read.
func (s *Server) AuditLogRead(r *http.Request, p *auth.Principal, requestID string, e iam.UsageEvent) error {
	if s.IAM == nil {
		return nil
	}
	return s.IAM.AuditLogRead(r.Context(), iam.Actor{ID: p.UserID, Kind: string(p.Kind)}, requestID, e)
}

// TeamFilter returns the teams whose rows a listing may include. A nil slice
// means every team, which only a platform administrator receives; an empty slice
// means no team. It is fail-closed: a guard that could not be built is an error
// rather than an unfiltered listing.
func (s *Server) TeamFilter(r *http.Request, p *auth.Principal) ([]string, error) {
	g, err := s.guard(r.Context(), p)
	if err != nil {
		return nil, err
	}
	return g.TeamFilter(), nil
}

// WriteAuthz turns a refused authorization into the response: 404 when the
// object does not exist or is not visible, 403 when it is visible but not
// permitted, 500 when the ownership could not be read. It reports whether a
// response was written.
func (s *Server) WriteAuthz(w http.ResponseWriter, r *http.Request, err error) bool {
	if err == nil {
		return false
	}
	s.writeAuthzError(w, r, err)
	return true
}

// WriteAuthError turns a failed identification or a failed identity write into
// the response. It reports whether a response was written.
func (s *Server) WriteAuthError(w http.ResponseWriter, r *http.Request, err error) bool {
	if err == nil {
		return false
	}
	s.writeAuthError(w, r, err)
	return true
}

// WriteIAMError turns a store failure into the response. It reports whether a
// response was written.
func (s *Server) WriteIAMError(w http.ResponseWriter, r *http.Request, err error) bool {
	if err == nil {
		return false
	}
	s.writeIAMError(w, r, err)
	return true
}

// DataPlane hands this inference call to dataplane.Serve. op is the operation name that was already recognized.
func (s *Server) DataPlane(w http.ResponseWriter, r *http.Request, op string) {
	s.dataPlane(w, r, op)
}

// Config returns the in-process config. A router-settings write changes fields that are already present on RouterSettings.
func (s *Server) Config() *config.Config { return s.Cfg }
