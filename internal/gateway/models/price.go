// Package models manages the price catalog: the embedded Modelink baseline plus
// the rows and suppliers an operator adds from the console. The stored overrides
// are laid over the embedded file at startup, so editing a price never rewrites
// the generated document.
package models

import (
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/sunqirui1987/xhub/internal/catalog"
	"github.com/sunqirui1987/xhub/internal/httpx"
	"github.com/sunqirui1987/xhub/internal/logx"
	"sync"
)

const (
	// priceModelNS holds one row per hand-entered or edited model, keyed by id.
	priceModelNS = "price_model"
	// priceProviderNS holds one row per hand-added supplier, keyed by slug.
	priceProviderNS = "price_provider"
	// priceRemovedNS holds the ids an operator deleted from the catalog.
	priceRemovedNS = "price_removed"
)

var logTraceOncePrice sync.Once

// rateFields are the per-token rates the console edits. They are stored per
// token, which is the unit the gateway bills in.
var rateFields = []string{
	"input_cost_per_token",
	"output_cost_per_token",
	"input_cost_per_token_peak",
	"output_cost_per_token_peak",
	"cache_read_input_token_cost",
	"cache_creation_input_token_cost",
	"input_cost_per_image",
	"output_cost_per_image",
	"input_cost_per_second",
	"output_cost_per_second",
	"search_context_cost_per_query",
}

// textFields are the descriptive fields carried on a price row.
var textFields = []string{
	"display_name",
	"description",
	"upstream_model",
	"supplier_name",
	"mode",
	"endpoint_type",
	"source",
	"max_input_tokens",
	"max_output_tokens",
	"avatar",
	"pricing_page_url",
	"retirement_at",
	"release_at",
}

// flagFields are the capability booleans the console shows as badges.
var flagFields = []string{
	"supports_function_calling",
	"supports_response_schema",
	"supports_reasoning",
	"supports_vision",
	"supports_prompt_caching",
}

// LoadPriceOverrides applies the stored catalog edits over the embedded
// baseline. It runs at startup, before the process serves traffic.
// 参数 s（Host）：能读框架记录和模型表的宿主。
// 返回：无。把库里保存的价格改动并进内存价格表。
// 调用：gateway.New。
// 测试：无直接单测
func LoadPriceOverrides(s Host) {
	logTraceOncePrice.Do(func() { logx.Trace("enter models.LoadPriceOverrides") })

	st := s.RecordStore()
	if st == nil {
		return
	}
	// Deletions first: a row that was removed must not come back when the
	// stored overrides are replayed.
	if removed, err := st.ListConfig(priceRemovedNS); err == nil {
		for id, raw := range removed {
			if gone, ok := raw.(bool); ok && gone {
				catalog.RemoveModel(id)
			}
		}
	}
	rows, err := st.ListConfig(priceModelNS)
	if err != nil {
		logx.Error("price overrides unreadable: %v", err)
		return
	}
	for id, raw := range rows {
		if row, ok := raw.(map[string]any); ok {
			catalog.SetModel(id, row)
		}
	}
	providers, err := st.ListConfig(priceProviderNS)
	if err != nil {
		logx.Error("price providers unreadable: %v", err)
		return
	}
	for slug, raw := range providers {
		if row, ok := raw.(map[string]any); ok {
			catalog.SetProvider(slug, row)
		}
	}
	// A hand-added supplier that the operator removed is not stored again, so
	// only the embedded ones need the provider tombstone.
	if removed, err := st.ListConfig(priceRemovedNS + "_provider"); err == nil {
		for slug, raw := range removed {
			if gone, ok := raw.(bool); ok && gone {
				catalog.RemoveProvider(slug)
			}
		}
	}
}

