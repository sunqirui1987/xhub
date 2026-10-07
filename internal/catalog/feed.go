package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/sunqirui1987/xhub/internal/logx"
)

// MarketURL is the Modelink market feed the price catalog is generated from.
// Both the build-time generator and the runtime reload read this one URL.
const MarketURL = "https://api.modelink.ai/v1/market/models"

// feedDocument is the Modelink market response.
type feedDocument struct {
	Status bool        `json:"status"`
	Data   []feedModel `json:"data"`
}

// feedModel is one model in the market feed. Only the fields the catalog keeps
// are decoded; the feed carries several more that no rate depends on.
type feedModel struct {
	ID                string            `json:"id"`
	Name              string            `json:"name"`
	Description       string            `json:"description"`
	Avatar            string            `json:"avatar"`
	Features          []string          `json:"features"`
	ModelConstraints  feedConstraints   `json:"model_constraints"`
	Issuer            feedIssuer        `json:"issuer"`
	Architecture      feedArchitecture  `json:"architecture"`
	PricingRulesV2    []feedPricingRule `json:"pricing_rules_v2"`
	SupportAPIProtos  []string          `json:"support_api_protocols"`
	RetirementAt      string            `json:"retirement_at"`
	ReleaseAt         string            `json:"release_at"`
	SuggestedModel    string            `json:"suggested_model"`
	ModelAlias        []string          `json:"model_alias"`
	PricingPageURL    string            `json:"pricing_page_url"`
	ModelDocURL       string            `json:"model_doc_url"`
	IntegrationDocURL string            `json:"integration_doc_url"`
}

type feedConstraints struct {
	ContextLength       int `json:"context_length"`
	MaxCompletionTokens int `json:"max_completion_tokens"`
	MaxTokens           int `json:"max_tokens"`
}

type feedIssuer struct {
	Name string `json:"name"`
}

type feedArchitecture struct {
	InputModalities  []string    `json:"input_modalities"`
	OutputModalities []string    `json:"output_modalities"`
	SchemaOutput     feedSupport `json:"schema_output"`
	FunctionCalling  feedSupport `json:"function_calling"`
	Reasoning        feedSupport `json:"reasoning"`
	ContentCache     feedSupport `json:"content_cache"`
}

type feedSupport struct {
	Supported bool `json:"supported"`
}

type feedPricingRule struct {
	DetailsV2 map[string]feedUnit `json:"details_v2"`
}

// feedUnit is one rate in the feed. UnitPriceUSD is dollars per UnitSize units.
type feedUnit struct {
	UnitName     string  `json:"unit_name"`
	UnitSize     float64 `json:"unit_size"`
	UnitPrice    float64 `json:"unit_price"`
	UnitPriceUSD float64 `json:"unit_price_usd"`
	Name         string  `json:"name"`
}

// supplier is how one issuer in the feed appears as a supplier in the catalog.
// The slug is the litellm_provider value deployments match on, so it must stay
// stable once a deployment uses it.
type supplier struct {
	slug    string
	display string
	apiBase string
}

// suppliers maps a feed issuer name to its supplier entry. An issuer that is not
// listed still becomes a supplier, under a slug derived from its name.
var suppliers = map[string]supplier{
	"Aliyun":        {"dashscope", "Aliyun", "https://dashscope.aliyuncs.com/compatible-mode"},
	"OpenAI":        {"openai", "OpenAI", "https://api.openai.com"},
	"Google":        {"gemini", "Google", "https://generativelanguage.googleapis.com"},
	"DeepSeek":      {"deepseek", "DeepSeek", "https://api.deepseek.com"},
	"ByteDance":     {"volcengine", "ByteDance", "https://ark.cn-beijing.volces.com"},
	"Anthropic":     {"anthropic", "Anthropic", "https://api.anthropic.com"},
	"xAI":           {"xai", "xAI", "https://api.x.ai"},
	"Kling":         {"kling", "Kling", "https://api.klingai.com"},
	"Minimax":       {"minimax", "Minimax", "https://api.minimaxi.com"},
	"zAI":           {"zai", "z.ai", "https://api.z.ai"},
	"Vidu":          {"vidu", "Vidu", "https://api.vidu.cn"},
	"Moonshot-Kimi": {"moonshot", "Moonshot Kimi", "https://api.moonshot.cn"},
	"BytePlus":      {"byteplus", "BytePlus", "https://ark.ap-southeast.bytepluses.com"},
	"OpenRouter":    {"openrouter", "OpenRouter", "https://openrouter.ai/api"},
	"Tencent":       {"tencent", "Tencent", "https://api.hunyuan.cloud.tencent.com"},
	"Arcee-AI":      {"arcee_ai", "Arcee AI", "https://api.arcee.ai"},
	"Meituan":       {"meituan", "Meituan", "https://api.longcat.chat"},
	"Nvidia":        {"nvidia_nim", "NVIDIA", "https://integrate.api.nvidia.com"},
	"Stepfun":       {"stepfun", "StepFun", "https://api.stepfun.com"},
}

