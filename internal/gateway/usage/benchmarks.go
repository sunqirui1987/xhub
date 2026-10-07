// Package usage serves auto-router benchmarks. With no sample it returns an empty list and does not invent scores.
package usage

import (
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/sunqirui1987/xhub/internal/httpx"
	"github.com/sunqirui1987/xhub/internal/logx"
	"sync"
)

var logTraceOnceBenchmarks sync.Once

// Benchmarks returns auto-router benchmark rows. With no sample every count is 0, and latency and hit rate are not invented.
// 参数 s（Host）：Benchmarks使用的数据面宿主；w（http.ResponseWriter）：调用方的 HTTP 响应，状态和正文写在这里；r（*http.Request）：入站 HTTP 请求。
// 返回：无。状态码和正文写进调用方的响应。
// 调用：gateway/usage/mount.go
// 测试：无直接单测
func Benchmarks(s Host, w http.ResponseWriter, r *http.Request) {
	logTraceOnceBenchmarks.Do(func() { logx.Trace("enter usage.Benchmarks") })

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

// idleRouters lists the strategy routers in the config together with an empty benchmark bucket.
// 参数 s（Host）：idleRouters使用的数据面宿主。
// 返回 []map[string]any（[]map[string]any）：一组map[string]any。没有匹配时为空切片，不是 nil 分页。
// 调用：仅在 benchmarks.go 内使用
// 测试：无直接单测
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

// strategyRouterKind reads the strategy-router kind from a model name. An unrecognized name returns ok false.
// 参数 model（string）：对外模型名，用来选部署和记用量。
// 返回 string（string）：从模型名认出的策略路由种类。认不出时为空串；bool（bool）：模型名能认出策略路由种类时返回真。认不出时返回假，同时字符串为空。
// 调用：仅在 benchmarks.go 内使用
// 测试：无直接单测
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

// zeroCacheBucket is a zeroed cache-stat bucket used when the router has no traffic yet.
// 参数：无。
// 返回 map[string]any（map[string]any）：zero缓存Bucket的字段表。缺键表示上游或库里没有这个字段。
// 调用：仅在 benchmarks.go 内使用
// 测试：无直接单测
func zeroCacheBucket() map[string]any {
	return map[string]any{"turns": 0, "hits": 0, "hit_rate_pct": 0}
}

// zeroBenchmarkTotals is a zeroed benchmark total used before any sample exists.
// 参数：无。
// 返回 map[string]any（map[string]any）：zeroBenchmarkTotals的字段表。缺键表示上游或库里没有这个字段。
// 调用：仅在 benchmarks.go 内使用
// 测试：无直接单测
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
