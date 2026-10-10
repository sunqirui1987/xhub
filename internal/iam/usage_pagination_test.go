package iam

import (
	"context"
	"fmt"
	"testing"
	"time"
	"xorm.io/builder"
)

// TestUsageSessionPagination 前置私有数据库及同时间会话、独立日志、跨调用方记录；
// 验证先授权筛选再合并分页、完整统计、稳定顺序、越界和取消错误；参数 t 管理测试及 schema 自动清理。
func TestUsageSessionPagination(t *testing.T) {
	db := testDB(t)
	now := time.Now().UTC()
	records := []UsageRecord{}
	for i := 0; i < 32; i++ {
		records = append(records, UsageRecord{RequestID: fmt.Sprintf("page-%02d", i), TS: now, UserID: "u", KeyID: "k", SessionID: "shared", Status: "success", HTTPStatus: 200, PromptTokens: 2, CompletionTokens: 3, Cost: 0.25})
	}
	for i := 0; i < 26; i++ {
		records = append(records, UsageRecord{RequestID: fmt.Sprintf("solo-%02d", i), TS: now, UserID: "u", Status: "success", HTTPStatus: 200})
	}
	records = append(records, UsageRecord{RequestID: "foreign", TS: now, UserID: "other", KeyID: "k", SessionID: "shared", Status: "success", Cost: 99}, UsageRecord{RequestID: "failed", TS: now, UserID: "u", KeyID: "k", SessionID: "shared", Status: "failed", Cost: 99})
	if err := db.RecordUsage(t.Context(), records); err != nil {
		t.Fatal(err)
	}
	q := UsageQuery{Cond: builder.Eq{"user_id": "u"}, Status: "non_error", Limit: 25}
	rows, total, err := db.ListUsageSessions(t.Context(), q)
	if err != nil || total != 27 || len(rows) != 25 {
		t.Fatalf("会话分页第一页: rows=%d total=%d err=%v", len(rows), total, err)
	}
	seen := map[string]bool{}
	for _, row := range rows {
		seen[row.RequestID] = true
	}
	q.Offset = 25
	rows, total, err = db.ListUsageSessions(t.Context(), q)
	if err != nil || total != 27 || len(rows) != 2 {
		t.Fatalf("末页: rows=%d total=%d err=%v", len(rows), total, err)
	}
	for _, row := range rows {
		if seen[row.RequestID] {
			t.Fatalf("跨页重复 %s", row.RequestID)
		}
		if row.SessionID == "shared" && (row.SessionCount != 32 || row.SessionTokens != 160 || row.SessionSpend != 8 || row.RequestID != "page-31") {
			t.Fatalf("完整会话汇总或代表请求错误: %+v", row)
		}
	}
	q.Offset = 100
	rows, total, err = db.ListUsageSessions(t.Context(), q)
	if err != nil || len(rows) != 0 || total != 27 {
		t.Fatalf("越界页应空且保留总数: %d %d %v", len(rows), total, err)
	}
	q.Offset = 0
	q.Search = "does-not-exist"
	rows, total, err = db.ListUsageSessions(t.Context(), q)
	if err != nil || len(rows) != 0 || total != 0 {
		t.Fatalf("空搜索: %d %d %v", len(rows), total, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, _, err := db.ListUsageSessions(ctx, q); err == nil {
		t.Fatal("取消查询必须返回错误")
	}
	q.Search = ""
	q.Limit = 1
	q.SortAsc = true
	raw, err := db.ListUsage(t.Context(), q)
	if err != nil || len(raw) != 1 || raw[0].RequestID != "page-00" {
		t.Fatalf("同时间排序应按 ID 稳定升序: %+v %v", raw, err)
	}
}

// TestLogOrder 前置不依赖数据库；验证正常、默认和恶意排序字段只能生成白名单 SQL；无数据需要清理。
func TestLogOrder(t *testing.T) {
	for _, tc := range []struct {
		q    UsageQuery
		want string
	}{
		{UsageQuery{}, "ts DESC, id DESC"},
		{UsageQuery{SortBy: "spend", SortAsc: true}, "cost ASC, id ASC"},
		{UsageQuery{SortBy: "ts; DROP TABLE usage_events"}, "ts DESC, id DESC"},
		{UsageQuery{SortBy: "total_tokens"}, "(prompt_tokens + completion_tokens) DESC, id DESC"},
	} {
		if got := tc.q.logOrder(); got != tc.want {
			t.Fatalf("排序 %q，期望 %q", got, tc.want)
		}
	}
}

// TestUsageSessionIdentityAndSort 前置私有 schema；同会话的密钥、用户、匿名及空会话分别写入。
// 验证身份命名空间隔离、缺失身份独立、负偏移/default/cap 边界及会话合计排序；参数 t 自动清理私有库。
func TestUsageSessionIdentityAndSort(t *testing.T) {
	db := testDB(t)
	now := time.Now().UTC()
	records := []UsageRecord{
		{RequestID: "key-older", TS: now.Add(-time.Second), KeyID: "same", UserID: "owner", SessionID: "shared", Cost: 10, PromptTokens: 10},
		{RequestID: "key-latest", TS: now, KeyID: "same", UserID: "owner", SessionID: "shared", Cost: 1, PromptTokens: 1},
		{RequestID: "user", TS: now, UserID: "same", SessionID: "shared", Cost: 5, PromptTokens: 5},
		{RequestID: "other-key", TS: now, KeyID: "other", SessionID: "shared", Cost: 3, PromptTokens: 3},
		{RequestID: "anonymous-1", TS: now, SessionID: "shared"},
		{RequestID: "anonymous-2", TS: now, SessionID: "shared"},
		{RequestID: "no-session-1", TS: now, KeyID: "same"},
		{RequestID: "no-session-2", TS: now, KeyID: "same"},
	}
	if err := db.RecordUsage(t.Context(), records); err != nil {
		t.Fatal(err)
	}
	for _, limit := range []int{0, maxUsageRead + 1} {
		rows, total, err := db.ListUsageSessions(t.Context(), UsageQuery{Limit: limit, Offset: -10})
		if err != nil || total != 7 || len(rows) != 7 {
			t.Fatalf("默认/上限与负偏移: rows=%d total=%d err=%v", len(rows), total, err)
		}
		for _, row := range rows {
			want := int64(1)
			if row.RequestID == "key-latest" {
				want = 2
			}
			if row.SessionCount != want {
				t.Fatalf("身份不应混合: %+v", row)
			}
		}
	}
	for _, field := range []string{"spend", "total_tokens"} {
		rows, _, err := db.ListUsageSessions(t.Context(), UsageQuery{SortBy: field, Limit: 1})
		if err != nil || len(rows) != 1 || rows[0].RequestID != "key-latest" {
			t.Fatalf("会话按显示合计排序 %s: %+v %v", field, rows, err)
		}
		rows, _, err = db.ListUsageSessions(t.Context(), UsageQuery{SortBy: field, SortAsc: true, Offset: 4, Limit: 3})
		if err != nil || len(rows) != 3 || rows[0].RequestID != "other-key" || rows[1].RequestID != "user" || rows[2].RequestID != "key-latest" {
			t.Fatalf("合计升序及分页 %s: %+v %v", field, rows, err)
		}
	}
}
