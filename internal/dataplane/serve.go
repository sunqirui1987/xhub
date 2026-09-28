// Package dataplane sends one inference call: it picks a deployment, encodes the request, and moves to the next deployment after a failure.
package dataplane

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/sunqirui1987/xhub/internal/cache"
	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/httpx"
	"github.com/sunqirui1987/xhub/internal/llm"

	"github.com/sunqirui1987/xhub/internal/plugin"
	"github.com/sunqirui1987/xhub/internal/router"
)

var (
	errUpstreamStatus = errors.New("upstream_error")
	errEmptyUpstream  = errors.New("empty upstream stream")
)

// Serve runs one inference. It picks deployments with the routing strategy, encodes the upstream request, and tries the next deployment after a failure.
// Serve calls EnforceIdentityLimits for the model allow-list, budget, and rate. When that check returns false the response is already written. Budget and parallel refusals from the hook engine are written here as 429.
// After those checks, and before the cache and the upstream, extensions run in registration order. A refusal writes an error and returns.
// An empty extension registry allows the call. A retry count below 1 is treated as one attempt. An empty api_base uses the provider default when one exists. A deployment that still has no key or no base is skipped.
func Serve(h Host, w http.ResponseWriter, r *http.Request, op string) {
	start := time.Now()
	callID := httpx.CallID()
	httpx.SetCallID(w, callID)
	p := h.RequireLLMPrincipal(w, r)
	if p == nil {
		return
	}
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		httpx.WriteTypedError(w, r.URL.Path, 400, "invalid_request", "invalid body")
		return
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		httpx.WriteTypedError(w, r.URL.Path, 400, "invalid_request", "invalid json")
		return
	}
	alias, _ := body["model"].(string)
	if alias == "" {
		httpx.WriteTypedError(w, r.URL.Path, 400, "invalid_request", "model required")
		return
	}
	est := EstimateTokens(body)
	if !h.EnforceIdentityLimits(w, r.URL.Path, p, alias, est) {
		return
	}
	if op == "chat" || op == "" {
		if blocked, msg := h.GuardrailBlocks(body); blocked {
			httpx.WriteTypedError(w, r.URL.Path, 400, "guardrail_failed", msg)
			return
		}
	}
	done, reason := h.HookEngine().Begin(p.Key)
	if reason == "budget" {
		httpx.WriteTypedError(w, r.URL.Path, 429, "budget_exceeded", "Budget has been exceeded")
		return
	}
	if reason == "parallel" {
		httpx.WriteTypedError(w, r.URL.Path, 429, "rate_limit", "max_parallel_requests exceeded")
		return
	}
	if done != nil {
		defer done()
	}
	if refused := applyExtensions(w, r, h, op, alias); refused {
		return
	}

	cfg := h.GatewayConfig()
	tenant := ""
	if p.Key != nil {
		tenant = p.Hash
	}
	ck := cache.Key(tenant, op, alias, string(raw))
	stream, _ := body["stream"].(bool)
	if !stream {
		if hit, ok := h.ResponseCache().Get(ck); ok {
			h.WriteCacheHit(w, p, callID, alias, ck, hit, start)
			return
		}
	}

	if err := router.ValidateStrategy(cfg.RouterSettings.RoutingStrategy); err != nil {
		httpx.WriteTypedError(w, r.URL.Path, 400, "invalid_request", err.Error())
		return
	}
	pool := router.Order(cfg.ModelList, alias, cfg.RouterSettings.RoutingStrategy, h.RouteState())
	if len(pool) == 0 {
		log.Printf("error model %s %s model not found: %s", r.Method, r.URL.Path, alias)
		httpx.WriteTypedError(w, r.URL.Path, 400, "invalid_request", "model not found: "+alias)
		return
	}
	attempts := cfg.RouterSettings.NumRetries
	if attempts < 1 {
		attempts = 1
	}

	var lastErr error
	var lastStatus int
	var lastProvider string
	triedHTTP := false
	missingCredential := false

	for di, rawDep := range pool {
		if di > 0 {
			p2, err := h.ResolveRequest(r)
			if err != nil || p2 == nil || !p2.CanLLM(cfg) {
				httpx.WriteTypedError(w, r.URL.Path, 401, "invalid_api_key", "Authentication Error, No api key passed in.")
				return
			}
			p = p2
			if !h.EnforceIdentityLimits(w, r.URL.Path, p, alias, est) {
				return
			}
		}
		dep := h.AttachCredential(rawDep)
		upstreamModel := dep.ParamString("model", alias)
		provider, realModel := config.SplitProviderModel(upstreamModel)
		if custom := dep.ParamString("custom_llm_provider", ""); custom != "" {
			provider = custom
		}
		provider = strings.ToLower(strings.TrimSpace(provider))
		lastProvider = provider
		if _, ok := llm.ProtocolGroup(provider); !ok || provider == "" {
			continue
		}
		apiBase := trimBase(dep.ParamString("api_base", ""))
		usedDefaultBase := false
		if apiBase == "" {
			apiBase = llm.DefaultAPIBase(provider)
			usedDefaultBase = apiBase != ""
		}
		apiKey := dep.ParamString("api_key", "")
		if apiKey == "" || apiBase == "" {
			missingCredential = true
			log.Printf("debug skip deployment path=%s provider=%s model=%s reason=missing api key or api base", r.URL.Path, provider, realModel)
			continue
		}
		if usedDefaultBase {
			log.Printf("debug api_base empty path=%s provider=%s model=%s using default host", r.URL.Path, provider, realModel)
		} else {
			log.Printf("debug upstream path=%s provider=%s model=%s base_host=%s", r.URL.Path, provider, realModel, baseHost(apiBase))
		}
		did := dep.ParamString("api_base", "") + "|" + upstreamModel
		built, err := llm.Build(r.Context(), llm.Request{
			Op: op, Provider: provider, APIBase: apiBase, APIKey: apiKey, Model: realModel, Body: body,
			VertexProject:  dep.ParamString("vertex_project", ""),
			VertexLocation: dep.ParamString("vertex_location", ""),
		})
		if err != nil {
			lastErr = err
			log.Printf("error upstream encode path=%s provider=%s model=%s err=%s", r.URL.Path, provider, realModel, safeErr(err))
			continue
		}

		for try := 0; try < attempts; try++ {
			triedHTTP = true
			h.IncBusy(did)
			req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, built.URL, bytes.NewReader(built.Body))
			if err != nil {
				h.DecBusy(did)
				lastErr = err
				log.Printf("error upstream request path=%s provider=%s model=%s err=%s", r.URL.Path, provider, realModel, safeErr(err))
				continue
			}
			for key, values := range built.Header {
				req.Header[key] = values
			}
			resp, err := h.HTTPClient().Do(req)
			if err != nil {
				h.DecBusy(did)
				lastErr = err
				log.Printf("error upstream dial path=%s provider=%s model=%s base_host=%s err=%s", r.URL.Path, provider, realModel, baseHost(apiBase), safeErr(err))
				continue
			}
			if resp.StatusCode >= 500 || resp.StatusCode == http.StatusTooManyRequests {
				_, _ = io.ReadAll(resp.Body)
				resp.Body.Close()
				h.DecBusy(did)
				h.NoteFailure(router.DeploymentID(rawDep))
				lastStatus = resp.StatusCode
				lastErr = errUpstreamStatus
				log.Printf("error upstream status path=%s provider=%s model=%s base_host=%s status=%d", r.URL.Path, provider, realModel, baseHost(apiBase), resp.StatusCode)
				continue
			}

			h.NoteLatency(router.DeploymentID(rawDep), float64(time.Since(start).Milliseconds()))
			h.SetChatHeaders(w, p, alias, apiBase)
			if stream {
				wrote, usage := pipeStream(w, resp)
				h.DecBusy(did)
				if wrote {
					if usage == nil {
						usage = map[string]any{"prompt_tokens": EstimateTokens(body), "completion_tokens": 0}
					}
					h.RecordSpend(w, p, callID, alias, usage, start, false, router.DeploymentID(rawDep))
					return
				}
				lastErr = errEmptyUpstream
				log.Printf("error upstream stream path=%s provider=%s model=%s base_host=%s err=%s", r.URL.Path, provider, realModel, baseHost(apiBase), safeErr(errEmptyUpstream))
				continue
			}
			respBody, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			h.DecBusy(did)
			h.WriteChatJSON(w, p, callID, alias, ck, op, provider, respBody, resp.StatusCode, start, router.DeploymentID(rawDep))
			return
		}
	}

	if !triedHTTP {
		if lastProvider != "" || missingCredential {
			log.Printf("error dataplane path=%s status=401 code=authentication_error provider=%s", r.URL.Path, lastProvider)
			httpx.WriteTypedError(w, r.URL.Path, 401, "authentication_error", "Authentication Error, No api key passed in.")
			return
		}
		log.Printf("error dataplane path=%s status=400 code=provider_not_implemented", r.URL.Path)
		httpx.WriteTypedError(w, r.URL.Path, 400, "provider_not_implemented", "provider_not_implemented")
		return
	}
	if lastStatus > 0 {
		log.Printf("error dataplane path=%s status=502 code=upstream_error detail=upstream %d", r.URL.Path, lastStatus)
		httpx.WriteTypedError(w, r.URL.Path, 502, "upstream_error", "upstream "+strconv.Itoa(lastStatus))
		return
	}
	if lastErr != nil {
		log.Printf("error dataplane path=%s status=502 code=upstream_error detail=%s", r.URL.Path, safeErr(lastErr))
		httpx.WriteTypedError(w, r.URL.Path, 502, "upstream_error", lastErr.Error())
		return
	}
	log.Printf("error dataplane path=%s status=502 code=upstream_error detail=all deployments failed", r.URL.Path)
	httpx.WriteTypedError(w, r.URL.Path, 502, "upstream_error", "all deployments failed")
}

