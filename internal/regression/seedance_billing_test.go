package regression

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

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
	model := "qiniu/bytedance/doubao-seedance-2-0-mini-260615"
	dep := seedanceDeployment(model, "qiniu_contents_generation")
	dep.LiteLLMParams["api_base"] = up.URL
	h := newHarness(t, dep)
	admin := h.adminSession()
	owner := h.openScope(t, admin, "seedance-billing")
	before := h.moneyOf(t, owner)
	h.ok(http.MethodPost, "/v3/contents/generations/tasks", owner.key, map[string]any{"model": model, "resolution": "480p", "content": []any{map[string]any{"type": "text", "text": "apple"}}})
	for _, status := range []string{"queued", "creating_assets", "running", "failed", "expired", "unknown", ""} {
		response.Store(map[string]any{"status": status, "usage": map[string]any{"completion_tokens": 40594}})
		h.ok(http.MethodGet, "/v3/contents/generations/tasks/billing-task", owner.key, nil)
		if !h.moneyOf(t, owner).same(before) {
			t.Fatalf("charged status %q", status)
		}
	}
	response.Store(map[string]any{"id": "billing-task", "status": "succeeded", "resolution": "480p", "usage": map[string]any{"completion_tokens": 40594, "total_tokens": 40594}})
	const want = 40594 * 3.33333e-6
	for i := 0; i < 3; i++ {
		r := h.ok(http.MethodGet, "/v3/contents/generations/tasks/billing-task", owner.key, nil)
		if !nearlyEqual(parseFloatOrZero(r.header("x-litellm-response-cost")), want) {
			t.Fatal("wrong measured cost", r.describe())
		}
		if got := h.moneyOf(t, owner); !got.grewBy(before, want) {
			t.Fatalf("poll %d lost/duplicate settlement: %+v", i, got)
		}
	}
	var settled []map[string]any
	for _, row := range h.successRows(t, admin, model) {
		if numberOrZero(row["spend"]) > 0 {
			settled = append(settled, row)
		}
	}
	if len(settled) != 1 {
		t.Fatalf("expected one paid event: %+v", settled)
	}
	id := stringField(settled[0], "request_id")
	if id == "" {
		id = stringField(settled[0], "id")
	}
	if !strings.HasPrefix(id, "official-settlement:") {
		t.Fatal("missing durable settlement id", settled[0])
	}
	bill := breakdownOf(t, h, admin, id)
	raw, _ := json.Marshal(bill)
	if !strings.Contains(string(raw), "woiv") || strings.Contains(string(raw), "\"fallback\":true") {
		t.Fatalf("wrong snapshot: %s", raw)
	}
}
