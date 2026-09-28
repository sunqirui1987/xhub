package gateway

import "testing"

func TestRemovedColumnsStayUnregistered(t *testing.T) {
	removed := []string{"/v1/mcp", "/v1/mcp/oauth/token", "/tools", "/public/agent_hub"}
	for _, path := range removed {
		if !IsRemovedColumn(path) {
			t.Errorf("%s is still registered", path)
		}
	}
	kept := []string{"/get/ui_settings", "/sso/get/ui_settings", "/v1/tool/spend", "/spend/tags", "/v1/chat/completions", "/budget/list", "/budget/new", "/tag/list", "/v1/agents", "/get/ui_theme_settings", "/update/ui_theme_settings", "/config_overrides/hashicorp_vault", "/config_overrides/cyberark", "/model_hub", "/prompts", "/budgets", "/v1/mcp/server/oauth/e2e-mcp/token"}
	for _, path := range kept {
		if IsRemovedColumn(path) {
			t.Errorf("%s was removed", path)
		}
	}
}
