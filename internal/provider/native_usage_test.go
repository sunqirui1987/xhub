package provider

import "testing"

// TestNativeImageTokenFacts 验证图片协议保留输入模态分桶，并把输出 token 标记为图片。
// 参数 t：测试上下文。前置条件为本地构造的 OpenAI Images 成功响应；返回无。
// 验证结果：文本/图片输入明细原样保留、输出变体为 image；无外部资源，无需清理。
func TestNativeImageTokenFacts(t *testing.T) {
	details := map[string]any{"text_tokens": float64(8), "image_tokens": float64(3)}
	facts := NativeUsage("openai-images")(map[string]any{
		"data": []any{map[string]any{"b64_json": "AAAA"}},
		"usage": map[string]any{
			"input_tokens": float64(11), "output_tokens": float64(7024),
			"input_tokens_details": details,
		},
	}, map[string]any{"size": "1024x1024"})
	got, _ := facts["input_tokens_details"].(map[string]any)
	if facts["output_variant"] != "image" || facts["images"] != 1 ||
		got["text_tokens"] != float64(8) || got["image_tokens"] != float64(3) {
		t.Fatalf("图片 token 事实提取错误：%+v", facts)
	}
}

// TestNativeTextUsageHasNoImageVariant 验证非图片协议不会被补上图片输出变体。
// 参数 t：测试上下文。前置条件为本地构造的聊天 usage；返回无。
// 验证结果：聊天 token 保持原样且不存在 output_variant；无外部资源，无需清理。
func TestNativeTextUsageHasNoImageVariant(t *testing.T) {
	facts := NativeUsage("openai-chat")(map[string]any{
		"usage": map[string]any{"prompt_tokens": float64(2), "completion_tokens": float64(3)},
	}, nil)
	if facts["prompt_tokens"] != float64(2) || facts["completion_tokens"] != float64(3) {
		t.Fatalf("聊天 token 被改写：%+v", facts)
	}
	if _, exists := facts["output_variant"]; exists {
		t.Fatalf("聊天用量被错误标记为图片输出：%+v", facts)
	}
}

// TestNativeImageEventFacts 验证图片完成事实可由 SSE 事件头提供，部分图片不产生账单量。
// 参数 t：测试上下文。返回：无。调用：go test；全部使用本地协议载荷。
func TestNativeImageEventFacts(t *testing.T) {
	request := map[string]any{"quality": "high", "size": "1024x1024"}
	partial := []byte("event: image_generation.partial_image\ndata: {\"b64_json\":\"partial\"}\n\n")
	if facts := NativeStreamUsage("openai-images", partial, request); facts != nil {
		t.Fatal("partial image incorrectly produced usage", facts)
	}
	complete := []byte("event: image_generation.completed\ndata: {\"usage\":{\"output_tokens\":5}}\n\n")
	facts := NativeStreamUsage("openai-images", complete, request)
	if facts["images"] != 1 || facts["image_variant"] != "high_1024x1024" || facts["output_tokens"] != float64(5) {
		t.Fatal("completed image facts missing", facts)
	}
	var state NativeStreamState
	state.Observe("openai-images", partial)
	if state.Completed || state.Images != 0 {
		t.Fatal("partial image marked completed")
	}
	state.Observe("openai-images", complete)
	state.Observe("openai-images", complete)
	if !state.Completed || state.Images != 2 {
		t.Fatal("completed image count incorrect", state)
	}
}

// TestNativeCompletionUsesProtocol 验证通用结束标记和其他协议事件不能触发成功结算。
// 参数 t：测试上下文。返回：无。调用：go test；无网络调用。
func TestNativeCompletionUsesProtocol(t *testing.T) {
	var state NativeStreamState
	state.Observe("openai-responses", []byte("data: [DONE]\n\n"))
	state.Observe("openai-responses", []byte("event: message_stop\ndata: {}\n\n"))
	if state.Completed {
		t.Fatal("wrong protocol completed native response")
	}
	state.Observe("openai-responses", []byte("event: response.completed\ndata: {}\n\n"))
	state.Observe("openai-responses", []byte("event: response.failed\ndata: {}\n\n"))
	if !state.Completed || !state.Failed {
		t.Fatal("failure did not remain sticky", state)
	}
}
