package usage

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/sunqirui1987/xhub/internal/httpx"
	"github.com/sunqirui1987/xhub/internal/iam"
)

const guardrailUsageLimit = 5000

// guardrailFinding 是一次已持久化护栏执行的监控视图。
// 参数由 decodeGuardrailFindings 从 UsageEvent 和其中的 Guardrail JSON 填充。
// 返回值由聚合和日志函数读取；它不包含请求或响应正文，避免监控接口泄露敏感内容。
type guardrailFinding struct {
	ID, Name, Provider, Mode, Action, Reason, RequestID, Model string
	Timestamp                                                  time.Time
	LatencyMS                                                  float64
}

// guardrailAggregate 保存一条护栏在查询窗口内的真实执行统计。
// 参数由 aggregateGuardrails 逐条累加；调用方读取计数、延迟和按日趋势，不修改持久化事件。
type guardrailAggregate struct {
	ID, Name, Provider, Mode        string
	Total, Blocked, Flagged, Passed int
	LatencyMS                       float64
	Daily                           map[string]*guardrailDay
}

// guardrailDay 保存单日通过和拦截数量。
// 参数由聚合器根据事件 UTC 日期写入；返回时转换成前端图表点。
type guardrailDay struct{ Passed, Blocked int }

// GuardrailOverview 返回平台范围内真实持久化护栏事件的总览。
// 参数 s 提供管理员鉴权和用量库，w/r 承载日期筛选与响应；仅平台管理员可调用。
// 返回值写入 HTTP 响应；空窗口返回零统计，存储错误交给统一 IAM 错误处理。
func GuardrailOverview(s Host, w http.ResponseWriter, r *http.Request) {
	findings, ok := readGuardrailFindings(s, w, r)
	if !ok {
		return
	}
	aggs := aggregateGuardrails(findings)
	total, blocked, passed := 0, 0, 0
	rows := make([]any, 0, len(aggs))
	allDays := map[string]*guardrailDay{}
	for _, agg := range aggs {
		total += agg.Total
		blocked += agg.Blocked
		passed += agg.Passed
		rows = append(rows, guardrailOverviewRow(agg))
		for day, point := range agg.Daily {
			if allDays[day] == nil {
				allDays[day] = &guardrailDay{}
			}
			allDays[day].Passed += point.Passed
			allDays[day].Blocked += point.Blocked
		}
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"totalRequests": total, "totalBlocked": blocked, "passRate": percentage(passed, total),
		"totalCost": nil, "totalUsageUnits": map[string]float64{}, "totalUntrackedUsageUnits": map[string]float64{},
		"rows": rows, "chart": guardrailChart(allDays),
	})
}

// GuardrailDetail 返回指定护栏在查询窗口内的真实统计明细。
// 参数 guardrail_id 来自路径，日期来自查询串；调用方必须是平台管理员。
// 找不到有事件的护栏时返回 404，成功时返回前端 UsageDetailResponse 的完整字段。
func GuardrailDetail(s Host, w http.ResponseWriter, r *http.Request) {
	findings, ok := readGuardrailFindings(s, w, r)
	if !ok {
		return
	}
	id := strings.TrimSpace(r.PathValue("guardrail_id"))
	var found *guardrailAggregate
	for _, agg := range aggregateGuardrails(findings) {
		if agg.ID == id {
			found = agg
			break
		}
	}
	if found == nil {
		httpx.WriteError(w, http.StatusNotFound, "not_found", "guardrail usage not found")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, guardrailDetailBody(found))
}

// GuardrailLogs 返回指定护栏的真实执行日志并支持 action、日期和分页筛选。
// 参数来自查询串；正文片段始终为空，只暴露持久化动作、原因、模型、耗时和请求标识。
// 返回值写入 HTTP；非法页码回退到安全默认值，管理员鉴权和存储错误由统一流程处理。
func GuardrailLogs(s Host, w http.ResponseWriter, r *http.Request) {
	findings, ok := readGuardrailFindings(s, w, r)
	if !ok {
		return
	}
	id, action := strings.TrimSpace(r.URL.Query().Get("guardrail_id")), strings.TrimSpace(r.URL.Query().Get("action"))
	filtered := make([]guardrailFinding, 0, len(findings))
	for _, finding := range findings {
		if (id == "" || finding.ID == id) && (action == "" || finding.Action == action) {
			filtered = append(filtered, finding)
		}
	}
	sort.SliceStable(filtered, func(i, j int) bool { return filtered[i].Timestamp.After(filtered[j].Timestamp) })
	page, pageSize := queryInt(r, "page", 1), queryInt(r, "page_size", 50)
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = 50
	}
	start := (page - 1) * pageSize
	if start > len(filtered) {
		start = len(filtered)
	}
	end := start + pageSize
	if end > len(filtered) {
		end = len(filtered)
	}
	logs := make([]any, 0, end-start)
	for _, finding := range filtered[start:end] {
		logs = append(logs, map[string]any{"id": finding.RequestID, "timestamp": finding.Timestamp.UTC().Format(time.RFC3339Nano), "action": finding.Action, "score": nil, "model": nullableString(finding.Model), "input_snippet": nil, "output_snippet": nil, "reason": nullableString(finding.Reason), "latency_ms": finding.LatencyMS})
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"logs": logs, "page": page, "page_size": pageSize, "total": len(filtered)})
}

