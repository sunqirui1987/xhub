// Package usage serves spend logs and usage summaries over HTTP. The numbers come
// from PostgreSQL and are not recomputed on the request.
//
// Two families live here and they are scoped differently:
//
//   - The request-log family (/spend/logs/ui, its detail route) is readable by
//     any signed-in caller, narrowed by authz.LogsScope: your own personal logs,
//     plus service-key logs for teams you administer. Platform administrators
//     can read every log. Reading someone else's content as a platform
//     administrator writes an audit row.
//   - The global spend family (/global/spend/*) is a platform-wide view and is
//     gated on a platform administrator session.
package usage

import (
	"encoding/json"
	"github.com/sunqirui1987/xhub/internal/pagination"
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

// LogsV2 is the paged spend-log API behind GET /spend/logs/ui. Every row is narrowed by the caller's log scope, so a member sees their own calls and a team administrator additionally sees their team's service keys. The response shape is the one the console's table reads: data plus the paging metadata.
// 参数 s（Host）：LogsV2使用的数据面宿主；w（http.ResponseWriter）：调用方的 HTTP 响应，状态和正文写在这里；r（*http.Request）：入站 HTTP 请求。
// 返回：无。状态码和正文写进调用方的响应。
// 调用：gateway/usage/mount.go
// 测试：无直接单测
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
	q.Offset = pagination.Offset(page, pageSize)

	db := s.Identity()
	if db == nil {
		httpx.WriteJSON(w, 200, logPageResponse(nil, 0, page, pageSize))
		return
	}
	var rows []map[string]any
	var total int64
	if r.URL.Query().Get("group_by_session") == "true" {
		// SQL 在合并会话之后分页，保证行数、总数和会话汇总采用同一粒度。
		groups, count, readErr := db.ListUsageSessions(r.Context(), q)
		if readErr != nil {
			s.WriteIAMError(w, r, readErr)
			return
		}
		total = count
		rows = make([]map[string]any, 0, len(groups))
		for _, group := range groups {
			row := eventRows([]iam.UsageEvent{group.UsageEvent})[0]
			if group.SessionID != "" && (group.KeyID != "" || group.UserID != "") {
				row["session_total_count"] = group.SessionCount
				row["session_llm_count"] = group.SessionCount
				row["session_total_spend"] = group.SessionSpend
				row["session_total_tokens"] = group.SessionTokens
			}
			rows = append(rows, row)
		}
	} else {
		events, readErr := db.ListUsage(r.Context(), q)
		if readErr != nil {
			s.WriteIAMError(w, r, readErr)
			return
		}
		total, err = db.CountUsage(r.Context(), q)
		if err != nil {
			s.WriteIAMError(w, r, err)
			return
		}
		rows = eventRows(events)
	}
	httpx.WriteJSON(w, 200, logPageResponse(rows, total, page, pageSize))
}

// SessionLogs lists every call in one caller's session. The list route folds a
// caller/session pair into a single row; this route is what the drawer opens
// when that row is clicked. api_key takes precedence over user_id, matching the
// grouping rule used by collapseSessions.
// 参数 s（Host）：会话Logs使用的数据面宿主；w（http.ResponseWriter）：调用方的 HTTP 响应，状态和正文写在这里；r（*http.Request）：入站 HTTP 请求。
// 返回：无。状态码和正文写进调用方的响应。
// 调用：gateway/usage/mount.go
// 测试：无直接单测
func SessionLogs(s Host, w http.ResponseWriter, r *http.Request) {
	if strings.TrimSpace(r.URL.Query().Get("session_id")) == "" {
		httpx.WriteJSON(w, 200, logPageResponse(nil, 0, 1, logPageSize))
		return
	}
	q := r.URL.Query()
	q.Del("group_by_session")
	if strings.TrimSpace(q.Get("api_key")) != "" {
		q.Del("user_id")
	}
	r.URL.RawQuery = q.Encode()
	LogsV2(s, w, r)
}

