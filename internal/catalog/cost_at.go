package catalog

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/sunqirui1987/xhub/internal/logx"
)

// Usage is what one call reports. The field names follow the OpenAI usage
// object, which is what the adapters pass through.
//
// A measure the model is not billed by stays zero, and zero quantities are
// skipped rather than billed at zero: a chat model reports no Seconds, a video
// model reports no tokens, and neither should be charged for the other.
type Usage struct {
	PromptTokens     int
	CompletionTokens int
	// CachedTokens is the prompt side that hit the cache. It selects the cached
	// rate for the input side when the model prices one.
	CachedTokens int
	// CacheWriteTokens is the prompt side written into the cache.
	CacheWriteTokens int
	// Images is the number of pictures generated.
	Images int
	// Seconds is the length of generated video or speech.
	Seconds float64
	// Searches is the number of search queries the call ran.
	Searches int
}

// Charge is what one call costs, priced at the instant it started.
//
// Total is the sum, and the three sides are broken out so the usage row and the
// log detail can show where the money went without re-deriving it.
type Charge struct {
	Total   float64
	Input   float64
	Output  float64
	Cache   float64
	Window  string
	Applied []AppliedRate
}

// AppliedRate is one rate the call actually used, with the quantity it was
// applied to. It is what price_snapshot stores, so a log row can be explained
// years later without consulting the price table of that day.
type AppliedRate struct {
	Measure   string  `json:"measure"`
	Side      string  `json:"side"`
	Variant   string  `json:"variant,omitempty"`
	UnitSize  float64 `json:"unit_size"`
	USD       float64 `json:"usd"`
	Quantity  float64 `json:"quantity"`
	SourceKey string  `json:"source_key,omitempty"`
}

// PriceSnapshot is the shape stored in usage_events.price_snapshot: which
// billing window the call fell in, and the rates it was actually charged at
// with the quantities they applied to.
//
// Keeping the window makes the row self-explanatory: "charged the peak rate"
// is a different statement from "charged double", and only the first one
// explains a bill.
type PriceSnapshot struct {
	Window  string        `json:"window"`
	Applied []AppliedRate `json:"applied"`
}

// windowOf returns the window a rate lookup should prefer for a call that
// started at t. A window-priced model has both peak and offpeak entries; a
// model that does not vary by time has only "all".
// 参数 t（time.Time）：这次调用的开始时刻。
// 返回 string（string）：peak 或 offpeak。
// 调用：CostAt。
// 测试：cost_at_test.go
func windowOf(t time.Time) string {
	if IsPeakHour(t) {
		return "peak"
	}
	return "offpeak"
}

// WindowAt reports which billing window a call starting at t falls in.
// Callers that price from their own rate table need the window on its own.
// 参数 t（time.Time）：这次调用的开始时刻。
// 返回 string（string）：peak 或 offpeak。
// 调用：gateway/deploymentCost。
// 测试：cost_at_test.go
func WindowAt(t time.Time) string { return windowOf(t) }

// DecodeRates reads a rate table from the shape it arrives in over the wire -
// either the embedded catalog's own structure or a deployment's JSON. One
// decoder for both keeps an operator-typed table and a generated one priced
// the same way.
//
// 参数 raw（any）：费率表，来自 JSON 解析或 []Rate。
// 返回 []Rate（[]Rate）：解出来的费率表；bool（bool）：解出至少一条时为真。
// 调用：gateway/deploymentRates。
// 测试：cost_at_test.go
func DecodeRates(raw any) ([]Rate, bool) {
	switch v := raw.(type) {
	case []Rate:
		return v, len(v) > 0
	case []any:
		out := make([]Rate, 0, len(v))
		for _, item := range v {
			if r, ok := decodeRate(item); ok {
				out = append(out, r)
			}
		}
		return out, len(out) > 0
	case []map[string]any:
		out := make([]Rate, 0, len(v))
		for _, item := range v {
			if r, ok := decodeRate(item); ok {
				out = append(out, r)
			}
		}
		return out, len(out) > 0
	default:
		return nil, false
	}
}