// readGuardrailFindings 完成管理员鉴权、平台用量范围计算和事件读取。
// 参数 s/w/r 与 HTTP 处理函数一致；日期筛选使用 start_date/end_date，最多读取五千条事件。
// 返回已解码结果和成功标志；鉴权、范围或存储失败时已写响应并返回 false。
func readGuardrailFindings(s Host, w http.ResponseWriter, r *http.Request) ([]guardrailFinding, bool) {
	scope, ok := openGlobal(s, w, r)
	if !ok {
		return nil, false
	}
	if s.Identity() == nil {
		return []guardrailFinding{}, true
	}
	q := globalQuery(r, scope)
	q.Limit = guardrailUsageLimit
	events, err := s.Identity().ListUsage(r.Context(), q)
	if err != nil {
		s.WriteIAMError(w, r, err)
		return nil, false
	}
	return decodeGuardrailFindings(events), true
}

// decodeGuardrailFindings 把 UsageEvent.Guardrail JSON 转成监控行。
// 参数 events 是权限和时间窗已经收窄的事件；返回所有合法 finding，空白或损坏 JSON 被跳过。
// 每条结果保留真实请求 ID 和时间，但不复制请求正文；该函数无外部副作用。
func decodeGuardrailFindings(events []iam.UsageEvent) []guardrailFinding {
	out := []guardrailFinding{}
	for _, event := range events {
		if strings.TrimSpace(event.Guardrail) == "" {
			continue
		}
		var raw []map[string]any
		if json.Unmarshal([]byte(event.Guardrail), &raw) != nil {
			continue
		}
		for _, row := range raw {
			id := anyString(row["guardrail_id"])
			name := anyString(row["guardrail_name"])
			if id == "" {
				id = name
			}
			if name == "" {
				name = id
			}
			if id == "" {
				continue
			}
			response, _ := row["guardrail_response"].(map[string]any)
			action := normalizeGuardrailAction(anyString(response["action"]), anyString(row["guardrail_status"]))
			out = append(out, guardrailFinding{ID: id, Name: name, Provider: anyString(row["guardrail_provider"]), Mode: anyString(row["guardrail_mode"]), Action: action, Reason: anyString(row["reason"]), RequestID: event.RequestID, Model: event.Model, Timestamp: event.TS, LatencyMS: anyFloat(row["duration"]) * 1000})
		}
	}
	return out
}

// aggregateGuardrails 按护栏 ID 汇总执行结果。
// 参数 findings 是已解码的真实事件；返回按 ID 升序排列的聚合，便于 API 和测试稳定比较。
// flagged 计入失败率但不计入 Blocked Requests，按日图中的 blocked 也只表示真实拦截。
func aggregateGuardrails(findings []guardrailFinding) []*guardrailAggregate {
	byID := map[string]*guardrailAggregate{}
	for _, finding := range findings {
		agg := byID[finding.ID]
		if agg == nil {
			agg = &guardrailAggregate{ID: finding.ID, Name: finding.Name, Provider: finding.Provider, Mode: finding.Mode, Daily: map[string]*guardrailDay{}}
			byID[finding.ID] = agg
		}
		agg.Total++
		agg.LatencyMS += finding.LatencyMS
		day := finding.Timestamp.UTC().Format("2006-01-02")
		if agg.Daily[day] == nil {
			agg.Daily[day] = &guardrailDay{}
		}
		switch finding.Action {
		case "blocked":
			agg.Blocked++
			agg.Daily[day].Blocked++
		case "flagged":
			agg.Flagged++
		default:
			agg.Passed++
			agg.Daily[day].Passed++
		}
	}
	ids := make([]string, 0, len(byID))
	for id := range byID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]*guardrailAggregate, 0, len(ids))
	for _, id := range ids {
		out = append(out, byID[id])
	}
	return out
}