// tokenRateFields maps a feed pricing key onto the LiteLLM rate the gateway
// bills. A key that is not here is kept verbatim under price_units instead of
// being dropped, so the console can still show it.
var tokenRateFields = map[string]string{
	"ncache":     "input_cost_per_token",
	"input":      "input_cost_per_token",
	"output":     "output_cost_per_token",
	"cache":      "cache_read_input_token_cost",
	"c_cache":    "cache_creation_input_token_cost",
	"c_1h_cache": "cache_creation_input_token_cost_above_1hr",
	"bi_input":   "input_cost_per_token_batches",
	"bi_output":  "output_cost_per_token_batches",
}

// nonTokenRateFields are feed keys that bill per call rather than per token.
var nonTokenRateFields = map[string]string{
	"web_search_req": "search_context_cost_per_query",
}

// imageRateKeys and videoSecondKeys are the feed keys that price a picture and a
// second of video. A model that carries one is priced per output, not per token.
//
// They are the *unqualified* spellings. Most video and image models in the feed
// do not use them: they quote one price per variant instead, keyed by resolution
// and by whether the input carries a video (480p_v_duration, 4k_av_duration,
// wiv_v_output). Twenty-some keys for one model is normal.
//
// That is why variantRates exists. Before it, a model whose price was quoted only
// in variants ended up with no billable rate at all: the console showed "price not
// provided" for a model the feed had priced, and a call to it was recorded at zero
// cost. Thirty-four models were in that state.
var (
	imageRateKeys   = []string{"ti_quantity", "ii_quantity", "mi2i_quantity", "omi_quantity"}
	videoSecondKeys = []string{"av_duration", "v_duration"}
)

// variantUnitSuffix falls back to the unit the feed declares when a variant key
// does not carry a recognizable one.
// variantUnitOf 把市场声明的单位名收成展示用的单位词。
// 参数 unit（feedUnit）：一段计价单位。空单位名或认不出的名字一律算 unit。
// 返回 string（string）：second、picture、token 或 unit。
// 调用：convertFeedModel 在写 price_units 时。
// 测试：无直接单测
func variantUnitOf(unit feedUnit) string {
	switch strings.ToLower(strings.TrimSpace(unit.UnitName)) {
	case "second", "time":
		return "second"
	case "pic":
		return "picture"
	case "token":
		return "token"
	default:
		return "unit"
	}
}

// tokenKeys keeps the variant keys whose feed unit is a token. A picture or a
// second that happens to have "input" in its key is not a per-token rate.
// tokenKeys 只留下单位是 token 的键。
//
// 键名里带 input 的不一定是按 token 计费：i_input_quantity 是按张的输入图价。
// 少了这一层过滤，一个出图模型的"每张"价会被写进"每 token"字段，差六个数量级。
//
// 参数 details（map[string]feedUnit）：计价块；keys（[]string）：候选键。
// 返回 []string（[]string）：单位是 token 的那些键。
// 调用：convertFeedModel 在给按 token 的兜底价时。
// 测试：feed_test.go
func tokenKeys(details map[string]feedUnit, keys []string) []string {
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		unit, ok := details[key]
		if !ok {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(unit.UnitName), "token") {
			out = append(out, key)
		}
	}
	return out
}

