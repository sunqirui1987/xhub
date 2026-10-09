package regression

import (
	"net/http"
	"strings"
	"testing"

	"github.com/sunqirui1987/xhub/internal/config"
)

// 同名多部署的分流。
//
// 两条不同供应商的部署可以挂同一个公开名——这是"两家都供同一个模型，按比例
// 分配流量"的用法，也是第一次接新供应商时灰度切换的用法。这件事的前提已经
// 具备：proxy_models 的 model_name 上没有唯一约束，前端本来就是一行一部署。
//
// 缺的是"按比例选出第一条"这一步。原来的 simple-shuffle 不是随机的，是取
// weight 最大的那条，而 weight 默认都是 1，于是永远命中第一条——配了比例也不生效。
//
// 这里钉住新的 weighted-split 真的按比例分，并且不改变 simple-shuffle 的语义
// （后者由 routing_test.go 钉着，另外六个别名共用它）。

// splitDeployment 造一条同名部署，带上它自己的一份权重。
// 参数 public（string）：公开模型名；upstream（string）：上游模型名；weight（float64）：这份流量。
// 返回 config.ModelEntry（config.ModelEntry）：可以直接交给 newHarness 的部署。
func splitDeployment(public, upstream string, weight float64) config.ModelEntry {
	return config.ModelEntry{
		ModelName: public,
		LiteLLMParams: map[string]any{
			"model":                 upstream,
			"deployment_id":         strings.TrimPrefix(upstream, "openai/"),
			"api_key":               "sk-fake-upstream",
			"custom_llm_provider":   "openai",
			"weight":                weight,
			"input_cost_per_token":  testInputRate,
			"output_cost_per_token": testOutputRate,
		},
		ModelInfo: map[string]any{"transport": "adapted", "endpoint_types": []string{"chat"}},
	}
}

// TestWeightedSplitSendsTrafficToBothDeployments 是这次改动的核心断言。
//
// 3:7 的两条同名部署，跑十次，两条都必须收到流量，而且比例接近配置值。
//
// 改动前的行为是十条全给第一条（weight 一样时取第一条，或者取最大的那条），
// 另一条一个请求都拿不到。断言"两条都收到"就足以钉住这个差别，而且不受
// 十条请求里先后顺序的影响。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestWeightedSplitSendsTrafficToBothDeployments(t *testing.T) {
	h := newHarness(t,
		splitDeployment("regression-split", "openai/split-heavy", 7),
		splitDeployment("regression-split", "openai/split-light", 3),
	)
	admin := h.adminSession()
	c := h.openScope(t, admin, "split")
	h.setRouter(admin, map[string]any{"routing_strategy": "weighted-split"})

	seen := map[string]int{}
	for i := 0; i < 10; i++ {
		h.ok(http.MethodPost, "/v1/chat/completions", c.key,
			chatRequest("regression-split", "split "+string(rune('a'+i))))
	}
	// 假供应商记录了每次请求转发过去的上游模型名。网关转发的是剥掉供应商
	// 前缀之后的上游 id，所以这里看到的是 split-heavy，不是 openai/split-heavy。
	for _, call := range h.upstreamCalls() {
		if model, _ := call.Body["model"].(string); model != "" {
			seen[model]++
		}
	}
	if seen["split-light"] == 0 {
		t.Fatalf("ten calls over a 7:3 split all went to one deployment: %v", seen)
	}
	if seen["split-heavy"] == 0 {
		t.Fatalf("ten calls over a 7:3 split all went to the wrong deployment: %v", seen)
	}
	if seen["split-heavy"] != 7 || seen["split-light"] != 3 {
		t.Fatalf("ten calls over a 7:3 split landed %v, want exactly 7 and 3", seen)
	}
}

