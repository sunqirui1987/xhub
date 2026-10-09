package dataplane

import (
	"github.com/sunqirui1987/xhub/internal/llm"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestResponseIDProtocols 验证 JSON/SSE 正常、坏帧和输出项归属；参数 t 为测试上下文。
// 只使用内存，必须返回响应 ID 且忽略输出项，无外部数据需要清理。
func TestResponseIDProtocols(t *testing.T) {
	for _, tc := range []struct{ raw, want string }{
		{"{\"id\":\"chatcmpl-a\"}", "chatcmpl-a"},
		{"data: {\"type\":\"response.output_item.added\",\"id\":\"item-a\"}\n\ndata: {\"type\":\"response.created\",\"response\":{\"id\":\"resp-a\"}}\n\n", "resp-a"},
		{"data: {\"type\":\"message_start\",\"message\":{\"id\":\"msg-a\"}}\n", "msg-a"},
		{"data: broken\ndata: {\"id\":\"\"}\ndata: [DONE]\n", ""},
		{"{\"type\":\"response.created\",\"response\":{\"id\":12}}", ""},
		{"data: {\"id\":\"chatcmpl-b\"}\n", "chatcmpl-b"},
	} {
		if got := responseID([]byte(tc.raw)); got != tc.want {
			t.Fatalf("归属 ID raw=%q got=%q want=%q", tc.raw, got, tc.want)
		}
	}
}

// TestResponsesRequestRestoresToolsAndValidates 验证增量输入恢复工具调用和指令隔离。
// 参数 t 为测试上下文；正常、缺输入、错误字段均在内存检查，无外部数据需要清理。
func TestResponsesRequestRestoresToolsAndValidates(t *testing.T) {
	history := []llm.Turn{{Role: "user", Text: "first"}, {Role: "assistant", Calls: []llm.Turn{{ID: "call-a", Name: "lookup", Arguments: "{}"}}}}
	body := map[string]any{"input": []any{map[string]any{"type": "function_call_output", "call_id": "call-a", "output": "found"}}, "previous_response_id": "resp-a", "store": false, "instructions": "current", "guardrails": []any{"rule"}}
	out, err := responsesRequestBody(body, history)
	if err != nil {
		t.Fatal(err)
	}
	d, err := llm.ParseDialogue("openai-responses", dialogueRequestBody(out))
	if err != nil || len(d.Turns) != 4 || d.Turns[2].Calls[0].ID != "call-a" || d.Turns[3].Role != "tool" {
		t.Fatalf("工具历史恢复失败: %+v %v", d, err)
	}
	next := responseHistory(d, out, llm.DialogueResult{Text: "done"})
	if len(next) != 4 || next[0].Text != "first" || len(history) != 2 || out["guardrails"] == nil || body["previous_response_id"] != "resp-a" {
		t.Fatalf("指令隔离或原始数据修改: %+v", next)
	}
	for _, invalid := range []map[string]any{{"input": "x", "store": "false"}, {"input": "x", "previous_response_id": 12}, {"input": "x", "previous_response_id": ""}, {"input": []any{}}, {"input": "x", "unknown": true}} {
		if _, err := responsesRequestBody(invalid, history); err == nil {
			t.Fatalf("无效增量输入被接受: %v", invalid)
		}
	}
	empty, err := responsesRequestBody(map[string]any{"input": "new", "store": true}, nil)
	if err != nil || empty["store"] != nil {
		t.Fatalf("首次请求失败: %v %v", empty, err)
	}
}

// TestResponsesStreamCommitsBeforeCompletion 验证成功流先提交回复再发终态，截断流不能提交。
// 参数 t 为测试上下文；上游和响应均为内存，验证错误路径，无外部数据需要清理。
func TestResponsesStreamCommitsBeforeCompletion(t *testing.T) {
	good := "data: {\"id\":\"chatcmpl-stream\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"answer\"}}]}\n\ndata: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n"
	for _, tc := range []struct {
		raw     string
		success bool
	}{{good, true}, {strings.Split(good, "data: {\"choices\"")[0], false}} {
		w := httptest.NewRecorder()
		resp := &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(tc.raw))}
		calls := 0
		_, _, _, _, err := pipeDialogue(w, resp, time.Now(), "model", "openai-chat", "openai-responses", func(result llm.DialogueResult) {
			calls++
			if strings.Contains(w.Body.String(), "\"type\":\"response.completed\"") || result.Text != "answer" || result.ID != "chatcmpl-stream" {
				t.Fatalf("提交时序/内容错误: %+v", result)
			}
		})
		if tc.success != (err == nil) || (tc.success && calls != 1) || (!tc.success && calls != 0) {
			t.Fatalf("失败流发布上下文: calls=%d err=%v", calls, err)
		}
	}
}
