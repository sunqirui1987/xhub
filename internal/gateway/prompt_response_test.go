package gateway

import (
	"strings"
	"testing"
)

func TestAssembleLoggedResponseReadsCompletedEvent(t *testing.T) {
	raw := []byte("event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"output\":[{\"type\":\"message\",\"content\":[{\"type\":\"output_text\",\"text\":\"hello\"}]}]}}\n")
	got := assembleLoggedResponse(raw, map[string]any{"body": string(raw)})
	doc, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("type %T", got)
	}
	out, _ := doc["output"].([]any)
	if len(out) != 1 {
		t.Fatalf("output %#v", doc["output"])
	}
}

func TestAssembleLoggedResponseKeepsStreamedToolCalls(t *testing.T) {
	raw := []byte(strings.Join([]string{
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call-1","function":{"name":"exec","arguments":"{\"input\":"}}]}}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"ls\""}}]}}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"}"}}]}}]}`,
		"data: [DONE]",
	}, "\n"))
	got := assembleLoggedResponse(raw, map[string]any{"body": string(raw)})
	doc, _ := got.(map[string]any)
	choices, _ := doc["choices"].([]any)
	choice, _ := choices[0].(map[string]any)
	msg, _ := choice["message"].(map[string]any)
	calls, _ := msg["tool_calls"].([]any)
	call, _ := calls[0].(map[string]any)
	fn, _ := call["function"].(map[string]any)
	if fn["name"] != "exec" || fn["arguments"] != `{"input":"ls"}` {
		t.Fatalf("tool call %#v", fn)
	}
}

func TestAssembleLoggedResponseJoinsChatDeltas(t *testing.T) {
	raw := []byte("data: {\"choices\":[{\"delta\":{\"content\":\"Hel\"}}]}\ndata: {\"choices\":[{\"delta\":{\"content\":\"lo\"}}]}\ndata: [DONE]\n")
	got := assembleLoggedResponse(raw, nil)
	doc, _ := got.(map[string]any)
	choices, _ := doc["choices"].([]any)
	choice, _ := choices[0].(map[string]any)
	msg, _ := choice["message"].(map[string]any)
	if msg["content"] != "Hello" {
		t.Fatalf("content %#v", msg["content"])
	}
}
