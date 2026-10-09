package regression

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/sunqirui1987/xhub/internal/catalog"
	"github.com/sunqirui1987/xhub/internal/config"
)

// 计价这一段要回答的问题是：同一批用量，在不同时段、不同计费维度下各收多少，
// 以及事后还能不能解释清楚那一笔。
//
// 前面的用例核对的是"一次成功调用扣了多少钱"，这里核对的是**那个数怎么来的**：
// 时段、缓存、按秒按张，以及日志详情读的是调用当时的价还是今天的价。
//
// 每一条都对应一个具体的漏钱方式，所以断言的是金额，不是状态码。

// addPricedModel 通过控制台加一条带费率表的部署，返回它的公开名。
//
// 费率表是计费读的那张表：它带四个维度（维度、侧、变体、时段），表达力比扁平的
// 每 token 两个字段强——分时价、缓存价、按秒按张都在里面。这里走控制台的添加接口，
// 因为价格最终是 litellm_params 的一个键，从界面写进去和从配置文件写进去必须
// 被同一段代码读到。
//
// 参数 h（*harness）：当前用例的网关；t（*testing.T）：当前测试；admin（string）：管理员的会话令牌；
// public（string）：公开模型名；rates（[]any）：费率表；mode（string）：调用方式。
// 返回：无。
func addPricedModel(t *testing.T, h *harness, admin, public string, rates []any, mode string) {
	t.Helper()
	h.ok(http.MethodPost, "/model/new", admin, map[string]any{
		"model_name": public,
		"litellm_params": map[string]any{
			"model":               "openai/" + public,
			"api_key":             "sk-fake-upstream",
			"api_base":            h.credentialAPIBase(),
			"custom_llm_provider": "openai",
			"rates":               rates,
		},
		"model_info": map[string]any{"transport": "bypass_openai_chat", "endpoint_types": []string{mode}},
	})
}

// tokenRates 造一张普通的每 token 费率表：一个输入侧、一个输出侧，不分时段。
// 参数 input（float64）：每 token 输入价；output（float64）：每 token 输出价。
// 返回 []any（[]any）：费率表。
func tokenRates(input, output float64) []any {
	return []any{
		map[string]any{"measure": "token", "unit_size": 1, "side": "input",
			"variant": "uncached", "window": "all", "source_key": "input", "usd": input},
		map[string]any{"measure": "token", "unit_size": 1, "side": "output",
			"window": "all", "source_key": "output", "usd": output},
	}
}

