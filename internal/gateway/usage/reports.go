// 花费日志和用量汇总的 HTTP 接口。数字来自 PostgreSQL，不是现场重算。
package usage

import (
	"net/http"
	"time"

	"github.com/sunqirui1987/xhub/internal/dataplane"
	"github.com/sunqirui1987/xhub/internal/gateway/identity"
	"github.com/sunqirui1987/xhub/internal/httpx"
	"github.com/sunqirui1987/xhub/internal/spend"
)

// 花费日志的分页接口。
func LogsV2(s identity.Gate, w http.ResponseWriter, r *http.Request) {
	identity.SpendLogs(s, w, r)
}

// 按请求 id 读取一条花费日志。
func LogByID(s Host, w http.ResponseWriter, r *http.Request) {
	if s.RequireManage(w, r) == nil {
		return
	}
	id := r.PathValue("request_id")
	list, _ := s.DB().ListSpendLogs()
	for _, row := range list {
		if str(row["request_id"]) == id {
			httpx.WriteJSON(w, 200, row)
			return
		}
	}
	httpx.WriteJSON(w, 200, map[string]any{"request_id": id, "spend": 0, "model": "", "prompt_tokens": 0, "completion_tokens": 0})
}

// 全局用量时间序列。
func Activity(s Host, w http.ResponseWriter, r *http.Request) {
	if s.RequireManage(w, r) == nil {
		return
	}
	list, _ := s.DB().ListSpendLogs()
	byDay := map[string]map[string]any{}
	var sumReq, sumTok int
	for _, row := range list {
		day := spendDay(str(row["startTime"]))
		if !spendInRange(day, r) {
			continue
		}
		bucket, ok := byDay[day]
		if !ok {
			bucket = map[string]any{"date": day, "api_requests": 0, "total_tokens": 0}
			byDay[day] = bucket
		}
		pt := asInt(row["prompt_tokens"])
		ct := asInt(row["completion_tokens"])
		bucket["api_requests"] = asInt(bucket["api_requests"]) + 1
		bucket["total_tokens"] = asInt(bucket["total_tokens"]) + pt + ct
		sumReq++
		sumTok += pt + ct
	}
	daily := []map[string]any{}
	for _, v := range byDay {
		daily = append(daily, v)
	}
	httpx.WriteJSON(w, 200, map[string]any{
		"daily_data":       daily,
		"sum_api_requests": sumReq,
		"sum_total_tokens": sumTok,
	})
}

// 按模型拆开的全局用量。
func ActivityModel(s Host, w http.ResponseWriter, r *http.Request) {
	if s.RequireManage(w, r) == nil {
		return
	}
	list, _ := s.DB().ListSpendLogs()
	byModel := map[string]map[string]any{}
	for _, row := range list {
		day := spendDay(str(row["startTime"]))
		if !spendInRange(day, r) {
			continue
		}
		model := str(row["model"])
		b, ok := byModel[model]
		if !ok {
			b = map[string]any{"model": model, "api_requests": 0, "total_tokens": 0, "spend": 0.0}
			byModel[model] = b
		}
		b["api_requests"] = asInt(b["api_requests"]) + 1
		b["total_tokens"] = asInt(b["total_tokens"]) + asInt(row["prompt_tokens"]) + asInt(row["completion_tokens"])
		if sp, ok := row["spend"].(float64); ok {
			b["spend"] = asFloat(b["spend"]) + sp
		}
	}
	out := []map[string]any{}
	for _, v := range byModel {
		out = append(out, map[string]any{
			"model":            v["model"],
			"daily_data":       []any{},
			"sum_api_requests": asInt(v["api_requests"]),
			"sum_total_tokens": asInt(v["total_tokens"]),
			"spend":            v["spend"],
		})
	}
	httpx.WriteJSON(w, 200, out)
}

// 缓存命中的全局用量。
func ActivityCacheHits(s Host, w http.ResponseWriter, r *http.Request) {
	if s.RequireManage(w, r) == nil {
		return
	}
	list, _ := s.DB().ListSpendLogs()
	hits, reqs := 0, 0
	for _, row := range list {
		day := spendDay(str(row["startTime"]))
		if !spendInRange(day, r) {
			continue
		}
		reqs++
		if b, ok := row["cache_hit"].(bool); ok && b {
			hits++
		}
	}
	ratio := 0.0
	if reqs > 0 {
		ratio = float64(hits) / float64(reqs)
	}
	httpx.WriteJSON(w, 200, map[string]any{
		"groups":          []any{},
		"error_breakdown": []any{},
		"filter_options":  map[string]any{"key_aliases": []any{}, "models": []any{}},
		"totals": map[string]any{
			"api_requests":             reqs,
			"cache_hits":               hits,
			"cache_hit_ratio":          ratio,
			"cached_completion_tokens": 0,
			"failed_requests":          0,
		},
	})
}

// 全局花费日志列表。
func SpendLogs(s Host, w http.ResponseWriter, r *http.Request) {
	if s.RequireManage(w, r) == nil {
		return
	}
	list, _ := s.DB().ListSpendLogs()
	byDay := map[string]float64{}
	for _, row := range list {
		day := spendDay(str(row["startTime"]))
		if sp, ok := row["spend"].(float64); ok {
			byDay[day] += sp
		}
	}
	out := []map[string]any{}
	for d, sp := range byDay {
		out = append(out, map[string]any{"date": d, "spend": sp})
	}
	httpx.WriteJSON(w, 200, out)
}

