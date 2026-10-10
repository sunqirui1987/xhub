package regression

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/sunqirui1987/xhub/internal/iam"
)

// TestLogsPaginationContract 前置真实网关、私有 schema 和本地上游；参数 t 管理生命周期。
// 验证会话聚合页数、原始请求翻页、完整统计、排序、筛选、非法参数和未鉴权错误；服务及数据自动清理。
func TestLogsPaginationContract(t *testing.T) {
	h := openHarness(t, false, chatDeployment("pagination"))
	admin := h.adminSession()
	tn := h.provision(t, admin, "pagination")
	now := time.Now().UTC()
	records := []iam.UsageRecord{}
	for i := 0; i < 60; i++ {
		session := fmt.Sprintf("session-%02d", i/2)
		records = append(records, iam.UsageRecord{RequestID: fmt.Sprintf("pagination-%02d", i), TS: now, UserID: tn.userID, SessionID: session, Status: "success", HTTPStatus: 200, PromptTokens: 3, CompletionTokens: 2, Cost: 0.1})
	}
	if err := h.db.RecordUsage(t.Context(), records); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for page := 1; page <= 2; page++ {
		body := h.ok(http.MethodGet, fmt.Sprintf("/spend/logs/ui?group_by_session=true&status_filter=non_error&page_size=25&page=%d", page), admin, nil)
		rows := rowsOf(body, "data")
		want := 25
		if page == 2 {
			want = 5
		}
		if firstFloat(body.json(), "total") != 30 || firstFloat(body.json(), "total_pages") != 2 || len(rows) != want {
			t.Fatalf("聚合分页契约: page=%d body=%v", page, body)
		}
		for _, row := range rows {
			id := firstString(row, "session_id")
			if seen[id] {
				t.Fatalf("会话跨页重复: %s", id)
			}
			seen[id] = true
			if firstFloat(row, "session_total_count") != 2 || firstFloat(row, "session_total_tokens") != 10 || firstFloat(row, "session_total_spend") != 0.2 {
				t.Fatalf("会话完整汇总: %v", row)
			}
		}
	}
	raw := h.ok(http.MethodGet, "/spend/logs/ui?group_by_session=false&page_size=25&page=3&sort_by=startTime&sort_order=asc", admin, nil)
	rows := rowsOf(raw, "data")
	if len(rows) != 10 || firstFloat(raw.json(), "total") != 60 || firstString(rows[0], "request_id") != "pagination-50" {
		t.Fatalf("原始日志稳定排序与末页: %v", raw)
	}
	session := h.ok(http.MethodGet, "/spend/logs/session/ui?session_id=session-00&page_size=1&page=2&group_by_session=true", admin, nil)
	if len(rowsOf(session, "data")) != 1 || firstFloat(session.json(), "total") != 2 {
		t.Fatalf("会话详情仍应逐条分页: %v", session)
	}
	for _, query := range []string{"page=3&page_size=25", "page=9223372036854775807&page_size=25", "page=1&page_size=25&search=does-not-exist", "page=1&page_size=25&status_filter=error"} {
		result := h.ok(http.MethodGet, "/spend/logs/ui?group_by_session=true&"+query, admin, nil)
		if len(rowsOf(result, "data")) != 0 {
			t.Fatalf("空页或空过滤错误: %s %v", query, result)
		}
	}
	invalid := h.ok(http.MethodGet, "/spend/logs/ui?group_by_session=true&page=-1&page_size=0&sort_by=invalid", admin, nil)
	if firstFloat(invalid.json(), "page") != 1 || firstFloat(invalid.json(), "page_size") != 50 || len(rowsOf(invalid, "data")) != 30 {
		t.Fatalf("非法参数默认值错误: %v", invalid)
	}
	unauth := h.do(http.MethodGet, "/spend/logs/ui?page=2", "", nil)
	if unauth.status != 401 {
		t.Fatalf("未鉴权必须拒绝: %s", unauth.describe())
	}
}
