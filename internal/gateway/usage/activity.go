package usage

import (
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sunqirui1987/xhub/internal/authz"
	"github.com/sunqirui1987/xhub/internal/catalog"
	"github.com/sunqirui1987/xhub/internal/httpx"
	"github.com/sunqirui1987/xhub/internal/iam"
	"github.com/sunqirui1987/xhub/internal/logx"
)

const activityPageSize = 50

var logTraceOnceActivity sync.Once

// UserDailyActivity is GET /user/daily/activity. It groups stored usage into the
// daily rollup the usage page reads, newest day first and paged. The entity
// breakdown is per user, which is what "用户用量" renders, and the scope is the
// caller's own rows plus the teams they oversee.
func UserDailyActivity(s Host, w http.ResponseWriter, r *http.Request) {
	logTraceOnceActivity.Do(func() { logx.Trace("enter usage.UserDailyActivity") })
	writeDailyActivity(s, w, r, entityUser, false)
}

// UserDailyActivityAggregated is GET /user/daily/activity/aggregated. The same
// rollup in a single response, which is what "你的用量" and the global view load
// first. A user id on the query narrows to that account inside the scope.
func UserDailyActivityAggregated(s Host, w http.ResponseWriter, r *http.Request) {
	writeDailyActivity(s, w, r, entityUser, true)
}

// GatewayDailyActivity is GET /gateway/daily/activity. Request counts come from
// the same usage rows, split by outcome and route. The scope decides whose calls
// are counted: a platform administrator sees the gateway, anyone else sees only
// their own calls and the calls inside the teams they belong to.
func GatewayDailyActivity(s Host, w http.ResponseWriter, r *http.Request) {
	sc, ok := openActivity(s, w, r, r.URL.Query().Get("team_id"))
	if !ok {
		return
	}
	rows, err := loadActivity(s, r, sc)
	if err != nil {
		s.WriteIAMError(w, r, err)
		return
	}
	httpx.WriteJSON(w, 200, gatewayActivityBody(rows))
}

// openActivity accepts any signed-in caller and returns the usage scope that
// narrows every row the handler will read. A key reaches only its own usage; a
// session reaches its own rows plus the teams it belongs to; a platform
// administrator reaches everything. The scope is required rather than optional,
// so a handler cannot fall through to an unfiltered read.
func openActivity(s Host, w http.ResponseWriter, r *http.Request, teamID string) (*authz.Scope, bool) {
	p := s.RequireUser(w, r)
	if p == nil {
		return nil, false
	}
	sc, err := s.UsageScope(r, p, teamID)
	if err != nil {
		s.WriteAuthz(w, r, err)
		return nil, false
	}
	return sc, true
}

type activityRow struct {
	day            string
	model          string
	provider       string
	apiKey         string
	keyAlias       string
	teamID         string
	teamAlias      string
	organizationID string
	userID         string
	route          string
	prompt         int
	completion     int
	spend          float64
	success        bool
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
	entities  map[string]*entityBucket
}

// loadActivity reads the usage rows the caller may see and turns them into the
// flat rows the rollup folds.
//
// The scope is applied first and the query's own filters only narrow within it,
// never widen it. That ordering matters: `user_id` and `api_key` arrive straight
// from the query string, so a caller could otherwise ask for another account's
// rows by passing their id. Inside the scope the worst that reaches is a row the
// caller was already allowed to see, and a filter that names someone else simply
// returns the empty intersection.
func loadActivity(s Host, r *http.Request, sc *authz.Scope) ([]activityRow, error) {
	return loadActivityQuery(s, r, activityQuery(r, sc))
}

// loadActivityQuery reads the events for a query that already carries the scope
// and any narrowing filters, then flattens them for the roll-up.
func loadActivityQuery(s Host, r *http.Request, q iam.UsageQuery) ([]activityRow, error) {
	db := s.Identity()
	if db == nil {
		return nil, nil
	}
	events, err := db.ListUsage(r.Context(), q)
	if err != nil {
		return nil, err
	}
	return eventsToActivity(events, uiTimezone(r)), nil
}

