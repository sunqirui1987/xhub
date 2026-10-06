package gateway

import "testing"

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
