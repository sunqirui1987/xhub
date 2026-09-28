// Package gateway records spend, response headers, and in-process concurrency after a successful inference. When Redis is set, the request does not write PostgreSQL.
package gateway

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/sunqirui1987/xhub/internal/auth"
	"github.com/sunqirui1987/xhub/internal/catalog"
	"github.com/sunqirui1987/xhub/internal/gateway/prefs"
	"github.com/sunqirui1987/xhub/internal/live"
	"github.com/sunqirui1987/xhub/internal/logx"
	"github.com/sunqirui1987/xhub/internal/router"
	"sync"
)

var logTraceOnceSpend sync.Once

// incBusy increments the in-process concurrency count for a deployment.
func (s *Server) incBusy(id string) {
	logTraceOnceSpend.Do(func() { logx.Trace("enter gateway.incBusy") })

	s.mu.Lock()
	s.Busy[id]++
	s.mu.Unlock()
}

// decBusy decrements the in-process concurrency count for a deployment.
func (s *Server) decBusy(id string) {
	s.mu.Lock()
	s.Busy[id]--
	s.mu.Unlock()
}

// setChatHeaders sets response headers such as the model name, spend, and latency.
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
			w.Header().Set("x-litellm-key-max-budget", catalog.Format(p.Key.MaxBudget.Float64))
		}
		w.Header().Set("x-litellm-key-spend", catalog.Format(p.Key.Spend))
	}
}

