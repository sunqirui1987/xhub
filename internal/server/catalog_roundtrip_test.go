package server

import (
	"strings"
	"testing"
)

func TestCatalogCreateThenListConsumed(t *testing.T) {
	s, master := testEnv(t)
	h := s.Handler()

	type tc struct {
		createMethod, createPath string
		body                     map[string]any
		listPath                 string
		needle                   string
	}
	cases := []tc{
		{"POST", "/v1/agents", map[string]any{"agent_name": "e2e-agent"}, "/v1/agents", "e2e-agent"},
		{"POST", "/v1/mcp/server", map[string]any{"server_name": "E2E_MCP", "url": "http://127.0.0.1:9"}, "/v1/mcp/server", "E2E_MCP"},
		{"POST", "/guardrails", map[string]any{"guardrail_name": "e2e-gr", "litellm_params": map[string]any{"guardrail": "custom"}}, "/guardrails/list", "e2e-gr"},
		{"POST", "/policies", map[string]any{"policy_name": "e2e-pol"}, "/policies/list", "e2e-pol"},
		{"POST", "/prompts", map[string]any{"prompt_id": "e2e-prompt"}, "/prompts/list", "e2e-prompt"},
		{"POST", "/tag/new", map[string]any{"tag_name": "e2e-tag"}, "/tag/list", "e2e-tag"},
		{"POST", "/v1/access_group", map[string]any{"access_group_name": "e2e-ag"}, "/v1/access_group", "e2e-ag"},
		{"POST", "/search_tools", map[string]any{"search_tool_name": "e2e-search"}, "/search_tools/list", "e2e-search"},
		{"POST", "/vector_store/new", map[string]any{"vector_store_name": "e2e-vs", "name": "e2e-vs"}, "/vector_store/list", "e2e-vs"},
		{"POST", "/v1/skills", map[string]any{"name": "e2e-skill"}, "/v1/skills", "e2e-skill"},
		{"POST", "/credentials", map[string]any{"credential_name": "e2e-cred", "credential_values": map[string]any{"api_key": "sk-x"}}, "/credentials", "e2e-cred"},
	}
	for _, c := range cases {
		unauth := doJSON(t, h, c.createMethod, c.createPath, "", c.body)
		if unauth.Code != 401 {
			t.Fatalf("%s unauth want 401 got %d %s", c.createPath, unauth.Code, unauth.Body.String())
		}
		created := doJSON(t, h, c.createMethod, c.createPath, master, c.body)
		if created.Code != 200 {
			t.Fatalf("%s create %d %s", c.createPath, created.Code, created.Body.String())
		}
		if !strings.Contains(created.Body.String(), c.needle) {
			t.Fatalf("%s create body missing %q: %s", c.createPath, c.needle, created.Body.String())
		}
		listed := doJSON(t, h, "GET", c.listPath, master, nil)
		if listed.Code != 200 {
			t.Fatalf("%s list %d %s", c.listPath, listed.Code, listed.Body.String())
		}
		if !strings.Contains(listed.Body.String(), c.needle) {
			t.Fatalf("%s list missing created %q: %s", c.listPath, c.needle, listed.Body.String())
		}
	}
}
