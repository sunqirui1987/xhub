package prefs

import (
	"math"
	"testing"
)

func TestWeightOverridesPreserveZero(t *testing.T) {
	for _, weights := range []any{
		map[string]any{"a|model": float64(0), "b|model": float64(2), "invalid": math.Inf(1)},
		[]any{map[string]any{"api_base": "a", "model": "model", "weight": float64(0)}, map[string]any{"api_base": "b", "model": "model", "weight": float64(2)}, map[string]any{"model": "invalid", "weight": math.NaN()}},
	} {
		r := RouteSettings{Settings: map[string]any{"routing_strategy_args": map[string]any{"weights": weights}}}
		got := r.WeightOverrides()
		zero, ok := got["a|model"]
		if !ok || zero != 0 || got["b|model"] != 2 || len(got) != 2 {
			t.Fatalf("overrides: %v", got)
		}
	}
}
