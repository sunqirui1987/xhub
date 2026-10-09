package provider_test

import (
	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/provider"
	"testing"
)

// TestDeploymentEndpointProjection 验证模型只公开真实绑定，官方 Ark 模型不暴露其他供应商创建路径。
// 参数 t：测试上下文。返回：无。覆盖显式配置、模型白名单及别名端点并集。
func TestDeploymentEndpointProjection(t *testing.T) {
	m := config.ModelEntry{ModelName: "kling", LiteLLMParams: map[string]any{"custom_llm_provider": "volcengine", "model": "volcengine/doubao-seedance-2-0-260128"}, ModelInfo: map[string]any{"transport": "ark_contents_generation", "endpoint_types": []string{"bypass:ark-video"}}}
	endpoints := provider.DeploymentEndpoints(m)
	if len(endpoints) != 1 || endpoints[0].Path != "/api/v3/contents/generations/tasks" {
		t.Fatalf("unexpected endpoints: %+v", endpoints)
	}
	for _, action := range endpoints[0].Actions {
		if action.UpstreamPath != "" || action.Model != "" {
			t.Fatal("internal routing metadata exposed")
		}
	}
	m.ModelInfo = map[string]any{"transport": "bypass_openai_responses", "endpoint_types": []string{"bypass:openai-responses"}}
	m.LiteLLMParams["model"] = "openai/gpt"
	m.LiteLLMParams["custom_llm_provider"] = "openai"
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

// TestDeploymentEndpointProjectionKeepsSelectedSupplier 验证共享 Ark 入口类型时只公开显式选择的传输路径。
// 参数 t：测试上下文；返回：无。分别覆盖 Ark 与七牛兼容传输，测试只读取本地注册表，无外部调用或清理。
func TestDeploymentEndpointProjectionKeepsSelectedSupplier(t *testing.T) {
	for _, tc := range []struct {
		name, providerID, model, transport, path string
	}{
		{"ark", "volcengine", "volcengine/doubao-seedance-2-0-260128", "ark_contents_generation", "/api/v3/contents/generations/tasks"},
		{"qiniu compatible", "custom", "qiniu/bytedance/doubao-seedance-2-0-260128", "qiniu_contents_generation", "/v3/contents/generations/tasks"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := config.ModelEntry{
				ModelName:     tc.name,
				LiteLLMParams: map[string]any{"custom_llm_provider": tc.providerID, "model": tc.model},
				ModelInfo:     map[string]any{"transport": tc.transport, "endpoint_types": []string{"bypass:ark-video"}},
			}
			got := provider.DeploymentEndpoints(m)
			if len(got) != 1 || got[0].Transport != tc.transport || got[0].Path != tc.path {
				t.Fatalf("显式传输 %s 的公开路径错误：%+v", tc.transport, got)
			}
		})
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