// PriceList returns the live price catalog for the console: every model with the
// flag saying whether it came from the embedded file, and every supplier.
// 参数 s（Host）：能读框架记录的宿主；w（http.ResponseWriter）：响应写到这；r（*http.Request）：入站请求。
// 返回：无。状态码和正文写进调用方的响应。
// 调用：gateway/models/mount.go
// 测试：无直接单测
func PriceList(s Host, w http.ResponseWriter, r *http.Request) {
	if s.RequireManage(w, r) == nil {
		return
	}
	rows := catalog.CostMap()
	overrides := storedKeys(s, priceModelNS)
	removed := storedKeys(s, priceRemovedNS)
	models := make([]map[string]any, 0, len(rows)+len(removed))
	for id, row := range rows {
		entry := make(map[string]any, len(row)+3)
		for k, v := range row {
			entry[k] = v
		}
		entry["id"] = id
		entry["baseline"] = catalog.IsBaseline(id)
		entry["overridden"] = overrides[id]
		models = append(models, entry)
	}
	// A baseline row the operator deleted is gone from the live map, so the
	// console learns about it from the tombstone. Keeping it in the list is what
	// makes the deletion undoable.
	for id := range removed {
		if _, live := rows[id]; live {
			continue
		}
		models = append(models, map[string]any{
			"id": id, "baseline": true, "overridden": false, "removed": true,
		})
	}
	sort.Slice(models, func(i, j int) bool {
		return str(models[i]["id"]) < str(models[j]["id"])
	})

	providers := catalog.Providers()
	providerOverrides := storedKeys(s, priceProviderNS)
	out := make([]map[string]any, 0, len(providers))
	for _, p := range providers {
		slug := str(p["litellm_provider"])
		entry := make(map[string]any, len(p)+2)
		for k, v := range p {
			entry[k] = v
		}
		entry["baseline"] = catalog.IsBaselineProvider(slug)
		entry["overridden"] = providerOverrides[slug]
		out = append(out, entry)
	}
	sort.Slice(out, func(i, j int) bool {
		return str(out[i]["litellm_provider"]) < str(out[j]["litellm_provider"])
	})

	httpx.WriteJSON(w, 200, map[string]any{
		"models":       models,
		"providers":    out,
		"source":       catalog.PriceSource(),
		"generated_at": catalog.PriceGeneratedAt(),
		"loaded_at":    catalog.LoadedAt(),
		"count":        len(models),
	})
}

// UpsertPriceModel adds or edits one model in the price catalog. A row that the
// embedded file already has is overridden, not duplicated.
// 参数 s（Host）：能写框架记录的宿主；w（http.ResponseWriter）：响应写到这；r（*http.Request）：入站请求。
// 返回：无。状态码和正文写进调用方的响应。
// 调用：gateway/models/mount.go
// 测试：无直接单测
func UpsertPriceModel(s Host, w http.ResponseWriter, r *http.Request) {
	if s.RequireManage(w, r) == nil {
		return
	}
	body := readMap(r)
	id := strings.TrimSpace(str(body["id"]))
	if id == "" {
		httpx.WriteError(w, 400, "invalid_request", "id is required")
		return
	}
	provider := strings.TrimSpace(str(body["litellm_provider"]))
	if provider == "" {
		httpx.WriteError(w, 400, "invalid_request", "litellm_provider is required")
		return
	}
	row := priceRowFrom(body, provider)
	for _, field := range rateFields {
		v, exists := body[field]
		if !exists || v == nil {
			continue
		}
		if text, ok := v.(string); ok && strings.TrimSpace(text) == "" {
			continue
		}
		if n, valid := numberField(v); !valid || n < 0 {
			httpx.WriteError(w, 400, "invalid_request", field+" must be a finite non-negative number")
			return
		}
	}

	if err := s.RecordStore().PutConfig(priceModelNS, id, row); err != nil {
		httpx.WriteError(w, 500, "internal", err.Error())
		return
	}
	// An id that was deleted and is now being written back is live again.
	if s.RecordStore().DeleteConfig(priceRemovedNS, id) != nil {
		logx.Debug("price tombstone for %s was already absent", id)
	}
	catalog.SetModel(id, row)
	httpx.WriteJSON(w, 200, map[string]any{"status": "success", "id": id})
}

// DeletePriceModel removes one model from the live catalog. The embedded file is
// not rewritten, so the deletion is remembered as a tombstone.
// 参数 s（Host）：能写框架记录的宿主；w（http.ResponseWriter）：响应写到这；r（*http.Request）：入站请求。
// 返回：无。状态码和正文写进调用方的响应。
// 调用：gateway/models/mount.go
// 测试：无直接单测
func DeletePriceModel(s Host, w http.ResponseWriter, r *http.Request) {
	if s.RequireManage(w, r) == nil {
		return
	}
	id := strings.TrimSpace(str(readMap(r)["id"]))
	if id == "" {
		httpx.WriteError(w, 400, "invalid_request", "id is required")
		return
	}
	if _, ok := catalog.BaselineModel(id); ok {
		if err := s.RecordStore().PutConfig(priceRemovedNS, id, true); err != nil {
			httpx.WriteError(w, 500, "internal", err.Error())
			return
		}
	}
	if err := s.RecordStore().DeleteConfig(priceModelNS, id); err != nil {
		logx.Debug("price override for %s was already absent", id)
	}
	catalog.RemoveModel(id)
	httpx.WriteJSON(w, 200, map[string]any{"status": "success", "id": id})
}