// cheapestUnit returns the lowest-priced variant of one base unit, which is the
// rate a variant-only model is billed at.
//
// It is the cheapest rather than the dearest on purpose: over-charging a customer
// for a resolution they did not ask for is worse than under-charging for one they
// did, and the console shows every variant so the operator can price them
// individually.
// cheapestUnit 取一组变体里最便宜的那一档，作为这条模型的兜底费率。
//
// 取最便宜而不是最贵是有意的：按运维没要的分辨率多收钱，比少收钱更糟。
// 每一档都还留在 price_units 里，控制台会把它们都显示出来，运维可以逐个定价。
//
// 参数 details（map[string]feedUnit）：计价块；keys（...string）：候选键。
// 返回 float64（float64）：最便宜那一档的单价；bool（bool）：有可用的一档时为真。
// 调用：convertFeedModel 在给出兜底价时。
// 测试：feed_test.go
func cheapestUnit(details map[string]feedUnit, keys ...string) (float64, bool) {
	best := 0.0
	found := false
	for _, key := range keys {
		unit, ok := details[key]
		if !ok {
			continue
		}
		rate, ok := perUnit(unit)
		if !ok || rate <= 0 {
			continue
		}
		if !found || rate < best {
			best = rate
			found = true
		}
	}
	return best, found
}

// keysContaining returns the variant keys whose name contains any of the given
// fragments. Substring rather than suffix, because the feed puts the qualifier on
// either side: 480p_v_duration, ncache_offpeak, nth_input all describe the same
// kind of thing and only a substring match finds all three.
// keysContaining 返回名字里含任一给定片段的变体键。
//
// 用子串而不是后缀，因为市场把限定词放在词的两侧：480p_v_duration 在后面，
// nth_input 在前面，output_peak 直接连在后面。只认后缀会漏掉后两种。
//
// 参数 details（map[string]feedUnit）：计价块；fragments（...string）：要匹配的片段。
// 返回 []string（[]string）：名字含任一片段、并且排好序的键。
// 调用：convertFeedModel 在挑候选变体时。
// 测试：feed_test.go
func keysContaining(details map[string]feedUnit, fragments ...string) []string {
	var out []string
	for key := range details {
		for _, fragment := range fragments {
			if strings.Contains(key, fragment) {
				out = append(out, key)
				break
			}
		}
	}
	sort.Strings(out)
	return out
}

// pictureOutputKeys returns the per-picture variant keys that price an output.
//
// A picture model quotes both sides: i_input_quantity is what an input picture
// costs, i_output_quantity what a generated one costs. Only the output side may
// become output_cost_per_image.
//
// 参数 details（map[string]feedUnit）：这条模型的计价块。
// 返回 []string（[]string）：单位是张、并且描述输出那一侧的键。
// 调用：convertFeedModel 在给出图模型兜底价时。
// 测试：feed_test.go
func pictureOutputKeys(details map[string]feedUnit) []string {
	var out []string
	for _, key := range keysContaining(details, "_quantity") {
		if strings.Contains(key, "input") && !strings.Contains(key, "output") {
			continue
		}
		unit, ok := details[key]
		if !ok || !strings.EqualFold(strings.TrimSpace(unit.UnitName), "pic") {
			continue
		}
		out = append(out, key)
	}
	return out
}

// customProvider is the generic supplier that keeps "Custom Bypass" usable: an
// operator points api_base at any documented endpoint without registering a Go
// package first.
var customProvider = map[string]any{
	"provider":                  "Custom",
	"provider_display_name":     "Custom",
	"litellm_provider":          "custom",
	"credential_fields":         credentialFields("Custom", "https://api.example.com", true),
	"default_model_placeholder": nil,
	"default_api_base":          nil,
	"model_count":               0,
}

