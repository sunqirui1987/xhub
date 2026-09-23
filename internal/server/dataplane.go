package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/sunqirui1987/xhub/internal/auth"
	"github.com/sunqirui1987/xhub/internal/cache"
	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/httpx"
	"github.com/sunqirui1987/xhub/internal/router"
)

func (s *Server) dataPlane(w http.ResponseWriter, r *http.Request, op string) {
	start := time.Now()
	callID := httpx.CallID()
	httpx.SetCallID(w, callID)
	p := s.requireLLMPrincipal(w, r)
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
	est := estimateTokens(body)
	if !s.enforceIdentityLimits(w, r.URL.Path, p, alias, est) {
		return
	}
	if op == "chat" || op == "" {
		if blocked, msg := s.preCallGuardrails(body); blocked {
			httpx.WriteTypedError(w, r.URL.Path, 400, "guardrail_failed", msg)
			return
		}
	}
	done, reason := s.Hooks.Begin(p.Key)
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

	tenant := ""
	if p.Key != nil {
		tenant = p.Hash
	}
	ck := cache.Key(tenant, op, alias, string(raw))
	stream, _ := body["stream"].(bool)
	if !stream {
		if hit, ok := s.Cache.Get(ck); ok {
			s.writeCacheHit(w, p, callID, alias, ck, hit, start)
			return
		}
	}

	pool := router.Order(s.Cfg.ModelList, alias, s.Cfg.RouterSettings.RoutingStrategy, s.Busy)
	if len(pool) == 0 {
		httpx.WriteTypedError(w, r.URL.Path, 400, "invalid_request", "model not found: "+alias)
		return
	}
	attempts := s.Cfg.RouterSettings.NumRetries
	if attempts < 1 {
		attempts = 1
	}

	var lastErr error
	var lastStatus int
	var lastProvider string
	triedHTTP := false

	for di, dep := range pool {
		if di > 0 {
			p2, err := s.resolve(r)
			if err != nil || p2 == nil || !p2.CanLLM(s.Cfg) {
				httpx.WriteTypedError(w, r.URL.Path, 401, "invalid_api_key", "Authentication Error, No api key passed in.")
				return
			}
			p = p2
			if !s.enforceIdentityLimits(w, r.URL.Path, p, alias, est) {
				return
			}
		}
		upstreamModel := dep.ParamString("model", alias)
		provider, realModel := config.SplitProviderModel(upstreamModel)
		if p := dep.ParamString("custom_llm_provider", ""); p != "" {
			provider = p
		}
		lastProvider = provider
		if !router.KnownAdapter(provider) {
			continue
		}
		apiBase := stringsTrim(dep.ParamString("api_base", "https://api.openai.com/v1"))
		apiKey := dep.ParamString("api_key", "")
		if apiKey == "" {
			continue
		}
		did := dep.ParamString("api_base", "") + "|" + upstreamModel
		payload, err := router.EncodeRequest(op, provider, body, realModel)
		if err != nil {
			lastErr = err
			continue
		}
		upURL := router.AdapterURLOp(op, provider, apiBase, realModel)

		for try := 0; try < attempts; try++ {
			triedHTTP = true
			s.incBusy(did)
			req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, upURL, bytes.NewReader(payload))
			if err != nil {
				s.decBusy(did)
				lastErr = err
				continue
			}
			req.Header.Set("Authorization", "Bearer "+apiKey)
			req.Header.Set("Content-Type", "application/json")
			resp, err := s.Client.Do(req)
			if err != nil {
				s.decBusy(did)
				lastErr = err
				continue
			}
			if resp.StatusCode >= 500 || resp.StatusCode == http.StatusTooManyRequests {
				_, _ = io.ReadAll(resp.Body)
				resp.Body.Close()
				s.decBusy(did)
				lastStatus = resp.StatusCode
				lastErr = errUpstreamStatus
				continue
			}

			s.setChatHeaders(w, p, alias, apiBase)
			if stream {
				wrote := pipeStream(w, resp)
				s.decBusy(did)
				if wrote {
					return
				}
				lastErr = errEmptyUpstream
				continue
			}
			respBody, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			s.decBusy(did)
			s.writeChatJSON(w, p, callID, alias, ck, op, provider, respBody, resp.StatusCode, start)
			return
		}
	}

	if !triedHTTP {
		msg := "provider_not_implemented"
		if lastProvider != "" {
			msg = "provider_not_implemented: " + lastProvider
		}
		httpx.WriteTypedError(w, r.URL.Path, 400, "provider_not_implemented", msg)
		return
	}
	if lastStatus > 0 {
		httpx.WriteTypedError(w, r.URL.Path, 502, "upstream_error", "upstream "+strconv.Itoa(lastStatus))
		return
	}
	if lastErr != nil {
		httpx.WriteTypedError(w, r.URL.Path, 502, "upstream_error", lastErr.Error())
		return
	}
	httpx.WriteTypedError(w, r.URL.Path, 502, "upstream_error", "all deployments failed")
}

