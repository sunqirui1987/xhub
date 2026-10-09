package catalog

import "testing"

func TestImageVariantSelectsMeasuredPrice(t *testing.T) {
	rates := []Rate{
		{Measure: "picture", Side: "output", Variant: "low_1024x1024", Window: "all", USD: 0.01},
		{Measure: "picture", Side: "output", Variant: "high_1024x1024", Window: "all", USD: 0.1},
		{Measure: "token", Side: "output", Window: "all", USD: 0.00001},
	}
	u := NormalizeUsage(map[string]any{"images": 2, "image_variant": "high_1024x1024", "output_tokens": 10})
	c, ok := CostFromRates(rates, u, offpeakInstant())
	if !ok || c.Total != 0.2001 {
		t.Fatal(c, ok)
	}
	u.ImageVariant = "high_1536x1024"
	if c, ok := CostFromRates(rates, u, offpeakInstant()); ok {
		t.Fatal("unknown band priced", c)
	}
}
