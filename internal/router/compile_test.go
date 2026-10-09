package router

import (
	"encoding/json"
	"errors"
	"github.com/sunqirui1987/xhub/internal/config"
	"reflect"
	"testing"
)

type compileStore struct {
	data map[string]map[string]any
	err  string
}

// ListConfig 提供编译器只读测试存储；参数为命名空间，返回内存配置或指定错误，不写状态。
func (s compileStore) ListConfig(name string) (map[string]any, error) {
	if name == s.err {
		return nil, errors.New("store unavailable")
	}
	return s.data[name], nil
}

// compileDoc 将测试对象转为实际JSON读回类型；返回独立文档，失败终止测试，无外部副作用。
func compileDoc(t *testing.T, fields map[string]any) map[string]any {
	t.Helper()
	raw, e := json.Marshal(fields)
	if e != nil {
		t.Fatal(e)
	}
	var out map[string]any
	if e = json.Unmarshal(raw, &out); e != nil {
		t.Fatal(e)
	}
	return out
}

// TestCompileOverrides 验证按来源/类别覆盖、空数组继承、显式禁用、输入隔离及模型默认实时读取；纯内存无需清理。
func TestCompileOverrides(t *testing.T) {
	models := []config.ModelEntry{{ModelName: "a"}, {ModelName: "b"}, {ModelName: "c"}}
	store := compileStore{data: map[string]map[string]any{"model_fallbacks": {"a": map[string]any{"fallbacks": []string{"b"}, "context_window_fallbacks": []string{"c"}}}, "model_defaults": {"a": map[string]any{"allocations": []any{map[string]any{"deployment_id": "a1", "weight": float64(3)}}}}}}
	doc := compileDoc(t, map[string]any{"model_routes": []any{}, "routing_groups": []any{}, "fallbacks": []any{map[string]any{"a": []any{}}}})
	before, _ := json.Marshal(store.data)
	got := Compile(RouteSettings{Settings: doc}, store, models)
	if got.Err != nil {
		t.Fatal(got.Err)
	}
	if len(got.ModelFallbacks["a"].Fallbacks) != 0 || !reflect.DeepEqual(got.ModelFallbacks["a"].ContextWindow, []string{"c"}) {
		t.Fatalf("按类别禁用错误: %+v", got.ModelFallbacks)
	}
	resolved := got.ForModel("a")
	if resolved.Err != nil || resolved.RuleSource != "model-default" || resolved.Policy.Shares()["a1"] != 3 {
		t.Fatalf("默认权重未合并: %+v", resolved)
	}
	doc["fallbacks"] = []any{}
	inherited := Compile(RouteSettings{Settings: doc}, store, models)
	if !reflect.DeepEqual(inherited.ModelFallbacks["a"].Fallbacks, []string{"b"}) {
		t.Fatal("空表未继承默认")
	}
	after, _ := json.Marshal(store.data)
	if string(before) != string(after) {
		t.Fatal("编译修改了存储输入")
	}
	store.data["model_defaults"]["a"] = map[string]any{"allocations": []any{map[string]any{"deployment_id": "a1", "weight": float64(7)}}}
	if Compile(RouteSettings{Settings: doc}, store, models).ForModel("a").Policy.Shares()["a1"] != 7 {
		t.Fatal("权重未实时读取")
	}
}

// TestCompileFailures 验证合并循环、悬空引用、存储失败和已有失败快速返回；内存目录无需清理。
func TestCompileFailures(t *testing.T) {
	models := []config.ModelEntry{{ModelName: "a"}, {ModelName: "b"}}
	store := compileStore{data: map[string]map[string]any{"model_fallbacks": {"a": map[string]any{"fallbacks": []string{"b"}}}}}
	for _, fields := range []map[string]any{{"fallbacks": []any{map[string]any{"b": []string{"a"}}}}, {"fallbacks": []any{map[string]any{"missing": []string{"a"}}}}, {"routing_groups": []any{map[string]any{"group_name": "g", "models": []string{"missing"}, "routing_strategy": "random"}}}} {
		if got := Compile(RouteSettings{Settings: compileDoc(t, fields)}, store, models); got.Err == nil {
			t.Fatalf("非法合并未拒绝: %v", fields)
		}
	}
	for _, name := range []string{"model_defaults", "routing_groups", "model_fallbacks"} {
		store.err = name
		if Compile(RouteSettings{Settings: map[string]any{}}, store, models).Err == nil {
			t.Fatalf("存储错误未传播: %s", name)
		}
	}
	original := errors.New("selected invalid")
	if Compile(RouteSettings{Err: original}, store, nil).Err != original {
		t.Fatal("覆盖了上游错误")
	}
}

