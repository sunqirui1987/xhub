package prefs

import "testing"

// TestTemplateOnlyOverridesNamedModels 验证模板只覆盖明确列出的公开模型；未列出的模型继续读取模型管理实时权重。
// 前置条件为四个模型均有默认分配，模板只包含 a、b；测试只使用内存数据，无需清理。
func TestTemplateOnlyOverridesNamedModels(t *testing.T) {
	weights := func(id string, weight float64) map[string]any {
		return map[string]any{"allocations": []any{map[string]any{"deployment_id": id, "weight": weight}}}
	}
	settings := RouteSettings{
		Settings: map[string]any{
			"model_routes": []any{
				map[string]any{"model": "a", "strategy": "least-busy"},
				map[string]any{"model": "b", "strategy": "traffic-split"},
			},
			"retry_policy": map[string]any{"max_attempts": 4, "timeout_seconds": 60, "failure_threshold": 3, "cooldown_seconds": 0},
		},
		ModelDefaults: map[string]any{"a": weights("a1", 9), "b": weights("b1", 7), "c": weights("c1", 3), "d": weights("d1", 1)},
	}

	a := settings.ForModel("a")
	if a.Err != nil || a.Strategy() != "least-busy" || a.RuleSource != "template-model" || len(a.Policy.Allocations) != 0 {
		t.Fatalf("模型 a 没有使用模板策略: %+v", a)
	}
	b := settings.ForModel("b")
	if b.Err != nil || b.Strategy() != "traffic-split" || b.RuleSource != "template-model" || b.Policy.Shares()["b1"] != 7 {
		t.Fatalf("模型 b 没有使用模板策略和模型管理权重: %+v", b)
	}
	for _, model := range []string{"c", "d"} {
		got := settings.ForModel(model)
		if got.Err != nil || got.Strategy() != "traffic-split" || got.RuleSource != "model-default" || len(got.Policy.Allocations) != 1 {
			t.Fatalf("未覆盖模型 %s 没有使用模型管理默认分配: %+v", model, got)
		}
	}
	settings.ModelDefaults["b"] = weights("b2", 5)
	if got := settings.ForModel("b"); got.Policy.Shares()["b2"] != 5 {
		t.Fatalf("模板复制了模型权重，没有读取实时配置: %+v", got.Policy)
	}
}

// TestBuiltinAndEmptyWeightsUseEqualTrafficSplit 验证未绑定模板及未显式设置权重时都采用按流量分流。
// 空权重交由调度器按每个兼容部署权重 1 处理；测试不访问持久化，无需清理。
func TestBuiltinAndEmptyWeightsUseEqualTrafficSplit(t *testing.T) {
	settings := BuiltinSettings()
	settings.ModelDefaults = map[string]any{}
	got := settings.ForModel("unconfigured")
	if got.Err != nil || got.Strategy() != "traffic-split" || got.RuleSource != "model-default" || len(got.Policy.Allocations) != 0 {
		t.Fatalf("内置默认不是等权流量分流: %+v", got)
	}
}

// TestStrictRouteTemplateDocument 验证模板拒绝旧字段、入口规则、无效部署权重、重复模型和非法数值。
// 每个输入都应在写入前失败；测试仅构造内存文档，无需清理。
func TestStrictRouteTemplateDocument(t *testing.T) {
	validRetry := func() map[string]any {
		return map[string]any{"max_attempts": 1, "timeout_seconds": 60, "failure_threshold": 3, "cooldown_seconds": 0}
	}
	doc := func(routes []any) map[string]any {
		return map[string]any{"model_routes": routes, "retry_policy": validRetry()}
	}
	cases := []map[string]any{
		{}, {"model_routes": []any{}}, {"retry_policy": validRetry()},
		{"model_routes": []any{}, "retry_policy": map[string]any{"max_attempts": 1}},
		{"model_routes": []any{}, "retry_policy": map[string]any{"max_attempts": 1, "timeout_seconds": 60, "failure_threshold": 3, "cooldown_seconds": 0, "extra": true}},
		{"model_routes": []any{}, "retry_policy": validRetry(), "num_retries": 1},
		{"model_routing": []any{}, "retry_policy": validRetry()},
		{"model_overrides": []any{}, "retry_policy": validRetry()},
		doc([]any{map[string]any{"model": "m", "strategy": "traffic-split", "endpoint_id": "chat"}}),
		doc([]any{map[string]any{"model": "m", "strategy": "traffic-split", "allocations": nil}}),
		doc([]any{map[string]any{"model": "m", "strategy": "traffic-split"}, map[string]any{"model": "m", "strategy": "least-busy"}}),
		doc([]any{map[string]any{"model": "m", "strategy": "unknown"}}),
	}
	for _, doc := range cases {
		if ValidateRouteTemplateDocument(doc) == nil {
			t.Fatalf("无效模板被接受: %v", doc)
		}
	}
	if err := ValidateRouteTemplateDocument(doc([]any{map[string]any{"model": "m", "strategy": "traffic-split"}})); err != nil {
		t.Fatalf("合法模板被拒绝: %v", err)
	}
}

// TestTemplateDefaultStrategyAndOverride 验证模板默认、模型例外和旧格式继承优先级。
// 前置内存文档及模型权重；正常输入生效，非法默认拒绝，无外部数据清理。
func TestTemplateDefaultStrategyAndOverride(t *testing.T) {
	settings := RouteSettings{Settings: BuiltinDocument(), ModelDefaults: map[string]any{}}
	settings.Settings["routing_strategy"] = "simple-shuffle"
	settings.Settings["model_routes"] = []any{map[string]any{"model": "special", "strategy": "least-busy"}}
	if err := ValidateRouteTemplateDocument(settings.Settings); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ model, strategy, source string }{{"normal", "simple-shuffle", "template-default"}, {"special", "least-busy", "template-model"}} {
		got := settings.ForModel(tc.model)
		if got.Err != nil || got.Strategy() != tc.strategy || got.RuleSource != tc.source {
			t.Fatalf("默认及例外优先级错误: %+v", got)
		}
	}
	for _, raw := range []any{"", "unknown", nil, 1} {
		settings.Settings["routing_strategy"] = raw
		if ValidateRouteTemplateDocument(settings.Settings) == nil {
			t.Fatalf("接受非法默认策略: %v", raw)
		}
		if settings.ForModel("normal").Err == nil {
			t.Fatalf("运行时未拒绝非法默认策略: %v", raw)
		}
	}
	delete(settings.Settings, "routing_strategy")
	got := settings.ForModel("normal")
	if got.Strategy() != "traffic-split" || got.RuleSource != "model-default" {
		t.Fatalf("旧模板继承失效: %+v", got)
	}
}