// applyExtensions runs extensions in registration order before the upstream HTTP call. A refusal has already written the response and returns true.
// A missing status defaults to 400 and a missing error code defaults to extension_refused. With nothing registered it returns false.
func applyExtensions(w http.ResponseWriter, r *http.Request, h Host, op, alias string) bool {
	d := h.Extensions().Run(plugin.Call{Op: op, Model: alias, Path: r.URL.Path})
	for k, v := range d.Header {
		w.Header().Set(k, v)
	}
	if !d.Refuse {
		return false
	}
	status := d.Status
	if status == 0 {
		status = 400
	}
	code := d.Code
	if code == "" {
		code = "extension_refused"
	}
	msg := d.Message
	if msg == "" {
		msg = "extension refused the call"
	}
	httpx.WriteTypedError(w, r.URL.Path, status, code, msg)
	return true
}

// EstimateTokens estimates a token hold from the body length. It is not a tokenizer. It only gives budget and TPM checks an upper bound.
func EstimateTokens(body map[string]any) int {
	n := 32
	if mt := asInt(body["max_tokens"]); mt > 0 {
		n += mt
	} else {
		n += 64
	}
	switch t := body["messages"].(type) {
	case []any:
		for _, m := range t {
			if mm, ok := m.(map[string]any); ok {
				n += len(textOf(mm["content"])) / 4
			}
		}
	}
	if p, ok := body["prompt"].(string); ok {
		n += len(p) / 4
	}
	if p, ok := body["input"].(string); ok {
		n += len(p) / 4
	}
	return n
}

