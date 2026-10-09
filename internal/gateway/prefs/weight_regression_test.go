package prefs

import (
	"math"
	"testing"
)

func TestWeightOverridesPreserveZero(t *testing.T) {
	for _, weights := range []any{
		map[string]any{"deployment:a": float64(0), "deployment:b": float64(2), "invalid": math.Inf(1)},
		[]any{map[string]any{"deployment_id": "a", "weight": float64(0)}, map[string]any{"deployment_id": "b", "weight": float64(2)}, map[string]any{"deployment_id": "invalid", "weight": math.NaN()}},
	} {
		r := RouteSettings{Settings: map[string]any{"routing_strategy_args": map[string]any{"weights": weights}}}
		got := r.WeightOverrides()
		zero, ok := got["deployment:a"]
		if !ok || zero != 0 || got["deployment:b"] != 2 || len(got) != 2 {
			t.Fatalf("overrides: %v", got)
		}
	}
}
