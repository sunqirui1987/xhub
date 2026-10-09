package regression

import (
	"net/http"
	"strings"
	"testing"
)

// TestRouterSettingsChain 验证当前路由配置的权限、模板即时生效、重试和回退完整链路。
// 前置为两个同名部署和一个回退模型；模型默认接口控制平台分配，模板使用 model_routes 与 retry_policy。
// 隔离 schema 在测试结束时清理模型默认、模板、绑定和用量数据。
func TestRouterSettingsChain(t *testing.T) { runSimulated(t, routerSettingsSimulated) }

// routerSettingsSimulated 执行路由设置链路；参数 t 为测试上下文，无返回值。
// 成功路径核对实际上游及五层计费，失败路径核对成员不能修改模型默认且重试失败不重复计费。
func routerSettingsSimulated(t *testing.T) {
	const public = "routed-settings"
	h := newHarness(t,
		deployment(public, "openai/heavy", map[string]any{"input_cost_per_token": 0.01}),
		deployment(public, "openai/light", map[string]any{"input_cost_per_token": 0.0000001}),
		deployment("other-model", "openai/other-model", nil),
	)
	admin := h.adminSession()
	c := h.openScope(t, admin, "settings")
	h.setModelDefault(t, admin, public, map[string]float64{"heavy": 1, "light": 0})

	refused := h.do(http.MethodPut, "/model/default", c.session, map[string]any{
		"model_name": public,
		"weights":    map[string]any{"allocations": routeAllocations(map[string]float64{"heavy": 0, "light": 1})},
	})
	if refused.status != http.StatusForbidden {
		t.Fatalf("成员修改了模型默认分配: %s", refused.describe())
	}
	h.assertBilled(t, c, admin, public, "平台默认仍选 heavy", []string{"heavy"})

	body := routeTemplateBody([]any{map[string]any{"model": public, "strategy": "cost-based-routing"}}, 1, 60, 0, 0)
	templateID := routeTemplate(t, h, admin, "settings route", body)
	bindTemplate(t, h, admin, "key", c.keyID, templateID)
	h.assertBilled(t, c, admin, public, "成本策略立即生效", []string{"light"})

	h.scriptStatus("light", http.StatusInternalServerError)
	body = routeTemplateBody([]any{map[string]any{"model": public, "strategy": "cost-based-routing"}}, 2, 60, 0, 0)
	h.ok(http.MethodPost, "/route_template/"+templateID+"/update", admin, map[string]any{"body": body})
	before := h.moneyOf(t, c)
	mark := len(h.upstreamCalls())
	retried := h.ok(http.MethodPost, "/v1/chat/completions", c.key, chatRequest(public, "retry then succeed"))
	if got := h.upstreamSince(mark); strings.Join(got, ",") != "light,light,heavy" {
		t.Fatalf("重试顺序=%v，期望 light,light,heavy", got)
	}
	cost := parseFloatOrZero(retried.header("x-litellm-response-cost"))
	if after := h.moneyOf(t, c); !after.grewBy(before, cost) {
		t.Fatalf("重试重复计费: before=%+v after=%+v cost=%v", before, after, cost)
	}

	body["fallbacks"] = []any{map[string]any{public: []any{"other-model"}}}
	h.ok(http.MethodPost, "/route_template/"+templateID+"/update", admin, map[string]any{"body": body})
	h.scriptStatus("heavy", http.StatusInternalServerError)
	mark = len(h.upstreamCalls())
	h.ok(http.MethodPost, "/v1/chat/completions", c.key, chatRequest(public, "fallback model"))
	if got := h.upstreamSince(mark); strings.Join(got, ",") != "light,light,heavy,heavy,other-model" {
		t.Fatalf("模型回退顺序=%v", got)
	}
}
