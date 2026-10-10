package regression

import (
	"net/http"
	"strings"
	"testing"
)

// TestPublicModelsDiscovery 验证匿名根目录、显式无效凭据、版本兼容路径及推理鉴权；参数为测试对象，无返回。
// 前置隔离网关和部署，公开响应只有模型元信息，错误仍 401/400，测试底座结束清理隔离数据库。
func TestPublicModelsDiscovery(t *testing.T) {
	h := newHarness(t, chatDeployment("public-discovery"))
	r := h.ok(http.MethodGet, "/models", "", nil)
	if !contains(modelIDs(rowsOf(r, "data")), "public-discovery") {
		t.Fatalf("匿名目录缺少已启用模型: %s", r.body)
	}
	for _, row := range rowsOf(r, "data") {
		if len(row) != 4 {
			t.Fatalf("匿名目录出现管理字段: %v", row)
		}
	}
	for _, path := range []string{"/models", "/v1/models"} {
		if got := h.do(http.MethodGet, path, "invalid-public-key", nil); got.status != 401 {
			t.Fatalf("错误凭据被放行: %s", path)
		}
	}
	// 通用密钥头必须进入鉴权链，不能因未使用 Authorization 而降级为匿名发现。
	for _, header := range []string{"api-key", "x-litellm-api-key", "x-api-key"} {
		if got := h.doHeaders(http.MethodGet, "/models", "", nil, map[string]string{header: "invalid-public-key"}); got.status != 401 {
			t.Fatalf("错误密钥头被放行: %s, 状态 %d", header, got.status)
		}
	}
	if got := h.do(http.MethodGet, "/models?scope=invalid", "", nil); got.status != 400 {
		t.Fatalf("scope 错误未报告: %v", got)
	}
	if got := h.do(http.MethodPost, "/chat/completions", "", map[string]any{"model": "public-discovery", "messages": []any{map[string]any{"role": "user", "content": "Hello"}}}); got.status != 401 {
		t.Fatalf("匿名推理未拒绝: %v", got)
	}
	if strings.Contains(string(r.body), "api_key") {
		t.Fatal("匿名目录泄漏凭据")
	}
}
