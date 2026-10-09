package router

import (
	"encoding/json"
	"math"
	"testing"
	"time"

	"github.com/sunqirui1987/xhub/internal/catalog"
	"github.com/sunqirui1987/xhub/internal/config"
)

func TestCostRoutingUsesSettlementRatePrecedenceAndWindow(t *testing.T) {
	peak := time.Date(2026, 3, 2, 2, 0, 0, 0, time.UTC)
	offpeak := peak.Add(10 * time.Hour)
	for _, table := range []bool{false, true} {
		a := deployment("shared-name", "https://a.example", 1)
		b := deployment("shared-name", "https://b.example", 1)
		a.LiteLLMParams["input_cost_per_token"] = 1.0
		b.LiteLLMParams["input_cost_per_token"] = 2.0
		if table {
			a.LiteLLMParams["rates"] = []catalog.Rate{
				{Measure: "token", UnitSize: 1, Side: "input", Window: "peak", USD: 3},
				{Measure: "token", UnitSize: 1, Side: "input", Window: "offpeak", USD: 0},
			}
		} else {
			a.LiteLLMParams["input_cost_per_token_peak"] = 3.0
		}
		for _, tc := range []struct {
			at   time.Time
			want string
		}{
			{peak, "https://b.example"}, {offpeak, "https://a.example"},
		} {
			got := Pick([]config.ModelEntry{b, a}, "shared-name", "cost-based-routing", State{Now: tc.at})
			if got == nil || got.ParamString("api_base", "") != tc.want {
				t.Fatalf("table=%v at=%s got=%v want=%s", table, tc.at, got, tc.want)
			}
		}
	}
}

func TestCostRoutingAcceptsValidNumericRates(t *testing.T) {
	for _, value := range []any{0.0, float32(0), 0, int64(0), json.Number("0"), " 0 "} {
		e := config.ModelEntry{LiteLLMParams: map[string]any{"input_cost_per_token": value}}
		if got := comparableRate(e, State{}); got != 0 {
			t.Errorf("%T(%v): got %v want zero", value, value, got)
		}
	}
	for _, value := range []any{-1.0, math.NaN(), math.Inf(1), "NaN", "broken"} {
		e := config.ModelEntry{LiteLLMParams: map[string]any{"input_cost_per_token": value}}
		if got := comparableRate(e, State{}); !math.IsInf(got, 1) {
			t.Errorf("%v: got %v want unknown price", value, got)
		}
	}
}
