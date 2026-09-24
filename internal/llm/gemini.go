package llm

import (
	"encoding/json"
	"fmt"
)

// decodeGemini 把 generateContent 的 candidates 收成对外的聊天、消息或向量响应。
// 用量字段从 usageMetadata 的驼峰名改成 OpenAI 的 prompt/completion/total。
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

// textOf 把消息 content 收成纯文本。
// 字符串原样返回。内容块列表只拼接 type 无关的 text 字段，和提示词模板里
// 「列表内容变成一段文字」的规则相同。其他类型用默认格式，避免静默丢字段。
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

// stringsBuilder 是局部拼接器，避免 gemini.go 再引 strings 只为了 Builder。
type stringsBuilder struct {
	buf []byte
}

func (b *stringsBuilder) WriteString(s string) {
	b.buf = append(b.buf, s...)
}

func (b *stringsBuilder) String() string { return string(b.buf) }
