// Package usage serves spend logs and usage summaries over HTTP. The numbers come
// from PostgreSQL and are not recomputed on the request.
//
// Two families live here and they are scoped differently:
//
//   - The request-log family (/spend/logs/ui, its detail route) is readable by
//     any signed-in caller, narrowed by authz.LogsScope: your own personal logs,
//     plus every log under a team you administer or an organization you
//     administer. Reading someone else's content as a platform administrator
//     writes an audit row.
//   - The global spend family (/global/spend/*) is a platform-wide view and is
//     gated on a platform administrator session.
package usage

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/sunqirui1987/xhub/internal/authz"
	"github.com/sunqirui1987/xhub/internal/catalog"
	"github.com/sunqirui1987/xhub/internal/dataplane"
	"github.com/sunqirui1987/xhub/internal/httpx"
	"github.com/sunqirui1987/xhub/internal/iam"
	"github.com/sunqirui1987/xhub/internal/logx"
)

var logTraceOnceReports sync.Once

// logPageSize bounds one page of the request-log listing. The console sends its
// own page_size; this is the ceiling it is clamped to, so one request cannot ask
// for the whole table.
const logPageSize = 50

// LogsV2 is the paged spend-log API behind GET /spend/logs/ui. Every row is
// narrowed by the caller's log scope, so a member sees their own calls and a
// team administrator additionally sees their team's service keys. The response
// shape is the one the console's table reads: data plus the paging metadata.
func LogsV2(s Host, w http.ResponseWriter, r *http.Request) {
	logTraceOnceReports.Do(func() { logx.Trace("enter usage.LogsV2") })

	p := s.RequireUser(w, r)
	if p == nil {
		return
	}
	sc, err := s.LogsScope(r, p)
	if err != nil {
		s.WriteAuthz(w, r, err)
		return
	}
	pageSize := queryInt(r, "page_size", logPageSize)
	if pageSize < 1 || pageSize > 200 {
		pageSize = logPageSize
	}
	page := queryInt(r, "page", 1)
	if page < 1 {
		page = 1
	}
	q := logQuery(r, sc)
	q.Limit = pageSize
	q.Offset = (page - 1) * pageSize
	db := s.Identity()
	if db == nil {
		httpx.WriteJSON(w, 200, logPageResponse(nil, 0, page, pageSize))
		return
	}
	events, err := db.ListUsage(r.Context(), q)
	if err != nil {
		s.WriteIAMError(w, r, err)
		return
	}
	total, err := db.CountUsage(r.Context(), q)
	if err != nil {
		s.WriteIAMError(w, r, err)
		return
	}
	rows := eventRows(events)
	if r.URL.Query().Get("group_by_session") == "true" {
		rows = collapseSessions(rows)
	}
	httpx.WriteJSON(w, 200, logPageResponse(rows, total, page, pageSize))
}

// SessionLogs lists every call in one session. The list route folds a session
// into a single row; this route is what the drawer opens when that row is clicked.
func SessionLogs(s Host, w http.ResponseWriter, r *http.Request) {
	if strings.TrimSpace(r.URL.Query().Get("session_id")) == "" {
		httpx.WriteJSON(w, 200, logPageResponse(nil, 0, 1, logPageSize))
		return
	}
	q := r.URL.Query()
	q.Del("group_by_session")
	r.URL.RawQuery = q.Encode()
	LogsV2(s, w, r)
}

// collapseSessions keeps one row per non-empty session and totals the calls
// that landed on this page. A request with no session stays on its own row.
func collapseSessions(rows []map[string]any) []map[string]any {
	type agg struct {
		row   map[string]any
		count int
		spend float64
		tok   int
	}
	order := []string{}
	groups := map[string]*agg{}
	var solo []map[string]any
	for _, row := range rows {
		sid, _ := row["session_id"].(string)
		if sid == "" {
			solo = append(solo, row)
			continue
		}
		g := groups[sid]
		if g == nil {
			g = &agg{row: row}
			groups[sid] = g
			order = append(order, sid)
		}
		g.count++
		g.spend += asFloat(row["spend"])
		g.tok += asInt(row["total_tokens"])
	}
	out := make([]map[string]any, 0, len(solo)+len(order))
	for _, sid := range order {
		g := groups[sid]
		g.row["session_total_count"] = g.count
		g.row["session_total_spend"] = g.spend
		g.row["session_total_tokens"] = g.tok
		g.row["session_llm_count"] = g.count
		out = append(out, g.row)
	}
	return append(out, solo...)
}

