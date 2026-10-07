package catalog

import (
	"encoding/json"
	"testing"
)

// feedOf 把一段精简的市场响应喂给 BuildPriceDocument。
// 参数 t（*testing.T）：当前测试；models（...map[string]any）：市场里的模型条目。
// 返回 PriceDocument（PriceDocument）：建好的价格目录。
func feedOf(t *testing.T, models ...map[string]any) PriceDocument {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"status": true, "data": models})
	if err != nil {
		t.Fatalf("marshal feed: %v", err)
	}
	doc, err := BuildPriceDocument(raw)
	if err != nil {
		t.Fatalf("build price document: %v", err)
	}
	return doc
}

// feedModelWith 造一条市场模型，只带这次测试要断言的字段。
// 参数 id（string）：模型 id；outputs（[]string）：输出模态；details（map[string]any）：计价块。
// 返回 map[string]any（map[string]any）：一条市场模型。
func feedModelWith(id string, outputs []string, details map[string]any) map[string]any {
	anyOutputs := make([]any, 0, len(outputs))
	for _, o := range outputs {
		anyOutputs = append(anyOutputs, o)
	}
	return map[string]any{
		"id": id, "name": id,
		"issuer":            map[string]any{"name": "Kling"},
		"model_constraints": map[string]any{"context_length": 1000},
		"architecture": map[string]any{
			"output_modalities": anyOutputs,
			"input_modalities":  []any{"text"},
		},
		"pricing_rules_v2": []any{map[string]any{"details_v2": details}},
	}
}

// unit 造一段计价单位。
// 参数 usd（float64）：每 unit_size 个单位的美元价；size（float64）：单位大小；name（string）：单位名。
// 返回 map[string]any（map[string]any）：一段计价单位。
func unit(usd, size float64, name string) map[string]any {
	return map[string]any{
		"unit_name": name, "unit_size": size,
		"unit_price": usd * 7, "unit_price_usd": usd, "name": name,
	}
}

// TestPlainTokenModelKeepsBothSides 是最普通的一种：模型直接报输入和输出单价。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestPlainTokenModelKeepsBothSides(t *testing.T) {
	doc := feedOf(t, feedModelWith("plain-chat", []string{"text"}, map[string]any{
		"input":  unit(0.000003, 1, "token"),
		"output": unit(0.000015, 1, "token"),
	}))
	row := doc.Models["plain-chat"]
	if row["input_cost_per_token"] != 0.000003 {
		t.Fatalf("input rate %v", row["input_cost_per_token"])
	}
	if row["output_cost_per_token"] != 0.000015 {
		t.Fatalf("output rate %v", row["output_cost_per_token"])
	}
	// unit_size 是 1 时不该被再除一次。
	if row["mode"] != "chat" {
		t.Fatalf("mode %v", row["mode"])
	}
}

// TestVariantPricedVideoModelGetsABillableRate 是这次修的那个缺陷。
//
// 视频和图像模型的市场价不是"输入/输出"两条，而是按分辨率和输入方式分成十几档
// （480p_v_duration、4k_av_duration、wiv_v_output）。修之前这些模型一个可计费
// 字段都没有：控制台显示"价格未提供"，而调用它们会记一行零费用。
//
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestVariantPricedVideoModelGetsABillableRate(t *testing.T) {
	doc := feedOf(t, feedModelWith("variant-video", []string{"video"}, map[string]any{
		"480p_v_duration":  unit(0.02, 1, "second"),
		"720p_v_duration":  unit(0.05, 1, "second"),
		"1080p_v_duration": unit(0.12, 1, "second"),
	}))
	row := doc.Models["variant-video"]

	rate, ok := row["output_cost_per_second"]
	if !ok {
		t.Fatal("a video model priced only by variants has no billable rate")
	}
	// 取最便宜的一档：按运维没要的分辨率多收钱，比少收钱更糟。
	if rate != 0.02 {
		t.Fatalf("output_cost_per_second = %v, want the cheapest variant 0.02", rate)
	}
	if row["mode"] != "video_generation" {
		t.Fatalf("mode %v", row["mode"])
	}
	// 每一档仍然留在 price_units 里，控制台要把它们都显示出来。
	units, _ := row["price_units"].(map[string]any)
	if len(units) != 3 {
		t.Fatalf("price_units kept %d variants, want all 3", len(units))
	}
}

