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

// TestRowWithoutRatesIsBilledFromItsFlatFields 覆盖没有费率表的行。
//
// 这种行有两种来源：控制台手加的模型（只写扁平字段），以及 rates[] 之前生成的
// 行。它们表达不了时段和变体，但**确实写了价**。把它们判成未定价，等于让运维在
// 界面上填的价格一分钱都收不到——正是这次改动要修的那种静默漏收。
//
// 扁平字段是"基础价"（控制台把它标成空闲价），所以没有 _peak 字段时它按每个
// 小时都适用计费。这不是猜测：运维只报了一个价，就是每个小时这个价。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestRowWithoutRatesIsBilledFromItsFlatFields(t *testing.T) {
	installRow(t, "flat-only", map[string]any{
		"input_cost_per_token":  0.000003,
		"output_cost_per_token": 0.000015,
	})
	charge, ok := CostAt("flat-only", Usage{PromptTokens: 1000, CompletionTokens: 100}, peakInstant())
	if !ok {
		t.Fatal("a row that states two prices was reported as unpriced")
	}
	assertClose(t, "flat-only row", charge.Total, 1000*0.000003+100*0.000015)
	// 没有高峰价的行不能因为落在高峰时段就取不到价。
	if _, ok := CostAt("flat-only", Usage{PromptTokens: 10}, offpeakInstant()); !ok {
		t.Fatal("a row with no peak price was unpriced off-peak")
	}
}

// TestFlatPeakFieldIsHonoured 证明扁平形式里的 _peak 字段真的被读了。
//
// 控制台的单价表单有两组格子（空闲/高峰），提交的是 input_cost_per_token 和
// input_cost_per_token_peak。改动前计费只读前者，高峰时段按空闲价收——表单上
// 那一格填了不读，少收一半。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestFlatPeakFieldIsHonoured(t *testing.T) {
	installRow(t, "flat-peak", map[string]any{
		"input_cost_per_token":       0.000001,
		"input_cost_per_token_peak":  0.000002,
		"output_cost_per_token":      0.000004,
		"output_cost_per_token_peak": 0.000008,
	})
	usage := Usage{PromptTokens: 1000, CompletionTokens: 100}
	peak, ok := CostAt("flat-peak", usage, peakInstant())
	if !ok {
		t.Fatal("the peak rate was not usable")
	}
	off, ok := CostAt("flat-peak", usage, offpeakInstant())
	if !ok {
		t.Fatal("the off-peak rate was not usable")
	}
	assertClose(t, "flat peak", peak.Total, 1000*0.000002+100*0.000008)
	assertClose(t, "flat offpeak", off.Total, 1000*0.000001+100*0.000004)
}

// TestFlatNonTokenMeasuresAreBilled 覆盖扁平形式里的按张、按秒、按次价。
//
// 控制台的单价表单按计费维度分节，一个视频部署存的是 output_cost_per_second。
// 改动前计费只读每 token 的两个字段，取不到就记一行零费用——模型照常能用，
// 账单上是零，而且没有任何地方能看出来这是漏收而不是免费。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestFlatNonTokenMeasuresAreBilled(t *testing.T) {
	installRow(t, "flat-second", map[string]any{"output_cost_per_second": 0.05})
	charge, ok := CostAt("flat-second", Usage{Seconds: 8}, peakInstant())
	if !ok {
		t.Fatal("a per-second deployment was recorded as free")
	}
	assertClose(t, "eight seconds", charge.Total, 0.4)

	installRow(t, "flat-picture", map[string]any{"output_cost_per_image": 0.04347826})
	charge, ok = CostAt("flat-picture", Usage{Images: 3}, peakInstant())
	if !ok {
		t.Fatal("a per-image deployment was recorded as free")
	}
	assertClose(t, "three pictures", charge.Total, 3*0.04347826)

	installRow(t, "flat-query", map[string]any{"search_context_cost_per_query": 0.01})
	charge, ok = CostAt("flat-query", Usage{Searches: 2}, peakInstant())
	if !ok {
		t.Fatal("a per-query deployment was recorded as free")
	}
	assertClose(t, "two searches", charge.Total, 0.02)
}

