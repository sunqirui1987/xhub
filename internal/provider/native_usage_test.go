package provider

import "testing"

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