// CostFromRates prices a call against a rate table handed in by the caller
// rather than one looked up from the catalog. A deployment that typed its own
// rates goes through here, so both sources are billed by the same rules.
//
// 参数 rates（[]Rate）：要用来计费的费率表；usage（Usage）：这一次调用报出来的用量；startedAt（time.Time）：调用开始的时刻。
// 返回 Charge（Charge）：账单，含实际用到的费率；bool（bool）：至少有一侧被计费时为真。
// 调用：gateway/deploymentCost。
// 测试：cost_at_test.go
func CostFromRates(rates []Rate, usage Usage, startedAt time.Time) (Charge, bool) {
	if len(rates) == 0 {
		return Charge{}, false
	}
	return price(newLookup(rates), usage, windowOf(startedAt))
}

// Snapshot renders the rates a charge used as the JSON stored beside the usage
// row. A caller with no charge gets an empty string, which is how a usage row
// records that this call was never priced.
// 参数 c（Charge）：一次计费的账单。
// 返回 string（string）：可以直接写进 price_snapshot 的 JSON。没有用到任何费率时为空串。
// 调用：gateway/recordSpend。
// 测试：cost_at_test.go
func Snapshot(c Charge) string {
	if len(c.Applied) == 0 {
		return ""
	}
	raw, err := json.Marshal(PriceSnapshot{Window: c.Window, Applied: c.Applied})
	if err != nil {
		logx.Error("price snapshot is not encodable err=%v", err)
		return ""
	}
	return string(raw)
}

// rateLookup indexes one model's rates by the four dimensions, so a billed
// quantity can find its rate without scanning.
type rateLookup struct {
	byKey map[string]Rate
}

// rateKey is the lookup key for one rate. The variant is folded in because two
// rates can differ only by variant - cached and uncached on the same side.
// 参数 measure（string）：token、second、picture 或 query；side（string）：input 或 output；
// variant（string）：同一侧内部的限定词；window（string）：这次调用落在哪个时段。
// 返回 string（string）：四个维度拼成的查找键。
// 调用：newLookup 和 rateLookup.find。
// 测试：cost_at_test.go
func rateKey(measure, side, variant, window string) string {
	return measure + "\x00" + side + "\x00" + variant + "\x00" + window
}

// newLookup indexes a model's rates. When the same four dimensions appear twice
// the first one wins, which keeps pricing deterministic rather than dependent on
// map iteration order.
// 参数 rates（[]Rate）：价格行里的费率表。
// 返回 rateLookup（rateLookup）：按四个维度建好的索引。
// 调用：CostAt。
// 测试：cost_at_test.go
func newLookup(rates []Rate) rateLookup {
	l := rateLookup{byKey: make(map[string]Rate, len(rates))}
	for _, r := range rates {
		if r.Measure == "" {
			continue
		}
		key := rateKey(r.Measure, r.Side, r.Variant, r.Window)
		if _, exists := l.byKey[key]; !exists {
			l.byKey[key] = r
		}
	}
	return l
}

// find resolves one side of one measure.
//
// The preference chain is ordered by specificity, and it never picks by price.
// Picking the cheapest rate is what made a peak-hour call bill at the offpeak
// rate: both entries exist, and the cheaper one won. Here the window decides
// first, and only the *variant* degrades.
//
// 参数 measure（string）：token、second、picture 或 query；side（string）：input 或 output；
// variant（string）：调用事实带来的限定词，例如 cached。空串表示没有限定词；
// window（string）：这次调用落在哪个时段。
//
// 返回 Rate（Rate）：命中的费率；bool（bool）：命中时为真。
// 调用：CostAt。
// 测试：cost_at_test.go
func (l rateLookup) find(measure, side, variant, window string) (Rate, bool) {
	// A window-priced model is billed in the window the call landed in. A model
	// that is not window-priced carries "all" instead, so both spellings are
	// tried in that order - the call's own window first, then "all".
	windows := []string{window, "all"}
	// The variant from the call is the most specific, then the unqualified rate
	// on the same side. Never another variant: "thinking" is not a substitute
	// for "non_thinking".
	variants := []string{variant, ""}
	if variant == "" {
		variants = []string{""}
	}
	for _, w := range windows {
		for _, v := range variants {
			if r, ok := l.byKey[rateKey(measure, side, v, w)]; ok {
				return r, true
			}
		}
	}
	return Rate{}, false
}

