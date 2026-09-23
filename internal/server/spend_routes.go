package server

import (
	"net/http"
	"time"

	"github.com/sunqirui1987/xhub/internal/httpx"
	"github.com/sunqirui1987/xhub/internal/spend"
)

func (s *Server) spendLogsV2(w http.ResponseWriter, r *http.Request) {
	s.spendLogs(w, r)
}

func (s *Server) spendLogByID(w http.ResponseWriter, r *http.Request) {
	if s.requireManage(w, r) == nil {
		return
	}
	id := r.PathValue("request_id")
	list, _ := s.Store.ListSpendLogs()
	for _, row := range list {
		if str(row["request_id"]) == id {
			httpx.WriteJSON(w, 200, row)
			return
		}
	}
	httpx.WriteJSON(w, 200, map[string]any{"request_id": id, "spend": 0, "model": "", "prompt_tokens": 0, "completion_tokens": 0})
}

func (s *Server) globalActivity(w http.ResponseWriter, r *http.Request) {
	if s.requireManage(w, r) == nil {
		return
	}
	list, _ := s.Store.ListSpendLogs()
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

func (s *Server) globalActivityModel(w http.ResponseWriter, r *http.Request) {
	if s.requireManage(w, r) == nil {
		return
	}
	list, _ := s.Store.ListSpendLogs()
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

func (s *Server) globalActivityCacheHits(w http.ResponseWriter, r *http.Request) {
	if s.requireManage(w, r) == nil {
		return
	}
	list, _ := s.Store.ListSpendLogs()
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

func (s *Server) globalSpendLogs(w http.ResponseWriter, r *http.Request) {
	if s.requireManage(w, r) == nil {
		return
	}
	list, _ := s.Store.ListSpendLogs()
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

func (s *Server) globalSpendKeys(w http.ResponseWriter, r *http.Request) {
	if s.requireManage(w, r) == nil {
		return
	}
	list, _ := s.Store.ListSpendLogs()
	byKey := map[string]float64{}
	alias := map[string]string{}
	for _, row := range list {
		k := str(row["api_key"])
		if k == "" {
			continue
		}
		if sp, ok := row["spend"].(float64); ok {
			byKey[k] += sp
		}
		if a := str(row["key_alias"]); a != "" {
			alias[k] = a
		}
	}
	out := []map[string]any{}
	for k, sp := range byKey {
		out = append(out, map[string]any{"api_key": k, "total_spend": sp, "spend": sp, "key_alias": alias[k]})
	}
	httpx.WriteJSON(w, 200, out)
}

func (s *Server) globalSpendModels(w http.ResponseWriter, r *http.Request) {
	if s.requireManage(w, r) == nil {
		return
	}
	list, _ := s.Store.ListSpendLogs()
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

func (s *Server) globalSpendProvider(w http.ResponseWriter, r *http.Request) {
	if s.requireManage(w, r) == nil {
		return
	}
	list, _ := s.Store.ListSpendLogs()
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

func (s *Server) globalSpendTeams(w http.ResponseWriter, r *http.Request) {
	if s.requireManage(w, r) == nil {
		return
	}
	teams, _ := s.Store.ListTeams()
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

func (s *Server) globalSpendTags(w http.ResponseWriter, r *http.Request) {
	if s.requireManage(w, r) == nil {
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"spend_per_tag": []any{}})
}

func (s *Server) globalSpendTagNames(w http.ResponseWriter, r *http.Request) {
	if s.requireManage(w, r) == nil {
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"tag_names": []any{}})
}

func (s *Server) globalSpendEndUsers(w http.ResponseWriter, r *http.Request) {
	if s.requireManage(w, r) == nil {
		return
	}
	httpx.WriteJSON(w, 200, []any{})
}

func (s *Server) spendCalculate(w http.ResponseWriter, r *http.Request) {
	if s.requireManage(w, r) == nil {
		return
	}
	body := readMap(r)
	model := str(body["model"])
	prompt, completion := estimateTokens(body), 0
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

func (s *Server) spendKeys(w http.ResponseWriter, r *http.Request) {
	s.globalSpendKeys(w, r)
}

func (s *Server) spendUsers(w http.ResponseWriter, r *http.Request) {
	if s.requireManage(w, r) == nil {
		return
	}
	users, _ := s.Store.ListUsers()
	out := []map[string]any{}
	for _, u := range users {
		out = append(out, map[string]any{"user_id": u.ID, "user_email": u.Email, "spend": u.Spend})
	}
	httpx.WriteJSON(w, 200, out)
}

func (s *Server) spendTags(w http.ResponseWriter, r *http.Request) {
	if s.requireManage(w, r) == nil {
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"spend_per_tag": []any{}})
}

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

func asFloat(v any) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case int:
		return float64(t)
	default:
		return 0
	}
}

func (s *Server) healthTestConnection(w http.ResponseWriter, r *http.Request) {
	if s.requireManage(w, r) == nil {
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

func (s *Server) healthServices(w http.ResponseWriter, r *http.Request) {
	if s.requireManage(w, r) == nil {
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{
		"healthy_services":   []any{},
		"unhealthy_services": []any{},
		"status":             "healthy",
	})
}

func (s *Server) healthTest(w http.ResponseWriter, r *http.Request) {
	httpx.SetCallID(w, httpx.CallID())
	httpx.WriteJSON(w, 200, map[string]any{"status": "ok", "message": "LiteLLM Proxy is running"})
}
