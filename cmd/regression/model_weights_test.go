package regression

import (
	"fmt"
	"net/http"
	"testing"
)

// TestModelWeightsLifecycle 验证同名部署默认权重、模板独立覆盖及省略权重时实时继承。
// 前置为隔离数据库、真实网关和本地上游；验证接口错误、实际型号与账单，harness 删除整个 schema。
func TestModelWeightsLifecycle(t *testing.T) {
	h := newHarness(t)
	admin := h.adminSession()
	owner := h.openScope(t, admin, "relative-weights")
	ids := []string{}
	for i, upstream := range []string{"custom/alpha/model", "custom/beta/model"} {
		created := h.ok("POST", "/model/new", admin, map[string]any{"model_name": "shared", "litellm_params": map[string]any{"model": upstream, "api_base": h.prices.URL + "/v1", "api_key": "sk-local", "custom_llm_provider": "custom", "input_cost_per_token": 0.000001, "output_cost_per_token": 0.000002}, "model_info": map[string]any{"transport": "bypass_openai_chat", "endpoint_types": []string{"chat"}, "pricing_source": "manual"}}).json()
		id := stringField(created["model_info"].(map[string]any), "id")
		if id == "" {
			t.Fatalf("部署 %d 未返回 ID", i)
		}
		ids = append(ids, id)
	}
	defer func() {
		for _, id := range ids {
			h.ok("POST", "/model/delete", admin, map[string]any{"id": id})
		}
	}()
	// weights 根据两个部署的相对权重生成接口正文；返回新对象，默认与模板校验共用，无副作用。
	weights := func(a, b float64) map[string]any {
		return map[string]any{"allocations": []any{map[string]any{"deployment_id": ids[0], "weight": a}, map[string]any{"deployment_id": ids[1], "weight": b}}}
	}
	h.ok("PUT", "/model/default", admin, map[string]any{"model_name": "shared", "weights": weights(3, 7)})
	groups := listField(h.ok("GET", "/model/groups?search=alpha", admin, nil).json(), "data")
	if len(groups) != 1 || len(groups[0]["deployments"].([]any)) != 2 {
		t.Fatalf("搜索拆散部署: %v", groups)
	}
	raw := groups[0]["default_weights"].(map[string]any)["allocations"].([]any)
	if raw[0].(map[string]any)["weight"] != 3.0 || raw[1].(map[string]any)["weight"] != 7.0 {
		t.Fatalf("相对权重回读错误: %v", raw)
	}
	for _, bad := range []map[string]any{weights(0, 0), weights(-1, 1), {"allocations": []any{map[string]any{"deployment_id": "foreign", "weight": 1}}}} {
		r := h.do("PUT", "/model/default", admin, map[string]any{"model_name": "shared", "weights": bad})
		if r.status != 400 {
			t.Fatalf("无效权重未拒绝: %s", r.describe())
		}
	}
	if r := h.do("PUT", "/model/default", owner.session, map[string]any{"model_name": "shared", "weights": weights(1, 1)}); r.status != 403 {
		t.Fatalf("客户修改平台默认: %s", r.describe())
	}
	h.ok("PUT", "/model/default", admin, map[string]any{"model_name": "shared", "weights": weights(1, 0)})
	callSequence := 0
	// checkCall 用不同提示词避免缓存，核对真实上游及账单；仅清空测试服务记录。
	checkCall := func(want string) {
		callSequence++
		t.Helper()
		before := h.moneyOf(t, owner)
		h.resetUpstream()
		h.ok("POST", "/v1/chat/completions", owner.key, chatRequest("shared", fmt.Sprintf("%s-%d", want, callSequence)))
		calls := h.upstreamCalls()
		if len(calls) != 1 || calls[0].Body["model"] != want {
			t.Fatalf("实际部署错误: %+v", calls)
		}
		h.flushSpend()
		after := h.moneyOf(t, owner)
		if after.key <= before.key {
			t.Fatal("实际部署调用未计费")
		}
	}
	checkCall("custom/alpha/model")
	doc := routeTemplateBody([]any{map[string]any{"model": "shared", "strategy": "traffic-split"}}, 1, 60, 3, 0)
	template := h.ok("POST", "/route_template/new", admin, map[string]any{"name": "customer-weights", "body": doc}).json()
	templateID := stringField(template, "id")
	h.ok("POST", "/key/update", admin, map[string]any{"key": owner.key, "route_template_id": templateID})
	defer func() {
		h.ok("POST", "/key/update", admin, map[string]any{"key": owner.key, "route_template_id": nil})
		h.ok("POST", "/route_template/"+templateID+"/delete", admin, map[string]any{})
	}()
	checkCall("custom/alpha/model")
	// 省略 allocations 的模板规则实时继承模型管理权重。
	h.ok("PUT", "/model/default", admin, map[string]any{"model_name": "shared", "weights": weights(0, 2)})
	checkCall("custom/beta/model")
	// 独立模板权重与默认相反，实际请求必须命中 alpha，模型默认仍保留 beta。
	customRule := map[string]any{"model": "shared", "strategy": "traffic-split", "allocations": weights(1, 0)["allocations"]}
	doc["model_routes"] = []any{customRule}
	h.ok("POST", "/route_template/"+templateID+"/update", admin, map[string]any{"body": doc})
	checkCall("custom/alpha/model")
	if got := templateBody(t, h, admin, templateID); got["model_routes"].([]any)[0].(map[string]any)["allocations"] == nil {
		t.Fatal("独立权重未持久化")
	}
	for _, bad := range []any{nil, weights(0, 0)["allocations"], []any{map[string]any{"deployment_id": "foreign", "weight": 1}}} {
		invalid := routeTemplateBody([]any{map[string]any{"model": "shared", "strategy": "traffic-split", "allocations": bad}}, 1, 60, 0, 0)
		if r := h.do("POST", "/route_template/"+templateID+"/update", admin, map[string]any{"body": invalid}); r.status != 400 {
			t.Fatalf("非法模板权重保存: %s", r.describe())
		}
	}
	checkCall("custom/alpha/model")
	h.ok("POST", "/route_template/"+templateID+"/update", admin, map[string]any{"body": routeTemplateBody([]any{}, 1, 60, 3, 0)})
	checkCall("custom/beta/model")
	if r := h.do(http.MethodGet, "/model/groups", "", nil); r.status != 401 {
		t.Fatalf("匿名目录未拒绝: %s", r.describe())
	}
	// 保留与默认相反的模板，删除默认正权重部署；两份持久化分配均须清理。
	doc["model_routes"] = []any{customRule}
	h.ok("POST", "/route_template/"+templateID+"/update", admin, map[string]any{"body": doc})
	h.ok("POST", "/model/delete", admin, map[string]any{"id": ids[1]})
	ids = ids[:1]
	if got := templateBody(t, h, admin, templateID)["model_routes"].([]any)[0].(map[string]any); got["allocations"] != nil {
		t.Fatalf("删除后模板仍有部署引用: %v", got)
	}
	groups = listField(h.ok("GET", "/model/groups?search=shared", admin, nil).json(), "data")
	if len(groups) != 1 || groups[0]["default_weights"] != nil {
		t.Fatalf("单部署仍保留默认分配: %v", groups)
	}
	checkCall("custom/alpha/model")
	// 改名最后一条部署后旧默认不得残留；模板规则不再引用任何部署 ID。
	h.ok("POST", "/model/update", admin, map[string]any{"model_info": map[string]any{"id": ids[0]}, "model_name": "renamed-shared"})
	if r := h.do("POST", "/model/delete", owner.session, map[string]any{"id": ids[0]}); r.status != 403 {
		t.Fatalf("客户可删除部署: %s", r.describe())
	}
}