// TestCompileGroupIsolation 验证新模板组为独立文档、旧模板兼容全局组；两者均只读，无外部清理。
func TestCompileGroupIsolation(t *testing.T) {
	models := []config.ModelEntry{{ModelName: "a"}}
	group := map[string]any{"group_name": "legacy", "models": []string{"a"}, "routing_strategy": "random"}
	store := compileStore{data: map[string]map[string]any{"routing_groups": {"legacy": group}}}
	if got := Compile(RouteSettings{Settings: map[string]any{}}, store, models); got.Err != nil || len(got.RoutingGroups) != 1 {
		t.Fatalf("旧模板兼容失败: %+v", got)
	}
	if got := Compile(RouteSettings{Settings: map[string]any{"routing_groups": []any{}}}, store, models); got.Err != nil || len(got.RoutingGroups) != 0 {
		t.Fatalf("新模板泄漏全局组: %+v", got)
	}
}

// TestCompileModelWeights 验证独立权重覆盖、空列表均等、省略继承及非法输入拒绝。
// 前置纯内存部署和配置存储；编译器不可修改默认配置，测试不产生外部数据。
func TestCompileModelWeights(t *testing.T) {
	models := []config.ModelEntry{{ModelName: "a", LiteLLMParams: map[string]any{"deployment_id": "a1"}}, {ModelName: "a", LiteLLMParams: map[string]any{"deployment_id": "a2"}}}
	store := compileStore{data: map[string]map[string]any{"model_defaults": {"a": map[string]any{"allocations": []any{map[string]any{"deployment_id": "a1", "weight": 1}, map[string]any{"deployment_id": "a2", "weight": 0}}}}}}
	for _, tc := range []struct {
		name        string
		custom      bool
		allocations any
		want        map[string]float64
	}{
		{"inherit", false, nil, map[string]float64{"a1": 1, "a2": 0}},
		{"custom", true, []any{map[string]any{"deployment_id": "a1", "weight": 0}, map[string]any{"deployment_id": "a2", "weight": 7}}, map[string]float64{"a1": 0, "a2": 7}},
		{"uniform", true, []any{}, map[string]float64{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rule := map[string]any{"model": "a", "strategy": "traffic-split"}
			if tc.custom {
				rule["allocations"] = tc.allocations
			}
			doc := compileDoc(t, map[string]any{"model_routes": []any{rule}, "routing_groups": []any{}})
			got := Compile(RouteSettings{Settings: doc}, store, models).ForModel("a")
			if got.Err != nil || !reflect.DeepEqual(got.Policy.Shares(), tc.want) {
				t.Fatalf("权重语义错误: %+v", got)
			}
			if p, _ := ParseDefaultWeights(store.data["model_defaults"]["a"]); p.Shares()["a1"] != 1 {
				t.Fatal("默认配置被修改")
			}
		})
	}
	for _, raw := range []any{nil, "bad", []any{nil}, []any{map[string]any{"deployment_id": "foreign", "weight": 1}}, []any{map[string]any{"deployment_id": "a1", "weight": nil}}, []any{map[string]any{"deployment_id": "a1", "weight": -1}}, []any{map[string]any{"deployment_id": "a1", "weight": 0}}, []any{map[string]any{"deployment_id": "a1", "weight": 1}, map[string]any{"deployment_id": "a1", "weight": 1}}} {
		doc := compileDoc(t, map[string]any{"model_routes": []any{map[string]any{"model": "a", "strategy": "traffic-split", "allocations": raw}}})
		if Compile(RouteSettings{Settings: doc}, store, models).Err == nil {
			t.Fatalf("非法权重被接受: %v", raw)
		}
	}
}
