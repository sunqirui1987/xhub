package provider_test

import (
	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/provider"
	"testing"
)

// TestExplicitEndpointContract 验证部署必须显式声明上游传输，并拒绝不匹配的供应商或固定模型。
// 参数 t：Go 测试上下文。返回：无；断言失败时报告协议或计费契约回归。
// 调用：go test；使用本地注册表、价格目录或假上游，不创建真实付费任务。
func TestExplicitEndpointContract(t *testing.T) {
	for _, tc := range []struct {
		transport, endpoint, slug, model string
		valid                            bool
	}{
		{"", "chat", "openai", "gpt", false},
		{"qiniu_fal_kling", "video", "qiniu", "fal-ai/kling-video/v2.5-turbo/pro/text-to-video", false},
		{"qiniu_fal_kling", "fal:queue", "custom", "qiniu/fal-ai/kling-video/v2.5-turbo/pro/text-to-video", true},
		{"qiniu_fal_kling", "fal:queue", "openai", "qiniu/fal-ai/kling-video/v2.5-turbo/pro/text-to-video", true},
		{"qiniu_fal_kling", "fal:queue", "qiniu", "fal-ai/vidu/q1/text-to-video", false},
		{"bypass_openai_responses", "bypass:openai-responses", "qiniu", "gpt", true},
	} {
		m := config.ModelEntry{ModelName: "alias", ModelInfo: map[string]any{"transport": tc.transport, "endpoint_types": []string{tc.endpoint}}, LiteLLMParams: map[string]any{"custom_llm_provider": tc.slug, "model": tc.model}}
		if err := provider.ValidateDeployment(m); (err == nil) != tc.valid {
			t.Errorf("%+v: %v", tc, err)
		}
	}
}
