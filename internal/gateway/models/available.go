package models

import (
	"net/http"
	"strings"

	"github.com/sunqirui1987/xhub/internal/catalog"
	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/httpx"
	"github.com/sunqirui1987/xhub/internal/logx"
)

// Available serves model cards for the signed-in caller. It exposes only display metadata. A view-only session may read the models granted to it. Calling a model stays on AllowLLM.
// 参数 s（Host）：可用使用的数据面宿主；w（http.ResponseWriter）：调用方的 HTTP 响应，状态和正文写在这里；r（*http.Request）：入站 HTTP 请求。
// 返回：无。状态码和正文写进调用方的响应。
// 调用：gateway/models/mount.go
// 测试：无直接单测
func Available(s Host, w http.ResponseWriter, r *http.Request) {
	logx.Trace("enter models.Available")
	p, err := s.Resolve(r)
	if err != nil || p == nil {
		httpx.WriteTypedError(w, r.URL.Path, 401, "invalid_api_key", "Authentication Error, No api key passed in.")
		return
	}
	s.LockModels()
	list := append([]config.ModelEntry(nil), (*s.ModelTable())...)
	s.UnlockModels()
	teamID := r.URL.Query().Get("team_id")
	ctx := r.Context()
	data := make([]map[string]any, 0)
	seen := map[string]struct{}{}
	for _, entry := range list {
		if nonModelEntry(entry) || modelBlocked(entry) || !AllowsModel(s, ctx, p, teamID, entry.ModelName) {
			continue
		}
		if _, ok := seen[entry.ModelName]; ok {
			continue
		}
		seen[entry.ModelName] = struct{}{}
		data = append(data, availableCard(entry))
	}
	httpx.WriteJSON(w, 200, map[string]any{"object": "list", "data": data})
}

// 把一条部署收成可选模型卡片，价格优先用部署覆盖。
// 参数 entry（config.ModelEntry）：一条模型部署，含对外名、供应商参数和价格覆盖。
// 返回 map[string]any（map[string]any）：可用卡片的字段表。缺键表示上游或库里没有这个字段。
// 调用：仅在 available.go 内使用
// 测试：无直接单测
func availableCard(entry config.ModelEntry) map[string]any {
	info := entry.ModelInfo
	row := availableCostRow(entry.ModelName)
	card := map[string]any{
		"id":                entry.ModelName,
		"provider":          firstString(info, row, "litellm_provider", "provider"),
		"category":          firstString(info, row, "category", "mode"),
		"capabilities":      []string{},
		"max_input_tokens":  availableNumber(info, row, "max_input_tokens"),
		"max_output_tokens": availableNumber(info, row, "max_output_tokens"),
		"input_price":       firstPrice(info, row, "input_price", "input_cost_per_token"),
		"output_price":      firstPrice(info, row, "output_price", "output_cost_per_token"),
		"cache_read_price":  firstPrice(info, row, "cache_read_price", "cache_read_input_token_cost"),
		"cache_write_price": firstPrice(info, row, "cache_write_price", "cache_creation_input_token_cost"),
	}
	if card["provider"] == "" {
		card["provider"] = firstString(entry.LiteLLMParams, nil, "custom_llm_provider")
	}
	if card["category"] == "" {
		// A deployment that never set a mode is a chat model. The playground
		// hides anything that is not chat from the chat endpoint picker.
		card["category"] = "chat"
	}
	for _, capability := range []struct{ field, name string }{
		{"supports_function_calling", "tools"}, {"supports_response_schema", "structured"},
		{"supports_reasoning", "reasoning"}, {"supports_vision", "vision"}, {"supports_prompt_caching", "caching"},
	} {
		if boolValue(info[capability.field]) || boolValue(row[capability.field]) {
			card["capabilities"] = append(card["capabilities"].([]string), capability.name)
		}
	}
	return card
}

// 从内置价目表取出这个模型的价格行。带前缀找不到时再试去掉前缀。
// 参数 id（string）：可用费用行使用的主键。空串表示调用方没有指定记录。
// 返回 map[string]any（map[string]any）：可用费用行的字段表。缺键表示上游或库里没有这个字段。
// 调用：仅在 available.go 内使用
// 测试：无直接单测
func availableCostRow(id string) map[string]any {
	prices := catalog.CostMap()
	if row := prices[id]; row != nil {
		return row
	}
	if i := strings.LastIndex(id, "/"); i >= 0 {
		return prices[id[i+1:]]
	}
	return nil
}

// 按键的顺序从优先表和备用表里取第一个非空字符串。
// 参数 primary（map[string]any）：优先读取的字段表。没有该键时再看备用表；fallback（map[string]any）：缺值或解析失败时用的默认；keys（...string）：首个字符串使用的string。
// 返回 string（string）：按字符串读出的值。不是字符串或没有该键时为空串，不 panic。
// 调用：仅在 available.go 内使用。
// 测试：无直接单测
func firstString(primary, fallback map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := primary[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
		if value, ok := fallback[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

// 从优先表或备用表取一个大于 0 的数字。
// 参数 primary（map[string]any）：优先读取的字段表。没有该键时再看备用表；fallback（map[string]any）：缺值或解析失败时用的默认；key（string）：上游或调用方的密钥。空串表示还不能转发或还没有密钥。
// 返回 any（any）：找到的价格或数字。没有合格值时为 nil，调用方不要显示成 0。
// 调用：仅在 available.go 内使用
// 测试：无直接单测
func availableNumber(primary, fallback map[string]any, key string) any {
	if value, ok := primary[key]; ok && number(value) > 0 {
		return value
	}
	if value, ok := fallback[key]; ok && number(value) > 0 {
		return value
	}
	return nil
}

// 从优先表或备用表取价格。0 也算有效价格。
// 参数 primary（map[string]any）：优先读取的字段表。没有该键时再看备用表；fallback（map[string]any）：缺值或解析失败时用的默认；direct（string）：首个价格使用的direct。空串表示调用方没有提供这项；perToken（string）：首个价格使用的每令牌。空串表示调用方没有提供这项。
// 返回 any（any）：找到的价格或数字。没有合格值时为 nil，调用方不要显示成 0。
// 调用：仅在 available.go 内使用
// 测试：无直接单测
func firstPrice(primary, fallback map[string]any, direct, perToken string) any {
	if value, ok := primary[direct]; ok && number(value) >= 0 {
		return value
	}
	if value, ok := fallback[direct]; ok && number(value) >= 0 {
		return value
	}
	if value, ok := fallback[perToken]; ok && number(value) >= 0 {
		return number(value) * 1000000
	}
	return nil
}

// 把 JSON 数字收成 float64。类型不符时为 0。
// 参数 value（any）：JSON 里读出的动态值。数字、字符串和对象都要接住，类型不符时按零值而不是 panic。
// 返回 float64（float64）：数字。没有计数或类型不符时为 0。
// 调用：仅在 available.go 内使用
// 测试：无直接单测
func number(value any) float64 {
	switch n := value.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case int64:
		return float64(n)
	default:
		return -1
	}
}

// 把动态值收成布尔。不是布尔时为假。
// 参数 value（any）：JSON 里读出的动态值。数字、字符串和对象都要接住，类型不符时按零值而不是 panic。
// 返回 bool（bool）：这个动态值本身是布尔真时返回真。不是布尔时返回假。
// 调用：仅在 available.go 内使用
// 测试：无直接单测
func boolValue(value any) bool { b, _ := value.(bool); return b }
