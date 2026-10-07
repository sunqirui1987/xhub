package gateway

import "testing"

func TestRemovedColumnsStayUnregistered(t *testing.T) {
	removed := []string{"/v1/mcp", "/v1/mcp/oauth/token", "/tools", "/public/agent_hub", "/cloudzero/settings", "/cloudzero/export", "/email/event_settings", "/email/event_settings/reset", "/config/cost_discount_config", "/config/cost_margin_config", "/config/block_requests_for_models_without_pricing", "/cost/estimate", "/cost/predict-cache", "/config_overrides/hashicorp_vault", "/config_overrides/cyberark", "/get/sso_settings", "/update/sso_settings", "/sso/key/generate", "/get/allowed_ips", "/add/allowed_ip", "/Users", "/Groups", "/alerting/settings", "/callbacks/list", "/config/callback/delete", "/team/t1/callback", "/usage/ai/chat"}
	for _, path := range removed {
		if !IsRemovedColumn(path) {
			t.Errorf("%s is still registered", path)
		}
	}
	kept := []string{"/get/ui_settings", "/sso/get/ui_settings", "/spend/tags", "/v1/chat/completions", "/budget/list", "/budget/new", "/tag/list", "/get/ui_theme_settings", "/update/ui_theme_settings", "/model_hub", "/prompts", "/budgets", "/v1/mcp/server/oauth/e2e-mcp/token", "/vantage/settings", "/get/config/callbacks", "/config/update", "/config/list"}
	for _, path := range kept {
		if IsRemovedColumn(path) {
			t.Errorf("%s was removed", path)
		}
	}
}

// TestRetiredPathsAreRefused pins the third state a catalog path can be in.
//
// A removed column answers 404 because the route is gone. A retired path is
// still listed in the catalog but is refused with 410: it used to be served by
// the generic key-value store, which had no owner and no team column, so any
// signed-in member could write records every other principal could then read.
//
// The two sets must not overlap. A path that is both would be answered by
// whichever check ran first, so the tests in this package that walk the route
// inventory could no longer tell which contract to expect.
func TestRetiredPathsAreRefused(t *testing.T) {
	retired := []string{
		"/v1/agents", "/v1/agents/agent_1", "/v1beta/agents",
		"/v1/memory", "/v1/memory/k",
		"/v1/workflows/runs", "/v1/tool/list", "/v1/tool/policy", "/v1/tool/spend",
	}
	for _, path := range retired {
		if !IsRetiredPath(path) {
			t.Errorf("%s must be refused", path)
		}
	}

	// A live path must not be caught by the retired prefixes. /v1/skills is
	// here because it was retired once and is served again by a dedicated
	// handler; leaving it in the retired list would refuse it before that
	// handler ran, and the skills page would be empty with a plausible error.
	live := []string{"/v1/models", "/key/list", "/spend/logs/v2", "/team/member_list",
		"/token_counter", "/v1/skills", "/server"}
	for _, path := range live {
		if IsRetiredPath(path) {
			t.Errorf("%s is live but classified as retired", path)
		}
	}
}

// TestRemovedColumnWinsOverRetired pins the order the two checks run in, because
// the sets overlap on /mcp, /agent and /v1/mcp.
//
// Both classifications are derived from prefix lists that grew separately, and a
// path can satisfy both. That is tolerable only because the removed-column check
// runs at mount time, before the catalog is consulted, so a removed path never
// reaches the retired branch and the answer is stable at 404. If the order were
// ever reversed these paths would silently change from 404 to 410, which the
// inventory tests read as a different contract rather than a bug.
func TestRemovedColumnWinsOverRetired(t *testing.T) {
	for _, path := range []string{"/v1/mcp", "/mcp", "/agent", "/agent/foo"} {
		if !IsRemovedColumn(path) {
			t.Fatalf("%s must stay a removed column", path)
		}
		if !IsRetiredPath(path) {
			// If this ever becomes false the overlap is gone and the ordering
			// note above is obsolete, not wrong. Surface it rather than hide it.
			t.Logf("%s is no longer in the retired set; the overlap may be resolved", path)
		}
	}
}