// BuildPriceDocument converts a market feed response into the price catalog
// document. An empty feed returns an error rather than an empty catalog, so a
// failed reload cannot wipe the prices in use.
// 参数 raw（[]byte）：市场接口返回的原始正文。
// 返回 PriceDocument（PriceDocument）：可直接嵌入或写入磁盘的价格目录；error（error）：解析失败或没有模型时不为 nil。
// 调用：cmd/pricedata 生成内嵌文件；ReloadFromMarket 刷新运行中的价格表。
// 测试：pricedata_test.go
func BuildPriceDocument(raw []byte) (PriceDocument, error) {
	var feed feedDocument
	if err := json.Unmarshal(raw, &feed); err != nil {
		return PriceDocument{}, fmt.Errorf("market feed is not readable: %w", err)
	}
	if len(feed.Data) == 0 {
		logx.Error("market feed carried no models; refusing to build an empty catalog")
		return PriceDocument{}, fmt.Errorf("market feed returned no models")
	}

	models := make(map[string]map[string]any, len(feed.Data))
	perSupplier := map[string][]string{}
	seen := map[string]bool{}
	for _, model := range feed.Data {
		id := strings.TrimSpace(model.ID)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		s := supplierFor(model.Issuer.Name)
		row := convertFeedModel(model)
		row["litellm_provider"] = s.slug
		models[id] = row
		perSupplier[s.slug] = append(perSupplier[s.slug], id)
	}

	providers := make([]map[string]any, 0, len(perSupplier)+1)
	for slug, ids := range perSupplier {
		sort.Strings(ids)
		info := supplierForSlug(slug)
		providers = append(providers, map[string]any{
			"provider":                  info.display,
			"provider_display_name":     info.display,
			"litellm_provider":          slug,
			"credential_fields":         credentialFields(info.display, info.apiBase, false),
			"default_model_placeholder": ids[0],
			"default_api_base":          info.apiBase,
			"model_count":               len(ids),
		})
	}
	sort.Slice(providers, func(i, j int) bool {
		return fmt.Sprint(providers[i]["litellm_provider"]) < fmt.Sprint(providers[j]["litellm_provider"])
	})
	providers = append(providers, customProvider)

	return PriceDocument{
		Version:     1,
		Source:      MarketURL,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Providers:   providers,
		Models:      models,
	}, nil
}

// supplierFor 把发行方名字收成供应商。名单里没有的发行方保留原名，slug 由名字小写并把空格和连字符换成下划线。空名字收成 other。
// 参数 issuer（string）：市场条目里的发行方名字。
// 返回 supplier（supplier）：slug 和显示名。未知发行方不会被丢弃。
// 调用：BuildPriceDocument。
// 测试：无直接单测
func supplierFor(issuer string) supplier {
	issuer = strings.TrimSpace(issuer)
	if s, ok := suppliers[issuer]; ok {
		return s
	}
	slug := strings.ToLower(issuer)
	slug = strings.NewReplacer("-", "_", " ", "_").Replace(slug)
	if slug == "" {
		slug, issuer = "other", "Other"
	}
	return supplier{slug: slug, display: issuer}
}

// supplierForSlug 按已经得出的 slug 找回供应商。没有这条 slug 时显示名就用 slug 本身。
// 参数 slug（string）：供应商标识，例如 openai。
// 返回 supplier（supplier）：名单里的供应商。没有时 slug 和显示名都是传入的 slug。
// 调用：BuildPriceDocument 在汇总每个供应商的模型数时。
// 测试：无直接单测
func supplierForSlug(slug string) supplier {
	for _, s := range suppliers {
		if s.slug == slug {
			return s
		}
	}
	return supplier{slug: slug, display: slug}
}

// credentialFields 给出每个供应商都要填的两项：API Base 和 API Key。
// 参数 display（string）：显示名，写进密钥字段的标签；apiBase（string）：默认根地址，空串时不预填；requireBase（bool）：为真时 API Base 必填，并且不把根地址写成默认值。
// 返回 []any（[]any）：两条字段定义，先是 api_base，后是 api_key。
// 调用：BuildPriceDocument。
// 测试：无直接单测
func credentialFields(display, apiBase string, requireBase bool) []any {
	var placeholder, defaultValue any
	if apiBase != "" {
		placeholder = apiBase
		if !requireBase {
			defaultValue = apiBase
		}
	}
	return []any{
		map[string]any{
			"key": "api_base", "label": "API Base", "placeholder": placeholder, "tooltip": nil,
			"required": requireBase, "field_type": "text", "options": nil, "default_value": defaultValue,
		},
		map[string]any{
			"key": "api_key", "label": display + " API Key", "placeholder": nil, "tooltip": nil,
			"required": true, "field_type": "password", "options": nil, "default_value": nil,
		},
	}
}