// TestFlatCacheReadIsNotCountedTwice 钉住一个具体的算错法。
//
// 扁平分支曾经把缓存那部分算进 input，又把整段 cache 加进 total，于是带缓存的
// 调用多收了一遍缓存钱。这里断言总额就是各侧之和，且提示侧除以缓存价之外不再多收。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestFlatCacheReadIsNotCountedTwice(t *testing.T) {
	installRow(t, "flat-cache", map[string]any{
		"input_cost_per_token":        0.00001,
		"output_cost_per_token":       0.00002,
		"cache_read_input_token_cost": 0.000001,
	})
	charge, ok := CostAt("flat-cache", Usage{PromptTokens: 1000, CompletionTokens: 100, CachedTokens: 800}, peakInstant())
	if !ok {
		t.Fatal("a deployment with a cache price was unpriced")
	}
	want := 200*0.00001 + 800*0.000001 + 100*0.00002
	assertClose(t, "cached flat call", charge.Total, want)
	if charge.Total != charge.Input+charge.Output+charge.Cache {
		t.Fatalf("total %v is not the sum of its sides input=%v output=%v cache=%v",
			charge.Total, charge.Input, charge.Output, charge.Cache)
	}
	// 提示侧（含缓存那部分）要能覆盖缓存那一行，否则控制台减完会看到负数。
	if charge.Input+charge.Cache < charge.Cache {
		t.Fatalf("the prompt side %v does not cover the cache side %v", charge.Input, charge.Cache)
	}
}

// TestVariantOnlySideFallsBackAndSaysSo 覆盖只按变体报价的一侧。
//
// 视频和图片模型的价目表是按分辨率、按输入模态分的：一秒钟 1080p 和 4K 是两个价，
// 一张文生图和一张图生图也是。网关并不知道这次是哪一种——上游不报分辨率。
//
// 改动前这种侧直接取不到价，于是一次调用 ok=false，账上记零费用：模型照常能用，
// 钱一分没收到。现在取该侧最便宜的那一档，并且把这一档标成 fallback——选出来的
// 数字和量出来的数字不是一回事，账单要能分开。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestVariantOnlySideFallsBackAndSaysSo(t *testing.T) {
	installRow(t, "variant-seconds", ratesOf(t,
		Rate{Measure: "second", UnitSize: 1, Side: "output", Variant: "720p_v_duration", SourceKey: "720p_v_duration", Window: "all", USD: 0.10},
		Rate{Measure: "second", UnitSize: 1, Side: "output", Variant: "1080p_v_duration", SourceKey: "1080p_v_duration", Window: "all", USD: 0.30},
	))
	charge, ok := CostAt("variant-seconds", Usage{Seconds: 8}, peakInstant())
	if !ok {
		t.Fatal("a side quoted only in variants was reported as unpriced")
	}
	// 最便宜的那一档，不是最贵的，也不是平均值。
	assertClose(t, "variant fallback", charge.Total, 8*0.10)
	if len(charge.Applied) != 1 || !charge.Applied[0].Fallback {
		t.Fatalf("a chosen variant was not marked as one: %+v", charge.Applied)
	}
	if charge.Applied[0].SourceKey != "720p_v_duration" {
		t.Fatalf("the applied rate is %s, want the cheapest variant", charge.Applied[0].SourceKey)
	}
}

// TestAnUnqualifiedRateIsNotDisplacedByTheFallback 钉住反面。
//
// 一侧只要有无限定词的价，就以它为准，变体不能把它挤掉。否则 kling-v2 这种
// 同时报了"文生图/图生图"和一条普通出图价的模型，会按某个变体收钱。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestAnUnqualifiedRateIsNotDisplacedByTheFallback(t *testing.T) {
	installRow(t, "mixed-side", ratesOf(t,
		Rate{Measure: "picture", UnitSize: 1, Side: "output", SourceKey: "i_output_quantity", Window: "all", USD: 0.04347826},
		Rate{Measure: "picture", UnitSize: 1, Side: "output", Variant: "ti", SourceKey: "ti_quantity", Window: "all", USD: 0.00362319},
	))
	charge, ok := CostAt("mixed-side", Usage{Images: 1}, peakInstant())
	if !ok {
		t.Fatal("the call was not priced")
	}
	assertClose(t, "unqualified picture rate", charge.Total, 0.04347826)
	if charge.Applied[0].Fallback {
		t.Fatal("an exactly priced side was reported as a fallback choice")
	}
}

