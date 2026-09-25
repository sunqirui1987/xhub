package suite

import (
	"encoding/json"
	"github.com/sunqirui1987/xhub/internal/gateway"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/sunqirui1987/xhub/internal/plugin"
)

// refuseExt 拒绝推理。数据面源码里没有这个名字。
type refuseExt struct{}

func (refuseExt) Name() string { return "gate-refuse" }

func (refuseExt) BeforeUpstream(plugin.Call) plugin.Decision {
	return plugin.Decision{Refuse: true, Status: 403, Code: "extension_refused", Message: "blocked by gate-refuse"}
}

// stampExt 在响应头写下自己的注册名。数据面只按注册表调用，不写这个名字。
type stampExt struct{}

func (stampExt) Name() string { return "lab-stamp" }

func (stampExt) BeforeUpstream(plugin.Call) plugin.Decision {
	return plugin.Decision{Header: map[string]string{"X-Xhub-Extension": "lab-stamp"}}
}

func countingChat(t *testing.T, s *gateway.Server, master string) (plain string, hits *atomic.Int32) {
	t.Helper()
	hits = &atomic.Int32{}
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl_test","object":"chat.completion","choices":[{"message":{"role":"assistant","content":"hello"}}],"usage":{"prompt_tokens":8,"completion_tokens":2,"total_tokens":10}}`))
	}))
	t.Cleanup(up.Close)
	s.Cfg.ModelList[0].LiteLLMParams["api_base"] = up.URL
	gen := doJSON(t, s.Handler(), "POST", "/key/generate", master, map[string]any{"key_type": "llm_api"})
	var g map[string]any
	if err := json.Unmarshal(gen.Body.Bytes(), &g); err != nil {
		t.Fatal(err)
	}
	key, _ := g["key"].(string)
	if key == "" {
		t.Fatalf("key generate %d %s", gen.Code, gen.Body.String())
	}
	return key, hits
}

func chatBody() map[string]any {
	return map[string]any{
		"model":    "gpt-4o-mini",
		"messages": []any{map[string]any{"role": "user", "content": "hi"}},
	}
}

func TestExtensionRefuseSkipsUpstream(t *testing.T) {
	s, master := testEnv(t)
	plain, hits := countingChat(t, s, master)
	if err := s.Extensions().Register(refuseExt{}); err != nil {
		t.Fatal(err)
	}
	rec := doJSON(t, s.Handler(), "POST", "/v1/chat/completions", plain, chatBody())
	if rec.Code != 403 || !strings.Contains(rec.Body.String(), "blocked by gate-refuse") {
		t.Fatalf("refusal %d %s", rec.Code, rec.Body.String())
	}
	if hits.Load() != 0 {
		t.Fatalf("upstream called %d", hits.Load())
	}

	open, master := testEnv(t)
	plain, hits = countingChat(t, open, master)
	rec = doJSON(t, open.Handler(), "POST", "/v1/chat/completions", plain, chatBody())
	if rec.Code != 200 {
		t.Fatalf("empty registry %d %s", rec.Code, rec.Body.String())
	}
	if hits.Load() != 1 {
		t.Fatalf("upstream hits %d", hits.Load())
	}
}

func TestExtensionEffectOnlyWhenRegisteredByName(t *testing.T) {
	s, master := testEnv(t)
	plain, hits := countingChat(t, s, master)
	if err := s.Extensions().Register(stampExt{}); err != nil {
		t.Fatal(err)
	}
	selected, err := s.Extensions().Invoke("lab-stamp", plugin.Call{Op: "chat", Model: "gpt-4o-mini"})
	if err != nil || selected.Header["X-Xhub-Extension"] != "lab-stamp" {
		t.Fatalf("invoke %+v %v", selected, err)
	}
	rec := doJSON(t, s.Handler(), "POST", "/v1/chat/completions", plain, chatBody())
	if rec.Code != 200 {
		t.Fatalf("stamped %d %s", rec.Code, rec.Body.String())
	}
	if rec.Result().Header.Get("X-Xhub-Extension") != "lab-stamp" {
		t.Fatalf("header %q body %s", rec.Result().Header.Get("X-Xhub-Extension"), rec.Body.String())
	}
	if hits.Load() != 1 {
		t.Fatalf("upstream hits %d", hits.Load())
	}

	open, master := testEnv(t)
	plain, _ = countingChat(t, open, master)
	if _, err := open.Extensions().Invoke("lab-stamp", plugin.Call{}); err == nil {
		t.Fatal("unregistered name invoked")
	}
	rec = doJSON(t, open.Handler(), "POST", "/v1/chat/completions", plain, chatBody())
	if rec.Code != 200 {
		t.Fatalf("unstamped %d %s", rec.Code, rec.Body.String())
	}
	if rec.Result().Header.Get("X-Xhub-Extension") != "" {
		t.Fatalf("header leaked %q", rec.Result().Header.Get("X-Xhub-Extension"))
	}
}
