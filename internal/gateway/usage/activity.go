package usage

import (
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/sunqirui1987/xhub/internal/catalog"
	"github.com/sunqirui1987/xhub/internal/httpx"
	"github.com/sunqirui1987/xhub/internal/logx"
	"github.com/sunqirui1987/xhub/internal/store"
	"sync"
)

const activityPageSize = 50

var logTraceOnceActivity sync.Once

// UserDailyActivity is GET /user/daily/activity. It groups stored spend logs into the daily rollup the usage page reads.
func UserDailyActivity(s Host, w http.ResponseWriter, r *http.Request) {
	logTraceOnceActivity.Do(func() { logx.Trace("enter usage.UserDailyActivity") })

	if s.RequireManage(w, r) == nil {
		return
	}
	httpx.WriteJSON(w, 200, dailyActivityResponse(loadActivity(s, r), pageFromQuery(r), false))
}

// UserDailyActivityAggregated is GET /user/daily/activity/aggregated. Same rollup, returned as one page.
func UserDailyActivityAggregated(s Host, w http.ResponseWriter, r *http.Request) {
	if s.RequireManage(w, r) == nil {
		return
	}
	httpx.WriteJSON(w, 200, dailyActivityResponse(loadActivity(s, r), 1, true))
}

// GatewayDailyActivity is GET /gateway/daily/activity. Request counts come from the same spend logs, split by outcome and route.
func GatewayDailyActivity(s Host, w http.ResponseWriter, r *http.Request) {
	if s.RequireManage(w, r) == nil {
		return
	}
	httpx.WriteJSON(w, 200, gatewayActivityBody(loadActivity(s, r)))
}

type activityRow struct {
	day        string
	model      string
	provider   string
	apiKey     string
	keyAlias   string
	teamID     string
	userID     string
	route      string
	prompt     int
	completion int
	spend      float64
	success    bool
}

type metric struct {
	spend      float64
	prompt     int
	completion int
	requests   int
	success    int
	failed     int
}

func (m *metric) add(prompt, completion int, spend float64, success bool) {
	m.spend += spend
	m.prompt += prompt
	m.completion += completion
	m.requests++
	if success {
		m.success++
		return
	}
	m.failed++
}

func (m *metric) fold(other metric) {
	m.spend += other.spend
	m.prompt += other.prompt
	m.completion += other.completion
	m.requests += other.requests
	m.success += other.success
	m.failed += other.failed
}

func (m metric) json() map[string]any {
	return map[string]any{
		"spend":                                  m.spend,
		"flat_cost":                              0.0,
		"prompt_tokens":                          m.prompt,
		"completion_tokens":                      m.completion,
		"total_tokens":                           m.prompt + m.completion,
		"api_requests":                           m.requests,
		"successful_requests":                    m.success,
		"failed_requests":                        m.failed,
		"cache_read_input_tokens":                0,
		"cache_creation_input_tokens":            0,
		"compression_saved_tokens":               0,
		"compression_savings_spend":              0.0,
		"prompt_caching_savings_spend":           0.0,
		"gateway_injected_caching_savings_spend": 0.0,
		"autorouter_savings_spend":               0.0,
	}
}

type keyMetric struct {
	metric
	alias  string
	teamID string
	userID string
}

type namedMetric struct {
	metric
	keys map[string]*keyMetric
}

type dayMetric struct {
	metric
	models    map[string]*namedMetric
	groups    map[string]*namedMetric
	providers map[string]*namedMetric
	endpoints map[string]*namedMetric
	keys      map[string]*keyMetric
	routes    map[string]*metric
}

func loadActivity(s Host, r *http.Request) []activityRow {
	list, _ := s.DB().ListSpendLogs()
	keys, _ := s.DB().ListKeys()
	return selectActivity(list, keys, r.URL.Query().Get("start_date"), r.URL.Query().Get("end_date"), r.URL.Query().Get("timezone"), r.URL.Query().Get("user_id"), r.URL.Query().Get("api_key"))
}

func selectActivity(list []map[string]any, keys []store.Key, start, end, timezone, wantUser, wantKey string) []activityRow {
	byHash := map[string]store.Key{}
	for _, k := range keys {
		byHash[k.TokenHash] = k
	}
	offsetMin, _ := strconv.Atoi(timezone)
	prices := catalog.CostMap()
	out := make([]activityRow, 0, len(list))
	for _, row := range list {
		apiKey := str(row["api_key"])
		owner := byHash[apiKey]
		if wantKey != "" && apiKey != wantKey {
			continue
		}
		ownerID := str(row["user_id"])
		if ownerID == "" {
			ownerID = owner.UserID
		}
		if wantUser != "" && ownerID != wantUser {
			continue
		}
		day := activityDay(str(row["startTime"]), offsetMin)
		if !dayInRange(day, start, end) {
			continue
		}
		model := str(row["model"])
		if model == "" {
			model = "unknown"
		}
		status := str(row["status"])
		out = append(out, activityRow{
			day:        day,
			model:      model,
			provider:   providerName(model, prices),
			apiKey:     apiKey,
			keyAlias:   owner.KeyAlias,
			teamID:     owner.TeamID,
			userID:     ownerID,
			route:      llmRoute(str(row["call_type"])),
			prompt:     asInt(row["prompt_tokens"]),
			completion: asInt(row["completion_tokens"]),
			spend:      asFloat(row["spend"]),
			success:    status == "" || status == "success" || status == "succeeded",
		})
	}
	return out
}

