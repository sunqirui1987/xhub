// 按路径判断公开、推理还是管理，并把目录模板匹配到具体 URL。
package catalog

import (
	"encoding/json"
	"net/http"
	"strings"
)

// Route 是 routes.json 里的一条方法加路径。路径可以含 {param}。
type Route struct {
	M string `json:"m"`
	P string `json:"p"`
}

// Load 读内置 routes.json。解析失败时返回 nil，调用方按空目录处理。
func Load() []Route {
	var r []Route
	_ = json.Unmarshal(routesJSON, &r)
	return r
}

// AuthClass 是一条路径在网关里要求的身份。
type AuthClass int

const (
	// AuthManagement 只接受管理密钥或会话。
	AuthManagement AuthClass = iota
	// AuthData 是推理路径，只接受 LLM 密钥。
	AuthData
	// AuthMixed 是已经下线但仍留在目录里的旧路径，管理密钥和 LLM 密钥都能进到拒绝逻辑。
	AuthMixed
)

// AuthOf 按路径前缀决定身份类别。混合前缀优先于推理前缀。
func AuthOf(path string) AuthClass {
	if IsMixedPath(path) {
		return AuthMixed
	}
	if IsLLMPrefix(path) {
		return AuthData
	}
	return AuthManagement
}

// IsMixedPath 是智能体、MCP、技能等已经从产品拿掉、但目录里还留着的路径。
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

// IsPublicPath 是不需要身份的 GET/HEAD。只有公开页、well-known 和模型中心入口。
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

// IsDataPlanePath 是会进推理数据面的路径，含已下线的混合前缀。
func IsDataPlanePath(path string) bool {
	return IsMixedPath(path) || IsLLMPrefix(path)
}

// IsLLMPrefix 是 OpenAI、Anthropic、Gemini 以及兼容端点的路径前缀。
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

// HasPrefix 比较路径前缀。末尾斜杠被忽略，避免 /v1/chat 误匹配 /v1/chatcompletions。
func HasPrefix(path, prefix string) bool {
	path = strings.TrimSuffix(path, "/")
	prefix = strings.TrimSuffix(prefix, "/")
	if path == prefix {
		return true
	}
	return strings.HasPrefix(path, prefix+"/")
}

// PublicBody 是公开 GET 的固定响应。价格表和创建字段来自内置 JSON，其余是空列表。
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

// 解析 JSON。失败时用 fallback，不把错误抛给公开接口。
func unmarshalOr(raw []byte, fallback any) any {
	var v any
	if json.Unmarshal(raw, &v) == nil {
		return v
	}
	return fallback
}

// PathMatch 判断目录模板是否覆盖具体路径。{name} 匹配一段，:path 和 {x:path} 匹配剩余部分。
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

// 比较路径模板和实际分段。花括号段匹配任意一段。
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

// Split 按斜杠拆路径。空路径得到 nil，而不是 [""]。
func Split(p string) []string {
	p = strings.Trim(p, "/")
	if p == "" {
		return nil
	}
	return strings.Split(p, "/")
}

// 转调 catalog.Split。空路径得到 nil。
func split(p string) []string { return Split(p) }