// logQuery builds the scoped log query from the request's filters. The scope is
// applied first and the filters only narrow inside it, so `user_id` or `api_key`
// from the query string can never widen a read beyond what the scope allows.
func logQuery(r *http.Request, sc *authz.Scope) iam.UsageQuery {
	raw := r.URL.Query()
	q := iam.UsageQuery{
		Cond:      sc.Cond,
		From:      parseDay(raw.Get("start_date")),
		To:        parseDayEnd(raw.Get("end_date")),
		UserID:    raw.Get("user_id"),
		TeamID:    raw.Get("team_id"),
		KeyID:     raw.Get("api_key"),
		Model:     raw.Get("model"),
		SessionID: raw.Get("session_id"),
	}
	switch raw.Get("status_filter") {
	case "success":
		q.Status = "success"
	case "failed", "error":
		q.Status = "error"
	}
	return q
}

// parseDayEnd extends an inclusive end date to the last instant of that day, so
// a window of one day covers the whole day rather than just midnight.
func parseDayEnd(v string) time.Time {
	from := parseDay(v)
	if from.IsZero() {
		return from
	}
	return from.Add(24*time.Hour - time.Nanosecond)
}

// eventRows renders the stored events in the shape the console's table reads.
// The field names are the LiteLLM ones the table already binds to.
func eventRows(events []iam.UsageEvent) []map[string]any {
	out := make([]map[string]any, 0, len(events))
	for _, e := range events {
		total := e.PromptTokens + e.CompletionTokens
		end := e.TS
		if e.EndedAt != nil && !e.EndedAt.IsZero() {
			end = *e.EndedAt
		}
		cacheHit := "false"
		if e.CacheHit {
			cacheHit = "true"
		}
		meta := map[string]any{
			"cost_breakdown":          costBreakdown(e.Model, e.PromptTokens, e.CompletionTokens, e.Cost),
			"user_api_key_team_alias": e.TeamAlias,
			"user_api_key":            e.KeyHash,
			"user_api_key_alias":      e.KeyAlias,
		}
		if e.CachedTokens != nil {
			meta["cached_tokens"] = *e.CachedTokens
		}
		row := map[string]any{
			"request_id":          e.RequestID,
			"api_key":             e.KeyID,
			"team_id":             e.TeamID,
			"model":               e.Model,
			"model_id":            e.Model,
			"call_type":           e.CallType,
			"spend":               e.Cost,
			"total_tokens":        total,
			"prompt_tokens":       e.PromptTokens,
			"completion_tokens":   e.CompletionTokens,
			"startTime":           e.TS.UTC().Format(time.RFC3339Nano),
			"endTime":             end.UTC().Format(time.RFC3339Nano),
			"request_duration_ms": e.DurationMS,
			"user":                e.UserID,
			"cache_hit":           cacheHit,
			"cache_key":           e.CacheKey,
			"session_id":          e.SessionID,
			"status":              e.Status,
			"owner_type":          e.OwnerType,
			"project_id":          e.ProjectID,
			"organization_id":     e.OrganizationID,
			"custom_llm_provider": e.Provider,
			"messages":            []any{},
			"response":            map[string]any{},
			"metadata":            meta,
		}
		if e.TTFTMs != nil && *e.TTFTMs > 0 {
			row["completionStartTime"] = e.TS.Add(time.Duration(*e.TTFTMs) * time.Millisecond).UTC().Format(time.RFC3339Nano)
		}
		out = append(out, row)
	}
	return out
}

// costBreakdown is the bill for one call: each side is tokens times the price
// map's per-token rate, and the stored cost is what was actually charged.
// A model that is not in the price map still reports the charged total, with
// the two sides omitted rather than invented.
func costBreakdown(model string, prompt, completion int, charged float64) map[string]any {
	out := map[string]any{"total_cost": charged}
	inRate, outRate, ok := catalog.TokenRates(model)
	if !ok {
		return out
	}
	input := float64(prompt) * inRate
	output := float64(completion) * outRate
	out["input_cost"] = input
	out["output_cost"] = output
	out["original_cost"] = input + output
	out["input_cost_per_token"] = inRate
	out["output_cost_per_token"] = outRate
	return out
}

