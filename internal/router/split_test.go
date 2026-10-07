package router

import (
	"testing"

	"github.com/sunqirui1987/xhub/internal/config"
)

// deployment builds a deployment under a public name, with a weight and an api
// base so each one gets a distinct id.
func deployment(alias, apiBase string, weight float64) config.ModelEntry {
	return config.ModelEntry{
		ModelName: alias,
		LiteLLMParams: map[string]any{
			"model":    alias,
			"api_base": apiBase,
			"weight":   weight,
		},
	}
}

// drawSplit asks the strategy for n consecutive picks and counts them.
func drawSplit(t *testing.T, pool []config.ModelEntry, n int) map[string]int {
	t.Helper()
	st := State{Splits: NewSplitState()}
	counts := map[string]int{}
	for i := 0; i < n; i++ {
		picked := Pick(pool, "shared-name", "weighted_split", st)
		if picked == nil {
			t.Fatalf("draw %d picked nothing", i)
		}
		counts[picked.ParamString("api_base", "")]++
	}
	return counts
}

// TestWeightedSplitFollowsTheConfiguredRatio 是这次分流的核心断言：两条同名
// 部署按 3:7 配置，跑十次的分配正好是 3 和 7。
//
// 改动前的行为是永远选权重最大的那条（默认都是 1，所以永远选第一条），
// 另一条拿不到任何流量——配了比例也不生效。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestWeightedSplitFollowsTheConfiguredRatio(t *testing.T) {
	pool := []config.ModelEntry{
		deployment("shared-name", "https://a.example.com", 3),
		deployment("shared-name", "https://b.example.com", 7),
	}
	counts := drawSplit(t, pool, 10)
	if counts["https://a.example.com"] != 3 || counts["https://b.example.com"] != 7 {
		t.Fatalf("a 3:7 split over ten draws gave %v, want exactly 3 and 7", counts)
	}
}

// TestWeightedSplitDoesNotRequireHundred 证明权重是比例而不是百分比：3 和 7
// 等价于 30 和 70。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestWeightedSplitDoesNotRequireHundred(t *testing.T) {
	pool := []config.ModelEntry{
		deployment("shared-name", "https://a.example.com", 30),
		deployment("shared-name", "https://b.example.com", 70),
	}
	counts := drawSplit(t, pool, 10)
	if counts["https://a.example.com"] != 3 || counts["https://b.example.com"] != 7 {
		t.Fatalf("30:70 over ten draws gave %v, want the same as 3:7", counts)
	}
}

// TestSplitWithoutWeightsIsEven 钉住默认值：谁都没配权重时平均分，而不是
// 全压在第一条上。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestSplitWithoutWeightsIsEven(t *testing.T) {
	pool := []config.ModelEntry{
		{ModelName: "shared-name", LiteLLMParams: map[string]any{"model": "shared-name", "api_base": "https://a.example.com"}},
		{ModelName: "shared-name", LiteLLMParams: map[string]any{"model": "shared-name", "api_base": "https://b.example.com"}},
	}
	counts := drawSplit(t, pool, 10)
	if counts["https://a.example.com"] != 5 || counts["https://b.example.com"] != 5 {
		t.Fatalf("two unweighted deployments split %v over ten draws, want 5 and 5", counts)
	}
}

// TestSplitSkipsACoolingDeployment 证明掉线的那条拿不到流量，它的份额分给
// 剩下的人。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestSplitSkipsACoolingDeployment(t *testing.T) {
	pool := []config.ModelEntry{
		deployment("shared-name", "https://a.example.com", 1),
		deployment("shared-name", "https://b.example.com", 1),
	}
	st := State{
		Splits:   NewSplitState(),
		Cooldown: map[string]bool{DeploymentID(pool[0]): true},
	}
	for i := 0; i < 6; i++ {
		picked := Pick(pool, "shared-name", "weighted_split", st)
		if picked == nil {
			t.Fatal("both deployments were treated as down")
		}
		if picked.ParamString("api_base", "") != "https://b.example.com" {
			t.Fatalf("draw %d went to a deployment that is cooling down", i)
		}
	}
}

// TestSplitIsEvenAfterACoolingDeploymentReturns 证明恢复后的分配仍然平均。
//
// 掉线期间那条会累积分数。如果不清掉，它一恢复就会连着把所有流量拿走，
// 看起来像"刚回来的那条被打爆了"。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestSplitIsEvenAfterACoolingDeploymentReturns(t *testing.T) {
	pool := []config.ModelEntry{
		deployment("shared-name", "https://a.example.com", 1),
		deployment("shared-name", "https://b.example.com", 1),
	}
	splits := NewSplitState()
	cooling := State{Splits: splits, Cooldown: map[string]bool{DeploymentID(pool[0]): true}}
	// 第一条掉线期间，全部流量都给第二条。
	for i := 0; i < 10; i++ {
		Pick(pool, "shared-name", "weighted_split", cooling)
	}
	// 恢复之后，接下来的十次应该仍然平均，而不是全给刚回来的第一条。
	healthy := State{Splits: splits}
	counts := map[string]int{}
	for i := 0; i < 10; i++ {
		picked := Pick(pool, "shared-name", "weighted_split", healthy)
		counts[picked.ParamString("api_base", "")]++
	}
	if counts["https://a.example.com"] != 5 || counts["https://b.example.com"] != 5 {
		t.Fatalf("after recovery the split was %v, want 5 and 5", counts)
	}
}

