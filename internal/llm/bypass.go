package llm

import (
	"bytes"
	"encoding/json"
	"github.com/sunqirui1987/xhub/internal/logx"
	"strings"
	"sync"
	"time"
)

var logTraceOnceBypass sync.Once

// QiniuBypassBase is the OpenAI Responses bypass root on Modelink.
// The call is POST {base}/responses, which is /bypass/openai/v1/responses.
const QiniuBypassBase = "https://api.qnaigc.com/bypass/openai/v1"

// IsQiniuBypass reports an api_base that already points at the Qiniu OpenAI bypass root.
func IsQiniuBypass(apiBase string) bool {
	base := strings.TrimRight(strings.TrimSpace(apiBase), "/")
	return base == QiniuBypassBase || strings.HasPrefix(base, QiniuBypassBase+"/")
}

// PrepareQiniuBypass sends chat and responses calls to the bypass Responses API.
// A chat body has messages. The bypass endpoint reads input, so messages are copied there
// and then removed. max_tokens is the chat name; the Responses API reads max_output_tokens.
// Other operations keep their own path. A base that is not the bypass root is unchanged.
func PrepareQiniuBypass(op, apiBase string, body map[string]any) string {
	logTraceOnceBypass.Do(func() { logx.Trace("enter llm.PrepareQiniuBypass") })

	if !IsQiniuBypass(apiBase) || body == nil {
		return op
	}
	if op != "" && op != OpChat && op != OpResponses {
		return op
	}
	if _, ok := body["input"]; !ok {
		if msgs, ok := body["messages"]; ok {
			body["input"] = msgs
		}
	}
	delete(body, "messages")
	if _, ok := body["max_output_tokens"]; !ok {
		if mt, ok := body["max_tokens"]; ok {
			body["max_output_tokens"] = mt
			delete(body, "max_tokens")
		}
	}
	return OpResponses
}

// ResponsesToChat turns one Responses JSON object into a chat completion.
// A body that is already a chat completion, or that is not a response object, is returned unchanged.
func ResponsesToChat(raw []byte, model string) []byte {
	var doc map[string]any
	if json.Unmarshal(raw, &doc) != nil {
		return raw
	}
	if _, ok := doc["choices"]; ok {
		return raw
	}
	if doc["object"] != "response" && doc["output"] == nil && doc["output_text"] == nil {
		return raw
	}
	out := map[string]any{
		"id":      doc["id"],
		"object":  "chat.completion",
		"created": time.Now().UTC().Unix(),
		"model":   model,
		"choices": []any{map[string]any{
			"index": 0,
			"message": map[string]any{
				"role":    "assistant",
				"content": responseText(doc),
			},
			"finish_reason": "stop",
		}},
		"usage": chatUsage(doc["usage"]),
	}
	encoded, err := json.Marshal(out)
	if err != nil {
		return raw
	}
	return encoded
}

// ResponsesSSEToChat turns complete Responses SSE events into chat completion chunks.
// rest is the unfinished tail. flush parses that tail and ends the chat stream with [DONE].
func ResponsesSSEToChat(buf []byte, model string, flush bool) (emit, rest []byte) {
	for {
		idx := bytes.Index(buf, []byte("\n\n"))
		if idx < 0 {
			break
		}
		emit = append(emit, chatChunkFromEvent(buf[:idx], model)...)
		buf = buf[idx+2:]
	}
	if flush {
		if len(bytes.TrimSpace(buf)) > 0 {
			emit = append(emit, chatChunkFromEvent(buf, model)...)
		}
		if len(emit) > 0 || len(bytes.TrimSpace(buf)) == 0 {
			emit = append(emit, []byte("data: [DONE]\n\n")...)
		}
		return emit, nil
	}
	return emit, buf
}

func chatChunkFromEvent(event []byte, model string) []byte {
	payload := eventData(event)
	if len(payload) == 0 || bytes.Equal(bytes.TrimSpace(payload), []byte("[DONE]")) {
		return nil
	}
	var doc map[string]any
	if json.Unmarshal(payload, &doc) != nil {
		return nil
	}
	kind, _ := doc["type"].(string)
	switch {
	case kind == "response.output_text.delta":
		return chatDelta(model, textOf(doc["delta"]), "", nil)
	case kind == "response.reasoning_text.delta" || kind == "response.reasoning_summary_text.delta":
		return chatDelta(model, "", textOf(doc["delta"]), nil)
	case kind == "response.completed" || kind == "response.incomplete":
		response, _ := doc["response"].(map[string]any)
		usage := chatUsage(nil)
		if response != nil {
			usage = chatUsage(response["usage"])
		}
		return chatDelta(model, "", "", usage)
	default:
		return nil
	}
}

func chatDelta(model, content, reasoning string, usage map[string]any) []byte {
	delta := map[string]any{}
	if content != "" {
		delta["content"] = content
	}
	if reasoning != "" {
		delta["reasoning_content"] = reasoning
	}
	choice := map[string]any{"index": 0, "delta": delta}
	if usage != nil {
		choice["finish_reason"] = "stop"
	}
	chunk := map[string]any{
		"object":  "chat.completion.chunk",
		"created": time.Now().UTC().Unix(),
		"model":   model,
		"choices": []any{choice},
	}
	if usage != nil {
		chunk["usage"] = usage
	}
	raw, err := json.Marshal(chunk)
	if err != nil {
		return nil
	}
	return append(append([]byte("data: "), raw...), []byte("\n\n")...)
}

func eventData(event []byte) []byte {
	var data []byte
	for _, line := range bytes.Split(event, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if !bytes.HasPrefix(line, []byte("data:")) {
			continue
		}
		part := bytes.TrimSpace(bytes.TrimPrefix(line, []byte("data:")))
		if len(data) > 0 {
			data = append(data, '\n')
		}
		data = append(data, part...)
	}
	return data
}

func responseText(doc map[string]any) string {
	if text, ok := doc["output_text"].(string); ok && text != "" {
		return text
	}
	var b strings.Builder
	items, _ := doc["output"].([]any)
	for _, item := range items {
		msg, ok := item.(map[string]any)
		if !ok {
			continue
		}
		parts, _ := msg["content"].([]any)
		for _, part := range parts {
			block, ok := part.(map[string]any)
			if !ok {
				continue
			}
			if text, ok := block["text"].(string); ok {
				b.WriteString(text)
			}
		}
	}
	return b.String()
}

func chatUsage(raw any) map[string]any {
	usage, _ := raw.(map[string]any)
	if usage == nil {
		return map[string]any{"prompt_tokens": 0, "completion_tokens": 0, "total_tokens": 0}
	}
	pt := usageInt(usage["prompt_tokens"])
	ct := usageInt(usage["completion_tokens"])
	if _, ok := usage["prompt_tokens"]; !ok {
		pt = usageInt(usage["input_tokens"])
		ct = usageInt(usage["output_tokens"])
	}
	total := usageInt(usage["total_tokens"])
	if total == 0 {
		total = pt + ct
	}
	return map[string]any{"prompt_tokens": pt, "completion_tokens": ct, "total_tokens": total}
}

func usageInt(v any) int {
	switch t := v.(type) {
	case float64:
		return int(t)
	case int:
		return t
	default:
		return 0
	}
}