// activityQuery builds the scoped query from the request. The scope comes from
// authz and is never nil, so an actor who may see nothing gets a filter that
// matches nothing rather than no filter at all.
func activityQuery(r *http.Request, sc *authz.Scope) iam.UsageQuery {
	raw := r.URL.Query()
	return iam.UsageQuery{
		Cond: sc.Cond,
		From: parseDay(raw.Get("start_date")),
		// The end date covers the whole day. The console sends the same date for
		// start and end when the user picks a single day, and a bare midnight
		// bound would leave that day's calls out of the window.
		To:     parseDayEnd(raw.Get("end_date")),
		UserID: raw.Get("user_id"),
		TeamID: raw.Get("team_id"),
		KeyID:  raw.Get("api_key"),
		Limit:  activityScanLimit,
		Offset: pageOffset(r, activityScanLimit),
	}
}

// eventsToActivity flattens the stored events into the rows the rollup folds.
// Provider comes from the price map, falling back to the model name so an
// unknown model is still attributed rather than dropped.
func eventsToActivity(events []iam.UsageEvent, tzMinutes int) []activityRow {
	prices := catalog.CostMap()
	out := make([]activityRow, 0, len(events))
	for _, e := range events {
		if e.Cost == 0 {
			continue
		}
		model := e.Model
		if model == "" {
			model = "unknown"
		}
		out = append(out, activityRow{
			day:            activityDay(e.TS, tzMinutes),
			model:          model,
			provider:       providerName(model, prices),
			apiKey:         e.KeyID,
			keyAlias:       e.KeyAlias,
			teamID:         e.TeamID,
			teamAlias:      e.TeamAlias,
			organizationID: e.OrganizationID,
			userID:         e.UserID,
			route:          llmRoute(e.CallType),
			prompt:         e.PromptTokens,
			completion:     e.CompletionTokens,
			spend:          e.Cost,
			success:        e.Status == "" || e.Status == "success" || e.Status == "succeeded",
		})
	}
	return out
}

// uiTimezone reads the browser's UTC offset in minutes. The console sends
// Date.getTimezoneOffset(), which is positive west of Greenwich, so the sign is
// flipped to get the offset east of it that the day boundary needs.
func uiTimezone(r *http.Request) int {
	off, err := strconv.Atoi(r.URL.Query().Get("timezone"))
	if err != nil {
		return 0
	}
	return -off
}

// parseDay reads a YYYY-MM-DD date filter. An unset or unparseable value leaves
// that side of the window open rather than filtering everything out.
func parseDay(v string) time.Time {
	if v == "" {
		return time.Time{}
	}
	t, err := time.Parse("2006-01-02", v)
	if err != nil {
		return time.Time{}
	}
	return t
}

// activityScanLimit bounds how many events one request folds. It is a request
// bound, not an authorization one: the scope already decided what is visible.
const activityScanLimit = 5000

func dailyActivityResponse(rows []activityRow, page int, aggregated bool) map[string]any {
	return activityBody(rows, page, aggregated, entityNone, entityLabels{})
}

// activityBody is the usage-page payload. dim chooses the breakdown.entities
// key: a team, an organization, or a user. entityNone leaves that map empty,
// which is what the gateway count view and the older callers want.
func activityBody(rows []activityRow, page int, aggregated bool, dim entityDim, labels entityLabels) map[string]any {
	days := rollupDays(rows, dim)
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
				"entities":     entitiesJSON(day.entities, dim, labels),
			},
		})
	}
	return map[string]any{
		"results":  results,
		"metadata": metadataJSON(total, page, totalPages),
	}
}

func gatewayActivityBody(rows []activityRow) map[string]any {
	days := rollupDays(rows, entityNone)
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

func rollupDays(rows []activityRow, dim entityDim) map[string]*dayMetric {
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
		if dim != entityNone {
			addEntity(day, dim, row)
		}
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

// activityDay renders the calendar day of an event in the caller's timezone.
// The roll-up is folded per rendered day, so the offset is applied here rather
// than in SQL: the event carries a timestamp, not a day.
func activityDay(ts time.Time, tzMinutes int) string {
	if ts.IsZero() {
		return time.Now().UTC().Add(time.Duration(tzMinutes) * time.Minute).Format("2006-01-02")
	}
	return ts.UTC().Add(time.Duration(tzMinutes) * time.Minute).Format("2006-01-02")
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