// TestPicturePricedInputIsNotATokenRate 证明按张计费的输入不能写进每 token 单价。
// 图像模型的键名里也有 input，单位却是 pic。当成 token 价会让回归看到六个数量级的错账。
func TestPicturePricedInputIsNotATokenRate(t *testing.T) {
	doc := feedOf(t, feedModelWith("seedream-like", []string{"image"}, map[string]any{
		"i_input_quantity":  unit(0.00289855, 1, "pic"),
		"i_output_quantity": unit(0.04347826, 1, "pic"),
	}))
	row := doc.Models["seedream-like"]
	if _, ok := row["input_cost_per_token"]; ok {
		t.Fatalf("a per-picture input became a token rate: %v", row["input_cost_per_token"])
	}
	if _, ok := row["output_cost_per_token"]; ok {
		t.Fatalf("a per-picture output became a token rate: %v", row["output_cost_per_token"])
	}
	if row["output_cost_per_image"] != 0.00289855 && row["output_cost_per_image"] != 0.04347826 {
		t.Fatalf("output_cost_per_image = %v", row["output_cost_per_image"])
	}
}

// TestVariantPricedImageModelGetsABillableRate 是图像那一半。
//
// ti_quantity / ii_quantity 这些键名不带 "cost"，只认通用字段的实现会把它们
// 全丢掉。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestVariantPricedImageModelGetsABillableRate(t *testing.T) {
	doc := feedOf(t, feedModelWith("variant-image", []string{"image"}, map[string]any{
		"ti_quantity": unit(0.0036, 1, "pic"),
		"ii_quantity": unit(0.0036, 1, "pic"),
	}))
	row := doc.Models["variant-image"]
	if _, ok := row["output_cost_per_image"]; !ok {
		t.Fatal("an image model priced per picture has no billable rate")
	}
	if row["output_cost_per_image"] != 0.0036 {
		t.Fatalf("output_cost_per_image = %v", row["output_cost_per_image"])
	}
}

// TestVariantPerOutputTokenIsBillable 覆盖只报"含/不含视频输入"两种价的模型。
//
// 它们的单位是 token（每 1000 个），键名里没有 input/output 这两个词。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestVariantPerOutputTokenIsBillable(t *testing.T) {
	doc := feedOf(t, feedModelWith("wiv-video", []string{"video"}, map[string]any{
		"wiv_v_output":  unit(0.0033, 1000, "token"),
		"woiv_v_output": unit(0.0056, 1000, "token"),
	}))
	row := doc.Models["wiv-video"]
	rate, ok := row["output_cost_per_token"]
	if !ok {
		t.Fatal("a model priced per output token has no billable rate")
	}
	// 每 1000 token 0.0033 美元，折成每 token 是 3.3e-6。
	if rate != 0.0000033 {
		t.Fatalf("output_cost_per_token = %v, want 3.3e-06", rate)
	}
}

// TestPeakOffPeakVariantsStillBill 覆盖按时间定价的模型。
//
// deepseek 报的是"空闲/高峰"两档，键名是 cache_offpeak、ncache_peak、
// output_peak 这些。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestPeakOffPeakVariantsStillBill(t *testing.T) {
	doc := feedOf(t, feedModelWith("peak-offpeak", []string{"text"}, map[string]any{
		"ncache_offpeak": unit(0.00000065, 1, "token"),
		"ncache_peak":    unit(0.0000013, 1, "token"),
		"output_offpeak": unit(0.00000195, 1, "token"),
		"output_peak":    unit(0.0000039, 1, "token"),
		"cache_offpeak":  unit(0.00000002, 1, "token"),
	}))
	row := doc.Models["peak-offpeak"]
	// 输入取便宜的那档，输出同理：这是"最便宜变体"的约定。
	if row["input_cost_per_token"] != 0.00000065 {
		t.Fatalf("input rate %v, want the off-peak variant", row["input_cost_per_token"])
	}
	if row["output_cost_per_token"] != 0.00000195 {
		t.Fatalf("output rate %v, want the off-peak variant", row["output_cost_per_token"])
	}
	// 缓存读也要接上：漏了它，一次带缓存的调用会按输入价扣。
	if _, ok := row["cache_read_input_token_cost"]; !ok {
		t.Fatal("cache_offpeak did not become a cache-read rate")
	}
}

// TestReasoningVariantsStillBill 覆盖"思考/非思考"两档（qwen 那一族）。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestReasoningVariantsStillBill(t *testing.T) {
	doc := feedOf(t, feedModelWith("thinking-chat", []string{"text"}, map[string]any{
		"nth_input":  unit(0.0000000435, 1, "token"),
		"nth_output": unit(0.000000087, 1, "token"),
		"th_input":   unit(0.0000000435, 1, "token"),
		"th_output":  unit(0.000000435, 1, "token"),
	}))
	row := doc.Models["thinking-chat"]
	if _, ok := row["input_cost_per_token"]; !ok {
		t.Fatal("nth_input did not become an input rate")
	}
	if _, ok := row["output_cost_per_token"]; !ok {
		t.Fatal("nth_output did not become an output rate")
	}
}

