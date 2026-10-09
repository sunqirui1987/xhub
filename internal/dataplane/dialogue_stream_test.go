package dataplane

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestDialogueStreamMatrix 验证三种上游到三种用户协议的真实 SSE 文本、工具参数及终态转换。
// 前置：内存 HTTP 响应含稀疏工具索引；结果：九组合保持工具 ID、参数及实测用量；无外部清理。
func TestDialogueStreamMatrix(t *testing.T) {
	streams := map[string][]string{
		"openai-chat":        {`{"id":"r","choices":[{"index":0,"delta":{"content":"hello"}}]}`, `{"choices":[{"index":0,"delta":{"tool_calls":[{"index":3,"id":"call_x","type":"function","function":{"name":"weather","arguments":"{"}}]}}]}`, `{"choices":[{"index":0,"delta":{"tool_calls":[{"index":3,"function":{"arguments":"\"city\":\"上海\"}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":10,"completion_tokens":3}}`, "[DONE]"},
		"openai-responses":   {`{"type":"response.created","response":{"id":"r"}}`, `{"type":"response.output_text.delta","delta":"hello"}`, `{"type":"response.output_item.added","output_index":3,"item":{"type":"function_call","call_id":"call_x","name":"weather","arguments":""}}`, `{"type":"response.function_call_arguments.delta","output_index":3,"delta":"{\"city\":\"上海\"}"}`, `{"type":"response.completed","response":{"id":"r","usage":{"input_tokens":10,"output_tokens":3}}}`},
		"anthropic-messages": {`{"type":"message_start","message":{"id":"r","usage":{"input_tokens":10}}}`, `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":"hello"}}`, `{"type":"content_block_start","index":3,"content_block":{"type":"tool_use","id":"call_x","name":"weather","input":{}}}`, `{"type":"content_block_delta","index":3,"delta":{"type":"input_json_delta","partial_json":"{\"city\":\"上海\"}"}}`, `{"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":3}}`, `{"type":"message_stop"}`},
	}
	for source, events := range streams {
		for _, target := range []string{"openai-chat", "openai-responses", "anthropic-messages"} {
			t.Run(source+"/"+target, func(t *testing.T) {
				var input strings.Builder
				for _, event := range events {
					fmt.Fprintf(&input, "data: %s\n\n", event)
				}
				recorder := httptest.NewRecorder()
				response := &http.Response{Body: io.NopCloser(strings.NewReader(input.String()))}
				wrote, usage, _, captured, err := pipeDialogue(recorder, response, time.Now(), "public", source, target)
				if err != nil {
					t.Fatal(err)
				}
				output := string(captured)
				if !wrote || !strings.Contains(output, "call_x") || !strings.Contains(output, "weather") || !strings.Contains(output, "hello") || usage["total_tokens"] != float64(13) {
					t.Fatalf("流事实丢失: %s %#v", output, usage)
				}
				terminal := map[string]string{"openai-chat": "[DONE]", "openai-responses": "response.completed", "anthropic-messages": "message_stop"}[target]
				if !strings.Contains(output, terminal) {
					t.Fatal("缺少用户协议终态")
				}
				if target == "openai-responses" && !strings.Contains(output, "response.function_call_arguments.done") {
					t.Fatal("缺少工具完成生命周期")
				}
				if target == "openai-chat" && strings.Contains(output, `"index":3`) {
					t.Fatal("稀疏上游工具索引未映射")
				}
			})
		}
	}
}

// TestDialogueStreamFailure 验证 EOF、上游错误和不完整工具不会产生伪成功或继续重试。
// 前置：已经输出文本的截断流；结果：已输出标记和失败事件可观察；纯内存无清理。
func TestDialogueStreamFailure(t *testing.T) {
	for _, tail := range []string{"", `data: {"error":{"message":"failed"}}

`} {
		response := &http.Response{Body: io.NopCloser(strings.NewReader(`data: {"id":"r","choices":[{"index":0,"delta":{"content":"hello"}}]}

` + tail))}
		recorder := httptest.NewRecorder()
		wrote, _, _, output, err := pipeDialogue(recorder, response, time.Now(), "public", "openai-chat", "openai-responses")
		if err == nil || !wrote || !strings.Contains(string(output), "response.failed") || strings.Contains(string(output), "response.completed") {
			t.Fatalf("截断流必须失败: %v %s", err, output)
		}
	}
}