// TestACacheReadIsNeverBilledAtTheInputPrice 钉住一个具体的算错法。
//
// 输入侧的价格链先试 (input, cached)，再试 (cache_read, "")。如果缓存那两条都
// 没有，退到输入侧的兜底价就会把缓存读按输入价收——差两个数量级。
//
// 这里造一个输入侧只有变体价、没有缓存价的模型：缓存读必须取不到，而整段提示
// 按输入价收。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestACacheReadIsNeverBilledAtTheInputPrice(t *testing.T) {
	installRow(t, "no-cache-price", ratesOf(t,
		Rate{Measure: "token", UnitSize: 1, Side: "input", Variant: "uncached", SourceKey: "input", Window: "all", USD: 0.000003},
	))
	// 模型没有缓存价，所以整段提示按输入价算，不拆缓存。
	charge, ok := CostAt("no-cache-price", Usage{PromptTokens: 1000, CachedTokens: 800}, peakInstant())
	if !ok {
		t.Fatal("the call was not priced")
	}
	assertClose(t, "whole prompt at the input rate", charge.Total, 1000*0.000003)
	if charge.Cache != 0 {
		t.Fatalf("a model with no cache price billed a cache read: %v", charge.Cache)
	}
}

// TestASearchPriceIsBilledAsAQuery 覆盖联网搜索。
//
// 市场把"联网搜索数"放在单位名 second 下（web_search_req），但它是**次数**不是
// 秒数。改动前它被归成按秒，而计费找的是按次，两边对不上，44 个模型的搜索一次
// 都没收过钱；更糟的是它会去匹配真正的按秒费率。
//
// 现在它按 query 计费，且一次调用报秒数时不会误收搜索费。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestASearchPriceIsBilledAsAQuery(t *testing.T) {
	installRow(t, "search-model", ratesOf(t,
		Rate{Measure: "token", UnitSize: 1, Side: "output", SourceKey: "output", Window: "all", USD: 0.000015},
		Rate{Measure: "second", UnitSize: 1, Side: "output", Variant: searchVariant, SourceKey: "web_search_req", Window: "all", USD: 0.01},
	))
	charge, ok := CostAt("search-model", Usage{Searches: 3}, peakInstant())
	if !ok {
		t.Fatal("three searches were not priced")
	}
	assertClose(t, "three searches", charge.Total, 0.03)
	if charge.Applied[0].Measure != "query" {
		t.Fatalf("the search rate billed as %q, want query", charge.Applied[0].Measure)
	}
	// 反面：报秒数不该收到搜索费上——那是一条搜索，不是一段视频。
	charge, ok = CostAt("search-model", Usage{Seconds: 8}, peakInstant())
	if ok {
		t.Fatalf("a call reporting 8 seconds was billed from the search price: %v", charge.Total)
	}
}

// TestAQueryWithoutAPriceIsNotBillable 证明按次计费只在模型真的报了按次价时发生。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestAQueryWithoutAPriceIsNotBillable(t *testing.T) {
	installRow(t, "no-search-price", ratesOf(t,
		Rate{Measure: "token", UnitSize: 1, Side: "output", SourceKey: "output", Window: "all", USD: 0.000015},
	))
	if charge, ok := CostAt("no-search-price", Usage{Searches: 2}, peakInstant()); ok {
		t.Fatalf("a model with no search price billed searches: %v", charge.Total)
	}
}

