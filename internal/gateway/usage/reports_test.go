package usage

import (
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/sunqirui1987/xhub/internal/authz"
	"github.com/sunqirui1987/xhub/internal/iam"
)

// TestTaskStatusFilters 验证日志查询接受任务阶段和旧失败筛选，未知状态不扩大筛选；纯函数测试无数据清理。
func TestTaskStatusFilters(t *testing.T) {
	// 普通日志由 non_error 明确指定，不能退化为不带条件的全部日志。
	got := logQuery(httptest.NewRequest("GET", "/spend/logs/ui?status_filter=non_error", nil), &authz.Scope{})
	if got.Status != "non_error" {
		t.Fatalf("普通日志分类丢失: %q", got.Status)
	}
	searched := logQuery(httptest.NewRequest("GET", "/spend/logs/ui?request_id=exact-id&search=partial-id", nil), &authz.Scope{})
	if searched.RequestID != "exact-id" || searched.Search != "partial-id" {
		t.Fatalf("请求 ID 筛选丢失: %+v", searched)
	}
	for _, tc := range []struct{ input, want string }{{"executing", "executing"}, {"polling", "polling"}, {"completed", "completed"}, {"success", "success"}, {"failed", "error"}, {"error", "error"}, {"unknown", ""}, {"", ""}} {
		raw := url.Values{"status_filter": {tc.input}}
		got := logQuery(httptest.NewRequest("GET", "/spend/logs/ui?"+raw.Encode(), nil), &authz.Scope{})
		if got.Status != tc.want {
			t.Fatalf("status filter %q = %q, want %q", tc.input, got.Status, tc.want)
		}
	}
}

func TestCollapseSessionsSeparatesCallersWithTheSameSessionID(t *testing.T) {
	rows := []map[string]any{
		{"request_id": "key-a-1", "session_id": "shared", "api_key": "key-a", "user": "user-1", "spend": 1.0, "total_tokens": 10},
		{"request_id": "key-b-1", "session_id": "shared", "api_key": "key-b", "user": "user-1", "spend": 2.0, "total_tokens": 20},
		{"request_id": "key-a-2", "session_id": "shared", "api_key": "key-a", "user": "user-1", "spend": 3.0, "total_tokens": 30},
		{"request_id": "user-2-1", "session_id": "shared", "api_key": "", "user": "user-2", "spend": 4.0, "total_tokens": 40},
	}

	got := collapseSessions(rows)
	if len(got) != 3 {
		t.Fatalf("collapsed rows = %d, want 3: %#v", len(got), got)
	}
	assertSessionTotals(t, got[0], 2, 4, 40)
	assertSessionTotals(t, got[1], 1, 2, 20)
	assertSessionTotals(t, got[2], 1, 4, 40)
}

func TestCollapseSessionsUsesUserForKeylessCalls(t *testing.T) {
	rows := []map[string]any{
		{"request_id": "u1", "session_id": "shared", "user": "user-1"},
		{"request_id": "u2", "session_id": "shared", "user": "user-2"},
	}

	got := collapseSessions(rows)
	if len(got) != 2 {
		t.Fatalf("collapsed rows = %d, want 2: %#v", len(got), got)
	}
}

func TestCollapseSessionsDoesNotJoinRowsWithoutCaller(t *testing.T) {
	rows := []map[string]any{
		{"request_id": "first", "session_id": "shared"},
		{"request_id": "second", "session_id": "shared"},
		{"session_id": "shared"},
		{"session_id": "shared"},
	}
	got := collapseSessions(rows)
	if len(got) != 4 {
		t.Fatalf("anonymous calls were merged: %#v", got)
	}
}

func assertSessionTotals(t *testing.T, row map[string]any, count int, spend float64, tokens int) {
	t.Helper()
	if row["session_total_count"] != count || row["session_total_spend"] != spend || row["session_total_tokens"] != tokens {
		t.Fatalf("session totals = count %#v spend %#v tokens %#v", row["session_total_count"], row["session_total_spend"], row["session_total_tokens"])
	}
}

// TestErrorRowFields 验证失败列表行提供前端约定的状态和 HTTP 状态。
// 前置条件是持久化事件带 error/502；结果应在 metadata.http_status 中原样返回；纯内存测试无需清理。
func TestErrorRowFields(t *testing.T) {
	rows := eventRows([]iam.UsageEvent{{RequestID: "failed", Status: "error", HTTPStatus: 502}})
	if len(rows) != 1 || rows[0]["status"] != "error" {
		t.Fatalf("失败状态字段错误: %#v", rows)
	}
	meta, _ := rows[0]["metadata"].(map[string]any)
	if meta["http_status"] != 502 {
		t.Fatalf("HTTP 状态未返回: %#v", meta)
	}
}

// TestJSONOrTextPreservesRawResponse 验证详情 response 对 JSON 解码、对纯文本原样返回。
// 前置条件是数据库可能存两种正文；结果不得清空纯文本；纯函数测试无需清理。
func TestJSONOrTextPreservesRawResponse(t *testing.T) {
	if got := jsonOrText("plain upstream error"); got != "plain upstream error" {
		t.Fatalf("纯文本响应被改写: %#v", got)
	}
	jsonBody, ok := jsonOrText("{\"error\":\"bad\"}").(map[string]any)
	if !ok || jsonBody["error"] != "bad" {
		t.Fatalf("JSON 响应未解码: %#v", jsonBody)
	}
	if got, ok := jsonOrText("  ").(map[string]any); !ok || len(got) != 0 {
		t.Fatalf("空响应边界错误: %#v", got)
	}
}

// TestLogPaginationQuery 验证排序方向、未知列传递及安全偏移边界；前置内存请求，无数据库数据需要清理。
// 排序列由 IAM 白名单处理，这里必须保留前端列名且只有 asc 启用升序。
func TestLogPaginationQuery(t *testing.T) {
	for _, c := range []struct {
		raw, field string
		asc        bool
	}{{"", "", false}, {"sort_by=spend&sort_order=asc", "spend", true}, {"sort_by=unknown&sort_order=broken", "unknown", false}} {
		q := logQuery(httptest.NewRequest("GET", "/?"+c.raw, nil), &authz.Scope{})
		if q.SortBy != c.field || q.SortAsc != c.asc {
			t.Fatalf("日志排序条件错误: %+v", q)
		}
	}
	for _, c := range []struct {
		raw  string
		want int
	}{{"", 0}, {"page=2", 5000}, {"page=-1", 0}, {"page=broken", 0}, {"page=9223372036854775807", int(^uint(0) >> 1)}} {
		if got := pageOffset(httptest.NewRequest("GET", "/?"+c.raw, nil), 5000); got != c.want {
			t.Fatalf("用量分页偏移%q: %d != %d", c.raw, got, c.want)
		}
	}
}
