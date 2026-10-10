package regression

import (
	"strings"
	"testing"

	"github.com/sunqirui1987/xhub/internal/config"
)

// TestRouteDiagnosticContract 验证真实路由对缺失、暂停、声明错误与残留零权重给出不同响应。
// 前置隔离 PostgreSQL、本地上游和真实密钥；拒绝不出站不扣费，修复后有响应和账单；harness 删除全部 schema 数据。
func TestRouteDiagnosticContract(t *testing.T) {
	invalid := config.ModelEntry{ModelName: "invalid-config", LiteLLMParams: map[string]any{"model": "old"}, ModelInfo: map[string]any{"id": "invalid"}}
	paused := config.ModelEntry{ModelName: "paused", LiteLLMParams: map[string]any{"model": "paused"}, ModelInfo: map[string]any{"id": "paused", "disabled": true, "transport": "bypass_openai_chat"}}
	h := newHarness(t, invalid, paused)
	admin := h.adminSession()
	owner := h.openScope(t, admin, "route-diagnosis")
	created := h.ok("POST", "/model/new", admin, map[string]any{
		"model_name":     "gpt-5.6-sol",
		"litellm_params": map[string]any{"model": "gpt-5.6-sol", "api_base": h.prices.URL + "/v1", "api_key": "sk-fake", "custom_llm_provider": "custom", "input_cost_per_token": 0.000001, "output_cost_per_token": 0.000002},
		"model_info":     map[string]any{"transport": "bypass_openai_chat", "pricing_source": "manual"},
	}).json()
	id := stringField(created["model_info"].(map[string]any), "id")
	// 直接植入历史分配，重现删除部署后仍残留正权重 ID 的数据；当前写接口会拒绝悬空引用。
	if err := h.store.PutConfig("model_defaults", "gpt-5.6-sol", map[string]any{"allocations": []any{map[string]any{"deployment_id": id, "weight": 0}, map[string]any{"deployment_id": "deleted", "weight": 100}}}); err != nil {
		t.Fatal(err)
	}
	before := h.moneyOf(t, owner)
	for _, tc := range []struct {
		model, kind, message string
		status               int
	}{
		{"not-in-directory", "invalid_request", "model not found", 400},
		{"paused", "model_disabled", "model is disabled", 400},
		{"invalid-config", "model_unavailable", "exists", 400},
		{"gpt-5.6-sol", "model_unavailable", "no allocated traffic", 503},
	} {
		for _, path := range []string{"/chat/completions", "/v1/chat/completions"} {
			r := h.do("POST", path, owner.key, map[string]any{"model": tc.model, "messages": []any{map[string]any{"role": "user", "content": "你好"}}})
			if r.status != tc.status {
				t.Fatalf("%s %s 状态错误: %s", path, tc.model, r.describe())
			}
			err := r.json()["error"].(map[string]any)
			if stringField(err, "type") != tc.kind || !strings.Contains(stringField(err, "message"), tc.message) {
				t.Fatalf("%s 分类错误: %s", tc.model, r.describe())
			}
			if r.headers.Get("x-litellm-call-id") == "" {
				t.Fatal("拒绝请求缺失关联日志 ID")
			}
		}
	}
	if len(h.upstreamCalls()) != 0 || !h.moneyOf(t, owner).grewBy(before, 0) {
		t.Fatal("筛选拒绝调用了上游或产生费用")
	}
	h.ok("PUT", "/model/default", admin, map[string]any{"model_name": "gpt-5.6-sol", "weights": map[string]any{"allocations": []any{map[string]any{"deployment_id": id, "weight": 1}}}})
	r := h.ok("POST", "/v1/chat/completions", owner.key, map[string]any{"model": "gpt-5.6-sol", "messages": []any{map[string]any{"role": "user", "content": "修复后实调"}}})
	if !strings.Contains(string(r.body), "regression-ok") || len(h.upstreamCalls()) != 1 {
		t.Fatalf("修复权重未恢复真实上游: %s", r.describe())
	}
	if !h.moneyOf(t, owner).grewBy(before, 0.000021) {
		t.Fatal("修复后响应未按11输入5输出记录费用")
	}
	h.ok("POST", "/model/delete", admin, map[string]any{"id": id})
}
