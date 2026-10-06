package usage

import (
	"testing"
	"time"

	"github.com/sunqirui1987/xhub/internal/catalog"
	"github.com/sunqirui1987/xhub/internal/iam"
)

func TestCostBreakdownMultipliesTokensByRate(t *testing.T) {
	inRate, outRate, ok := catalog.TokenRates("gpt-4o")
	if !ok || inRate <= 0 || outRate <= 0 {
		t.Fatal("gpt-4o has no per-token rates")
	}
	got := costBreakdown("gpt-4o", 96, 0, 96*inRate)
	if got["input_cost"] != 96*inRate {
		t.Fatalf("input cost: %#v", got["input_cost"])
	}
	if got["output_cost"] != 0.0 {
		t.Fatalf("output cost: %#v", got["output_cost"])
	}
	if got["original_cost"] != 96*inRate {
		t.Fatalf("original: %#v", got["original_cost"])
	}
	if got["input_cost_per_token"] != inRate || got["output_cost_per_token"] != outRate {
		t.Fatalf("rates: %#v", got)
	}
}

func TestEventRowCarriesTheBill(t *testing.T) {
	rows := eventRows([]iam.UsageEvent{{
		RequestID: "r1", Model: "gpt-4o", PromptTokens: 96, CompletionTokens: 0,
		Cost: 0.000384, TS: time.Now().UTC(),
	}})
	meta, _ := rows[0]["metadata"].(map[string]any)
	bill, _ := meta["cost_breakdown"].(map[string]any)
	if bill["input_cost"] == nil || bill["output_cost"] == nil || bill["original_cost"] == nil {
		t.Fatalf("bill missing sides: %#v", bill)
	}
	if bill["total_cost"] != 0.000384 {
		t.Fatalf("charged total: %#v", bill["total_cost"])
	}
}

func TestEventRowExposesTheConsoleColumns(t *testing.T) {
	ttft := 420
	cached := 100
	end := time.Now().UTC()
	start := end.Add(-time.Second)
	rows := eventRows([]iam.UsageEvent{{
		RequestID: "r-cols", Model: "gpt-4o", PromptTokens: 10, CompletionTokens: 2,
		Cost: 0.01, TS: start, EndedAt: &end, TTFTMs: &ttft, CacheHit: true,
		KeyHash: "hash-1", KeyAlias: "key-1", TeamAlias: "team-1",
		Provider: "openai", CachedTokens: &cached, SessionID: "sess-1", CacheKey: "ck-1",
	}})
	row := rows[0]
	meta := row["metadata"].(map[string]any)
	if meta["user_api_key"] != "hash-1" || meta["user_api_key_alias"] != "key-1" || meta["user_api_key_team_alias"] != "team-1" {
		t.Fatalf("snapshot %#v", meta)
	}
	if row["completionStartTime"] == nil || row["cache_hit"] != "true" || row["session_id"] != "sess-1" {
		t.Fatalf("row %#v", row)
	}
	if row["custom_llm_provider"] != "openai" || row["cache_key"] != "ck-1" || meta["cached_tokens"] != 100 {
		t.Fatalf("provider/cache %#v", row)
	}
	if row["endTime"] == row["startTime"] {
		t.Fatal("end time collapsed onto start")
	}
}
