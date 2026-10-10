package regression

import (
	"fmt"
	"github.com/sunqirui1987/xhub/internal/iam"
	"net/http"
	"net/url"
	"testing"
	"time"
)

// TestTeamUsageWithoutAgentSurface 验证真实团队调用在聚合及分页接口可见，已移除的 Agent 接口保持 JSON 404。
// 参数 t 为回归上下文，无返回值；前置私有 PostgreSQL schema、真实网关及本地供应商。
// 覆盖空统计、两次数据面调用、团队筛选和匿名拒绝；harness 关闭服务并清理用户、密钥、账单及 schema。
func TestTeamUsageWithoutAgentSurface(t *testing.T) {
	h := newHarness(t, chatDeployment("regression-team-usage"))
	admin := h.adminSession()
	tn := h.provision(t, admin, "team-usage")
	for _, path := range []string{"/team/daily/activity/aggregated", "/team/daily/activity"} {
		body := h.ok(http.MethodGet, path+"?team_ids="+url.QueryEscape(tn.teamID), admin, nil).json()
		if body["metadata"].(map[string]any)["total_api_requests"] != float64(0) {
			t.Fatalf("新团队应返回零请求统计：%v", body)
		}
	}
	for i := 0; i < 2; i++ {
		h.ok(http.MethodPost, "/v1/chat/completions", tn.key, map[string]any{
			"model":    "regression-team-usage",
			"messages": []any{map[string]any{"role": "user", "content": "team usage probe"}},
		})
	}
	h.flushSpend()
	for _, path := range []string{"/team/daily/activity/aggregated", "/team/daily/activity"} {
		body := h.ok(http.MethodGet, path+"?team_ids="+url.QueryEscape(tn.teamID), admin, nil).json()
		meta := body["metadata"].(map[string]any)
		if meta["total_api_requests"] != float64(2) || meta["total_successful_requests"] != float64(2) {
			t.Fatalf("%s 必须统计两次成功调用：%v", path, body)
		}
		denied := h.do(http.MethodGet, path, "", nil)
		if denied.status != http.StatusUnauthorized || denied.json()["error"] == nil {
			t.Fatalf("%s 匿名读取必须返回 JSON 401：%s", path, denied.describe())
		}
	}
	removed := h.do(http.MethodGet, "/agent/daily/activity", admin, nil)
	if removed.status != http.StatusNotFound || removed.json()["error"] == nil {
		t.Fatalf("已移除的 Agent 用量接口必须保持 JSON 404：%s", removed.describe())
	}
}

