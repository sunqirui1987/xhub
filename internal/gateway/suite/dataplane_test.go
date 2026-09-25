package suite

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/sunqirui1987/xhub/internal/config"
)

func TestEmbeddingsCompletionsMessagesProxy(t *testing.T) {
	s, master := testEnv(t)
	gen := doJSON(t, s.Handler(), "POST", "/key/generate", master, map[string]any{"key_type": "llm_api"})
	var g map[string]any
	_ = json.Unmarshal(gen.Body.Bytes(), &g)
	sk := g["key"].(string)

	emb := doJSON(t, s.Handler(), "POST", "/v1/embeddings", sk, map[string]any{
		"model": "text-embedding-3-small", "input": "hello",
	})
	if emb.Code != 200 {
		t.Fatalf("embeddings %d %s", emb.Code, emb.Body.String())
	}
	if emb.Result().Header.Get("x-litellm-call-id") == "" {
		t.Fatal("embeddings missing call-id")
	}
	var eb map[string]any
	_ = json.Unmarshal(emb.Body.Bytes(), &eb)
	if eb["model"] != "text-embedding-3-small" {
		t.Fatalf("embeddings alias %v", eb["model"])
	}

	cmp := doJSON(t, s.Handler(), "POST", "/v1/completions", sk, map[string]any{
		"model": "gpt-4o-mini", "prompt": "hi",
	})
	if cmp.Code != 200 {
		t.Fatalf("completions %d %s", cmp.Code, cmp.Body.String())
	}
	if cmp.Result().Header.Get("x-litellm-call-id") == "" {
		t.Fatal("completions missing call-id")
	}
	var cb map[string]any
	_ = json.Unmarshal(cmp.Body.Bytes(), &cb)
	if cb["model"] != "gpt-4o-mini" {
		t.Fatalf("completions alias %v", cb["model"])
	}

	msg := doJSON(t, s.Handler(), "POST", "/v1/messages", sk, map[string]any{
		"model": "gpt-4o-mini", "messages": []any{map[string]any{"role": "user", "content": "hi"}}, "max_tokens": 16,
	})
	if msg.Code != 200 {
		t.Fatalf("messages %d %s", msg.Code, msg.Body.String())
	}
	if msg.Result().Header.Get("x-litellm-call-id") == "" {
		t.Fatal("messages missing call-id")
	}
	var mb map[string]any
	_ = json.Unmarshal(msg.Body.Bytes(), &mb)
	if mb["model"] != "gpt-4o-mini" {
		t.Fatalf("messages alias %v", mb["model"])
	}
}

func TestMasterKeyCannotDataPlaneExtras(t *testing.T) {
	s, master := testEnv(t)
	for _, path := range []string{"/v1/embeddings", "/v1/completions", "/v1/messages"} {
		rec := doJSON(t, s.Handler(), "POST", path, master, map[string]any{"model": "gpt-4o-mini", "input": "x", "prompt": "x", "messages": []any{}})
		if rec.Code != 401 {
			t.Fatalf("%s master want 401 got %d %s", path, rec.Code, rec.Body.String())
		}
	}
}

