package usage

import (
	"testing"
	"time"

	"github.com/sunqirui1987/xhub/internal/iam"
)

// TestDecodeAndAggregateGuardrailEvents 验证持久化 JSON 中的拦截、脱敏、flag 和放行被正确聚合。
// 前置条件是四条同一护栏事件；结果需保留真实请求标识、非零延迟和动作差异；测试仅使用内存夹具，无需清理。
func TestDecodeAndAggregateGuardrailEvents(t *testing.T) {
	ts := time.Date(2026, 10, 9, 8, 0, 0, 0, time.UTC)
	events := []iam.UsageEvent{
		{RequestID: "block", TS: ts, Model: "m", Guardrail: "[{\"guardrail_id\":\"g-1\",\"guardrail_name\":\"secrets\",\"guardrail_provider\":\"xhub\",\"guardrail_mode\":\"pre_call\",\"guardrail_status\":\"blocked\",\"duration\":0.012,\"reason\":\"matched\",\"guardrail_response\":{\"action\":\"block\"}}]"},
		{RequestID: "redact", TS: ts, Model: "m", Guardrail: "[{\"guardrail_id\":\"g-1\",\"guardrail_name\":\"secrets\",\"guardrail_status\":\"guardrail_flagged\",\"duration\":0.004,\"guardrail_response\":{\"action\":\"redact\"}}]"},
		{RequestID: "flag", TS: ts, Model: "m", Guardrail: "[{\"guardrail_id\":\"g-1\",\"guardrail_name\":\"secrets\",\"guardrail_status\":\"guardrail_flagged\",\"guardrail_response\":{\"action\":\"flag\"}}]"},
		{RequestID: "pass", TS: ts, Model: "m", Guardrail: "[{\"guardrail_id\":\"g-1\",\"guardrail_name\":\"secrets\",\"guardrail_status\":\"success\",\"guardrail_response\":{\"action\":\"allow\"}}]"},
	}
	findings := decodeGuardrailFindings(events)
	if len(findings) != 4 || findings[0].Action != "blocked" || findings[1].Action != "flagged" || findings[2].Action != "flagged" || findings[3].Action != "passed" {
		t.Fatalf("动作映射错误: %#v", findings)
	}
	aggs := aggregateGuardrails(findings)
	if len(aggs) != 1 || aggs[0].Total != 4 || aggs[0].Blocked != 1 || aggs[0].Flagged != 2 || aggs[0].Passed != 1 {
		t.Fatalf("聚合错误: %#v", aggs)
	}
	row := guardrailOverviewRow(aggs[0])
	if row["requestsEvaluated"] != 4 || row["failRate"] != float64(75) || row["avgLatency"].(float64) <= 0 {
		t.Fatalf("总览不是非空真实统计: %#v", row)
	}
	if len(guardrailDetailBody(aggs[0])["time_series"].([]any)) != 1 {
		t.Fatalf("详情趋势缺失: %#v", guardrailDetailBody(aggs[0]))
	}
}

// TestDecodeGuardrailEventsSkipsMalformedAndEmptyRows 验证空事件、损坏 JSON 和无标识 finding 不会制造监控数据或导致接口崩溃。
// 前置输入均不构成可识别护栏；结果必须为空切片；测试没有数据库或外部副作用，无需清理。
func TestDecodeGuardrailEventsSkipsMalformedAndEmptyRows(t *testing.T) {
	got := decodeGuardrailFindings([]iam.UsageEvent{{Guardrail: "{"}, {Guardrail: "[{\"guardrail_status\":\"blocked\"}]"}, {Guardrail: ""}})
	if got == nil || len(got) != 0 {
		t.Fatalf("非法事件未被安全跳过: %#v", got)
	}
	if rows := aggregateGuardrails(got); rows == nil || len(rows) != 0 {
		t.Fatalf("空聚合必须是非 nil 空数组: %#v", rows)
	}
}
