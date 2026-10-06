// Package gateway records spend, response headers, and in-process concurrency after a successful inference. When Redis is set, the request does not write PostgreSQL.
package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/sunqirui1987/xhub/internal/auth"
	"github.com/sunqirui1987/xhub/internal/authz"
	"github.com/sunqirui1987/xhub/internal/catalog"
	"github.com/sunqirui1987/xhub/internal/gateway/prefs"
	"github.com/sunqirui1987/xhub/internal/iam"
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
		if p.Key.TPMLimit != nil {
			w.Header().Set("x-litellm-key-tpm-limit", strconv.Itoa(*p.Key.TPMLimit))
		}
		if p.Key.RPMLimit != nil {
			w.Header().Set("x-litellm-key-rpm-limit", strconv.Itoa(*p.Key.RPMLimit))
		}
		if p.Key.MaxBudget != nil {
			w.Header().Set("x-litellm-key-max-budget", catalog.Format(*p.Key.MaxBudget))
		}
		w.Header().Set("x-litellm-key-spend", catalog.Format(p.Key.Spend))
	}
}

// recordSpend records this call's spend. With Redis it updates the hot path and queues a log instead of writing PostgreSQL inside the request.
func (s *Server) recordSpend(w http.ResponseWriter, p *auth.Principal, callID, alias, op string, usage map[string]any, start time.Time, cacheHit bool, status int, depID string) {
	if usage == nil {
		usage = map[string]any{}
	}
	pt := asInt(usage["prompt_tokens"])
	ct := asInt(usage["completion_tokens"])
	total, in, out, okc := catalog.Cost(alias, pt, ct)
	// A cache hit is a request fact, but it is not a second upstream generation.
	// Keep the token metadata for observability while making the billable delta
	// explicitly zero. This must happen before both the Redis and PostgreSQL
	// persistence paths so the two paths cannot disagree.
	if cacheHit {
		total, in, out = 0, 0, 0
	}
	// LiteLLM stores response_cost or 0.0. Unknown models still get a row, with spend 0.
	spend := 0.0
	if okc {
		spend = total
		w.Header().Set("x-litellm-response-cost", catalog.Format(total))
		w.Header().Set("x-litellm-response-cost-original", catalog.Format(total))
		w.Header().Set("x-litellm-response-cost-input", catalog.Format(in))
		w.Header().Set("x-litellm-response-cost-output", catalog.Format(out))
		if p.Key != nil {
			shown := p.Key.Spend + total
			if s.Live != nil {
				shown = p.Key.Spend + s.Live.HotSpend(live.SpendRef("key", p.Hash)) + total
			}
			w.Header().Set("x-litellm-key-spend", catalog.Format(shown))
		}
	}
	hash := ""
	ownerType := ""
	teamID, userID, orgID, projectID := "", "", "", ""
	if p != nil {
		hash = p.Hash
		ownerType = p.OwnerType
		userID = p.UserID
		if p.Key != nil {
			teamID, projectID = p.Key.TeamID, deref(p.Key.ProjectID)
			// A key carries no organization of its own; its team's is the
			// billing scope above it, snapshotted onto the usage row.
			orgID = s.teamOrg(p.Key.TeamID)
			if p.Key.UserID != nil {
				userID = *p.Key.UserID
			}
		} else if p.Kind == authz.KindSession && userID != "" && s.IAM != nil {
			// A console session has no key. Leaving the owner blank makes the
			// store call it a service log with no team, which neither the caller
			// nor their organization administrator is allowed to read.
			memberships, err := s.IAM.MemberTeams(context.Background(), userID)
			if err != nil {
				logx.Error("usage team lookup failed user=%s err=%v", userID, err)
			}
			ownerType, teamID, orgID = sessionLogBinding(ownerType, teamID, orgID, memberships)
		}
	}
	end := time.Now()
	tokens := pt + ct
	if tokens == 0 {
		tokens = asInt(usage["total_tokens"])
	}
	s.noteUsage(depID, tokens)
	ex := s.takeExchange(callID)
	callType := strings.TrimSpace(op)
	if callType == "" {
		callType = "chat"
	}
	rowStatus := "success"
	if status >= 400 {
		rowStatus = "error"
		// Failed gateway requests are observations, not billable completions.
		// Keep the response usage for diagnostics but never turn an upstream 4xx/5xx
		// into spend or route-usage.
		spend = 0
		tokens = 0
	}
	note := s.takeNote(callID)
	row := live.SpendLog{
		RequestID: callID, CallType: callType, Model: alias, APIKey: hash,
		KeyID:  p.KeyID,
		Prompt: pt, Completion: ct, Spend: spend, SpendValid: true,
		Start: start.UTC().Format(time.RFC3339Nano), End: end.UTC().Format(time.RFC3339Nano),
		CacheHit: cacheHit || note.CacheHit, Status: rowStatus, OwnerType: ownerType,
		TeamID: teamID, UserID: userID, OrgID: orgID, ProjectID: projectID,
		Messages: ex.messages, Response: ex.response, ProxyRequest: ex.proxy,
		TTFTMs: note.TTFTMs, Provider: note.Provider, CacheKey: note.CacheKey,
		SessionID: note.SessionID, CachedTokens: cachedTokens(usage),
	}
	if p != nil && p.Key != nil {
		row.KeyHash = p.Key.TokenHash
		row.KeyAlias = p.Key.Name
	}
	if teamID != "" && s.IAM != nil {
		if team, err := s.IAM.GetTeam(context.Background(), teamID); err == nil && team != nil {
			row.TeamAlias = team.Name
		}
	}
	if s.Live != nil && s.Live.EnqueueSpend(row) == nil {
		return
	}
	s.persistSpend(row, spend, ex, start, end)
}

