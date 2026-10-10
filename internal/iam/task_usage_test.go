package iam

import (
	"context"
	"math"
	"sync"
	"testing"

	"xorm.io/builder"
)

// TestTaskUsageLifecycle 前置隔离数据库，验证原请求及上游诊断更新、失败正文、终态防回退和结算去重；schema自动清理。
func TestTaskUsageLifecycle(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	r := usageRecord("task-original", "", 0)
	r.Status = "executing"
	r.RequestBody = "original"
	r.UpstreamResponse = `{"status_code":202}`
	if err := db.RecordTaskUsage(ctx, r, true, false); err != nil {
		t.Fatal(err)
	}
	r.Status = "polling"
	r.RequestBody = ""
	r.ResponseBody = "running"
	if err := db.RecordTaskUsage(ctx, r, false, false); err != nil {
		t.Fatal(err)
	}
	r.Status = "failed"
	r.ResponseBody = "upstream failed"
	r.UpstreamResponse = `{"status_code":502,"headers":{"X-Trace":["poll-failed"]}}`
	if err := db.RecordTaskUsage(ctx, r, false, false); err != nil {
		t.Fatal(err)
	}
	var detail RequestLog
	if _, err := db.Engine.Where("request_id = ?", r.RequestID).Get(&detail); err != nil {
		t.Fatal(err)
	}
	if detail.RequestBody != "original" || detail.Error != "upstream failed" || detail.UpstreamResponse != r.UpstreamResponse {
		t.Fatalf("lost request or error: %+v", detail)
	}
	r.Status = "completed"
	r.Cost = 1.25
	r.ResponseBody = "result"
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := db.RecordTaskUsage(ctx, r, false, true); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	r.Status = "polling"
	r.Cost = 0
	if err := db.RecordTaskUsage(ctx, r, false, false); err != nil {
		t.Fatal(err)
	}
	var row UsageEvent
	if _, err := db.Engine.Where("request_id = ?", r.RequestID).Get(&row); err != nil {
		t.Fatal(err)
	}
	if row.Status != "completed" || row.Cost != 1.25 || !row.TaskSettled || countEvents(t, db, r.RequestID) != 1 || dailyRequests(t, db, "") != 1 || dailyCost(t, db, "") != 1.25 {
		t.Fatalf("task duplicated or regressed: %+v", row)
	}
	r.Status = "completed"
	r.UserID = "other"
	if db.RecordTaskUsage(ctx, r, false, true) == nil {
		t.Fatal("cross-owner update accepted")
	}
	r.RequestID = "missing"
	if db.RecordTaskUsage(ctx, r, false, false) == nil {
		t.Fatal("missing task accepted")
	}
	r.RequestID = ""
	if db.RecordTaskUsage(ctx, r, true, false) == nil {
		t.Fatal("empty identity accepted")
	}
}

// TestTaskUsageValidationAndZeroSettlement 验证非法记录原子拒绝、零费用结算及状态筛选；前置隔离库，schema 自动清理。
func TestTaskUsageValidationAndZeroSettlement(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	for _, r := range []UsageRecord{
		{RequestID: "negative", Status: "completed", Cost: -1},
		{RequestID: "nan", Status: "completed", Cost: math.NaN()},
		{RequestID: "infinite", Status: "completed", Cost: math.Inf(1)},
		{RequestID: "tokens", Status: "completed", CompletionTokens: -1},
		{RequestID: "unknown", Status: "unknown"},
		{RequestID: "pending-bill", Status: "polling"},
	} {
		if db.RecordTaskUsage(ctx, r, true, true) == nil {
			t.Fatalf("invalid record accepted: %+v", r)
		}
	}
	r := usageRecord("zero-task", "", 0)
	r.Status = "executing"
	if err := db.RecordTaskUsage(ctx, r, true, false); err != nil {
		t.Fatal(err)
	}
	r.Status = "completed"
	if err := db.RecordTaskUsage(ctx, r, false, true); err != nil {
		t.Fatal(err)
	}
	r.Cost = 2
	if err := db.RecordTaskUsage(ctx, r, false, true); err != nil {
		t.Fatal(err)
	}
	var stored UsageEvent
	if _, err := db.Engine.Where("request_id = ?", r.RequestID).Get(&stored); err != nil {
		t.Fatal(err)
	}
	if stored.Cost != 0 || !stored.TaskSettled || dailyCost(t, db, "") != 0 {
		t.Fatalf("zero settlement replay charged: %+v", stored)
	}
	for _, status := range []string{"executing", "polling", "failed", "error", "success"} {
		row := usageRecord("filter-"+status, "", 0)
		row.Status = status
		if err := db.RecordUsage(ctx, []UsageRecord{row}); err != nil {
			t.Fatal(err)
		}
	}
	for status, want := range map[string]int64{"success": 2, "error": 2, "completed": 1, "polling": 1, "executing": 1, "unrecognized": 0} {
		got, err := db.CountUsage(ctx, UsageQuery{Cond: builder.Eq{"model": r.Model}, Status: status})
		if err != nil || got != want {
			t.Fatalf("filter %s count %d want %d: %v", status, got, want, err)
		}
	}
}
