// public_hub.go serves the signed-out model hub from the built-in price map.
// It does not list deployments and does not require a key.

package gateway

import (
	"encoding/json"
	"github.com/sunqirui1987/xhub/internal/pagination"
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
// 参数 w（http.ResponseWriter）：调用方的 HTTP 响应，状态码和正文写在这里；r（*http.Request）：入站 HTTP 请求，用来读路径、头和正文。
// 调用：仅在 public_hub.go 内使用
// 测试：无直接单测
// 返回：无。状态码和正文写进调用方的响应。
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
	// 超大页码返回空页，避免整数溢出造成切片崩溃。
	start, end := pagination.Bounds(total, page, size)
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
// 参数 w（http.ResponseWriter）：调用方的 HTTP 响应，状态码和正文写在这里；r（*http.Request）：入站 HTTP 请求，用来读路径、头和正文。
// 调用：仅在 public_hub.go 内使用
// 测试：无直接单测
// 返回：无。状态码和正文写进调用方的响应。
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
// 参数 w（http.ResponseWriter）：调用方的 HTTP 响应，状态码和正文写在这里；r（*http.Request）：入站 HTTP 请求，用来读路径、头和正文。
// 调用：仅在 public_hub.go 内使用
// 测试：无直接单测
// 返回：无。状态码和正文写进调用方的响应。
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
// 参数 links（map[string]any）：公开Hub信息读到的 JSON 对象。缺键表示没有该字段。
// 返回 map[string]any（map[string]any）：公开Hub信息的字段表。缺键表示上游或库里没有这个字段。
// 调用：仅在 public_hub.go 内使用
// 测试：无直接单测
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
// 参数 w（http.ResponseWriter）：调用方的 HTTP 响应，状态码和正文写在这里；r（*http.Request）：入站 HTTP 请求，用来读路径、头和正文。
// 调用：仅在 public_hub.go 内使用
// 测试：无直接单测
// 返回：无。状态码和正文写进调用方的响应。
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
	// 超大页码返回空页，避免整数溢出造成切片崩溃。
	start, end := pagination.Bounds(total, page, size)
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
// 参数 r（*http.Request）：入站 HTTP 请求，用来读路径、头和正文。
// 返回 []publicModelRow（[]publicModelRow）：按 hub 查询条件滤过的公开模型行。没有命中时为空切片。
// 调用：仅在 public_hub.go 内使用
// 测试：无直接单测
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
// 参数 name（string）：公开行要查找或展示的名称。空串表示还没有命名；info（map[string]any）：一行价格或模型字段。缺键表示价目表没有这项。
// 返回 publicModelRow（publicModelRow）：由模型名和价格字段拼出的一条公开模型行，含供应商和模式。
// 调用：仅在 public_hub.go 内使用
// 测试：无直接单测
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
// 参数 rows（[]publicModelRow）：从用量或目录读出的publicModelRow；sortParam（string）：排序公开模型使用的排序参数。空串表示调用方没有提供这项。
// 返回：无。公开模型行已按请求的字段排序。字段以 - 开头时降序，缺省按 model_group。
// 调用：仅在 public_hub.go 内使用
// 测试：无直接单测
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
// 参数 a（publicModelRow）：排序时的前一行；b（publicModelRow）：排序时的后一行；field（string）：比较的字段，例如 mode 或 input_cost_per_token。
// 返回 int（int）：a 小于 b 时为负，相等时为 0，a 大于 b 时为正。不认识的字段按 model_group 比较。
// 调用：仅在 public_hub.go 内使用
// 测试：无直接单测
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
// 参数 a（any）：价格表里的一个数字字段；b（any）：同一字段的另一个值。不是数字时按 0 比较。
// 返回 int（int）：a 小于 b 时为 -1，大于时为 1，相等时为 0。
// 调用：仅在 public_hub.go 内使用
// 测试：无直接单测
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
// 参数 v（any）：JSON 里读出的动态值。数字、字符串和对象都要接住，类型不符时按零值而不是 panic。
// 返回 bool（bool）：价格表里的这个标志被打开时返回真。
// 调用：仅在 public_hub.go 内使用
// 测试：无直接单测
func truthy(v any) bool {
	b, ok := v.(bool)
	return ok && b
}

// pageQuery reads the page and page size, using def when missing and capN as the maximum size.
// 参数 r（*http.Request）：入站 HTTP 请求；def（int）：page_size 缺失时的默认条数；capN（int）：page_size 的上限。
// 返回 int（int）：页码，最小是 1；int（int）：每页条数，落在 1 和 capN 之间。
// 调用：仅在 public_hub.go 内使用
// 测试：无直接单测
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
// 参数 s（string）：要解析的十进制文本；def（int）：空串或不是整数时用的默认值。
// 返回 int（int）：解析出的整数。空串或无法解析时返回 def，不是固定的 0。
// 调用：仅在 public_hub.go 内使用
// 测试：无直接单测
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
// 参数 s（string）：拆分逗号分隔要处理的文本。空串表示这段没有内容。
// 返回 []string（[]string）：拆分逗号分隔。没有匹配时为 nil 或空切片，调用方按长度判断。
// 调用：仅在 public_hub.go 内使用
// 测试：无直接单测
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
// 参数 list（[]string）：列出列表。空切片表示没有可处理的项；v（string）：包含合并使用的值。空串表示调用方没有提供这项。
// 返回 bool（bool）：列表里有这个值（忽略大小写）时返回真。
// 调用：仅在 public_hub.go 内使用
// 测试：无直接单测
func containsFold(list []string, v string) bool {
	for _, item := range list {
		if strings.EqualFold(item, v) {
			return true
		}
	}
	return false
}

// overlapsFold reports whether any wanted value appears in have, ignoring case.
// 参数 want（[]string）：want列表。空切片表示没有可处理的项；have（[]string）：have列表。空切片表示没有可处理的项。
// 返回 bool（bool）：wanted 里至少有一个值出现在 have 中（忽略大小写）时返回真。
// 调用：仅在 public_hub.go 内使用
// 测试：无直接单测
func overlapsFold(want, have []string) bool {
	for _, h := range have {
		if containsFold(want, h) {
			return true
		}
	}
	return false
}