// persistSpend writes spend to PostgreSQL immediately. Requests take this path when Redis is not configured.
func (s *Server) persistSpend(row live.SpendLog, spend float64, ex promptExchange, start, end time.Time) {
	if s.IAM == nil {
		return
	}
	// The event, its stored bodies, the daily roll-up and all five billing scopes
	// commit together and deduplicate by request_id.
	rec := usageFromSpend(row, spend, ex.messages, ex.response, ex.proxy, start, end)
	if err := s.IAM.RecordUsage(context.Background(), []iam.UsageRecord{rec}); err != nil {
		logx.Error("persist spend failed: %v", err)
	}
}

// sessionLogBinding marks a console call as that person's own log. When they
// belong to one team, the row is filed there so the team and its organization
// administrator can read it. More than one membership leaves the team blank:
// the caller still sees the row, and a guess would file it under the wrong team.
func sessionLogBinding(ownerType, teamID, orgID string, memberships []iam.Membership) (string, string, string) {
	if ownerType == "" {
		ownerType = iam.OwnerPersonal
	}
	if teamID == "" && len(memberships) == 1 {
		teamID = memberships[0].TeamID
		if orgID == "" {
			orgID = memberships[0].OrganizationID
		}
	}
	return ownerType, teamID, orgID
}

// teamOrg returns the organization that owns a team, for the ownership snapshot
// written onto a usage row. A lookup failure records an empty organization rather
// than dropping the row: the spend itself is already known and must be billed.
func (s *Server) teamOrg(teamID string) string {
	if s.IAM == nil || teamID == "" {
		return ""
	}
	team, err := s.IAM.GetTeam(context.Background(), teamID)
	if err != nil {
		logx.Error("usage organization lookup failed team=%s err=%v", teamID, err)
		return ""
	}
	return team.OrganizationID
}

