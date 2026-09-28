package gateway

import (
	"github.com/sunqirui1987/xhub/internal/logx"
	"strings"
	"sync"
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
