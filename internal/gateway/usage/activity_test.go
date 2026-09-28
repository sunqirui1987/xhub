package usage

import (
	"testing"

	"github.com/sunqirui1987/xhub/internal/store"
)

func TestSelectActivityGroupsSpendIntoTheDailyRollup(t *testing.T) {
	logs := []map[string]any{
		{
			"request_id": "in-range", "call_type": "chat", "model": "gpt-4o-mini", "api_key": "hash-a",
			"prompt_tokens": 10, "completion_tokens": 4, "spend": 1.5,
			"startTime": "2026-09-27T13:30:00Z", "status": "success",
		},
		{
			"request_id": "failed", "call_type": "chat", "model": "gpt-4o-mini", "api_key": "hash-a",
			"prompt_tokens": 2, "completion_tokens": 0, "spend": 0.25,
			"startTime": "2026-09-27T01:00:00Z", "status": "failure",
		},
		{
			"request_id": "other-day", "call_type": "embeddings", "model": "claude-3", "api_key": "hash-b",
			"prompt_tokens": 8, "completion_tokens": 0, "spend": 9.0,
			"startTime": "2020-01-01T00:00:00Z", "status": "success",
		},
	}
	keys := []store.Key{{
		TokenHash: "hash-a", KeyAlias: "desk", TeamID: "team-1", UserID: "user-1",
	}}

	rows := selectActivity(logs, keys, "2026-09-01", "2026-09-30", "-480", "", "")
	if len(rows) != 2 {
		t.Fatalf("rows in range: got %d want 2", len(rows))
	}
	body := dailyActivityResponse(rows, 1, true)
	meta := body["metadata"].(map[string]any)
	if meta["total_api_requests"] != 2 || meta["total_successful_requests"] != 1 || meta["total_failed_requests"] != 1 {
		t.Fatalf("metadata counts: %#v", meta)
	}
	if meta["total_spend"] != 1.75 {
		t.Fatalf("total_spend: %#v", meta["total_spend"])
	}
	if meta["total_prompt_tokens"] != 12 || meta["total_completion_tokens"] != 4 || meta["total_tokens"] != 16 {
		t.Fatalf("token totals: %#v", meta)
	}
	results := body["results"].([]any)
	if len(results) != 1 {
		t.Fatalf("days: got %d", len(results))
	}
	day := results[0].(map[string]any)
	// 2026-09-27T01:00Z is still Sep 27 in UTC+8 (offset -480).
	if day["date"] != "2026-09-27" {
		t.Fatalf("date: %#v", day["date"])
	}
	breakdown := day["breakdown"].(map[string]any)
	models := breakdown["models"].(map[string]any)
	model := models["gpt-4o-mini"].(map[string]any)
	if model["metrics"].(map[string]any)["api_requests"] != 2 {
		t.Fatalf("model requests: %#v", model["metrics"])
	}
	if _, ok := breakdown["model_groups"].(map[string]any)["gpt-4o-mini"]; !ok {
		t.Fatal("model group missing")
	}
	if _, ok := breakdown["providers"].(map[string]any)["openai"]; !ok {
		t.Fatalf("provider: %#v", breakdown["providers"])
	}
	key := breakdown["api_keys"].(map[string]any)["hash-a"].(map[string]any)
	if key["metadata"].(map[string]any)["key_alias"] != "desk" {
		t.Fatalf("alias: %#v", key["metadata"])
	}

	scoped := selectActivity(logs, keys, "2026-09-01", "2026-09-30", "0", "user-1", "")
	if len(scoped) != 2 {
		t.Fatalf("user scope: got %d", len(scoped))
	}
	other := selectActivity(logs, keys, "2026-09-01", "2026-09-30", "0", "someone-else", "")
	if len(other) != 0 {
		t.Fatalf("other user: got %d", len(other))
	}

	gateway := gatewayActivityBody(rows)
	if gateway["total_successful_requests"] != 1 || gateway["total_failed_requests"] != 1 {
		t.Fatalf("gateway totals: %#v", gateway)
	}
	routes := gateway["by_route"].([]any)
	if len(routes) != 1 || routes[0].(map[string]any)["route"] != "/chat/completions" {
		t.Fatalf("routes: %#v", routes)
	}
}

func TestActivityDayShiftsByTimezoneOffset(t *testing.T) {
	// JS getTimezoneOffset for UTC+8 is -480. 16:30Z is the next local morning.
	if got := activityDay("2026-09-26T16:30:00Z", -480); got != "2026-09-27" {
		t.Fatalf("local day: %s", got)
	}
	if got := activityDay("2026-09-26T16:30:00Z", 0); got != "2026-09-26" {
		t.Fatalf("utc day: %s", got)
	}
}