func TestTPMLimitZeroNoUpstream(t *testing.T) {
	s, master := testEnv(t)
	var hits atomic.Int32
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"object":"list","data":[],"usage":{"prompt_tokens":1}}`))
	}))
	t.Cleanup(up.Close)
	s.Cfg.ModelList[0].LiteLLMParams["api_base"] = up.URL
	gen := doJSON(t, s.Handler(), "POST", "/key/generate", master, map[string]any{"key_type": "llm_api", "tpm_limit": 0})
	var g map[string]any
	_ = json.Unmarshal(gen.Body.Bytes(), &g)
	rec := doJSON(t, s.Handler(), "POST", "/v1/chat/completions", g["key"].(string), map[string]any{
		"model": "gpt-4o-mini", "messages": []any{map[string]any{"role": "user", "content": "hi"}},
	})
	if rec.Code != 429 {
		t.Fatalf("tpm 0 want 429 got %d %s", rec.Code, rec.Body.String())
	}
	if rec.Result().Header.Get("Retry-After") == "" {
		t.Fatal("missing Retry-After")
	}
	if hits.Load() != 0 {
		t.Fatalf("upstream called %d", hits.Load())
	}
}

func TestRPMLimitZeroNoUpstream(t *testing.T) {
	s, master := testEnv(t)
	var hits atomic.Int32
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(up.Close)
	s.Cfg.ModelList[0].LiteLLMParams["api_base"] = up.URL
	gen := doJSON(t, s.Handler(), "POST", "/key/generate", master, map[string]any{"key_type": "llm_api", "rpm_limit": 0})
	var g map[string]any
	_ = json.Unmarshal(gen.Body.Bytes(), &g)
	rec := doJSON(t, s.Handler(), "POST", "/v1/embeddings", g["key"].(string), map[string]any{
		"model": "text-embedding-3-small", "input": "hi",
	})
	if rec.Code != 429 {
		t.Fatalf("rpm 0 want 429 got %d %s", rec.Code, rec.Body.String())
	}
	if rec.Result().Header.Get("Retry-After") == "" {
		t.Fatal("missing Retry-After")
	}
	if hits.Load() != 0 {
		t.Fatalf("upstream called %d", hits.Load())
	}
}

func TestUserBudgetBlocksChat(t *testing.T) {
	s, master := testEnv(t)
	ur := doJSON(t, s.Handler(), "POST", "/user/new", master, map[string]any{
		"user_email": "u@x", "max_budget": 0, "models": []string{"gpt-4o-mini"},
	})
	if ur.Code != 200 {
		t.Fatal(ur.Body.String())
	}
	var u map[string]any
	_ = json.Unmarshal(ur.Body.Bytes(), &u)
	gen := doJSON(t, s.Handler(), "POST", "/key/generate", master, map[string]any{
		"key_type": "llm_api", "user_id": u["user_id"], "models": []string{"gpt-4o-mini", "other"},
	})
	var g map[string]any
	_ = json.Unmarshal(gen.Body.Bytes(), &g)
	rec := doJSON(t, s.Handler(), "POST", "/v1/chat/completions", g["key"].(string), map[string]any{
		"model": "gpt-4o-mini", "messages": []any{map[string]any{"role": "user", "content": "hi"}},
	})
	if rec.Code != 429 {
		t.Fatalf("user budget want 429 got %d %s", rec.Code, rec.Body.String())
	}
}

func TestOrgBudgetBlocksChat(t *testing.T) {
	s, master := testEnv(t)
	or := doJSON(t, s.Handler(), "POST", "/organization/new", master, map[string]any{
		"organization_alias": "acme", "max_budget": 0, "models": []string{"gpt-4o-mini"},
	})
	var o map[string]any
	_ = json.Unmarshal(or.Body.Bytes(), &o)
	gen := doJSON(t, s.Handler(), "POST", "/key/generate", master, map[string]any{
		"key_type": "llm_api", "organization_id": o["organization_id"], "models": []string{"gpt-4o-mini"},
	})
	var g map[string]any
	_ = json.Unmarshal(gen.Body.Bytes(), &g)
	rec := doJSON(t, s.Handler(), "POST", "/v1/chat/completions", g["key"].(string), map[string]any{
		"model": "gpt-4o-mini", "messages": []any{map[string]any{"role": "user", "content": "hi"}},
	})
	if rec.Code != 429 {
		t.Fatalf("org budget want 429 got %d %s", rec.Code, rec.Body.String())
	}
}

func TestGeminiAdapterBody(t *testing.T) {
	s, master := testEnv(t)
	var path string
	var got map[string]any
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"candidates":    []any{map[string]any{"content": map[string]any{"parts": []any{map[string]any{"text": "hi"}}}}},
			"usageMetadata": map[string]any{"promptTokenCount": 8, "candidatesTokenCount": 2},
		})
	}))
	t.Cleanup(up.Close)
	s.Cfg.ModelList = []config.ModelEntry{{
		ModelName: "gem",
		LiteLLMParams: map[string]any{
			"model": "gemini/gemini-1.5-flash", "api_key": "k", "api_base": up.URL,
		},
	}}
	gen := doJSON(t, s.Handler(), "POST", "/key/generate", master, map[string]any{"key_type": "llm_api"})
	var g map[string]any
	_ = json.Unmarshal(gen.Body.Bytes(), &g)
	rec := doJSON(t, s.Handler(), "POST", "/v1/chat/completions", g["key"].(string), map[string]any{
		"model": "gem", "messages": []any{map[string]any{"role": "user", "content": "hello"}},
	})
	if rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	if !strings.Contains(path, ":generateContent") {
		t.Fatalf("gemini url %s", path)
	}
	if _, ok := got["contents"]; !ok {
		t.Fatalf("expected contents, got %#v", got)
	}
	if _, ok := got["messages"]; ok {
		t.Fatalf("openai messages leaked %#v", got)
	}
	if rec.Result().Header.Get("x-litellm-call-id") == "" {
		t.Fatal("missing call-id")
	}
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if out["model"] != "gem" {
		t.Fatalf("alias %v", out["model"])
	}
}
