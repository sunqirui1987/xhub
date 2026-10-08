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
// 参数 apiBase（string）：上游根地址，末尾斜杠会被去掉再拼路径。
// 返回 bool（bool）：api_base 已经指向七牛 OpenAI 直通根地址时返回真。
// 调用：仅在 bypass.go 内使用
// 测试：无直接单测
func IsQiniuBypass(apiBase string) bool {
	base := strings.TrimRight(strings.TrimSpace(apiBase), "/")
	return base == QiniuBypassBase || strings.HasPrefix(base, QiniuBypassBase+"/")
}

// PrepareQiniuBypass sends chat and responses calls to the bypass Responses API. A chat body has messages. The bypass endpoint reads input, so messages are copied there and then removed. max_tokens is the chat name; the Responses API reads max_output_tokens. Other operations keep their own path. A base that is not the bypass root is unchanged.
// 参数 op（string）：操作名，例如 chat；apiBase（string）：上游根地址，末尾斜杠会被去掉再拼路径；body（map[string]any）：已解析或原始的 JSON。
// 返回 string（string）：改写后的七牛直通地址。聊天和 responses 走直通的 Responses 路径。
// 调用：dataplane/serve.go
// 测试：bypass_test.go
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

// ResponsesToChat turns one Responses JSON object into a chat completion. A body that is already a chat completion, or that is not a response object, is returned unchanged.
// 参数 raw（[]byte）：原始正文。可能是 JSON，也可能是 SSE，由调用方按内容解析；model（string）：对外模型名，用来选部署和记用量。
// 返回 []byte（[]byte）：序列化后的 JSON 字节。失败时为 nil。
// 调用：dataplane/serve.go
// 测试：bypass_test.go
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

// ResponsesSSEToChat turns complete Responses SSE events into chat completion chunks. rest is the unfinished tail. flush parses that tail and ends the chat stream with [DONE].
// 参数 buf（[]byte）：原始正文。可能是 JSON，也可能是 SSE，由调用方按内容解析；model（string）：对外模型名，用来选部署和记用量；flush（bool）：为真时把缓冲区里剩余的事件也转成对话块，并补上 data: [DONE]。
// 返回 emit（[]byte）：SSE 字节。没有可下发的事件时为 nil 或空；rest（[]byte）：SSE 字节。没有可下发的事件时为 nil 或空。
// 调用：dataplane/stream.go
// 测试：bypass_test.go
func ResponsesSSEToChat(buf []byte, model string, flush bool) (emit, rest []byte) {
	for {
		idx, separatorLen := sseBoundary(buf)
		if idx < 0 {
			break
		}
		emit = append(emit, chatChunkFromEvent(buf[:idx], model)...)
		buf = buf[idx+separatorLen:]
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

// sseBoundary finds the first complete SSE event for either LF or CRLF line endings.
// 参数 buf（[]byte）：尚未解析的 SSE 字节。
// 返回 idx（int）：事件结束位置，未收齐时为 -1；separatorLen（int）：空行分隔符的字节数。
// 调用：ResponsesSSEToChat。
// 测试：bypass_test.go。
func sseBoundary(buf []byte) (idx, separatorLen int) {
	lf := bytes.Index(buf, []byte("\n\n"))
	crlf := bytes.Index(buf, []byte("\r\n\r\n"))
	switch {
	case lf < 0:
		return crlf, 4
	case crlf < 0 || lf < crlf:
		return lf, 2
	default:
		return crlf, 4
	}
}

// 把一条上游 SSE 事件转成对话补全块。增量文本、推理文本和完成用量走不同分支。
// 参数 event（[]byte）：一条 SSE 事件的原始字节，帧之间以空行分隔；model（string）：对外模型名，用来选部署和记用量。
// 返回 []byte（[]byte）：一条对话 SSE 块。事件是 [DONE]、空或无法解析时为 nil。
// 调用：仅在 bypass.go 内使用
// 测试：无直接单测
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

// 组装一条 OpenAI 对话流式块。有用量时带上 finish_reason 和 usage。
// 参数 model（string）：发给上游或对外展示的模型名；content（string）：要拼接或展示的文本。空串表示这段没有内容；reasoning（string）：推理过程文本，写进对话块的 reasoning_content。空串表示这次没有推理增量；usage（map[string]any）：用量对象。字段可能是 prompt_tokens，也可能是 input_tokens。
// 返回 []byte（[]byte）：以 data: 开头、空行结尾的对话补全块。
// 调用：仅在 bypass.go 内使用
// 测试：无直接单测
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

// 从一条 SSE 事件里抽出所有 data: 行，拼成待解析的负载。
// 参数 event（[]byte）：一条 SSE 事件的原始字节，帧之间以空行分隔。
// 返回 []byte（[]byte）：从 SSE 帧抽出的 data 负载，不含 data: 前缀。没有 data 行时为空。
// 调用：仅在 bypass.go 内使用
// 测试：无直接单测
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

// 从官方响应 JSON 里取出用户能看见的文本。
// 参数 doc（map[string]any）：已经解析的 JSON 对象。缺字段表示上游或调用方没有给这项。
// 返回 string（string）：从官方响应里拼出的可见文本。没有 output_text 时拼接 output 里的 text，仍然没有则为空串。
// 调用：仅在 bypass.go 内使用
// 测试：无直接单测
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

// 把官方用量收成 prompt_tokens、completion_tokens 和 total_tokens。
// 参数 raw（any）：JSON 里读出的动态值。数字、字符串和对象都要接住，类型不符时按零值而不是 panic。
// 返回 map[string]any（map[string]any）：对话用量，键是 prompt_tokens、completion_tokens、total_tokens。上游没有用量时三个数都是 0。
// 调用：仅在 bypass.go 内使用
// 测试：无直接单测
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

// 把 JSON 数字收成 int。float64 截断，其他类型当 0，不 panic。
// 参数 v（any）：JSON 里读出的动态值。数字、字符串和对象都要接住，类型不符时按零值而不是 panic。
// 返回 int（int）：从 JSON 或查询参数转成的整数。类型不符或缺失时为 0，不 panic。
// 调用：仅在 bypass.go 内使用
// 测试：无直接单测
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
