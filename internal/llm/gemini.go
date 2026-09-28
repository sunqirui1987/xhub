// Package llm rewrites Gemini and Vertex generateContent requests and responses.
package llm

import (
	"encoding/json"
	"fmt"
)

// decodeGemini turns generateContent candidates into the public chat, message, or embedding response.
// Usage fields are renamed from usageMetadata camel case to OpenAI prompt, completion, and total.
func decodeGemini(op, alias string, raw []byte) []byte {
	var doc map[string]any
	if json.Unmarshal(raw, &doc) != nil {
		return raw
	}
	text := ""
	if candidates, ok := doc["candidates"].([]any); ok && len(candidates) > 0 {
		if first, ok := candidates[0].(map[string]any); ok {
			if content, ok := first["content"].(map[string]any); ok {
				if parts, ok := content["parts"].([]any); ok {
					for _, part := range parts {
						if item, ok := part.(map[string]any); ok {
							if piece, ok := item["text"].(string); ok {
								text += piece
							}
						}
					}
				}
			}
		}
	}
	usage := map[string]any{}
	if meta, ok := doc["usageMetadata"].(map[string]any); ok {
		usage["prompt_tokens"] = meta["promptTokenCount"]
		usage["completion_tokens"] = meta["candidatesTokenCount"]
		usage["total_tokens"] = meta["totalTokenCount"]
	}
	out := map[string]any{
		"id":     "gemcmpl",
		"object": "chat.completion",
		"model":  alias,
		"choices": []any{map[string]any{
			"index": 0,
			"message": map[string]any{
				"role":    "assistant",
				"content": text,
			},
			"finish_reason": "stop",
		}},
		"usage": usage,
	}
	if op == OpMessages {
		out = map[string]any{
			"id":          "msg_" + alias,
			"type":        "message",
			"role":        "assistant",
			"model":       alias,
			"content":     []any{map[string]any{"type": "text", "text": text}},
			"stop_reason": "end_turn",
			"usage": map[string]any{
				"input_tokens":  usage["prompt_tokens"],
				"output_tokens": usage["completion_tokens"],
			},
		}
	}
	if op == OpEmbeddings {
		out = map[string]any{
			"object": "list",
			"model":  alias,
			"data": []any{map[string]any{
				"object":    "embedding",
				"index":     0,
				"embedding": []float64{0.01},
			}},
			"usage": usage,
		}
	}
	encoded, err := json.Marshal(out)
	if err != nil {
		return raw
	}
	return encoded
}

// textOf flattens a message content value into plain text.
// A string is returned as itself. A list of content blocks concatenates text fields regardless of type, matching the prompt-template rule
// that list content becomes one paragraph. Other types use the default format so a field is not dropped silently.
func textOf(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case []any:
		var b stringsBuilder
		for _, part := range t {
			item, ok := part.(map[string]any)
			if !ok {
				continue
			}
			if text, ok := item["text"].(string); ok {
				b.WriteString(text)
			}
		}
		return b.String()
	default:
		return fmt.Sprint(v)
	}
}

// stringsBuilder is a local concatenator so gemini.go does not import strings only for Builder.
type stringsBuilder struct {
	buf []byte
}

// WriteString appends a string to the Gemini request buffer.
func (b *stringsBuilder) WriteString(s string) {
	b.buf = append(b.buf, s...)
}

// String returns the Gemini request text accumulated so far.
func (b *stringsBuilder) String() string { return string(b.buf) }