// deref reads an optional string. A nil pointer is the empty string.
func usageFromSpend(row live.SpendLog, spend float64, messages, response, proxy string, start, end time.Time) iam.UsageRecord {
	return iam.UsageRecord{
		RequestID: row.RequestID, TS: start, KeyID: row.KeyID, OwnerType: row.OwnerType,
		UserID: row.UserID, TeamID: row.TeamID, ProjectID: row.ProjectID, OrganizationID: row.OrgID,
		Model: row.Model, CallType: row.CallType, Status: row.Status,
		PromptTokens: row.Prompt, CompletionTokens: row.Completion, Cost: spend,
		DurationMS:  int(end.Sub(start).Milliseconds()),
		RequestBody: messages, ResponseBody: response, ProxyRequest: proxy,
		EndedAt: end, TTFTMs: row.TTFTMs, CacheHit: row.CacheHit,
		KeyHash: row.KeyHash, KeyAlias: row.KeyAlias, TeamAlias: row.TeamAlias,
		Provider: row.Provider, CachedTokens: row.CachedTokens,
		SessionID: row.SessionID, CacheKey: row.CacheKey,
	}
}

func cachedTokens(usage map[string]any) *int {
	if usage == nil {
		return nil
	}
	details, _ := usage["prompt_tokens_details"].(map[string]any)
	if details == nil {
		details, _ = usage["input_tokens_details"].(map[string]any)
	}
	if details == nil {
		return nil
	}
	raw, ok := details["cached_tokens"]
	if !ok {
		return nil
	}
	n := asInt(raw)
	return &n
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
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
	if assembled := assembleLoggedResponse(respBody, respDoc); assembled != nil {
		respDoc = assembled
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

// assembleLoggedResponse turns a stored event stream into the final response
// document the log drawer can read. A chat or Responses stream is not one JSON
// value, so it used to be kept as a raw body and the output panel stayed empty.
func assembleLoggedResponse(raw []byte, parsed any) any {
	text := ""
	switch doc := parsed.(type) {
	case map[string]any:
		if body, ok := doc["body"].(string); ok {
			text = body
		}
	}
	if text == "" && looksLikeEventStream(raw) {
		text = string(raw)
	}
	if text == "" {
		return nil
	}
	var completed map[string]any
	var outputText strings.Builder
	var chat strings.Builder
	var reasoning strings.Builder
	tools := map[int]*streamTool{}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		line = strings.TrimPrefix(line, "data:")
		line = strings.TrimSpace(line)
		if line == "" || line == "[DONE]" {
			continue
		}
		var doc map[string]any
		if json.Unmarshal([]byte(line), &doc) != nil {
			continue
		}
		if doc["type"] == "response.completed" {
			if resp, ok := doc["response"].(map[string]any); ok {
				completed = resp
			}
		}
		if doc["type"] == "response.output_text.delta" {
			if delta, ok := doc["delta"].(string); ok {
				outputText.WriteString(delta)
			}
		}
		if choices, ok := doc["choices"].([]any); ok && len(choices) > 0 {
			choice, _ := choices[0].(map[string]any)
			if delta, ok := choice["delta"].(map[string]any); ok {
				if part, ok := delta["content"].(string); ok {
					chat.WriteString(part)
				}
				if part, ok := delta["reasoning_content"].(string); ok {
					reasoning.WriteString(part)
				}
				appendStreamTools(tools, delta["tool_calls"])
			}
			if msg, ok := choice["message"].(map[string]any); ok {
				if part, ok := msg["content"].(string); ok && part != "" {
					chat.Reset()
					chat.WriteString(part)
				}
			}
		}
	}
	if completed != nil {
		return completed
	}
	if outputText.Len() > 0 {
		return map[string]any{
			"output": []any{map[string]any{
				"type": "message", "role": "assistant",
				"content": []any{map[string]any{"type": "output_text", "text": outputText.String()}},
			}},
		}
	}
	if chat.Len() > 0 || reasoning.Len() > 0 || len(tools) > 0 {
		msg := map[string]any{"role": "assistant", "content": chat.String()}
		if reasoning.Len() > 0 {
			msg["reasoning_content"] = reasoning.String()
		}
		if calls := streamToolCalls(tools); len(calls) > 0 {
			msg["tool_calls"] = calls
		}
		return map[string]any{"choices": []any{map[string]any{"message": msg}}}
	}
	return nil
}

type streamTool struct {
	id   string
	name string
	args strings.Builder
}

func appendStreamTools(dst map[int]*streamTool, raw any) {
	list, _ := raw.([]any)
	for _, item := range list {
		call, _ := item.(map[string]any)
		idx := 0
		switch n := call["index"].(type) {
		case float64:
			idx = int(n)
		case int:
			idx = n
		}
		tool := dst[idx]
		if tool == nil {
			tool = &streamTool{}
			dst[idx] = tool
		}
		if id, ok := call["id"].(string); ok && id != "" {
			tool.id = id
		}
		fn, _ := call["function"].(map[string]any)
		if name, ok := fn["name"].(string); ok && name != "" {
			tool.name = name
		}
		if args, ok := fn["arguments"].(string); ok {
			tool.args.WriteString(args)
		}
	}
}

func streamToolCalls(tools map[int]*streamTool) []any {
	if len(tools) == 0 {
		return nil
	}
	indexes := make([]int, 0, len(tools))
	for idx := range tools {
		indexes = append(indexes, idx)
	}
	sort.Ints(indexes)
	out := make([]any, 0, len(indexes))
	for _, idx := range indexes {
		tool := tools[idx]
		out = append(out, map[string]any{
			"id":   tool.id,
			"type": "function",
			"function": map[string]any{
				"name":      tool.name,
				"arguments": tool.args.String(),
			},
		})
	}
	return out
}

func looksLikeEventStream(raw []byte) bool {
	s := string(raw)
	return strings.Contains(s, "data:") && (strings.Contains(s, "event:") || strings.Contains(s, "\"choices\""))
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
func (s *Server) writeCacheHit(w http.ResponseWriter, p *auth.Principal, callID, alias, ck, op string, hit []byte, start time.Time) {
	w.Header().Set("cache_hit", "true")
	w.Header().Set("x-litellm-cache-hit", "true")
	w.Header().Set("x-litellm-cache-key", ck)
	w.Header().Set("x-litellm-model-name", alias)
	w.Header().Set("x-litellm-version", Version)
	var parsed map[string]any
	var usage map[string]any
	if json.Unmarshal(hit, &parsed) == nil {
		usage, _ = parsed["usage"].(map[string]any)
	}
	// Always write the cache-hit request log, even when the cached response has
	// no usage object. Missing usage is different from a missing request fact;
	// recordSpend will keep the row and, because cacheHit is true, charge zero.
	s.recordSpend(w, p, callID, alias, op, usage, start, true, http.StatusOK, "")
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("x-litellm-response-duration-ms", strconv.FormatInt(time.Since(start).Milliseconds(), 10))
	w.WriteHeader(200)
	_, _ = w.Write(hit)
}

// writeChatJSON writes the upstream JSON back to the client and records the spend.
func (s *Server) writeChatJSON(w http.ResponseWriter, p *auth.Principal, callID, alias, ck, op, provider string, respBody []byte, status int, start time.Time, depID string) {
	if op == "audio_speech" {
		// Speech responses do not carry chat-token usage. Do not manufacture
		// 8/2/10 tokens: that corrupts both usage reports and billing.
		s.recordSpend(w, p, callID, alias, op, nil, start, false, status, depID)
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
		// Some providers omit usage (especially non-chat operations). Keep the
		// request log, but do not turn an unknown measurement into fake tokens.
		// Providers that do return usage are normalized below.
		if usage == nil {
			usage = map[string]any{}
		}
		if _, ok := usage["prompt_tokens"]; !ok {
			usage["prompt_tokens"] = usage["input_tokens"]
			usage["completion_tokens"] = usage["output_tokens"]
		}
		s.recordSpend(w, p, callID, alias, op, usage, start, false, status, depID)
		respBody, _ = json.Marshal(parsed)
		s.Cache.Set(ck, respBody)
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("x-litellm-response-duration-ms", strconv.FormatInt(time.Since(start).Milliseconds(), 10))
	w.WriteHeader(status)
	_, _ = w.Write(respBody)
}
