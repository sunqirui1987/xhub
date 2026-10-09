package router

import (
	"github.com/sunqirui1987/xhub/internal/config"
	"testing"
)

// TestScheduleUsesOnlyTemplateWeights 验证旧部署权重无效，默认一、零排除以及粘性不推进计数。
// 参数 t 为测试上下文；纯内存状态覆盖正常、空池、冷却边界，无外部清理。
func TestScheduleUsesOnlyTemplateWeights(t *testing.T) {
	a, b := databaseDeployment("schedule-a", 1), databaseDeployment("schedule-b", 1)
	pool := ApplyWeights([]config.ModelEntry{a, b}, nil)
	state := State{Splits: NewSplitState()}
	control := State{Splits: NewSplitState()}
	for i := 0; i < 8; i++ {
		if got := Schedule(pool, "shared-name", "weighted-split", state, CooldownID(b)); len(got) != 2 || CooldownID(got[0]) != CooldownID(b) {
			t.Fatal("粘性未固定")
		}
	}
	for i := 0; i < 10; i++ {
		got := Schedule(pool, "shared-name", "weighted-split", state, "")
		want := Schedule(pool, "shared-name", "weighted-split", control, "")
		if CooldownID(got[0]) != CooldownID(want[0]) {
			t.Fatal("粘性推进了轮询计数")
		}
	}
	zero := ApplyWeights(pool, map[string]float64{WeightID(b): 0})
	if got := Schedule(zero, "shared-name", "weighted-split", state, CooldownID(b)); len(got) != 1 || CooldownID(got[0]) != CooldownID(a) {
		t.Fatal("零权重进入候选")
	}
	cooling := State{Cooldown: map[string]bool{CooldownID(a): true, CooldownID(b): true}}
	if len(Available(pool, "weighted-split", cooling)) != 0 {
		t.Fatal("加权不应选择全部冷却部署")
	}
	if len(Available(pool, "simple-shuffle", cooling)) != 2 {
		t.Fatal("非加权冷却回退行为变化")
	}
	if len(Schedule(nil, "missing", "weighted-split", state, "")) != 0 {
		t.Fatal("空池应返回空")
	}
}