// TestNormalizeUsageResolvesTheProviderShapes 覆盖用量归一化。
//
// 供应商对"输入 token"的定义不一致，这是最贵的一处不一致：
//
//	OpenAI        prompt_tokens 是整段提示，命中的部分在 prompt_tokens_details 里重述一次（子集）
//	Anthropic     input_tokens 只是没命中的那部分，cache_read_input_tokens 是另外一笔
//	Responses     同 Anthropic
//
// 用前者的读法读后者，800 个缓存 token 会整个丢掉，而剩下的 200 个反而按缓存价
// 收——一次调用少收一半。所以两种形状要按出现的键分开读，读出来都是"提示总数 +
// 其中的命中数"。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestNormalizeUsageResolvesTheProviderShapes(t *testing.T) {
	cases := []struct {
		name   string
		usage  map[string]any
		prompt int
		cached int
		write  int
		expect int
	}{
		{
			name:   "openai",
			usage:  map[string]any{"prompt_tokens": 1000, "completion_tokens": 50, "prompt_tokens_details": map[string]any{"cached_tokens": 800}},
			prompt: 1000, cached: 800, expect: 50,
		},
		{
			name:   "anthropic",
			usage:  map[string]any{"input_tokens": 200, "output_tokens": 50, "cache_read_input_tokens": 800, "cache_creation_input_tokens": 30},
			prompt: 1000, cached: 800, write: 30, expect: 50,
		},
		{
			// The Responses shape reports input_tokens as the whole prompt and
			// nests the cached subset beside it, the way OpenAI spells it.
			name:   "responses",
			usage:  map[string]any{"input_tokens": 1000, "output_tokens": 50, "input_tokens_details": map[string]any{"cached_tokens": 800}},
			prompt: 1000, cached: 800, expect: 50,
		},
		{
			name:   "top-level cache count",
			usage:  map[string]any{"prompt_tokens": 1000, "completion_tokens": 50, "cached_tokens": 800},
			prompt: 1000, cached: 800, expect: 50,
		},
		{
			name:   "no cache reported",
			usage:  map[string]any{"prompt_tokens": 1000, "completion_tokens": 50},
			prompt: 1000, cached: 0, expect: 50,
		},
	}
	for _, c := range cases {
		got := NormalizeUsage(c.usage)
		if got.PromptTokens != c.prompt {
			t.Errorf("%s: prompt tokens = %d, want %d", c.name, got.PromptTokens, c.prompt)
		}
		if got.CachedTokens != c.cached {
			t.Errorf("%s: cached tokens = %d, want %d", c.name, got.CachedTokens, c.cached)
		}
		if got.CacheWriteTokens != c.write {
			t.Errorf("%s: cache write = %d, want %d", c.name, got.CacheWriteTokens, c.write)
		}
		if got.CompletionTokens != c.expect {
			t.Errorf("%s: completion = %d, want %d", c.name, got.CompletionTokens, c.expect)
		}
	}
}

// TestNormalizeUsageReadsTheNonTokenMeasures 覆盖按张、按秒、按次的数量。
//
// 这三个量只有上游会报，而且各家拼法不同。读不到就记零费用，所以每一种拼法都要认。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestNormalizeUsageReadsTheNonTokenMeasures(t *testing.T) {
	got := NormalizeUsage(map[string]any{
		"images": 3, "seconds": 8.5, "searches": 2,
	})
	if got.Images != 3 {
		t.Errorf("images = %d, want 3", got.Images)
	}
	if got.Seconds != 8.5 {
		t.Errorf("seconds = %v, want 8.5", got.Seconds)
	}
	if got.Searches != 2 {
		t.Errorf("searches = %d, want 2", got.Searches)
	}
	// 别名也要认。
	got = NormalizeUsage(map[string]any{"output_images": 2, "duration_seconds": 4, "web_search_requests": 1})
	if got.Images != 2 || got.Seconds != 4 || got.Searches != 1 {
		t.Errorf("aliases not read: %+v", got)
	}
	// 上游没报的量是零，不是编出来的。
	got = NormalizeUsage(map[string]any{"prompt_tokens": 10})
	if got.Images != 0 || got.Seconds != 0 || got.Searches != 0 {
		t.Errorf("unreported measures were invented: %+v", got)
	}
}

// TestNormalizeUsageOfNothingIsZero 证明上游一个用量都没报时不会 panic 也不会编数。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestNormalizeUsageOfNothingIsZero(t *testing.T) {
	if got := NormalizeUsage(nil); got != (Usage{}) {
		t.Fatalf("nil usage normalized to %+v", got)
	}
	if got := NormalizeUsage(map[string]any{}); got != (Usage{}) {
		t.Fatalf("empty usage normalized to %+v", got)
	}
}

