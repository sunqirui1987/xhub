package gateway

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/sunqirui1987/xhub/internal/auth"
	"github.com/sunqirui1987/xhub/internal/dataplane"
)

// legacyClaudeUserID is Claude Code's older metadata.user_id:
// user_{device}_account_{account}_session_{uuid}.
var legacyClaudeUserID = regexp.MustCompile(`^user_[a-fA-F0-9]{64}_account_[a-fA-F0-9-]*_session_([a-fA-F0-9-]{36})$`)

const affinityTTL = time.Hour

// PlanRoute resolves the session and any deployment already pinned to it.
// previous_response_id wins, then an explicit session, then a stable prompt prefix.
func (s *Server) PlanRoute(r *http.Request, alias string, body map[string]any, p *auth.Principal) dataplane.RoutePlan {
	plan := dataplane.RoutePlan{Alias: alias, Caller: callerScope(p)}
	if body != nil {
		if prev := strings.TrimSpace(asString(body["previous_response_id"])); prev != "" {
			if id := s.affinityGet("deployment_affinity:v1:response:" + prev); id != "" {
				plan.Pinned = id
			}
		}
	}
	plan.SessionID = sessionID(r, body)
	if plan.Pinned == "" && plan.SessionID != "" && plan.Caller != "" {
		plan.Pinned = s.affinityGet(sessionPinKey(alias, plan.Caller, plan.SessionID))
	}
	return plan
}

// CommitRoute remembers which deployment served this session and, when the
// response has an id, which deployment produced it.
func (s *Server) CommitRoute(plan dataplane.RoutePlan, deploymentID, responseID string) {
	if deploymentID == "" {
		return
	}
	if plan.SessionID != "" && plan.Caller != "" {
		s.affinitySet(sessionPinKey(plan.Alias, plan.Caller, plan.SessionID), deploymentID)
	}
	if responseID != "" {
		s.affinitySet("deployment_affinity:v1:response:"+responseID, deploymentID)
	}
}

func sessionPinKey(alias, caller, sessionID string) string {
	sum := sha256.Sum256([]byte(caller))
	return "deployment_affinity:v1:session:" + alias + ":" + hex.EncodeToString(sum[:8]) + ":" + sessionID
}

func callerScope(p *auth.Principal) string {
	if p == nil {
		return ""
	}
	if p.Hash != "" {
		return "key:" + p.Hash
	}
	if p.UserID != "" {
		return "user:" + p.UserID
	}
	return ""
}

// sessionID is the stable conversation id. The order follows sub2api: a client
// session wins, then a cache key, then the previous response, then a hash of
// the prompt prefix that stays the same across turns.
func sessionID(r *http.Request, body map[string]any) string {
	if r != nil {
		for _, name := range []string{
			"Session-Id", "Session_id", "Conversation_id",
			"X-Session-Affinity", "X-Session-Id", "X-Opencode-Session", "X-Conversation-Id",
		} {
			if v := strings.TrimSpace(r.Header.Get(name)); v != "" {
				return v
			}
		}
	}
	if body == nil {
		return ""
	}
	if meta, ok := body["metadata"].(map[string]any); ok {
		if v := claudeSession(asString(meta["user_id"])); v != "" {
			return v
		}
		if v := firstString(meta, "session_id", "litellm_session_id", "prompt_cache_key"); v != "" {
			return v
		}
	}
	if v := firstString(body, "prompt_cache_key", "litellm_session_id"); v != "" {
		return v
	}
	if prev := strings.TrimSpace(asString(body["previous_response_id"])); strings.HasPrefix(prev, "resp_") {
		return "prev:" + prev
	}
	return contentSessionID(body)
}

// claudeSession reads the session uuid Claude Code puts in metadata.user_id.
func claudeSession(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(raw, "{") {
		var doc struct {
			SessionID string `json:"session_id"`
		}
		if json.Unmarshal([]byte(raw), &doc) == nil && doc.SessionID != "" {
			return doc.SessionID
		}
	}
	if m := legacyClaudeUserID.FindStringSubmatch(raw); len(m) == 2 {
		return m[1]
	}
	return ""
}

func contentSessionID(body map[string]any) string {
	var b strings.Builder
	b.WriteString(asString(body["model"]))
	b.WriteByte('\n')
	b.WriteString(asString(body["instructions"]))
	b.WriteByte('\n')
	if tools, ok := body["tools"].([]any); ok {
		for _, tool := range tools {
			m, _ := tool.(map[string]any)
			b.WriteString(asString(m["name"]))
			if fn, ok := m["function"].(map[string]any); ok {
				b.WriteString(asString(fn["name"]))
			}
			b.WriteByte(',')
		}
	}
	b.WriteByte('\n')
	b.WriteString(firstUserText(body["messages"]))
	if strings.Trim(firstUserText(body["messages"]), "\n") == "" {
		b.WriteString(firstUserText(body["input"]))
	}
	if b.Len() == 0 || strings.Trim(b.String(), "\n,") == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(b.String()))
	return "content:" + hex.EncodeToString(sum[:16])
}

func firstUserText(messages any) string {
	list, _ := messages.([]any)
	var system string
	for _, item := range list {
		m, _ := item.(map[string]any)
		role := asString(m["role"])
		text := messageText(m["content"])
		if (role == "system" || role == "developer") && system == "" {
			system = text
		}
		if role == "user" && text != "" {
			return system + "\n" + text
		}
	}
	return system
}

func messageText(content any) string {
	if s, ok := content.(string); ok {
		return s
	}
	parts, _ := content.([]any)
	var b strings.Builder
	for _, part := range parts {
		switch p := part.(type) {
		case string:
			b.WriteString(p)
		case map[string]any:
			if t := asString(p["text"]); t != "" {
				b.WriteString(t)
			}
		}
	}
	return b.String()
}

func firstString(m map[string]any, keys ...string) string {
	for _, key := range keys {
		if v := strings.TrimSpace(asString(m[key])); v != "" {
			return v
		}
	}
	return ""
}

func asString(v any) string {
	s, _ := v.(string)
	return s
}

type affinityPin struct {
	deployment string
	until      time.Time
}

func (s *Server) affinityGet(key string) string {
	if s == nil || key == "" {
		return ""
	}
	if s.Live != nil {
		if v, ok := s.Live.GetString(context.Background(), key); ok {
			return v
		}
	}
	s.affinityMu.Lock()
	defer s.affinityMu.Unlock()
	pin, ok := s.affinity[key]
	if !ok || time.Now().After(pin.until) {
		return ""
	}
	return pin.deployment
}

func (s *Server) affinitySet(key, deployment string) {
	if s == nil || key == "" || deployment == "" {
		return
	}
	if s.Live != nil {
		s.Live.SetString(context.Background(), key, deployment, affinityTTL)
	}
	s.affinityMu.Lock()
	defer s.affinityMu.Unlock()
	if s.affinity == nil {
		s.affinity = map[string]affinityPin{}
	}
	s.affinity[key] = affinityPin{deployment: deployment, until: time.Now().Add(affinityTTL)}
}