// collapseSessions keeps one row per caller and non-empty session and totals
// the calls that landed on this page. API key is the caller identity when
// present; keyless calls use user. Rows without a caller identity remain
// separate. A request with no session stays on its own row.
// 参数 rows（[]map[string]any）：从用量或目录读出的map[string]any。
// 返回 []map[string]any（[]map[string]any）：一组map[string]any。没有匹配时为空切片，不是 nil 分页。
// 调用：仅在 reports.go 内使用
// 测试：无直接单测
func collapseSessions(rows []map[string]any) []map[string]any {
	type sessionKey struct {
		kind, caller, session string
	}
	type agg struct {
		row   map[string]any
		count int
		spend float64
		tok   int
	}
	order := []sessionKey{}
	groups := map[sessionKey]*agg{}
	var solo []map[string]any
	for _, row := range rows {
		sid, _ := row["session_id"].(string)
		if sid == "" {
			solo = append(solo, row)
			continue
		}
		groupKey := sessionKey{session: sid}
		if keyID, _ := row["api_key"].(string); keyID != "" {
			groupKey.kind, groupKey.caller = "key", keyID
		} else if userID, _ := row["user"].(string); userID != "" {
			groupKey.kind, groupKey.caller = "user", userID
		} else {
			// Without a caller, even a missing or duplicated request ID cannot
			// prove that two rows belong to the same session.
			solo = append(solo, row)
			continue
		}
		g := groups[groupKey]
		if g == nil {
			g = &agg{row: row}
			groups[groupKey] = g
			order = append(order, groupKey)
		}
		g.count++
		g.spend += asFloat(row["spend"])
		g.tok += asInt(row["total_tokens"])
	}
	out := make([]map[string]any, 0, len(solo)+len(order))
	for _, groupKey := range order {
		g := groups[groupKey]
		g.row["session_total_count"] = g.count
		g.row["session_total_spend"] = g.spend
		g.row["session_total_tokens"] = g.tok
		g.row["session_llm_count"] = g.count
		out = append(out, g.row)
	}
	return append(out, solo...)
}

// logQuery builds the scoped log query from the request's filters. The scope is applied first and the filters only narrow inside it, so `user_id` or `api_key` from the query string can never widen a read beyond what the scope allows.
// 参数 r（*http.Request）：入站 HTTP 请求；sc（*authz.Scope）：日志查询使用的权限范围。
// 返回 UsageQuery（iam.UsageQuery）：在权限范围之内、再按请求筛选收窄的日志查询。筛选不能把范围扩大。
// 调用：仅在 reports.go 内使用
// 测试：reports_test.go；排序字段由 iam 白名单处理，未知方向使用降序。
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
		RequestID: raw.Get("request_id"),
		Search:    raw.Get("search"),
		SortBy:    raw.Get("sort_by"),
		SortAsc:   raw.Get("sort_order") == "asc",
	}
	switch raw.Get("status_filter") {
	case "success", "completed", "executing", "polling", "non_error":
		q.Status = raw.Get("status_filter")
	case "failed", "error":
		q.Status = "error"
	}
	return q
}

// parseDayEnd extends an inclusive end date to the last instant of that day, so a window of one day covers the whole day rather than just midnight.
// 参数 v（string）：解析日期终点使用的值。空串表示调用方没有提供这项。
// 返回 time.Time（time.Time）：解析出的时间。
// 调用：gateway/usage/activity.go
// 测试：activity_test.go
func parseDayEnd(v string) time.Time {
	from := parseDay(v)
	if from.IsZero() {
		return from
	}
	return from.Add(24*time.Hour - time.Nanosecond)
}

