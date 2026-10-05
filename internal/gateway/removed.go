package gateway

import (
	"strings"
	"sync"

	"github.com/sunqirui1987/xhub/internal/catalog"
	"github.com/sunqirui1987/xhub/internal/logx"
)

var logTraceOnceRemoved sync.Once

func IsRemovedColumn(path string) bool {
	logTraceOnceRemoved.Do(func() { logx.Trace("enter gateway.IsRemovedColumn") })
	return removedColumnRoute(path)
}

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

// IsRetiredPath reports whether a path belongs to the catalog's retired class.
//
// These are the "mixed" routes the catalog still lists but the product removed.
// They are answered with 410 rather than served, so a test that walks the route
// inventory has to expect that and not a 200.
//
// It asks the catalog for the classification rather than repeating the prefix
// list, so a path cannot be retired in one place and served in the other.
func IsRetiredPath(path string) bool {
	return catalog.IsMixedPath(path)
}
