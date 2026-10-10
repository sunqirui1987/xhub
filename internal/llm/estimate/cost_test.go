package estimate

import (
	"math"
	"testing"
)

// TestDiscountAndMarginContract 验证折扣先应用、供应商加价优先于全局及费用分解。
// 前置为内存规则，覆盖正常、零值、缺项和显式禁用；结果必须保留费用与规则的对应关系，无外部数据清理。
func TestDiscountAndMarginContract(t *testing.T) {
	for _, tc := range []struct {
		name, provider      string
		rules               map[string]float64
		final, rate, amount float64
	}{
		{"供应商折扣", "openai", map[string]float64{"openai": .2}, 8, .2, 2},
		{"零折扣", "openai", map[string]float64{"openai": 0}, 10, 0, 0},
		{"供应商缺失", "unknown", map[string]float64{"openai": .2}, 10, 0, 0},
		{"空供应商不匹配空键", "", map[string]float64{"": .2}, 10, 0, 0},
		{"无配置", "openai", nil, 10, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			final, rate, amount := ApplyDiscount(10, tc.provider, tc.rules)
			if final != tc.final || rate != tc.rate || amount != tc.amount {
				t.Fatalf("折扣费用分解错误: final=%v rate=%v amount=%v", final, rate, amount)
			}
		})
	}
	for _, tc := range []struct {
		name, provider            string
		rules                     map[string]Margin
		final, rate, fixed, total float64
	}{
		{"纯百分比", "openai", map[string]Margin{"openai": {IsPercent: true, Percent: .1, FixedAmount: 99}}, 11, .1, 0, 1},
		{"百分比和固定额", "openai", map[string]Margin{"openai": {HasPercent: true, Percent: .2, HasFixed: true, FixedAmount: 3}}, 15, .2, 3, 5},
		{"全局回退", "unknown", map[string]Margin{"global": {HasFixed: true, FixedAmount: 2}}, 12, 0, 2, 2},
		{"空供应商全局规则", "", map[string]Margin{"global": {HasFixed: true, FixedAmount: 2}}, 12, 0, 2, 2},
		{"供应商零规则覆盖全局", "openai", map[string]Margin{"openai": {}, "global": {HasFixed: true, FixedAmount: 2}}, 10, 0, 0, 0},
		{"未启用字段不计费", "openai", map[string]Margin{"openai": {Percent: .2, FixedAmount: 3}}, 10, 0, 0, 0},
		{"无规则", "openai", nil, 10, 0, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			final, rate, fixed, total := ApplyMargin(10, tc.provider, tc.rules)
			if final != tc.final || rate != tc.rate || fixed != tc.fixed || total != tc.total {
				t.Fatalf("加价费用分解错误: final=%v rate=%v fixed=%v total=%v", final, rate, fixed, total)
			}
		})
	}
	discounted, _, _ := ApplyDiscount(10, "openai", map[string]float64{"openai": .2})
	final, _, _, total := ApplyMargin(discounted, "openai", map[string]Margin{"global": {IsPercent: true, Percent: .1}})
	if math.Abs(final-8.8) > 1e-12 || math.Abs(total-.8) > 1e-12 {
		t.Fatalf("先折扣后加价契约错误: final=%v markup=%v", final, total)
	}
}

// TestTokenCostContract 验证输入输出分别按每 token 费率计算，覆盖正常、零 token 和零费率。
// 前置为确定性计数和费率，结果核对费用量纲而不复制上层 HTTP 流程，无需数据清理。
func TestTokenCostContract(t *testing.T) {
	for _, tc := range []struct {
		prompt, completion                   int
		input, output, wantInput, wantOutput float64
	}{
		{1000, 2000, .000001, .000002, .001, .004}, {0, 0, .1, .2, 0, 0}, {10, 20, 0, 0, 0, 0},
	} {
		input, output := TokenCost(tc.prompt, tc.completion, tc.input, tc.output)
		if math.Abs(input-tc.wantInput) > 1e-12 || math.Abs(output-tc.wantOutput) > 1e-12 {
			t.Fatalf("token费用错误: input=%v output=%v", input, output)
		}
	}
}

// TestBillingTierContract 验证 Gemini 档位、未知值的标准回退标志及 OpenAI auto 路由偏好的识别。
// 前置为协议字符串和类型标志，正常、空值、大小写与非法类型均有断言，无网络或持久化清理。
func TestBillingTierContract(t *testing.T) {
	for _, tc := range []struct {
		value, tier     string
		known, standard bool
	}{
		{"ON_DEMAND_PRIORITY", "priority", true, false}, {"flex", "flex", true, false}, {"BATCH", "flex", true, false},
		{"ON_DEMAND_FLEX", "flex", true, false}, {"on_demand", "", true, true}, {"", "", false, false}, {"unsupported", "", false, false},
	} {
		tier, known, standard := MapTrafficType(tc.value)
		if tier != tc.tier || known != tc.known || standard != tc.standard {
			t.Fatalf("trafficType=%q 档位错误: %q/%v/%v", tc.value, tier, known, standard)
		}
	}
	for _, tc := range []struct {
		value    string
		isString bool
		want     string
		ok       bool
	}{
		{"priority", true, "priority", true}, {"flex", true, "flex", true}, {"AUTO", true, "", false}, {"priority", false, "", false},
		{"", true, "", true},
	} {
		value, ok := NormalizeServiceTier(tc.value, tc.isString)
		if value != tc.want || ok != tc.ok {
			t.Fatalf("service_tier=%q type=%v 归一化错误: %q/%v", tc.value, tc.isString, value, ok)
		}
	}
}
