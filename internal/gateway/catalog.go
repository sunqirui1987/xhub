// catalog.go loads routes.json and answers public catalog reads. Matching and
// the price map live in internal/catalog. This file chooses which handler
// runs for a catalog path that ingress.go did not give to a family.

package gateway

import (
	"net/http"
	"strings"

	"github.com/sunqirui1987/xhub/internal/catalog"
	"github.com/sunqirui1987/xhub/internal/gateway/family"
	"github.com/sunqirui1987/xhub/internal/httpx"
	"github.com/sunqirui1987/xhub/internal/logx"
	"sync"
)

// These names stay for this package. The implementation is in catalog so route matching and the price map are not piled into HTTP registration.
type catRoute = catalog.Route

var logTraceOnceCatalog sync.Once

// loadCatalog reads the embedded routes.json. A parse failure returns an empty slice.
// 参数：无。
// 返回 []catRoute（[]catRoute）：嵌入的 routes.json 解析出的路由。解析失败时为空切片。
// 调用：gateway/server.go
// 测试：无直接单测
func loadCatalog() []catRoute {
	logTraceOnceCatalog.Do(func() { logx.Trace("enter gateway.loadCatalog") })
	return catalog.Load()
}

// authClassOf decides whether a path needs a management identity, an inference identity, or either.
// 参数 path（string）：认证Class的要定位的路径。可能是 URL，也可能是字段路径。
// 返回 authClass（authClass）：这条路径要求的鉴权级别。
// 调用：仅在 catalog.go 内使用
// 测试：无直接单测
func authClassOf(path string) authClass { return authClass(catalog.AuthOf(path)) }

type authClass = catalog.AuthClass

const (
	authManagement = catalog.AuthManagement
	authData       = catalog.AuthData
	authMixed      = catalog.AuthMixed
)

// isMixedPath reports a removed path such as agents, MCP, or skills.
// 参数 path（string）：URL 路径，用来匹配路由或选择错误包络。
// 返回 bool（bool）：路径属于已删除的目录类，例如 agents、MCP 或 skills 时返回真。
// 调用：仅在 catalog.go 内使用
// 测试：无直接单测
func isMixedPath(path string) bool { return catalog.IsMixedPath(path) }

// isPublicPath reports whether a GET or HEAD needs no identity.
// 参数 method（string）：HTTP 方法，例如 GET 或 POST；path（string）：URL 路径，用来匹配路由。
// 返回 bool（bool）：这个 GET 或 HEAD 不需要身份时返回真。
// 调用：仅在 catalog.go 内使用
// 测试：无直接单测
func isPublicPath(method, path string) bool { return catalog.IsPublicPath(method, path) }

// isDataPlanePath reports whether the path enters the inference data plane.
// 参数 path（string）：URL 路径，用来匹配路由或选择错误包络。
// 返回 bool（bool）：路径进入推理数据面时返回真。
// 调用：gateway/engine.go
// 测试：无直接单测
func isDataPlanePath(path string) bool { return catalog.IsDataPlanePath(path) }

// isLLMPrefix reports a chat, embedding, image, or other inference prefix.
// 参数 path（string）：是否LLMPrefix要定位的路径。可能是 URL，也可能是字段路径。
// 返回 bool（bool）：路径是 chat、embedding、image 或其他推理前缀时返回真。
// 调用：仅在 catalog.go 内使用
// 测试：无直接单测
func isLLMPrefix(path string) bool { return catalog.IsLLMPrefix(path) }

// hasPathPrefix compares path prefixes. A trailing slash is ignored so a longer word is not matched by mistake.
// 参数 path（string）：URL 路径，用来匹配路由；prefix（string）：要匹配或要去掉的前缀。空串表示不处理前缀。
// 返回 bool（bool）：去掉末尾斜杠后路径匹配该前缀时返回真，避免把更长的单词误判为前缀。
// 调用：仅在 catalog.go 内使用
// 测试：无直接单测
func hasPathPrefix(path, prefix string) bool { return catalog.HasPrefix(path, prefix) }

// publicListBody is the fixed JSON for a public GET. The price map and field definitions come from embedded files.
// 参数 path（string）：公开列表正文要定位的路径。可能是 URL，也可能是字段路径。
// 返回 any（any）：公开列表正文的结果。具体类型由调用方断言。
// 调用：仅在 catalog.go 内使用
// 测试：无直接单测
func publicListBody(path string) any { return catalog.PublicBody(path) }

