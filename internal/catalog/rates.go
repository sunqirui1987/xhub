package catalog

import (
	"strings"

	"github.com/sunqirui1987/xhub/internal/logx"
)

// Rate is one entry in a model's rate table.
//
// Four dimensions capture everything the configured price feed can express:
//
//   - Measure: how the quantity is counted — token, second, picture, query.
//   - Side: which side of the call this price applies to — input, output,
//     cache_read, cache_write, batch_input, batch_output.
//   - Variant: a qualifier within one side, e.g. uncached vs cached, 480p vs
//     1080p, thinking vs non_thinking. Empty string means the unqualified form.
//   - Window: whether the price depends on time of day — peak, offpeak, or all
//     (time-independent). Only models with peak/offpeak entries require the
//     IsPeakHour check at billing time.
//
// UnitSize is the number of base units one USD price covers (1000 for tokens,
// 1 for pictures and seconds). USD is the price for one base unit, already
// divided by UnitSize.
type Rate struct {
	Measure   string  `json:"measure"`
	UnitSize  float64 `json:"unit_size"`
	Side      string  `json:"side"`
	Variant   string  `json:"variant,omitempty"`
	Window    string  `json:"window"` // peak | offpeak | all
	SourceKey string  `json:"source_key"`
	Label     string  `json:"label,omitempty"`
	USD       float64 `json:"usd"`
	// Fallback marks a rate the lookup had to choose for the model rather than
	// read for the call, because the side is quoted only in variants. It is set
	// by rateLookup, never by the feed, and it travels into the price snapshot so
	// a bill built on a chosen variant can be told from one built on a fact.
	Fallback bool `json:"fallback,omitempty"`
}

// rateSpec is the static classification of a known feed key.
type rateSpec struct {
	side    string
	variant string
	window  string
}

// knownKeys maps every feed key that has a stable semantic meaning to its
// classification. Keys not in this table are handled by inferRateSpec.
var knownKeys = map[string]rateSpec{
	// Plain token sides
	"input":  {"input", "uncached", "all"},
	"ncache": {"input", "uncached", "all"},
	"output": {"output", "", "all"},

	// Cache
	"cache":      {"cache_read", "", "all"},
	"c_cache":    {"cache_write", "", "all"},
	"c_1h_cache": {"cache_write", "1h", "all"},

	// Batch
	"bi_input":  {"batch_input", "", "all"},
	"bi_output": {"batch_output", "", "all"},

	// DeepSeek / generic peak+offpeak
	"ncache_offpeak": {"input", "uncached", "offpeak"},
	"ncache_peak":    {"input", "uncached", "peak"},
	"output_offpeak": {"output", "", "offpeak"},
	"output_peak":    {"output", "", "peak"},
	"cache_offpeak":  {"cache_read", "", "offpeak"},
	"cache_peak":     {"cache_read", "", "peak"},

	// Qwen thinking / non-thinking
	"nth_input":  {"input", "non_thinking", "all"},
	"th_input":   {"input", "thinking", "all"},
	"nth_output": {"output", "non_thinking", "all"},
	"th_output":  {"output", "thinking", "all"},

	// GPT-image text vs image input
	"t_input":  {"input", "text", "all"},
	"i_input":  {"input", "image", "all"},
	"t_output": {"output", "text", "all"},
	"i_output": {"output", "image", "all"},

	// Vidu wiv / woiv (with/without video input)
	"wiv_v_output":  {"output", "wiv", "all"},
	"woiv_v_output": {"output", "woiv", "all"},

	// Web search
	"web_search_req": {"output", "search", "all"},

	// Picture quantities (image models)
	"ti_quantity":                  {"output", "ti", "all"},
	"ii_quantity":                  {"output", "ii", "all"},
	"mi2i_quantity":                {"output", "mi2i", "all"},
	"omi_quantity":                 {"output", "omi", "all"},
	"i_input_quantity":             {"input", "", "all"},
	"i_output_quantity":            {"output", "", "all"},
	"1_5k_i_output_quantity":       {"output", "1_5k", "all"},
	"layer_i_output_quantity":      {"output", "layer", "all"},
	"1_5k_layer_i_output_quantity": {"output", "1_5k_layer", "all"},

	// Plain video duration
	"av_duration": {"output", "av", "all"},
	"v_duration":  {"output", "v", "all"},
}

