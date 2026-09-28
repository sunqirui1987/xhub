// Package gateway serves the signed-out public model hub from the built-in price map.
package gateway

import (
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/sunqirui1987/xhub/internal/catalog"
	"github.com/sunqirui1987/xhub/internal/httpx"
	"github.com/sunqirui1987/xhub/internal/logx"
	"sync"
)

type publicModelRow struct {
	ModelGroup                   string   `json:"model_group"`
	Providers                    []string `json:"providers"`
	Mode                         string   `json:"mode"`
	MaxInputTokens               any      `json:"max_input_tokens"`
	MaxOutputTokens              any      `json:"max_output_tokens"`
	InputCostPerToken            any      `json:"input_cost_per_token"`
	OutputCostPerToken           any      `json:"output_cost_per_token"`
	RPM                          any      `json:"rpm"`
	TPM                          any      `json:"tpm"`
	IsPublicModelGroup           bool     `json:"is_public_model_group"`
	SupportsFunctionCalling      bool     `json:"supports_function_calling"`
	SupportsParallelFunctionCall bool     `json:"supports_parallel_function_calling"`
	SupportsVision               bool     `json:"supports_vision"`
	SupportedOpenAIParams        []string `json:"supported_openai_params"`
}

var logTraceOncePublicHub sync.Once

// publicModelHub returns the paged list the signed-out model hub reads. The rows come from the built-in price map.
func (s *Server) publicModelHub(w http.ResponseWriter, r *http.Request) {
	logTraceOncePublicHub.Do(func() { logx.Trace("enter gateway.publicModelHub") })

	rows := filterPublicModels(r)
	page, size := pageQuery(r, 50, 100)
	total := len(rows)
	pages := 1
	if size > 0 {
		pages = (total + size - 1) / size
		if pages < 1 {
			pages = 1
		}
	}
	start := (page - 1) * size
	if start > total {
		start = total
	}
	end := start + size
	if end > total {
		end = total
	}
	slice := rows[start:end]
	if slice == nil {
		slice = []publicModelRow{}
	}
	httpx.WriteJSON(w, 200, map[string]any{
		"data": slice,
		"links": map[string]any{
			"self": r.URL.RequestURI(),
			"next": nil,
			"prev": nil,
		},
		"meta": map[string]any{
			"page":        page,
			"page_size":   size,
			"total_count": total,
			"total_pages": pages,
		},
	})
}

// publicModelHubInfo returns the About block the public hub reads. Useful links come from the stored override when one exists.
func (s *Server) publicModelHubInfo(w http.ResponseWriter, r *http.Request) {
	links := map[string]any{}
	if saved, err := s.Store.GetKV("model_hub", "info"); err == nil {
		if raw, ok := saved["useful_links"].(map[string]any); ok {
			links = raw
		}
	}
	httpx.WriteJSON(w, 200, publicHubInfo(links))
}

// updateUsefulLinks stores the About links shown on the public model hub.
func (s *Server) updateUsefulLinks(w http.ResponseWriter, r *http.Request) {
	if s.requireManage(w, r) == nil {
		return
	}
	var body map[string]any
	_ = json.NewDecoder(r.Body).Decode(&body)
	links := map[string]any{}
	if raw, ok := body["useful_links"].(map[string]any); ok {
		links = raw
	}
	payload, _ := json.Marshal(map[string]any{"useful_links": links})
	if err := s.Store.PutKV("model_hub", "info", string(payload)); err != nil {
		httpx.WriteError(w, 500, "internal", err.Error())
		return
	}
	httpx.WriteJSON(w, 200, publicHubInfo(links))
}

// publicHubInfo is the JSON the public hub About section reads.
func publicHubInfo(links map[string]any) map[string]any {
	if links == nil {
		links = map[string]any{}
	}
	return map[string]any{
		"docs_title":              "XHub",
		"custom_docs_description": "Proxy Server to call 100+ LLMs in the OpenAI format.",
		"litellm_version":         Version,
		"useful_links":            links,
	}
}

// publicModelHubFacet returns the providers, modes, or capabilities used by the filter dropdowns.
func (s *Server) publicModelHubFacet(w http.ResponseWriter, r *http.Request) {
	facet := r.PathValue("facet")
	rows := filterPublicModels(r)
	seen := map[string]struct{}{}
	var values []string
	add := func(v string) {
		v = strings.TrimSpace(v)
		if v == "" {
			return
		}
		if _, ok := seen[v]; ok {
			return
		}
		seen[v] = struct{}{}
		values = append(values, v)
	}
	for _, row := range rows {
		switch facet {
		case "providers":
			for _, p := range row.Providers {
				add(p)
			}
		case "modes":
			add(row.Mode)
		case "features":
			if row.SupportsVision {
				add("vision")
			}
			if row.SupportsFunctionCalling {
				add("function_calling")
			}
			if row.SupportsParallelFunctionCall {
				add("parallel_function_calling")
			}
		default:
			httpx.WriteJSON(w, 200, map[string]any{
				"data": []string{},
				"links": map[string]any{
					"self": r.URL.RequestURI(),
					"next": nil,
					"prev": nil,
				},
				"meta": map[string]any{
					"has_more":  false,
					"page":      1,
					"page_size": 0,
					"facet":     facet,
				},
			})
			return
		}
	}
	sort.Strings(values)
	page, size := pageQuery(r, 100, 100)
	total := len(values)
	start := (page - 1) * size
	if start > total {
		start = total
	}
	end := start + size
	if end > total {
		end = total
	}
	slice := values[start:end]
	if slice == nil {
		slice = []string{}
	}
	httpx.WriteJSON(w, 200, map[string]any{
		"data": slice,
		"links": map[string]any{
			"self": r.URL.RequestURI(),
			"next": nil,
			"prev": nil,
		},
		"meta": map[string]any{
			"has_more":  end < total,
			"page":      page,
			"page_size": size,
		},
	})
}