// eventRows renders the stored events in the shape the console's table reads. The field names are the LiteLLM ones the table already binds to.
// 参数 events（[]iam.UsageEvent）：事件行使用的用量事件。
// 返回 []map[string]any（[]map[string]any）：一组map[string]any。没有匹配时为空切片，不是 nil 分页。
// 调用：仅在 reports.go 内使用
// 测试：cost_breakdown_test.go、log_guardrail_test.go
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
			"cost_breakdown":          costBreakdown(e),
			"user_api_key_team_alias": e.TeamAlias,
			"user_api_key":            e.KeyHash,
			"user_api_key_alias":      e.KeyAlias,
		}
		if e.CachedTokens != nil {
			meta["cached_tokens"] = *e.CachedTokens
		}
		if e.HTTPStatus > 0 {
			meta["http_status"] = e.HTTPStatus
		}
		if info := guardrailInformation(e.Guardrail); info != nil {
			meta["guardrail_information"] = info
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
		if e.CachedTokens != nil {
			row["cache_read_input_tokens"] = *e.CachedTokens
		}
		if e.TTFTMs != nil && *e.TTFTMs > 0 {
			row["completionStartTime"] = e.TS.Add(time.Duration(*e.TTFTMs) * time.Millisecond).UTC().Format(time.RFC3339Nano)
		}
		out = append(out, row)
	}
	return out
}

// guardrailInformation decodes the monitoring rows stored on the event. A blank or unreadable value is omitted so an ordinary call does not grow an empty section.
// 参数 raw（string）：护栏Information使用的原始内容。空串表示调用方没有提供这项。
// 返回 any（any）：护栏Information。没有合格值时为 nil。
// 调用：仅在 reports.go 内使用
// 测试：无直接单测
func guardrailInformation(raw string) any {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var out any
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return out
}

// costBreakdown is the bill for one call: each side is tokens times the price map's per-token rate, and the stored cost is what was actually charged. A model that is not in the price map still reports the charged total, with the two sides omitted rather than invented.
// 参数 model（string）：发给上游或对外展示的模型名；prompt（int）：提示 token 数，用来估价；completion（int）：完成 token 数，用来估价；charged（float64）：费用Breakdown使用的小数。0 表示没有费用或尚未计价。
// 返回 map[string]any（map[string]any）：费用Breakdown的字段表。缺键表示上游或库里没有这个字段。
// 调用：仅在 reports.go 内使用
// 测试：cost_breakdown_test.go
// costBreakdown explains what one call was charged, in the shape the log detail
// drawer reads.
//
// It reads the rates stored on the row rather than today's price table. That is
// the point of storing them: the flat fields in the price table carry only one
// variant per side - for a window-priced model that is the off-peak one - so
// recomputing a peak call from them reports roughly half what was charged. Any
// price edit or catalog reload moves the number again, which makes a bill that
// cannot be checked against itself.
//
// Rows written before price_snapshot existed have nothing to read, so they fall
// back to recomputing at the instant the call started. That instant is the row's
// own start, so the window is stable across reads; what moves is the price
// table. The response says which happened: "source" is "snapshot" or
// "recomputed", so the console can label a re-derived figure instead of
// presenting it as the original record.
//
// 参数 e（iam.UsageEvent）：一条用量行，含存下来的费率快照、开始时刻和金额。
// 返回 map[string]any（map[string]any）：日志详情读的费用明细。未定价的行只有 total_cost。
// 调用：eventRows。
// 测试：cost_breakdown_test.go
func costBreakdown(e iam.UsageEvent) map[string]any {
	out := map[string]any{"total_cost": e.Cost}

	if snap := parsePriceSnapshot(e.PriceSnapshot); snap != nil {
		out["window"] = snap.Window
		out["applied"] = appliedRateRows(snap.Applied)
		out["source"] = "snapshot"
		if snap.PricingStatus != "" {
			out["pricing_status"] = snap.PricingStatus
		}
		if snap.Usage != nil {
			out["usage"] = snap.Usage
		}
		if snap.PricingStatus == "unpriced" {
			return out
		}
		for _, rate := range snap.Applied {
			amount := rate.Quantity * rate.USD
			switch rate.Side {
			case "input":
				out["input_cost"] = valueOrZero(out["input_cost"]) + amount
			case "cache_read":
				out["cache_read_cost"] = valueOrZero(out["cache_read_cost"]) + amount
			case "cache_write":
				out["cache_creation_cost"] = valueOrZero(out["cache_creation_cost"]) + amount
			default:
				out["output_cost"] = valueOrZero(out["output_cost"]) + amount
			}
			// The per-token rates exist for the console's "N tokens × $X/1M"
			// line, so they are set only for a rate that really is per token.
			// A picture or a query rate on the same side would otherwise be
			// shown as a per-token price, which is a unit the model never quoted.
			if rate.Measure != "token" {
				continue
			}
			switch rate.Side {
			case "input":
				if _, seen := out["input_cost_per_token"]; !seen {
					out["input_cost_per_token"] = rate.USD
				}
			case "output":
				if _, seen := out["output_cost_per_token"]; !seen {
					out["output_cost_per_token"] = rate.USD
				}
			}
		}
		// The prompt side is reported whole, with the cached part broken out
		// again beside it. That is the shape the log drawer expects: it renders
		// "Input Cost" net of the cache rows it also shows, so a prompt side that
		// excluded the cached tokens would render negative.
		cacheTotal := valueOrZero(out["cache_read_cost"]) + valueOrZero(out["cache_creation_cost"])
		if cacheTotal > 0 {
			out["input_cost"] = valueOrZero(out["input_cost"]) + cacheTotal
		}
		// A charged side that came out at exactly zero is still a side that was
		// priced. Reporting it keeps the console from falling through to a
		// token-proportional estimate, which would invent a number.
		for _, key := range []string{"input_cost", "output_cost"} {
			if _, present := out[key]; !present {
				out[key] = 0.0
			}
		}
		// The sum of the sides is what the rates add up to. It can differ from
		// total_cost above if a model was repriced mid-flight; keeping both lets
		// the console show the discrepancy instead of hiding it.
		out["original_cost"] = valueOrZero(out["input_cost"]) + valueOrZero(out["output_cost"])
		return out
	}

	// No snapshot: this row predates it, or the call was never priced.
	charge, ok := catalog.CostAt(e.Model, catalog.Usage{
		PromptTokens:     e.PromptTokens,
		CompletionTokens: e.CompletionTokens,
		CachedTokens:     intOrZero(e.CachedTokens),
	}, e.TS)
	if !ok {
		return out
	}
	// A recomputed row reports the cache sides the way costBreakdown has always
	// shaped them for the console - folded into input_cost, with the cached part
	// broken out separately - so the drawer does not have to know which of the
	// two shapes it is reading.
	out["input_cost"] = charge.Input + charge.Cache
	out["output_cost"] = charge.Output
	if charge.Cache > 0 {
		out["cache_read_cost"] = charge.Cache
	}
	out["original_cost"] = charge.Total
	out["window"] = charge.Window
	out["applied"] = appliedRateRows(charge.Applied)
	out["source"] = "recomputed"
	return out
}