// convertFeedModel 把一条市场模型收成价格行。市场没给的费率留空，不写成 0，避免没标价的模型被当成免费。
// 参数 model（feedModel）：市场接口里的一条模型。
// 返回 map[string]any（map[string]any）：价格表的一行，含模式、上下文长度和能折算的费率。没折进账单的单价留在 price_units。
// 调用：BuildPriceDocument。
// 测试：无直接单测
func convertFeedModel(model feedModel) map[string]any {
	details := feedDetails(model.PricingRulesV2)

	row := map[string]any{
		"mode":              modeOf(model.Architecture),
		"source":            MarketURL,
		"display_name":      firstNonEmpty(model.Name, model.ID),
		"max_input_tokens":  positiveOrNil(model.ModelConstraints.ContextLength),
		"max_output_tokens": positiveOrNil(firstPositive(model.ModelConstraints.MaxCompletionTokens, model.ModelConstraints.MaxTokens)),
	}
	putNonEmpty(row, "description", model.Description)
	putNonEmpty(row, "avatar", model.Avatar)
	putNonEmpty(row, "retirement_at", model.RetirementAt)
	putNonEmpty(row, "release_at", model.ReleaseAt)
	putNonEmpty(row, "suggested_model", model.SuggestedModel)
	putNonEmpty(row, "pricing_page_url", model.PricingPageURL)
	putNonEmpty(row, "model_doc_url", model.ModelDocURL)
	putNonEmpty(row, "integration_doc_url", model.IntegrationDocURL)
	putNonEmptyList(row, "features", model.Features)
	putNonEmptyList(row, "model_alias", model.ModelAlias)
	putNonEmptyList(row, "support_api_protocols", model.SupportAPIProtos)

	// ncache and input both mean the ordinary input side. Every model that
	// carries both quotes them equal, so the first one written wins.
	for _, key := range []string{"ncache", "input", "output", "cache", "c_cache", "c_1h_cache", "bi_input", "bi_output"} {
		unit, ok := details[key]
		if !ok {
			continue
		}
		field := tokenRateFields[key]
		if _, exists := row[field]; exists {
			continue
		}
		if rate, ok := perUnit(unit); ok {
			row[field] = rate
		}
	}
	for key, field := range nonTokenRateFields {
		if unit, ok := details[key]; ok {
			if rate, ok := perUnit(unit); ok {
				row[field] = rate
			}
		}
	}
	for _, key := range imageRateKeys {
		if unit, ok := details[key]; ok {
			if rate, ok := perUnit(unit); ok {
				row["output_cost_per_image"] = rate
				break
			}
		}
	}
	for _, key := range videoSecondKeys {
		if unit, ok := details[key]; ok {
			if rate, ok := perUnit(unit); ok {
				row["output_cost_per_second"] = rate
				break
			}
		}
	}

	// Variant pricing: the feed quotes one price per resolution or per input mode
	// instead of a single unqualified rate. Without this fallback such a model has
	// no billable rate, so it shows no price and bills at zero.
	//
	// The cheapest variant becomes the model's rate. Which variant was picked is
	// not recorded here; every one of them is still listed under price_units, so
	// the console shows the operator what the choice was.
	if _, ok := row["output_cost_per_second"]; !ok {
		if rate, found := cheapestUnit(details, keysContaining(details, "_v_duration", "_av_duration", "_v_input_duration")...); found {
			row["output_cost_per_second"] = rate
		}
	}
	if _, ok := row["output_cost_per_image"]; !ok {
		// Only output keys. A picture model quotes both sides (i_input_quantity is
		// what an input picture costs), and taking the cheapest of the two put the
		// *input* price on the output rate.
		if rate, found := cheapestUnit(details, pictureOutputKeys(details)...); found {
			row["output_cost_per_image"] = rate
		}
	}
	// A video model that quotes only per-token variants (wiv_v_output) is billed
	// per output token.
	if _, ok := row["output_cost_per_token"]; !ok {
		if rate, found := cheapestUnit(details, tokenKeys(details, keysContaining(details, "_v_output"))...); found {
			row["output_cost_per_token"] = rate
		}
	}

	// The remaining families quote variants that are not about resolution but
	// about *when* or *how* the call runs. Each still has one ordinary input and
	// one ordinary output side, and the qualified keys are variants of those two.
	// Picking the cheapest of each side gives the model a rate; price_units keeps
	// every variant visible.
	//
	//   非思考 / 思考               qwen: nth_input, th_output
	//   空闲 / 高峰                 deepseek: ncache_offpeak, output_peak
	//   文本输入 / 图片输入 / 图片输出  gpt-image: t_input, i_output
	//   文生 / 图生 / 参考主体生      vidu: 1080p_t2v_duration, 540p_i2v_duration
	//
	// Matching is by substring rather than by suffix, because the qualifier sits
	// on either side of the word: ncache_offpeak has it after, nth_input has it
	// before, and output_peak has it after with no separator. A suffix-only match
	// found the cache rate but missed both the input and output sides, which left
	// a deepseek model billable on cache reads alone.
	if _, ok := row["input_cost_per_token"]; !ok {
		if rate, found := cheapestUnit(details, tokenKeys(details, keysContaining(details, "input", "ncache"))...); found {
			row["input_cost_per_token"] = rate
		}
	}
	if _, ok := row["output_cost_per_token"]; !ok {
		if rate, found := cheapestUnit(details, tokenKeys(details, keysContaining(details, "output", "oth_output"))...); found {
			row["output_cost_per_token"] = rate
		}
	}
	if _, ok := row["cache_read_input_token_cost"]; !ok {
		if rate, found := cheapestUnit(details, tokenKeys(details, keysContaining(details, "cache"))...); found {
			row["cache_read_input_token_cost"] = rate
		}
	}
	// vidu quotes only per-second variants named after resolution and how the
	// video is made (t2v = text to video, i2v = image to video, r2v = reference).
	if _, ok := row["output_cost_per_second"]; !ok {
		if rate, found := cheapestUnit(details, keysContaining(details, "_duration")...); found {
			row["output_cost_per_second"] = rate
		}
	}

	putTrue(row, "supports_function_calling", model.Architecture.FunctionCalling.Supported)
	putTrue(row, "supports_response_schema", model.Architecture.SchemaOutput.Supported)
	putTrue(row, "supports_reasoning", model.Architecture.Reasoning.Supported)
	_, priced := row["cache_read_input_token_cost"]
	putTrue(row, "supports_prompt_caching", model.Architecture.ContentCache.Supported || priced)
	putTrue(row, "supports_vision", contains(model.Architecture.InputModalities, "image"))
	putTrue(row, "supports_video_input", contains(model.Architecture.InputModalities, "video"))
	putTrue(row, "supports_audio_input", contains(model.Architecture.InputModalities, "audio"))

	// Anything not folded into a billable rate stays under its feed key so the
	// console can still show it.
	leftovers := map[string]any{}
	for key, unit := range details {
		if isMappedRateKey(key) {
			continue
		}
		leftovers[key] = unitBlock(unit)
	}
	if len(leftovers) > 0 {
		row["price_units"] = leftovers
	}
	return row
}

