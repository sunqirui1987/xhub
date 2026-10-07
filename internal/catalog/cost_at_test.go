package catalog

import (
	"testing"
	"time"
)

// peakInstant and offpeakInstant are two moments on the same weekday, chosen so
// that only the window differs. 2026-03-02 is a Monday with no holiday entry.
func peakInstant() time.Time    { return cst(2026, 3, 2, 10, 0) }
func offpeakInstant() time.Time { return cst(2026, 3, 2, 20, 0) }

// ratesOf is the price row a model with the given rates has. It builds the map
// the way the embedded catalog presents it, so the test exercises the same
// decode path production uses.
func ratesOf(t *testing.T, rates ...Rate) map[string]any {
	t.Helper()
	return map[string]any{"rates": rates}
}

// installRow puts a price row into the live map for the duration of the test.
func installRow(t *testing.T, model string, row map[string]any) {
	t.Helper()
	modelCostMu.Lock()
	defer modelCostMu.Unlock()
	raw, ok := modelCostMapValue.(map[string]any)
	if !ok {
		raw = map[string]any{}
		modelCostMapValue = raw
	}
	raw[model] = row
}

// tokenRates builds an input/output rate pair for one window.
func tokenRate(side, window string, usd float64) Rate {
	return Rate{
		Measure: "token", UnitSize: 1000, Side: side, Variant: "uncached",
		Window: window, SourceKey: side + "_" + window, USD: usd,
	}
}

// TestPeakCostsTwiceOffpeak 是这次改动的核心断言：同一批 token，落在高峰时段
// 的那次调用正好贵一倍。
//
// 修之前的行为是：两档价都在表里，但计费只读扁平字段（等于空闲价），
// 所以高峰调用按空闲价收——少收一半，而且不报错。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestPeakCostsTwiceOffpeak(t *testing.T) {
	installRow(t, "peak-model", ratesOf(t,
		Rate{Measure: "token", UnitSize: 1, Side: "input", Variant: "uncached", Window: "offpeak", USD: 0.00000065},
		Rate{Measure: "token", UnitSize: 1, Side: "input", Variant: "uncached", Window: "peak", USD: 0.0000013},
		Rate{Measure: "token", UnitSize: 1, Side: "output", Window: "offpeak", USD: 0.00000195},
		Rate{Measure: "token", UnitSize: 1, Side: "output", Window: "peak", USD: 0.0000039},
	))
	usage := Usage{PromptTokens: 1000, CompletionTokens: 500}

	peak, ok := CostAt("peak-model", usage, peakInstant())
	if !ok {
		t.Fatal("peak call was not priced")
	}
	off, ok := CostAt("peak-model", usage, offpeakInstant())
	if !ok {
		t.Fatal("off-peak call was not priced")
	}

	if peak.Window != "peak" || off.Window != "offpeak" {
		t.Fatalf("windows: peak=%q off=%q", peak.Window, off.Window)
	}
	// 0.00000065/1e6... the rates are per token here, so 1000 tokens at
	// 1.3e-6 is 1.3e-3.
	wantPeak := 1000*0.0000013 + 500*0.0000039
	wantOff := 1000*0.00000065 + 500*0.00000195
	assertClose(t, "peak total", peak.Total, wantPeak)
	assertClose(t, "offpeak total", off.Total, wantOff)
	assertClose(t, "peak is double offpeak", peak.Total, off.Total*2)
}

// TestUnwindowedModelCostsTheSameAtBothInstants 钉住反面：不分时的模型在
// 高峰和空闲算出来的价必须一样。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestUnwindowedModelCostsTheSameAtBothInstants(t *testing.T) {
	installRow(t, "flat-model", ratesOf(t,
		Rate{Measure: "token", UnitSize: 1, Side: "input", Variant: "uncached", Window: "all", USD: 0.000003},
		Rate{Measure: "token", UnitSize: 1, Side: "output", Window: "all", USD: 0.000015},
	))
	usage := Usage{PromptTokens: 100, CompletionTokens: 50}
	a, ok := CostAt("flat-model", usage, peakInstant())
	if !ok {
		t.Fatal("peak call was not priced")
	}
	b, ok := CostAt("flat-model", usage, offpeakInstant())
	if !ok {
		t.Fatal("off-peak call was not priced")
	}
	if a.Total != b.Total {
		t.Fatalf("an unwindowed model charged %v at peak and %v off-peak", a.Total, b.Total)
	}
	if a.Window != "peak" {
		t.Fatalf("window recorded as %q; the instant is still reported even when the rate does not vary", a.Window)
	}
}

