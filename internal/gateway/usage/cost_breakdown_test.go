package usage

import (
	"testing"
	"time"

	"github.com/sunqirui1987/xhub/internal/catalog"
	"github.com/sunqirui1987/xhub/internal/iam"
)

func TestCostBreakdownMultipliesTokensByRate(t *testing.T) {
	// 没有快照的历史行按调用开始时刻重算，并在响应里标明是重算的。
	got := costBreakdown(iam.UsageEvent{
		Model: "gpt-oss-120b", PromptTokens: 96, CompletionTokens: 0,
		Cost: 96 * rateOf(t, "gpt-oss-120b"), TS: offpeakTestInstant(),
	})
	if got["source"] != "recomputed" {
		t.Fatalf("a row with no stored snapshot must say the number was recomputed: %#v", got["source"])
	}
	if got["output_cost"] != 0.0 {
		t.Fatalf("output cost: %#v", got["output_cost"])
	}
	if got["input_cost"] == nil || got["original_cost"] == nil {
		t.Fatalf("bill missing sides: %#v", got)
	}
}

// rateOf reads the input rate a model is billed at when nothing is cached, at an
// off-peak instant so the answer does not depend on when the test runs.
func rateOf(t *testing.T, model string) float64 {
	t.Helper()
	charge, ok := catalog.CostAt(model, catalog.Usage{PromptTokens: 1}, offpeakTestInstant())
	if !ok || charge.Input <= 0 {
		t.Fatalf("%s has no per-token input rate", model)
	}
	return charge.Input
}

// offpeakTestInstant is a moment outside the peak window, so a test that is not
// about the window is not affected by the hour it runs at.
func offpeakTestInstant() time.Time {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.FixedZone("CST", 8*3600)
	}
	return time.Date(2026, 3, 2, 20, 0, 0, 0, loc)
}

// TestCostBreakdownReadsTheStoredSnapshot 是这次改动的关键断言：日志详情读
// 存下来的单价，不按今天的价目表重算。
//
// 重算的后果是同一行日志的"原价"会随着改价而变，分时模型更严重——上午点开
// 和晚上点开显示不同的数字。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestCostBreakdownReadsTheStoredSnapshot(t *testing.T) {
	// 这一行写着按每 token 0.001 美元、共 1000 个 token 扣的费。
	row := iam.UsageEvent{
		Model: "gpt-oss-120b", PromptTokens: 1000, Cost: 1.0, TS: offpeakTestInstant(),
		PriceSnapshot: `{"window":"peak","applied":[{"measure":"token","side":"input",` +
			`"variant":"uncached","unit_size":1000,"usd":0.001,"quantity":1000,"source_key":"ncache_peak"}]}`,
	}
	got := costBreakdown(row)
	if got["source"] != "snapshot" {
		t.Fatalf("the stored snapshot was not used: %#v", got)
	}
	if got["window"] != "peak" {
		t.Fatalf("window = %#v, want the window the call actually fell in", got["window"])
	}
	if got["input_cost"] != 1.0 {
		t.Fatalf("input cost = %#v, want the amount the snapshot records", got["input_cost"])
	}
	if got["input_cost_per_token"] != 0.001 {
		t.Fatalf("rate = %#v, want the stored rate and not today's", got["input_cost_per_token"])
	}
	// 明细里每一档都要带自己的单位，控制台才能不假设所有模型都按 token 计费。
	applied, _ := got["applied"].([]map[string]any)
	if len(applied) != 1 || applied[0]["measure"] != "token" || applied[0]["quantity"] != 1000.0 {
		t.Fatalf("applied rates: %#v", got["applied"])
	}
}

// TestCostBreakdownIsNotRewrittenByAPriceChange 钉住快照存在的理由：改价目表
// 之后读同一行日志，数字不能动。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestCostBreakdownIsNotRewrittenByAPriceChange(t *testing.T) {
	row := iam.UsageEvent{
		Model: "gpt-oss-120b", PromptTokens: 1000, Cost: 2.0, TS: offpeakTestInstant(),
		PriceSnapshot: `{"window":"offpeak","applied":[{"measure":"token","side":"input",` +
			`"variant":"uncached","unit_size":1000,"usd":0.002,"quantity":1000}]}`,
	}
	first := costBreakdown(row)
	// 同一条历史行再读一次，结果必须逐字相同。
	second := costBreakdown(row)
	if first["input_cost"] != second["input_cost"] || first["original_cost"] != second["original_cost"] {
		t.Fatalf("reading the same row twice gave different numbers: %#v vs %#v", first, second)
	}
	if first["input_cost"] != 2.0 {
		t.Fatalf("the recorded amount drifted: %#v", first["input_cost"])
	}
}

