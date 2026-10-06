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
