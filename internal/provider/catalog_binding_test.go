package provider_test

import (
	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/provider"
	_ "github.com/sunqirui1987/xhub/internal/provider/openai"
	_ "github.com/sunqirui1987/xhub/internal/provider/qiniu"
	_ "github.com/sunqirui1987/xhub/internal/provider/volcengine"
	"testing"
)

// TestCatalogBinding 前置编译时七牛与方舟注册；验证正常、前缀、已注册目录的未知模型、跨目录和错协议，中转目录保留显式配置。
// 参数 t 为单测上下文，返回无；仅内存部署，无外部依赖及清理。
func TestCatalogBinding(t *testing.T) {
	for _, tc := range []struct {
		catalog, model, transport string
		valid                     bool
	}{
		{"qiniu", "bytedance/doubao-seedance-2-0-260128", "qiniu_contents_generation", true},
		{"qiniu", "qiniu/bytedance/doubao-seedance-2-0-260128", "qiniu_contents_generation", true},
		{"qiniu", "bytedance/seedance-2.0/text-to-video", "qiniu_fal_doubao_20", true},
		{"qiniu", "bytedance/seedance-2.0/text-to-video", "qiniu_contents_generation", false},
		{"qiniu", "bytedance/doubao-seedance-2-0-260128", "ark_contents_generation", false},
		{"qiniu", "unknown", "qiniu_contents_generation", false},
		{"qiniu", "gpt-5.6-sol", "bypass_openai_chat", true},
		{"qiniu", "bytedance/seedance-2.0/text-to-video", "bypass_openai_chat", true},
		{"missing", "unknown", "qiniu_contents_generation", true},
		{"", "unknown", "qiniu_contents_generation", true},
	} {
		m := config.ModelEntry{LiteLLMParams: map[string]any{"model": tc.model}, ModelInfo: map[string]any{"transport": tc.transport}}
		if err := provider.ValidateCatalogBinding(tc.catalog, m); (err == nil) != tc.valid {
			t.Fatalf("目录=%s 模型=%s 协议=%s 结果=%v", tc.catalog, tc.model, tc.transport, err)
		}
	}
}

// TestRelayCatalogDeployment 验证未注册中转目录仍受已实现传输、连接类型和固定型号约束。
// 参数 t：单测上下文；前置 FAL 注册；返回无，合法路径通过、错误型号/连接/执行失败；纯内存无清理。
func TestRelayCatalogDeployment(t *testing.T) {
	for _, tc := range []struct {
		model, connection, transport string
		valid                        bool
	}{
		{"byteplus/seedance-2.0/text-to-video", "openai", "qiniu_fal_dreamina_20", true},
		{"unknown/video", "openai", "qiniu_fal_dreamina_20", false},
		{"byteplus/seedance-2.0/text-to-video", "anthropic", "qiniu_fal_dreamina_20", false},
		{"byteplus/seedance-2.0/text-to-video", "openai", "unknown", false},
	} {
		m := config.ModelEntry{ModelName: "relay/video:latest", LiteLLMParams: map[string]any{"model": tc.model, "custom_llm_provider": tc.connection}, ModelInfo: map[string]any{"catalog_id": "fennoai", "transport": tc.transport, "endpoint_types": []string{"fal:queue"}}}
		if err := provider.ValidateDeployment(m); (err == nil) != tc.valid {
			t.Fatalf("中转目录执行约束 %+v 校验=%v", tc, err)
		}
	}
}

// TestCatalogModelsCopy 验证目录副本不允许调用方改变执行权限；前置七牛注册，修改副本后无需清理全局表。
func TestCatalogModelsCopy(t *testing.T) {
	copy := provider.CatalogModels()
	copy["qiniu"]["bytedance/doubao-seedance-2-0-260128"][0] = "forged"
	delete(copy, "volcengine")
	fresh := provider.CatalogModels()
	if fresh["qiniu"]["bytedance/doubao-seedance-2-0-260128"][0] != "qiniu_contents_generation" || fresh["volcengine"] == nil {
		t.Fatal("目录返回值改变了注册表")
	}
}
