// 自动路由的基准数据。没有样本时返回空列表，不编造分数。
package usage

import (
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/sunqirui1987/xhub/internal/httpx"
)

// 返回自动路由基准。没有样本时各计数为 0，不编造延迟或命中率。
func Benchmarks(s Host, w http.ResponseWriter, r *http.Request) {
	httpx.SetCallID(w, httpx.CallID())
	if s.RequireManage(w, r) == nil {
		return
	}
	end := time.Now().UTC()
	if v := strings.TrimSpace(r.URL.Query().Get("end_date")); v != "" {
		parsed, err := time.Parse("2006-01-02", v)
		if err != nil {
			httpx.WriteError(w, 400, "invalid_request", "Invalid date format: "+v+". Expected: 'YYYY-MM-DD'")
			return
		}
		end = parsed.UTC()
	}
	start := end.AddDate(0, 0, -30)
	if v := strings.TrimSpace(r.URL.Query().Get("start_date")); v != "" {
		parsed, err := time.Parse("2006-01-02", v)
		if err != nil {
			httpx.WriteError(w, 400, "invalid_request", "Invalid date format: "+v+". Expected: 'YYYY-MM-DD'")
			return
		}
		start = parsed.UTC()
	}
	if end.Before(start) {
		httpx.WriteError(w, 400, "invalid_request", "end_date must not be earlier than start_date")
		return
	}
	groups := idleRouters(s)
	httpx.WriteJSON(w, 200, map[string]any{
		"start_date":       start.Format("2006-01-02"),
		"end_date":         end.Format("2006-01-02"),
		"routers_in_scope": len(groups),
		"totals":           zeroBenchmarkTotals(),
		"groups":           groups,
	})
}

// 列出配置里的策略路由器及其空的基准桶。
func idleRouters(s Host) []map[string]any {
	list := s.ModelList()
	type key struct{ name, kind string }
	seen := map[key]struct{}{}
	var keys []key
	for _, m := range list {
		model := m.ParamString("model", "")
		kind, ok := strategyRouterKind(model)
		if !ok || m.ModelName == "" {
			continue
		}
		k := key{m.ModelName, kind}
		if _, dup := seen[k]; dup {
			continue
		}
		seen[k] = struct{}{}
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].name != keys[j].name {
			return keys[i].name < keys[j].name
		}
		return keys[i].kind < keys[j].kind
	})
	groups := make([]map[string]any, 0, len(keys))
	for _, k := range keys {
		g := zeroBenchmarkTotals()
		g["router_name"] = k.name
		g["router_type"] = k.kind
		g["tier_turns"] = map[string]any{}
		groups = append(groups, g)
	}
	return groups
}

// 从模型名识别策略路由器种类。认不出时 ok 为 false。
func strategyRouterKind(model string) (string, bool) {
	const prefix = "auto_router/"
	if !strings.HasPrefix(model, prefix) {
		return "", false
	}
	rest := model[len(prefix):]
	switch {
	case strings.HasPrefix(rest, "complexity_router"):
		return "complexity", true
	case strings.HasPrefix(rest, "adaptive_router"):
		return "adaptive", true
	case strings.HasPrefix(rest, "quality_router"):
		return "quality", true
	default:
		return "", false
	}
}

// 一个全零的缓存统计桶，供还没有流量时的响应使用。
func zeroCacheBucket() map[string]any {
	return map[string]any{"turns": 0, "hits": 0, "hit_rate_pct": 0}
}

// 一组全零的基准合计。
func zeroBenchmarkTotals() map[string]any {
	return map[string]any{
		"sessions":               0,
		"turns":                  0,
		"avg_turns_per_session":  0,
		"avg_session_seconds":    0,
		"avg_tokens_per_session": 0,
		"spend":                  0,
		"classifier_cost":        0,
		"saved_spend":            0,
		"baseline_spend":         0,
		"saved_pct":              0,
		"saved_per_session":      0,
		"cache": map[string]any{
			"coverage_pct":             0,
			"hit_rate_pct":             0,
			"same_model":               zeroCacheBucket(),
			"first_visit":              zeroCacheBucket(),
			"return_to_tier":           zeroCacheBucket(),
			"unordered_turns":          0,
			"return_misses_expired":    0,
			"return_misses_within_ttl": 0,
			"return_misses_unknown":    0,
			"ttl_5m_turns":             0,
			"ttl_1h_turns":             0,
		},
	}
}