// CostAt prices one call at the instant it started.
//
// The instant decides the window, and the call's own facts decide the variant.
// Both are needed: a cached prompt on a peak-hour call at a window-priced model
// is four different rates away from an uncached prompt on an offpeak one.
//
// It returns ok false when the model is unknown or when none of the quantities
// the call reported can be priced. A caller that gets false must record the call
// as unpriced - not as free - because the two are different facts.
//
// 参数 model（string）：对外模型名，用来选部署和记用量；usage（Usage）：这一次调用报出来的用量；
// startedAt（time.Time）：调用开始的时刻。时段只由它决定，不用结束时刻。
//
// 返回 Charge（Charge）：总价、各侧明细，以及这一次实际用到的费率；bool（bool）：真表示找到了可用结果。
// 调用：gateway/spend.go、gateway/usage/reports.go。
// 测试：cost_at_test.go
func CostAt(model string, usage Usage, startedAt time.Time) (Charge, bool) {
	logTraceOnceCost.Do(func() { logx.Trace("enter catalog.CostAt") })

	row, found := priceRowFor(model)
	if !found {
		return Charge{}, false
	}
	rates, ok := rateTableOf(row)
	if !ok || len(rates) == 0 {
		return Charge{}, false
	}
	return price(newLookup(rates), usage, windowOf(startedAt))
}

// price bills one usage against one indexed rate table. CostAt and CostFromRates
// differ only in where the table came from, so the arithmetic lives here once.
//
// 参数 lookup（rateLookup）：这条模型的费率索引；usage（Usage）：这一次调用报出来的用量；
// window（string）：这次调用落在哪个时段。
//
// 返回 Charge（Charge）：账单，含实际用到的费率；bool（bool）：至少有一侧被计费时为真。
// 调用：CostAt 和 CostFromRates。
// 测试：cost_at_test.go
func price(lookup rateLookup, usage Usage, window string) (Charge, bool) {
	var charge Charge
	charge.Window = window

	// The prompt side splits into cached and uncached. A cache read is a subset
	// of the prompt count, so each token is billed exactly once, at whichever
	// cache rate the model declares, with the remainder at the plain input rate.
	//
	// Two spellings appear in the feed and both are tried, most specific first:
	// a cached variant of the input side, or a cache_read side of its own
	// (deepseek spells it that way). Neither existing means the model does not
	// price the split at all, and the whole prompt is billed at the input rate.
	//
	// Billing the cached part twice, or not at all, are both wrong; only the
	// second is easy to notice, so the ordering here is explicit rather than
	// "apply every candidate that happens to match".
	cached := usage.CachedTokens
	if cached > usage.PromptTokens {
		// A provider reporting more hits than prompt tokens contradicts itself.
		// Trust the prompt count so the uncached remainder cannot go negative.
		cached = usage.PromptTokens
	}
	inputCandidates := [][3]string{{"token", "input", "uncached"}, {"token", "input", ""}}
	cacheCandidates := [][3]string{{"token", "input", "cached"}, {"token", "cache_read", ""}}

	cacheRate, pricesCache := findFirst(lookup, window, cacheCandidates...)
	if cached > 0 && pricesCache {
		chargeRate(&charge, cacheRate, float64(cached))
		chargeFirst(&charge, lookup, window, float64(usage.PromptTokens-cached), inputCandidates...)
	} else {
		// Either nothing was cached, or the model does not price a cache read.
		// In both cases the whole prompt is ordinary input.
		chargeFirst(&charge, lookup, window, float64(usage.PromptTokens), inputCandidates...)
	}

	chargeFirst(&charge, lookup, window, float64(usage.CompletionTokens),
		[3]string{"token", "output", ""})
	chargeFirst(&charge, lookup, window, float64(usage.CacheWriteTokens),
		[3]string{"token", "cache_write", ""})

	// Non-token measures. Pictures, seconds and queries have no input/output
	// split in the feed's common case, so the output spelling is tried first.
	chargeFirst(&charge, lookup, window, float64(usage.Images),
		[3]string{"picture", "output", ""}, [3]string{"picture", "input", ""})
	chargeFirst(&charge, lookup, window, usage.Seconds,
		[3]string{"second", "output", ""}, [3]string{"second", "input", ""})
	chargeFirst(&charge, lookup, window, float64(usage.Searches),
		[3]string{"query", "output", ""}, [3]string{"query", "input", ""})

	if len(charge.Applied) == 0 {
		return Charge{}, false
	}
	charge.Total = charge.Input + charge.Output + charge.Cache
	return charge, true
}