// TestUsageHistoryPaginationAndTimezone 前置私有 schema 与真实网关，用批量历史账单验证大数据、日期和归属契约。
// 参数 t 为测试上下文，无返回值；写入 5100 条跨 51 天历史事件及跨日终态/处理中事件。
// 验证全量、按天分页不漏计、本地日期、用户/组织/团队/网关一致与未支持维度的鉴权错误；harness 清理全部数据。
func TestUsageHistoryPaginationAndTimezone(t *testing.T) {
	h := newHarness(t, chatDeployment("history-usage"))
	admin := h.adminSession()
	tn := h.provision(t, admin, "history-usage")
	base := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	records := make([]iam.UsageRecord, 0, 5107)
	for day := 0; day < 51; day++ {
		for i := 0; i < 100; i++ {
			records = append(records, iam.UsageRecord{RequestID: fmt.Sprintf("history-%d-%d", day, i), TS: base.AddDate(0, 0, day), TeamID: tn.teamID, OrganizationID: tn.orgID, UserID: tn.userID, Model: "history-usage", Provider: "fixture", Status: "success", HTTPStatus: 200, PromptTokens: 3, CompletionTokens: 2, Cost: 0.125})
		}
	}
	// UTC+8 的 3 月 1 日应包括前一 UTC 日 16:00 到本日 15:59:59.999；所有日报共用此范围。
	localStart := time.Date(2025, 2, 28, 16, 0, 0, 0, time.UTC)
	for i, offset := range []time.Duration{-time.Microsecond, 0, 24*time.Hour - time.Microsecond, 24 * time.Hour} {
		records = append(records, iam.UsageRecord{RequestID: fmt.Sprintf("boundary-%d", i), TS: localStart.Add(offset), TeamID: tn.teamID, OrganizationID: tn.orgID, UserID: tn.userID, Model: "history-usage", Provider: "fixture", Status: "completed", HTTPStatus: 200, PromptTokens: 3, CompletionTokens: 2, Cost: 0.125})
	}
	for i, status := range []string{"executing", "polling", "failed"} {
		records = append(records, iam.UsageRecord{RequestID: fmt.Sprintf("outcome-%d", i), TS: localStart.Add(time.Hour), TeamID: tn.teamID, OrganizationID: tn.orgID, UserID: tn.userID, Model: "history-usage", Provider: "fixture", Status: status, HTTPStatus: 200})
	}
	if err := h.db.RecordUsage(t.Context(), records); err != nil {
		t.Fatalf("写入隔离历史账单：%v", err)
	}
	query := "?start_date=2025-01-01&end_date=2025-02-20&timezone=0&team_ids=" + url.QueryEscape(tn.teamID)
	aggregate := h.ok(http.MethodGet, "/team/daily/activity/aggregated"+query, admin, nil).json()
	meta := aggregate["metadata"].(map[string]any)
	if meta["total_api_requests"] != float64(5100) || meta["total_tokens"] != float64(25500) || meta["total_spend"] != 637.5 {
		t.Fatalf("超过5000条时总量必须完整：%v", meta)
	}
	seen := map[string]bool{}
	count := float64(0)
	for page := 1; page <= 2; page++ {
		body := h.ok(http.MethodGet, "/team/daily/activity"+query+fmt.Sprintf("&page=%d", page), admin, nil).json()
		rows := body["results"].([]any)
		expected := 50
		if page == 2 {
			expected = 1
		}
		if len(rows) != expected {
			t.Fatalf("日报第%d页应有%d天，实际%d", page, expected, len(rows))
		}
		for _, raw := range rows {
			row := raw.(map[string]any)
			day := row["date"].(string)
			if seen[day] {
				t.Fatalf("日报分页重复日期：%s", day)
			}
			seen[day] = true
		}
		count += body["metadata"].(map[string]any)["total_api_requests"].(float64)
	}
	if count != 5100 || len(seen) != 51 {
		t.Fatalf("分页总计必须和聚合一致：count=%v days=%d", count, len(seen))
	}
	localQuery := "?start_date=2025-03-01&end_date=2025-03-01&timezone=-480"
	for _, path := range []string{"/user/daily/activity/aggregated", "/team/daily/activity/aggregated", "/organization/daily/activity"} {
		body := h.ok(http.MethodGet, path+localQuery, admin, nil).json()
		meta := body["metadata"].(map[string]any)
		if meta["total_api_requests"] != float64(3) || meta["total_successful_requests"] != float64(2) || meta["total_failed_requests"] != float64(1) || meta["total_tokens"] != float64(10) || meta["total_spend"] != 0.25 {
			t.Fatalf("%s 本地日期及终态应为2成功1失败：%v", path, meta)
		}
		rows := body["results"].([]any)
		if len(rows) != 1 || rows[0].(map[string]any)["date"] != "2025-03-01" {
			t.Fatalf("本地日期应仅有3月1日：%v", rows)
		}
	}
	gateway := h.ok(http.MethodGet, "/gateway/daily/activity"+localQuery, admin, nil).json()
	if gateway["total_successful_requests"] != float64(2) || gateway["total_failed_requests"] != float64(1) {
		t.Fatalf("网关成败统计必须同源：%v", gateway)
	}
	byUser := h.ok(http.MethodGet, "/team/spend/by_user"+localQuery+"&team_ids="+url.QueryEscape(tn.teamID), admin, nil).json()["results"].([]any)
	if len(byUser) != 1 || byUser[0].(map[string]any)["api_requests"] != float64(3) {
		t.Fatalf("团队用户归属及日期必须一致：%v", byUser)
	}
	empty := h.ok(http.MethodGet, "/user/daily/activity/aggregated"+localQuery+"&user_id=unrelated-user", admin, nil).json()
	if empty["metadata"].(map[string]any)["total_api_requests"] != float64(0) {
		t.Fatalf("无关用户应没有账单：%v", empty)
	}
	for _, path := range []string{"/tag/daily/activity", "/customer/daily/activity"} {
		unsupported := h.do(http.MethodGet, path, admin, nil)
		if unsupported.status != http.StatusNotImplemented || unsupported.json()["error"].(map[string]any)["type"] != "unsupported_usage_dimension" {
			t.Fatalf("移除维度不得伪造零用量：%s", unsupported.describe())
		}
		if denied := h.do(http.MethodGet, path, "", nil); denied.status != http.StatusUnauthorized {
			t.Fatalf("匿名请求应401：%s", denied.describe())
		}
	}
}
