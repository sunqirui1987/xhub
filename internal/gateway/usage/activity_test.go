package usage

import (
	"testing"
	"time"
)

// rollupRows is the fixture shared by the rollup tests: two calls on one day in
// two timezones, plus a third that is out of range.
func rollupRows() []activityRow {
	return []activityRow{
		{
			day: "2026-09-27", model: "gpt-4o-mini", provider: "openai", apiKey: "hash-a",
			keyAlias: "desk", teamID: "team-1", userID: "user-1",
			route: "/chat/completions", prompt: 10, completion: 4, spend: 1.5, success: true,
		},
		{
			day: "2026-09-27", model: "gpt-4o-mini", provider: "openai", apiKey: "hash-a",
			keyAlias: "desk", teamID: "team-1", userID: "user-1",
			route: "/chat/completions", prompt: 2, completion: 0, spend: 0.25, success: false,
		},
	}
}

// TestDailyActivityResponseFoldsOneDay pins the shape the usage page reads: the
// per-day metrics, the model and endpoint breakdowns, and the key bucket that
// carries the display alias.
func TestDailyActivityResponseFoldsOneDay(t *testing.T) {
	body := dailyActivityResponse(rollupRows(), 1, true)

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
	endpoint := breakdown["endpoints"].(map[string]any)["/chat/completions"].(map[string]any)
	if endpoint["metrics"].(map[string]any)["api_requests"] != 2 {
		t.Fatalf("endpoint requests: %#v", endpoint["metrics"])
	}
	key := breakdown["api_keys"].(map[string]any)["hash-a"].(map[string]any)
	if key["metadata"].(map[string]any)["key_alias"] != "desk" {
		t.Fatalf("alias: %#v", key["metadata"])
	}
}

// TestGatewayActivityBodySplitsByOutcomeAndRoute covers the request-count view
// the gateway activity panel reads.
func TestGatewayActivityBodySplitsByOutcomeAndRoute(t *testing.T) {
	gateway := gatewayActivityBody(rollupRows())
	if gateway["total_successful_requests"] != 1 || gateway["total_failed_requests"] != 1 {
		t.Fatalf("gateway totals: %#v", gateway)
	}
	routes := gateway["by_route"].([]any)
	if len(routes) != 1 || routes[0].(map[string]any)["route"] != "/chat/completions" {
		t.Fatalf("routes: %#v", routes)
	}
}

// TestActivityRowWithNoKeyStillLandsInABucket pins that a call whose key id is
// empty is still counted. A row is attributed by its ownership snapshot, and an
// empty key is a fact about the row rather than a reason to drop it.
func TestActivityRowWithNoKeyStillLandsInABucket(t *testing.T) {
	bare := []activityRow{{
		day: "2026-09-27", model: "gpt-6-astra", provider: "openai",
		route: "/chat/completions", prompt: 10, completion: 4, spend: 0.5, success: true,
	}}
	body := dailyActivityResponse(bare, 1, true)
	if body["metadata"].(map[string]any)["total_spend"].(float64) <= 0 {
		t.Fatal("a row with no key was dropped")
	}
	day := body["results"].([]any)[0].(map[string]any)["breakdown"].(map[string]any)
	if day["models"].(map[string]any)["gpt-6-astra"] == nil {
		t.Fatal("model bucket missing")
	}
	if len(day["api_keys"].(map[string]any)) == 0 {
		t.Fatal("an empty key id did not land in a key bucket")
	}
	if day["endpoints"].(map[string]any)["/chat/completions"] == nil {
		t.Fatal("endpoint bucket missing")
	}
}

// TestActivityDayShiftsByTimezoneOffset pins the day boundary. The console sends
// Date.getTimezoneOffset(), which is positive west of Greenwich; uiTimezone
// flips it to the offset east of Greenwich that the fold needs.
func TestActivityDayShiftsByTimezoneOffset(t *testing.T) {
	ts := time.Date(2026, 9, 26, 16, 30, 0, 0, time.UTC)
	// 16:30Z is already the next morning at UTC+8.
	if got := activityDay(ts, 480); got != "2026-09-27" {
		t.Fatalf("utc+8 day: %s", got)
	}
	if got := activityDay(ts, 0); got != "2026-09-26" {
		t.Fatalf("utc day: %s", got)
	}
	// And still the previous evening at UTC-8.
	if got := activityDay(ts, -480); got != "2026-09-26" {
		t.Fatalf("utc-8 day: %s", got)
	}
}

// TestParseDayRejectsGarbageWithoutWideningTheWindow pins that a malformed date
// leaves that side of the window open rather than filtering every row out. A
// zero time is what the caller reads as "no bound".
func TestParseDayRejectsGarbageWithoutWideningTheWindow(t *testing.T) {
	if !parseDay("").IsZero() {
		t.Fatal("an empty date must leave the bound unset")
	}
	if !parseDay("not-a-date").IsZero() {
		t.Fatal("an unparseable date must leave the bound unset")
	}
	got := parseDay("2026-09-27")
	if got.IsZero() || got.Format("2006-01-02") != "2026-09-27" {
		t.Fatalf("parsed date: %v", got)
	}
	// The end bound covers the whole day, not just its first instant.
	end := parseDayEnd("2026-09-27")
	if end.Hour() != 23 || end.Minute() != 59 {
		t.Fatalf("end of day: %v", end)
	}
}
