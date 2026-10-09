package regression

import (
	"net/http"
	"reflect"
	"testing"
)

// templateBody 返回模板详情中的正文对象；缺少正文时立即终止测试。
func templateBody(t *testing.T, h *harness, admin, id string) map[string]any {
	t.Helper()
	row := h.ok(http.MethodGet, "/route_template/"+id, admin, nil).json()
	body, _ := row["body"].(map[string]any)
	if body == nil {
		t.Fatalf("模板详情缺少 body: %v", row)
	}
	return body
}

// TestRouteTemplateConfigurationRoundTrip 验证完整新 JSON 原样保存和整体替换。
// 模板正文保留已声明的模型规则、可靠性、组及回退，不隐式补字段。
func TestRouteTemplateConfigurationRoundTrip(t *testing.T) {
	h := newHarness(t)
	admin := h.adminSession()
	first := routeTemplateBody([]any{
		map[string]any{"model": "gpt-6-sol", "strategy": "least-busy"},
		map[string]any{"model": "gpt-5.5", "strategy": "traffic-split"},
	}, 2, 45, 4, 20)
	id := routeTemplate(t, h, admin, "strict document", first)
	if got := templateBody(t, h, admin, id); !reflect.DeepEqual(got, first) {
		t.Fatalf("模板保存后发生隐式补全或改写: got=%v want=%v", got, first)
	}

	second := routeTemplateBody([]any{map[string]any{"model": "gpt-6-sol", "strategy": "random"}}, 1, 30, 0, 0)
	h.ok(http.MethodPost, "/route_template/"+id+"/update", admin, map[string]any{"body": second})
	if got := templateBody(t, h, admin, id); !reflect.DeepEqual(got, second) {
		t.Fatalf("模板更新不是整份替换: got=%v want=%v", got, second)
	}
}

// TestRouteTemplateModelRulesContainNoDeploymentData 验证模板拒绝入口和不属于模型的部署引用。
// 入口不能写入规则；独立权重必须引用该公开模型的真实部署，否则返回 400。
func TestRouteTemplateModelRulesContainNoDeploymentData(t *testing.T) {
	h := newHarness(t)
	admin := h.adminSession()
	for _, extra := range []map[string]any{
		{"endpoint_id": "chat"},
		{"allocations": []any{map[string]any{"deployment_id": "a", "weight": 1}}},
		{"deployment_id": "a"},
	} {
		rule := map[string]any{"model": "shared", "strategy": "traffic-split"}
		for key, value := range extra {
			rule[key] = value
		}
		body := routeTemplateBody([]any{rule}, 1, 60, 3, 0)
		if r := h.do(http.MethodPost, "/route_template/new", admin, map[string]any{"name": "invalid", "body": body}); r.status != http.StatusBadRequest {
			t.Fatalf("模板接受了部署级字段 %v: %s", extra, r.describe())
		}
	}
}

// TestRouteTemplateDuplicateModelsAreRejected 验证一个模板内每个公开模型最多一条规则。
func TestRouteTemplateDuplicateModelsAreRejected(t *testing.T) {
	h := newHarness(t)
	admin := h.adminSession()
	body := routeTemplateBody([]any{
		map[string]any{"model": "shared", "strategy": "random"},
		map[string]any{"model": "shared", "strategy": "least-busy"},
	}, 1, 60, 3, 0)
	if r := h.do(http.MethodPost, "/route_template/new", admin, map[string]any{"name": "duplicate", "body": body}); r.status != http.StatusBadRequest {
		t.Fatalf("重复公开模型未拒绝: %s", r.describe())
	}
}

// TestRouteTemplateDefaultStrategyContract 验证真实接口保存、回读、整体更新和非法默认错误响应。
// 前置隔离 schema 和管理员；创建的模板由 harness 结束时清理，不依赖供应商凭据。
func TestRouteTemplateDefaultStrategyContract(t *testing.T) {
	h := newHarness(t)
	admin := h.adminSession()
	body := routeTemplateBody([]any{map[string]any{"model": "special", "strategy": "least-busy"}}, 2, 60, 3, 60)
	body["routing_strategy"] = "simple-shuffle"
	id := routeTemplate(t, h, admin, "default strategy", body)
	if got := templateBody(t, h, admin, id); !reflect.DeepEqual(got, body) {
		t.Fatalf("模板默认保存回读改变: got=%v want=%v", got, body)
	}
	for _, invalid := range []any{"", "unknown", nil, 1} {
		body["routing_strategy"] = invalid
		r := h.do(http.MethodPost, "/route_template/"+id+"/update", admin, map[string]any{"body": body})
		if r.status != http.StatusBadRequest {
			t.Fatalf("非法默认策略未返回 400: %s", r.describe())
		}
	}
	body["routing_strategy"] = "cost-based-routing"
	h.ok(http.MethodPost, "/route_template/"+id+"/update", admin, map[string]any{"body": body})
	if got := templateBody(t, h, admin, id); !reflect.DeepEqual(got, body) {
		t.Fatalf("默认策略更新回读不一致: %v", got)
	}
	delete(body, "routing_strategy")
	h.ok(http.MethodPost, "/route_template/"+id+"/update", admin, map[string]any{"body": body})
	if got := templateBody(t, h, admin, id); !reflect.DeepEqual(got, body) {
		t.Fatalf("未恢复旧格式继承: %v", got)
	}
}