// filterPublicModels applies the hub query filters to rows built from the price map.
func filterPublicModels(r *http.Request) []publicModelRow {
	q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	modes := splitCSV(r.URL.Query().Get("filter[mode][in]"))
	providers := splitCSV(r.URL.Query().Get("filter[providers][in]"))
	var rows []publicModelRow
	for name, info := range catalog.CostMap() {
		if name == "sample_spec" {
			continue
		}
		row := publicRow(name, info)
		if q != "" && !strings.Contains(strings.ToLower(row.ModelGroup), q) {
			continue
		}
		if len(modes) > 0 && !containsFold(modes, row.Mode) {
			continue
		}
		if len(providers) > 0 && !overlapsFold(providers, row.Providers) {
			continue
		}
		rows = append(rows, row)
	}
	sortPublicModels(rows, r.URL.Query().Get("sort"))
	return rows
}

// publicRow builds one hub row from a model name and its price-map fields.
func publicRow(name string, info map[string]any) publicModelRow {
	prov, _ := info["litellm_provider"].(string)
	providers := []string{}
	if prov != "" {
		providers = []string{prov}
	}
	mode, _ := info["mode"].(string)
	if mode == "" {
		mode = "chat"
	}
	return publicModelRow{
		ModelGroup:                   name,
		Providers:                    providers,
		Mode:                         mode,
		MaxInputTokens:               info["max_input_tokens"],
		MaxOutputTokens:              info["max_output_tokens"],
		InputCostPerToken:            info["input_cost_per_token"],
		OutputCostPerToken:           info["output_cost_per_token"],
		RPM:                          info["rpm"],
		TPM:                          info["tpm"],
		IsPublicModelGroup:           true,
		SupportsFunctionCalling:      truthy(info["supports_function_calling"]),
		SupportsParallelFunctionCall: truthy(info["supports_parallel_function_calling"]),
		SupportsVision:               truthy(info["supports_vision"]),
		SupportedOpenAIParams:        []string{},
	}
}

// sortPublicModels orders hub rows by the requested field.
func sortPublicModels(rows []publicModelRow, sortParam string) {
	desc := strings.HasPrefix(sortParam, "-")
	field := strings.TrimPrefix(sortParam, "-")
	if field == "" {
		field = "model_group"
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].ModelGroup == rows[j].ModelGroup {
			return false
		}
		c := publicCmp(rows[i], rows[j], field)
		if c == 0 {
			if desc {
				return rows[i].ModelGroup > rows[j].ModelGroup
			}
			return rows[i].ModelGroup < rows[j].ModelGroup
		}
		if desc {
			return c > 0
		}
		return c < 0
	})
}

// publicCmp compares two hub rows on one field.
func publicCmp(a, b publicModelRow, field string) int {
	switch field {
	case "mode":
		return strings.Compare(a.Mode, b.Mode)
	case "input_cost_per_token":
		return cmpNum(a.InputCostPerToken, b.InputCostPerToken)
	case "output_cost_per_token":
		return cmpNum(a.OutputCostPerToken, b.OutputCostPerToken)
	case "max_input_tokens":
		return cmpNum(a.MaxInputTokens, b.MaxInputTokens)
	case "max_output_tokens":
		return cmpNum(a.MaxOutputTokens, b.MaxOutputTokens)
	default:
		return strings.Compare(a.ModelGroup, b.ModelGroup)
	}
}

// cmpNum compares two numeric price-map values.
func cmpNum(a, b any) int {
	af, bf := asFloat(a), asFloat(b)
	switch {
	case af < bf:
		return -1
	case af > bf:
		return 1
	default:
		return 0
	}
}

// truthy reports whether a price-map flag is set.
func truthy(v any) bool {
	b, ok := v.(bool)
	return ok && b
}

// pageQuery reads the page and page size, using def when missing and capN as the maximum size.
func pageQuery(r *http.Request, def, capN int) (int, int) {
	page := atoiDefault(r.URL.Query().Get("page"), 1)
	if page < 1 {
		page = 1
	}
	size := atoiDefault(r.URL.Query().Get("page_size"), def)
	if size < 1 {
		size = def
	}
	if size > capN {
		size = capN
	}
	return page, size
}

// atoiDefault parses an integer and returns def when the text is not a number.
func atoiDefault(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}

// splitCSV splits a comma-separated query value and drops empty pieces.
func splitCSV(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// containsFold reports whether list contains v, ignoring case.
func containsFold(list []string, v string) bool {
	for _, item := range list {
		if strings.EqualFold(item, v) {
			return true
		}
	}
	return false
}

// overlapsFold reports whether any wanted value appears in have, ignoring case.
func overlapsFold(want, have []string) bool {
	for _, h := range have {
		if containsFold(want, h) {
			return true
		}
	}
	return false
}
