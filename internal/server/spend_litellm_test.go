package server

import (
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/sunqirui1987/xhub/internal/config"
)

// TestSpendLogMatchesLiteLLM compares a finished chat, a finished stream, and
// GET /spend/logs/ui with the payload LiteLLM's spend writer stores.
func TestSpendLogMatchesLiteLLM(t *testing.T) {
	py := os.Getenv("LITELLM_PYTHON")
	if py == "" {
		py = "/Users/sunqirui/Downloads/litellm-main/.venv/bin/python"
	}
	outPath := filepath.Join(t.TempDir(), "spend.json")
	cmd := exec.Command(py, "spend_litellm_capture.py", outPath)
	cmd.Env = append(os.Environ(), "PYTHONPATH=/Users/sunqirui/Downloads/litellm-main", "LITELLM_ROOT=/Users/sunqirui/Downloads/litellm-main")
	if msg, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("litellm spend: %v\n%s", err, msg)
	}
	raw, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	var pyOut struct {
		KnownCost float64 `json:"known_cost"`
		Mystery   struct {
			Spend            float64 `json:"spend"`
			Status           string  `json:"status"`
			PromptTokens     float64 `json:"prompt_tokens"`
			CompletionTokens float64 `json:"completion_tokens"`
		} `json:"mystery_log"`
		Stream struct {
			Spend  float64 `json:"spend"`
			Status string  `json:"status"`
		} `json:"stream_log"`
		UIKeys     []string `json:"ui_keys"`
		UIPage     float64  `json:"ui_page"`
		UIPageSize float64  `json:"ui_page_size"`
		UITotal    float64  `json:"ui_total"`
		UIPages    float64  `json:"ui_total_pages"`
	}
	if err := json.Unmarshal(raw, &pyOut); err != nil {
		t.Fatalf("decode: %v\n%s", err, raw)
	}

	s, master := testEnv(t)
	h := s.Handler()
	stream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"usage\":{\"prompt_tokens\":8,\"completion_tokens\":2,\"total_tokens\":10}}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	t.Cleanup(stream.Close)
	s.Cfg.ModelList = append(s.Cfg.ModelList, config.ModelEntry{
		ModelName: "mystery",
		LiteLLMParams: map[string]any{
			"model": "openai/mystery", "api_key": "sk-upstream", "api_base": stream.URL,
		},
	})
	sk := mintLLM(t, s, master)

	chat := doJSON(t, h, "POST", "/v1/chat/completions", sk, map[string]any{
		"model": "gpt-4o-mini", "messages": []any{map[string]any{"role": "user", "content": "hi"}},
	})
	if chat.Code != 200 {
		t.Fatal(chat.Body.String())
	}
	streamed := doJSON(t, h, "POST", "/v1/chat/completions", sk, map[string]any{
		"model": "mystery", "stream": true, "messages": []any{map[string]any{"role": "user", "content": "hi"}},
	})
	if streamed.Code != 200 {
		t.Fatalf("stream %d %s", streamed.Code, streamed.Body.String())
	}

	ui := doJSON(t, h, "GET", "/spend/logs/ui?page=1&page_size=25", master, nil)
	if ui.Code != 200 {
		t.Fatal(ui.Body.String())
	}
	page := decodeBody(t, ui.Body.Bytes())
	for _, key := range []string{"data", "total", "page", "page_size", "total_pages"} {
		if _, ok := page[key]; !ok {
			t.Fatalf("ui missing %s in %s", key, ui.Body.String())
		}
	}
	if page["page"] != pyOut.UIPage || page["page_size"] != pyOut.UIPageSize {
		t.Fatalf("page go %v/%v py %v/%v", page["page"], page["page_size"], pyOut.UIPage, pyOut.UIPageSize)
	}
	rows, _ := page["data"].([]any)
	var mystery, known map[string]any
	for _, item := range rows {
		row, _ := item.(map[string]any)
		switch row["model"] {
		case "mystery":
			mystery = row
		case "gpt-4o-mini":
			known = row
		}
	}
	if mystery == nil || known == nil {
		t.Fatalf("logs missing rows %s", ui.Body.String())
	}
	if mystery["spend"] != pyOut.Mystery.Spend || mystery["status"] != pyOut.Mystery.Status {
		t.Fatalf("mystery go spend %v status %v py %v %s", mystery["spend"], mystery["status"], pyOut.Mystery.Spend, pyOut.Mystery.Status)
	}
	if mystery["prompt_tokens"] != pyOut.Mystery.PromptTokens || mystery["completion_tokens"] != pyOut.Mystery.CompletionTokens {
		t.Fatalf("mystery tokens go %v/%v py %v/%v", mystery["prompt_tokens"], mystery["completion_tokens"], pyOut.Mystery.PromptTokens, pyOut.Mystery.CompletionTokens)
	}
	if mystery["spend"] != pyOut.Stream.Spend || mystery["status"] != pyOut.Stream.Status {
		t.Fatalf("stream log go %v %v py %v %s", mystery["spend"], mystery["status"], pyOut.Stream.Spend, pyOut.Stream.Status)
	}
	knownSpend, _ := known["spend"].(float64)
	if math.Abs(knownSpend-pyOut.KnownCost) > 1e-12 {
		t.Fatalf("gpt-4o-mini spend go %v py %v", known["spend"], pyOut.KnownCost)
	}
	if path := os.Getenv("LITELLM_SPEND_COMPARE_OUT"); path != "" {
		side := map[string]any{"litellm": json.RawMessage(raw), "go_ui": page}
		enc, _ := json.MarshalIndent(side, "", "  ")
		if err := os.WriteFile(path, append(enc, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