// feedDetails 取这条模型正在生效的计价块。市场按顺序给多段规则，第一段非空的 DetailsV2 就是当前费率。
// 参数 rules（[]feedPricingRule）：这条模型的计价规则。
// 返回 map[string]feedUnit（map[string]feedUnit）：费率键到单价。每一段都空时是空表。
// 调用：convertFeedModel。
// 测试：无直接单测
func feedDetails(rules []feedPricingRule) map[string]feedUnit {
	for _, rule := range rules {
		if len(rule.DetailsV2) > 0 {
			return rule.DetailsV2
		}
	}
	return map[string]feedUnit{}
}

// isMappedRateKey 判断这个市场键是否已经折进可计费字段。折过的键不再放进 price_units。
// 参数 key（string）：市场计价块里的键，例如 input 或 output。
// 返回 bool（bool）：这个键已经对应某个账单字段时返回真。
// 调用：convertFeedModel。
// 测试：无直接单测
func isMappedRateKey(key string) bool {
	if _, ok := tokenRateFields[key]; ok {
		return true
	}
	if _, ok := nonTokenRateFields[key]; ok {
		return true
	}
	return contains(imageRateKeys, key) || contains(videoSecondKeys, key)
}

// modeOf 按模型产出决定目录分类，分类决定能调用它的端点。
// 参数 a（feedArchitecture）：市场给出的输入输出形态。
// 返回 string（string）：video_generation、image_generation、audio_speech 或 chat。没有视频、图像、音频输出时是 chat。
// 调用：convertFeedModel。
// 测试：无直接单测
func modeOf(a feedArchitecture) string {
	switch {
	case contains(a.OutputModalities, "video"):
		return "video_generation"
	case contains(a.OutputModalities, "image"):
		return "image_generation"
	case contains(a.OutputModalities, "audio"):
		return "audio_speech"
	default:
		return "chat"
	}
}

