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

// UserDailyActivity is GET /user/daily/activity. It groups stored usage into the daily rollup the usage page reads, newest day first and paged. The entity breakdown is per user, which is what "用户用量" renders, and the scope is the caller's own rows plus the teams they oversee.
// 参数 s（Host）：用户按天活动使用的数据面宿主；w（http.ResponseWriter）：调用方的 HTTP 响应，状态和正文写在这里；r（*http.Request）：入站 HTTP 请求。
// 返回：无。状态码和正文写进调用方的响应。
// 调用：gateway/usage/mount.go
// 测试：无直接单测
func UserDailyActivity(s Host, w http.ResponseWriter, r *http.Request) {
	logTraceOnceActivity.Do(func() { logx.Trace("enter usage.UserDailyActivity") })
	writeDailyActivity(s, w, r, entityUser, false)
}

// UserDailyActivityAggregated is GET /user/daily/activity/aggregated. The same rollup in a single response, which is what "你的用量" and the global view load first. A user id on the query narrows to that account inside the scope.
// 参数 s（Host）：用户按天活动聚合使用的数据面宿主；w（http.ResponseWriter）：调用方的 HTTP 响应，状态和正文写在这里；r（*http.Request）：入站 HTTP 请求。
// 返回：无。状态码和正文写进调用方的响应。
// 调用：gateway/usage/mount.go
// 测试：无直接单测
func UserDailyActivityAggregated(s Host, w http.ResponseWriter, r *http.Request) {
	writeDailyActivity(s, w, r, entityUser, true)
}

// GatewayDailyActivity is GET /gateway/daily/activity. Request counts come from the same usage rows, split by outcome and route. The scope decides whose calls are counted: a platform administrator seesthe gateway, anyone else sees only their own calls and the calls inside the teams they belong to.
// 参数 s（Host）：网关按天活动使用的数据面宿主；w（http.ResponseWriter）：调用方的 HTTP 响应，状态和正文写在这里；r（*http.Request）：入站 HTTP 请求。
// 返回：无。状态码和正文写进调用方的响应。
// 调用：gateway/usage/mount.go
// 测试：无直接单测
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

// openActivity accepts any signed-in caller and returns the usage scope that narrows every row the handler will read. A key reaches only its own usage; a session reaches its own rows plus the teams it belongs to; a platform administrator reaches everything. The scope is required rather than optional, so a handler cannot fall through to an unfiltered read.
// 参数 s（Host）：打开活动使用的数据面宿主；w（http.ResponseWriter）：调用方的 HTTP 响应，状态和正文写在这里；r（*http.Request）：入站 HTTP 请求；teamID（string）：团队 id。空串表示没有指定团队。
// 返回 Scope（*authz.Scope）：缩小到调用方可见用量的范围。未登录或计算失败时为 nil；bool（bool）：调用方已登录且拿到了用量范围时返回真。密钥只能看自己的用量。
// 调用：仅在 activity.go 内使用
// 测试：无直接单测
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

// 把一次调用的费用、token 和成败累加进这个指标。
// 参数 prompt（int）：提示 token 数，用来估价；completion（int）：完成 token 数，用来估价；spend（float64）：这一行要累加的费用，单位是美元；success（bool）：为真时这次调用计入成功次数。
// 返回：无。这次调用的费用、token 和成败已累加进指标。成功计入成功次数，否则计入失败次数。
// 调用：gateway/usage/entity_activity.go。
// 测试：无直接单测
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

// 把另一组指标累加进当前指标。
// 参数 other（metric）：合并使用的metric。
// 返回：无。另一组指标的费用、token 和次数已加进当前指标。
// 调用：仅在 activity.go 内使用
// 测试：无直接单测
func (m *metric) fold(other metric) {
	m.spend += other.spend
	m.prompt += other.prompt
	m.completion += other.completion
	m.requests += other.requests
	m.success += other.success
	m.failed += other.failed
}

// 把指标收成报表 JSON。费用和 token 用这里累加的值。
// 参数：无。
// 返回 map[string]any（map[string]any）：给响应或报表用的 JSON 对象。键是前端已经约定的字段，缺键表示这项没有数据。
// 调用：gateway/usage/entity_activity.go
// 测试：无直接单测
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