// TestCostBreakdownFallsBackForRowsWithoutASnapshot 证明没有快照的旧行仍然能
// 显示，但标明数字是重算的，不假装是原始记录。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestCostBreakdownFallsBackForRowsWithoutASnapshot(t *testing.T) {
	got := costBreakdown(iam.UsageEvent{
		Model: "gpt-oss-120b", PromptTokens: 96, Cost: 0.000384, TS: offpeakTestInstant(),
	})
	if got["source"] != "recomputed" {
		t.Fatalf("source = %#v, want recomputed", got["source"])
	}
}

// TestCostBreakdownTreatsAnUnreadableSnapshotAsAbsent 证明坏掉的快照不会让
// 整个日志页报错，而是退回重算并标明。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestCostBreakdownTreatsAnUnreadableSnapshotAsAbsent(t *testing.T) {
	got := costBreakdown(iam.UsageEvent{
		Model: "gpt-oss-120b", PromptTokens: 96, Cost: 0.000384,
		TS: offpeakTestInstant(), PriceSnapshot: `{"window":"peak","applied":`,
	})
	if got["source"] != "recomputed" {
		t.Fatalf("a truncated snapshot was not treated as absent: %#v", got)
	}
}

// TestCostBreakdownReportsThePromptSideWhole 钉住一个会让界面显示负数的约定。
//
// 日志抽屉渲染的 "Input Cost" 是 input_cost 减掉它也显示的缓存那两行。
// 所以 input_cost 必须包含缓存的那部分——否则一条缓存占大头的调用会显示成负数。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestCostBreakdownReportsThePromptSideWhole(t *testing.T) {
	row := iam.UsageEvent{
		Model: "gpt-oss-120b", PromptTokens: 1000, Cost: 0.36, TS: offpeakTestInstant(),
		PriceSnapshot: `{"window":"offpeak","applied":[` +
			`{"measure":"token","side":"cache_read","unit_size":1000,"usd":0.0003,"quantity":800},` +
			`{"measure":"token","side":"input","variant":"uncached","unit_size":1000,"usd":0.003,"quantity":200},` +
			`{"measure":"token","side":"output","unit_size":1000,"usd":0.006,"quantity":50}]}`,
	}
	got := costBreakdown(row)
	input, _ := got["input_cost"].(float64)
	cache, _ := got["cache_read_cost"].(float64)
	output, _ := got["output_cost"].(float64)

	// 提示侧整体：800 个缓存 × 0.0003 + 200 个未命中 × 0.003 = 0.84。
	if input != 0.84 {
		t.Fatalf("input_cost = %v, want the whole prompt side including the cached part", input)
	}
	if input-cache != 0.6 {
		t.Fatalf("netting the cache row off gave %v; the drawer would render a negative Input Cost", input-cache)
	}
	if output != 0.3 {
		t.Fatalf("output_cost = %v", output)
	}
}

// TestCostBreakdownDoesNotInventAPerTokenRateForPictures 证明按张的输入价不会
// 被当成每 token 价显示。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestCostBreakdownDoesNotInventAPerTokenRateForPictures(t *testing.T) {
	row := iam.UsageEvent{
		Model: "image-model", Cost: 0.05, TS: offpeakTestInstant(),
		PriceSnapshot: `{"window":"all","applied":[` +
			`{"measure":"picture","side":"input","unit_size":1,"usd":0.0025,"quantity":2}]}`,
	}
	got := costBreakdown(row)
	if _, present := got["input_cost_per_token"]; present {
		t.Fatalf("a per-picture rate was reported as a per-token rate: %#v", got["input_cost_per_token"])
	}
	if got["input_cost"] != 0.005 {
		t.Fatalf("input_cost = %#v, want the picture charge", got["input_cost"])
	}
}