// findFirst resolves the first candidate that this model prices, in the order
// given. The order is the caller's statement of specificity, so a later
// candidate is never reached once an earlier one hits.
//
// 参数 lookup（rateLookup）：这条模型的费率索引；window（string）：这次调用落在哪个时段；
// candidates（...[3]string）：按优先顺序给出的 measure、side、variant。
//
// 返回 Rate（Rate）：命中的费率；bool（bool）：命中时为真。
// 调用：CostAt。
// 测试：cost_at_test.go
func findFirst(lookup rateLookup, window string, candidates ...[3]string) (Rate, bool) {
	for _, c := range candidates {
		if rate, hit := lookup.find(c[0], c[1], c[2], window); hit {
			return rate, true
		}
	}
	return Rate{}, false
}

// chargeFirst bills a quantity at the first candidate rate the model prices.
// A zero quantity is skipped, and a side the model does not price is left out
// rather than billed at zero.
//
// 参数 charge（*Charge）：累加中的账单；lookup（rateLookup）：这条模型的费率索引；
// window（string）：这次调用落在哪个时段；quantity（float64）：这一侧的数量；
// candidates（...[3]string）：按优先顺序给出的 measure、side、variant。
//
// 返回：无。命中时 charge 会累加金额并记下用到的费率。
// 调用：CostAt。
// 测试：cost_at_test.go
func chargeFirst(charge *Charge, lookup rateLookup, window string, quantity float64, candidates ...[3]string) {
	if quantity <= 0 {
		return
	}
	rate, hit := findFirst(lookup, window, candidates...)
	if !hit {
		return
	}
	chargeRate(charge, rate, quantity)
}

// chargeRate books one rate against one quantity and files it under the side it
// belongs to.
//
// The reported rate is stored with a normalized side. The feed spells a cache
// read two ways - a `cached` variant of the input side, or a `cache_read` side
// of its own - and the billing arithmetic treats them identically, so a stored
// snapshot that kept the raw spelling would describe the same event two
// different ways depending on which model it was.
//
// 参数 charge（*Charge）：累加中的账单；rate（Rate）：命中的费率；quantity（float64）：数量。
// 返回：无。charge 会累加金额并记下这条费率。
// 调用：CostAt 和 chargeFirst。
// 测试：cost_at_test.go
func chargeRate(charge *Charge, rate Rate, quantity float64) {
	amount := quantity * rate.USD
	side := rate.Side
	if side == "input" && rate.Variant == "cached" {
		// The cached variant of the input side is a cache read; only the
		// spelling differs.
		side = "cache_read"
	}
	switch side {
	case "input":
		charge.Input += amount
	case "cache_read", "cache_write":
		charge.Cache += amount
	default:
		charge.Output += amount
	}
	charge.Applied = append(charge.Applied, AppliedRate{
		Measure:   rate.Measure,
		Side:      side,
		Variant:   rate.Variant,
		UnitSize:  rate.UnitSize,
		USD:       rate.USD,
		Quantity:  quantity,
		SourceKey: rate.SourceKey,
	})
}

