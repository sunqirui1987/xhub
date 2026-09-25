// 控制台已经去掉的栏目。这些路径固定 404，不再注册实现。
package gateway

import "strings"

// removedColumnRoute 是已经从控制台拿掉的栏目对应的 HTTP 路径。
// 智能体、工作流、记忆、MCP、技能、策略、搜索工具、向量存储、提示词、标签管理、缓存管理、模型中心、界面主题、界面设置写入、Hashicorp Vault 和 CyberArk 不再注册。
// /spend/tags 是用量汇总，不是标签管理页，仍然保留。
// IsRemovedColumn 报告这条路径是否属于已下线栏目。测试和路由注册都用它判断该不该挂上。
func IsRemovedColumn(path string) bool { return removedColumnRoute(path) }

func removedColumnRoute(path string) bool {
	p := strings.ToLower(strings.TrimSuffix(path, "/"))
	if p == "" {
		return false
	}
	// 费用优化仍读取每个工具的花费，这不是已经去掉的工具策略页。
	if p == "/v1/tool/spend" {
		return false
	}
	switch p {
	case "/flushall",
		"/authorize/mcp-session",
		"/{mcp_server_name}/authorize", "/{mcp_server_name}/register", "/{mcp_server_name}/token",
		"/get/ui_theme_settings", "/update/ui_theme_settings", "/update/ui_settings":
		return true
	}
	prefixes := []string{
		"/cache/",
		"/tools",
		"/test/tools",
		"/v1/tool/",
		"/.well-known/agent-skills",
		"/.well-known/skills",
		"/.well-known/oauth-authorization-server/{root_path}/v1/mcp",
		"/a2a",
		"/agent/",
		"/v1/agents",
		"/v1beta/agents",
		"/v1/a2a",
		"/v1/workflows",
		"/v1/memory",
		"/v1/mcp",
		"/mcp",
		"/v1/skills",
		"/skills",
		"/v1/tool/policy",
		"/v1/vector_store",
		"/v1/vector_stores",
		"/v1/search/",
		"/policies",
		"/policy",
		"/prompts",
		"/search_tools",
		"/search/",
		"/vector_store",
		"/vector_stores",
		"/public/agent_hub",
		"/public/agents",
		"/public/mcp_hub",
		"/public/skill_hub",
		"/public/model_hub",
		"/model_hub",
		"/get/mcp_",
		"/update/mcp_",
		"/utils/dotprompt",
		"/tag/",
		"/config_overrides/cyberark",
		"/config_overrides/hashicorp_vault",
	}
	for _, prefix := range prefixes {
		if p == strings.TrimSuffix(prefix, "/") || strings.HasPrefix(p, prefix) {
			return true
		}
	}
	return false
}