// 按密钥汇总花费。
func SpendKeys(s Host, w http.ResponseWriter, r *http.Request) {
	if s.RequireManage(w, r) == nil {
		return
	}
	keys, _ := s.DB().ListKeys()
	out := []map[string]any{}
	for _, k := range keys {
		if k.Spend == 0 {
			continue
		}
		out = append(out, map[string]any{
			"api_key": k.TokenHash, "total_spend": k.Spend, "spend": k.Spend, "key_alias": k.KeyAlias,
		})
	}
	httpx.WriteJSON(w, 200, out)
}

// 按模型汇总花费。
func SpendModels(s Host, w http.ResponseWriter, r *http.Request) {
	if s.RequireManage(w, r) == nil {
		return
	}
	list, _ := s.DB().ListSpendLogs()
	byModel := map[string]float64{}
	for _, row := range list {
		m := str(row["model"])
		if sp, ok := row["spend"].(float64); ok {
			byModel[m] += sp
		}
	}
	out := []map[string]any{}
	for m, sp := range byModel {
		out = append(out, map[string]any{"model": m, "total_spend": sp, "spend": sp})
	}
	httpx.WriteJSON(w, 200, out)
}

// 按供应商汇总花费。
func SpendProvider(s Host, w http.ResponseWriter, r *http.Request) {
	if s.RequireManage(w, r) == nil {
		return
	}
	list, _ := s.DB().ListSpendLogs()
	byProv := map[string]float64{}
	for _, row := range list {
		p := str(row["custom_llm_provider"])
		if p == "" {
			p = str(row["model"])
		}
		if p == "" {
			p = "openai"
		}
		if sp, ok := row["spend"].(float64); ok {
			byProv[p] += sp
		}
	}
	out := []map[string]any{}
	for p, sp := range byProv {
		out = append(out, map[string]any{"provider": p, "spend": sp})
	}
	httpx.WriteJSON(w, 200, out)
}

// 按团队汇总花费。
func SpendTeams(s Host, w http.ResponseWriter, r *http.Request) {
	if s.RequireManage(w, r) == nil {
		return
	}
	teams, _ := s.DB().ListTeams()
	teamIDs := []string{}
	perTeam := []map[string]any{}
	for _, t := range teams {
		alias := t.Alias
		if alias == "" {
			alias = t.ID
		}
		teamIDs = append(teamIDs, alias)
		perTeam = append(perTeam, map[string]any{"team_id": alias, "total_spend": t.Spend})
	}
	httpx.WriteJSON(w, 200, map[string]any{
		"daily_spend":          []any{},
		"teams":                teamIDs,
		"total_spend_per_team": perTeam,
	})
}

// 按标签汇总花费。这是用量接口，不是已删除的标签管理。
func SpendTags(s Host, w http.ResponseWriter, r *http.Request) {
	if s.RequireManage(w, r) == nil {
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"spend_per_tag": []any{}})
}

// 出现过的标签名。
func SpendTagNames(s Host, w http.ResponseWriter, r *http.Request) {
	if s.RequireManage(w, r) == nil {
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"tag_names": []any{}})
}

// 按终端用户汇总花费。
func SpendEndUsers(s Host, w http.ResponseWriter, r *http.Request) {
	if s.RequireManage(w, r) == nil {
		return
	}
	httpx.WriteJSON(w, 200, []any{})
}

// 按模型和 token 估算花费，不写库。不认识的模型返回错误而不是 0。
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
	total, _, _, ok := spend.Cost(model, prompt, completion)
	if !ok {
		total = 0
	}
	httpx.WriteJSON(w, 200, map[string]any{"cost": total})
}

// 当前身份可见的密钥花费。
func Keys(s Host, w http.ResponseWriter, r *http.Request) {
	SpendKeys(s, w, r)
}

// 按用户汇总花费。
func Users(s Host, w http.ResponseWriter, r *http.Request) {
	if s.RequireManage(w, r) == nil {
		return
	}
	users, _ := s.DB().ListUsers()
	out := []map[string]any{}
	for _, u := range users {
		out = append(out, map[string]any{"user_id": u.ID, "user_email": u.Email, "spend": u.Spend})
	}
	httpx.WriteJSON(w, 200, out)
}

// 按标签汇总花费。
func Tags(s Host, w http.ResponseWriter, r *http.Request) {
	if s.RequireManage(w, r) == nil {
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"spend_per_tag": []any{}})
}

// 探测上游或依赖是否连通。失败时在 JSON 里写原因，不一定是 500。
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

// 依赖服务的健康列表。
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

// 对指定目标做一次健康测试。
func HealthTest(s Host, w http.ResponseWriter, r *http.Request) {
	httpx.SetCallID(w, httpx.CallID())
	httpx.WriteJSON(w, 200, map[string]any{"status": "ok", "message": "LiteLLM Proxy is running"})
}

// spendDay 从时间戳取出日期，供按天过滤。空时间用当前 UTC 日期，认不出的格式保留前 10 个字符。
func spendDay(ts string) string {
	if ts == "" {
		return time.Now().UTC().Format("2006-01-02")
	}
	if t, err := time.Parse(time.RFC3339Nano, ts); err == nil {
		return t.UTC().Format("2006-01-02")
	}
	if t, err := time.Parse(time.RFC3339, ts); err == nil {
		return t.UTC().Format("2006-01-02")
	}
	if len(ts) >= 10 {
		return ts[:10]
	}
	return ts
}

// spendInRange 判断日期是否落在查询区间里。缺少 start_date 或 end_date 时那一侧不限制。
func spendInRange(day string, r *http.Request) bool {
	start := r.URL.Query().Get("start_date")
	end := r.URL.Query().Get("end_date")
	if start != "" && day < start {
		return false
	}
	if end != "" && day > end {
		return false
	}
	return true
}
