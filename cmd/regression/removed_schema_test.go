package regression

import (
	"net/http"
	"testing"
)

// TestRemovedPublicSchemaRoute 验证真实网关不再提供公共定义入口，数据面仍要求认证。
// 参数 t 为回归上下文，无返回值；前置隔离 PostgreSQL、本地上游和临时模型。
// 覆盖匿名与认证访问、错误方法、JSON 错误契约及上游零调用；harness 自动关闭服务并删除私有 schema。
func TestRemovedPublicSchemaRoute(t *testing.T) {
	h := newHarness(t, chatDeployment("regression-removed-schema"))
	h.resetUpstream()
	for _, method := range []string{http.MethodGet, http.MethodHead, http.MethodPost} {
		for _, token := range []string{"", h.master} {
			r := h.do(method, "/openapi.json", token, nil)
			if r.status != http.StatusNotFound {
				t.Fatalf("%s 公共接口定义应不存在: %s", method, r.describe())
			}
			if method != http.MethodHead && r.json()["error"] == nil {
				t.Fatalf("已移除入口应保持 JSON 错误响应: %s", r.describe())
			}
			if method == http.MethodHead && len(r.body) != 0 {
				t.Fatal("HEAD 错误响应不得包含正文")
			}
		}
	}
	denied := h.do(http.MethodPost, "/v1/chat/completions", "", map[string]any{
		"model":    "regression-removed-schema",
		"messages": []any{map[string]any{"role": "user", "content": "hi"}},
	})
	if denied.status != http.StatusUnauthorized {
		t.Fatalf("移除接口定义不得改变数据面鉴权: %s", denied.describe())
	}
	if len(h.upstreamCalls()) != 0 {
		t.Fatal("已移除入口和拒绝匿名调用不应触达上游")
	}
}
