package dataplane

import (
	"encoding/json"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/sunqirui1987/xhub/internal/auth"
	"github.com/sunqirui1987/xhub/internal/catalog"
	"github.com/sunqirui1987/xhub/internal/config"
)

// Opt-in: creates one paid, minimum-duration video. Never log API credentials.
func TestQiniuSeedanceLive(t *testing.T) {
	if os.Getenv("XHUB_QINIU_SEEDANCE_LIVE") != "1" {
		t.Skip("paid live test disabled")
	}
	key := os.Getenv("QINIU_API_KEY")
	if key == "" {
		t.Fatal("QINIU_API_KEY required")
	}
	model := "qiniu/bytedance/doubao-seedance-2-0-mini-260615"
	dep := deployment("seedance-live", model, key, "https://api.qnaigc.com", "qiniu_contents_generation", nil)
	h := &logicHost{cfg: &config.Config{}, client: &http.Client{Timeout: 60 * time.Second}, models: []config.ModelEntry{dep}, pins: map[string]string{}}
	created := h.call(t, http.MethodPost, "/v3/contents/generations/tasks",
		"{\"model\":\"seedance-live\",\"content\":[{\"type\":\"text\",\"text\":\"A red apple on a white table, static camera, gentle natural light.\"}],\"duration\":4,\"resolution\":\"480p\",\"ratio\":\"16:9\",\"generate_audio\":false}")
	var doc map[string]any
	if err := json.Unmarshal(created.Body.Bytes(), &doc); err != nil {
		t.Fatal("invalid create response", created.Code)
	}
	id, _ := doc["id"].(string)
	if created.Code < 200 || created.Code >= 300 || id == "" {
		t.Fatalf("create HTTP %d: %s", created.Code, created.Body.String())
	}
	t.Logf("created task=%s model=%s", id, model)
	deadline := time.Now().Add(10 * time.Minute)
	for time.Now().Before(deadline) {
		time.Sleep(10 * time.Second)
		polled := h.call(t, http.MethodGet, "/v3/contents/generations/tasks/"+id, "")
		doc = nil
		if json.Unmarshal(polled.Body.Bytes(), &doc) != nil {
			t.Fatal("invalid poll response", polled.Code)
		}
		status, _ := doc["status"].(string)
		t.Logf("task=%s HTTP=%d status=%s", id, polled.Code, status)
		if polled.Code != 200 || status == "failed" || status == "expired" || status == "cancelled" {
			t.Fatalf("task failed: %s", polled.Body.String())
		}
		if status != "succeeded" {
			continue
		}
		row := h.spend[len(h.spend)-1]
		u := catalog.NormalizeUsage(row.usage)
		facts := h.OfficialContext(officialTaskScope(&auth.Principal{UserID: "test-user"}, "qiniu_contents_generation", id))
		c, ok := catalog.CostAt(model, u, facts.StartedAt)
		content, _ := doc["content"].(map[string]any)
		video, _ := content["video_url"].(string)
		if !ok || u.CompletionTokens <= 0 || u.PromptTokens != 0 || u.OutputVariant != "woiv" || video == "" || h.notes[len(h.notes)-1].SettlementID == "" {
			t.Fatalf("incomplete settlement: usage=%+v charge=%+v priced=%v", u, c, ok)
		}
		// Public generation evidence, excluding secrets and prompts.
		t.Logf("completed task=%s usage=%+v charge_usd=%.8f rates=%+v video_present=%t", id, u, c.Total, c.Applied, video != "")
		return
	}
	t.Fatalf("poll timeout; resume existing task %s instead of creating again", id)
}

// Opt-in: exercise the Fal path model, Key auth, local queue URLs and settlement.
func TestQiniuFalLive(t *testing.T) {
	if os.Getenv("XHUB_QINIU_FAL_LIVE") != "1" {
		t.Skip("paid live test disabled")
	}
	key := os.Getenv("QINIU_API_KEY")
	if key == "" {
		t.Fatal("QINIU_API_KEY required")
	}
	const model = "qiniu/bytedance/seedance-2.0/mini/text-to-video"
	const transport = "qiniu_fal_doubao_20"
	dep := deployment(model, model, key, "https://api.qnaigc.com", transport, nil)
	h := &logicHost{cfg: &config.Config{}, client: &http.Client{Timeout: 60 * time.Second}, models: []config.ModelEntry{dep}, pins: map[string]string{}}
	created := h.call(t, http.MethodPost, "/queue/bytedance/seedance-2.0/mini/text-to-video",
		`{"prompt":"A red apple on a white table, static camera, gentle natural light.","duration":"4","resolution":"480p","aspect_ratio":"16:9","generate_audio":false}`)
	var doc map[string]any
	if json.Unmarshal(created.Body.Bytes(), &doc) != nil {
		t.Fatal("invalid create response", created.Code)
	}
	id, _ := doc["request_id"].(string)
	if created.Code < 200 || created.Code >= 300 || id == "" {
		t.Fatalf("create HTTP %d: %s", created.Code, created.Body.String())
	}
	resultPath, _ := doc["response_url"].(string)
	statusPath, _ := doc["status_url"].(string)
	if resultPath != "/queue/bytedance/seedance-2.0/requests/"+id || statusPath != resultPath+"/status" {
		t.Fatalf("invalid local queue paths for task %s", id)
	}
	t.Logf("created task=%s model=%s status_path=%s", id, model, statusPath)
	deadline := time.Now().Add(10 * time.Minute)
	for time.Now().Before(deadline) {
		time.Sleep(10 * time.Second)
		polled := h.call(t, http.MethodGet, statusPath, "")
		doc = nil
		if json.Unmarshal(polled.Body.Bytes(), &doc) != nil {
			t.Fatal("invalid poll response", polled.Code)
		}
		status, _ := doc["status"].(string)
		t.Logf("task=%s HTTP=%d status=%s", id, polled.Code, status)
		if polled.Code != 200 && polled.Code != 202 || doc["error"] != nil || doc["detail"] != nil {
			t.Fatalf("task %s failed HTTP %d (error=%t detail=%t)", id, polled.Code, doc["error"] != nil, doc["detail"] != nil)
		}
		if status != "COMPLETED" {
			continue
		}
		final := h.call(t, http.MethodGet, resultPath, "")
		if final.Code != 200 {
			t.Fatalf("task %s result HTTP %d", id, final.Code)
		}
		doc = nil
		if json.Unmarshal(final.Body.Bytes(), &doc) != nil {
			t.Fatal("invalid result response")
		}
		row := h.spend[len(h.spend)-1]
		u := catalog.NormalizeUsage(row.usage)
		facts := h.OfficialContext(officialTaskScope(&auth.Principal{UserID: "test-user"}, transport, id))
		c, priced := catalog.CostAt(model, u, facts.StartedAt)
		result := doc
		if nested, ok := doc["result"].(map[string]any); ok {
			result = nested
		}
		video, _ := result["video"].(map[string]any)
		videoURL, _ := video["url"].(string)
		if !priced || u.CompletionTokens <= 0 || u.OutputVariant != "woiv" || videoURL == "" || h.notes[len(h.notes)-1].SettlementID == "" {
			t.Fatalf("task %s incomplete settlement: usage=%+v charge=%+v priced=%v", id, u, c, priced)
		}
		t.Logf("completed task=%s usage=%+v charge_usd=%.11f rates=%+v video_present=true", id, u, c.Total, c.Applied)
		return
	}
	t.Fatalf("poll timeout; resume existing task %s instead of creating again", id)
}