func stringsTrim(s string) string {
	for len(s) > 0 && s[len(s)-1] == '/' {
		s = s[:len(s)-1]
	}
	return s
}

func estimateTokens(body map[string]any) int {
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
				n += len(stringifyAny(mm["content"])) / 4
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

func stringifyAny(v any) string {
	s, _ := v.(string)
	return s
}

func (s *Server) enforceIdentityLimits(w http.ResponseWriter, path string, p *auth.Principal, alias string, est int) bool {
	if p.Key != nil && p.Hash != "" {
		if k, err := s.Store.GetByHash(p.Hash); err == nil {
			p.Key = k
		}
	}
	if alias != "" && p.Key != nil && !p.Key.AllowsModel(alias) {
		httpx.WriteTypedError(w, path, 401, "invalid_request_error", "model not in allowed model list")
		return false
	}
	if p.Key != nil && p.Key.MaxBudget.Valid && p.Key.Spend >= p.Key.MaxBudget.Float64 {
		httpx.WriteTypedError(w, path, 429, "budget_exceeded", "Budget has been exceeded")
		return false
	}
	if p.Key != nil && p.Key.TeamID != "" {
		if team, err := s.Store.GetTeam(p.Key.TeamID); err == nil {
			if team.ExtraBool("blocked") {
				httpx.WriteTypedError(w, path, 401, "invalid_request_error", "team blocked")
				return false
			}
			if !team.AllowsModel(alias) {
				httpx.WriteTypedError(w, path, 401, "invalid_request_error", "model not in team allowed model list")
				return false
			}
			if team.MaxBudget.Valid && team.Spend >= team.MaxBudget.Float64 {
				httpx.WriteTypedError(w, path, 429, "budget_exceeded", "Team budget has been exceeded")
				return false
			}
		}
	}
	if p.Key != nil && p.Key.UserID != "" {
		if user, err := s.Store.GetUser(p.Key.UserID); err == nil {
			if !user.AllowsModel(alias) {
				httpx.WriteTypedError(w, path, 401, "invalid_request_error", "model not in user allowed model list")
				return false
			}
			if user.MaxBudget.Valid && user.Spend >= user.MaxBudget.Float64 {
				httpx.WriteTypedError(w, path, 429, "budget_exceeded", "User budget has been exceeded")
				return false
			}
		}
	}
	if p.Key != nil && p.Key.OrganizationID != "" {
		if org, err := s.Store.GetOrg(p.Key.OrganizationID); err == nil {
			if !org.AllowsModel(alias) {
				httpx.WriteTypedError(w, path, 401, "invalid_request_error", "model not in organization allowed model list")
				return false
			}
			if org.MaxBudget.Valid && org.Spend >= org.MaxBudget.Float64 {
				httpx.WriteTypedError(w, path, 429, "budget_exceeded", "Organization budget has been exceeded")
				return false
			}
		}
	}
	if p.Key != nil && !s.enforceRateLimits(w, path, p, est) {
		return false
	}
	return true
}

func (s *Server) enforceRateLimits(w http.ResponseWriter, path string, p *auth.Principal, est int) bool {
	if p.Key == nil {
		return true
	}
	now := time.Now()
	win := now.Add(-time.Minute)
	s.mu.Lock()
	defer s.mu.Unlock()
	hash := p.Hash
	if p.Key.RPMLimit.Valid {
		var keep []time.Time
		for _, t := range s.rpmHits[hash] {
			if t.After(win) {
				keep = append(keep, t)
			}
		}
		s.rpmHits[hash] = keep
		if int64(len(keep)) >= p.Key.RPMLimit.Int64 {
			httpx.WriteTypedError(w, path, 429, "rate_limit", "rpm_limit exceeded")
			return false
		}
		s.rpmHits[hash] = append(keep, now)
	}
	if p.Key.TPMLimit.Valid {
		var keep []tokHit
		sum := 0
		for _, h := range s.tpmHits[hash] {
			if h.t.After(win) {
				keep = append(keep, h)
				sum += h.n
			}
		}
		s.tpmHits[hash] = keep
		if int64(sum+est) > p.Key.TPMLimit.Int64 || p.Key.TPMLimit.Int64 == 0 {
			httpx.WriteTypedError(w, path, 429, "rate_limit", "tpm_limit exceeded")
			return false
		}
		s.tpmHits[hash] = append(keep, tokHit{t: now, n: est})
	}
	return true
}
