package router

import (
	"github.com/sunqirui1987/xhub/internal/config"
	"math"
	"testing"
)

// deployment 为纯内存测试构造稳定 ID 部署；参数第三项不参与调度，返回独立配置，无外部清理。
func deployment(alias, base string, _ float64) config.ModelEntry {
	return config.ModelEntry{ModelName: alias, LiteLLMParams: map[string]any{"model": alias, "api_base": base, "deployment_id": base}}
}

// TestRandomAndTrafficIntervals 用确定抽样值验证随机区间、相对权重边界及健康过滤；无外部状态。
func TestRandomAndTrafficIntervals(t *testing.T) {
	a, b, c := deployment("m", "a", 0), deployment("m", "b", 0), deployment("m", "c", 0)
	pool := []config.ModelEntry{a, b, c}
	for _, tc := range []struct {
		draw           float64
		strategy, want string
	}{{0, "random", "a"}, {0.34, "random", "b"}, {0.999, "random", "c"}, {0, "traffic-split", "a"}, {0.6999, "traffic-split", "a"}, {0.7, "traffic-split", "b"}, {0.999, "traffic-split", "b"}} {
		st := State{Allocations: map[string]float64{"a": 70, "b": 30, "c": 0}, Draw: func() float64 { return tc.draw }}
		got := Pick(pool, "m", tc.strategy, st)
		if got == nil || DeploymentID(*got) != tc.want {
			t.Fatalf("draw=%v strategy=%s got=%v want=%s", tc.draw, tc.strategy, got, tc.want)
		}
	}
	st := State{Allocations: map[string]float64{"a": 70, "b": 30, "c": 0}, Cooldown: map[string]bool{CooldownID(a): true}, Draw: func() float64 { return 0 }}
	if got := Order(pool, "m", "traffic-split", st); len(got) != 1 || DeploymentID(got[0]) != "b" {
		t.Fatalf("过滤后归一化错误: %v", got)
	}
	st.Cooldown[CooldownID(b)] = true
	if got := Order(pool, "m", "traffic-split", st); len(got) != 0 {
		t.Fatal("全部冷却仍被调用")
	}
	if got := Order(pool, "m", "invalid", State{}); len(got) != 0 {
		t.Fatal("无效策略仍被调用")
	}
}

// TestStickySchedulingDoesNotDraw 验证有效粘性不抽样、零份额不能命中粘性、空池返回空；内存测试无需清理。
func TestStickySchedulingDoesNotDraw(t *testing.T) {
	a, b := deployment("m", "a", 0), deployment("m", "b", 0)
	calls := 0
	st := State{Allocations: map[string]float64{"a": 70, "b": 30}, Draw: func() float64 { calls++; return 0 }}
	got := Schedule([]config.ModelEntry{a, b}, "m", "traffic-split", st, CooldownID(b))
	if len(got) != 2 || DeploymentID(got[0]) != "b" || calls != 0 {
		t.Fatal("有效粘性发生抽样")
	}
	st.Allocations["b"] = 0
	got = Schedule([]config.ModelEntry{a, b}, "m", "traffic-split", st, CooldownID(b))
	if len(got) != 1 || DeploymentID(got[0]) != "a" {
		t.Fatal("零份额进入候选池")
	}
	if len(Schedule(nil, "missing", "random", State{}, "")) != 0 {
		t.Fatal("空池非空")
	}
}

// TestAllocationPolicyValidation 覆盖相对权重、重复身份、非法数值及跨模型引用；全部纯内存无需清理。
func TestAllocationPolicyValidation(t *testing.T) {
	valid := Policy{Strategy: "traffic-split", Allocations: []Allocation{{"a", 70}, {"b", 30}}}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := (Policy{Strategy: "traffic-split"}).Validate(); err != nil {
		t.Fatalf("空权重应表示所有兼容部署等权随机: %v", err)
	}
	bad := []Policy{{Strategy: "unknown"}, {Strategy: "random", Allocations: valid.Allocations}, {Strategy: "traffic-split", Allocations: []Allocation{{"a", 0}}}, {Strategy: "traffic-split", Allocations: []Allocation{{"a", 50}, {"a", 50}}}, {Strategy: "traffic-split", Allocations: []Allocation{{"", 100}}}, {Strategy: "traffic-split", Allocations: []Allocation{{"a", math.NaN()}}}, {Strategy: "traffic-split", Allocations: []Allocation{{"a", math.Inf(1)}}}}
	for i, p := range bad {
		if p.Validate() == nil {
			t.Fatalf("无效配置 %d 被接受", i)
		}
	}
	list := []config.ModelEntry{deployment("m", "a", 0), deployment("other", "b", 0)}
	if valid.ValidateDeployments(list, "m") == nil {
		t.Fatal("跨模型部署被接受")
	}
	if _, err := ParsePolicy(map[string]any{"strategy": "random", "unknown": 1}); err == nil {
		t.Fatal("未知字段被接受")
	}
}