// parsePriceSnapshot decodes a stored snapshot. A row that was never priced has
// an empty column, and a payload that does not decode is treated the same way
// rather than failing the whole log page.
// 参数 raw（string）：usage_events.price_snapshot 里的 JSON。
// 返回 *catalog.PriceSnapshot（*catalog.PriceSnapshot）：解出来的快照。为空或解不出时为 nil。
// 调用：costBreakdown。
// 测试：cost_breakdown_test.go
func parsePriceSnapshot(raw string) *catalog.PriceSnapshot {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}
	var snap catalog.PriceSnapshot
	if err := json.Unmarshal([]byte(trimmed), &snap); err != nil {
		logx.Error("price snapshot on a usage row is not readable err=%v", err)
		return nil
	}
	if len(snap.Applied) == 0 && snap.PricingStatus != "unpriced" {
		return nil
	}
	return &snap
}

// appliedRateRows renders the rates a call used for the log detail drawer. Each
// row carries its own unit so the console does not have to assume every model is
// billed per token, and the key it came from so an operator can match the row
// back to the market listing that priced it.
// 参数 applied（[]catalog.AppliedRate）：这次调用实际用到的费率。
// 返回 []map[string]any（[]map[string]any）：可以直接写进响应的费率行。
// 调用：costBreakdown。
// 测试：cost_breakdown_test.go
func appliedRateRows(applied []catalog.AppliedRate) []map[string]any {
	out := make([]map[string]any, 0, len(applied))
	for _, rate := range applied {
		out = append(out, map[string]any{
			"measure":    rate.Measure,
			"side":       rate.Side,
			"variant":    rate.Variant,
			"unit_size":  rate.UnitSize,
			"usd":        rate.USD,
			"quantity":   rate.Quantity,
			"cost":       rate.Quantity * rate.USD,
			"source_key": rate.SourceKey,
		})
	}
	return out
}