// ResetPriceModel drops an override and restores the embedded row.
// 参数 s（Host）：能写框架记录的宿主；w（http.ResponseWriter）：响应写到这；r（*http.Request）：入站请求。
// 返回：无。状态码和正文写进调用方的响应。
// 调用：gateway/models/mount.go
// 测试：无直接单测
func ResetPriceModel(s Host, w http.ResponseWriter, r *http.Request) {
	if s.RequireManage(w, r) == nil {
		return
	}
	id := strings.TrimSpace(str(readMap(r)["id"]))
	if id == "" {
		httpx.WriteError(w, 400, "invalid_request", "id is required")
		return
	}
	if err := s.RecordStore().DeleteConfig(priceModelNS, id); err != nil {
		logx.Debug("price override for %s was already absent", id)
	}
	if err := s.RecordStore().DeleteConfig(priceRemovedNS, id); err != nil {
		logx.Debug("price tombstone for %s was already absent", id)
	}
	row, ok := catalog.BaselineModel(id)
	if !ok {
		catalog.RemoveModel(id)
		httpx.WriteJSON(w, 200, map[string]any{"status": "success", "id": id, "restored": false})
		return
	}
	catalog.SetModel(id, row)
	httpx.WriteJSON(w, 200, map[string]any{"status": "success", "id": id, "restored": true})
}

// UpsertPriceProvider adds or edits one supplier in the add-model dropdown.
// 参数 s（Host）：能写框架记录的宿主；w（http.ResponseWriter）：响应写到这；r（*http.Request）：入站请求。
// 返回：无。状态码和正文写进调用方的响应。
// 调用：gateway/models/mount.go
// 测试：无直接单测
func UpsertPriceProvider(s Host, w http.ResponseWriter, r *http.Request) {
	if s.RequireManage(w, r) == nil {
		return
	}
	body := readMap(r)
	slug := strings.TrimSpace(str(body["litellm_provider"]))
	if slug == "" {
		httpx.WriteError(w, 400, "invalid_request", "litellm_provider is required")
		return
	}
	row := providerRowFrom(body, slug)
	if err := s.RecordStore().PutConfig(priceProviderNS, slug, row); err != nil {
		httpx.WriteError(w, 500, "internal", err.Error())
		return
	}
	if err := s.RecordStore().DeleteConfig(priceRemovedNS+"_provider", slug); err != nil {
		logx.Debug("provider tombstone for %s was already absent", slug)
	}
	catalog.SetProvider(slug, row)
	httpx.WriteJSON(w, 200, map[string]any{"status": "success", "litellm_provider": slug})
}

// DeletePriceProvider removes one supplier from the dropdown.
// 参数 s（Host）：能写框架记录的宿主；w（http.ResponseWriter）：响应写到这；r（*http.Request）：入站请求。
// 返回：无。状态码和正文写进调用方的响应。
// 调用：gateway/models/mount.go
// 测试：无直接单测
func DeletePriceProvider(s Host, w http.ResponseWriter, r *http.Request) {
	if s.RequireManage(w, r) == nil {
		return
	}
	slug := strings.TrimSpace(str(readMap(r)["litellm_provider"]))
	if slug == "" {
		httpx.WriteError(w, 400, "invalid_request", "litellm_provider is required")
		return
	}
	if catalog.IsBaselineProvider(slug) {
		if err := s.RecordStore().PutConfig(priceRemovedNS+"_provider", slug, true); err != nil {
			httpx.WriteError(w, 500, "internal", err.Error())
			return
		}
	}
	if err := s.RecordStore().DeleteConfig(priceProviderNS, slug); err != nil {
		logx.Debug("price provider %s was already absent", slug)
	}
	catalog.RemoveProvider(slug)
	httpx.WriteJSON(w, 200, map[string]any{"status": "success", "litellm_provider": slug})
}

