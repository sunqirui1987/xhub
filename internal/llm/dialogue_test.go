package llm

import (
	"encoding/json"
	"reflect"
	"testing"
)

// TestDialogueProtocolMatrix 验证三个入口到三种上游的文本、工具定义、调用和结果往返。
// 前置：纯内存共享对话；结果：关联 ID、参数和完整模型标识保持；无外部数据需要清理。
func TestDialogueProtocolMatrix(t *testing.T) {
	protocols := []string{"openai-chat", "openai-responses", "anthropic-messages"}
	original := Dialogue{Turns: []Turn{{Role: "system", Text: "规则"}, {Role: "user", Text: "天气"}, {Role: "assistant", Calls: []Turn{{ID: "call_1", Name: "weather", Arguments: "{\"city\":\"上海\"}"}}}, {Role: "tool", ID: "call_1", Text: "晴"}}, Tools: []map[string]any{{"name": "weather", "parameters": map[string]any{"type": "object"}}}, Options: map[string]any{"max_tokens": float64(20)}}
	for _, caller := range protocols {
		t.Run(caller, func(t *testing.T) {
			body, err := EncodeDialogue(original, caller, "public")
			if err != nil {
				t.Fatal(err)
			}
			delete(body, "store")
			raw, _ := json.Marshal(body)
			_ = json.Unmarshal(raw, &body)
			parsed, err := ParseDialogue(caller, body)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(parsed.Turns, original.Turns) {
				t.Fatalf("工具历史改变: %#v", parsed.Turns)
			}
			for _, upstream := range protocols {
				t.Run(upstream, func(t *testing.T) {
					encoded, err := EncodeDialogue(parsed, upstream, "namespace/vendor/model")
					if err != nil {
						t.Fatal(err)
					}
					if encoded["model"] != "namespace/vendor/model" {
						t.Fatal("上游型号被修改")
					}
					delete(encoded, "store")
					raw, _ := json.Marshal(encoded)
					_ = json.Unmarshal(raw, &encoded)
					roundtrip, err := ParseDialogue(upstream, encoded)
					if err != nil {
						t.Fatal(err)
					}
					if !reflect.DeepEqual(roundtrip.Turns, original.Turns) {
						t.Fatalf("跨协议历史改变: %#v", roundtrip.Turns)
					}
				})
			}
		})
	}
}

// TestDialogueRejectsUnsupported 验证未知能力、错误类型及无效工具在调用上游前失败。
// 前置：有效 Chat 请求分别注入非法字段；结果：明确错误；纯内存测试无清理副作用。
func TestDialogueRejectsUnsupported(t *testing.T) {
	cases := []map[string]any{
		{"previous_response_id": "r"}, {"input": "wrong protocol"}, {"temperature": -1.0}, {"max_tokens": 1.5}, {"stream": "true"}, {"tools": "invalid"},
		{"messages": []any{map[string]any{"role": "user", "content": "x", "audio": map[string]any{}}}},
		{"messages": []any{map[string]any{"role": "user", "content": []any{map[string]any{"type": "image_url", "image_url": map[string]any{"url": "x"}}}}}},
		{"messages": []any{map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{"id": "x", "type": "function", "function": map[string]any{"name": "f", "arguments": "broken"}}}}}},
	}
	for i, override := range cases {
		body := map[string]any{"messages": []any{map[string]any{"role": "user", "content": "hello"}}}
		for key, value := range override {
			body[key] = value
		}
		if _, err := ParseDialogue("openai-chat", body); err == nil {
			t.Fatalf("案例 %d 应拒绝: %#v", i, body)
		}
	}
}

// TestDialogueResultMatrix 验证回复在三种协议间保持工具、结束原因与实测用量。
// 前置：含工具回复及缓存 token；结果：九种组合等价，缺失用量禁止计价；无需外部清理。
func TestDialogueResultMatrix(t *testing.T) {
	protocols := []string{"openai-chat", "openai-responses", "anthropic-messages"}
	original := DialogueResult{ID: "r_1", Text: "正在查询", Stop: "tool_calls", Calls: []Turn{{ID: "call_1", Name: "weather", Arguments: "{}"}}, Usage: map[string]any{"prompt_tokens": float64(10), "completion_tokens": float64(3), "prompt_tokens_details": map[string]any{"cached_tokens": float64(2)}}}
	for _, source := range protocols {
		for _, target := range protocols {
			t.Run(source+"/"+target, func(t *testing.T) {
				raw, err := EncodeDialogueResult(original, source, "m")
				if err != nil {
					t.Fatal(err)
				}
				result, err := ParseDialogueResult(source, raw)
				if err != nil {
					t.Fatal(err)
				}
				raw, err = EncodeDialogueResult(result, target, "m")
				if err != nil {
					t.Fatal(err)
				}
				result, err = ParseDialogueResult(target, raw)
				if err != nil {
					t.Fatal(err)
				}
				if result.Text != original.Text || !reflect.DeepEqual(result.Calls, original.Calls) || result.Stop != "tool_calls" || result.Usage["total_tokens"] != float64(13) {
					t.Fatalf("回复事实改变: %#v", result)
				}
			})
		}
	}
	if NormalizeDialogueUsage(nil)["pricing_blocked"] != "upstream_usage_missing" {
		t.Fatal("缺少用量不得推算计费")
	}
	for _, raw := range []string{`{"error":{"message":"failure"}}`, `{"status":"in_progress","output":[]}`} {
		if _, err := ParseDialogueResult("openai-responses", []byte(raw)); err == nil {
			t.Fatal("未完成回复应拒绝")
		}
	}
}