// valueOrZero reads a number already placed in the response map, treating a
// missing side as zero so the sides can be summed.
// 参数 v（any）：已经放进响应里的一个金额。
// 返回 float64（float64）：读到的小数。还不是数字时为 0。
// 调用：costBreakdown。
// 测试：cost_breakdown_test.go
func valueOrZero(v any) float64 {
	f, _ := v.(float64)
	return f
}

// intOrZero dereferences an optional count. A call that never reported cached
// tokens has no value, which is the same as none being cached for the purpose
// of pricing.
// 参数 v（*int）：一个可选的计数，来自用量行。
// 返回 int（int）：指针里的数。指针为空时为 0。
// 调用：costBreakdown。
// 测试：cost_breakdown_test.go
func intOrZero(v *int) int {
	if v == nil {
		return 0
	}
	return *v
}

// logPageResponse 返回分页信封；total 表示当前行粒度的总行数，空结果的 total_pages 为 0。
// 参数 rows 为当前页，total 为筛选后的总行数，page 从 1 开始，pageSize 必须为正数。
// 返回 map[string]any（map[string]any）：给响应或报表用的 JSON 对象。键是前端已经约定的字段，缺键表示这项没有数据。
// 调用：仅在 reports.go 内使用
// 测试：无直接单测
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

// LogByID serves GET /spend/logs/ui/{request_id} and returns one log with its stored request and response bodies. The row must first be visible through the caller's log scope. Only then is the body read
// . A row outside the scope answers 404, not 403: a caller must not be able to probe which request ids exist. When a platform administrator reads a log that is not their own, the read is audited. The audit row names the request being read, so the access is recorded even though the content leaves no other trace.
// 参数 s（Host）：日志按标识使用的数据面宿主；w（http.ResponseWriter）：调用方的 HTTP 响应，状态和正文写在这里；r（*http.Request）：入站 HTTP 请求。
// 返回：无。状态码和正文写进调用方的响应。
// 调用：gateway/usage/mount.go
// 测试：无直接单测
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
	// 诊断仅进入鉴权后的详情，列表不暴露上游响应头。
	if body.UpstreamResponse != "" {
		metadata := row["metadata"].(map[string]any)
		metadata["upstream_response"] = jsonOrEmpty(body.UpstreamResponse)
	}
	row["messages"] = jsonOrEmpty(body.RequestBody)
	row["response"] = jsonOrText(body.ResponseBody)
	row["error"] = body.Error
	if strings.TrimSpace(body.ProxyRequest) != "" {
		row["proxy_server_request"] = jsonOrEmpty(body.ProxyRequest)
	}
	httpx.WriteJSON(w, 200, row)
}

// jsonOrEmpty decodes a stored body for display. A body that is not JSON, or is empty, is returned as an empty document rather than as a parse error.
// 参数 raw（string）：JSON或空使用的原始内容。空串表示调用方没有提供这项。
// 返回 any（any）：JSON或空的结果。具体类型由调用方断言。
// 调用：仅在 reports.go 内使用
// 测试：无直接单测
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

// jsonOrText 为日志详情解码响应；JSON 返回原结构，非 JSON 返回未经改写的原始文本。
// 参数 raw：数据库中保存的响应正文；空白表示没有响应。
// 返回：JSON 值、原始字符串或空对象。调用场景是详情页 response 字段，无副作用。
// 测试：reports_test.go 覆盖 JSON、纯文本和空输入。
func jsonOrText(raw string) any {
	if strings.TrimSpace(raw) == "" {
		return map[string]any{}
	}
	var out any
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return raw
	}
	return out
}