// TestAWindowPricedModelIsChargedTheWindowItLandedIn 是这次改动要保证的第一件事。
//
// 一条按高峰/空闲两档报价的部署，同一时刻只能落在其中一档。这里用一条空闲价是
// 输入价两倍的部署：如果计费真的读到了时段，这次调用要么按空闲价收，要么按两倍收，
// 不可能落在中间的某个数上。收成"最便宜那一档"（改动前的行为）就会得到一倍的价，
// 而那一刻是不是高峰由 catalog 的单测钉住。
//
// 断言的是**落在两档之一**，因为这条用例跑在哪一天哪一分钟不是它能决定的。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestAWindowPricedModelIsChargedTheWindowItLandedIn(t *testing.T) {
	h := newHarness(t)
	admin := h.adminSession()
	tn := h.provision(t, admin, "windowed")
	h.flushSpend()

	// 空闲价 1 倍，高峰价 2 倍。
	addPricedModel(t, h, admin, "regression-windowed", []any{
		map[string]any{"measure": "token", "unit_size": 1, "side": "input",
			"variant": "uncached", "window": "offpeak", "source_key": "ncache_offpeak", "usd": testInputRate},
		map[string]any{"measure": "token", "unit_size": 1, "side": "input",
			"variant": "uncached", "window": "peak", "source_key": "ncache_peak", "usd": testInputRate * 2},
		map[string]any{"measure": "token", "unit_size": 1, "side": "output",
			"window": "offpeak", "source_key": "output_offpeak", "usd": testOutputRate},
		map[string]any{"measure": "token", "unit_size": 1, "side": "output",
			"window": "peak", "source_key": "output_peak", "usd": testOutputRate * 2},
	}, "chat")

	r := h.ok(http.MethodPost, "/v1/chat/completions", tn.key, chatRequest("regression-windowed", "which window"))
	charged := parseFloatOrZero(r.header("x-litellm-response-cost"))

	offpeak := expectedCost()
	peak := offpeak * 2
	switch {
	case nearlyEqual(charged, offpeak):
		// 落在空闲时段，按空闲价收，正确。
	case nearlyEqual(charged, peak):
		// 落在高峰时段，按高峰价收，正确。
	default:
		t.Fatalf("a window-priced call was charged %v, which is neither the off-peak %v nor the peak %v",
			charged, offpeak, peak)
	}

	// 而且账单要说清是哪一档，并且**收的那一档和说出口的那一档必须一致**。
	//
	// 这是这条端到端用例真正能钉住的东西。绝对时段取决于用例跑在哪一分钟，
	// 所以它不能断言"这次是高峰"；但它能断言计费路径和时段判定是同一个答案——
	// 账单说高峰却按空闲价收，或者反过来，都要红。日历本身（周几、几点、
	// 节假日、调休）由 internal/catalog 的单测覆盖。
	bill := breakdownOf(t, h, admin, r.header("x-litellm-call-id"))
	window, _ := bill["window"].(string)
	if window != "peak" && window != "offpeak" {
		t.Fatalf("the breakdown recorded window %q; it must say which window the call landed in", window)
	}
	want := catalog.WindowAt(startOf(t, h, admin, r.header("x-litellm-call-id")))
	if window != want {
		t.Fatalf("the breakdown says %q but the call started at an instant that is %q", window, want)
	}
	// 用到的费率必须来自这个时段那一档，不能是另一档。这一条是上面那个
	// "收的钱等于该时段的价"的另一种说法，但它直接点名了键，失败信息更清楚。
	applied, _ := bill["applied"].([]any)
	if len(applied) == 0 {
		t.Fatalf("the breakdown recorded no applied rate: %s", truncate(string(mustJSON(bill)), 300))
	}
	for _, item := range applied {
		rate, _ := item.(map[string]any)
		key, _ := rate["source_key"].(string)
		if key == "" {
			continue
		}
		if strings.HasSuffix(key, "_peak") && window != "peak" {
			t.Fatalf("a call in the %s window was billed at the peak rate %s", window, key)
		}
		if strings.HasSuffix(key, "_offpeak") && window != "offpeak" {
			t.Fatalf("a call in the %s window was billed at the off-peak rate %s", window, key)
		}
	}
}

// startOf 读一行日志的开始时刻。
// 参数 t（*testing.T）：当前测试；h（*harness）：当前用例的网关；admin（string）：管理员会话；callID（string）：调用 id。
// 返回 time.Time（time.Time）：这一行的开始时刻。
func startOf(t *testing.T, h *harness, admin, callID string) time.Time {
	t.Helper()
	if callID == "" {
		t.Fatal("the response carried no call id")
	}
	row := logDetail(t, h, admin, callID)
	raw, _ := row["startTime"].(string)
	if raw == "" {
		t.Fatalf("the log row carried no start time: %s", truncate(string(mustJSON(row)), 300))
	}
	at, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		t.Fatalf("the log row start time %q is not readable: %v", raw, err)
	}
	return at
}