// textOf reads v as a string. A non-string returns an empty string.
func textOf(v any) string {
	s, _ := v.(string)
	return s
}

// asInt converts a JSON number to int. A float64 is truncated. Any other type returns 0.
func asInt(v any) int {
	switch t := v.(type) {
	case float64:
		return int(t)
	case int:
		return t
	default:
		return 0
	}
}

var secretInErr = regexp.MustCompile(`sk-[A-Za-z0-9_\-]+|(?i)bearer\s+\S+`)

// safeErr is an error string with URLs reduced to a host and secrets removed.
func safeErr(err error) string {
	if err == nil {
		return ""
	}
	s := secretInErr.ReplaceAllString(err.Error(), "***")
	return regexp.MustCompile(`https?://\S+`).ReplaceAllStringFunc(s, func(raw string) string {
		host := baseHost(strings.TrimRight(raw, `",)`))
		if host == "" {
			return "***"
		}
		return host
	})
}

// baseHost is the hostname of an upstream address. Userinfo, path, and query stay out of the log.
func baseHost(apiBase string) string {
	u, err := url.Parse(apiBase)
	if err != nil || u.Host == "" {
		return ""
	}
	return u.Hostname()
}

// trimBase removes a trailing slash from api_base so joining an endpoint path does not produce a double slash.
func trimBase(s string) string {
	for len(s) > 0 && s[len(s)-1] == '/' {
		s = s[:len(s)-1]
	}
	return s
}

// pipeStream copies upstream SSE to the client and tries to extract usage from the stream. It returns wrote false when no byte has been written yet.
func pipeStream(w http.ResponseWriter, resp *http.Response) (bool, map[string]any) {
	defer resp.Body.Close()
	buf := make([]byte, 4096)
	flusher, _ := w.(http.Flusher)
	wrote := false
	var pending []byte
	var usage map[string]any
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			if !wrote {
				w.Header().Set("Content-Type", "text/event-stream")
				w.WriteHeader(resp.StatusCode)
				wrote = true
			}
			_, _ = w.Write(buf[:n])
			if flusher != nil {
				flusher.Flush()
			}
			pending = append(pending, buf[:n]...)
			usage = streamUsage(pending, usage)
			if len(pending) > 1<<20 {
				pending = pending[len(pending)-4096:]
			}
		}
		if err != nil {
			return wrote, usage
		}
	}
}

// streamUsage finds the last usage object in the SSE buffer already received. If none is present it keeps prev.
func streamUsage(raw []byte, prev map[string]any) map[string]any {
	for _, line := range bytes.Split(raw, []byte("\n")) {
		line = bytes.TrimSpace(line)
		line = bytes.TrimPrefix(line, []byte("data:"))
		line = bytes.TrimSpace(line)
		if len(line) == 0 || bytes.Equal(line, []byte("[DONE]")) {
			continue
		}
		var doc map[string]any
		if json.Unmarshal(line, &doc) != nil {
			continue
		}
		if u, ok := doc["usage"].(map[string]any); ok {
			prev = u
		}
	}
	return prev
}
