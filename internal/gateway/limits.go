// Package gateway checks budget and rate and fills credentials before an upstream call. The send loop itself lives in the dataplane package.
package gateway

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/sunqirui1987/xhub/internal/auth"
	"github.com/sunqirui1987/xhub/internal/authz"
	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/dataplane"
	modelaccess "github.com/sunqirui1987/xhub/internal/gateway/models"
	"github.com/sunqirui1987/xhub/internal/httpx"
	"github.com/sunqirui1987/xhub/internal/iam"
	"github.com/sunqirui1987/xhub/internal/live"
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
var (
	errCredentialUnavailable = errors.New("credential_unavailable")
	errCredentialInvalid     = errors.New("credential_invalid")
)

func (s *Server) withCredential(dep config.ModelEntry) (config.ModelEntry, error) {
	name := dep.ParamString("litellm_credential_name", "")
	var values map[string]any
	if name != "" {
		if s.Store == nil {
			return dep, errCredentialUnavailable
		}
		rec, err := s.Store.GetKV("credentials", name)
		if err != nil {
			logx.Error("credential lookup failed name=%s err=%v", name, err)
			return dep, errCredentialUnavailable
		} else {
			values, _ = rec["credential_values"].(map[string]any)
			if values == nil {
				logx.Error("credential invalid name=%s reason=missing credential_values", name)
				return dep, errCredentialInvalid
			}
			fillBuiltinKey(name, values)
		}
	}
	out := dep
	out.LiteLLMParams = llm.Hydrate(dep.LiteLLMParams, values)
	return out, nil
}

// fillBuiltinKey uses the provider environment variable when the stored key is blank.
// A first install with no key writes an empty credential and then never reads the
// environment again. The call still picks up QINIU_API_KEY or FENNOAI_API_KEY.
func fillBuiltinKey(name string, values map[string]any) {
	if text, _ := values["api_key"].(string); strings.TrimSpace(text) != "" {
		return
	}
	for _, spec := range modelaccess.Builtins() {
		if spec.ID != name {
			continue
		}
		if key := strings.TrimSpace(os.Getenv(spec.KeyEnv)); key != "" {
			values["api_key"] = key
		}
		return
	}
}

// enforceIdentityLimits checks the model allow-list, budget, and rate. On rejection it has already written the response and returns false.
//
// The budget is checked from the narrowest scope outwards, so the refusal names
// the scope that is actually exhausted: the owner user, then the key, its
// project, its team, and finally the team's organization. Each scope compares
// its stored spend plus the spend still hot in Redis against its ceiling.
func (s *Server) enforceIdentityLimits(w http.ResponseWriter, path string, p *auth.Principal, alias string, est int) bool {
	if s.IAM == nil {
		httpx.WriteTypedError(w, path, http.StatusServiceUnavailable, "authz_unavailable", "authorization is temporarily unavailable")
		return false
	}
	ctx := context.Background()
	if p.Kind == authz.KindSession {
		user, err := s.IAM.GetUser(ctx, p.UserID)
		if err != nil {
			httpx.WriteTypedError(w, path, 401, "invalid_request_error", "user not found")
			return false
		}
		if !user.Active() {
			httpx.WriteTypedError(w, path, 401, "invalid_request_error", "user not found or blocked")
			return false
		}
		if overBudget(user.MaxBudget, user.Spend, s.hotSpendRef("user", user.ID)) {
			httpx.WriteTypedError(w, path, 429, "budget_exceeded", "User budget has been exceeded")
			return false
		}
	}
	if p.Key != nil {
		if err := s.keyBudgetOK(ctx, p); err != nil {
			budgetRefusal(w, path, err)
			return false
		}
	}
	if alias != "" && !modelaccess.AllowsModel(s, ctx, p, p.TeamID, alias) {
		httpx.WriteTypedError(w, path, 401, "invalid_request_error", "model not in allowed model list")
		return false
	}
	return p.Key == nil || s.enforceRateLimits(w, path, p, est)
}

// keyBudgetOK walks the key's ownership chain and returns the first scope that is
// over budget. The key row is re-read so a spend or status change is visible
// immediately, and every ceiling above it is the live database value. A missing
// parent scope is an error rather than a skip: it means the row was deleted
// while the key still pointed at it.
func (s *Server) keyBudgetOK(ctx context.Context, p *auth.Principal) error {
	k, err := s.IAM.GetKey(ctx, p.KeyID)
	if err != nil {
		return errKeyGone
	}
	p.Key = k
	if k.Status != iam.StatusActive {
		return errKeyUnusable
	}
	if k.ExpiresAt != nil && !time.Now().Before(*k.ExpiresAt) {
		return errKeyUnusable
	}
	if k.UserID != nil {
		owner, err := s.IAM.GetUser(ctx, *k.UserID)
		if err != nil {
			return errKeyGone
		}
		if !owner.Active() {
			return errKeyUnusable
		}
		if overBudget(owner.MaxBudget, owner.Spend, s.hotSpendRef("user", owner.ID)) {
			return errBudget{scope: "User"}
		}
	}
	if overBudget(k.MaxBudget, k.Spend, s.hotSpendRef("key", p.Hash)) {
		return errBudget{scope: "Key"}
	}
	if k.ProjectID != nil && *k.ProjectID != "" {
		project, err := s.IAM.GetProject(ctx, *k.ProjectID)
		if err != nil {
			return errKeyGone
		}
		if overBudget(project.MaxBudget, project.Spend, s.hotSpendRef("project", project.ID)) {
			return errBudget{scope: "Project"}
		}
	}
	team, err := s.IAM.GetTeam(ctx, k.TeamID)
	if err != nil {
		return errKeyGone
	}
	if overBudget(team.MaxBudget, team.Spend, s.hotSpendRef("team", team.ID)) {
		return errBudget{scope: "Team"}
	}
	org, err := s.IAM.GetOrg(ctx, team.OrganizationID)
	if err != nil {
		return errKeyGone
	}
	if overBudget(org.MaxBudget, org.Spend, s.hotSpendRef("org", org.ID)) {
		return errBudget{scope: "Organization"}
	}
	return nil
}

