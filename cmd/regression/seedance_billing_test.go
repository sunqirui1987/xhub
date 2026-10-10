package regression

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// TestSeedanceSettlementUsesMeasuredBandAndDeduplicates 验证官方 Ark 任务只按成功实测用量结算一次。
// 参数 t：测试上下文；返回：无。前置本地上游与真实数据库，核对状态边界和账单；schema 自动删除。
func TestSeedanceSettlementUsesMeasuredBandAndDeduplicates(t *testing.T) {
	var response atomic.Value
	response.Store(map[string]any{"status": "running", "usage": map[string]any{"completion_tokens": 40594}})
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			writeJSON(w, map[string]any{"id": "billing-task"})
			return
		}
		writeJSON(w, response.Load())
	}))
	defer up.Close()
	model := "volcengine/doubao-seedance-2-0-fast-260128"
	dep := seedanceDeployment(model, "ark_contents_generation")
	dep.LiteLLMParams["api_base"] = up.URL
	dep.LiteLLMParams["output_cost_per_token"] = 3.33333e-6
	h := newHarness(t, dep)
	admin := h.adminSession()
	owner := h.openScope(t, admin, "seedance-billing")
	before := h.moneyOf(t, owner)
	created := h.ok(http.MethodPost, "/api/v3/contents/generations/tasks", owner.key, map[string]any{"model": model, "resolution": "480p", "content": []any{map[string]any{"type": "text", "text": "apple"}}})
	originalID := created.header("x-litellm-call-id")
	rowsAtCreate := h.spendLogs(t, admin)
	if len(rowsAtCreate) != 1 || stringField(rowsAtCreate[0], "status") != "executing" {
		t.Fatalf("create lifecycle: %+v", rowsAtCreate)
	}
	for _, status := range []string{"queued", "creating_assets", "running", "failed", "expired", "unknown", ""} {
		response.Store(map[string]any{"status": status, "usage": map[string]any{"completion_tokens": 40594}})
		h.ok(http.MethodGet, "/api/v3/contents/generations/tasks/billing-task", owner.key, nil)
		rows := h.spendLogs(t, admin)
		if len(rows) != 1 || stringField(rows[0], "request_id") != originalID {
			t.Fatalf("poll created a second log: %+v", rows)
		}
		if status == "queued" && stringField(rows[0], "status") != "polling" {
			t.Fatalf("missing polling status: %+v", rows)
		}
		if status == "failed" && stringField(rows[0], "status") != "failed" {
			t.Fatalf("missing failure status: %+v", rows)
		}
		if status == "failed" {
			errors := h.ok(http.MethodGet, "/spend/logs/ui?status_filter=error", admin, nil)
			if len(rowsOf(errors, "data", "logs")) != 1 {
				t.Fatal("task failure missing from error tab", errors.describe())
			}
		}
		if !h.moneyOf(t, owner).same(before) {
			t.Fatalf("charged status %q", status)
		}
	}
	response.Store(map[string]any{"id": "billing-task", "status": "succeeded", "resolution": "480p", "usage": map[string]any{"completion_tokens": 40594, "total_tokens": 40594}})
	const want = 40594 * 3.33333e-6
	for i := 0; i < 3; i++ {
		r := h.ok(http.MethodGet, "/api/v3/contents/generations/tasks/billing-task", owner.key, nil)
		if !nearlyEqual(parseFloatOrZero(r.header("x-litellm-response-cost")), want) {
			t.Fatal("wrong measured cost", r.describe())
		}
		if got := h.moneyOf(t, owner); !got.grewBy(before, want) {
			t.Fatalf("poll %d lost/duplicate settlement: %+v", i, got)
		}
	}
	var settled []map[string]any
	rows := h.spendLogs(t, admin)
	for _, row := range rows {
		if stringField(row, "model") != model {
			continue
		}
		if numberOrZero(row["spend"]) > 0 {
			settled = append(settled, row)
		}
	}
	if len(settled) != 1 || len(rows) != 1 || stringField(settled[0], "request_id") != originalID {
		t.Fatalf("expected one paid event: %+v", settled)
	}
	id := stringField(settled[0], "request_id")
	if id == "" {
		id = stringField(settled[0], "id")
	}
	if strings.HasPrefix(id, "official-settlement:") || stringField(settled[0], "status") != "completed" {
		t.Fatal("settlement must update the original completed task", settled[0])
	}
	bill := breakdownOf(t, h, admin, id)
	raw, _ := json.Marshal(bill)
	if strings.Contains(string(raw), "\"fallback\":true") {
		t.Fatalf("wrong snapshot: %s", raw)
	}
}
