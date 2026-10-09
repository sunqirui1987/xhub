package prefs

import (
	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/router"
	"testing"
)

// TestRoutingGroupPrecedence 验证模板例外、组、模板默认优先级，缺省组权重不继承成员权重；内存测试无清理。
func TestRoutingGroupPrecedence(t *testing.T) {
	settings := BuiltinSettings()
	settings.Settings["routing_strategy"] = "least-busy"
	settings.ModelDefaults = map[string]any{"a": map[string]any{"allocations": []any{map[string]any{"deployment_id": "a1", "weight": 0}, map[string]any{"deployment_id": "a2", "weight": 1}}}}
	settings.RoutingGroups = []router.Group{{Name: "group", Models: []string{"a", "b"}, Strategy: "traffic-split"}}
	r := settings.ForModel("a")
	if r.Err != nil || r.RuleSource != "routing-group" || len(r.Policy.Allocations) != 0 {
		t.Fatalf("组缺省继承成员权重: %+v", r)
	}
	settings.Settings["model_routes"] = []any{map[string]any{"model": "a", "strategy": "simple-shuffle"}}
	if r := settings.ForModel("a"); r.RuleSource != "template-model" || r.Strategy() != "simple-shuffle" {
		t.Fatalf("例外失效: %+v", r)
	}
	if r := settings.ForModel("other"); r.RuleSource != "template-default" || r.Strategy() != "least-busy" {
		t.Fatal("非组成员未继承模板")
	}
	settings.RoutingGroups[0].Args = &router.GroupArgs{Allocations: []router.Allocation{{DeploymentID: "a1", Weight: 0}, {DeploymentID: "b1", Weight: 1}}}
	list := []config.ModelEntry{{ModelName: "a", LiteLLMParams: map[string]any{"model": "up-a", "deployment_id": "a1"}}, {ModelName: "b", LiteLLMParams: map[string]any{"model": "up-b", "deployment_id": "b1"}}}
	r = settings.ForModel("group")
	if got := r.Schedule(list, "group", router.State{}, ""); len(got) != 1 || got[0].ModelName != "b" {
		t.Fatalf("组调度失效: %v", got)
	}
	r = settings.ForModel("b")
	if got := r.Schedule(list, "b", router.State{}, ""); len(got) != 1 || got[0].ModelName != "b" {
		t.Fatalf("成员调度跨模型: %v", got)
	}
	if len(r.Candidates(nil, "group")) != 0 {
		t.Fatal("空池非空")
	}
	settings.Settings["model_routes"] = []any{map[string]any{"model": "a", "strategy": "unknown"}}
	if settings.ForModel("group").Err == nil {
		t.Fatal("非法模板未失败")
	}
}