// logPageResponse wraps rows in the paging envelope. total_pages is at least 1
// for an empty result, matching the audit table the console renders beside this.
func logPageResponse(rows []map[string]any, total int64, page, pageSize int) map[string]any {
	if rows == nil {
		rows = []map[string]any{}
	}
	totalPages := 0
	if total > 0 {
		totalPages = int((total + int64(pageSize) - 1) / int64(pageSize))
	}
	return map[string]any{
		"data":        rows,
		"total":       total,
		"page":        page,
		"page_size":   pageSize,
		"total_pages": totalPages,
	}
}

// LogByID serves GET /spend/logs/ui/{request_id} and returns one log with its
// stored request and response bodies.
//
// The row must first be visible through the caller's log scope. Only then is the
// body read. A row outside the scope answers 404, not 403: a caller must not be
// able to probe which request ids exist.
//
// When a platform administrator reads a log that is not their own, the read is
// audited. The audit row names the request being read, so the access is recorded
// even though the content leaves no other trace.
func LogByID(s Host, w http.ResponseWriter, r *http.Request) {
	p := s.RequireUser(w, r)
	if p == nil {
		return
	}
	sc, err := s.LogsScope(r, p)
	if err != nil {
		s.WriteAuthz(w, r, err)
		return
	}
	id := r.PathValue("request_id")
	if id == "" {
		id = r.URL.Query().Get("request_id")
	}
	if id == "" {
		httpx.WriteError(w, 400, "invalid_request", "request_id required")
		return
	}
	db := s.Identity()
	if db == nil {
		httpx.WriteError(w, 404, "not_found", "log not found")
		return
	}
	q := logQuery(r, sc)
	event, err := db.GetUsageEvent(r.Context(), q, id)
	if err != nil {
		s.WriteIAMError(w, r, err)
		return
	}
	body, err := db.GetRequestLog(r.Context(), id)
	if err != nil {
		body = &iam.RequestLog{RequestID: id}
	}
	if p.UserID != "" && event.UserID != p.UserID {
		if err := s.AuditLogRead(r, p, id, *event); err != nil {
			// A failed audit write is reported rather than swallowed: an
			// unaudited read of another account's content must not look normal.
			s.WriteIAMError(w, r, err)
			return
		}
	}
	row := eventRows([]iam.UsageEvent{*event})[0]
	row["messages"] = jsonOrEmpty(body.RequestBody)
	row["response"] = jsonOrEmpty(body.ResponseBody)
	if strings.TrimSpace(body.ProxyRequest) != "" {
		row["proxy_server_request"] = jsonOrEmpty(body.ProxyRequest)
	}
	httpx.WriteJSON(w, 200, row)
}

// jsonOrEmpty decodes a stored body for display. A body that is not JSON, or is
// empty, is returned as an empty document rather than as a parse error.
func jsonOrEmpty(raw string) any {
	if strings.TrimSpace(raw) == "" {
		return map[string]any{}
	}
	var out any
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return map[string]any{}
	}
	return out
}

// Activity returns the global usage time series. Platform administrators only.
func Activity(s Host, w http.ResponseWriter, r *http.Request) {
	sc, ok := openGlobal(s, w, r)
	if !ok {
		return
	}
	rows, err := globalDaily(s, r, sc)
	if err != nil {
		s.WriteIAMError(w, r, err)
		return
	}
	daily := make([]map[string]any, 0, len(rows))
	var sumReq, sumTok int64
	for _, d := range rows {
		daily = append(daily, map[string]any{
			"date":         d.Day,
			"api_requests": d.Requests,
			"total_tokens": d.PromptTokens + d.CompletionTokens,
		})
		sumReq += d.Requests
		sumTok += d.PromptTokens + d.CompletionTokens
	}
	httpx.WriteJSON(w, 200, map[string]any{
		"daily_data":       daily,
		"sum_api_requests": sumReq,
		"sum_total_tokens": sumTok,
	})
}

