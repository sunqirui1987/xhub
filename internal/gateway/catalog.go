// Package gateway dispatches catalog routes. Matching and the price map live in the catalog package. This file only chooses which kind of handler runs.
package gateway

import (
	"net/http"
	"strings"

	"github.com/sunqirui1987/xhub/internal/catalog"
	"github.com/sunqirui1987/xhub/internal/gateway/family"
	"github.com/sunqirui1987/xhub/internal/httpx"
)

// These names stay for this package. The implementation is in catalog so route matching and the price map are not piled into HTTP registration.
type catRoute = catalog.Route

// loadCatalog reads the embedded routes.json. A parse failure returns an empty slice.
func loadCatalog() []catRoute { return catalog.Load() }

// authClassOf decides whether a path needs a management identity, an inference identity, or either.
func authClassOf(path string) authClass { return authClass(catalog.AuthOf(path)) }

type authClass = catalog.AuthClass

const (
	authManagement = catalog.AuthManagement
	authData       = catalog.AuthData
	authMixed      = catalog.AuthMixed
)

// isMixedPath reports a removed path such as agents, MCP, or skills.
func isMixedPath(path string) bool { return catalog.IsMixedPath(path) }

// isPublicPath reports whether a GET or HEAD needs no identity.
func isPublicPath(method, path string) bool { return catalog.IsPublicPath(method, path) }

// isDataPlanePath reports whether the path enters the inference data plane.
func isDataPlanePath(path string) bool { return catalog.IsDataPlanePath(path) }

// isLLMPrefix reports a chat, embedding, image, or other inference prefix.
func isLLMPrefix(path string) bool { return catalog.IsLLMPrefix(path) }

// hasPathPrefix compares path prefixes. A trailing slash is ignored so a longer word is not matched by mistake.
func hasPathPrefix(path, prefix string) bool { return catalog.HasPrefix(path, prefix) }

// publicListBody is the fixed JSON for a public GET. The price map and field definitions come from embedded files.
func publicListBody(path string) any { return catalog.PublicBody(path) }

// pathMatch reports whether a catalog template covers this concrete path.
func pathMatch(pat, path string) bool { return catalog.PathMatch(pat, path) }

// split calls catalog.Split. An empty path returns nil.
func split(p string) []string { return catalog.Split(p) }

// providerModels calls catalog.ProviderModels.
func providerModels(provider string) []string { return catalog.ProviderModels(provider) }

// modelCostMapCount is the number of models in the built-in price map, excluding sample_spec.
func modelCostMapCount() int { return catalog.Count() }

// localCostMapForced reports whether only the built-in price map may be used.
func localCostMapForced() bool { return catalog.EnvForced() }

// modelCostMap maps a model name to its price fields.
func modelCostMap() map[string]map[string]any { return catalog.CostMap() }

// modelCostMapValue and modelCostMapLoadedAt keep the old read path used by callers in this package.
var (
	modelCostMapValue    = catalog.Raw()
	modelCostMapLoadedAt = catalog.LoadedAt()
)

// serveFamilyRoute handles a catalog route that was registered on Gin by path and has no earlier dedicated handler.
// It is not mounted on "/". An unregistered path gets a 404 from the engine NoRoute.
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

// matchCatalog reports whether this request hits one embedded catalog route. A mixed prefix counts as a match even when it is not in the list.
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
