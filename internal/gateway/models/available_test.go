package models

import (
	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/provider"
	"testing"
)

// TestAvailableCardEndpoints 验证有效声明公开精确路径，缺失或旧声明返回原因且没有猜测绑定。
// 参数 t 为测试上下文；前置内存部署与登记目录，返回无；不写数据，无需清理。
func TestAvailableCardEndpoints(t *testing.T) {
	for _, tc := range []struct {
		name, transport, endpoint string
		valid                     bool
	}{
		{"chat", "bypass_openai_chat", "chat", true},
		{"gemini", "gemini_generate_content", "gemini", true},
		{"missing", "", "", false},
		{"old-image", "openai_image_generation", "image", false},
		{"mismatch", "openai_image_generation", "image_edit", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			entry := config.ModelEntry{ModelName: tc.name, LiteLLMParams: map[string]any{"model": "real", "custom_llm_provider": "custom"}, ModelInfo: map[string]any{"transport": tc.transport, "endpoint_types": []string{tc.endpoint}}}
			card := availableCard(entry)
			bindings := card["endpoints"].([]provider.EndpointBinding)
			if (len(bindings) > 0) != tc.valid {
				t.Fatalf("模型端点状态错误: %+v", card)
			}
			if (card["unavailable_reason"] == nil) != tc.valid {
				t.Fatalf("配置错误原因错误: %+v", card)
			}
			foundGoogle := false
			for _, binding := range bindings {
				if binding.Path == "/v1beta/models/gemini:generateContent" {
					foundGoogle = true
				}
			}
			if tc.name == "gemini" && !foundGoogle {
				t.Fatalf("Google 别名路径错误: %+v", bindings)
			}
		})
	}
}