// TestSplitWithOneDeploymentDoesNotDisturbIt 证明只有一条部署时永远选它，
// 不受别的别名共用计数的影响。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestSplitWithOneDeploymentDoesNotDisturbIt(t *testing.T) {
	pool := []config.ModelEntry{deployment("solo-name", "https://only.example.com", 1)}
	splits := NewSplitState()
	st := State{Splits: splits}
	for i := 0; i < 5; i++ {
		picked := Pick(pool, "solo-name", "weighted_split", st)
		if picked == nil || picked.ParamString("api_base", "") != "https://only.example.com" {
			t.Fatalf("draw %d did not pick the only deployment", i)
		}
	}
}

// TestSplitWithoutStateFallsBackToHighestWeight 证明没有装分流状态时退回
// "权重最大者优先"，而不是每个请求都失败。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestSplitWithoutStateFallsBackToHighestWeight(t *testing.T) {
	pool := []config.ModelEntry{
		deployment("shared-name", "https://a.example.com", 1),
		deployment("shared-name", "https://b.example.com", 5),
	}
	picked := Pick(pool, "shared-name", "weighted_split", State{})
	if picked == nil || picked.ParamString("api_base", "") != "https://b.example.com" {
		t.Fatalf("without split state the pick was %v, want the heaviest", picked)
	}
}

// TestWeightedSplitIsItsOwnStrategy 钉住这次改动的边界：加了分流策略之后，
// simple-shuffle 的语义不能跟着变。
//
// regression/routing_test.go 明确断言 simple-shuffle 稳定选权重最大的那条，
// 而它是另外六个别名的实现。改它会连带改那六个。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestWeightedSplitIsItsOwnStrategy(t *testing.T) {
	kind, ok := strategyKind("weighted_split")
	if !ok || kind != "split" {
		t.Fatalf("weighted_split resolved to %q (ok=%v), want split", kind, ok)
	}
	kind, ok = strategyKind("simple-shuffle")
	if !ok || kind != "weight" {
		t.Fatalf("simple-shuffle resolved to %q (ok=%v); its semantics must not change", kind, ok)
	}
	// 旧拼法也要认得。
	for _, name := range []string{"weighted_split", "weighted_round_robin", "traffic_split"} {
		if _, ok := strategyKind(name); !ok {
			t.Fatalf("%s is not accepted", name)
		}
	}
}

// TestSplitKeepsRatioAcrossManyDraws 用多轮抽样确认比例是稳定的，而不是
// 恰好在前十次对上。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestSplitKeepsRatioAcrossManyDraws(t *testing.T) {
	pool := []config.ModelEntry{
		deployment("shared-name", "https://a.example.com", 1),
		deployment("shared-name", "https://b.example.com", 4),
	}
	counts := drawSplit(t, pool, 500)
	if counts["https://a.example.com"] != 100 || counts["https://b.example.com"] != 400 {
		t.Fatalf("a 1:4 split over 500 draws gave %v, want exactly 100 and 400", counts)
	}
}

// TestCostStrategyDoesNotLetAnUnpricedDeploymentWin 钉住 cost 策略的一个
// 静默行为：没有写价的部署不能因为声明顺序靠前就赢。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestCostStrategyDoesNotLetAnUnpricedDeploymentWin(t *testing.T) {
	unpriced := config.ModelEntry{
		ModelName:     "shared-name",
		LiteLLMParams: map[string]any{"model": "shared-name", "api_base": "https://unknown.example.com"},
	}
	cheap := config.ModelEntry{
		ModelName: "shared-name",
		LiteLLMParams: map[string]any{
			"model": "shared-name", "api_base": "https://cheap.example.com",
			"input_cost_per_token": 0.000001,
		},
	}
	// 没写价的那条排在前面，它不能因此胜出。
	picked := Pick([]config.ModelEntry{unpriced, cheap}, "shared-name", "lowest_cost", State{})
	if picked == nil {
		t.Fatal("the cost strategy picked nothing")
	}
	if picked.ParamString("api_base", "") != "https://cheap.example.com" {
		t.Fatal("a deployment with no stated price won the cost comparison by declaration order")
	}
}
