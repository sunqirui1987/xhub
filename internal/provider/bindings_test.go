package provider_test

import (
	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/provider"
	"testing"
)

// TestDeploymentEndpointProjection 验证模型只公开真实绑定，固定 Fal 模型不暴露同组其他创建路径。
// 参数 t：测试上下文。返回：无。覆盖显式配置、模型白名单及别名端点并集。
func TestDeploymentEndpointProjection(t *testing.T) {
	m := config.ModelEntry{ModelName: "kling", LiteLLMParams: map[string]any{"custom_llm_provider": "qiniu", "model": "qiniu/fal-ai/kling-video/v2.5-turbo/pro/text-to-video"}, ModelInfo: map[string]any{"transport": "qiniu_fal_kling", "endpoint_types": []string{"bypass:fal-video"}}}
	endpoints := provider.DeploymentEndpoints(m)
	if len(endpoints) != 1 || endpoints[0].Path != "/queue/fal-ai/kling-video/v2.5-turbo/pro/text-to-video" {
		t.Fatalf("unexpected endpoints: %+v", endpoints)
	}
	for _, action := range endpoints[0].Actions {
		if action.UpstreamPath != "" || action.Model != "" {
			t.Fatal("internal routing metadata exposed")
		}
	}
	m.ModelInfo = map[string]any{"transport": "bypass_openai_responses", "endpoint_types": []string{"bypass:openai-responses"}}
	m.LiteLLMParams["model"] = "openai/gpt"
	merged := provider.MergeEndpoints(endpoints, provider.DeploymentEndpoints(m))
	merged = provider.MergeEndpoints(merged, endpoints)
	if len(merged) != 2 {
		t.Fatalf("alias endpoint union: %d", len(merged))
	}
	delete(m.ModelInfo, "transport")
	if len(provider.DeploymentEndpoints(m)) != 0 {
		t.Fatal("missing transport must not infer endpoints")
	}
}

// TestNativeTerminalUsageBlocking 验证流式图片终态缺少规格不能被中间事件的空用量过滤逻辑吞掉。
func TestNativeTerminalUsageBlocking(t *testing.T) {
	request := map[string]any{"quality": "auto", "size": "auto"}
	partial := provider.NativeStreamUsage("openai-images", []byte("data: {\"type\":\"image_generation.partial_image\"}\n\n"), request)
	if len(partial) != 0 {
		t.Fatal(partial)
	}
	final := provider.NativeStreamUsage("openai-images", []byte("data: {\"type\":\"image_generation.completed\",\"usage\":{\"input_tokens\":10}}\n\n"), request)
	if final["pricing_blocked"] != "image_variant_unknown" {
		t.Fatal(final)
	}
}