// TestPeakNeverFallsBackToTheCheapRate 证明分时模型在高峰时段不会被静默地按
// 空闲价计费——这正是改动前发生的事。
//
// 只有空闲价、没有高峰价的模型是自相矛盾的，宁可判成未定价让人看见，
// 也不能悄悄按便宜的那档收。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestPeakNeverFallsBackToTheCheapRate(t *testing.T) {
	installRow(t, "offpeak-only", ratesOf(t,
		Rate{Measure: "token", UnitSize: 1, Side: "input", Variant: "uncached", Window: "offpeak", USD: 0.00000065},
	))
	if charge, ok := CostAt("offpeak-only", Usage{PromptTokens: 1000}, peakInstant()); ok {
		t.Fatalf("a peak call was priced at %v using the off-peak rate", charge.Total)
	}
	// 空闲时段仍然要能算出来。
	if _, ok := CostAt("offpeak-only", Usage{PromptTokens: 1000}, offpeakInstant()); !ok {
		t.Fatal("the off-peak rate was not used at an off-peak instant")
	}
}

// TestCacheReadIsNotBilledAtTheInputRate 覆盖缓存读。
//
// 缓存的价通常比输入价低两个数量级。把它当成输入价，一次带缓存的调用会
// 多收几百倍；反过来漏掉它，则少收。两种都要钉住。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestCacheReadIsNotBilledAtTheInputRate(t *testing.T) {
	installRow(t, "cache-model", ratesOf(t,
		Rate{Measure: "token", UnitSize: 1, Side: "input", Variant: "uncached", Window: "all", USD: 0.000003},
		Rate{Measure: "token", UnitSize: 1, Side: "cache_read", Window: "all", USD: 0.0000003},
	))
	// 800 命中的缓存 + 200 未命中，共 1000 个提示 token。
	charge, ok := CostAt("cache-model", Usage{PromptTokens: 1000, CachedTokens: 800}, peakInstant())
	if !ok {
		t.Fatal("a cached call was not priced")
	}
	want := 800*0.0000003 + 200*0.000003
	assertClose(t, "cached prompt", charge.Total, want)
	if charge.Cache <= 0 {
		t.Fatal("the cache read was not broken out separately")
	}
}

// TestCachedVariantOfInputSideIsHonoured 覆盖另一种拼法：缓存价写在输入侧的
// cached 变体上，而不是单独的 cache_read 侧。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestCachedVariantOfInputSideIsHonoured(t *testing.T) {
	installRow(t, "variant-cache", ratesOf(t,
		Rate{Measure: "token", UnitSize: 1, Side: "input", Variant: "uncached", Window: "all", USD: 0.000003},
		Rate{Measure: "token", UnitSize: 1, Side: "input", Variant: "cached", Window: "all", USD: 0.0000003},
	))
	charge, ok := CostAt("variant-cache", Usage{PromptTokens: 1000, CachedTokens: 800}, peakInstant())
	if !ok {
		t.Fatal("a cached call was not priced")
	}
	assertClose(t, "cached variant", charge.Total, 800*0.0000003+200*0.000003)
}

// TestCachedTokensAreNotDoubleBilled 钉住一个具体的算错法：缓存的那部分不能
// 既按缓存价收一次、又按输入价收一次。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestCachedTokensAreNotDoubleBilled(t *testing.T) {
	installRow(t, "no-double", ratesOf(t,
		Rate{Measure: "token", UnitSize: 1, Side: "input", Variant: "uncached", Window: "all", USD: 0.000003},
		Rate{Measure: "token", UnitSize: 1, Side: "input", Variant: "cached", Window: "all", USD: 0.0000003},
	))
	// 全部命中缓存时，未命中的部分是零，只能收缓存那一份。
	charge, ok := CostAt("no-double", Usage{PromptTokens: 1000, CachedTokens: 1000}, peakInstant())
	if !ok {
		t.Fatal("a fully cached call was not priced")
	}
	assertClose(t, "fully cached", charge.Total, 1000*0.0000003)
}