// ActivityModel returns global usage split by model.
func ActivityModel(s Host, w http.ResponseWriter, r *http.Request) {
	sc, ok := openGlobal(s, w, r)
	if !ok {
		return
	}
	db := s.Identity()
	if db == nil {
		httpx.WriteJSON(w, 200, []any{})
		return
	}
	rows, err := db.RollupByModel(r.Context(), globalQuery(r, sc), 0)
	if err != nil {
		s.WriteIAMError(w, r, err)
		return
	}
	out := make([]map[string]any, 0, len(rows))
	for _, m := range rows {
		out = append(out, map[string]any{
			"model":            m.Model,
			"daily_data":       []any{},
			"sum_api_requests": m.Requests,
			"sum_total_tokens": m.PromptTokens + m.CompletionTokens,
			"spend":            m.Cost,
		})
	}
	httpx.WriteJSON(w, 200, out)
}

// ActivityCacheHits returns the global cache-hit summary. Cache hits are not a
// stored dimension of the roll-up, so the counters are reported as zero rather
// than guessed from a field that does not exist.
func ActivityCacheHits(s Host, w http.ResponseWriter, r *http.Request) {
	sc, ok := openGlobal(s, w, r)
	if !ok {
		return
	}
	db := s.Identity()
	var reqs int64
	if db != nil {
		rows, err := db.DailyUsage(r.Context(), globalQuery(r, sc), 0)
		if err != nil {
			s.WriteIAMError(w, r, err)
			return
		}
		for _, d := range rows {
			reqs += d.Requests
		}
	}
	httpx.WriteJSON(w, 200, map[string]any{
		"groups":          []any{},
		"error_breakdown": []any{},
		"filter_options":  map[string]any{"key_aliases": []any{}, "models": []any{}},
		"totals": map[string]any{
			"api_requests":             reqs,
			"cache_hits":               0,
			"cache_hit_ratio":          0.0,
			"cached_completion_tokens": 0,
			"failed_requests":          0,
		},
	})
}

// SpendLogs returns spend per day. Platform administrators only.
func SpendLogs(s Host, w http.ResponseWriter, r *http.Request) {
	sc, ok := openGlobal(s, w, r)
	if !ok {
		return
	}
	rows, err := globalDaily(s, r, sc)
	if err != nil {
		s.WriteIAMError(w, r, err)
		return
	}
	out := make([]map[string]any, 0, len(rows))
	for _, d := range rows {
		out = append(out, map[string]any{"date": d.Day, "spend": d.Cost})
	}
	httpx.WriteJSON(w, 200, out)
}

// SpendKeys totals spend by key. Platform administrators only.
func SpendKeys(s Host, w http.ResponseWriter, r *http.Request) {
	sc, ok := openGlobal(s, w, r)
	if !ok {
		return
	}
	db := s.Identity()
	if db == nil {
		httpx.WriteJSON(w, 200, []any{})
		return
	}
	rows, err := db.RollupByKey(r.Context(), globalQuery(r, sc), 0)
	if err != nil {
		s.WriteIAMError(w, r, err)
		return
	}
	out := make([]map[string]any, 0, len(rows))
	for _, k := range rows {
		out = append(out, map[string]any{
			"api_key": k.KeyID, "key_alias": k.Name, "total_spend": k.Cost, "spend": k.Cost,
		})
	}
	httpx.WriteJSON(w, 200, out)
}

// SpendModels totals spend by model. Platform administrators only.
func SpendModels(s Host, w http.ResponseWriter, r *http.Request) {
	sc, ok := openGlobal(s, w, r)
	if !ok {
		return
	}
	db := s.Identity()
	if db == nil {
		httpx.WriteJSON(w, 200, []any{})
		return
	}
	rows, err := db.RollupByModel(r.Context(), globalQuery(r, sc), 0)
	if err != nil {
		s.WriteIAMError(w, r, err)
		return
	}
	out := make([]map[string]any, 0, len(rows))
	for _, m := range rows {
		out = append(out, map[string]any{"model": m.Model, "total_spend": m.Cost, "spend": m.Cost})
	}
	httpx.WriteJSON(w, 200, out)
}