// TestModelWithNoPricingAtAllStaysUnpriced 钉住反面：市场根本没给价的模型，
// 不能编一个 0 出来。
//
// 0 和"未定价"是两件事。写成 0 会让运维以为这条模型免费，而实际是"我们不知道
// 多少钱"。控制台要给运维看到这个区别。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestModelWithNoPricingAtAllStaysUnpriced(t *testing.T) {
	doc := feedOf(t, feedModelWith("no-price", []string{"text"}, nil))
	row := doc.Models["no-price"]
	for _, field := range []string{
		"input_cost_per_token", "output_cost_per_token",
		"output_cost_per_second", "output_cost_per_image",
	} {
		if _, ok := row[field]; ok {
			t.Fatalf("a model the feed did not price got %s = %v", field, row[field])
		}
	}
}

// TestGenuineZeroIsKept 钉住一个例外：市场明说 0 的价是真价。
//
// 有些模型免费开放。把它当成"未定价"会让运维去手填一个不该填的数。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestGenuineZeroIsKept(t *testing.T) {
	doc := feedOf(t, feedModelWith("free-chat", []string{"text"}, map[string]any{
		"input": unit(0, 1, "token"),
	}))
	row := doc.Models["free-chat"]
	rate, ok := row["input_cost_per_token"]
	if !ok {
		t.Fatal("a model the feed priced at zero lost its rate")
	}
	if rate != float64(0) {
		t.Fatalf("input rate %v, want 0", rate)
	}
}

// TestPeakOffPeakRatesHaveWindowDimension 确认 peak/offpeak 的键各自带上了 window 维度，
// 而不是被合并成一个扁平字段。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestPeakOffPeakRatesHaveWindowDimension(t *testing.T) {
	doc := feedOf(t, feedModelWith("peak-offpeak2", []string{"text"}, map[string]any{
		"ncache_offpeak": unit(0.00000065, 1, "token"),
		"ncache_peak":    unit(0.0000013, 1, "token"),
		"output_offpeak": unit(0.00000195, 1, "token"),
		"output_peak":    unit(0.0000039, 1, "token"),
	}))
	row := doc.Models["peak-offpeak2"]
	rates, ok := row["rates"].([]Rate)
	if !ok {
		t.Fatal("rates field is missing or wrong type")
	}
	windows := map[string]map[string]bool{} // side → window → found
	for _, r := range rates {
		if windows[r.Side] == nil {
			windows[r.Side] = map[string]bool{}
		}
		windows[r.Side][r.Window] = true
	}
	if !windows["input"]["peak"] || !windows["input"]["offpeak"] {
		t.Fatalf("input side should have both peak and offpeak windows; got %v", windows["input"])
	}
	if !windows["output"]["peak"] || !windows["output"]["offpeak"] {
		t.Fatalf("output side should have both peak and offpeak windows; got %v", windows["output"])
	}
}

// TestPlainModelRatesAllWindow 非分时模型的每条 rate 的 window 都应该是 all。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestPlainModelRatesAllWindow(t *testing.T) {
	doc := feedOf(t, feedModelWith("plain-window", []string{"text"}, map[string]any{
		"input":  unit(0.000003, 1, "token"),
		"output": unit(0.000015, 1, "token"),
	}))
	row := doc.Models["plain-window"]
	rates, ok := row["rates"].([]Rate)
	if !ok {
		t.Fatal("rates field is missing or wrong type")
	}
	for _, r := range rates {
		if r.Window != "all" {
			t.Fatalf("non-windowed model rate %q has window=%q, want all", r.SourceKey, r.Window)
		}
	}
}

// TestEmptyFeedIsRefused 证明空的市场响应会被拒绝，而不是生成一份空目录。
//
// 一次失败的抓取如果能把在用的价格清空，账单就会在那段时间里全按零算。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestEmptyFeedIsRefused(t *testing.T) {
	if _, err := BuildPriceDocument([]byte(`{"status":true,"data":[]}`)); err == nil {
		t.Fatal("an empty feed produced a catalog")
	}
	if _, err := BuildPriceDocument([]byte(`not json`)); err == nil {
		t.Fatal("unreadable json produced a catalog")
	}
}