// Activity returns the global usage time series. Platform administrators only.
// 参数 s（Host）：活动使用的数据面宿主；w（http.ResponseWriter）：调用方的 HTTP 响应，状态和正文写在这里；r（*http.Request）：入站 HTTP 请求。
// 返回：无。状态码和正文写进调用方的响应。
// 调用：gateway/usage/mount.go
// 测试：无直接单测
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
// 参数 s（Host）：活动模型使用的数据面宿主；w（http.ResponseWriter）：调用方的 HTTP 响应，状态和正文写在这里；r（*http.Request）：入站 HTTP 请求。
// 返回：无。状态码和正文写进调用方的响应。
// 调用：gateway/usage/mount.go
// 测试：无直接单测
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

// ActivityCacheHits returns the global cache-hit summary. Cache hits are not a stored dimension of the roll-up, so the counters are reported as zero rather than guessed from a field that does not exist.
// 参数 s（Host）：活动缓存Hits使用的数据面宿主；w（http.ResponseWriter）：调用方的 HTTP 响应，状态和正文写在这里；r（*http.Request）：入站 HTTP 请求。
// 返回：无。状态码和正文写进调用方的响应。
// 调用：gateway/usage/mount.go
// 测试：无直接单测
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
// 参数 s（Host）：花费Logs使用的数据面宿主；w（http.ResponseWriter）：调用方的 HTTP 响应，状态和正文写在这里；r（*http.Request）：入站 HTTP 请求。
// 返回：无。状态码和正文写进调用方的响应。
// 调用：gateway/usage/mount.go
// 测试：无直接单测
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
// 参数 s（Host）：花费密钥使用的数据面宿主；w（http.ResponseWriter）：调用方的 HTTP 响应，状态和正文写在这里；r（*http.Request）：入站 HTTP 请求。
// 返回：无。状态码和正文写进调用方的响应。
// 调用：gateway/usage/mount.go
// 测试：无直接单测
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
// 参数 s（Host）：花费模型使用的数据面宿主；w（http.ResponseWriter）：调用方的 HTTP 响应，状态和正文写在这里；r（*http.Request）：入站 HTTP 请求。
// 返回：无。状态码和正文写进调用方的响应。
// 调用：gateway/usage/mount.go
// 测试：无直接单测
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

// SpendProvider totals spend by provider. The provider is derived from the model name, which is the only provider evidence a usage row carries.
// 参数 s（Host）：花费供应商使用的数据面宿主；w（http.ResponseWriter）：调用方的 HTTP 响应，状态和正文写在这里；r（*http.Request）：入站 HTTP 请求。
// 返回：无。状态码和正文写进调用方的响应。
// 调用：gateway/usage/mount.go
// 测试：无直接单测
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
// 参数 s（Host）：花费Teams使用的数据面宿主；w（http.ResponseWriter）：调用方的 HTTP 响应，状态和正文写在这里；r（*http.Request）：入站 HTTP 请求。
// 返回：无。状态码和正文写进调用方的响应。
// 调用：gateway/usage/mount.go
// 测试：无直接单测
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

// SpendTags totals spend by tag. Tags are not a stored dimension of the usage row, so the answer is empty rather than invented.
// 参数 s（Host）：花费Tags使用的数据面宿主；w（http.ResponseWriter）：调用方的 HTTP 响应，状态和正文写在这里；r（*http.Request）：入站 HTTP 请求。
// 返回：无。状态码和正文写进调用方的响应。
// 调用：gateway/usage/mount.go
// 测试：无直接单测
func SpendTags(s Host, w http.ResponseWriter, r *http.Request) {
	if s.RequireManage(w, r) == nil {
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"spend_per_tag": []any{}})
}

// SpendTagNames returns tag names that have appeared.
// 参数 s（Host）：花费Tag名称使用的数据面宿主；w（http.ResponseWriter）：调用方的 HTTP 响应，状态和正文写在这里；r（*http.Request）：入站 HTTP 请求。
// 返回：无。状态码和正文写进调用方的响应。
// 调用：gateway/usage/mount.go
// 测试：无直接单测
func SpendTagNames(s Host, w http.ResponseWriter, r *http.Request) {
	if s.RequireManage(w, r) == nil {
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"tag_names": []any{}})
}