// TestCostBreakdownAlwaysReportsBothSides 证明只要定了价，两侧就都要出现。
//
// 只报一侧会让前端退回"按 token 比例估算"，那是一个编出来的数字，
// 但显示得和真的一模一样。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestCostBreakdownAlwaysReportsBothSides(t *testing.T) {
	// 只有输出侧：视频这类模型很常见。
	row := iam.UsageEvent{
		Model: "video-model", Cost: 0.4, TS: offpeakTestInstant(),
		PriceSnapshot: `{"window":"all","applied":[` +
			`{"measure":"second","side":"output","unit_size":1,"usd":0.05,"quantity":8}]}`,
	}
	got := costBreakdown(row)
	if got["input_cost"] != 0.0 {
		t.Fatalf("input_cost = %#v, want an explicit zero rather than a missing key", got["input_cost"])
	}
	if got["output_cost"] != 0.4 {
		t.Fatalf("output_cost = %#v", got["output_cost"])
	}
}

// TestCostBreakdownKeepsAnUnpricedRowUnpriced 证明未定价的行不会凭空出现明细。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestCostBreakdownKeepsAnUnpricedRowUnpriced(t *testing.T) {
	got := costBreakdown(iam.UsageEvent{
		Model: "no-such-model-here", PromptTokens: 96, Cost: 0, TS: offpeakTestInstant(),
	})
	if got["total_cost"] != 0.0 {
		t.Fatalf("total: %#v", got["total_cost"])
	}
	if _, present := got["input_cost"]; present {
		t.Fatalf("an unpriced row grew a breakdown: %#v", got)
	}
}

func TestEventRowCarriesTheBill(t *testing.T) {
	rows := eventRows([]iam.UsageEvent{{
		RequestID: "r1", Model: "gpt-oss-120b", PromptTokens: 96, CompletionTokens: 0,
		Cost: 0.000384, TS: time.Now().UTC(),
	}})
	meta, _ := rows[0]["metadata"].(map[string]any)
	bill, _ := meta["cost_breakdown"].(map[string]any)
	if bill["input_cost"] == nil || bill["output_cost"] == nil || bill["original_cost"] == nil {
		t.Fatalf("bill missing sides: %#v", bill)
	}
	if bill["total_cost"] != 0.000384 {
		t.Fatalf("charged total: %#v", bill["total_cost"])
	}
}

func TestEventRowExposesTheConsoleColumns(t *testing.T) {
	ttft := 420
	cached := 100
	end := time.Now().UTC()
	start := end.Add(-time.Second)
	rows := eventRows([]iam.UsageEvent{{
		RequestID: "r-cols", Model: "gpt-oss-120b", PromptTokens: 10, CompletionTokens: 2,
		Cost: 0.01, TS: start, EndedAt: &end, TTFTMs: &ttft, CacheHit: true,
		KeyHash: "hash-1", KeyAlias: "key-1", TeamAlias: "team-1",
		Provider: "openai", CachedTokens: &cached, SessionID: "sess-1", CacheKey: "ck-1",
	}})
	row := rows[0]
	meta := row["metadata"].(map[string]any)
	if meta["user_api_key"] != "hash-1" || meta["user_api_key_alias"] != "key-1" || meta["user_api_key_team_alias"] != "team-1" {
		t.Fatalf("snapshot %#v", meta)
	}
	if row["completionStartTime"] == nil || row["cache_hit"] != "true" || row["session_id"] != "sess-1" {
		t.Fatalf("row %#v", row)
	}
	if row["custom_llm_provider"] != "openai" || row["cache_key"] != "ck-1" || meta["cached_tokens"] != 100 {
		t.Fatalf("provider/cache %#v", row)
	}
	if row["cache_read_input_tokens"] != 100 {
		t.Fatalf("cache read tokens %#v", row)
	}
	if row["endTime"] == row["startTime"] {
		t.Fatal("end time collapsed onto start")
	}
}

func TestCostBreakdownDoesNotRepriceUnpricedAsyncSnapshot(t *testing.T) {
	u := catalog.Usage{CompletionTokens: 40594, OutputVariant: "unknown", PricingModel: "gpt-oss-120b"}
	got := costBreakdown(iam.UsageEvent{
		Model: "gpt-oss-120b", CompletionTokens: 40594, TS: offpeakTestInstant(),
		PriceSnapshot: catalog.SnapshotUsage(catalog.Charge{}, u, false),
	})
	if got["source"] != "snapshot" || got["pricing_status"] != "unpriced" {
		t.Fatalf("lost unpriced state: %#v", got)
	}
	measured, _ := got["usage"].(*catalog.Usage)
	if measured == nil || *measured != u {
		t.Fatalf("lost measurements: %#v", got)
	}
	for _, key := range []string{"input_cost", "output_cost", "original_cost"} {
		if _, exists := got[key]; exists {
			t.Fatalf("invented %s: %#v", key, got)
		}
	}
}
