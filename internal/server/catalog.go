package server

import (
	_ "embed"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/sunqirui1987/xhub/internal/httpx"
)

//go:embed routes.json
var routesJSON []byte

//go:embed publicdata/agent_create_fields.json
var agentFieldsJSON []byte

//go:embed publicdata/provider_create_fields.json
var providerFieldsJSON []byte

//go:embed publicdata/autorouter_presets.json
var autoRouterPresetsJSON []byte

//go:embed publicdata/model_cost_map.json
var modelCostMapJSON []byte

type catRoute struct {
	M string `json:"m"`
	P string `json:"p"`
}

func loadCatalog() []catRoute {
	var r []catRoute
	_ = json.Unmarshal(routesJSON, &r)
	return r
}

// serveFamilyRoute 处理已经在 Gin 上按路径注册、但没有更早专用 handler 的 catalog 路由。
// 它不挂在 "/" 上。未注册的路径由引擎的 NoRoute 返回 404。
func (s *Server) serveFamilyRoute(w http.ResponseWriter, r *http.Request) {
	if !s.matchCatalog(r.Method, r.URL.Path) {
		httpx.WriteError(w, 404, "not_found", "Not Found")
		return
	}
	httpx.SetCallID(w, httpx.CallID())
	if isPublicPath(r.Method, r.URL.Path) {
		httpx.WriteJSON(w, 200, publicListBody(r.URL.Path))
		return
	}
	switch authClassOf(r.URL.Path) {
	case authData:
		s.serveDataPlaneCatalog(w, r)
	case authMixed:
		s.serveMixedCatalog(w, r)
	default:
		s.serveMgmtCatalog(w, r)
	}
}

type authClass int

const (
	authManagement authClass = iota
	authData
	authMixed
)

func authClassOf(path string) authClass {
	if isMixedPath(path) {
		return authMixed
	}
	if isLLMPrefix(path) {
		return authData
	}
	return authManagement
}

func isMixedPath(path string) bool {
	for _, p := range []string{
		"/v1/agents", "/v1beta/agents", "/v1/skills", "/v1/memory",
		"/v1/workflows", "/v1/tool", "/agent", "/mcp", "/v1/mcp",
	} {
		if hasPathPrefix(path, p) {
			return true
		}
	}
	return false
}

func publicListBody(path string) any {
	p := strings.ToLower(path)
	switch {
	case strings.Contains(p, "/public/agents/fields"):
		var v any
		if json.Unmarshal(agentFieldsJSON, &v) == nil {
			return v
		}
		return []any{}
	case strings.Contains(p, "/public/providers/fields"):
		var v any
		if json.Unmarshal(providerFieldsJSON, &v) == nil {
			return v
		}
		return []any{}
	case strings.Contains(p, "/public/autorouter_presets"):
		var v any
		if json.Unmarshal(autoRouterPresetsJSON, &v) == nil {
			return v
		}
		return map[string]any{"presets": []any{}}
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

func isPublicPath(method, path string) bool {
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

func isDataPlanePath(path string) bool {
	return isMixedPath(path) || isLLMPrefix(path)
}

func isLLMPrefix(path string) bool {
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
		if hasPathPrefix(path, p) {
			return true
		}
	}
	return false
}

func hasPathPrefix(path, prefix string) bool {
	path = strings.TrimSuffix(path, "/")
	prefix = strings.TrimSuffix(prefix, "/")
	if path == prefix {
		return true
	}
	return strings.HasPrefix(path, prefix+"/")
}

func (s *Server) matchCatalog(method, path string) bool {
	path = strings.TrimSuffix(path, "/")
	if path == "" {
		path = "/"
	}
	for _, rt := range s.catalog {
		if !strings.EqualFold(rt.M, method) {
			continue
		}
		if pathMatch(rt.P, path) {
			return true
		}
	}
	// Sub-routers such as /v1/mcp/server are not always in the OpenAPI inventory.
	return isMixedPath(path)
}

func pathMatch(pat, path string) bool {
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
		// {x:path} already stripped; allow prefix if last pat is {}
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

func split(p string) []string {
	p = strings.Trim(p, "/")
	if p == "" {
		return nil
	}
	return strings.Split(p, "/")
}