// loadActivity reads the usage rows the caller may see and turns them into the flat rows the rollup folds. The scope is applied first and the query's own filters only narrow within it, never widen it. That ordering matters: `user_id` and `api_key` arrive straight from the query string, so a caller could otherwise ask for another account's rows by passing their id. Inside the scope the worst that reaches is a row the caller was already allowed to see, and a filter that names someone else simply returns the empty intersection.
// 参数 s（Host）：载入活动使用的数据面宿主；r（*http.Request）：入站 HTTP 请求；sc（*authz.Scope）：载入活动使用的权限范围。
// 返回 []activityRow（[]activityRow）：调用方能看见的用量，收成汇总要折叠的扁平行。先套权限范围，再套筛选；error（error）：查询失败。nil 表示成功，没有行时为空切片。
// 调用：仅在 activity.go 内使用
// 测试：无直接单测
func loadActivity(s Host, r *http.Request, sc *authz.Scope) ([]activityRow, error) {
	return loadActivityQuery(s, r, activityQuery(r, sc))
}

// loadActivityQuery reads the events for a query that already carries the scope and any narrowing filters, then flattens them for the roll-up.
// 参数 s（Host）：载入活动查询使用的数据面宿主；r（*http.Request）：入站 HTTP 请求；q（iam.UsageQuery）：用量查询条件，含时间范围、团队和分页。
// 返回 []activityRow（[]activityRow）：这条已经带范围和筛选的查询读出的事件，收成扁平行。没有库时为空切片；error（error）：查询失败。nil 表示成功。
// 调用：gateway/usage/entity_activity.go
// 测试：无直接单测
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

// activityQuery builds the scoped query from the request. The scope comes from authz and is never nil, so an actor who may see nothing gets a filter that matches nothing rather than no filter at all.
// 参数 r（*http.Request）：入站 HTTP 请求；sc（*authz.Scope）：活动查询使用的权限范围。
// 返回 UsageQuery（iam.UsageQuery）：从请求的起止日期和筛选收成的用量查询。权限范围来自 authz，先套上，筛选只能在里面收窄。
// 调用：gateway/usage/entity_activity.go
// 测试：无直接单测
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

// eventsToActivity flattens the stored events into the rows the rollup folds. Provider comes from the price map, falling back to the model name so an unknown model is still attributed rather than dropped.
// 参数 events（[]iam.UsageEvent）：events到活动使用的用量事件；tzMinutes（int）：调用方时区相对 UTC 的分钟偏移，用来把日期切到本地日。
// 返回 []activityRow（[]activityRow）：用量事件收成的扁平行，费用为 0 的事件不进入。供应商先查价格表，没有时退回模型名里的前缀。
// 调用：仅在 activity.go 内使用
// 测试：无直接单测
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

// uiTimezone reads the browser's UTC offset in minutes. The console sends Date.getTimezoneOffset(), which is positive west of Greenwich, so the sign is flipped to get the offset east of it that the dayboundary needs.
// 参数 r（*http.Request）：入站 HTTP 请求，用来读路径、头和正文。
// 返回 int（int）：调用方时区相对 UTC 的分钟偏移，东边为正。查询参数 timezone 是浏览器的 getTimezoneOffset，这里取反。缺失或不是整数时为 0，也就是按 UTC。
// 调用：仅在 activity.go 内使用
// 测试：无直接单测
func uiTimezone(r *http.Request) int {
	off, err := strconv.Atoi(r.URL.Query().Get("timezone"))
	if err != nil {
		return 0
	}
	return -off
}

// parseDay reads a YYYY-MM-DD date filter. An unset or unparseable value leaves that side of the window open rather than filtering everything out.
// 参数 v（string）：解析日期使用的值。空串表示调用方没有提供这项。
// 返回 time.Time（time.Time）：解析出的时间。
// 调用：gateway/usage/reports.go
// 测试：activity_test.go
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

