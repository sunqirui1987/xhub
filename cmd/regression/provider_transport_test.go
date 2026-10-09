package regression

import (
	"net/http"
	"strings"
	"testing"

	"github.com/sunqirui1987/xhub/internal/config"
)

// TestProviderTransportContract 验证兼容传输支持自定义供应商标签，受限传输拒绝错误供应商。
// 参数 t 为测试上下文；返回无；调用后台回归，前置真实网关、私有 schema 和本地上游。
// 验证成功调用、五层记账和失败不落库不外发，夹具自动清理。
func TestProviderTransportContract(t *testing.T) {
	h := newHarness(t, config.ModelEntry{
		ModelName: "regression-custom-provider",
		LiteLLMParams: map[string]any{
			"model": "openai/regression-custom-provider", "api_key": "sk-fake",
			"custom_llm_provider":  "custom-relay",
			"input_cost_per_token": 0.000001, "output_cost_per_token": 0.000002,
		},
		ModelInfo: map[string]any{"transport": "bypass_openai_chat", "endpoint_types": []string{"chat"}},
	})
	admin := h.adminSession()
	owner := h.openScope(t, admin, "provider-transport")
	before := h.moneyOf(t, owner)
	h.ok(http.MethodPost, "/v1/chat/completions", owner.key, map[string]any{
		"model": "regression-custom-provider", "messages": []any{map[string]any{"role": "user", "content": "x"}},
	})
	if got := len(h.upstreamCalls()); got != 1 {
		t.Fatalf("兼容传输应调用一次自定义供应商，实际 %d 次", got)
	}
	if !h.moneyOf(t, owner).grewBy(before, 0.000021) {
		t.Fatal("自定义供应商的真实用量未推进五层金额")
	}

	// Ark 传输声明供应商白名单，因此在管理边界拒绝不匹配标签；通用 OpenAI 传输不受此约束。
	r := h.do(http.MethodPost, "/model/new", admin, map[string]any{
		"model_name": "regression-invalid-ark",
		"litellm_params": map[string]any{"model": "volcengine/doubao-seedance-2-0-260128", "api_base": h.prices.URL,
			"api_key": "sk-fake", "custom_llm_provider": "custom-relay"},
		"model_info": map[string]any{"transport": "ark_contents_generation", "endpoint_types": []string{"bypass:ark-video"}},
	})
	if r.status != http.StatusBadRequest || !strings.Contains(errorMessage(r), "custom-relay") {
		t.Fatalf("受限传输未明确拒绝不匹配供应商：%s", r.describe())
	}
	if strings.Contains(errorMessage(r), "API key") {
		t.Fatalf("受限传输错误误报为缺少凭据：%s", r.describe())
	}
	if contains(modelIDs(rowsOf(h.ok(http.MethodGet, "/v1/models", owner.key, nil), "data")), "regression-invalid-ark") {
		t.Fatal("非法供应商部署被保存到模型列表")
	}
	if got := len(h.upstreamCalls()); got != 1 {
		t.Fatalf("非法供应商部署仍外发请求，总次数 %d", got)
	}
}