func dailyActivityResponse(rows []activityRow, page int, aggregated bool) map[string]any {
	days := rollupDays(rows)
	names := dayNames(days, true)
	totalPages := 1
	selected := names
	if !aggregated {
		page, selected, totalPages = pageDays(names, page)
	} else {
		page = 1
	}
	results := make([]any, 0, len(selected))
	var total metric
	for _, name := range selected {
		day := days[name]
		total.fold(day.metric)
		results = append(results, map[string]any{
			"date":    name,
			"metrics": day.metric.json(),
			"breakdown": map[string]any{
				"models":       namedJSON(day.models),
				"model_groups": namedJSON(day.groups),
				"providers":    namedJSON(day.providers),
				"api_keys":     keysJSON(day.keys),
				"endpoints":    namedJSON(day.endpoints),
				"mcp_servers":  map[string]any{},
				"entities":     map[string]any{},
			},
		})
	}
	return map[string]any{
		"results":  results,
		"metadata": metadataJSON(total, page, totalPages),
	}
}

func gatewayActivityBody(rows []activityRow) map[string]any {
	days := rollupDays(rows)
	names := dayNames(days, false)
	byDate := make([]any, 0, len(names))
	routes := map[string]*metric{}
	var success, failed int
	for _, name := range names {
		day := days[name]
		success += day.success
		failed += day.failed
		byDate = append(byDate, map[string]any{
			"date":                name,
			"successful_requests": day.success,
			"failed_requests":     day.failed,
		})
		for route, m := range day.routes {
			bucket := routes[route]
			if bucket == nil {
				bucket = &metric{}
				routes[route] = bucket
			}
			bucket.success += m.success
			bucket.failed += m.failed
		}
	}
	routeNames := make([]string, 0, len(routes))
	for name := range routes {
		routeNames = append(routeNames, name)
	}
	sort.Slice(routeNames, func(i, j int) bool {
		if routes[routeNames[i]].success != routes[routeNames[j]].success {
			return routes[routeNames[i]].success > routes[routeNames[j]].success
		}
		return routeNames[i] < routeNames[j]
	})
	byRoute := make([]any, 0, len(routeNames))
	for _, name := range routeNames {
		m := routes[name]
		byRoute = append(byRoute, map[string]any{
			"category":            "llm",
			"route":               name,
			"successful_requests": m.success,
			"failed_requests":     m.failed,
		})
	}
	return map[string]any{
		"total_successful_requests": success,
		"total_failed_requests":     failed,
		"by_date":                   byDate,
		"by_route":                  byRoute,
	}
}

func dayNames(days map[string]*dayMetric, newestFirst bool) []string {
	names := make([]string, 0, len(days))
	for name := range days {
		names = append(names, name)
	}
	if newestFirst {
		sort.Sort(sort.Reverse(sort.StringSlice(names)))
		return names
	}
	sort.Strings(names)
	return names
}

func pageDays(names []string, page int) (int, []string, int) {
	if page < 1 {
		page = 1
	}
	totalPages := 1
	if n := len(names); n > 0 {
		totalPages = (n + activityPageSize - 1) / activityPageSize
	}
	if page > totalPages {
		return page, []string{}, totalPages
	}
	start := (page - 1) * activityPageSize
	end := start + activityPageSize
	if end > len(names) {
		end = len(names)
	}
	return page, names[start:end], totalPages
}

func pageFromQuery(r *http.Request) int {
	n, err := strconv.Atoi(r.URL.Query().Get("page"))
	if err != nil || n < 1 {
		return 1
	}
	return n
}

func rollupDays(rows []activityRow) map[string]*dayMetric {
	days := map[string]*dayMetric{}
	for _, row := range rows {
		day := days[row.day]
		if day == nil {
			day = &dayMetric{
				models:    map[string]*namedMetric{},
				groups:    map[string]*namedMetric{},
				providers: map[string]*namedMetric{},
				endpoints: map[string]*namedMetric{},
				keys:      map[string]*keyMetric{},
				routes:    map[string]*metric{},
			}
			days[row.day] = day
		}
		day.add(row.prompt, row.completion, row.spend, row.success)
		addNamed(day.models, row.model, row)
		addNamed(day.groups, row.model, row)
		addNamed(day.providers, row.provider, row)
		addNamed(day.endpoints, row.route, row)
		// An empty hash is still a key the usage page must list. Dropping it
		// hides every master-key call from the key tab.
		addKey(day.keys, row.apiKey, row)
		route := day.routes[row.route]
		if route == nil {
			route = &metric{}
			day.routes[row.route] = route
		}
		route.add(row.prompt, row.completion, row.spend, row.success)
	}
	return days
}