// pathMatch reports whether a catalog template covers this concrete path.
// 参数 pat（string）：目录里的路径模板；path（string）：这次请求的具体 URL 路径。
// 返回 bool（bool）：这条目录模板盖住当前具体路径时返回真。
// 调用：仅在 catalog.go 内使用
// 测试：无直接单测
func pathMatch(pat, path string) bool { return catalog.PathMatch(pat, path) }

// split calls catalog.Split. An empty path returns nil.
// 参数 p（string）：要按斜杠拆开的 URL 路径。空路径得到 nil。
// 返回 []string（[]string）：catalog.Split 的路径段。空路径为 nil。
// 调用：仅在 catalog.go 内使用。
// 测试：无直接单测
func split(p string) []string { return catalog.Split(p) }

// providerModels calls catalog.ProviderModels.
// 参数 provider（string）：供应商标识，例如 openai 或 volcengine。
// 返回 []string（[]string）：供应商模型。没有匹配时为空切片。
// 调用：仅在 catalog.go 内使用。
// 测试：无直接单测
func providerModels(provider string) []string { return catalog.ProviderModels(provider) }

// modelCostMapCount is the number of models in the built-in price map, excluding sample_spec.
// 参数：无。
// 返回 int（int）：价格表里的模型条数。零表示还没有载入。
// 调用：仅在 catalog.go 内使用。
// 测试：无直接单测
func modelCostMapCount() int { return catalog.Count() }

// localCostMapForced reports whether only the built-in price map may be used.
// 参数：无。
// 返回 bool（bool）：只能使用内置价格表时返回真。
// 调用：仅在 catalog.go 内使用。
// 测试：无直接单测
func localCostMapForced() bool { return catalog.EnvForced() }

// modelCostMap maps a model name to its price fields.
// 参数：无。
// 返回 map[string]map[string]any（map[string]map[string]any）：模型费用表的字段表。缺键表示上游或库里没有这个字段。
// 调用：gateway/tokens.go
// 测试：无直接单测
func modelCostMap() map[string]map[string]any { return catalog.CostMap() }

// modelCostMapValue and modelCostMapLoadedAt keep the old read path used by callers in this package.
var (
	modelCostMapValue    = catalog.Raw()
	modelCostMapLoadedAt = catalog.LoadedAt()
)

// serveFamilyRoute handles a catalog route that was registered on Gin by path and has no earlier dedicated handler. It is not mounted on "/". An unregistered path gets a 404 from the engine NoRoute.
// 参数 w（http.ResponseWriter）：调用方的 HTTP 响应，状态码和正文写在这里；r（*http.Request）：入站 HTTP 请求，用来读路径、头和正文。
// 调用：gateway/ingress.go
// 测试：无直接单测
// 返回：无。状态码和正文写进调用方的响应。
func (s *Server) serveFamilyRoute(w http.ResponseWriter, r *http.Request) {
	if !s.matchCatalog(r.Method, r.URL.Path) {
		logx.Debug("process %s %s step=catalog match=false", r.Method, r.URL.Path)
		httpx.WriteError(w, 404, "not_found", "Not Found")
		return
	}
	httpx.SetCallID(w, httpx.CallID())
	if isPublicPath(r.Method, r.URL.Path) {
		logx.Debug("process %s %s step=catalog class=public", r.Method, r.URL.Path)
		httpx.WriteJSON(w, 200, publicListBody(r.URL.Path))
		return
	}
	class := "manage"
	switch authClassOf(r.URL.Path) {
	case authData:
		class = "data"
	case authMixed:
		class = "mixed"
	}
	logx.Debug("process %s %s step=catalog class=%s", r.Method, r.URL.Path, class)
	switch class {
	case "data":
		family.ServeDataPlane(s, w, r)
	case "mixed":
		family.ServeMixed(s, w, r)
	default:
		family.ServeMgmt(s, w, r)
	}
}

// matchCatalog reports whether this request hits one embedded catalog route. A mixed prefix counts as a match even when it is not in the list.
// 参数 method（string）：HTTP 方法，例如 GET 或 POST；path（string）：匹配目录要定位的路径。可能是 URL，也可能是字段路径。
// 返回 bool（bool）：请求命中一条内嵌目录路由时返回真。混合前缀即使不在清单里也算命中。
// 调用：仅在 catalog.go 内使用
// 测试：无直接单测
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
	// The OpenAPI list does not always include a child route such as /v1/mcp/server.
	return isMixedPath(path)
}