// 把用量行收成按天的活动报表。
// 参数 rows（[]activityRow）：从用量或目录读出的用量行；page（int）：页码，从 1 开始；aggregated（bool）：为真时返回汇总值，而不是按天拆开。
// 返回 map[string]any（map[string]any）：给响应或报表用的 JSON 对象。键是前端已经约定的字段，缺键表示这项没有数据。
// 调用：仅在 activity.go 内使用
// 测试：activity_test.go
func dailyActivityResponse(rows []activityRow, page int, aggregated bool) map[string]any {
	return activityBody(rows, page, aggregated, entityNone, entityLabels{})
}

// activityBody 把用量行收成按天、并按请求维度拆开的报表 JSON。
// 参数 rows（[]activityRow）：从用量或目录读出的用量行；page（int）：页码，从 1 开始；aggregated（bool）：为真时返回汇总值，而不是按天拆开；dim（entityDim）：聚合维度，例如团队、组织、用户或密钥；labels（entityLabels）：活动正文使用的id 到展示名。
// 返回 map[string]any（map[string]any）：给响应或报表用的 JSON 对象。键是前端已经约定的字段，缺键表示这项没有数据。
// 调用：gateway/usage/entity_activity.go
// 测试：activity_test.go
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

// 把网关视角的用量收成按天、按路由的报表。
// 参数 rows（[]activityRow）：从用量或目录读出的用量行。
// 返回 map[string]any（map[string]any）：给响应或报表用的 JSON 对象。键是前端已经约定的字段，缺键表示这项没有数据。
// 调用：仅在 activity.go 内使用
// 测试：activity_test.go
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

// 列出有数据的日期，并可按从新到旧排序。
// 参数 days（map[string]*dayMetric）：日期名称使用的map[string]*dayMetric；newestFirst（bool）：为真时日期从新到旧。
// 返回 []string（[]string）：日期名称。没有匹配时为空切片。
// 调用：仅在 activity.go 内使用
// 测试：无直接单测
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

// 按页切出日期。页码小于 1 时从第 1 页开始。
// 参数 names（[]string）：名称列表。空切片表示没有可处理的项；page（int）：页码，从 1 开始。
// 返回 int（int）：分页日期的个数。没有元素时为 0；[]string（[]string）：分页日期。没有匹配时为空切片；int（int）：分页日期的个数。没有元素时为 0。
// 调用：仅在 activity.go 内使用
// 测试：无直接单测
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

// 从查询参数读取页码。缺失或小于 1 时为 1。
// 参数 r（*http.Request）：入站 HTTP 请求，用来读路径、头和正文。
// 返回 int（int）：分页来源查询。解析失败或小于 1 时用约定的默认。
// 调用：gateway/usage/entity_activity.go
// 测试：无直接单测
func pageFromQuery(r *http.Request) int {
	n, err := strconv.Atoi(r.URL.Query().Get("page"))
	if err != nil || n < 1 {
		return 1
	}
	return n
}

// 把用量行按天，并按请求的维度收进桶里。
// 参数 rows（[]activityRow）：从用量或目录读出的用量行；dim（entityDim）：聚合维度，例如团队、组织、用户或密钥。
// 返回 map[string]*dayMetric（map[string]*dayMetric）：汇总日期的字段表。缺键表示上游或库里没有这个字段。
// 调用：仅在 activity.go 内使用
// 测试：无直接单测
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
		// An empty hash is still a key the usage page must list. Dropping ithides every master-key call from the key tab.
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

// 按名称把这一行累加进对应的桶。
// 参数 into（map[string]*namedMetric）：累加命名使用的map[string]*namedMetric；name（string）：累加命名要查找或展示的名称。空串表示还没有命名；row（activityRow）：从用量或目录读出的用量行。
// 返回：无。这一行已按名称累加进对应的桶。桶不存在时先建。
// 调用：仅在 activity.go 内使用
// 测试：无直接单测
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