// TestRatesFromFlatKeepsEveryMeasure 覆盖扁平字段到费率表的转换。
//
// 控制台的单价表单存的是一条条扁平字段，不是一个费率表。这一段是两者之间的桥：
// 少了它，界面上的按秒价、按张价、按次价在计费时全部不存在。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestRatesFromFlatKeepsEveryMeasure(t *testing.T) {
	flat := map[string]float64{
		"input_cost_per_token":          0.000002,
		"output_cost_per_token":         0.000009,
		"cache_read_input_token_cost":   0.0000003,
		"output_cost_per_second":        0.05,
		"output_cost_per_image":         0.04347826,
		"search_context_cost_per_query": 0.01,
	}
	rates := RatesFromFlat(func(field string) (float64, bool) {
		v, ok := flat[field]
		return v, ok
	})
	peak := time.Date(2026, 3, 2, 10, 0, 0, 0, mustShanghai(t))
	charge, ok := CostFromRates(rates, Usage{PromptTokens: 100, CompletionTokens: 10, Seconds: 2, Images: 1, Searches: 1}, peak)
	if !ok {
		t.Fatal("the flat fields produced no billable rate")
	}
	want := 100*0.000002 + 10*0.000009 + 2*0.05 + 0.04347826 + 0.01
	assertClose(t, "flat fields billed together", charge.Total, want)
}

// TestRatesFromFlatMakesOnePriceApplyEveryHour 证明只报了一个价时它每个小时都适用。
//
// 运维只填了空闲那一格，就是"每个小时这个价"。把它当成只适用于空闲时段，会让高峰
// 时段的调用取不到价——把一张写明的价目表变成一行零费用。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestRatesFromFlatMakesOnePriceApplyEveryHour(t *testing.T) {
	rates := RatesFromFlat(func(field string) (float64, bool) {
		if field == "input_cost_per_token" {
			return 0.000003, true
		}
		return 0, false
	})
	for _, label := range []string{"peak", "offpeak"} {
		at := peakInstant()
		if label == "offpeak" {
			at = offpeakInstant()
		}
		charge, ok := CostFromRates(rates, Usage{PromptTokens: 1000}, at)
		if !ok {
			t.Fatalf("a single price was not applied at %s", label)
		}
		assertClose(t, label+" single price", charge.Total, 0.003)
	}
}

// TestRatesFromFlatKeepsThePeakLadder 证明扁平形式里的高峰价真的进了费率表。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestRatesFromFlatKeepsThePeakLadder(t *testing.T) {
	flat := map[string]float64{
		"output_cost_per_token":      0.000004,
		"output_cost_per_token_peak": 0.000008,
	}
	rates := RatesFromFlat(func(field string) (float64, bool) {
		v, ok := flat[field]
		return v, ok
	})
	peak, ok := CostFromRates(rates, Usage{CompletionTokens: 1000}, peakInstant())
	if !ok {
		t.Fatal("the peak price was not applied")
	}
	off, ok := CostFromRates(rates, Usage{CompletionTokens: 1000}, offpeakInstant())
	if !ok {
		t.Fatal("the off-peak price was not applied")
	}
	assertClose(t, "flat peak", peak.Total, 0.008)
	assertClose(t, "flat off-peak", off.Total, 0.004)
}

// TestRatesFromFlatOfNothingIsEmpty 证明一个价都没写时转换不出费率表。
//
// 空表是"没有价"，不是"零元"。catalog 的调用方据此记一行未定价。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestRatesFromFlatOfNothingIsEmpty(t *testing.T) {
	rates := RatesFromFlat(func(string) (float64, bool) { return 0, false })
	if len(rates) != 0 {
		t.Fatalf("no fields produced %d rates", len(rates))
	}
	if _, ok := CostFromFlatOrRates(func(string) (float64, bool) { return 0, false }, Usage{PromptTokens: 10}, peakInstant()); ok {
		t.Fatal("an unpriced source produced a charge")
	}
}

// mustShanghai is the billing location the peak rule is defined in. It fails the
// test rather than falling back, so a machine without the zone does not silently
// judge a different set of hours.
// 参数 t（*testing.T）：当前测试。
// 返回 *time.Location（*time.Location）：Asia/Shanghai。
// 调用：本文件的费率测试。
// 测试：自身即测试辅助。
func mustShanghai(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatalf("Asia/Shanghai is unavailable: %v", err)
	}
	return loc
}

