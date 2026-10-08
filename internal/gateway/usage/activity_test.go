package usage

import (
	"testing"
	"time"

	"github.com/sunqirui1987/xhub/internal/authz"
	"github.com/sunqirui1987/xhub/internal/iam"
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

func TestActivityCountsZeroCostCallsAndPreservesRecordedProvider(t *testing.T) {
	start := time.Date(2026, 9, 27, 8, 0, 0, 0, time.UTC)
	cached := 3
	events := []iam.UsageEvent{
		{RequestID: "paid", TS: start, Model: "gpt-activity-model", Provider: "historical-provider", KeyID: "key-a", Cost: 1.5, Status: "success", PromptTokens: 10, CompletionTokens: 4},
		{RequestID: "free", TS: start, Model: "free-model", KeyID: "key-a", Cost: 0, Status: "success", PromptTokens: 2, CompletionTokens: 1},
		{RequestID: "cache", TS: start, Model: "gpt-activity-model", Provider: "historical-provider", KeyID: "key-a", Cost: 0, Status: "success", CacheHit: true, PromptTokens: 3, CompletionTokens: 2, CachedTokens: &cached},
		{RequestID: "failed", TS: start, Model: "gpt-activity-model", Provider: "historical-provider", KeyID: "key-a", Cost: 0, Status: "error", PromptTokens: 7},
	}
	rows := eventsToActivity(events, 0)
	if len(rows) != len(events) {
		t.Fatalf("activity dropped zero-cost calls: got %d rows, want %d", len(rows), len(events))
	}
	body := dailyActivityResponse(rows, 1, true)
	meta := body["metadata"].(map[string]any)
	if meta["total_api_requests"] != 4 || meta["total_successful_requests"] != 3 || meta["total_failed_requests"] != 1 {
		t.Fatalf("paid, free, cached and failed calls were not all counted: %#v", meta)
	}
	if meta["total_spend"] != 1.5 || meta["total_prompt_tokens"] != 22 || meta["total_completion_tokens"] != 7 || meta["total_tokens"] != 29 {
		t.Fatalf("activity totals changed stored usage or spend: %#v", meta)
	}
	if meta["total_cache_read_input_tokens"] != 3 {
		t.Fatalf("zero-cost cached usage was dropped: %#v", meta)
	}
	day := body["results"].([]any)[0].(map[string]any)
	providers := day["breakdown"].(map[string]any)["providers"].(map[string]any)
	historical, ok := providers["historical-provider"].(map[string]any)
	if !ok || historical["metrics"].(map[string]any)["api_requests"] != 3 {
		t.Fatalf("historical provider was replaced by a current-model guess: %#v", providers)
	}
	if _, ok := providers["openai"]; ok {
		t.Fatalf("model name overrode recorded provider: %#v", providers)
	}
	gateway := gatewayActivityBody(rows)
	if gateway["total_successful_requests"] != 3 || gateway["total_failed_requests"] != 1 {
		t.Fatalf("gateway activity dropped zero-cost outcomes: %#v", gateway)
	}
}

func TestDailyActivityReportsKnownCacheReadsAndOmitsUnknownCacheFields(t *testing.T) {
	cached := 7
	rows := rollupRows()
	rows[0].cacheRead = &cached
	body := dailyActivityResponse(rows, 1, true)
	meta := body["metadata"].(map[string]any)
	if meta["total_cache_read_input_tokens"] != 7 {
		t.Fatalf("cache read total: %#v", meta)
	}
	if _, ok := meta["total_cache_creation_input_tokens"]; ok {
		t.Fatalf("unknown cache creation was reported as measured: %#v", meta)
	}
	day := body["results"].([]any)[0].(map[string]any)
	metrics := day["metrics"].(map[string]any)
	if metrics["cache_read_input_tokens"] != 7 {
		t.Fatalf("day cache reads: %#v", metrics)
	}

	unknown := dailyActivityResponse(rollupRows(), 1, true)["metadata"].(map[string]any)
	if _, ok := unknown["total_cache_read_input_tokens"]; ok {
		t.Fatalf("unknown cache reads were reported as zero: %#v", unknown)
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

// TestActivityBodyGroupsByTheRequestedEntity checks the breakdown the team,
// organization, and user views read. The key is the id the console already
// has, and the label comes from the lookup rather than from the raw id.
func TestActivityBodyGroupsByTheRequestedEntity(t *testing.T) {
	body := activityBody(rollupRows(), 1, true, entityTeam, entityLabels{
		teams: map[string]iam.Team{"team-1": {Name: "Desk"}},
	})
	day := body["results"].([]any)[0].(map[string]any)
	entities := day["breakdown"].(map[string]any)["entities"].(map[string]any)
	team := entities["team-1"].(map[string]any)
	if team["metrics"].(map[string]any)["spend"] != 1.75 {
		t.Fatalf("team spend: %#v", team["metrics"])
	}
	if team["metadata"].(map[string]any)["team_alias"] != "Desk" {
		t.Fatalf("team alias: %#v", team["metadata"])
	}

	plain := dailyActivityResponse(rollupRows(), 1, true)
	plainDay := plain["results"].([]any)[0].(map[string]any)
	if len(plainDay["breakdown"].(map[string]any)["entities"].(map[string]any)) != 0 {
		t.Fatal("a response with no entity dimension must leave entities empty")
	}
}

// TestKeepVisibleRefusesAnEmptyIntersection checks that a filter naming only
// unseen ids does not collapse into "no filter".
func TestKeepVisibleRefusesAnEmptyIntersection(t *testing.T) {
	seen := map[string]struct{}{"a": {}}
	kept, err := keepVisible([]string{"a", "b"}, seen)
	if err != nil || len(kept) != 1 || kept[0] != "a" {
		t.Fatalf("kept: %v %v", kept, err)
	}
	if _, err := keepVisible([]string{"b"}, seen); !authz.IsNotFound(err) {
		t.Fatalf("unseen only: %v", err)
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
