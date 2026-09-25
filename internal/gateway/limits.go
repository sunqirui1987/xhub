// 调用上游前的预算、速率和凭证填充。真正发请求的循环在 dataplane 包。
package gateway

import (
	"net/http"
	"time"

	"github.com/sunqirui1987/xhub/internal/auth"
	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/dataplane"
	"github.com/sunqirui1987/xhub/internal/httpx"
	"github.com/sunqirui1987/xhub/internal/llm"
)

// 把这次推理交给 dataplane.Serve。身份、预算和上游循环不在这个包装里重写。
func (s *Server) dataPlane(w http.ResponseWriter, r *http.Request, op string) {
	dataplane.Serve(s, w, r, op)
}

// 转调 dataplane.EstimateTokens，给预算和 TPM 一个上界。
func estimateTokens(body map[string]any) int { return dataplane.EstimateTokens(body) }

// 按部署上的 credential 名字把密钥库中的值填进参数。没有名字时原样返回。
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

// 检查模型允许列表、预算和速率。拒绝时已经写好响应并返回 false。
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
	if p.Key != nil && p.Key.MaxBudget.Valid && p.Key.Spend+s.hotSpend(p.Hash) >= p.Key.MaxBudget.Float64 {
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
			if team.MaxBudget.Valid && team.Spend+s.hotSpend(team.ID) >= team.MaxBudget.Float64 {
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
			if user.MaxBudget.Valid && user.Spend+s.hotSpend(user.ID) >= user.MaxBudget.Float64 {
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
			if org.MaxBudget.Valid && org.Spend+s.hotSpend(org.ID) >= org.MaxBudget.Float64 {
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

// Redis 里还没落库的花费。没有 Redis 时为 0，预算只看 PostgreSQL。
func (s *Server) hotSpend(id string) float64 {
	if s.Live == nil {
		return 0
	}
	return s.Live.HotSpend(id)
}

// 有 Redis 时用分钟桶，否则用进程内滑窗。超限返回 false 并写 429。
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

// 用 Redis 分钟桶检查 RPM/TPM。限额为 0 也视为超限。
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