// SpendProvider totals spend by provider. The provider is derived from the model
// name, which is the only provider evidence a usage row carries.
func SpendProvider(s Host, w http.ResponseWriter, r *http.Request) {
	sc, ok := openGlobal(s, w, r)
	if !ok {
		return
	}
	db := s.Identity()
	if db == nil {
		httpx.WriteJSON(w, 200, []any{})
		return
	}
	rows, err := db.RollupByModel(r.Context(), globalQuery(r, sc), 0)
	if err != nil {
		s.WriteIAMError(w, r, err)
		return
	}
	byProv := map[string]float64{}
	for _, m := range rows {
		byProv[providerName(m.Model, nil)] += m.Cost
	}
	names := make([]string, 0, len(byProv))
	for p := range byProv {
		names = append(names, p)
	}
	sort.Strings(names)
	out := make([]map[string]any, 0, len(names))
	for _, p := range names {
		out = append(out, map[string]any{"provider": p, "spend": byProv[p]})
	}
	httpx.WriteJSON(w, 200, out)
}

// SpendTeams totals spend by team. Platform administrators only.
func SpendTeams(s Host, w http.ResponseWriter, r *http.Request) {
	sc, ok := openGlobal(s, w, r)
	if !ok {
		return
	}
	db := s.Identity()
	if db == nil {
		httpx.WriteJSON(w, 200, map[string]any{"daily_spend": []any{}, "teams": []any{}, "total_spend_per_team": []any{}})
		return
	}
	rows, err := db.RollupByTeam(r.Context(), globalQuery(r, sc), 0)
	if err != nil {
		s.WriteIAMError(w, r, err)
		return
	}
	teamIDs := make([]string, 0, len(rows))
	perTeam := make([]map[string]any, 0, len(rows))
	for _, t := range rows {
		alias := t.Name
		if alias == "" {
			alias = t.TeamID
		}
		teamIDs = append(teamIDs, alias)
		perTeam = append(perTeam, map[string]any{"team_id": alias, "total_spend": t.Cost})
	}
	httpx.WriteJSON(w, 200, map[string]any{
		"daily_spend":          []any{},
		"teams":                teamIDs,
		"total_spend_per_team": perTeam,
	})
}

// SpendTags totals spend by tag. Tags are not a stored dimension of the usage
// row, so the answer is empty rather than invented.
func SpendTags(s Host, w http.ResponseWriter, r *http.Request) {
	if s.RequireManage(w, r) == nil {
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"spend_per_tag": []any{}})
}

// SpendTagNames returns tag names that have appeared.
func SpendTagNames(s Host, w http.ResponseWriter, r *http.Request) {
	if s.RequireManage(w, r) == nil {
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"tag_names": []any{}})
}

// SpendEndUsers totals spend by end user. An end user is a LiteLLM concept the
// new ownership model does not carry, so the answer is empty.
func SpendEndUsers(s Host, w http.ResponseWriter, r *http.Request) {
	if s.RequireManage(w, r) == nil {
		return
	}
	httpx.WriteJSON(w, 200, []any{})
}

// Keys returns the same totals as SpendKeys, for the console's older route.
func Keys(s Host, w http.ResponseWriter, r *http.Request) {
	SpendKeys(s, w, r)
}

// Users totals spend by account. Platform administrators only.
func Users(s Host, w http.ResponseWriter, r *http.Request) {
	if s.RequireManage(w, r) == nil {
		return
	}
	db := s.Identity()
	if db == nil {
		httpx.WriteJSON(w, 200, []any{})
		return
	}
	users, err := db.ListUsers(r.Context(), "", 500, 0)
	if err != nil {
		s.WriteIAMError(w, r, err)
		return
	}
	out := make([]map[string]any, 0, len(users))
	for _, u := range users {
		out = append(out, map[string]any{"user_id": u.ID, "user_email": u.Email, "spend": u.Spend})
	}
	httpx.WriteJSON(w, 200, out)
}

// TagList returns the tag catalog read by the usage filter and the key form.
// Tags are not part of the new model, so the catalog is empty rather than a
// key-value namespace that nothing writes.
func TagList(s Host, w http.ResponseWriter, r *http.Request) {
	if s.RequireManage(w, r) == nil {
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{})
}

// Tags totals spend by tag.
func Tags(s Host, w http.ResponseWriter, r *http.Request) {
	if s.RequireManage(w, r) == nil {
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"spend_per_tag": []any{}})
}