// TestAPerSecondModelIsNotRecordedAsFree 覆盖按秒计费。
//
// 视频那类模型在价目表里没有每 token 的价，只有每档分辨率下每秒的价。改动前
// 计费只读每 token 的两个字段，取不到就记一行零费用，也不报错——模型照常能用，
// 账单上是零，而且没有任何地方能看出来这是漏收而不是免费。这条用例钉住它按秒收。
//
// 这里走对话端点而不是视频任务的直通路径，因为要考的是**计费**而不是端点路由：
// 上游报 token、报秒数都可以，关键是这条部署的价目表里没有每 token 的价，
// 计费必须按秒把它算出来。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestAPerSecondModelIsNotRecordedAsFree(t *testing.T) {
	h := newHarness(t)
	admin := h.adminSession()
	tn := h.provision(t, admin, "persec")
	h.flushSpend()

	addPricedModel(t, h, admin, "regression-persec", []any{
		map[string]any{"measure": "second", "unit_size": 1, "side": "output",
			"window": "all", "source_key": "1080p_v_duration", "usd": 0.05},
	}, "chat")

	// 上游报的用量里没有秒数时这条部署一分钱都收不到，所以先让它报出来。
	// 视频任务接口回的就是这个形状：没有 token，只有时长。
	h.usageOverride("regression-persec", map[string]any{"seconds": 8.0})

	r := h.ok(http.MethodPost, "/v1/chat/completions", tn.key, chatRequest("regression-persec", "video"))
	charged := parseFloatOrZero(r.header("x-litellm-response-cost"))
	if charged <= 0 {
		t.Fatalf("a per-second model was billed %v; an upstream without token usage must not become a free call", charged)
	}
	if !nearlyEqual(charged, 8*0.05) {
		t.Fatalf("eight seconds at $0.05/s was billed %v, want 0.4", charged)
	}

	// 账单要按秒解释，不能把秒数说成 token 数。
	bill := breakdownOf(t, h, admin, r.header("x-litellm-call-id"))
	applied, _ := bill["applied"].([]any)
	if len(applied) == 0 {
		t.Fatalf("the breakdown recorded no applied rate: %s", truncate(string(mustJSON(bill)), 300))
	}
	rate, _ := applied[0].(map[string]any)
	if rate["measure"] != "second" {
		t.Fatalf("the applied rate says measure=%v, want second", rate["measure"])
	}
	if numberOrZero(rate["quantity"]) != 8 {
		t.Fatalf("the applied quantity is %v, want 8 seconds", rate["quantity"])
	}
}

// TestLogDetailExplainsTheChargeWithoutRecomputing 是"账目要存单价"这一条。
//
// 一次调用之后读日志详情，它必须拿**这次调用实际用到的费率**来解释那笔钱，
// 而不是按今天的价目表重算。判据是响应里带着 source 和 applied：
// source 说明这组数字的来源，applied 是被应用的那几条费率。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestLogDetailExplainsTheChargeWithoutRecomputing(t *testing.T) {
	h := newHarness(t)
	admin := h.adminSession()
	tn := h.provision(t, admin, "snapshot")
	h.flushSpend()

	addPricedModel(t, h, admin, "regression-snapshot", tokenRates(testInputRate, testOutputRate), "chat")

	r := h.ok(http.MethodPost, "/v1/chat/completions", tn.key, chatRequest("regression-snapshot", "explain"))
	charged := parseFloatOrZero(r.header("x-litellm-response-cost"))

	bill := breakdownOf(t, h, admin, r.header("x-litellm-call-id"))
	if bill["source"] != "snapshot" {
		t.Fatalf("source = %v, want snapshot for a freshly written row", bill["source"])
	}
	applied, _ := bill["applied"].([]any)
	if len(applied) == 0 {
		t.Fatalf("the breakdown did not record which rates were applied: %s", truncate(string(mustJSON(bill)), 400))
	}
	// 每条费率都要带自己的单位、数量和单价，否则控制台只能假设都是 token。
	for _, item := range applied {
		rate, _ := item.(map[string]any)
		if rate["measure"] == nil || rate["quantity"] == nil || rate["usd"] == nil {
			t.Fatalf("an applied rate is missing its unit, quantity or price: %v", rate)
		}
	}
	// 明细要能对上实际扣的钱。
	if got := numberOrZero(bill["original_cost"]); !nearlyEqual(got, charged) {
		t.Fatalf("the breakdown explains %v but the call was charged %v", got, charged)
	}
}

