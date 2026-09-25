// 目录路由的分发。匹配和价格表在 catalog 包，这里只决定交给哪一类 handler。
package gateway

import (
	"net/http"
	"strings"

	"github.com/sunqirui1987/xhub/internal/catalog"
	"github.com/sunqirui1987/xhub/internal/gateway/family"
	"github.com/sunqirui1987/xhub/internal/httpx"
)

// 这些名字留给本包和同包测试。实现在 catalog，避免目录匹配和价格表再堆在 HTTP 注册里。
type catRoute = catalog.Route

// 读取内置 routes.json。解析失败时得到空切片。
func loadCatalog() []catRoute { return catalog.Load() }

// 按路径决定要管理身份、推理身份，还是两者都可能。
func authClassOf(path string) authClass { return authClass(catalog.AuthOf(path)) }

type authClass = catalog.AuthClass

const (
	authManagement = catalog.AuthManagement
	authData       = catalog.AuthData
	authMixed      = catalog.AuthMixed
)

// 是否为已下线的智能体、MCP、技能等路径。
func isMixedPath(path string) bool { return catalog.IsMixedPath(path) }

// GET/HEAD 是否不需要身份。
func isPublicPath(method, path string) bool { return catalog.IsPublicPath(method, path) }

// 路径是否会进入推理数据面。
func isDataPlanePath(path string) bool { return catalog.IsDataPlanePath(path) }

// 是否为聊天、嵌入、图像等推理前缀。
func isLLMPrefix(path string) bool { return catalog.IsLLMPrefix(path) }

// 路径前缀比较。末尾斜杠不参与，避免误匹配更长的单词。
func hasPathPrefix(path, prefix string) bool { return catalog.HasPrefix(path, prefix) }

// 公开 GET 的固定 JSON。价格表和字段定义来自内置文件。
func publicListBody(path string) any { return catalog.PublicBody(path) }

// 目录模板是否覆盖这条具体路径。
func pathMatch(pat, path string) bool { return catalog.PathMatch(pat, path) }

// 转调 catalog.Split。空路径得到 nil。
func split(p string) []string { return catalog.Split(p) }

// 转调 catalog.ProviderModels。
func providerModels(provider string) []string { return catalog.ProviderModels(provider) }

// 内置价格表中的模型条数，不含 sample_spec。
func modelCostMapCount() int { return catalog.Count() }

// 是否强制只用内置价格表。
func localCostMapForced() bool { return catalog.EnvForced() }

// 模型名到价格字段的映射。
func modelCostMap() map[string]map[string]any { return catalog.CostMap() }

// modelCostMapValue 与 modelCostMapLoadedAt 保持同包测试的旧读法。
var (
	modelCostMapValue    = catalog.Raw()
	modelCostMapLoadedAt = catalog.LoadedAt()
)

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
		family.ServeDataPlane(s, w, r)
	case authMixed:
		family.ServeMixed(s, w, r)
	default:
		family.ServeMgmt(s, w, r)
	}
}

// 当前请求是否落在内置目录的某一条上。混合前缀即使不在清单里也算匹配。
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
	// OpenAPI 清单不一定包含 /v1/mcp/server 这种子路由。
	return isMixedPath(path)
}