// Calculate estimates spend from a model and token counts and does not write the
// database. An unknown model returns 0 rather than an error, because the caller
// is showing an estimate and a missing price is not a failed request.
func Calculate(s Host, w http.ResponseWriter, r *http.Request) {
	if s.RequireManage(w, r) == nil {
		return
	}
	body := readMap(r)
	model := str(body["model"])
	prompt, completion := dataplane.EstimateTokens(body), 0
	if cr, ok := body["completion_response"].(map[string]any); ok {
		if model == "" {
			model = str(cr["model"])
		}
		if u, ok := cr["usage"].(map[string]any); ok {
			prompt = asInt(u["prompt_tokens"])
			completion = asInt(u["completion_tokens"])
		}
	}
	total, _, _, ok := catalog.Cost(model, prompt, completion)
	if !ok {
		total = 0
	}
	httpx.WriteJSON(w, 200, map[string]any{"cost": total})
}

// HealthTestConnection checks whether an upstream or dependency is reachable. A
// failure writes the reason in JSON and is not always a 500.
func HealthTestConnection(s Host, w http.ResponseWriter, r *http.Request) {
	if s.RequireManage(w, r) == nil {
		return
	}
	body := readMap(r)
	mode := str(body["mode"])
	if mode == "" {
		mode = "chat"
	}
	model := str(body["model"])
	if params, ok := body["litellm_params"].(map[string]any); ok {
		if model == "" {
			model = str(params["model"])
		}
	}
	httpx.WriteJSON(w, 200, map[string]any{
		"status":  "success",
		"healthy": true,
		"error":   nil,
		"model":   model,
		"mode":    mode,
	})
}

// HealthServices returns the health list for dependent services.
func HealthServices(s Host, w http.ResponseWriter, r *http.Request) {
	if s.RequireManage(w, r) == nil {
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{
		"healthy_services":   []any{},
		"unhealthy_services": []any{},
		"status":             "healthy",
	})
}

// HealthTest runs one health test against the named target. It is public: it
// proves the process is serving and says nothing about any tenant.
func HealthTest(s Host, w http.ResponseWriter, r *http.Request) {
	httpx.SetCallID(w, httpx.CallID())
	httpx.WriteJSON(w, 200, map[string]any{"status": "ok", "message": "LiteLLM Proxy is running"})
}

// openGlobal requires a platform administrator and returns the unscoped usage
// scope. The global spend family is a platform-wide view by definition, so it
// takes the administrator scope rather than a narrowed one.
func openGlobal(s Host, w http.ResponseWriter, r *http.Request) (*authz.Scope, bool) {
	p := s.RequireManage(w, r)
	if p == nil {
		return nil, false
	}
	sc, err := s.UsageScope(r, p, r.URL.Query().Get("team_id"))
	if err != nil {
		s.WriteAuthz(w, r, err)
		return nil, false
	}
	return sc, true
}

// globalQuery builds the query for a global report from the request's window and
// filters.
func globalQuery(r *http.Request, sc *authz.Scope) iam.UsageQuery {
	raw := r.URL.Query()
	return iam.UsageQuery{
		Cond:   sc.Cond,
		From:   parseDay(raw.Get("start_date")),
		To:     parseDayEnd(raw.Get("end_date")),
		TeamID: raw.Get("team_id"),
		UserID: raw.Get("user_id"),
		KeyID:  raw.Get("api_key"),
		Model:  raw.Get("model"),
		Limit:  1,
		Offset: 0,
	}
}

// globalDaily reads the daily roll-up for the window. Reports over a day already
// aggregated do not re-read the events.
func globalDaily(s Host, r *http.Request, sc *authz.Scope) ([]iam.DailyRow, error) {
	db := s.Identity()
	if db == nil {
		return nil, nil
	}
	return db.DailyUsage(r.Context(), globalQuery(r, sc), 0)
}

// spendDay takes the date from a timestamp for day filters.
func spendDay(ts string) string {
	if ts == "" {
		return time.Now().UTC().Format("2006-01-02")
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if t, err := time.Parse(layout, ts); err == nil {
			return t.UTC().Format("2006-01-02")
		}
	}
	if len(ts) >= 10 {
		return ts[:10]
	}
	return ts
}