// perUnit 把市场单价换成网关计费用的一单位美元价：一个 token、一张图、一秒或一次查询。
// 参数 unit（feedUnit）：市场的一段单价，含单位大小和美元价格。
// 返回 float64（float64）：每一基础单位的美元价。负价或三项都空时为 0；bool（bool）：这项可以写入价格行时为真。市场写成 0 的价格是真价格，也会返回真。
// 调用：convertFeedModel 和 unitBlock。
// 测试：无直接单测
func perUnit(unit feedUnit) (float64, bool) {
	if unit.UnitName == "" && unit.UnitSize == 0 && unit.UnitPriceUSD == 0 {
		return 0, false
	}
	size := unit.UnitSize
	if size <= 0 {
		size = 1
	}
	rate := unit.UnitPriceUSD / size
	if rate < 0 {
		return 0, false
	}
	return roundRate(rate), true
}

// unitBlock 把一段没折进账单的单价留下，给控制台展示。
// 参数 unit（feedUnit）：市场的一段单价。
// 返回 map[string]any（map[string]any）：含折算后的 usd、原始 cny、单位名、单位大小和标签。
// 调用：convertFeedModel。
// 测试：无直接单测
func unitBlock(unit feedUnit) map[string]any {
	rate, _ := perUnit(unit)
	return map[string]any{
		"usd":   rate,
		"cny":   unit.UnitPrice,
		"unit":  unit.UnitName,
		"size":  unit.UnitSize,
		"label": unit.Name,
	}
}

// roundRate 去掉二进制浮点的尾巴，让存下来的费率和市场公布的数字一致。
// 参数 v（float64）：折算后的单价。
// 返回 float64（float64）：保留 12 位有效数字后的单价。传入 0 时仍是 0。
// 调用：perUnit。
// 测试：无直接单测
func roundRate(v float64) float64 {
	if v == 0 {
		return 0
	}
	return float64(roundToSignificant(v, 12))
}

// roundToSignificant 把 v 收成指定位数的有效数字。
// 参数 v（float64）：要收的数；digits（int）：保留的有效数字位数。
// 返回 float64（float64）：按有效数字重新解析后的值。解析失败时退回 v。
// 调用：roundRate。
// 测试：无直接单测
func roundToSignificant(v float64, digits int) float64 {
	out, err := strconv.ParseFloat(strconv.FormatFloat(v, 'g', digits, 64), 64)
	if err != nil {
		return v
	}
	return out
}

// firstNonEmpty 返回第一个去掉空白后仍非空的字符串。
// 参数 values（...string）：按优先顺序给出的候选。
// 返回 string（string）：第一个非空文本。全都是空白时为空串。
// 调用：convertFeedModel，用来取显示名。
// 测试：无直接单测
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}

// firstPositive 返回第一个大于 0 的整数。
// 参数 values（...int）：按优先顺序给出的上限。
// 返回 int（int）：第一个正数。都不是正数时为 0。
// 调用：convertFeedModel，用来取最大输出 token。
// 测试：无直接单测
func firstPositive(values ...int) int {
	for _, v := range values {
		if v > 0 {
			return v
		}
	}
	return 0
}

// positiveOrNil 只在 v 大于 0 时返回它。缺了的上限必须从价格行里消失，不能存成 0。
// 参数 v（int）：上下文或输出上限。
// 返回 any（any）：大于 0 时是这个整数。否则是 nil，这一项不会写进价格行。
// 调用：convertFeedModel。
// 测试：无直接单测
func positiveOrNil(v int) any {
	if v > 0 {
		return v
	}
	return nil
}

