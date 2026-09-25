package suite

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/gateway"
	"github.com/sunqirui1987/xhub/internal/store"
)

func testDatabaseURL(t *testing.T) string {
	t.Helper()
	base := os.Getenv("XHUB_TEST_DATABASE_URL")
	if base == "" {
		base = "postgres://xhub:xhub_dev_password@127.0.0.1:5433/xhub?sslmode=disable"
	}
	sum := sha256.Sum256([]byte(t.Name() + time.Now().UTC().Format(time.RFC3339Nano)))
	schema := "t_" + hex.EncodeToString(sum[:8])
	if strings.Contains(base, "?") {
		return base + "&search_path=" + schema
	}
	return base + "?search_path=" + schema
}

func testEnv(t *testing.T) (*gateway.Server, string) {
	t.Helper()
	db := testDatabaseURL(t)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "generateContent"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"candidates":    []any{map[string]any{"content": map[string]any{"parts": []any{map[string]any{"text": "hello"}}}}},
				"usageMetadata": map[string]any{"promptTokenCount": 8, "candidatesTokenCount": 2, "totalTokenCount": 10},
			})
		case strings.HasSuffix(r.URL.Path, "/embeddings"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"object": "list",
				"model":  body["model"],
				"data":   []any{map[string]any{"object": "embedding", "index": 0, "embedding": []float64{0.1, 0.2}}},
				"usage":  map[string]any{"prompt_tokens": 3, "total_tokens": 3},
			})
		case strings.HasSuffix(r.URL.Path, "/completions") && !strings.Contains(r.URL.Path, "chat"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "cmpl_test", "object": "text_completion", "model": body["model"],
				"choices": []any{map[string]any{"text": "hello", "index": 0, "finish_reason": "stop"}},
				"usage":   map[string]any{"prompt_tokens": 8, "completion_tokens": 2, "total_tokens": 10},
			})
		case strings.Contains(r.URL.Path, "/messages"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "msg_test", "type": "message", "role": "assistant", "model": body["model"],
				"content": []any{map[string]any{"type": "text", "text": "hello"}},
				"usage":   map[string]any{"input_tokens": 8, "output_tokens": 2},
			})
		case strings.Contains(r.URL.Path, "/images"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"created": 1,
				"data":    []any{map[string]any{"url": "https://example.invalid/img/1"}},
			})
		case strings.Contains(r.URL.Path, "/audio/speech"):
			w.Header().Set("Content-Type", "audio/mpeg")
			_, _ = w.Write([]byte("ID3"))
		case strings.Contains(r.URL.Path, "/audio/transcriptions"), strings.Contains(r.URL.Path, "/audio/translations"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"text": "hello", "language": "en", "duration": 1.0, "segments": []any{},
			})
		case strings.Contains(r.URL.Path, "/moderations"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "modr_test", "model": body["model"],
				"results": []any{map[string]any{"flagged": false, "categories": map[string]any{}, "category_scores": map[string]any{}}},
			})
		case strings.Contains(r.URL.Path, "/rerank"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "rerank_test",
				"results": []any{
					map[string]any{"index": 0, "relevance_score": 0.9},
					map[string]any{"index": 1, "relevance_score": 0.1},
				},
				"meta": map[string]any{"tokens": map[string]any{"input_tokens": 1}},
			})
		case strings.Contains(r.URL.Path, "/responses"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "resp_test", "object": "response", "status": "completed",
				"model": body["model"], "created_at": 1,
				"output": []any{map[string]any{"type": "message", "content": []any{map[string]any{"type": "output_text", "text": "ok"}}}},
				"usage":  map[string]any{"input_tokens": 8, "output_tokens": 2, "total_tokens": 10},
			})
		case strings.HasSuffix(r.URL.Path, "/videos") || strings.Contains(r.URL.Path, "/videos/"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "video_test", "object": "video", "status": "queued",
				"model": body["model"], "created_at": 1,
			})
		case strings.HasSuffix(r.URL.Path, "/chat/completions"):
			if body["stream"] == true {
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = w.Write([]byte("data: {\"id\":\"c1\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\ndata: [DONE]\n\n"))
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":      "chatcmpl_test",
				"object":  "chat.completion",
				"created": 1,
				"model":   body["model"],
				"choices": []any{map[string]any{"index": 0, "message": map[string]any{"role": "assistant", "content": "hello"}, "finish_reason": "stop"}},
				"usage":   map[string]any{"prompt_tokens": 8, "completion_tokens": 2, "total_tokens": 10},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(upstream.Close)
	openai := func(name string) config.ModelEntry {
		return config.ModelEntry{
			ModelName: name,
			LiteLLMParams: map[string]any{
				"model": "openai/" + name, "api_key": "sk-upstream", "api_base": upstream.URL,
			},
		}
	}
	cfg := &config.Config{
		ModelList: []config.ModelEntry{
			openai("gpt-4o-mini"),
			openai("text-embedding-3-small"),
			openai("dall-e-3"),
			openai("whisper-1"),
			openai("tts-1"),
			openai("omni-moderation-latest"),
			openai("sora"),
			openai("rerank-english-v3.0"),
			openai("r"),
			{
				ModelName: "gemini-pro",
				LiteLLMParams: map[string]any{
					"model": "gemini/gemini-pro", "api_key": "sk-upstream", "api_base": upstream.URL,
				},
			},
		},
		RouterSettings:  config.RouterSettings{RoutingStrategy: "simple-shuffle", NumRetries: 2, Timeout: 15},
		GeneralSettings: config.GeneralSettings{MasterKey: "sk-master", DatabaseURL: db},
	}
	st, err := store.Open(cfg.GeneralSettings.DatabaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.DB.Close() })
	return gateway.New(cfg, st), "sk-master"
}

func doJSON(t *testing.T, h http.Handler, method, path, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, rdr)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

func asInt(v any) int {
	switch t := v.(type) {
	case float64:
		return int(t)
	case int:
		return t
	default:
		return 0
	}
}
