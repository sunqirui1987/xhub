// Package catalog classifies a path as public, inference, or management, and matches a catalog template to a concrete URL.
package catalog

import (
	"encoding/json"
	"github.com/sunqirui1987/xhub/internal/logx"
	"net/http"
	"strings"
	"sync"
)

// Route is one method and path from routes.json. The path may contain {param}.
type Route struct {
	M string `json:"m"`
	P string `json:"p"`
}

var logTraceOnceClassify sync.Once

// Load reads the embedded routes.json. A parse failure returns nil, and the caller treats that as an empty catalog.
// 参数：无。
// 调用：config/config.go、gateway/catalog.go
// 测试：activity_http_test.go、builtin_providers_test.go、chains_test.go
// 返回 []Route（[]Route）：目录里的路由表。
func Load() []Route {
	logTraceOnceClassify.Do(func() { logx.Trace("enter catalog.Load") })

	var r []Route
	_ = json.Unmarshal(Embedded("routes", routesJSON), &r)
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
// 参数 path（string）：认证的要定位的路径。可能是 URL，也可能是字段路径。
// 返回 AuthClass（AuthClass）：这条路径要求的鉴权级别。
// 调用：gateway/catalog.go
// 测试：无直接单测
func AuthOf(path string) AuthClass {
	if IsMixedPath(path) {
		return AuthMixed
	}
	if IsLLMPrefix(path) {
		return AuthData
	}
	return AuthManagement
}

// IsMixedPath reports the catalog paths that were removed from the product and are refused rather than served. /v1/skills is deliberately absent: it is a real route again, serving the organization's skills. A path leaves this list when a dedicated handler takes it over, or the refusal would answer before the handler ever ran.
// 参数 path（string）：URL 路径，用来匹配路由或选择错误包络。
// 返回 bool（bool）：这条路径属于已从产品移除、应当拒绝而不是继续服务的目录路径时返回真。/v1/skills 已恢复为真实路由，不在此列。
// 调用：gateway/catalog.go、gateway/removed.go
// 测试：无直接单测
func IsMixedPath(path string) bool {
	for _, p := range []string{
		"/v1/agents", "/v1beta/agents", "/v1/memory",
		"/v1/workflows", "/v1/tool", "/agent", "/mcp", "/v1/mcp",
	} {
		if HasPrefix(path, p) {
			return true
		}
	}
	return false
}

// IsPublicPath reports a GET or HEAD that needs no identity. Only public pages, well-known URLs, and the model-hub entry qualify.
// 参数 method（string）：HTTP 方法，例如 GET 或 POST；path（string）：URL 路径，用来匹配路由。
// 返回 bool（bool）：这是不需要身份的 GET 或 HEAD，例如公开页、well-known 或模型广场入口时返回真。
// 调用：gateway/catalog.go
// 测试：无直接单测
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
// 参数 path（string）：URL 路径，用来匹配路由或选择错误包络。
// 返回 bool（bool）：这条路径进入推理数据面，包括已移除的混合前缀时返回真。
// 调用：gateway/catalog.go
// 测试：无直接单测
func IsDataPlanePath(path string) bool {
	return IsMixedPath(path) || IsLLMPrefix(path)
}

// IsLLMPrefix reports a path prefix for OpenAI, Anthropic, Gemini, and compatible endpoints.
// 参数 path（string）：是否LLMPrefix要定位的路径。可能是 URL，也可能是字段路径。
// 返回 bool（bool）：路径前缀属于 OpenAI、Anthropic、Gemini 或兼容端点时返回真。
// 调用：gateway/catalog.go
// 测试：无直接单测
func IsLLMPrefix(path string) bool {
	prefixes := []string{
		"/v1/chat", "/v1/completions", "/v1/messages", "/v1/responses",
		"/v1/embeddings", "/v1/images", "/v1/audio", "/v1/moderations",
		"/v1/rerank", "/v1/files", "/v1/batches", "/v1/assistants",
		"/v1/threads", "/v1/fine_tuning", "/v1/containers",
		"/v1/vector_stores", "/v1/vector_store", "/vector_stores", "/v1/videos",
		"/v3/contents", "/api/v3/contents",
		"/v1/realtime", "/v1/search", "/v1/ocr", "/v1/rag", "/v1/indexes",
		"/v1/a2a", "/v1beta/models", "/v1beta/interactions", "/interactions",
		"/v1/evals", "/evals",
		"/v2/rerank", "/chat", "/completions", "/embeddings", "/messages",
		"/responses", "/audio", "/images", "/moderations", "/rerank",
		"/files", "/batches", "/assistants", "/threads", "/fine_tuning",
		"/containers", "/videos", "/engines", "/openai", "/cursor", "/queue",
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
// 参数 path（string）：要比较的 URL 路径；prefix（string）：路径前缀。两边先去掉末尾斜杠再比，函数不改调用方手里的字符串。
// 返回 bool（bool）：去掉末尾斜杠后，路径以前缀开头时返回真。因此 /v1/chat 不会误匹配 /v1/chatcompletions。
// 调用：IsLLMPrefix，以及 gateway/catalog.go 的 hasPathPrefix。auth/auth.go 里的 HasPrefix 是 strings.HasPrefix，不是这个函数。
// 测试：无直接单测
func HasPrefix(path, prefix string) bool {
	path = strings.TrimSuffix(path, "/")
	prefix = strings.TrimSuffix(prefix, "/")
	if path == prefix {
		return true
	}
	return strings.HasPrefix(path, prefix+"/")
}

// PublicBody is the fixed response for a public GET. The price map and create fields come from embedded JSON. Everything else is an empty list.
// 参数 path（string）：公开正文要定位的路径。可能是 URL，也可能是字段路径。
// 返回 any（any）：公开正文的结果。具体类型由调用方断言。
// 调用：gateway/bypass.go、gateway/catalog.go、provider/registry.go
// 测试：无直接单测
func PublicBody(path string) any {
	p := strings.ToLower(path)
	switch {
	case strings.Contains(p, "/public/agents/fields"):
		// The agent product surface is not mounted. The path stays in the
		// catalog so a caller gets an empty list rather than a 404.
		return []any{}
	case strings.Contains(p, "/public/providers/fields"):
		return mergeProviders(Providers())
	case strings.Contains(p, "/public/autorouter_presets"):
		return unmarshalOr(Embedded("autorouter_presets", autoRouterPresetsJSON), map[string]any{"presets": []any{}})
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
// 参数 raw（[]byte）：原始文本或 JSON 字节；fallback（any）：缺值或解析失败时用的默认。
// 返回 any（any）：unmarshal或的结果。具体类型由调用方断言。
// 调用：仅在 classify.go 内使用
// 测试：无直接单测
func unmarshalOr(raw []byte, fallback any) any {
	var v any
	if json.Unmarshal(raw, &v) == nil {
		return v
	}
	return fallback
}

// PathMatch reports whether a catalog template covers a concrete path. {name} matches one segment. :path and {x:path} match the rest.
// 参数 pat（string）：目录里的路径模板，花括号表示一段占位；path（string）：这次请求的具体 URL 路径。
// 返回 bool（bool）：目录模板盖住这条具体路径时返回真。{name} 匹配一段，:path 和 {x:path} 匹配剩余部分。
// 调用：gateway/catalog.go
// 测试：无直接单测
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
// 参数 ps（[]string）：ps列表。空切片表示没有可处理的项；xs（[]string）：xs列表。空切片表示没有可处理的项。
// 返回 bool（bool）：模板的每一段都对上实际路径时返回真。花括号段可以匹配任意一段。
// 调用：仅在 classify.go 内使用
// 测试：无直接单测
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
// 参数 p（string）：要按斜杠拆开的 URL 路径。空路径返回 nil，而不是含一个空串的切片。
// 返回 []string（[]string）：去掉首尾斜杠后按 / 拆开的路径段。空路径为 nil。
// 调用：gateway/catalog.go、gateway/family/handlers.go。
// 测试：无直接单测
func Split(p string) []string {
	p = strings.Trim(p, "/")
	if p == "" {
		return nil
	}
	return strings.Split(p, "/")
}

// split calls Split. An empty path returns nil.
// 参数 p（string）：要按斜杠拆开的 URL 路径。空路径交给 Split，得到 nil。
// 返回 []string（[]string）：Split 的路径段。空路径为 nil。
// 调用：仅在 classify.go 内使用。
// 测试：无直接单测
func split(p string) []string { return Split(p) }
