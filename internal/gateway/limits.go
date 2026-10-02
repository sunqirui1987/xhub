// Package gateway checks budget and rate and fills credentials before an upstream call. The send loop itself lives in the dataplane package.
package gateway

import (
	"net/http"
	"time"

	"github.com/sunqirui1987/xhub/internal/auth"
	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/dataplane"
	modelaccess "github.com/sunqirui1987/xhub/internal/gateway/models"
	"github.com/sunqirui1987/xhub/internal/httpx"
	"github.com/sunqirui1987/xhub/internal/llm"
	"github.com/sunqirui1987/xhub/internal/logx"
	"sync"
)

var logTraceOnceLimits sync.Once

// dataPlane hands this inference call to dataplane.Serve. Identity, budget, and the upstream loop are not reimplemented in this wrapper.
func (s *Server) dataPlane(w http.ResponseWriter, r *http.Request, op string) {
	logTraceOnceLimits.Do(func() { logx.Trace("enter gateway.dataPlane") })
	logx.Debug("process %s %s step=dataplane op=%s", r.Method, r.URL.Path, op)
	dataplane.Serve(s, w, r, op)
}

// estimateTokens calls dataplane.EstimateTokens so budget and TPM checks have an upper bound.
func estimateTokens(body map[string]any) int { return dataplane.EstimateTokens(body) }

// withCredential fills deployment parameters from the credential store using the deployment's credential name. With no name the deployment is returned unchanged.
func (s *Server) withCredential(dep config.ModelEntry) config.ModelEntry {
	name := dep.ParamString("litellm_credential_name", "")
	var values map[string]any
	if name != "" {
		if rec, err := s.Store.GetKV("credentials", name); err == nil {
			values, _ = rec["credential_values"].(map[string]any)
		}
	}
	out := dep
	out.LiteLLMParams = llm.Hydrate(dep.LiteLLMParams, values)
	return out
}

// enforceIdentityLimits checks the model allow-list, budget, and rate. On rejection it has already written the response and returns false.
func (s *Server) enforceIdentityLimits(w http.ResponseWriter, path string, p *auth.Principal, alias string, est int) bool {
	if p.Key != nil && p.Hash != "" {
		if k, err := s.Store.GetByHash(p.Hash); err == nil {
			p.Key = k
		}
	}
	if alias != "" && !modelaccess.AllowsModel(s, p, alias) {
		httpx.WriteTypedError(w, path, 401, "invalid_request_error", "model not in allowed model list")
		return false
	}
	if p.Key != nil && p.Key.MaxBudget.Valid && p.Key.Spend+s.hotSpend(p.Hash) >= p.Key.MaxBudget.Float64 {
		httpx.WriteTypedError(w, path, 429, "budget_exceeded", "Budget has been exceeded")
		return false
	}
	if p.Key != nil && p.Key.TeamID != "" {
		if team, err := s.Store.GetTeam(p.Key.TeamID); err == nil {
			if team.BlockedState() {
				httpx.WriteTypedError(w, path, 401, "invalid_request_error", "team blocked")
				return false
			}
			if team.MaxBudget.Valid && team.Spend+s.hotSpend(team.ID) >= team.MaxBudget.Float64 {
				httpx.WriteTypedError(w, path, 429, "budget_exceeded", "Team budget has been exceeded")
				return false
			}
		} else {
			httpx.WriteTypedError(w, path, 401, "invalid_request_error", "team not found")
			return false
		}
	}
	if p.Key != nil && p.Key.UserID != "" {
		if user, err := s.Store.GetUser(p.Key.UserID); err == nil {
			if user.BlockedState() {
				httpx.WriteTypedError(w, path, 401, "invalid_request_error", "user blocked")
				return false
			}
			if user.MaxBudget.Valid && user.Spend+s.hotSpend(user.ID) >= user.MaxBudget.Float64 {
				httpx.WriteTypedError(w, path, 429, "budget_exceeded", "User budget has been exceeded")
				return false
			}
		} else {
			httpx.WriteTypedError(w, path, 401, "invalid_request_error", "user not found")
			return false
		}
	}
	if p.Key != nil && p.Key.OrganizationID != "" {
		if org, err := s.Store.GetOrg(p.Key.OrganizationID); err == nil {
			if org.BlockedState() {
				httpx.WriteTypedError(w, path, 401, "invalid_request_error", "organization blocked")
				return false
			}
			if org.MaxBudget.Valid && org.Spend+s.hotSpend(org.ID) >= org.MaxBudget.Float64 {
				httpx.WriteTypedError(w, path, 429, "budget_exceeded", "Organization budget has been exceeded")
				return false
			}
		} else {
			httpx.WriteTypedError(w, path, 401, "invalid_request_error", "organization not found")
			return false
		}
	}
	if p.Key != nil && p.Key.ProjectID != "" {
		if project, err := s.Store.GetProject(p.Key.ProjectID); err == nil {
			if project.BlockedState() {
				httpx.WriteTypedError(w, path, 401, "invalid_request_error", "project blocked")
				return false
			}
			if project.MaxBudget.Valid && project.Spend+s.hotSpend(project.ID) >= project.MaxBudget.Float64 {
				httpx.WriteTypedError(w, path, 429, "budget_exceeded", "Project budget has been exceeded")
				return false
			}
		} else {
			httpx.WriteTypedError(w, path, 401, "invalid_request_error", "project not found")
			return false
		}
	}
	if p.Key != nil && !s.enforceRateLimits(w, path, p, est) {
		return false
	}
	return true
}

// hotSpend is spend still sitting in Redis. Without Redis it is 0 and the budget check uses PostgreSQL only.
func (s *Server) hotSpend(id string) float64 {
	if s.Live == nil {
		return 0
	}
	return s.Live.HotSpend(id)
}

// enforceRateLimits uses the Redis minute bucket when Redis is set, otherwise a process-local sliding window. Over the limit it writes 429 and returns false.
func (s *Server) enforceRateLimits(w http.ResponseWriter, path string, p *auth.Principal, est int) bool {
	if p.Key == nil {
		return true
	}
	if s.Live != nil {
		return s.enforceRedisRateLimits(w, path, p, est)
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

// enforceRedisRateLimits checks RPM and TPM against the Redis minute bucket. A limit of 0 is treated as already exceeded.
func (s *Server) enforceRedisRateLimits(w http.ResponseWriter, path string, p *auth.Principal, est int) bool {
	if p.Key.RPMLimit.Valid {
		n, err := s.Live.HitRPM(p.Hash)
		if err == nil && (n > p.Key.RPMLimit.Int64 || p.Key.RPMLimit.Int64 == 0) {
			httpx.WriteTypedError(w, path, 429, "rate_limit", "rpm_limit exceeded")
			return false
		}
	}
	if p.Key.TPMLimit.Valid {
		n, err := s.Live.HitTPM(p.Hash, est)
		if err == nil && (n > p.Key.TPMLimit.Int64 || p.Key.TPMLimit.Int64 == 0) {
			httpx.WriteTypedError(w, path, 429, "rate_limit", "tpm_limit exceeded")
			return false
		}
	}
	return true
}
