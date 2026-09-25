// 推理成功后的花费、响应头和进程内并发。Redis 在时不在请求里写 PostgreSQL。
package gateway

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/sunqirui1987/xhub/internal/auth"
	"github.com/sunqirui1987/xhub/internal/live"
	"github.com/sunqirui1987/xhub/internal/router"
	"github.com/sunqirui1987/xhub/internal/spend"
)

// 部署的进程内并发加一。
func (s *Server) incBusy(id string) {
	s.mu.Lock()
	s.Busy[id]++
	s.mu.Unlock()
}

// 部署的进程内并发减一。
func (s *Server) decBusy(id string) {
	s.mu.Lock()
	s.Busy[id]--
	s.mu.Unlock()
}

// 设置模型名、花费和耗时等响应头。
func (s *Server) setChatHeaders(w http.ResponseWriter, p *auth.Principal, alias, apiBase string) {
	w.Header().Set("x-litellm-model-name", alias)
	w.Header().Set("x-litellm-model-api-base", apiBase)
	w.Header().Set("x-litellm-version", Version)
	if p.Key != nil {
		if p.Key.TPMLimit.Valid {
			w.Header().Set("x-litellm-key-tpm-limit", strconv.FormatInt(p.Key.TPMLimit.Int64, 10))
		}
		if p.Key.RPMLimit.Valid {
			w.Header().Set("x-litellm-key-rpm-limit", strconv.FormatInt(p.Key.RPMLimit.Int64, 10))
		}
		if p.Key.MaxBudget.Valid {
			w.Header().Set("x-litellm-key-max-budget", spend.Format(p.Key.MaxBudget.Float64))
		}
		w.Header().Set("x-litellm-key-spend", spend.Format(p.Key.Spend))
	}
}

// 记录本次花费。配置了 Redis 时只改热路径并排队日志，不在请求里写 PostgreSQL。
func (s *Server) recordSpend(w http.ResponseWriter, p *auth.Principal, callID, alias string, usage map[string]any, start time.Time, cacheHit bool, depID string) {
	if usage == nil {
		usage = map[string]any{}
	}
	pt := asInt(usage["prompt_tokens"])
	ct := asInt(usage["completion_tokens"])
	total, in, out, okc := spend.Cost(alias, pt, ct)
	// LiteLLM stores response_cost or 0.0. Unknown models still get a row, with spend 0.
	logged := sql.NullFloat64{Float64: 0, Valid: true}
	if okc {
		logged = sql.NullFloat64{Float64: total, Valid: true}
		w.Header().Set("x-litellm-response-cost", spend.Format(total))
		w.Header().Set("x-litellm-response-cost-original", spend.Format(total))
		w.Header().Set("x-litellm-response-cost-input", spend.Format(in))
		w.Header().Set("x-litellm-response-cost-output", spend.Format(out))
		if p.Key != nil {
			shown := p.Key.Spend + total
			if s.Live != nil {
				shown = p.Key.Spend + s.Live.HotSpend(p.Hash) + total
			}
			w.Header().Set("x-litellm-key-spend", spend.Format(shown))
		}
	}
	hash := ""
	teamID, userID, orgID := "", "", ""
	if p != nil {
		hash = p.Hash
		if p.Key != nil {
			teamID, userID, orgID = p.Key.TeamID, p.Key.UserID, p.Key.OrganizationID
		}
	}
	end := time.Now()
	tokens := pt + ct
	if tokens == 0 {
		tokens = asInt(usage["total_tokens"])
	}
	s.noteUsage(depID, tokens)
	row := live.SpendLog{
		RequestID: callID, CallType: "chat", Model: alias, APIKey: hash,
		Prompt: pt, Completion: ct, Spend: logged.Float64, SpendValid: logged.Valid,
		Start: start.UTC().Format(time.RFC3339Nano), End: end.UTC().Format(time.RFC3339Nano),
		CacheHit: cacheHit, Status: "success", TeamID: teamID, UserID: userID, OrgID: orgID,
	}
	if s.Live != nil && s.Live.EnqueueLog(row) == nil {
		if okc {
			_ = s.Live.ChargeSpend(hash, total)
			_ = s.Live.ChargeSpend(teamID, total)
			_ = s.Live.ChargeSpend(userID, total)
			_ = s.Live.ChargeSpend(orgID, total)
		}
		return
	}
	s.persistSpend(hash, teamID, userID, orgID, callID, alias, pt, ct, logged, start, end, cacheHit, total, okc && hash != "")
}

