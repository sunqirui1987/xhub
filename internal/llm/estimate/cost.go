// 从价格表读取单次调用的输入和输出单价。找不到模型时不算零。
package estimate

import "strings"

// Margin 是一条加价规则。
//
// LiteLLM 的 cost_margin_config 有两种写法。数字表示纯百分比，例如 0.10 是加 10%，
// 这时只看 Percent，并把 IsPercent 设为 true。对象可以同时有 percentage 和
// fixed_amount，百分比按成本相乘，固定金额再加一次，单位都是美元。
type Margin struct {
	Percent     float64
	FixedAmount float64
	HasPercent  bool
	HasFixed    bool
	IsPercent   bool
}

// ApplyDiscount 按供应商标识从折扣表里取一个比例，从基础费用里减掉。
//
// 折扣表的键是 custom_llm_provider，值是小数：0.05 表示减 5%。
// 没有这个供应商，或者供应商名为空，费用保持原样。
// 返回值依次是折后费用、折扣比例、折扣金额。
func ApplyDiscount(baseCost float64, provider string, discounts map[string]float64) (final, percent, amount float64) {
	if provider != "" {
		if rate, ok := discounts[provider]; ok {
			amount = baseCost * rate
			return baseCost - amount, rate, amount
		}
	}
	return baseCost, 0, 0
}

// ApplyMargin 在折扣之后加价。供应商自己的规则优先于键名 global 的全局规则。
//
// 返回值依次是加价后的费用、百分比、固定金额、加价合计。
// 没有任何规则时四个值里后三个是 0，费用不变。
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

// 按供应商找加价。没有单独配置时 ok 为 false，调用方保持原价。
func marginFor(provider string, margins map[string]Margin) (Margin, bool) {
	if provider != "" {
		if cfg, ok := margins[provider]; ok {
			return cfg, true
		}
	}
	cfg, ok := margins["global"]
	return cfg, ok
}

// TokenCost 是文本补全不计缓存时的费用。
// 提示词 token 乘输入单价，补全 token 乘输出单价。单价是每 token 美元。
func TokenCost(promptTokens, completionTokens int, inputRate, outputRate float64) (float64, float64) {
	return float64(promptTokens) * inputRate, float64(completionTokens) * outputRate
}

// MapTrafficType 把 Gemini usageMetadata.trafficType 收成计费档。
//
// ON_DEMAND_PRIORITY 用 priority 单价。FLEX、BATCH、ON_DEMAND_FLEX 用 flex 单价。
// ON_DEMAND 是标准价，tier 为空但 known 和 standard 都为 true。
// 空字符串和不认识的值 known 为 false，调用方应回退到标准价。
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

// NormalizeServiceTier 判断 service_tier 能不能拿去查价。
//
// "auto" 只是路由偏好，不是账单档。非字符串也不是。这两种都返回 ok=false，
// 后面的查价不会对空字符串调用 lower，也就不会把标准价查错。
func NormalizeServiceTier(serviceTier string, isString bool) (string, bool) {
	if !isString || strings.EqualFold(serviceTier, "auto") {
		return "", false
	}
	return serviceTier, true
}