// recordSpend records this call's spend. With Redis it updates the hot path and queues a log instead of writing PostgreSQL inside the request.
func (s *Server) recordSpend(w http.ResponseWriter, p *auth.Principal, callID, alias string, usage map[string]any, start time.Time, cacheHit bool, depID string) {
	if usage == nil {
		usage = map[string]any{}
	}
	pt := asInt(usage["prompt_tokens"])
	ct := asInt(usage["completion_tokens"])
	total, in, out, okc := catalog.Cost(alias, pt, ct)
	// LiteLLM stores response_cost or 0.0. Unknown models still get a row, with spend 0.
	logged := sql.NullFloat64{Float64: 0, Valid: true}
	if okc {
		logged = sql.NullFloat64{Float64: total, Valid: true}
		w.Header().Set("x-litellm-response-cost", catalog.Format(total))
		w.Header().Set("x-litellm-response-cost-original", catalog.Format(total))
		w.Header().Set("x-litellm-response-cost-input", catalog.Format(in))
		w.Header().Set("x-litellm-response-cost-output", catalog.Format(out))
		if p.Key != nil {
			shown := p.Key.Spend + total
			if s.Live != nil {
				shown = p.Key.Spend + s.Live.HotSpend(p.Hash) + total
			}
			w.Header().Set("x-litellm-key-spend", catalog.Format(shown))
		}
	}
	hash := ""
	teamID, userID, orgID := "", "", ""
	if p != nil {
		hash = p.Hash
		userID = p.UserID
		if p.Key != nil {
			teamID, orgID = p.Key.TeamID, p.Key.OrganizationID
			if p.Key.UserID != "" {
				userID = p.Key.UserID
			}
		}
	}
	end := time.Now()
	tokens := pt + ct
	if tokens == 0 {
		tokens = asInt(usage["total_tokens"])
	}
	s.noteUsage(depID, tokens)
	ex := s.takeExchange(callID)
	row := live.SpendLog{
		RequestID: callID, CallType: "chat", Model: alias, APIKey: hash,
		Prompt: pt, Completion: ct, Spend: logged.Float64, SpendValid: logged.Valid,
		Start: start.UTC().Format(time.RFC3339Nano), End: end.UTC().Format(time.RFC3339Nano),
		CacheHit: cacheHit, Status: "success", TeamID: teamID, UserID: userID, OrgID: orgID,
		Messages: ex.messages, Response: ex.response, ProxyRequest: ex.proxy,
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
	s.persistSpend(hash, teamID, userID, orgID, callID, alias, pt, ct, logged, start, end, cacheHit, total, okc && hash != "", ex)
}

// persistSpend writes spend to PostgreSQL immediately. Requests take this path when Redis is not configured.
func (s *Server) persistSpend(hash, teamID, userID, orgID, callID, alias string, pt, ct int, logged sql.NullFloat64, start, end time.Time, cacheHit bool, total float64, charge bool, ex promptExchange) {
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
	_ = s.Store.InsertSpendLogWithPrompt(callID, "chat", alias, hash, pt, ct, logged, start, end, cacheHit, "success", userID, ex.messages, ex.response, ex.proxy)
}

// promptExchange is the request and response saved on one spend log. Empty strings mean prompt storage was off.
type promptExchange struct {
	messages string
	response string
	proxy    string
}

// promptsEnabled reports whether new spend logs should keep the request and response.
// The YAML flag wins when it is set. A database override can turn the same key on later.
func (s *Server) promptsEnabled() bool {
	if s == nil || s.Cfg == nil {
		return false
	}
	if s.Cfg.GeneralSettings.StorePromptsInSpendLogs {
		return true
	}
	if s.Store == nil {
		return false
	}
	switch v := prefs.MergedGeneral(s)["store_prompts_in_spend_logs"].(type) {
	case bool:
		return v
	case string:
		return strings.EqualFold(v, "true")
	default:
		return false
	}
}

// rememberExchange keeps one call's headers and bodies until recordSpend writes the row.
func (s *Server) rememberExchange(callID string, r *http.Request, reqBody, respBody []byte) {
	if s == nil || callID == "" || !s.promptsEnabled() {
		return
	}
	messages, response, proxy := promptJSON(r, reqBody, respBody)
	s.mu.Lock()
	if s.exchanges == nil {
		s.exchanges = map[string]promptExchange{}
	}
	s.exchanges[callID] = promptExchange{messages: messages, response: response, proxy: proxy}
	s.mu.Unlock()
}

func (s *Server) takeExchange(callID string) promptExchange {
	if s == nil {
		return promptExchange{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	ex := s.exchanges[callID]
	delete(s.exchanges, callID)
	return ex
}

// promptJSON builds the three documents the log drawer reads: messages, response, and proxy_server_request.
// Authorization and API key headers are replaced so the stored log does not keep a credential.
func promptJSON(r *http.Request, reqBody, respBody []byte) (messages, response, proxy string) {
	reqDoc := jsonDocument(reqBody)
	respDoc := jsonDocument(respBody)
	if reqDoc == nil && len(reqBody) > 0 {
		reqDoc = map[string]any{"body": string(reqBody)}
	}
	if respDoc == nil && len(respBody) > 0 {
		respDoc = map[string]any{"body": string(respBody)}
	}
	messagesDoc := reqDoc
	if m, ok := reqDoc.(map[string]any); ok {
		if msgs, exists := m["messages"]; exists {
			messagesDoc = msgs
		}
	}
	headers := map[string]string{}
	method, path := "", ""
	if r != nil {
		method = r.Method
		path = r.URL.Path
		for k, vals := range r.Header {
			headers[k] = strings.Join(vals, ", ")
		}
		redactHeaders(headers)
	}
	proxyDoc := map[string]any{
		"method":  method,
		"url":     path,
		"headers": headers,
		"body":    reqDoc,
	}
	return mustJSON(messagesDoc), mustJSON(respDoc), mustJSON(proxyDoc)
}

func jsonDocument(raw []byte) any {
	if len(raw) == 0 {
		return nil
	}
	var v any
	if json.Unmarshal(raw, &v) != nil {
		return nil
	}
	return v
}

func mustJSON(v any) string {
	if v == nil {
		return ""
	}
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}

func redactHeaders(h map[string]string) {
	for k, v := range h {
		if sensitiveHeader(k) {
			h[k] = "***"
			continue
		}
		h[k] = redactSecretText(v)
	}
}

func sensitiveHeader(name string) bool {
	switch strings.ToLower(name) {
	case "authorization", "proxy-authorization", "cookie", "set-cookie", "x-api-key", "api-key", "x-litellm-api-key", "x-goog-api-key":
		return true
	default:
		return false
	}
}

var promptSecret = regexp.MustCompile(`sk-[A-Za-z0-9_\-]+`)

func redactSecretText(s string) string {
	return promptSecret.ReplaceAllString(s, "***")
}

// writeCacheHit returns a cached body and records a cache-hit spend log.
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

// writeChatJSON writes the upstream JSON back to the client and records the spend.
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