// 按密钥哈希把这一行累加进密钥桶，并保留别名和团队。
// 参数 into（map[string]*keyMetric）：累加密钥使用的map[string]*keyMetric；hash（string）：密钥哈希，用来对齐热花费和日志；row（activityRow）：从用量或目录读出的用量行。
// 返回：无。这一行已按密钥哈希累加，并保留别名、团队和用户。
// 调用：gateway/usage/entity_activity.go
// 测试：无直接单测
func addKey(into map[string]*keyMetric, hash string, row activityRow) {
	bucket := into[hash]
	if bucket == nil {
		bucket = &keyMetric{alias: row.keyAlias, teamID: row.teamID, userID: row.userID}
		into[hash] = bucket
	}
	bucket.add(row.prompt, row.completion, row.spend, row.success)
}

// 把按名称聚合的桶收成报表对象。
// 参数 in（map[string]*namedMetric）：调用方提交的map[string]*namedMetric。字段为空表示这项不改。
// 返回 map[string]any（map[string]any）：给响应或报表用的 JSON 对象。键是前端已经约定的字段，缺键表示这项没有数据。
// 调用：仅在 activity.go 内使用
// 测试：无直接单测
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

// 把按密钥聚合的桶收成报表对象，别名为空时写成 null。
// 参数 in（map[string]*keyMetric）：调用方提交的map[string]*keyMetric。字段为空表示这项不改。
// 返回 map[string]any（map[string]any）：给响应或报表用的 JSON 对象。键是前端已经约定的字段，缺键表示这项没有数据。
// 调用：gateway/usage/entity_activity.go
// 测试：无直接单测
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

// 收成报表顶部的合计，含费用、token、请求数和页码。
// 参数 total（metric）：这一次的总费用；page（int）：页码，从 1 开始；totalPages（int）：元数据JSON使用的整数。零表示没有这项或尚未计数。
// 返回 map[string]any（map[string]any）：给响应或报表用的 JSON 对象。键是前端已经约定的字段，缺键表示这项没有数据。
// 调用：仅在 activity.go 内使用
// 测试：无直接单测
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

// 空字符串改成 nil，这样 JSON 里是 null。
// 参数 s（string）：可能为空的文本。空串要变成 nil，避免把空值写成 JSON 字符串。
// 返回 any（any）：非空时是原字符串。空串返回 nil，这样 JSON 里是 null 而不是空字符串。
// 调用：gateway/usage/entity_activity.go
// 测试：无直接单测
func nilIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// activityDay renders the calendar day of an event in the caller's timezone. The roll-up is folded per rendered day, so the offset is applied here rather than in SQL: the event carries a timestamp, nota day.
// 参数 ts（time.Time）：时间点。零值表示调用方没有提供时间；tzMinutes（int）：调用方时区相对 UTC 的分钟偏移，用来把日期切到本地日。
// 返回 string（string）：事件在调用方时区里的日历日，格式 YYYY-MM-DD。时间是零值时用现在。
// 调用：仅在 activity.go 内使用
// 测试：activity_test.go
func activityDay(ts time.Time, tzMinutes int) string {
	if ts.IsZero() {
		return time.Now().UTC().Add(time.Duration(tzMinutes) * time.Minute).Format("2006-01-02")
	}
	return ts.UTC().Add(time.Duration(tzMinutes) * time.Minute).Format("2006-01-02")
}

// 判断这一天是否落在查询的起止日期里。
// 参数 day（string）：日期边界，格式由调用方约定，空串表示这一端不限制；start（string）：日期边界，格式由调用方约定，空串表示这一端不限制；end（string）：日期边界，格式由调用方约定，空串表示这一端不限制。
// 返回 bool（bool）：这一天落在起止日期里时为真。空日期为假。
// 调用：仅在 activity.go 内使用
// 测试：无直接单测
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

// 从价目表或模型名得到供应商名称。
// 参数 model（string）：对外模型名，用来选部署和记用量；prices（map[string]map[string]any）：供应商名称使用的map[string]map[string]any。
// 返回 string（string）：价目表或模型名上的供应商。都没有时给一个小写兜底。
// 调用：gateway/usage/reports.go
// 测试：无直接单测
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

// 把调用类型映射成公开路径。
// 参数 callType（string）：操作名或 call_type，写入用量行并选择协议。
// 返回 string（string）：这种调用类型对应的公开路径，例如 chat 对应 /chat/completions。
// 调用：仅在 activity.go 内使用
// 测试：无直接单测
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
