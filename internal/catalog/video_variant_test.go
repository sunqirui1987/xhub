package catalog

import "testing"

func TestMeasuredVideoVariantRequiresItsOutputRate(t *testing.T) {
	u := Usage{Seconds: 5.5, OutputVariant: "1080p_av"}
	for _, rates := range [][]Rate{
		{{Measure: "second", Side: "output", Variant: "720p_v", Window: "all", USD: 0.01}},
		{{Measure: "second", Side: "input", Variant: "1080p_av", Window: "all", USD: 0.02}},
	} {
		if c, ok := CostFromRates(rates, u, offpeakInstant()); ok {
			t.Fatalf("wrong video side/band selected: %+v", c)
		}
	}
	for _, variant := range []string{"1080p_av", ""} {
		rates := []Rate{{Measure: "second", Side: "output", Variant: variant, Window: "all", USD: 0.2}}
		c, ok := CostFromRates(rates, u, offpeakInstant())
		if !ok || c.Total != 1.1 || len(c.Applied) != 1 || c.Applied[0].Fallback {
			t.Fatalf("explicit or deliberately flat video rate: %+v %v", c, ok)
		}
	}
}