// TestRuntimeMetricsSeparateDeployments 验证相同连接型号的独立部署拥有独立冷却、并发和延迟；无需外部清理。
func TestRuntimeMetricsSeparateDeployments(t *testing.T) {
	a, b := deployment("m", "a", 0), deployment("m", "b", 0)
	b.LiteLLMParams["api_base"] = a.LiteLLMParams["api_base"]
	for _, tc := range []struct {
		strategy string
		state    State
	}{{"least-busy", State{Busy: map[string]int{CooldownID(a): 10, CooldownID(b): 1}}}, {"latency-based-routing", State{Latency: map[string]float64{CooldownID(a): 100, CooldownID(b): 1}}}, {"usage-based-routing", State{Usage: map[string]float64{CooldownID(a): 100, CooldownID(b): 1}}}, {"random", State{Cooldown: map[string]bool{CooldownID(a): true}}}} {
		got := Pick([]config.ModelEntry{a, b}, "m", tc.strategy, tc.state)
		if got == nil || DeploymentID(*got) != "b" {
			t.Fatalf("独立指标失效: %s %v", tc.strategy, got)
		}
	}
}

// TestRelativeDefaultWeights 验证 3:7 无需合计 100、未配置新部署默认 1、单部署直接选择和零排除。
// 参数 t 为测试上下文；使用确定抽样证明权重区间，无供应商调用和外部清理。
func TestRelativeDefaultWeights(t *testing.T) {
	a, b, c := deployment("m", "a", 0), deployment("m", "b", 0), deployment("m", "c", 0)
	weights, err := ParseDefaultWeights(map[string]any{"allocations": []any{map[string]any{"deployment_id": "a", "weight": 3}, map[string]any{"deployment_id": "b", "weight": 7}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		draw float64
		want string
	}{{0, "a"}, {2.999 / 11, "a"}, {3.0 / 11, "b"}, {9.999 / 11, "b"}, {10.0 / 11, "c"}} {
		state := State{Allocations: weights.Shares(), Draw: func() float64 { return tc.draw }}
		got := Pick([]config.ModelEntry{a, b, c}, "m", "traffic-split", state)
		if got == nil || DeploymentID(*got) != tc.want {
			t.Fatalf("相对权重区间 %v: %v", tc.draw, got)
		}
	}
	if got := Pick([]config.ModelEntry{a}, "m", "traffic-split", State{}); got == nil || DeploymentID(*got) != "a" {
		t.Fatal("单部署无法选路")
	}
	if got := Pick([]config.ModelEntry{a}, "m", "traffic-split", State{Allocations: map[string]float64{"a": 0}}); got != nil {
		t.Fatal("零权重仍被调用")
	}
}

// TestSimpleShuffleIsUniformRandom 验证 simple-shuffle 等价均匀随机且不受零权重影响。
// 前置两个内存部署；确定抽样验证区间边界及空候选，无外部调用和清理。
func TestSimpleShuffleIsUniformRandom(t *testing.T) {
	a, b := deployment("m", "a", 0), deployment("m", "b", 0)
	if err := ValidateStrategy("simple-shuffle"); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		draw float64
		want string
	}{{0, "a"}, {0.499, "a"}, {0.5, "b"}, {0.999, "b"}} {
		got := Pick([]config.ModelEntry{a, b}, "m", "simple-shuffle", State{Allocations: map[string]float64{"a": 0, "b": 7}, Draw: func() float64 { return tc.draw }})
		if got == nil || DeploymentID(*got) != tc.want {
			t.Fatalf("均匀随机区间错误 draw=%v got=%v", tc.draw, got)
		}
	}
	if Pick(nil, "m", "simple-shuffle", State{}) != nil {
		t.Fatal("空候选仍被调用")
	}
}
