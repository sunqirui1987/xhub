package router

import (
	"encoding/json"
	"fmt"
	"strings"
)

// EncodeRequest maps the public JSON body onto the provider wire shape.
func EncodeRequest(op, provider string, body map[string]any, realModel string) ([]byte, error) {
	if provider == "gemini" || provider == "vertex_ai" {
		return encodeGemini(body)
	}
	fwd := map[string]any{}
	for k, v := range body {
		fwd[k] = v
	}
	fwd["model"] = realModel
	if op == "messages" {
		if _, ok := fwd["max_tokens"]; !ok {
			fwd["max_tokens"] = 256
		}
	}
	return json.Marshal(fwd)
}

func encodeGemini(body map[string]any) ([]byte, error) {
	if _, ok := body["contents"]; ok {
		out := map[string]any{}
		for k, v := range body {
			if k == "model" || k == "messages" || k == "stream" {
				continue
			}
			out[k] = v
		}
		return json.Marshal(out)
	}
	var contents []any
	if msgs, ok := body["messages"].([]any); ok {
		for _, raw := range msgs {
			mm, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			role, _ := mm["role"].(string)
			if role == "assistant" {
				role = "model"
			}
			if role == "system" {
				role = "user"
			}
			contents = append(contents, map[string]any{
				"role":  role,
				"parts": []any{map[string]any{"text": stringifyContent(mm["content"])}},
			})
		}
	} else if p, ok := body["prompt"].(string); ok {
		contents = []any{map[string]any{"role": "user", "parts": []any{map[string]any{"text": p}}}}
	} else if p, ok := body["input"].(string); ok {
		contents = []any{map[string]any{"role": "user", "parts": []any{map[string]any{"text": p}}}}
	}
	out := map[string]any{"contents": contents}
	if g, ok := body["generationConfig"]; ok {
		out["generationConfig"] = g
	}
	return json.Marshal(out)
}

func decodeMessagesShape(m map[string]any) {
	if _, ok := m["content"]; !ok {
		if choices, ok := m["choices"].([]any); ok && len(choices) > 0 {
			if c0, ok := choices[0].(map[string]any); ok {
				text := ""
				if msg, ok := c0["message"].(map[string]any); ok {
					text = stringifyContent(msg["content"])
				} else if t, ok := c0["text"].(string); ok {
					text = t
				}
				m["content"] = []any{map[string]any{"type": "text", "text": text}}
			}
		}
	}
	delete(m, "choices")
	delete(m, "object")
	delete(m, "created")
	delete(m, "system_fingerprint")
	m["type"] = "message"
	if _, ok := m["role"]; !ok {
		m["role"] = "assistant"
	}
	if _, ok := m["stop_reason"]; !ok {
		m["stop_reason"] = "end_turn"
	}
	if u, ok := m["usage"].(map[string]any); ok {
		if _, has := u["input_tokens"]; !has {
			u["input_tokens"] = u["prompt_tokens"]
			u["output_tokens"] = u["completion_tokens"]
		}
	}
}

func stringifyContent(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case []any:
		var b strings.Builder
		for _, p := range t {
			if m, ok := p.(map[string]any); ok {
				if s, ok := m["text"].(string); ok {
					b.WriteString(s)
				}
			}
		}
		return b.String()
	default:
		return fmt.Sprint(v)
	}
}

// DecodeResponse maps a provider response onto the public shape and stamps alias.
func DecodeResponse(op, provider, alias string, raw []byte) []byte {
	if op == "audio_speech" {
		return raw
	}
	if (provider == "gemini" || provider == "vertex_ai") && (op == "chat" || op == "completions" || op == "messages" || op == "embeddings") {
		return decodeGemini(op, alias, raw)
	}
	var m map[string]any
	if json.Unmarshal(raw, &m) != nil {
		return raw
	}
	if op != "images" && op != "images_edits" && op != "rerank" && op != "audio_transcription" {
		m["model"] = alias
	}
	if op == "messages" {
		decodeMessagesShape(m)
	}
	b, err := json.Marshal(m)
	if err != nil {
		return raw
	}
	return b
}

func decodeGemini(op, alias string, raw []byte) []byte {
	var g map[string]any
	if json.Unmarshal(raw, &g) != nil {
		return raw
	}
	text := ""
	if cands, ok := g["candidates"].([]any); ok && len(cands) > 0 {
		if c0, ok := cands[0].(map[string]any); ok {
			if content, ok := c0["content"].(map[string]any); ok {
				if parts, ok := content["parts"].([]any); ok {
					for _, p := range parts {
						if pm, ok := p.(map[string]any); ok {
							if t, ok := pm["text"].(string); ok {
								text += t
							}
						}
					}
				}
			}
		}
	}
	usage := map[string]any{}
	if um, ok := g["usageMetadata"].(map[string]any); ok {
		usage["prompt_tokens"] = um["promptTokenCount"]
		usage["completion_tokens"] = um["candidatesTokenCount"]
		usage["total_tokens"] = um["totalTokenCount"]
	}
	out := map[string]any{
		"id":      "gemcmpl",
		"object":  "chat.completion",
		"model":   alias,
		"choices": []any{map[string]any{"index": 0, "message": map[string]any{"role": "assistant", "content": text}, "finish_reason": "stop"}},
		"usage":   usage,
	}
	if op == "messages" {
		out = map[string]any{
			"id":          "msg_" + alias,
			"type":        "message",
			"role":        "assistant",
			"model":       alias,
			"content":     []any{map[string]any{"type": "text", "text": text}},
			"stop_reason": "end_turn",
			"usage":       map[string]any{"input_tokens": usage["prompt_tokens"], "output_tokens": usage["completion_tokens"]},
		}
	}
	if op == "embeddings" {
		out = map[string]any{
			"object": "list",
			"model":  alias,
			"data":   []any{map[string]any{"object": "embedding", "index": 0, "embedding": []float64{0.01}}},
			"usage":  usage,
		}
	}
	b, _ := json.Marshal(out)
	return b
}