func addNamed(into map[string]*namedMetric, name string, row activityRow) {
	bucket := into[name]
	if bucket == nil {
		bucket = &namedMetric{keys: map[string]*keyMetric{}}
		into[name] = bucket
	}
	bucket.add(row.prompt, row.completion, row.spend, row.success)
	if row.apiKey != "" {
		addKey(bucket.keys, row.apiKey, row)
	}
}

func addKey(into map[string]*keyMetric, hash string, row activityRow) {
	bucket := into[hash]
	if bucket == nil {
		bucket = &keyMetric{alias: row.keyAlias, teamID: row.teamID, userID: row.userID}
		into[hash] = bucket
	}
	bucket.add(row.prompt, row.completion, row.spend, row.success)
}

func namedJSON(in map[string]*namedMetric) map[string]any {
	out := map[string]any{}
	for name, bucket := range in {
		out[name] = map[string]any{
			"metrics":           bucket.json(),
			"metadata":          map[string]any{},
			"api_key_breakdown": keysJSON(bucket.keys),
		}
	}
	return out
}

func keysJSON(in map[string]*keyMetric) map[string]any {
	out := map[string]any{}
	for hash, bucket := range in {
		out[hash] = map[string]any{
			"metrics": bucket.json(),
			"metadata": map[string]any{
				"key_alias":  nilIfEmpty(bucket.alias),
				"team_id":    nilIfEmpty(bucket.teamID),
				"user_id":    nilIfEmpty(bucket.userID),
				"user_email": nil,
			},
		}
	}
	return out
}

func metadataJSON(total metric, page, totalPages int) map[string]any {
	return map[string]any{
		"total_spend":                                  total.spend,
		"total_prompt_tokens":                          total.prompt,
		"total_completion_tokens":                      total.completion,
		"total_tokens":                                 total.prompt + total.completion,
		"total_api_requests":                           total.requests,
		"total_successful_requests":                    total.success,
		"total_failed_requests":                        total.failed,
		"total_cache_read_input_tokens":                0,
		"total_cache_creation_input_tokens":            0,
		"total_flat_cost":                              0.0,
		"total_compression_saved_tokens":               0,
		"total_compression_savings_spend":              0.0,
		"total_prompt_caching_savings_spend":           0.0,
		"total_gateway_injected_caching_savings_spend": 0.0,
		"total_autorouter_savings_spend":               0.0,
		"total_pages":                                  totalPages,
		"has_more":                                     page < totalPages,
		"page":                                         page,
	}
}

func nilIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func activityDay(ts string, offsetMin int) string {
	shift := -time.Duration(offsetMin) * time.Minute
	if ts == "" {
		return time.Now().UTC().Add(shift).Format("2006-01-02")
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if t, err := time.Parse(layout, ts); err == nil {
			return t.UTC().Add(shift).Format("2006-01-02")
		}
	}
	if len(ts) >= 10 {
		return ts[:10]
	}
	return ts
}

func dayInRange(day, start, end string) bool {
	if day == "" {
		return false
	}
	if start != "" && day < start {
		return false
	}
	if end != "" && day > end {
		return false
	}
	return true
}

func providerName(model string, prices map[string]map[string]any) string {
	if row, ok := prices[model]; ok {
		if p, ok := row["litellm_provider"].(string); ok && p != "" {
			return p
		}
	}
	lower := strings.ToLower(model)
	switch {
	case strings.Contains(lower, "claude"), strings.HasPrefix(lower, "anthropic"):
		return "anthropic"
	case strings.Contains(lower, "gemini"):
		return "gemini"
	case strings.HasPrefix(lower, "gpt"), strings.HasPrefix(lower, "o1"), strings.HasPrefix(lower, "o3"), strings.HasPrefix(lower, "chatgpt"):
		return "openai"
	}
	if i := strings.Index(model, "/"); i > 0 {
		return model[:i]
	}
	if model == "" {
		return "unknown"
	}
	return model
}

func llmRoute(callType string) string {
	switch callType {
	case "", "chat":
		return "/chat/completions"
	case "completions":
		return "/completions"
	case "embeddings":
		return "/embeddings"
	case "messages":
		return "/v1/messages"
	case "audio_speech", "audio":
		return "/audio/speech"
	case "images":
		return "/images/generations"
	case "moderations":
		return "/moderations"
	case "responses":
		return "/responses"
	default:
		return "/" + callType
	}
}
