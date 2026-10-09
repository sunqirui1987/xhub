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
	// OutputVariant is measured by the provider.
	OutputVariant string
	ImageVariant  string
	PricingModel  string
	// PricingBlocked preserves measurements without presenting incomplete rates as a bill.
	PricingBlocked string
	InputSeconds   float64
	InputImages    int
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
	// Fallback marks a rate that was chosen for the model rather than for the
	// call. Some models quote a side only in qualified variants - per second of
	// 1080p versus 4K video, per picture made from text versus from an image -
	// and the gateway is not told which one applies. Billing the cheapest is a
	// decision, not a measurement, so the row says so instead of presenting a
	// guess as the price of the call.
	Fallback bool `json:"fallback,omitempty"`
}

// PriceSnapshot is the shape stored in usage_events.price_snapshot: which
// billing window the call fell in, and the rates it was actually charged at
// with the quantities they applied to.
//
// Keeping the window makes the row self-explanatory: "charged the peak rate"
// is a different statement from "charged double", and only the first one
// explains a bill.
type PriceSnapshot struct {
	Window        string        `json:"window"`
	Applied       []AppliedRate `json:"applied"`
	PricingStatus string        `json:"pricing_status,omitempty"`
	Usage         *Usage        `json:"usage,omitempty"`
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

// flatRateFields lists the flat price fields a deployment or an older price row
// may carry, and the rate each one describes.
//
// The field names are LiteLLM's, which is the form an operator types and the
// form the generated rows keep for readers that predate rates[]. They are
// translated here so a deployment that typed its prices is billed by the same
// rules as one the catalog priced - including the peak ladder and the non-token
// measures, neither of which the old flat arithmetic understood.
//
// The window on each entry is the window the field describes:
//
//   - input_cost_per_token and output_cost_per_token are the base rate. They are
//     the off-peak price on a model that also quotes a peak one, which is what
//     the catalog's own flat fields hold and what the console labels them.
//   - the _peak fields are the peak price.
//
// measureAllWindows is applied afterwards: a deployment that quotes no peak
// price meant one price for every hour, so its base rate is promoted to "all".
var flatRateFields = []struct {
	field   string
	measure string
	side    string
	variant string
	window  string
}{
	{"input_cost_per_token", "token", "input", "uncached", "offpeak"},
	{"input_cost_per_token_peak", "token", "input", "uncached", "peak"},
	{"output_cost_per_token", "token", "output", "", "offpeak"},
	{"output_cost_per_token_peak", "token", "output", "", "peak"},
	{"cache_read_input_token_cost", "token", "cache_read", "", "all"},
	{"cache_creation_input_token_cost", "token", "cache_write", "", "all"},
	{"input_cost_per_image", "picture", "input", "", "all"},
	{"output_cost_per_image", "picture", "output", "", "all"},
	{"input_cost_per_second", "second", "input", "", "all"},
	{"output_cost_per_second", "second", "output", "", "all"},
	{"search_context_cost_per_query", "query", "output", "", "all"},
}

// RatesFromFlat builds a rate table from the flat price fields a deployment or
// an older price row carries.
//
// The flat form cannot say what it does not have a field for - a picture price
// per resolution, an input price per modality, a second of 4K video against a
// second of 1080p - but the four fields it does have must be read by the same
// biller as everything else. Routing them through here rather than through a
// second arithmetic path is what keeps a peak rate, a cache read and a per-image
// price from being dropped on the way to the usage row.
//
// read is called once per known field and returns ok false for one the source
// does not carry; a field present as zero is a real price of zero.
//
// 参数 read（func(string) (float64, bool)）：按字段名读一个小数，第二个返回值为假表示这一项没有。
// 返回 []Rate（[]Rate）：拼出来的费率表。一个字段都没有时为空。
// 调用：gateway/deploymentCost、catalog/CostAt 的旧行兜底。
// 测试：cost_at_test.go
func RatesFromFlat(read func(string) (float64, bool)) []Rate {
	out := make([]Rate, 0, len(flatRateFields))
	hasPeak := map[string]bool{}
	for _, f := range flatRateFields {
		if _, ok := read(f.field); ok && f.window == "peak" {
			hasPeak[f.measure+"\x00"+f.side] = true
		}
	}
	for _, f := range flatRateFields {
		usd, ok := read(f.field)
		if !ok {
			continue
		}
		window := f.window
		// A base rate on a side that quotes no peak price applies at every hour.
		// Leaving it as offpeak would make a peak call on such a deployment
		// unpriced, which is a worse answer than the one price the operator gave.
		if window == "offpeak" && !hasPeak[f.measure+"\x00"+f.side] {
			window = "all"
		}
		out = append(out, Rate{
			Measure:   f.measure,
			UnitSize:  1,
			Side:      f.side,
			Variant:   f.variant,
			Window:    window,
			SourceKey: f.field,
			USD:       usd,
		})
	}
	return out
}

// CostFromFlatOrRates prices a call against flat price fields, read through the
// callback, or reports that the source carries no price at all.
//
// It exists so a caller holding the flat form does not have to build a table and
// remember to keep the two paths in step.
// 参数 read（func(string) (float64, bool)）：按字段名读一个小数；usage（Usage）：这一次调用报出来的用量；
// startedAt（time.Time）：调用开始的时刻。
// 返回 Charge（Charge）：账单；bool（bool）：读到了价、并且有某一侧被计费时为真。
// 调用：gateway/deploymentCost。
// 测试：cost_at_test.go
func CostFromFlatOrRates(read func(string) (float64, bool), usage Usage, startedAt time.Time) (Charge, bool) {
	return CostFromRates(RatesFromFlat(read), usage, startedAt)
}

// NormalizeUsage reads the quantities the gateway bills on out of the usage
// object an upstream returned, resolving the field names providers spell
// differently into one set of counts.
//
// It exists because the providers do not agree on what "input tokens" contains,
// and the disagreement runs through several field names at once.
//
//	OpenAI      prompt_tokens is the whole prompt; the cached part is repeated
//	            under prompt_tokens_details.cached_tokens as a subset of it.
//	Anthropic   input_tokens is *only* the part that missed the cache, and
//	            cache_read_input_tokens is a separate count beside it.
//	Responses   input_tokens is the whole prompt, with the cached part nested
//	            under input_tokens_details the way OpenAI spells it.
//
// So the discriminator is *where the cache count lives*, not the field name of
// the prompt count: a cache count in its own top-level field is a second
// quantity that must be added back, and one nested under the prompt count's
// details is a subset that must not be. Reading the Anthropic shape with the
// OpenAI rule loses the whole cache read and bills the rest at the cache price,
// which is roughly half the money on a cached call.
//
// Every shape comes out with prompt tokens being the whole prompt and cache
// reads being a subset of it - the one reading the cache split has to have to
// bill each token exactly once.
//
// The gateway's own response normalization copies input_tokens onto prompt_tokens
// before billing sees it, so this cannot rely on the prompt field being absent:
// what marks the Anthropic shape here is the separate cache count, and adding it
// when it exceeds the prompt count is what recovers the lost half.
//
// A count the upstream did not report stays zero, and an absent key stays absent
// rather than becoming a zero: "this model reported no images" and "this model
// reported zero images" are the same for billing, but the first is also what a
// text model says, and nothing downstream should read it as a picture count.
//
// 参数 usage（map[string]any）：上游返回的 usage 对象。
// 返回 Usage（Usage）：可以交给 CostAt 的用量。上游没报的量为零。
// 调用：gateway/usageOf。
// 测试：cost_at_test.go
func NormalizeUsage(usage map[string]any) Usage {
	var out Usage
	if usage == nil {
		return out
	}
	out.PromptTokens = firstIntField(usage, "prompt_tokens", "input_tokens")
	out.CompletionTokens = firstIntField(usage, "completion_tokens", "output_tokens")
	out.OutputVariant, _ = usage["output_variant"].(string)
	out.ImageVariant, _ = usage["image_variant"].(string)
	out.PricingModel, _ = usage["pricing_model"].(string)
	out.PricingBlocked, _ = usage["pricing_blocked"].(string)
	out.InputSeconds = firstFloatField(usage, "input_seconds")
	out.InputImages = firstIntField(usage, "input_image_count")

	// A cache read in its own top-level field is a second count, so the prompt
	// count beside it excludes it and both have to be added up. The nested
	// spellings are subsets and are read as such below.
	separate := firstIntField(usage, "cache_read_input_tokens", "cache_read_tokens")
	_, hasSeparate := usage["cache_read_input_tokens"]
	if usage["cache_read_input_tokens"] == nil {
		hasSeparate = usage["cache_read_tokens"] != nil
	}
	if hasSeparate {
		out.CachedTokens = separate
		out.PromptTokens += separate
	} else {
		out.CachedTokens = cachedTokensIn(usage)
	}
	out.CacheWriteTokens = firstIntField(usage, "cache_creation_input_tokens", "cache_write_tokens")
	out.Images = firstIntField(usage, "images", "image_count", "num_images", "output_images")
	out.Seconds = firstFloatField(usage, "seconds", "duration_seconds", "video_seconds", "audio_seconds")
	out.Searches = firstIntField(usage, "searches", "search_count", "web_search_requests")
	return out
}

// cachedTokensIn reads the cached prompt count from the nested places providers
// put it. These are subsets of the prompt count, never additions to it.
// 参数 usage（map[string]any）：上游返回的 usage 对象。
// 返回 int（int）：命中缓存的提示 token 数。没有报这个数时为 0。
// 调用：NormalizeUsage。
// 测试：cost_at_test.go
func cachedTokensIn(usage map[string]any) int {
	if usage["cached_tokens"] != nil {
		return intField(usage, "cached_tokens")
	}
	for _, key := range []string{"prompt_tokens_details", "input_tokens_details"} {
		if details, ok := usage[key].(map[string]any); ok {
			if details["cached_tokens"] != nil {
				return intField(details, "cached_tokens")
			}
		}
	}
	return 0
}

// intField reads a whole number by key, reading 0 for a key that is absent or is
// not a number.
// 参数 m（map[string]any）：要读的对象；key（string）：字段名。
// 返回 int（int）：读到的整数。缺失或类型不符时为 0。
// 调用：NormalizeUsage 和 cachedTokensIn。
// 测试：cost_at_test.go
func intField(m map[string]any, key string) int {
	if m == nil {
		return 0
	}
	switch n := m[key].(type) {
	case float64:
		return int(n)
	case float32:
		return int(n)
	case int:
		return n
	case int64:
		return int(n)
	case json.Number:
		if v, err := n.Int64(); err == nil {
			return int(v)
		}
	}
	return 0
}

// firstIntField reads a whole number from the first of several spellings the
// object carries with a non-nil value.
// 参数 m（map[string]any）：要读的对象；keys（...string）：按优先顺序给出的字段名。
// 返回 int（int）：第一个有值的键对应的整数。一个都没有时为 0。
// 调用：NormalizeUsage。
// 测试：cost_at_test.go
func firstIntField(m map[string]any, keys ...string) int {
	for _, key := range keys {
		if v, ok := m[key]; ok && v != nil {
			return intField(m, key)
		}
	}
	return 0
}

// firstFloatField reads a fraction from the first of several spellings the object
// carries with a non-nil value. Durations are fractional - an upstream reports
// 8.5 seconds - so they are not read through the integer path.
// 参数 m（map[string]any）：要读的对象；keys（...string）：按优先顺序给出的字段名。
// 返回 float64（float64）：第一个有值的键对应的小数。一个都没有时为 0。
// 调用：NormalizeUsage。
// 测试：cost_at_test.go
func firstFloatField(m map[string]any, keys ...string) float64 {
	for _, key := range keys {
		v, ok := m[key]
		if !ok || v == nil {
			continue
		}
		switch n := v.(type) {
		case float64:
			return n
		case float32:
			return float64(n)
		case int:
			return float64(n)
		case int64:
			return float64(n)
		case json.Number:
			if f, err := n.Float64(); err == nil {
				return f
			}
		}
	}
	return 0
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
//
// byKey answers "what did this model quote for exactly this combination".
// fallback answers "this model quoted only qualified variants of this side, so
// what should a call that was not told which variant apply". The two are kept
// apart so an exact hit is never displaced by the fallback.
type rateLookup struct {
	byKey map[string]Rate
	// fallback is keyed by (measure, side, window) and holds the variant-chosen
	// rate for a side that has no unqualified entry at all.
	fallback map[string]Rate
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

// sideKey is the lookup key for the variant-free index of one side in one
// window: everything except the qualifier.
// 参数 measure（string）：token、second、picture 或 query；side（string）：input 或 output；
// window（string）：这次调用落在哪个时段。
// 返回 string（string）：三个维度拼成的查找键。
// 调用：newLookup 和 rateLookup.find。
// 测试：cost_at_test.go
func sideKey(measure, side, window string) string {
	return measure + "\x00" + side + "\x00" + window
}

// searchVariant is the qualifier the feed puts on the per-query price, under a
// unit it calls a second. It names a count of searches, not a length of media.
const searchVariant = "search"

// newLookup indexes a model's rates. When the same four dimensions appear twice
// the first one wins, which keeps pricing deterministic rather than dependent on
// map iteration order.
//
// Two normalizations happen here, both because the feed's spelling and the
// gateway's billable quantity are not one-to-one:
//
//   - A per-query price arrives spelled as a second (web_search_req has
//     unit_name "second" and a "search" qualifier). It is indexed under the
//     query measure too, so a call that reported searches is billed for them
//     rather than for a duration it never reported.
//   - A side quoted only in variants gets a fallback rate, chosen once here
//     rather than per lookup, so the same model always bills the same way. The
//     cheapest variant wins, and ties break on the key name; see cheapestFallback.
//
// 参数 rates（[]Rate）：价格行里的费率表。
// 返回 rateLookup（rateLookup）：按四个维度和按侧建好的索引。
// 调用：CostAt 和 CostFromRates。
// 测试：cost_at_test.go
func newLookup(rates []Rate) rateLookup {
	l := rateLookup{
		byKey:    make(map[string]Rate, len(rates)),
		fallback: map[string]Rate{},
	}
	variants := map[string]map[string]Rate{}
	for _, r := range rates {
		if r.Measure == "" {
			continue
		}
		// A search price is a query count. File it under both spellings so the
		// call's own quantity decides which one is read.
		if r.Measure == "second" && r.Variant == searchVariant {
			asQuery := r
			asQuery.Measure = "query"
			asQuery.Variant = ""
			asQuery.SourceKey = r.SourceKey
			if key := rateKey("query", r.Side, "", r.Window); l.byKey[key].Measure == "" {
				l.byKey[key] = asQuery
			}
		}
		key := rateKey(r.Measure, r.Side, r.Variant, r.Window)
		if _, exists := l.byKey[key]; !exists {
			l.byKey[key] = r
		}
		if r.Variant == "" {
			continue
		}
		// Only a duration may be billed as a duration. The search qualifier is
		// excluded here as well as being indexed as a query above, so a model
		// that quotes searches is not charged for them twice under two measures.
		if r.Measure == "second" && r.Variant == searchVariant {
			continue
		}
		side := sideKey(r.Measure, r.Side, r.Window)
		group := variants[side]
		if group == nil {
			group = map[string]Rate{}
			variants[side] = group
		}
		if _, exists := group[r.Variant]; !exists {
			group[r.Variant] = r
		}
	}
	for side, group := range variants {
		if _, exact := l.byKey[side+"\x00"]; exact {
			// The side has an unqualified price. Qualified ones are variants of
			// it, not substitutes for it, so there is nothing to fall back to.
			continue
		}
		if r, ok := cheapestFallback(group); ok {
			l.fallback[side] = r
		}
	}
	return l
}

// cheapestFallback picks the rate to use for a side the model quoted only in
// variants.
//
// Choosing is unavoidable: the gateway is not told whether a video came out at
// 1080p or 4K, or whether a picture was made from text or from another picture.
// The cheapest variant is the deliberate choice, matching the flat fallback
// fields the generator already writes, because charging for a variant the caller
// did not ask for is worse than charging for the cheapest one. The choice is
// recorded with Fallback set, so a bill built on it can be told apart from one
// built on the price of the call.
//
// 参数 group（map[string]Rate）：同一侧同一时段下的各个变体费率，键是变体名。
// 返回 Rate（Rate）：选中的费率；bool（bool）：这一组非空时为真。
// 调用：newLookup。
// 测试：cost_at_test.go
func cheapestFallback(group map[string]Rate) (Rate, bool) {
	names := make([]string, 0, len(group))
	for name := range group {
		names = append(names, name)
	}
	// Deterministic order, so two runs over the same table cannot disagree.
	sortStrings(names)
	var best Rate
	found := false
	for _, name := range names {
		r := group[name]
		if !found || r.USD < best.USD {
			best = r
			found = true
		}
	}
	if found {
		best.Variant = ""
		best.Fallback = true
	}
	return best, found
}

// find resolves one side of one measure.
//
// The preference chain is ordered by specificity, and it never picks by price
// across windows. Picking the cheapest rate is what made a peak-hour call bill
// at the offpeak rate: both entries exist, and the cheaper one won. Here the
// window decides first, and only the *variant* degrades.
//
// What the variant may degrade to is the unqualified rate for the same side, and
// - when the side has none - the single variant chosen for that side. It never
// degrades to a *different* variant: "uncached" is not a reading of "cached", and
// a cache read billed at the input price is off by two orders of magnitude. That
// is why the fallback is offered only on the probe that named no variant. A probe
// that asked for "cached" is asking a question about that variant, and the answer
// to it is "this model does not price it", not the side's generic rate.
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
	for _, w := range windows {
		if variant != "" {
			if r, ok := l.byKey[rateKey(measure, side, variant, w)]; ok {
				return r, true
			}
		}
		if r, ok := l.byKey[rateKey(measure, side, "", w)]; ok {
			return r, true
		}
		if variant != "" {
			// This probe named a variant and the model does not price it. The
			// next candidate in the caller's chain is the unqualified spelling
			// of the side, which is where the fallback belongs.
			continue
		}
		// The call named no variant and the side has no unqualified price: the
		// model quotes this side only in variants. Take the chosen one rather
		// than reporting the side as unpriced, which would record the call as
		// costing nothing. The rate comes back marked, so the guess is visible.
		if r, ok := l.fallback[sideKey(measure, side, w)]; ok {
			return r, true
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
	if rates, ok := rateTableOf(row); ok && len(rates) > 0 {
		return price(newLookup(rates), usage, windowOf(startedAt))
	}
	// A row with no rate table predates rates[]. Its flat fields carry one price
	// per side and cannot express a window, so the price they hold is the base
	// one - the same value the console shows as the off-peak rate. Billing it at
	// every hour is the honest reading of a row that never declared a peak; the
	// alternative, reporting the model as unpriced, turns a documented price into
	// a row of zero spend.
	return CostFromFlatOrRates(func(field string) (float64, bool) {
		return floatField(row[field])
	}, usage, startedAt)
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
	if usage.PricingBlocked != "" {
		return Charge{}, false
	}
	if usage.Images > 0 && usage.ImageVariant != "" {
		if _, ok := lookup.find("picture", "output", usage.ImageVariant, window); !ok {
			return Charge{}, false
		}
	}
	if usage.Seconds > 0 && usage.OutputVariant != "" {
		if _, ok := lookup.find("second", "output", usage.OutputVariant, window); !ok {
			return Charge{}, false
		}
	}
	// A measured band must be priceable; a different band's minimum is not evidence.
	if usage.CompletionTokens > 0 && usage.OutputVariant != "" {
		if _, ok := lookup.find("token", "output", usage.OutputVariant, window); !ok {
			return Charge{}, false
		}
	}
	if usage.OutputVariant != "" && usage.Searches > 0 {
		if _, ok := findFirst(lookup, window, [3]string{"query", "output", ""}, [3]string{"query", "input", ""}); !ok {
			return Charge{}, false
		}
	}
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
		[3]string{"token", "output", usage.OutputVariant})
	chargeFirst(&charge, lookup, window, float64(usage.CacheWriteTokens),
		[3]string{"token", "cache_write", ""})

	// Non-token measures. Pictures, seconds and queries have no input/output
	// split in the feed's common case, so the output spelling is tried first.
	if usage.ImageVariant != "" {
		chargeFirst(&charge, lookup, window, float64(usage.Images), [3]string{"picture", "output", usage.ImageVariant})
	} else {
		chargeFirst(&charge, lookup, window, float64(usage.Images), [3]string{"picture", "output", ""}, [3]string{"picture", "input", ""})
	}
	if usage.OutputVariant != "" {
		chargeFirst(&charge, lookup, window, usage.Seconds, [3]string{"second", "output", usage.OutputVariant})
	} else {
		chargeFirst(&charge, lookup, window, usage.Seconds, [3]string{"second", "output", ""}, [3]string{"second", "input", ""})
	}
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
		Fallback:  rate.Fallback,
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
	return priceRowForLocked(model)
}

// priceRowForLocked 是 priceRowFor 的已持锁版本。TokenRates 自己拿着读锁，
// 再调 priceRowFor 会死锁。
// 参数 model（string）：对外模型名。
// 返回 map[string]any（map[string]any）：这一条模型的价格行；bool（bool）：找到时为真。
// 调用：priceRowFor、TokenRates。
// 测试：cost_at_test.go
func priceRowForLocked(model string) (map[string]any, bool) {
	raw, isMap := modelCostMapValue.(map[string]any)
	if !isMap {
		return nil, false
	}
	model = strings.TrimSpace(model)
	if model == "" {
		return nil, false
	}
	var shell map[string]any
	for _, key := range priceKeysForLocked(model) {
		row, ok := priceRow(raw, key)
		if !ok {
			continue
		}
		// 没有费率的空壳不能赢。供应商登记时常把「网关名 → 官方 id」写成一条
		// 没有单价的行，它和价目表里那条真正报价的行同名不同键。先撞上的空壳
		// 会让后面的报价变成看不见，调用就被记成没有价格。
		if bound, _ := row["price_binding"].(bool); bound {
			return row, true
		}
		if rowHasPrice(row) {
			return row, true
		}
		if shell == nil {
			shell = row
		}
	}
	if shell != nil {
		return shell, true
	}
	return nil, false
}

// rowHasPrice 报告这一行带了任何可计费的价。费率为空、扁平字段也一个都没有，
// 就是一条还没报价的壳。
// 参数 row（map[string]any）：价格表里这一条模型的字段。
// 返回 bool（bool）：有费率表或任一扁平单价时为真。
// 调用：priceRowFor。
// 测试：cost_at_test.go
func rowHasPrice(row map[string]any) bool {
	if rates, ok := rateTableOf(row); ok && len(rates) > 0 {
		return true
	}
	for _, f := range flatRateFields {
		if _, ok := floatField(row[f.field]); ok {
			return true
		}
	}
	return false
}

// priceKeysForLocked lists the keys to try for one model name, most specific
// first. The caller must already hold at least a read lock.
//
// Three spellings are tried for a prefixed name, and the alias is looked up for
// each: the name as written, the name without its provider prefix, and the alias
// the feed declares for either. Looking up the alias of only the full name misses
// the common case - a deployment is named after the vendor ("supplier/<model>")
// while the catalog keys the row by the vendor's own model id, and the alias
// table is keyed by that id. Without this a prefixed deployment bills from the
// catalog only when the row happens to already carry the vendor's id verbatim.
//
// 参数 model（string）：对外模型名。
// 返回 []string（[]string）：按优先顺序排列的候选键。
// 调用：priceRowFor。
// 测试：cost_at_test.go
func priceKeysForLocked(model string) []string {
	keys := []string{model}
	if i := strings.Index(model, "/"); i > 0 {
		keys = append(keys, model[i+1:])
	}
	// Resolve the alias of every spelling collected so far. The loop is over the
	// slice as it grows so an alias is itself offered without a prefix too.
	for _, candidate := range keys {
		if id, found := aliasKeyLocked(candidate); found {
			keys = append(keys, id)
			if i := strings.Index(id, "/"); i > 0 {
				keys = append(keys, id[i+1:])
			}
		}
	}
	return dedupeStrings(keys)
}

// dedupeStrings removes repeats while keeping the first occurrence, so the
// preference order is unchanged.
// 参数 keys（[]string）：候选键，可能含重复。
// 返回 []string（[]string）：去重后的候选键，原顺序保留。
// 调用：priceKeysForLocked。
// 测试：cost_at_test.go
func dedupeStrings(keys []string) []string {
	seen := make(map[string]struct{}, len(keys))
	out := keys[:0]
	for _, key := range keys {
		if key == "" {
			continue
		}
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, key)
	}
	return out
}

// SnapshotUsage preserves async measurements when a band or tool rate is missing.
// 参数 c（Charge）：已计算的费用；u（Usage）：供应商上报的原始用量；priced（bool）：能否按当前费率计价。
// 返回 string：包含计价状态与用量的 JSON 快照；序列化失败时为空串。
// 调用：异步任务结算的费用快照写入。
// 测试：cost_at_test.go、regression/pricing_test.go。
func SnapshotUsage(c Charge, u Usage, priced bool) string {
	if u.OutputVariant == "" {
		return Snapshot(c)
	}
	status := "unpriced"
	if priced {
		status = "priced"
	}
	raw, err := json.Marshal(PriceSnapshot{Window: c.Window, Applied: c.Applied, PricingStatus: status, Usage: &u})
	if err != nil {
		return ""
	}
	return string(raw)
}