// TestTheBreakdownSurvivesAPriceChange 钉住快照存在的理由。
//
// 同一次调用读两遍：中间把这条部署的费率改成十倍。数字不能动——否则"当时按什么价
// 扣的"这个问题就没有答案了，运维每次看都要重新解释一遍历史账单。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestTheBreakdownSurvivesAPriceChange(t *testing.T) {
	h := newHarness(t)
	admin := h.adminSession()
	tn := h.provision(t, admin, "frozen")
	h.flushSpend()

	addPricedModel(t, h, admin, "regression-frozen", tokenRates(testInputRate, testOutputRate), "chat")

	r := h.ok(http.MethodPost, "/v1/chat/completions", tn.key, chatRequest("regression-frozen", "before"))
	callID := r.header("x-litellm-call-id")

	before := breakdownOf(t, h, admin, callID)
	// 把价改到十倍。历史那一行不该受影响。
	h.patchRates(t, admin, "regression-frozen", tokenRates(testInputRate*10, testOutputRate*10), nil)
	after := breakdownOf(t, h, admin, callID)

	if !nearlyEqual(numberOrZero(before["original_cost"]), numberOrZero(after["original_cost"])) {
		t.Fatalf("the recorded cost moved from %v to %v after a price change",
			before["original_cost"], after["original_cost"])
	}
	if !nearlyEqual(numberOrZero(before["input_cost"]), numberOrZero(after["input_cost"])) {
		t.Fatalf("the recorded input cost moved from %v to %v after a price change",
			before["input_cost"], after["input_cost"])
	}
	if after["source"] != "snapshot" {
		t.Fatalf("source = %v; a row written with a snapshot must not fall back to recomputing", after["source"])
	}

	// 反面：改价之后新发生的调用要按新价收。否则"快照没动"可能是因为计费根本没读价。
	next := h.ok(http.MethodPost, "/v1/chat/completions", tn.key, chatRequest("regression-frozen", "after"))
	nextCharged := parseFloatOrZero(next.header("x-litellm-response-cost"))
	if !nearlyEqual(nextCharged, parseFloatOrZero(r.header("x-litellm-response-cost"))*10) {
		t.Fatalf("after a tenfold price change a new call was charged %v, want ten times the previous %v",
			nextCharged, r.header("x-litellm-response-cost"))
	}
}

// TestAnUnpricedCallIsNotRecordedAsFree 证明目录里没有的模型记零费用时，
// 日志详情不会把零解释成"免费"。
//
// 零和"不知道多少钱"是两件事，控制台要给运维看到这个区别。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestAnUnpricedCallIsNotRecordedAsFree(t *testing.T) {
	// 部署上不写任何价，模型名也不在目录里。
	bare := config.ModelEntry{
		ModelName:     "regression-unpriced",
		LiteLLMParams: map[string]any{"model": "openai/regression-unpriced", "api_key": "sk-fake-upstream"},
		ModelInfo:     map[string]any{"transport": "bypass_openai_chat", "endpoint_types": []string{"chat"}},
	}
	h := newHarness(t, bare)
	admin := h.adminSession()
	tn := h.provision(t, admin, "unpriced")
	h.flushSpend()

	r := h.ok(http.MethodPost, "/v1/chat/completions", tn.key, chatRequest("regression-unpriced", "who knows"))

	// 没有价就不该发计费头：发了等于告诉调用方这次是零元，而实际是不知道。
	if r.header("x-litellm-response-cost") != "" {
		t.Fatalf("an unpriced call advertised a cost of %q", r.header("x-litellm-response-cost"))
	}

	bill := breakdownOf(t, h, admin, r.header("x-litellm-call-id"))
	// 但这一行仍然要存在，金额是零，而且不能有任何一侧的明细——那是编出来的。
	if _, present := bill["input_cost"]; present {
		t.Fatalf("an unpriced call grew an input cost: %v", bill["input_cost"])
	}
	if numberOrZero(bill["total_cost"]) != 0 {
		t.Fatalf("an unpriced call recorded %v", bill["total_cost"])
	}
}

