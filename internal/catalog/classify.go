// Package catalog classifies a path as public, inference, or management, and matches a catalog template to a concrete URL.
package catalog

import (
	"encoding/json"
	"net/http"
	"strings"
)

// Route is one method and path from routes.json. The path may contain {param}.
type Route struct {
	M string `json:"m"`
	P string `json:"p"`
}

// Load reads the embedded routes.json. A parse failure returns nil, and the caller treats that as an empty catalog.
func Load() []Route {
	var r []Route
	_ = json.Unmarshal(routesJSON, &r)
	return r
}

// AuthClass is the identity a path requires on the gateway.
type AuthClass int

const (
	// AuthManagement accepts only a management key or a session.
	AuthManagement AuthClass = iota
	// AuthData is an inference path and accepts only an LLM key.
	AuthData
	// AuthMixed is an old path that was removed but is still in the catalog. A management key and an LLM key both reach the rejection logic.
	AuthMixed
)

// AuthOf chooses the identity class from the path prefix. A mixed prefix wins over an inference prefix.
func AuthOf(path string) AuthClass {
	if IsMixedPath(path) {
		return AuthMixed
	}
	if IsLLMPrefix(path) {
		return AuthData
	}
	return AuthManagement
}

// IsMixedPath reports paths such as agents, MCP, and skills that were removed from the product but remain in the catalog.
func IsMixedPath(path string) bool {
	for _, p := range []string{
		"/v1/agents", "/v1beta/agents", "/v1/skills", "/v1/memory",
		"/v1/workflows", "/v1/tool", "/agent", "/mcp", "/v1/mcp",
	} {
		if HasPrefix(path, p) {
			return true
		}
	}
	return false
}

// IsPublicPath reports a GET or HEAD that needs no identity. Only public pages, well-known URLs, and the model-hub entry qualify.
func IsPublicPath(method, path string) bool {
	if method != http.MethodGet && method != http.MethodHead {
		return false
	}
	if strings.HasPrefix(path, "/public/") || path == "/public" {
		return true
	}
	if strings.HasPrefix(path, "/.well-known/") {
		return true
	}
	if path == "/model_hub" || path == "/model_hub_table" {
		return true
	}
	return false
}

// IsDataPlanePath reports a path that enters the inference data plane, including removed mixed prefixes.
func IsDataPlanePath(path string) bool {
	return IsMixedPath(path) || IsLLMPrefix(path)
}

// IsLLMPrefix reports a path prefix for OpenAI, Anthropic, Gemini, and compatible endpoints.
func IsLLMPrefix(path string) bool {
	prefixes := []string{
		"/v1/chat", "/v1/completions", "/v1/messages", "/v1/responses",
		"/v1/embeddings", "/v1/images", "/v1/audio", "/v1/moderations",
		"/v1/rerank", "/v1/files", "/v1/batches", "/v1/assistants",
		"/v1/threads", "/v1/fine_tuning", "/v1/containers",
		"/v1/vector_stores", "/v1/vector_store", "/vector_stores", "/v1/videos",
		"/v1/realtime", "/v1/search", "/v1/ocr", "/v1/rag", "/v1/indexes",
		"/v1/a2a", "/v1beta/models", "/v1beta/interactions", "/interactions",
		"/v1/evals", "/evals",
		"/v2/rerank", "/chat", "/completions", "/embeddings", "/messages",
		"/responses", "/audio", "/images", "/moderations", "/rerank",
		"/files", "/batches", "/engines", "/openai", "/cursor", "/queue",
		"/realtime", "/a2a",
	}
	for _, p := range prefixes {
		if HasPrefix(path, p) {
			return true
		}
	}
	return false
}

// HasPrefix compares path prefixes. A trailing slash is ignored so /v1/chat does not match /v1/chatcompletions.
func HasPrefix(path, prefix string) bool {
	path = strings.TrimSuffix(path, "/")
	prefix = strings.TrimSuffix(prefix, "/")
	if path == prefix {
		return true
	}
	return strings.HasPrefix(path, prefix+"/")
}

// PublicBody is the fixed response for a public GET. The price map and create fields come from embedded JSON. Everything else is an empty list.
func PublicBody(path string) any {
	p := strings.ToLower(path)
	switch {
	case strings.Contains(p, "/public/agents/fields"):
		return unmarshalOr(agentFieldsJSON, []any{})
	case strings.Contains(p, "/public/providers/fields"):
		return unmarshalOr(providerFieldsJSON, []any{})
	case strings.Contains(p, "/public/autorouter_presets"):
		return unmarshalOr(autoRouterPresetsJSON, map[string]any{"presets": []any{}})
	case strings.Contains(p, "blog_posts"):
		return map[string]any{"posts": []any{}}
	case strings.Contains(p, "scorer_defaults"):
		return map[string]any{
			"tier_boundaries":   map[string]any{},
			"token_thresholds":  map[string]any{},
			"dimension_weights": map[string]any{},
		}
	case strings.Contains(p, "model_cost_map"):
		if modelCostMapValue != nil {
			return modelCostMapValue
		}
		return map[string]any{}
	case strings.Contains(p, "skill_hub"):
		return map[string]any{"plugins": []any{}, "count": 0}
	case strings.Contains(p, "model_hub/info"):
		return map[string]any{"data": []any{}}
	case strings.Contains(p, "model_hub"), strings.Contains(p, "agent_hub"), strings.Contains(p, "mcp_hub"):
		return []any{}
	case strings.HasSuffix(p, "/fields"), strings.HasSuffix(p, "/providers"):
		return []any{}
	default:
		return map[string]any{"object": "list", "data": []any{}}
	}
}

// unmarshalOr parses JSON. On failure it uses fallback and does not surface the error on a public route.
func unmarshalOr(raw []byte, fallback any) any {
	var v any
	if json.Unmarshal(raw, &v) == nil {
		return v
	}
	return fallback
}

// PathMatch reports whether a catalog template covers a concrete path. {name} matches one segment. :path and {x:path} match the rest.
func PathMatch(pat, path string) bool {
	pat = strings.ReplaceAll(pat, ":path", "")
	if pat == path {
		return true
	}
	ps := split(pat)
	xs := split(path)
	if len(ps) != len(xs) && !strings.Contains(pat, "{") {
		return false
	}
	if len(ps) != len(xs) {
		if len(ps) > 0 && strings.HasPrefix(ps[len(ps)-1], "{") && len(xs) >= len(ps)-1 {
			return prefixMatch(ps[:len(ps)-1], xs)
		}
		if len(ps) != len(xs) {
			return false
		}
	}
	for i := range ps {
		if strings.HasPrefix(ps[i], "{") {
			continue
		}
		if ps[i] != xs[i] {
			return false
		}
	}
	return true
}

// prefixMatch compares a path template with the actual segments. A brace segment matches any one segment.
func prefixMatch(ps, xs []string) bool {
	for i := range ps {
		if strings.HasPrefix(ps[i], "{") {
			continue
		}
		if i >= len(xs) || ps[i] != xs[i] {
			return false
		}
	}
	return true
}

// Split splits a path on slashes. An empty path returns nil, not a slice containing an empty string.
func Split(p string) []string {
	p = strings.Trim(p, "/")
	if p == "" {
		return nil
	}
	return strings.Split(p, "/")
}

// split calls Split. An empty path returns nil.
func split(p string) []string { return Split(p) }