// TestCacheHitAbovePromptCountIsClamped 覆盖上游自相矛盾的用量：命中数比
// 提示 token 还多。未命中的部分不能变成负数。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestCacheHitAbovePromptCountIsClamped(t *testing.T) {
	installRow(t, "clamped", ratesOf(t,
		Rate{Measure: "token", UnitSize: 1, Side: "input", Variant: "uncached", Window: "all", USD: 0.000003},
		Rate{Measure: "token", UnitSize: 1, Side: "input", Variant: "cached", Window: "all", USD: 0.0000003},
	))
	charge, ok := CostAt("clamped", Usage{PromptTokens: 100, CachedTokens: 500}, peakInstant())
	if !ok {
		t.Fatal("the call was not priced")
	}
	assertClose(t, "clamped cache hit", charge.Total, 100*0.0000003)
	if charge.Total < 0 {
		t.Fatalf("a negative charge came out of an inconsistent usage report: %v", charge.Total)
	}
}

// TestModelWithoutACacheRateBillsTheWholePromptAsInput 证明没有缓存价的模型
// 不会被少收：整段提示按输入价算。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestModelWithoutACacheRateBillsTheWholePromptAsInput(t *testing.T) {
	installRow(t, "no-cache-rate", ratesOf(t,
		Rate{Measure: "token", UnitSize: 1, Side: "input", Variant: "uncached", Window: "all", USD: 0.000003},
	))
	charge, ok := CostAt("no-cache-rate", Usage{PromptTokens: 1000, CachedTokens: 800}, peakInstant())
	if !ok {
		t.Fatal("the call was not priced")
	}
	assertClose(t, "whole prompt at input rate", charge.Total, 1000*0.000003)
}

// TestPerSecondModelIsBilledBySeconds 覆盖按秒计费的模型。
//
// 视频和语音合成的上游不返回 token 用量，改动前这类调用记一行零费用。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestPerSecondModelIsBilledBySeconds(t *testing.T) {
	installRow(t, "video-model", ratesOf(t,
		Rate{Measure: "second", UnitSize: 1, Side: "output", Window: "all", USD: 0.05},
	))
	charge, ok := CostAt("video-model", Usage{Seconds: 8}, peakInstant())
	if !ok {
		t.Fatal("a per-second call was not priced")
	}
	assertClose(t, "eight seconds", charge.Total, 0.4)
}

// TestPerPictureModelIsBilledByPictures 覆盖按张计费的模型。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestPerPictureModelIsBilledByPictures(t *testing.T) {
	installRow(t, "image-model", ratesOf(t,
		Rate{Measure: "picture", UnitSize: 1, Side: "output", Window: "all", USD: 0.04347826},
	))
	charge, ok := CostAt("image-model", Usage{Images: 3}, peakInstant())
	if !ok {
		t.Fatal("a per-picture call was not priced")
	}
	assertClose(t, "three pictures", charge.Total, 3*0.04347826)
}

// TestMeasuredSidesDoNotBleedIntoEachOther 证明按秒的模型不会被按 token 收费，
// 反之亦然。上游只报其一，另一种数量是零。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestMeasuredSidesDoNotBleedIntoEachOther(t *testing.T) {
	installRow(t, "video-with-tokens", ratesOf(t,
		Rate{Measure: "second", UnitSize: 1, Side: "output", Window: "all", USD: 0.05},
		Rate{Measure: "token", UnitSize: 1, Side: "output", Window: "all", USD: 0.0000033},
	))
	// 只报秒数：token 数量是零，那一档不该被算进去。
	charge, ok := CostAt("video-with-tokens", Usage{Seconds: 2}, peakInstant())
	if !ok {
		t.Fatal("the call was not priced")
	}
	assertClose(t, "seconds only", charge.Total, 0.1)
	for _, applied := range charge.Applied {
		if applied.Measure == "token" {
			t.Fatalf("a token rate was charged for a call that reported no tokens: %+v", applied)
		}
	}
}

// TestUnknownModelIsNotPriced 证明目录里没有的模型返回 ok false，而不是零。
//
// 两者不是一回事：零是"免费"，false 是"不知道多少钱"。调用方据此记一行
// 未定价，而不是静默按免费处理。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestUnknownModelIsNotPriced(t *testing.T) {
	if _, ok := CostAt("no-such-model-anywhere", Usage{PromptTokens: 10}, peakInstant()); ok {
		t.Fatal("a model absent from the price table was priced")
	}
}

