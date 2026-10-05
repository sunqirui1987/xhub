package dataplane

import (
	"github.com/sunqirui1987/xhub/internal/live"
	"testing"
)

func TestHotDeltasIncludesProject(t *testing.T) {
	rows := []live.SpendLog{{APIKey: "k", TeamID: "t", UserID: "u", OrgID: "o", ProjectID: "p", Spend: 0.25, SpendValid: true}, {ProjectID: "p", Spend: 100}}
	got := hotDeltas(rows)
	for _, id := range []string{"key/k", "team/t", "user/u", "org/o", "project/p"} {
		if got[id] != 0.25 {
			t.Fatalf("%s delta = %v", id, got[id])
		}
	}
}