// storedKeys reads which ids have a row in a namespace. A nil store reads as an
// empty set, which is the same answer as an empty table.
// 参数 s（Host）：能读框架记录的宿主；namespace（string）：要读的命名空间。
// 返回 map[string]bool（map[string]bool）：该命名空间里出现过的键。读不到时为空表。
// 调用：PriceList 标记哪些行是人工覆盖的。
// 测试：无直接单测
func storedKeys(s Host, namespace string) map[string]bool {
	if s.RecordStore() == nil {
		return map[string]bool{}
	}
	cfg, err := s.RecordStore().ListConfig(namespace)
	if err != nil {
		return map[string]bool{}
	}
	out := make(map[string]bool, len(cfg))
	for key := range cfg {
		out[key] = true
	}
	return out
}

// priceRowFrom builds a price row from the request, keeping only known fields.
// A rate that is absent stays absent so an unpriced model is never billed as
// free; a rate that is present is taken per token exactly as sent.
// 参数 body（map[string]any）：请求正文；provider（string）：这一行归属的供应商标识。
// 返回 map[string]any（map[string]any）：写进价格表的一行。
// 调用：UpsertPriceModel
// 测试：无直接单测
func priceRowFrom(body map[string]any, provider string) map[string]any {
	row := map[string]any{"litellm_provider": provider}
	// Start from whatever is live so editing an override twice keeps the fields
	// the form did not send. The baseline is only the fallback for a first edit.
	if prior, ok := catalog.ModelRow(str(body["id"])); ok {
		for k, v := range prior {
			row[k] = v
		}
		row["litellm_provider"] = provider
	} else if fromFile, ok := catalog.BaselineModel(str(body["id"])); ok {
		for k, v := range fromFile {
			row[k] = v
		}
		row["litellm_provider"] = provider
	}
	for _, field := range textFields {
		if v, ok := body[field]; ok {
			row[field] = v
		}
	}
	for _, field := range rateFields {
		v, ok := body[field]
		if !ok {
			continue
		}
		if n, ok := numberField(v); ok && n >= 0 {
			row[field] = n
			continue
		}
		// A cleared rate is removed rather than set to zero, so the row stops
		// being billable on that side instead of billing nothing.
		delete(row, field)
	}
	reconcilePriceRates(row, body)
	for _, field := range flagFields {
		if v, ok := body[field].(bool); ok {
			row[field] = v
		}
	}
	return row
}

// providerRowFrom builds a dropdown entry from the request. The credential
// fields are the two every supplier needs: an API base and an API key.
// 参数 body（map[string]any）：请求正文；slug（string）：供应商标识。
// 返回 map[string]any（map[string]any）：写进下拉列表的一项。
// 调用：UpsertPriceProvider
// 测试：无直接单测
func providerRowFrom(body map[string]any, slug string) map[string]any {
	display := strings.TrimSpace(str(body["provider_display_name"]))
	if display == "" {
		display = strings.TrimSpace(str(body["provider"]))
	}
	if display == "" {
		display = slug
	}
	apiBase := strings.TrimSpace(str(body["default_api_base"]))
	fields := []any{
		providerField("api_base", "API Base", "text", false, apiBase),
		providerField("api_key", display+" API Key", "password", true, ""),
	}
	if raw, ok := body["credential_fields"].([]any); ok && len(raw) > 0 {
		fields = raw
	}
	return map[string]any{
		"provider":                  display,
		"provider_display_name":     display,
		"litellm_provider":          slug,
		"credential_fields":         fields,
		"default_model_placeholder": str(body["default_model_placeholder"]),
		"default_api_base":          apiBase,
	}
}

// providerField is one credential input on a supplier entry.
// 参数 key（string）：字段名；label（string）：控制台显示的标题；kind（string）：输入类型；required（bool）：是否必填；def（string）：默认值。
// 返回 map[string]any（map[string]any）：一项凭据字段。
// 调用：providerRowFrom
// 测试：无直接单测
func providerField(key, label, kind string, required bool, def string) map[string]any {
	var defaultValue any
	if def != "" {
		defaultValue = def
	}
	return map[string]any{
		"key":           key,
		"label":         label,
		"placeholder":   nil,
		"tooltip":       nil,
		"required":      required,
		"field_type":    kind,
		"options":       nil,
		"default_value": defaultValue,
	}
}