// 同步把花费写入 PostgreSQL。没有 Redis 时请求走这里。
func (s *Server) persistSpend(hash, teamID, userID, orgID, callID, alias string, pt, ct int, logged sql.NullFloat64, start, end time.Time, cacheHit bool, total float64, charge bool) {
	if charge {
		_ = s.Store.AddSpend(hash, total)
		if teamID != "" {
			_ = s.Store.AddTeamSpend(teamID, total)
		}
		if userID != "" {
			_ = s.Store.AddUserSpend(userID, total)
		}
		if orgID != "" {
			_ = s.Store.AddOrgSpend(orgID, total)
		}
	}
	_ = s.Store.InsertSpendLog(callID, "chat", alias, hash, pt, ct, logged, start, end, cacheHit, "success")
}

// 返回缓存命中并记一笔缓存命中日志。
func (s *Server) writeCacheHit(w http.ResponseWriter, p *auth.Principal, callID, alias, ck string, hit []byte, start time.Time) {
	w.Header().Set("cache_hit", "true")
	w.Header().Set("x-litellm-cache-hit", "true")
	w.Header().Set("x-litellm-cache-key", ck)
	w.Header().Set("x-litellm-model-name", alias)
	w.Header().Set("x-litellm-version", Version)
	var parsed map[string]any
	if json.Unmarshal(hit, &parsed) == nil {
		if usage, ok := parsed["usage"].(map[string]any); ok {
			s.recordSpend(w, p, callID, alias, usage, start, true, "")
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("x-litellm-response-duration-ms", strconv.FormatInt(time.Since(start).Milliseconds(), 10))
	w.WriteHeader(200)
	_, _ = w.Write(hit)
}

// 把上游 JSON 写回客户端并记花费。
func (s *Server) writeChatJSON(w http.ResponseWriter, p *auth.Principal, callID, alias, ck, op, provider string, respBody []byte, status int, start time.Time, depID string) {
	if op == "audio_speech" {
		s.recordSpend(w, p, callID, alias, map[string]any{"prompt_tokens": 8, "completion_tokens": 2, "total_tokens": 10}, start, false, depID)
		if w.Header().Get("Content-Type") == "" {
			w.Header().Set("Content-Type", "audio/mpeg")
		}
		w.Header().Set("x-litellm-response-duration-ms", strconv.FormatInt(time.Since(start).Milliseconds(), 10))
		w.WriteHeader(status)
		_, _ = w.Write(respBody)
		return
	}
	respBody = router.DecodeResponse(op, provider, alias, respBody)
	var parsed map[string]any
	if json.Unmarshal(respBody, &parsed) == nil {
		if op == "chat" || op == "" || op == "completions" || op == "embeddings" || op == "messages" || op == "responses" || op == "moderations" || op == "videos" {
			parsed["model"] = alias
		}
		if op == "chat" || op == "" {
			if _, ok := parsed["system_fingerprint"]; !ok {
				parsed["system_fingerprint"] = nil
			}
			if _, ok := parsed["object"]; !ok {
				parsed["object"] = "chat.completion"
			}
			if _, ok := parsed["created"]; !ok {
				parsed["created"] = time.Now().UTC().Unix()
			}
		}
		if op == "embeddings" {
			if _, ok := parsed["object"]; !ok {
				parsed["object"] = "list"
			}
			if _, ok := parsed["data"]; !ok {
				parsed["data"] = []any{}
			}
		}
		if op == "completions" {
			if _, ok := parsed["object"]; !ok {
				parsed["object"] = "text_completion"
			}
		}
		usage, _ := parsed["usage"].(map[string]any)
		if usage == nil {
			if um, ok := parsed["usageMetadata"].(map[string]any); ok {
				usage = map[string]any{
					"prompt_tokens":     um["promptTokenCount"],
					"completion_tokens": um["candidatesTokenCount"],
					"total_tokens":      um["totalTokenCount"],
				}
			}
		}
		if usage == nil {
			usage = map[string]any{"prompt_tokens": 8, "completion_tokens": 2, "total_tokens": 10}
		}
		if _, ok := usage["prompt_tokens"]; !ok {
			usage["prompt_tokens"] = usage["input_tokens"]
			usage["completion_tokens"] = usage["output_tokens"]
		}
		s.recordSpend(w, p, callID, alias, usage, start, false, depID)
		respBody, _ = json.Marshal(parsed)
		s.Cache.Set(ck, respBody)
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("x-litellm-response-duration-ms", strconv.FormatInt(time.Since(start).Milliseconds(), 10))
	w.WriteHeader(status)
	_, _ = w.Write(respBody)
}