// TestACachedCallIsNotBilledAtTheInputRate 覆盖缓存读。
//
// 缓存的价通常比输入价低两个数量级。把它当成输入价，一次带缓存的调用会多收
// 几百倍；反过来漏掉它，则少收。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestACachedCallIsNotBilledAtTheInputRate(t *testing.T) {
	h := newHarness(t)
	admin := h.adminSession()
	tn := h.provision(t, admin, "cached")
	h.flushSpend()

	addPricedModel(t, h, admin, "regression-cached", []any{
		map[string]any{"measure": "token", "unit_size": 1, "side": "input",
			"variant": "uncached", "window": "all", "source_key": "input", "usd": testInputRate},
		map[string]any{"measure": "token", "unit_size": 1, "side": "cache_read",
			"window": "all", "source_key": "cache", "usd": cacheReadRate},
		map[string]any{"measure": "token", "unit_size": 1, "side": "output",
			"window": "all", "source_key": "output", "usd": testOutputRate},
	}, "chat")

	// 800 个命中 + 200 个未命中，共 1000 个提示 token；完成 token 保持不变。
	h.usageOverride("regression-cached", map[string]any{
		"prompt_tokens": 1000, "completion_tokens": defaultReply.CompletionTokens,
		"prompt_tokens_details": map[string]any{"cached_tokens": 800},
	})

	r := h.ok(http.MethodPost, "/v1/chat/completions", tn.key, chatRequest("regression-cached", "cached prompt"))
	charged := parseFloatOrZero(r.header("x-litellm-response-cost"))
	want := 800*cacheReadRate + 200*testInputRate + float64(defaultReply.CompletionTokens)*testOutputRate
	if !nearlyEqual(charged, want) {
		t.Fatalf("a cached call was charged %v, want %v (800 cached + 200 uncached)", charged, want)
	}
	// 缓存那一档必须真的被用上：按输入价收整段提示会差两个数量级。
	if nearlyEqual(charged, 1000*testInputRate+float64(defaultReply.CompletionTokens)*testOutputRate) {
		t.Fatal("the cached prompt was billed at the input rate")
	}

	bill := breakdownOf(t, h, admin, r.header("x-litellm-call-id"))
	cacheCost := numberOrZero(bill["cache_read_cost"])
	if cacheCost <= 0 {
		t.Fatalf("the breakdown did not show what the cached prompt cost: %s", truncate(string(mustJSON(bill)), 300))
	}
	// 提示侧整体（含缓存那部分）必须不小于缓存那一行，否则控制台减完会看到负数。
	if input := numberOrZero(bill["input_cost"]); input < cacheCost {
		t.Fatalf("input_cost %v is below cache_read_cost %v; the drawer would render a negative input cost", input, cacheCost)
	}
}

// cacheReadRate 是一段缓存读的每 token 价，比输入价低两个数量级，
// 用来证明缓存那一档真的被用上了，而不是被当成输入价。
const cacheReadRate = 0.0000003

// breakdownOf 读一行日志的费用明细。
// 参数 t（*testing.T）：当前测试；h（*harness）：当前用例的网关；admin（string）：管理员会话；callID（string）：要读的那一次调用。
// 返回 map[string]any（map[string]any）：cost_breakdown 对象。
func breakdownOf(t *testing.T, h *harness, admin, callID string) map[string]any {
	t.Helper()
	if callID == "" {
		t.Fatal("the response carried no call id")
	}
	h.flushSpend()
	detail := h.ok(http.MethodGet, "/spend/logs/ui/"+callID, admin, nil).json()
	meta, _ := detail["metadata"].(map[string]any)
	bill, _ := meta["cost_breakdown"].(map[string]any)
	if bill == nil {
		t.Fatalf("the log detail for %s carried no cost breakdown: %s", callID, truncate(string(mustJSON(detail)), 400))
	}
	return bill
}

// numberOrZero 读一个已经是数字的字段。日志详情里的金额是 JSON 数字，
// 不是字符串头，所以走这条而不是解析响应头的那条。
// 参数 v（any）：JSON 解出来的一个值。
// 返回 float64（float64）：读到的小数。不是数字时为 0。
func numberOrZero(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	default:
		return 0
	}
}