// SpendEndUsers totals spend by end user. An end user is a LiteLLM concept the new ownership model does not carry, so the answer is empty.
// 参数 s（Host）：花费终点Users使用的数据面宿主；w（http.ResponseWriter）：调用方的 HTTP 响应，状态和正文写在这里；r（*http.Request）：入站 HTTP 请求。
// 返回：无。状态码和正文写进调用方的响应。
// 调用：gateway/usage/mount.go
// 测试：无直接单测
func SpendEndUsers(s Host, w http.ResponseWriter, r *http.Request) {
	if s.RequireManage(w, r) == nil {
		return
	}
	httpx.WriteJSON(w, 200, []any{})
}

// Keys returns the same totals as SpendKeys, for the console's older route.
// 参数 s（Host）：密钥使用的数据面宿主；w（http.ResponseWriter）：调用方的 HTTP 响应，状态和正文写在这里；r（*http.Request）：入站 HTTP 请求。
// 返回：无。状态码和正文写进调用方的响应。
// 调用：gateway/usage/mount.go
// 测试：无直接单测
func Keys(s Host, w http.ResponseWriter, r *http.Request) {
	SpendKeys(s, w, r)
}

// Users totals spend by account. Platform administrators only.
// 参数 s（Host）：Users使用的数据面宿主；w（http.ResponseWriter）：调用方的 HTTP 响应，状态和正文写在这里；r（*http.Request）：入站 HTTP 请求。
// 返回：无。状态码和正文写进调用方的响应。
// 调用：gateway/usage/mount.go
// 测试：无直接单测
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

// TagList returns the tag catalog read by the usage filter and the key form. Tags are not part of the new model, so the catalog is empty rather than a key-value namespace that nothing writes.
// 参数 s（Host）：Tag列表使用的数据面宿主；w（http.ResponseWriter）：调用方的 HTTP 响应，状态和正文写在这里；r（*http.Request）：入站 HTTP 请求。
// 返回：无。状态码和正文写进调用方的响应。
// 调用：gateway/usage/mount.go
// 测试：无直接单测
func TagList(s Host, w http.ResponseWriter, r *http.Request) {
	if s.RequireManage(w, r) == nil {
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{})
}

// Tags totals spend by tag.
// 参数 s（Host）：Tags使用的数据面宿主；w（http.ResponseWriter）：调用方的 HTTP 响应，状态和正文写在这里；r（*http.Request）：入站 HTTP 请求。
// 返回：无。状态码和正文写进调用方的响应。
// 调用：gateway/usage/mount.go
// 测试：无直接单测
func Tags(s Host, w http.ResponseWriter, r *http.Request) {
	if s.RequireManage(w, r) == nil {
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"spend_per_tag": []any{}})
}

// Calculate estimates spend from a model and token counts and does not write the database. An unknown model returns 0 rather than an error, because the caller is showing an estimate and a missing priceis not a failed request.
// 参数 s（Host）：Calculate使用的数据面宿主；w（http.ResponseWriter）：调用方的 HTTP 响应，状态和正文写在这里；r（*http.Request）：入站 HTTP 请求。
// 返回：无。状态码和正文写进调用方的响应。
// 调用：gateway/usage/mount.go
// 测试：无直接单测
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
	// An estimate is priced at the instant it is asked for, because that is the
	// best guess available: the caller has not made the call yet. A window-priced
	// model therefore quotes its peak rate during peak hours, which is what the
	// call would actually cost if made then.
	charge, ok := catalog.CostAt(model, catalog.Usage{
		PromptTokens:     prompt,
		CompletionTokens: completion,
	}, time.Now())
	total := 0.0
	if ok {
		total = charge.Total
	}
	httpx.WriteJSON(w, 200, map[string]any{"cost": total})
}

