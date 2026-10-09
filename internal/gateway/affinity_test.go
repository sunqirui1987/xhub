package gateway

import (
	"github.com/sunqirui1987/xhub/internal/auth"
	"github.com/sunqirui1987/xhub/internal/dataplane"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSessionIDSticksToThePromptPrefix(t *testing.T) {
	body := map[string]any{
		"model": "gpt-5.6-sol",
		"messages": []any{
			map[string]any{"role": "system", "content": "You are Codex."},
			map[string]any{"role": "user", "content": "hello"},
			map[string]any{"role": "user", "content": "again"},
		},
	}
	first := sessionID(&http.Request{Header: http.Header{}}, body)
	body["messages"] = []any{
		map[string]any{"role": "system", "content": "You are Codex."},
		map[string]any{"role": "user", "content": "hello"},
		map[string]any{"role": "assistant", "content": "hi"},
		map[string]any{"role": "user", "content": "second turn"},
	}
	second := sessionID(&http.Request{Header: http.Header{}}, body)
	if first == "" || first != second {
		t.Fatalf("prefix session changed: %q vs %q", first, second)
	}
}

// TestContinuationPinsDoNotCrossCallerOrModel 验证响应归属按调用方、公开型号和入口隔离。
// 参数 t：测试上下文；本地内存建立归属，跨范围续接必须拒绝，无外部数据需要清理。
func TestContinuationPinsDoNotCrossCallerOrModel(t *testing.T) {
	s := &Server{}
	s.CommitRoute(dataplane.RoutePlan{Alias: "model-a", Caller: "user:alice", Endpoint: "responses"}, "supplier-a", "resp_shared")
	req := httptest.NewRequest("POST", "/v1/responses", nil)
	body := map[string]any{"previous_response_id": "resp_shared"}
	if got := s.PlanRoute(req, "model-a", body, &auth.Principal{UserID: "alice"}).Pinned; got != "supplier-a" {
		t.Fatal(got)
	}
	if got := s.PlanRoute(req, "model-a", body, &auth.Principal{UserID: "bob"}).Pinned; got != "" {
		t.Fatal("cross-caller pin", got)
	}
	if got := s.PlanRoute(req, "model-b", body, &auth.Principal{UserID: "alice"}).Pinned; got != "" {
		t.Fatal("cross-model pin", got)
	}
	if sessionPinKey("a|b", "c", "d") == sessionPinKey("a", "b|c", "d") {
		t.Fatal("tuple separator collision")
	}
}

func TestSessionIDPrefersAnExplicitHeader(t *testing.T) {
	h := http.Header{}
	h.Set("session-id", "conv-1")
	got := sessionID(&http.Request{Header: h}, map[string]any{"prompt_cache_key": "other"})
	if got != "conv-1" {
		t.Fatalf("session id %q", got)
	}
}

func TestSessionIDReadsClaudeMetadataAndClientHeaders(t *testing.T) {
	body := map[string]any{
		"metadata":         map[string]any{"user_id": `{"device_id":"dev","session_id":"json-sess"}`},
		"prompt_cache_key": "cache-should-lose",
	}
	if got := sessionID(nil, body); got != "json-sess" {
		t.Fatalf("json metadata session %q", got)
	}
	h := http.Header{}
	h.Set("X-Session-Id", "opencode-1")
	if got := sessionID(&http.Request{Header: h}, map[string]any{"prompt_cache_key": "body"}); got != "opencode-1" {
		t.Fatalf("header session %q", got)
	}
	if got := sessionID(nil, map[string]any{"prompt_cache_key": "cache-1"}); got != "cache-1" {
		t.Fatalf("cache key session %q", got)
	}
	if got := sessionID(nil, map[string]any{"previous_response_id": "resp_chain"}); got != "prev:resp_chain" {
		t.Fatalf("previous response session %q", got)
	}
}

func TestContentSessionStaysStableAsTheTranscriptGrows(t *testing.T) {
	first := map[string]any{"input": []any{
		map[string]any{"role": "user", "content": []any{map[string]any{"type": "input_text", "text": "hello"}}},
	}}
	second := map[string]any{"input": []any{
		map[string]any{"role": "user", "content": []any{map[string]any{"type": "input_text", "text": "hello"}}},
		map[string]any{"role": "assistant", "content": "pong"},
		map[string]any{"role": "user", "content": "next"},
	}}
	if sessionID(nil, first) == "" || sessionID(nil, first) != sessionID(nil, second) {
		t.Fatalf("responses session drifted: %q %q", sessionID(nil, first), sessionID(nil, second))
	}
}

func TestAffinityPinIsReused(t *testing.T) {
	s := &Server{}
	s.affinitySet(sessionPinKey("gpt", "user:1", "conv-1"), "dep-a")
	if got := s.affinityGet(sessionPinKey("gpt", "user:1", "conv-1")); got != "dep-a" {
		t.Fatalf("pin %q", got)
	}
}
