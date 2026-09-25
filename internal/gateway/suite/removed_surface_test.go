package suite

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sunqirui1987/xhub/internal/config"
)

func TestRemovedColumns404AndChatKeepsTools(t *testing.T) {
	s, master := testEnv(t)
	h := s.Handler()
	for _, path := range []string{
		"/v1/agents",
		"/v1/workflows/runs",
		"/v1/memory",
		"/v1/mcp/server",
		"/v1/skills",
		"/policies/list",
		"/search_tools/list",
		"/v1/vector_stores",
		"/prompts/list",
		"/tag/list",
		"/cache/settings",
		"/cache/ping",
		"/tools",
		"/tools/list",
		"/toolset",
		"/v1/tool/list",
		"/model_hub",
	} {
		rec := doJSON(t, h, "GET", path, master, nil)
		if rec.Code != 404 {
			t.Fatalf("%s want 404 got %d %s", path, rec.Code, rec.Body.String())
		}
	}
	for _, path := range []string{"/cache/settings/test", "/tools/call", "/test/tools/list"} {
		rec := doJSON(t, h, "POST", path, master, map[string]any{})
		if rec.Code != 404 {
			t.Fatalf("POST %s want 404 got %d %s", path, rec.Code, rec.Body.String())
		}
	}
	ready := doJSON(t, h, "GET", "/health/readiness", "", nil)
	if ready.Code != 200 || !strings.Contains(ready.Body.String(), `"status":"ready"`) {
		t.Fatalf("readiness %d %s", ready.Code, ready.Body.String())
	}
	kept := doJSON(t, h, "GET", "/v1/tool/spend", master, nil)
	if kept.Code == 404 {
		t.Fatalf("tool spend should stay %d %s", kept.Code, kept.Body.String())
	}
	hits := doJSON(t, h, "GET", "/global/activity/cache_hits", master, nil)
	if hits.Code == 404 {
		t.Fatalf("cache hit activity should stay %d %s", hits.Code, hits.Body.String())
	}
	info := doJSON(t, h, "GET", "/model/info", master, nil)
	if info.Code != 200 || !strings.Contains(info.Body.String(), `"data"`) {
		t.Fatalf("model info %d %s", info.Code, info.Body.String())
	}

	var seen []byte
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"c","object":"chat.completion","choices":[{"message":{"role":"assistant","content":"ok"}}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	t.Cleanup(up.Close)
	s.Cfg.ModelList = []config.ModelEntry{{
		ModelName: "gpt-4o-mini",
		LiteLLMParams: map[string]any{
			"model": "openai/gpt-4o-mini", "api_key": "sk-upstream", "api_base": up.URL,
		},
	}}
	sk := mintLLM(t, s, master)
	chat := doJSON(t, h, "POST", "/v1/chat/completions", sk, map[string]any{
		"model":    "gpt-4o-mini",
		"messages": []any{map[string]any{"role": "user", "content": "hi"}},
		"tools":    []any{map[string]any{"type": "function", "function": map[string]any{"name": "lookup"}}},
	})
	if chat.Code != 200 {
		t.Fatalf("chat %d %s", chat.Code, chat.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(seen, &body); err != nil {
		t.Fatal(err)
	}
	tools, ok := body["tools"].([]any)
	if !ok || len(tools) != 1 {
		t.Fatalf("tools stripped %s", seen)
	}
}