// numberField reads a JSON number. A numeric string is accepted because the
// console posts form values as text.
// 参数 v（any）：从 JSON 里读到的值。控制台把表单数字当成文本提交，所以字符串也要能解析。
// 返回 float64（float64）：解析出的数。不合法时为 0，同时布尔值为假；bool（bool）：值是数字或数字字符串时为真。空串和其他类型为假。
// 调用：读取控制台提交的价格字段。
// 测试：无直接单测
func numberField(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, !math.IsNaN(n) && !math.IsInf(n, 0)
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case string:
		s := strings.TrimSpace(n)
		if s == "" {
			return 0, false
		}
		out, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return 0, false
		}
		return out, !math.IsNaN(out) && !math.IsInf(out, 0)
	default:
		return 0, false
	}
}

// Editing flat prices replaces their unqualified rate entries, while preserving
// catalog variants (resolution, thinking, batch) the flat form cannot express.
// 参数 row（map[string]any）：已合并旧目录行和平面字段的新行；patch（map[string]any）：本次请求实际提交的字段。
// 返回：无。只替换 patch 触及的平面费率，并在 peak 变化时修正对应基础费率的窗口。
// 调用：priceRowFrom。
// 测试：price_test.go。
func reconcilePriceRates(row, patch map[string]any) {
	prior, ok := catalog.DecodeRates(row["rates"])
	if !ok {
		return
	}
	type flatSpec struct {
		measure, side, variant, window string
	}
	specs := map[string]flatSpec{
		"input_cost_per_token":            {"token", "input", "uncached", "offpeak"},
		"output_cost_per_token":           {"token", "output", "", "offpeak"},
		"input_cost_per_token_peak":       {"token", "input", "uncached", "peak"},
		"output_cost_per_token_peak":      {"token", "output", "", "peak"},
		"cache_read_input_token_cost":     {"token", "cache_read", "", "all"},
		"cache_creation_input_token_cost": {"token", "cache_write", "", "all"},
		"input_cost_per_image":            {"picture", "input", "", "all"},
		"output_cost_per_image":           {"picture", "output", "", "all"},
		"input_cost_per_second":           {"second", "input", "", "all"},
		"output_cost_per_second":          {"second", "output", "", "all"},
		"search_context_cost_per_query":   {"query", "output", "", "all"},
	}
	touched := map[string]flatSpec{}
	for field, spec := range specs {
		if _, exists := patch[field]; exists {
			touched[field] = spec
		}
	}
	if len(touched) == 0 {
		return
	}

	rates := make([]catalog.Rate, 0, len(prior)+len(touched))
	for _, rate := range prior {
		replace := false
		for _, spec := range touched {
			if rate.Measure != spec.measure || rate.Side != spec.side || rate.Variant != spec.variant {
				continue
			}
			if rate.Window == spec.window || (spec.window == "offpeak" && rate.Window == "all") {
				replace = true
				break
			}
		}
		if !replace {
			rates = append(rates, rate)
		}
	}
	for field, spec := range touched {
		usd, valid := numberField(row[field])
		if !valid || usd < 0 {
			continue
		}
		rates = append(rates, catalog.Rate{
			Measure: spec.measure, UnitSize: 1, Side: spec.side, Variant: spec.variant,
			Window: spec.window, SourceKey: field, USD: usd,
		})
	}

	// A base token price applies all day unless the same side still has a peak
	// price. Adding or clearing only the peak field must therefore adjust the
	// base window without removing any other window or qualified variant.
	for _, side := range []struct{ side, variant string }{{"input", "uncached"}, {"output", ""}} {
		hasPeak := false
		for _, rate := range rates {
			if rate.Measure == "token" && rate.Side == side.side && rate.Variant == side.variant && rate.Window == "peak" {
				hasPeak = true
				break
			}
		}
		for i := range rates {
			rate := &rates[i]
			if rate.Measure != "token" || rate.Side != side.side || rate.Variant != side.variant {
				continue
			}
			if hasPeak && rate.Window == "all" {
				rate.Window = "offpeak"
			} else if !hasPeak && rate.Window == "offpeak" {
				rate.Window = "all"
			}
		}
	}
	row["rates"] = rates
}
