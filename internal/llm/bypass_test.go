package llm

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPrepareQiniuBypassUsesResponsesInput(t *testing.T) {
	body := map[string]any{
		"model":      "doubao-seed-1.6",
		"messages":   []any{map[string]any{"role": "user", "content": "hi"}},
		"max_tokens": 16,
		"stream":     true,
	}
	op := PrepareQiniuBypass("chat", QiniuBypassBase, body)
	if op != OpResponses {
		t.Fatalf("op %s", op)
	}
	if _, ok := body["messages"]; ok {
		t.Fatal("messages stayed on the bypass body")
	}
	if body["max_output_tokens"] != 16 {
		t.Fatalf("max_output_tokens %v", body["max_output_tokens"])
	}
	input, _ := body["input"].([]any)
	if len(input) != 1 {
		t.Fatalf("input %#v", body["input"])
	}
	if PrepareQiniuBypass("chat", "https://api.openai.com/v1", body) != "chat" {
		t.Fatal("a normal base was rewritten")
	}
	if got := Endpoint(OpResponses, "openai", QiniuBypassBase, "doubao-seed-1.6"); got != QiniuBypassBase+"/responses" {
		t.Fatalf("bypass url %s", got)
	}
}

func TestResponsesToChat(t *testing.T) {
	raw := []byte(`{"id":"resp_1","object":"response","output_text":"hello","output":[{"content":[{"type":"output_text","text":"hello"}]}],"usage":{"input_tokens":3,"output_tokens":1,"total_tokens":4}}`)
	got := ResponsesToChat(raw, "doubao-seed-1.6")
	var doc map[string]any
	if err := json.Unmarshal(got, &doc); err != nil {
		t.Fatal(err)
	}
	if doc["object"] != "chat.completion" || doc["model"] != "doubao-seed-1.6" {
		t.Fatalf("%s", got)
	}
	usage, _ := doc["usage"].(map[string]any)
	if usage["prompt_tokens"].(float64) != 3 || usage["completion_tokens"].(float64) != 1 {
		t.Fatalf("usage %#v", usage)
	}
}

func TestResponsesSSEToChat(t *testing.T) {
	src := strings.Join([]string{
		`event: response.output_text.delta`,
		`data: {"type":"response.output_text.delta","delta":"hi"}`,
		``,
		`event: response.completed`,
		`data: {"type":"response.completed","response":{"usage":{"input_tokens":2,"output_tokens":1,"total_tokens":3}}}`,
		``,
		``,
	}, "\n")
	emit, rest := ResponsesSSEToChat([]byte(src), "doubao-seed-1.6", true)
	if len(rest) != 0 {
		t.Fatalf("rest %q", rest)
	}
	text := string(emit)
	if !strings.Contains(text, `"content":"hi"`) || !strings.Contains(text, `"prompt_tokens":2`) || !strings.Contains(text, "data: [DONE]") {
		t.Fatalf("%s", text)
	}
}
