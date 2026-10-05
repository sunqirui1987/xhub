package models

import (
	"net/http"
	"strings"

	"github.com/sunqirui1987/xhub/internal/catalog"
	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/httpx"
	"github.com/sunqirui1987/xhub/internal/logx"
)

// Available serves model cards for the signed-in caller. It exposes only display metadata.
// A view-only session may read the models granted to it. Calling a model stays on AllowLLM.
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
		card["category"] = "other"
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

func availableNumber(primary, fallback map[string]any, key string) any {
	if value, ok := primary[key]; ok && number(value) > 0 {
		return value
	}
	if value, ok := fallback[key]; ok && number(value) > 0 {
		return value
	}
	return nil
}

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
func boolValue(value any) bool { b, _ := value.(bool); return b }
