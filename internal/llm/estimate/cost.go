// Package estimate reads the input and output unit price of one call. A missing model is not priced as zero.
package estimate

import "strings"

// Margin is one markup rule.
//
// LiteLLM cost_margin_config has two forms. A number is a pure percent, so 0.10 adds 10 percent,
// and only Percent is used with IsPercent set true. An object may have both percentage and
// fixed_amount. The percent multiplies the cost and the fixed amount is added once. Both are in dollars.
type Margin struct {
	Percent     float64
	FixedAmount float64
	HasPercent  bool
	HasFixed    bool
	IsPercent   bool
}

// ApplyDiscount looks up a rate for the provider in the discount map and subtracts it from the base cost.
//
// Discount-map keys are custom_llm_provider. The value is a fraction: 0.05 means subtract 5 percent.
// A missing provider, or an empty provider name, leaves the cost unchanged.
// The results are the discounted cost, the discount fraction, and the discount amount.
func ApplyDiscount(baseCost float64, provider string, discounts map[string]float64) (final, percent, amount float64) {
	if provider != "" {
		if rate, ok := discounts[provider]; ok {
			amount = baseCost * rate
			return baseCost - amount, rate, amount
		}
	}
	return baseCost, 0, 0
}

// ApplyMargin adds a markup after the discount. A provider's own rule wins over the global rule stored under the key global.
//
// The results are the marked-up cost, the percent, the fixed amount, and the markup total.
// With no rule the last three values are 0 and the cost is unchanged.
func ApplyMargin(baseCost float64, provider string, margins map[string]Margin) (final, percent, fixed, total float64) {
	cfg, ok := marginFor(provider, margins)
	if !ok {
		return baseCost, 0, 0, 0
	}
	if cfg.IsPercent {
		percent = cfg.Percent
		total = baseCost * percent
	} else {
		if cfg.HasPercent {
			percent = cfg.Percent
			total += baseCost * percent
		}
		if cfg.HasFixed {
			fixed = cfg.FixedAmount
			total += fixed
		}
	}
	return baseCost + total, percent, fixed, total
}

// marginFor finds the markup for a provider. With no specific configuration ok is false and the caller keeps the original price.
func marginFor(provider string, margins map[string]Margin) (Margin, bool) {
	if provider != "" {
		if cfg, ok := margins[provider]; ok {
			return cfg, true
		}
	}
	cfg, ok := margins["global"]
	return cfg, ok
}

// TokenCost is the cost of a text completion when cache tokens are not billed separately.
// Prompt tokens multiply the input rate and completion tokens multiply the output rate. Rates are dollars per token.
func TokenCost(promptTokens, completionTokens int, inputRate, outputRate float64) (float64, float64) {
	return float64(promptTokens) * inputRate, float64(completionTokens) * outputRate
}

// MapTrafficType folds a Gemini usageMetadata.trafficType into a billing tier.
//
// ON_DEMAND_PRIORITY uses the priority rate. FLEX, BATCH, and ON_DEMAND_FLEX use the flex rate.
// ON_DEMAND is the standard price. The tier is empty, but known and standard are both true.
// An empty string or an unknown value sets known to false, and the caller should fall back to the standard price.
func MapTrafficType(trafficType string) (tier string, known, standard bool) {
	if trafficType == "" {
		return "", false, false
	}
	switch strings.ToUpper(trafficType) {
	case "ON_DEMAND_PRIORITY":
		return "priority", true, false
	case "FLEX", "BATCH", "ON_DEMAND_FLEX":
		return "flex", true, false
	case "ON_DEMAND":
		return "", true, true
	default:
		return "", false, false
	}
}

// NormalizeServiceTier reports whether service_tier can be used to look up a price.
//
// "auto" is only a routing preference, not a billing tier. A non-string is not one either. Both return ok false,
// so a later lookup does not call lower on an empty string and mis-read the standard price.
func NormalizeServiceTier(serviceTier string, isString bool) (string, bool) {
	if !isString || strings.EqualFold(serviceTier, "auto") {
		return "", false
	}
	return serviceTier, true
}
