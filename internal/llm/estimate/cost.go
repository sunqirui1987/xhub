// Package estimate reads the input and output unit price of one call. A missing model is not priced as zero.
package estimate

import (
	"github.com/sunqirui1987/xhub/internal/logx"
	"strings"
	"sync"
)

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

var logTraceOnceCost sync.Once

// ApplyDiscount looks up a rate for the provider in the discount map and subtracts it from the base cost. Discount-map keys are custom_llm_provider. The value is a fraction: 0.05 means subtract 5 percent. A missing provider, or an empty provider name, leaves the cost unchanged. The results are the discounted cost, the discount fraction, and the discount amount.
// 参数 baseCost（float64）：单价或基础费用。价格表按每 token 或每百万 token 给出；provider（string）：供应商标识，例如 openai 或 volcengine；discounts（map[string]float64）：应用Discount使用的数值表。缺键表示这项还没有数。
// 返回 final（float64）：加价或折扣之后的金额；percent（float64）：按百分比加价的比例；amount（float64）：按固定额加价的金额。
// 调用：仅在 cost.go 内使用
// 测试：无直接单测
func ApplyDiscount(baseCost float64, provider string, discounts map[string]float64) (final, percent, amount float64) {
	logTraceOnceCost.Do(func() { logx.Trace("enter estimate.ApplyDiscount") })

	if provider != "" {
		if rate, ok := discounts[provider]; ok {
			amount = baseCost * rate
			return baseCost - amount, rate, amount
		}
	}
	return baseCost, 0, 0
}

// ApplyMargin adds a markup after the discount. A provider's own rule wins over the global rule stored under the key global. The results are the marked-up cost, the percent, the fixed amount, and the markup total. With no rule the last three values are 0 and the cost is unchanged.
// 参数 baseCost（float64）：单价或基础费用。价格表按每 token 或每百万 token 给出；provider（string）：供应商标识，例如 openai 或 volcengine；margins（map[string]Margin）：应用Margin使用的map[string]Margin。
// 返回 final（float64）：加价或折扣之后的金额；percent（float64）：按百分比加价的比例；fixed（float64）：固定加价金额；total（float64）：这一次的总费用。
// 调用：仅在 cost.go 内使用
// 测试：无直接单测
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
// 参数 provider（string）：供应商标识，例如 openai 或 volcengine；margins（map[string]Margin）：margin为使用的map[string]Margin。
// 返回 Margin（Margin）：这个供应商的加价规则。没有专门配置时是零值；bool（bool）：找到了这个供应商的加价规则时返回真。没有专门配置时返回假，调用方保持原价。
// 调用：仅在 cost.go 内使用
// 测试：无直接单测
func marginFor(provider string, margins map[string]Margin) (Margin, bool) {
	if provider != "" {
		if cfg, ok := margins[provider]; ok {
			return cfg, true
		}
	}
	cfg, ok := margins["global"]
	return cfg, ok
}

// TokenCost is the cost of a text completion when cache tokens are not billed separately. Prompt tokens multiply the input rate and completion tokens multiply the output rate. Rates are dollars per token.
// 参数 promptTokens（int）：令牌费用使用的整数。零表示没有这项或尚未计数；completionTokens（int）：令牌费用使用的整数。零表示没有这项或尚未计数；inputRate（float64）：单价或基础费用。价格表按每 token 或每百万 token 给出；outputRate（float64）：单价或基础费用。价格表按每 token 或每百万 token 给出。
// 返回 float64（float64）：令牌费用。缺失时为 0，不要把它理解成免费除非调用方另有约定；float64（float64）：令牌费用。缺失时为 0，不要把它理解成免费除非调用方另有约定。
// 调用：仅在 cost.go 内使用
// 测试：无直接单测
func TokenCost(promptTokens, completionTokens int, inputRate, outputRate float64) (float64, float64) {
	return float64(promptTokens) * inputRate, float64(completionTokens) * outputRate
}

// MapTrafficType folds a Gemini usageMetadata.trafficType into a billing tier. ON_DEMAND_PRIORITY uses the priority rate. FLEX, BATCH, and ON_DEMAND_FLEX use the flex rate. ON_DEMAND is the standard price. The tier is empty, but known and standard are both true. An empty string or an unknown value sets known to false, and the caller should fall back to the standard price.
// 参数 trafficType（string）：表Traffic类型使用的traffic类型。空串表示调用方没有提供这项。
// 返回 tier（string）：命中的价格档；known（bool）：价目表里是否认识这个模型；standard（bool）：是否按标准档计价。
// 调用：仅在 cost.go 内使用
// 测试：无直接单测
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

// NormalizeServiceTier reports whether service_tier can be used to look up a price. "auto" is only a routing preference, not a billing tier. A non-string is not one either. Both return ok false, so a later lookup does not call lower on an empty string and mis-read the standard price.
// 参数 serviceTier（string）：NormalizeServiceTier使用的serviceTier。空串表示调用方没有提供这项；isString（bool）：为真时走是否字符串这一支。为假时保持原来的路径。
// 返回 string（string）：能用来查价的 service_tier。不能使用时为空串，同时布尔值为假；bool（bool）：service_tier 可以用来查价时返回真。auto 只是路由偏好，非字符串也不能查价，这两种都返回假。
// 调用：仅在 cost.go 内使用
// 测试：无直接单测
func NormalizeServiceTier(serviceTier string, isString bool) (string, bool) {
	if !isString || strings.EqualFold(serviceTier, "auto") {
		return "", false
	}
	return serviceTier, true
}