// HealthTestConnection checks whether an upstream or dependency is reachable. A failure writes the reason in JSON and is not always a 500.
// 参数 s（Host）：HealthTestConnection使用的数据面宿主；w（http.ResponseWriter）：调用方的 HTTP 响应，状态和正文写在这里；r（*http.Request）：入站 HTTP 请求。
// 返回：无。状态码和正文写进调用方的响应。
// 调用：gateway/usage/mount.go
// 测试：无直接单测
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
// 参数 s（Host）：HealthServices使用的数据面宿主；w（http.ResponseWriter）：调用方的 HTTP 响应，状态和正文写在这里；r（*http.Request）：入站 HTTP 请求。
// 返回：无。状态码和正文写进调用方的响应。
// 调用：gateway/usage/mount.go
// 测试：gateway/dropped_alerts_test.go
func HealthServices(s Host, w http.ResponseWriter, r *http.Request) {
	if s.RequireManage(w, r) == nil {
		return
	}
	switch strings.ToLower(strings.TrimSpace(r.URL.Query().Get("service"))) {
	case "email", "ms_teams", "msteams", "slack":
		httpx.WriteError(w, 400, "invalid_request", "alerts are not supported")
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{
		"healthy_services":   []any{},
		"unhealthy_services": []any{},
		"status":             "healthy",
	})
}

// HealthTest runs one health test against the named target. It is public: it proves the process is serving and says nothing about any tenant.
// 参数 s（Host）：HealthTest使用的数据面宿主；w（http.ResponseWriter）：调用方的 HTTP 响应，状态和正文写在这里；r（*http.Request）：入站 HTTP 请求。
// 返回：无。状态码和正文写进调用方的响应。
// 调用：gateway/usage/mount.go
// 测试：无直接单测
func HealthTest(s Host, w http.ResponseWriter, r *http.Request) {
	httpx.SetCallID(w, httpx.CallID())
	httpx.WriteJSON(w, 200, map[string]any{"status": "ok", "message": "LiteLLM Proxy is running"})
}

// openGlobal requires a platform administrator and returns the unscoped usage scope. The global spend family is a platform-wide view by definition, so it takes the administrator scope rather than a narrowed one.
// 参数 s（Host）：打开Global使用的数据面宿主；w（http.ResponseWriter）：调用方的 HTTP 响应，状态和正文写在这里；r（*http.Request）：入站 HTTP 请求。
// 返回 Scope（*authz.Scope）：平台管理员的不缩小用量范围。不是管理员或鉴权失败时为 nil；bool（bool）：调用方是平台管理员且拿到了不缩小的用量范围时返回真。
// 调用：仅在 reports.go 内使用
// 测试：无直接单测
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

// globalQuery builds the query for a global report from the request's window and filters.
// 参数 r（*http.Request）：入站 HTTP 请求；sc（*authz.Scope）：global查询使用的权限范围。
// 返回 UsageQuery（iam.UsageQuery）：全局报表的查询。时间窗和筛选都落在权限范围之内。
// 调用：仅在 reports.go 内使用
// 测试：无直接单测
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

// globalDaily reads the daily roll-up for the window. Reports over a day already aggregated do not re-read the events.
// 参数 s（Host）：global按天使用的数据面宿主；r（*http.Request）：入站 HTTP 请求；sc（*authz.Scope）：global按天使用的权限范围。
// 返回 []iam.DailyRow（[]iam.DailyRow）：这个时间窗里按天汇总的用量。没有行时为空切片；error（error）：库不可用或查询失败。nil 表示成功。
// 调用：仅在 reports.go 内使用
// 测试：无直接单测
func globalDaily(s Host, r *http.Request, sc *authz.Scope) ([]iam.DailyRow, error) {
	db := s.Identity()
	if db == nil {
		return nil, nil
	}
	return db.DailyUsage(r.Context(), globalQuery(r, sc), 0)
}

// spendDay takes the date from a timestamp for day filters.
// 参数 ts（string）：用量事件上的时间戳。空串表示用今天的 UTC 日期。
// 返回 string（string）：用于按日筛选的日历日期，格式 YYYY-MM-DD。解析不了时取前 10 个字符。
// 调用：仅在 reports.go 内使用
// 测试：无直接单测
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
