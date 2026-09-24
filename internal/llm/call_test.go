package llm

import (
	"encoding/json"
	"testing"
)

func TestEndpointFollowsProviderProtocol(t *testing.T) {
	base := "https://relay.example/v1/"
	cases := []struct {
		op, provider, model, want string
	}{
		{"chat", "openai", "gpt-5.6-sol", "https://relay.example/v1/chat/completions"},
		{"chat", "zai", "glm", "https://relay.example/v1/chat/completions"},
		{"responses", "openai", "gpt-5.6-sol", "https://relay.example/v1/responses"},
		{"embeddings", "openai", "embed", "https://relay.example/v1/embeddings"},
		{"chat", "azure", "gpt-4o", "https://relay.example/v1/openai/deployments/gpt-4o/chat/completions"},
		{"embeddings", "azure", "embed", "https://relay.example/v1/openai/deployments/embed/embeddings"},
		{"chat", "anthropic", "claude", "https://relay.example/v1/v1/messages"},
		{"messages", "anthropic", "claude", "https://relay.example/v1/v1/messages"},
		{"chat", "gemini", "gemini-2", "https://relay.example/v1/v1beta/models/gemini-2:generateContent"},
		{"messages", "gemini", "gemini-2", "https://relay.example/v1/v1beta/models/gemini-2:generateContent"},
	}
	for _, tc := range cases {
		if got := Endpoint(tc.op, tc.provider, base, tc.model); got != tc.want {
			t.Fatalf("%s %s: got %s want %s", tc.op, tc.provider, got, tc.want)
		}
	}
}

func TestEncodeUsesUpstreamModelAndResponsesShape(t *testing.T) {
	body := map[string]any{
		"model": "alias",
		"messages": []any{
			map[string]any{"role": "user", "content": []any{map[string]any{"type": "text", "text": "hi"}}},
		},
	}
	raw, err := Encode("responses", "openai", body, "gpt-5.6-sol")
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	if out["model"] != "gpt-5.6-sol" {
		t.Fatalf("model %v", out["model"])
	}
	msgs := out["messages"].([]any)
	content := msgs[0].(map[string]any)["content"].([]any)
	if content[0].(map[string]any)["type"] != "input_text" {
		t.Fatalf("content %#v", content[0])
	}
	// 编码不能改掉调用方手里的原始消息。
	original := body["messages"].([]any)[0].(map[string]any)["content"].([]any)
	if original[0].(map[string]any)["type"] != "text" {
		t.Fatal("original message was mutated")
	}
}

func TestHeadersCarryBearerAndJSON(t *testing.T) {
	h := Headers("sk-test")
	if h.Get("Authorization") != "Bearer sk-test" || h.Get("Content-Type") != "application/json" {
		t.Fatalf("%v", h)
	}
}

func TestMessagesEncodeRequiresMaxTokens(t *testing.T) {
	raw, err := Encode("messages", "anthropic", map[string]any{"messages": []any{}}, "claude")
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	if out["max_tokens"] != float64(256) {
		t.Fatalf("max_tokens %v", out["max_tokens"])
	}
}