// contains 判断列表里有没有这个值。
// 参数 list（[]string）：要查的列表；want（string）：要找的值，按字符串相等比较。
// 返回 bool（bool）：列表里有 want 时返回真。
// 调用：modeOf、isMappedRateKey、convertFeedModel。
// 测试：无直接单测
func contains(list []string, want string) bool {
	for _, item := range list {
		if item == want {
			return true
		}
	}
	return false
}

// putNonEmpty 在去掉空白后仍非空时，把字符串写进价格行。
// 参数 row（map[string]any）：正在组装的价格行；key（string）：字段名；value（string）：市场给出的文本。
// 返回：无。空白不会写入，所以这一项不会变成空字符串。
// 调用：convertFeedModel。
// 测试：无直接单测
func putNonEmpty(row map[string]any, key, value string) {
	if s := strings.TrimSpace(value); s != "" {
		row[key] = s
	}
}

// putNonEmptyList 把去掉空白后仍非空的字符串写进价格行。整列都空时不写这个键。
// 参数 row（map[string]any）：正在组装的价格行；key（string）：字段名；values（[]string）：市场给出的列表。
// 返回：无。有内容时 row[key] 是收干净的字符串切片。
// 调用：convertFeedModel。
// 测试：无直接单测
func putNonEmptyList(row map[string]any, key string, values []string) {
	out := make([]string, 0, len(values))
	for _, v := range values {
		if s := strings.TrimSpace(v); s != "" {
			out = append(out, s)
		}
	}
	if len(out) > 0 {
		row[key] = out
	}
}

// putTrue 只在能力打开时写入 true。缺了这个键就表示不支持。
// 参数 row（map[string]any）：正在组装的价格行；key（string）：能力字段名；on（bool）：市场是否声明支持。
// 返回：无。on 为假时不写这个键。
// 调用：convertFeedModel。
// 测试：无直接单测
func putTrue(row map[string]any, key string, on bool) {
	if on {
		row[key] = true
	}
}

// FetchMarket reads the market feed. It is the runtime counterpart of the
// build-time generator, and both go through BuildPriceDocument.
// 参数 ctx（context.Context）：请求的取消和超时；url（string）：要读的地址。空串用 MarketURL。
// 返回 PriceDocument（PriceDocument）：转换好的价格目录；error（error）：网络或解析失败时不为 nil。
// 调用：ReloadFromMarket。
// 测试：pricedata_test.go 用本地服务覆盖。
func FetchMarket(ctx context.Context, url string) (PriceDocument, error) {
	if strings.TrimSpace(url) == "" {
		url = MarketURL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return PriceDocument{}, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := marketClient.Do(req)
	if err != nil {
		return PriceDocument{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return PriceDocument{}, fmt.Errorf("market feed answered %d", resp.StatusCode)
	}
	// The feed is a few hundred KB. The limit is a guard against an endless
	// response, not a size the feed is expected to approach.
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return PriceDocument{}, err
	}
	return BuildPriceDocument(raw)
}

// ReloadFromMarket fetches the market feed and swaps it in as the live price
// catalog. A failed fetch leaves the prices in use untouched and reports why.
// 参数 ctx（context.Context）：请求的取消和超时；url（string）：要读的地址。空串用 MarketURL。
// 返回 int（int）：换上的模型条数；error（error）：抓取、解析或应用失败时不为 nil。
// 调用：gateway/models.ReloadCostMap。
// 测试：pricedata_test.go 用本地服务覆盖。
func ReloadFromMarket(ctx context.Context, url string) (int, error) {
	doc, err := FetchMarket(ctx, url)
	if err != nil {
		logx.Error("price catalog fetch failed url=%s err=%v", url, err)
		return 0, err
	}
	n, err := ApplyDocument(doc)
	if err != nil {
		logx.Error("price catalog apply failed err=%v", err)
		return 0, err
	}
	logx.Info("price catalog reloaded models=%d source=%s", n, doc.Source)
	return n, nil
}

// marketClient is the HTTP client the reload uses. A reload that hangs must not
// hold the handler open, so the timeout is on the client rather than the caller.
var marketClient = &http.Client{Timeout: 60 * time.Second}