// TestZeroQuantityIsNotPriced 证明一次什么用量都没报的调用不会被算成 0 元。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestZeroQuantityIsNotPriced(t *testing.T) {
	installRow(t, "silent-model", ratesOf(t,
		Rate{Measure: "token", UnitSize: 1, Side: "input", Variant: "uncached", Window: "all", USD: 0.000003},
	))
	if _, ok := CostAt("silent-model", Usage{}, peakInstant()); ok {
		t.Fatal("a call that reported no usage came back priced")
	}
}

// TestAppliedRatesAreRecordedForTheSnapshot 证明账单里留下了这次实际用到的费率。
//
// 这是日志详情能显示"当时按什么价扣的"的依据。没有它，历史账目只能按今天
// 的价目表重算——价一变，解释就变了。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestAppliedRatesAreRecordedForTheSnapshot(t *testing.T) {
	installRow(t, "snapshot-model", ratesOf(t,
		Rate{Measure: "token", UnitSize: 1, Side: "input", Variant: "uncached", Window: "peak", SourceKey: "ncache_peak", USD: 0.0000013},
	))
	charge, ok := CostAt("snapshot-model", Usage{PromptTokens: 1000}, peakInstant())
	if !ok {
		t.Fatal("the call was not priced")
	}
	if len(charge.Applied) != 1 {
		t.Fatalf("applied rates = %d, want 1", len(charge.Applied))
	}
	got := charge.Applied[0]
	if got.SourceKey != "ncache_peak" || got.USD != 0.0000013 || got.Quantity != 1000 {
		t.Fatalf("applied rate did not record what was used: %+v", got)
	}
	if charge.Window != "peak" {
		t.Fatalf("applied window = %q, want peak", charge.Window)
	}
}

// TestUnitSizeIsAppliedOnce 钉住 unit_size 只除一次。
//
// 市场报的是每 1000 个 token 的价，buildRates 已经除过 unit_size 才写进 usd。
// 计费再除一次就是少收一千倍。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestUnitSizeIsAppliedOnce(t *testing.T) {
	// 每 1000 token 1.3e-3 美元 → 每 token 1.3e-6，也就是 usd 已经是单价。
	installRow(t, "unit-model", ratesOf(t,
		Rate{Measure: "token", UnitSize: 1000, Side: "output", Window: "all", USD: 0.0000013},
	))
	charge, ok := CostAt("unit-model", Usage{CompletionTokens: 1000}, peakInstant())
	if !ok {
		t.Fatal("the call was not priced")
	}
	assertClose(t, "1000 output tokens", charge.Total, 0.0013)
}

// TestRateTableIsReadFromJSONShape 证明从嵌入目录解析出来的形状也能用。
//
// 内嵌目录是从字节解析的，数字是 float64 而不是 Rate，走的是 decodeRate。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestRateTableIsReadFromJSONShape(t *testing.T) {
	installRow(t, "json-shape", map[string]any{
		"rates": []any{
			map[string]any{
				"measure": "token", "unit_size": float64(1000), "side": "output",
				"window": "all", "source_key": "output", "usd": 0.0000013,
			},
		},
	})
	charge, ok := CostAt("json-shape", Usage{CompletionTokens: 100}, peakInstant())
	if !ok {
		t.Fatal("a rate table parsed from JSON was not usable")
	}
	assertClose(t, "json-shaped rate", charge.Total, 100*0.0000013)
}

// TestRowWithoutRatesIsNotPriced 证明只有扁平字段、没有费率表的旧行不会被
// 拿扁平字段硬算——那正是丢掉 peak 的那条路。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestRowWithoutRatesIsNotPriced(t *testing.T) {
	installRow(t, "flat-only", map[string]any{
		"input_cost_per_token":  0.000003,
		"output_cost_per_token": 0.000015,
	})
	if _, ok := CostAt("flat-only", Usage{PromptTokens: 100}, peakInstant()); ok {
		t.Fatal("a row with no rate table was priced from its flat fields")
	}
}

// assertClose 比较两个金额，容忍浮点误差。
// 参数 t（*testing.T）：当前测试；label（string）：断言失败时显示的名字；got（float64）：实际值；want（float64）：期望值。
// 返回：无。差值超出相对容差时让测试失败。
func assertClose(t *testing.T, label string, got, want float64) {
	t.Helper()
	diff := got - want
	if diff < 0 {
		diff = -diff
	}
	limit := want * 1e-9
	if limit < 0 {
		limit = -limit
	}
	if limit < 1e-15 {
		limit = 1e-15
	}
	if diff > limit {
		t.Fatalf("%s = %v, want %v", label, got, want)
	}
}