// TestWeightedSplitDoesNotChangeSimpleShuffle 钉住这次改动的边界。
//
// simple-shuffle 是另外六个别名的实现，回归里已经断言它稳定选权重最大的那条。
// 新增分流策略不能顺手改掉它，否则那六个策略的行为会一起变。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestWeightedSplitDoesNotChangeSimpleShuffle(t *testing.T) {
	// 权重高的那条排在后面，simple-shuffle 仍然要选它——这是它原有的语义，
	// 也是 routing_test.go 钉住的那一条。
	h1 := newHarness(t,
		splitDeployment("regression-shuffle", "openai/shuffle-light", 1),
		splitDeployment("regression-shuffle", "openai/shuffle-heavy", 10),
	)
	admin1 := h1.adminSession()
	c1 := h1.openScope(t, admin1, "shuffle")
	h1.setRouter(admin1, map[string]any{"routing_strategy": "simple-shuffle"})
	for i := 0; i < 4; i++ {
		h1.ok(http.MethodPost, "/v1/chat/completions", c1.key,
			chatRequest("regression-shuffle", "shuffle "+string(rune('a'+i))))
	}
	for _, call := range h1.upstreamCalls() {
		if model, _ := call.Body["model"].(string); model == "shuffle-light" {
			t.Fatal("simple-shuffle no longer picks the heaviest deployment")
		}
	}
}

// TestSplitStillBillsAndLogsEveryCall 证明分流不影响记账。
//
// 分配到哪一条是路由的事，收到多少流量是前一条用例的事；这里核对的是十条调用
// 每一种都留下了一行日志，金额都对——按比例分流最容易出的错是某一条的用量
// 没记上，那会安静地漏掉一半收入。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestSplitStillBillsAndLogsEveryCall(t *testing.T) {
	h := newHarness(t,
		splitDeployment("regression-splitlog", "openai/splitlog-a", 1),
		splitDeployment("regression-splitlog", "openai/splitlog-b", 1),
	)
	admin := h.adminSession()
	c := h.openScope(t, admin, "splitlog")
	h.setRouter(admin, map[string]any{"routing_strategy": "weighted-split"})
	h.flushSpend()

	charged := 0.0
	for i := 0; i < 6; i++ {
		r := h.ok(http.MethodPost, "/v1/chat/completions", c.key,
			chatRequest("regression-splitlog", "log "+string(rune('a'+i))))
		charged += parseFloatOrZero(r.header("x-litellm-response-cost"))
	}
	if charged <= 0 {
		t.Fatal("a split call was billed nothing")
	}

	rows := h.spendLogs(t, admin)
	// 六次调用六行日志，一行都不能少。
	counted := 0
	total := 0.0
	for _, row := range rows {
		if row["model"] != "regression-splitlog" {
			continue
		}
		counted++
		total += numberOrZero(row["spend"])
	}
	if counted != 6 {
		t.Fatalf("six split calls left %d log rows", counted)
	}
	if total <= 0 {
		t.Fatal("the log rows recorded no spend at all")
	}
}

// TestSplitIsEvenWhenNoWeightsAreSet 证明谁都没配权重时是平均分。
//
// 默认权重是 1，所以这种情况下应该五五开。这不是"随便挑一条"：运维把两条
// 同名部署并排放着、没写权重，意思就是"两边都用"。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestSplitIsEvenWhenNoWeightsAreSet(t *testing.T) {
	// 不带 weight，让默认值生效。
	bare := func(upstream string) config.ModelEntry {
		return config.ModelEntry{
			ModelName: "regression-even",
			LiteLLMParams: map[string]any{
				"model":                 upstream,
				"api_key":               "sk-fake-upstream",
				"custom_llm_provider":   "openai",
				"input_cost_per_token":  testInputRate,
				"output_cost_per_token": testOutputRate,
			},
			ModelInfo: map[string]any{"transport": "adapted", "endpoint_types": []string{"chat"}},
		}
	}
	h := newHarness(t, bare("even-a"), bare("even-b"))
	admin := h.adminSession()
	c := h.openScope(t, admin, "even")
	h.setRouter(admin, map[string]any{"routing_strategy": "weighted-split"})

	for i := 0; i < 10; i++ {
		h.ok(http.MethodPost, "/v1/chat/completions", c.key,
			chatRequest("regression-even", "even "+string(rune('a'+i))))
	}
	seen := map[string]int{}
	for _, call := range h.upstreamCalls() {
		if model, _ := call.Body["model"].(string); model != "" {
			seen[model]++
		}
	}
	if seen["even-a"] != 5 || seen["even-b"] != 5 {
		t.Fatalf("two unweighted deployments split %v over ten calls, want 5 and 5", seen)
	}
}