// TestAPrefixedNameResolvesItsCatalogAlias 覆盖带供应商前缀的公开名。
//
// 部署通常按供应商命名（fenno/claude-haiku-4-5），而价目表按供应商自己的模型 id
// 收录（claude-haiku-4-5），别名表也是按那个 id 建的。只给完整名字查别名，就会漏掉
// 这一整类——于是带前缀的部署取不到目录价，只能靠部署自己写死单价，或者记零费用。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestAPrefixedNameResolvesItsCatalogAlias(t *testing.T) {
	// claude-haiku-4-5 在目录里是通过别名指向 claude-4.5-haiku 的：
	// 直接查这个名字查不到，必须先解析别名。
	bare, ok := priceRowFor("claude-haiku-4-5")
	if !ok {
		t.Fatal("the bare vendor model id did not resolve; this test needs a model that resolves by alias")
	}
	bareRates, _ := rateTableOf(bare)
	if len(bareRates) == 0 {
		t.Fatal("the bare vendor model id resolved to a row with no rates")
	}

	// 带前缀的同一个模型必须解析到同一行。
	prefixed, ok := priceRowFor("fenno/claude-haiku-4-5")
	if !ok {
		t.Fatal("a prefixed deployment name did not resolve, so a deployment named after its vendor cannot be priced from the catalog")
	}
	prefixedRates, _ := rateTableOf(prefixed)
	if len(prefixedRates) != len(bareRates) {
		t.Fatalf("the prefixed name resolved to %d rates, the bare id to %d; they must be the same row",
			len(prefixedRates), len(bareRates))
	}

	// 而且真的能算出钱。
	charge, ok := CostAt("fenno/claude-haiku-4-5", Usage{PromptTokens: 1000, CompletionTokens: 100}, peakInstant())
	if !ok || charge.Total <= 0 {
		t.Fatalf("a prefixed name was priced at %v (ok=%v)", charge.Total, ok)
	}
}

// TestAnUnpricedGatewayNameDoesNotHideTheCatalogRow 证明供应商用网关前缀再登记
// 一次、自己却不报价时，价目表里已有的那一行仍然能被这个网关名查到。
//
// 七牛把 Seedance 登记成 qiniu/bytedance/...，官方 id 就是价目表的键，登记本身没有单价。
// 再插一条空行、并把官方 id 指过去，查价会停在空行上，这次调用就变成没有价格。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestAnUnpricedGatewayNameDoesNotHideTheCatalogRow(t *testing.T) {
	const official = "bytedance/doubao-seedance-2-0-260128"
	at := time.Date(2026, 3, 7, 10, 0, 0, 0, time.UTC)
	usage := Usage{CompletionTokens: 1000}
	before, ok := CostAt(official, usage, at)
	if !ok || before.Total <= 0 {
		t.Fatalf("the catalog row for %s did not price: %v ok=%v", official, before.Total, ok)
	}
	Contribute(Row{
		ID: "qiniu/" + official, Provider: "qiniu", Official: official,
		TransportID: "qiniu_contents_generation", Mode: "qiniu_contents_generation",
	})
	got, ok := CostAt("qiniu/"+official, usage, at)
	if !ok || got.Total != before.Total {
		t.Fatalf("qiniu/%s priced %v ok=%v, catalog row priced %v", official, got.Total, ok, before.Total)
	}
	again, ok := CostAt(official, usage, at)
	if !ok || again.Total != before.Total {
		t.Fatalf("registering the gateway name changed the catalog price to %v ok=%v", again.Total, ok)
	}
}

// TestPriceKeysKeepTheirPreferenceOrder 证明候选键是"最具体优先"，而且去重不打乱顺序。
//
// 名字本身的拼法必须排在别名之前：一条部署的名字和目录里某条别名撞上时，要先按
// 调用方写的那一个查。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestPriceKeysKeepTheirPreferenceOrder(t *testing.T) {
	modelCostMu.RLock()
	keys := priceKeysForLocked("fenno/claude-haiku-4-5")
	modelCostMu.RUnlock()
	if len(keys) == 0 || keys[0] != "fenno/claude-haiku-4-5" {
		t.Fatalf("the name as written is not the first candidate: %q", keys)
	}
	if len(keys) < 2 || keys[1] != "claude-haiku-4-5" {
		t.Fatalf("the prefix-stripped name is not the second candidate: %q", keys)
	}
	seen := map[string]bool{}
	for _, key := range keys {
		if seen[key] {
			t.Fatalf("candidate %q appears twice: %q", key, keys)
		}
		seen[key] = true
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
