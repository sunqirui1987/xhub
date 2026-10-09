package catalog

import "testing"

func TestExplicitSellerPriceBindingSurvivesRefreshAndRejectsOtherSource(t *testing.T) {
	const official = "seller-binding-refresh-model"
	const qualified = "seller/" + official
	const source = "https://seller.example/prices"
	t.Cleanup(func() {
		RemoveModel(qualified)
		RemoveModel(official)
		modelCostMu.Lock()
		delete(contributions, qualified)
		modelCostMu.Unlock()
	})
	usage := Usage{CompletionTokens: 1000, OutputVariant: "woiv"}
	setPrice := func(usd float64, priceSource string) {
		SetModel(official, map[string]any{"source": priceSource, "rates": []Rate{{Measure: "token", Side: "output", Variant: "woiv", Window: "all", USD: usd}}})
	}
	setPrice(0.001, source)
	Contribute(Row{ID: qualified, Provider: "seller", Official: official, PriceModel: official, PriceSource: source})
	if c, ok := CostAt(qualified, usage, offpeakInstant()); !ok || c.Total != 1 {
		t.Fatalf("initial binding: %+v %v", c, ok)
	}
	// ApplyDocument replaces rows then calls refreshContributionsLocked. Removing
	// the qualified row simulates that replacement without changing other fixtures.
	for _, tc := range []struct {
		source string
		priced bool
	}{{source, true}, {"https://other.example/prices", false}} {
		RemoveModel(qualified)
		setPrice(0.002, tc.source)
		modelCostMu.Lock()
		refreshContributionsLocked()
		modelCostMu.Unlock()
		c, ok := CostAt(qualified, usage, offpeakInstant())
		if ok != tc.priced || (ok && c.Total != 2) {
			t.Fatalf("refreshed binding source=%s: %+v %v", tc.source, c, ok)
		}
	}
}
