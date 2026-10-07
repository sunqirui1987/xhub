// Package llm rewrites Gemini and Vertex generateContent requests and responses.
package llm

import (
	"encoding/json"
	"fmt"
	"github.com/sunqirui1987/xhub/internal/logx"
	"sync"
)

var logTraceOnceGemini sync.Once

// decodeGemini turns generateContent candidates into the public chat, message, or embedding response. Usage fields are renamed from usageMetadata camel case to OpenAI prompt, completion, and total.
// 参数 op（string）：操作名或 call_type，写入用量行并选择协议；alias（string）：对外模型名，用来选部署和记用量；raw（[]byte）：原始正文。可能是 JSON，也可能是 SSE，由调用方按内容解析。
// 返回 []byte（[]byte）：解码Gemini的原始字节。没有内容时长度为 0。
// 调用：llm/call.go
// 测试：无直接单测
func decodeGemini(op, alias string, raw []byte) []byte {
	logTraceOnceGemini.Do(func() { logx.Trace("enter llm.decodeGemini") })

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

// textOf flattens a message content value into plain text. A string is returned as itself. A list of content blocks concatenates text fields regardless of type, matching the prompt-template rule that list content becomes one paragraph. Other types use the default format so a field is not dropped silently.
// 参数 v（any）：JSON 里读出的动态值。数字、字符串和对象都要接住，类型不符时按零值而不是 panic。
// 返回 string（string）：拼成一段的纯文本。字符串原样返回，内容块列表把各 text 字段接在一起。
// 调用：llm/build.go、llm/bypass.go、llm/call.go。
// 测试：无直接单测
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
// 参数 s（string）：写入字符串要处理的文本。空串表示这段没有内容。
// 返回：无。这段文本已接进 Gemini 请求缓冲。
// 调用：仅在 gemini.go 内使用。
// 测试：builtin_providers_test.go、failure_log_test.go
func (b *stringsBuilder) WriteString(s string) {
	b.buf = append(b.buf, s...)
}

// String returns the Gemini request text accumulated so far.
// 参数：无。
// 返回 string（string）：目前已经拼进缓冲区的 Gemini 请求文本。
// 调用：仅在 gemini.go 内使用。
// 测试：access_log_test.go、bypass_logic_test.go、console_split_test.go
func (b *stringsBuilder) String() string { return string(b.buf) }