// errKeyGone and errKeyUnusable separate "the credential no longer resolves" from
// "a scope is over budget", because only the first is the caller's problem.
var (
	errKeyGone     = errors.New("key not found")
	errKeyUnusable = errors.New("key blocked or expired")
)

// errBudget names the exhausted scope. The message mirrors LiteLLM's wording,
// which keeps the "<Scope> budget has been exceeded" text the console shows.
type errBudget struct{ scope string }

func (e errBudget) Error() string { return e.scope + " budget has been exceeded" }

// budgetRefusal writes the response for a failed ownership walk.
func budgetRefusal(w http.ResponseWriter, path string, err error) {
	var over errBudget
	if errors.As(err, &over) {
		httpx.WriteTypedError(w, path, 429, "budget_exceeded", over.Error())
		return
	}
	httpx.WriteTypedError(w, path, 401, "invalid_request_error", err.Error())
}

// overBudget reports a scope that has reached its ceiling. Without a ceiling the
// scope is unlimited, and stored plus hot spend is what the caller has used.
func overBudget(ceiling *float64, spent, hot float64) bool {
	return ceiling != nil && spent+hot >= *ceiling
}

// hotSpend is spend still sitting in Redis. Without Redis it is 0 and the budget check uses PostgreSQL only.
func (s *Server) hotSpend(id string) float64 {
	if s.Live == nil {
		return 0
	}
	return s.Live.HotSpend(id)
}

func (s *Server) hotSpendRef(kind, id string) float64 {
	if s.Live == nil {
		return 0
	}
	return s.Live.HotSpend(live.SpendRef(kind, id))
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
	if p.Key.RPMLimit != nil {
		var keep []time.Time
		for _, t := range s.rpmHits[hash] {
			if t.After(win) {
				keep = append(keep, t)
			}
		}
		s.rpmHits[hash] = keep
		if int64(len(keep)) >= int64(*p.Key.RPMLimit) {
			httpx.WriteTypedError(w, path, 429, "rate_limit", "rpm_limit exceeded")
			return false
		}
		s.rpmHits[hash] = append(keep, now)
	}
	if p.Key.TPMLimit != nil {
		var keep []tokHit
		sum := 0
		for _, h := range s.tpmHits[hash] {
			if h.t.After(win) {
				keep = append(keep, h)
				sum += h.n
			}
		}
		s.tpmHits[hash] = keep
		if *p.Key.TPMLimit == 0 || int64(sum+est) > int64(*p.Key.TPMLimit) {
			httpx.WriteTypedError(w, path, 429, "rate_limit", "tpm_limit exceeded")
			return false
		}
		s.tpmHits[hash] = append(keep, tokHit{t: now, n: est})
	}
	return true
}

// enforceRedisRateLimits checks RPM and TPM against the Redis minute bucket. A limit of 0 is treated as already exceeded.
func (s *Server) enforceRedisRateLimits(w http.ResponseWriter, path string, p *auth.Principal, est int) bool {
	if p.Key.RPMLimit != nil {
		n, err := s.Live.HitRPM(p.Hash)
		if err != nil {
			httpx.WriteTypedError(w, path, http.StatusServiceUnavailable, "rate_limit_unavailable", "rate limiting is temporarily unavailable")
			return false
		}
		if *p.Key.RPMLimit == 0 || n > int64(*p.Key.RPMLimit) {
			httpx.WriteTypedError(w, path, 429, "rate_limit", "rpm_limit exceeded")
			return false
		}
	}
	if p.Key.TPMLimit != nil {
		n, err := s.Live.HitTPM(p.Hash, est)
		if err != nil {
			httpx.WriteTypedError(w, path, http.StatusServiceUnavailable, "rate_limit_unavailable", "rate limiting is temporarily unavailable")
			return false
		}
		if *p.Key.TPMLimit == 0 || n > int64(*p.Key.TPMLimit) {
			httpx.WriteTypedError(w, path, 429, "rate_limit", "tpm_limit exceeded")
			return false
		}
	}
	return true
}