// buildRates converts one model's details_v2 map into the Rate slice.
//
// Every key in the feed becomes a Rate. Known keys get a precise
// classification; unknown keys fall back to inferRateSpec, which derives
// side/variant/window from the key name by substring matching.
//
// 参数 details（map[string]feedUnit）：这条模型的 details_v2 计价块。
// 返回 []Rate（[]Rate）：按 source_key 排好序的费率表。没有计价块时为 nil。
// 调用：convertFeedModel。
// 测试：feed_test.go
func buildRates(details map[string]feedUnit) []Rate {
	if len(details) == 0 {
		return nil
	}
	keys := make([]string, 0, len(details))
	for k := range details {
		keys = append(keys, k)
	}
	// Stable order for deterministic JSON output.
	sortStrings(keys)

	out := make([]Rate, 0, len(details))
	for _, key := range keys {
		unit := details[key]
		usd, ok := perUnit(unit)
		if !ok {
			continue
		}
		spec, known := knownKeys[key]
		if !known {
			// The feed added a price this table has never seen. The guess below
			// still produces a rate, but it can put the wrong number on the
			// wrong side, so the key is named in the log for someone to classify.
			logx.Debug("market feed priced an unclassified key key=%s unit=%s", key, unit.UnitName)
			spec = inferRateSpec(key)
		}
		size := unit.UnitSize
		if size <= 0 {
			size = 1
		}
		out = append(out, Rate{
			Measure:   measureOf(unit),
			UnitSize:  size,
			Side:      spec.side,
			Variant:   spec.variant,
			Window:    spec.window,
			SourceKey: key,
			Label:     strings.TrimSpace(unit.Name),
			USD:       usd,
		})
	}
	return out
}

// measureOf maps the unit_name in the feed to the canonical measure name.
// 参数 unit（feedUnit）：市场的一段计价单位。
// 返回 string（string）：token、second、picture 或 query。
// 调用：buildRates。
// 测试：无直接单测
func measureOf(unit feedUnit) string {
	switch strings.ToLower(strings.TrimSpace(unit.UnitName)) {
	case "token":
		return "token"
	case "second", "time":
		return "second"
	case "pic":
		return "picture"
	default:
		return "query"
	}
}

// inferRateSpec derives side/variant/window for keys not in the knownKeys table.
//
// Rules applied in order:
//  1. Window: key contains "_peak" → peak; "_offpeak" → offpeak; else all.
//     "_offpeak" is checked first because it contains "_peak" as a substring.
//  2. Side: key contains "output" → output; "cache" → cache_read;
//     "input" or "ncache" → input; else output (most video/image keys describe
//     output quantities).
//  3. Variant: key with the window suffix stripped, with the side word stripped,
//     cleaned of leading/trailing underscores.
//
// 参数 key（string）：市场计价块里不在已知键名表里的键。
// 返回 rateSpec（rateSpec）：推断出来的分类。
// 调用：buildRates。
// 测试：无直接单测
func inferRateSpec(key string) rateSpec {
	window := "all"
	working := key
	if strings.Contains(working, "_offpeak") {
		window = "offpeak"
		working = strings.ReplaceAll(working, "_offpeak", "")
	} else if strings.Contains(working, "_peak") {
		window = "peak"
		working = strings.ReplaceAll(working, "_peak", "")
	}

	side := "output"
	switch {
	case strings.Contains(working, "cache"):
		side = "cache_read"
		working = strings.ReplaceAll(working, "cache", "")
	case strings.Contains(working, "input") || strings.Contains(working, "ncache"):
		side = "input"
		working = strings.ReplaceAll(working, "input", "")
		working = strings.ReplaceAll(working, "ncache", "")
	case strings.Contains(working, "output"):
		side = "output"
		working = strings.ReplaceAll(working, "output", "")
	}

	variant := strings.Trim(working, "_")
	return rateSpec{side: side, variant: variant, window: window}
}

// sortStrings sorts a string slice in place. Defined here to avoid an extra
// import since the catalog package already imports sort elsewhere.
// 参数 ss（[]string）：待排序的字符串切片，原地修改。
// 返回：无。
// 调用：buildRates。
// 测试：无直接单测
func sortStrings(ss []string) {
	// insertion sort is fine for the typical ~20 keys per model
	for i := 1; i < len(ss); i++ {
		for j := i; j > 0 && ss[j] < ss[j-1]; j-- {
			ss[j], ss[j-1] = ss[j-1], ss[j]
		}
	}
}
