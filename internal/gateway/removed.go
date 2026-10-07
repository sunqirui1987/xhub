// removed.go keeps a list of paths that must stay 404. They used to be
// mounted and were taken out. mount and handle both consult the list so a
// catalog row cannot bring one back.

package gateway

import (
	"strings"
	"sync"

	"github.com/sunqirui1987/xhub/internal/catalog"
	"github.com/sunqirui1987/xhub/internal/logx"
)

var logTraceOnceRemoved sync.Once

// 判断这条路径是不是已经从产品里删除的控制台列。
// 参数 path（string）：URL 路径，用来匹配路由或选择错误包络。
// 返回 bool（bool）：这条路径属于已经删除的控制台列，应当拒绝时为真。
// 调用：仅在 removed.go 内使用
// 测试：catalog_reads_test.go、removed_test.go
func IsRemovedColumn(path string) bool {
	logTraceOnceRemoved.Do(func() { logx.Trace("enter gateway.IsRemovedColumn") })
	return removedColumnRoute(path)
}

// 按路径判断是不是已删除列。花费标签和个别保留路径返回假。
// 参数 path（string）：URL 路径，用来匹配路由或选择错误包络。
// 返回 bool（bool）：这条路径属于已经删除的控制台列，应当拒绝时为真。
// 调用：gateway/ingress.go
// 测试：无直接单测
func removedColumnRoute(path string) bool {
	p := strings.ToLower(strings.TrimSuffix(path, "/"))
	if p == "" || p == "/v1/tool/spend" || p == "/spend/tags" {
		return false
	}
	switch p {
	case "/authorize/mcp-session", "/{mcp_server_name}/authorize", "/{mcp_server_name}/register", "/{mcp_server_name}/token":
		return true
	}
	if strings.Contains(p, "/v1/mcp/server/oauth/") && strings.HasSuffix(p, "/token") {
		return false
	}
	prefixes := []string{"/tools", "/test/tools", "/.well-known/agent-skills", "/.well-known/skills", "/.well-known/oauth-authorization-server/{root_path}/v1/mcp", "/a2a", "/agent/", "/v1/a2a", "/v1/mcp", "/mcp", "/v1/search/", "/search/", "/public/agent_hub", "/public/agents", "/public/mcp_hub", "/public/skill_hub", "/get/mcp_", "/update/mcp_", "/utils/dotprompt"}
	for _, prefix := range prefixes {
		if p == strings.TrimSuffix(prefix, "/") || strings.HasPrefix(p, prefix) {
			return true
		}
	}
	return false
}

// IsRetiredPath reports whether a path belongs to the catalog's retired class. These are the "mixed" routes the catalog still lists but the product removed. They are answered with 410 rather than served
// , so a test that walks the route inventory has to expect that and not a 200. It asks the catalog for the classification rather than repeating the prefix list, so a path cannot be retired in one placeand served in the other.
// 参数 path（string）：URL 路径，用来匹配路由或选择错误包络。
// 返回 bool（bool）：路径属于目录里已退役、应当回答 410 而不是继续服务的一类时返回真。
// 调用：仅在 removed.go 内使用
// 测试：catalog_reads_test.go、removed_test.go
func IsRetiredPath(path string) bool {
	return catalog.IsMixedPath(path)
}