// rateTableOf reads the rates array off a price row. A row generated before
// rates existed carries only the flat fields, which cannot express a window, so
// it is reported as having no table rather than as a table that lost its window.
// 参数 row（map[string]any）：价格表里这一条模型的字段。
// 返回 []Rate（[]Rate）：费率表；bool（bool）：这一行带费率表时为真。
// 调用：CostAt。
// 测试：cost_at_test.go
func rateTableOf(row map[string]any) ([]Rate, bool) {
	switch v := row["rates"].(type) {
	case []Rate:
		return v, true
	case []any:
		out := make([]Rate, 0, len(v))
		for _, item := range v {
			if r, ok := decodeRate(item); ok {
				out = append(out, r)
			}
		}
		return out, len(out) > 0
	default:
		return nil, false
	}
}

// decodeRate reads one rate out of the structure that arrives from JSON. The
// embedded catalog is parsed from bytes, so the numbers come back as float64
// rather than as the Rate type.
// 参数 item（any）：一行费率，来自 JSON 解析。
// 返回 Rate（Rate）：解出来的费率；bool（bool）：字段齐备时为真。
// 调用：rateTableOf。
// 测试：cost_at_test.go
func decodeRate(item any) (Rate, bool) {
	block, ok := item.(map[string]any)
	if !ok {
		return Rate{}, false
	}
	measure := stringField(block, "measure")
	if measure == "" {
		return Rate{}, false
	}
	usd, ok := floatField(block["usd"])
	if !ok {
		return Rate{}, false
	}
	size, ok := floatField(block["unit_size"])
	if !ok || size <= 0 {
		size = 1
	}
	return Rate{
		Measure:   measure,
		UnitSize:  size,
		Side:      stringField(block, "side"),
		Variant:   stringField(block, "variant"),
		Window:    stringField(block, "window"),
		SourceKey: stringField(block, "source_key"),
		Label:     stringField(block, "label"),
		USD:       usd,
	}, true
}

// priceRowFor finds the price row for a model name, trying the same three
// spellings TokenRates does: as stored, without the provider prefix, then the
// alias the feed declares.
// 参数 model（string）：对外模型名。
// 返回 map[string]any（map[string]any）：这一条模型的价格行；bool（bool）：找到时为真。
// 调用：CostAt。
// 测试：cost_at_test.go
func priceRowFor(model string) (map[string]any, bool) {
	modelCostMu.RLock()
	defer modelCostMu.RUnlock()
	raw, isMap := modelCostMapValue.(map[string]any)
	if !isMap {
		return nil, false
	}
	model = strings.TrimSpace(model)
	if model == "" {
		return nil, false
	}
	for _, key := range priceKeysForLocked(model) {
		if row, ok := priceRow(raw, key); ok {
			return row, true
		}
	}
	return nil, false
}

// priceKeysForLocked lists the keys to try for one model name, most specific
// first. The caller must already hold at least a read lock.
// 参数 model（string）：对外模型名。
// 返回 []string（[]string）：按优先顺序排列的候选键。
// 调用：priceRowFor。
// 测试：无直接单测
func priceKeysForLocked(model string) []string {
	keys := []string{model}
	if i := strings.Index(model, "/"); i > 0 {
		keys = append(keys, model[i+1:])
	}
	if id, found := aliasKeyLocked(model); found {
		keys = append(keys, id)
	}
	return keys
}
