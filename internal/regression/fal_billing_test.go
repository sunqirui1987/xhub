package regression

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestFalSettlementUsesOutputSecondsAndDeduplicates(t *testing.T) {
	const model = "qiniu/fal-ai/kling-video/v2.5-turbo/pro/text-to-video"
	const create = "/queue/fal-ai/kling-video/v2.5-turbo/pro/text-to-video"
	const result = "/queue/fal-ai/kling-video/requests/fal-billing-task"
	var response atomic.Value
	response.Store(map[string]any{"status": "IN_PROGRESS"})
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Key sk-fake" {
			w.WriteHeader(401)
			return
		}
		if r.Method == http.MethodPost {
			raw, _ := io.ReadAll(r.Body)
			var body map[string]any
			json.Unmarshal(raw, &body)
			if body["model"] != nil {
				w.WriteHeader(400)
				return
			}
			writeJSON(w, map[string]any{"request_id": "fal-billing-task", "response_url": "https://supplier.example/result", "status_url": "https://supplier.example/status"})
			return
		}
		writeJSON(w, response.Load())
	}))
	defer up.Close()
	dep := seedanceDeployment(model, "qiniu_fal_kling")
	dep.LiteLLMParams["api_base"] = up.URL
	dep.LiteLLMParams["api_key"] = "sk-fake"
	h := newHarness(t, dep)
	admin := h.adminSession()
	owner := h.openScope(t, admin, "fal-billing")
	before := h.moneyOf(t, owner)
	h.ok(http.MethodPost, create, owner.key, map[string]any{"prompt": "apple", "duration": "10"})
	video := map[string]any{"video": map[string]any{"url": "https://example.org/video.mp4", "duration": 5.5}, "metrics": map[string]any{"inference_time": 99999.}}
	for _, doc := range []map[string]any{
		{"status": "IN_QUEUE", "result": video}, {"status": "IN_PROGRESS", "result": video},
		{"status": "COMPLETED", "detail": map[string]any{"message": "failed"}, "result": video},
	} {
		response.Store(doc)
		h.ok(http.MethodGet, result+"/status", owner.key, nil)
		if !h.moneyOf(t, owner).same(before) {
			t.Fatal("pending/failure spent money")
		}
	}
	const want = 5.5 * 0.07246377
	for i := 0; i < 3; i++ {
		path := result
		if i == 0 {
			response.Store(map[string]any{"status": "COMPLETED", "result": video})
			path += "/status"
		} else {
			response.Store(video)
		}
		r := h.ok(http.MethodGet, path, owner.key, nil)
		if !nearlyEqual(parseFloatOrZero(r.header("x-litellm-response-cost")), want) {
			t.Fatal("wrong measured cost", r.describe())
		}
		if got := h.moneyOf(t, owner); !got.grewBy(before, want) {
			t.Fatalf("poll %d duplicated/lost spend: %+v", i, got)
		}
	}
	var paid []map[string]any
	for _, row := range h.successRows(t, admin, model) {
		if numberOrZero(row["spend"]) > 0 {
			paid = append(paid, row)
		}
	}
	if len(paid) != 1 {
		t.Fatal("expected one paid event", paid)
	}
	id := stringField(paid[0], "request_id")
	if id == "" {
		id = stringField(paid[0], "id")
	}
	if !strings.HasPrefix(id, "official-settlement:") {
		t.Fatal(id)
	}
	bill := breakdownOf(t, h, admin, id)
	raw, _ := json.Marshal(bill)
	if !strings.Contains(string(raw), "pro_norefv_v_duration") || strings.Contains(string(raw), "\"fallback\":true") {
		t.Fatal(string(raw))
	}
}