// guardrailOverviewRow 把一个聚合转换为总览表行。
// 参数 agg 必须来自 aggregateGuardrails；返回字段符合 UsageOverviewRow，不修改聚合。
func guardrailOverviewRow(agg *guardrailAggregate) map[string]any {
	failed := agg.Blocked + agg.Flagged
	return map[string]any{"id": agg.ID, "name": agg.Name, "provider": defaultString(agg.Provider, "xhub"), "type": defaultString(agg.Mode, "pre_call"), "requestsEvaluated": agg.Total, "failRate": percentage(failed, agg.Total), "avgScore": nil, "avgLatency": average(agg.LatencyMS, agg.Total), "status": guardrailHealth(failed, agg.Total), "trend": "stable", "cost": nil, "usageUnits": map[string]float64{}, "untrackedUsageUnits": map[string]float64{}}
}

// guardrailDetailBody 把聚合转换为详情接口完整结构。
// 参数 agg 是目标护栏的统计；返回空费用/用量映射表示当前护栏事件未记录计费单位。
func guardrailDetailBody(agg *guardrailAggregate) map[string]any {
	row := guardrailOverviewRow(agg)
	return map[string]any{"guardrail_id": agg.ID, "guardrail_name": agg.Name, "description": nil, "provider": row["provider"], "type": row["type"], "status": row["status"], "requestsEvaluated": agg.Total, "failRate": row["failRate"], "avgScore": nil, "avgLatency": row["avgLatency"], "trend": "stable", "cost": nil, "cost_by_key": map[string]any{}, "cost_by_team": map[string]any{}, "cost_by_unit": map[string]any{}, "time_series": guardrailChart(agg.Daily), "usage_units": map[string]float64{}, "usage_units_by_key": map[string]any{}, "usage_units_by_team": map[string]any{}, "usage_units_daily": []any{}, "untracked_usage_units": map[string]float64{}, "untracked_usage_units_by_key": map[string]any{}, "untracked_usage_units_by_team": map[string]any{}}
}

// guardrailChart 按日期升序生成趋势点。
// 参数 daily 是日期到真实通过/拦截数的映射；返回前端图表结构，空输入返回非 nil 空数组。
func guardrailChart(daily map[string]*guardrailDay) []any {
	days := make([]string, 0, len(daily))
	for day := range daily {
		days = append(days, day)
	}
	sort.Strings(days)
	out := make([]any, 0, len(days))
	for _, day := range days {
		out = append(out, map[string]any{"date": day, "passed": daily[day].Passed, "blocked": daily[day].Blocked, "score": nil})
	}
	return out
}

// normalizeGuardrailAction 统一执行器动作和历史状态为前端支持的三种动作。
// 参数 action/status 来自持久化 finding；返回 blocked、flagged 或 passed，未知成功动作按 passed 处理。
func normalizeGuardrailAction(action, status string) string {
	if action == "block" || status == "blocked" {
		return "blocked"
	}
	if action == "flag" || action == "redact" || action == "modify" || status == "guardrail_flagged" {
		return "flagged"
	}
	return "passed"
}

// guardrailHealth 根据失败比例生成页面状态。
// 参数 failed/total 是同一护栏计数；返回 healthy、warning 或 critical，空统计为 healthy。
func guardrailHealth(failed, total int) string {
	rate := percentage(failed, total)
	if rate > 15 {
		return "critical"
	}
	if rate > 5 {
		return "warning"
	}
	return "healthy"
}

// percentage 计算百分比并处理零分母。
// 参数 part/total 是计数；返回 0 到 100 的浮点百分比，total 非正时返回零。
func percentage(part, total int) float64 {
	if total <= 0 {
		return 0
	}
	return float64(part) * 100 / float64(total)
}

// average 计算总量的算术平均值并处理零分母。
// 参数 total/count 分别是累计值和次数；返回平均值，count 非正时返回零。
func average(total float64, count int) float64 {
	if count <= 0 {
		return 0
	}
	return total / float64(count)
}

// anyString 安全读取 JSON 字符串。
// 参数 v 是动态 JSON 值；返回字符串，其他类型返回空串。
func anyString(v any) string { s, _ := v.(string); return strings.TrimSpace(s) }

// anyFloat 安全读取 JSON 数字。
// 参数 v 是动态 JSON 值；返回可识别数值，其他类型返回零。
func anyFloat(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case int64:
		return float64(n)
	}
	return 0
}

// defaultString 为缺失展示字段提供稳定默认值。
// 参数 value/fallback 是原值和回退值；返回首个非空字符串。
func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}

// nullableString 把空字符串转换为 JSON null。
// 参数 value 是可选文本；返回非空字符串或 nil。
func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}
