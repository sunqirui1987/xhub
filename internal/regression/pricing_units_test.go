package regression

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/sunqirui1987/xhub/internal/catalog"
)

// 这个文件回答的问题是：**每一种计费维度、每一种上游用量形状，都被真的计费了吗。**
//
// 前面的 pricing_test.go 钉的是"这个数怎么来的"（时段、缓存、快照），用的是
// litellm_params.rates 这一种写法。这里换一个角度，覆盖两件在那边没覆盖的事：
//
//  1. 控制台真正写出来的价格形状。单价表单一个格子一个字段，提交的是
//     input_cost_per_token、output_cost_per_second 这样的扁平键，不是一个 rates
//     数组。真实部署上"计费读的是哪个价"取决于这一支；只测 rates 的话，扁平
//     那一支断掉了也没人知道。
//
//  2. 目录里每一条模型、每一个它报价了的计费维度。这是唯一能挡住"某个维度整体
//     取不到价"的用例——那种故障不报错、状态码 200、账单上是零，只有把整张表
//     跑一遍才看得见。改动前按秒的 146 条费率一条都取不到，没有任何一条用例红过。

// flatTokenRates 造一份控制台单价表单会提交的扁平字段：一个输入价、一个输出价。
// 参数 input（float64）：每 token 输入价；output（float64）：每 token 输出价。
// 返回 map[string]any（map[string]any）：litellm_params 上的单价字段。
func flatTokenRates(input, output float64) map[string]any {
	return map[string]any{
		"input_cost_per_token":  input,
		"output_cost_per_token": output,
	}
}

// TestConsolePriceFormIsBilledForEveryDimension 逐维度覆盖控制台的单价形状。
//
// 每个子测试是一条真实的部署 + 一次真实的调用，价格只写在扁平字段上（就是从界面
// 加模型时会写进去的那几个键），断言的是金额和那一档实际用到的费率。
//
// 这是一张表而不是几个独立的用例，因为要证明的是"每一种维度都通"：漏掉哪一种，
// 那一种的调用就记一行零费用，模型照常能用，账单上看不出来。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestConsolePriceFormIsBilledForEveryDimension(t *testing.T) {
	cases := []struct {
		id     string
		name   string
		mode   string
		params map[string]any
		// usage 是上游这一次要报的量。nil 表示用假供应商的默认 token 数。
		usage map[string]any
		// want 按上面报出来的量算出应付金额。
		want func() float64
		// wantMeasure 是账单里必须出现的那一档计费维度。
		wantMeasure string
	}{
		{
			id: "token", name: "regression-flat-token", mode: "chat",
			params:      flatTokenRates(testInputRate, testOutputRate),
			wantMeasure: "token",
			want:        expectedCost,
		},
		{
			// 缓存读必须按缓存价，不是输入价。代价差两个数量级。
			id: "cache", name: "regression-flat-cache", mode: "chat",
			params: map[string]any{
				"input_cost_per_token":        testInputRate,
				"output_cost_per_token":       testOutputRate,
				"cache_read_input_token_cost": cacheReadRate,
			},
			usage: map[string]any{
				"prompt_tokens": 1000, "completion_tokens": defaultReply.CompletionTokens,
				"prompt_tokens_details": map[string]any{"cached_tokens": 800},
			},
			wantMeasure: "token",
			want: func() float64 {
				return 200*testInputRate + 800*cacheReadRate + float64(defaultReply.CompletionTokens)*testOutputRate
			},
		},
		{
			// 按秒：视频那类模型在价目表里没有每 token 的价。
			id: "second", name: "regression-flat-second", mode: "chat",
			params:      map[string]any{"output_cost_per_second": 0.05},
			usage:       map[string]any{"seconds": 8.0},
			wantMeasure: "second",
			want:        func() float64 { return 8 * 0.05 },
		},
		{
			// 按张：出图模型按张收。
			id: "picture", name: "regression-flat-picture", mode: "chat",
			params:      map[string]any{"output_cost_per_image": 0.04347826},
			usage:       map[string]any{"images": 3},
			wantMeasure: "picture",
			want:        func() float64 { return 3 * 0.04347826 },
		},
		{
			// 按次：联网搜索的价是一次查询，不是一秒。
			id: "query", name: "regression-flat-query", mode: "chat",
			params:      map[string]any{"search_context_cost_per_query": 0.01},
			usage:       map[string]any{"searches": 2},
			wantMeasure: "query",
			want:        func() float64 { return 2 * 0.01 },
		},
	}
	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			h := newHarness(t)
			admin := h.adminSession()
			tn := h.provision(t, admin, "flat-"+tc.id)
			addFlatPricedModel(t, h, admin, tc.name, tc.params, tc.mode)
			if tc.usage != nil {
				h.usageOverride(tc.name, tc.usage)
			}

			r := h.ok(http.MethodPost, "/v1/chat/completions", tn.key, chatRequest(tc.name, "bill me"))
			charged := parseFloatOrZero(r.header("x-litellm-response-cost"))
			want := tc.want()
			if !nearlyEqual(charged, want) {
				t.Fatalf("a %s-priced deployment was charged %v, want %v", tc.id, charged, want)
			}
			// 零费用在这里就是漏收：这次调用报了可计费的量，而价格写得明明白白。
			if charged <= 0 {
				t.Fatalf("a deployment with a %s price recorded a free call", tc.id)
			}

			// 账单要能解释这一笔，而且解释的维度要和收的钱对得上。
			bill := breakdownOf(t, h, admin, r.header("x-litellm-call-id"))
			if bill["source"] != "snapshot" {
				t.Fatalf("source = %v; a call billed from the deployment's own price must record what it used",
					bill["source"])
			}
			applied, _ := bill["applied"].([]any)
			if len(applied) == 0 {
				t.Fatalf("the breakdown named no applied rate: %s", truncate(string(mustJSON(bill)), 300))
			}
			measures := map[string]bool{}
			for _, item := range applied {
				rate, _ := item.(map[string]any)
				measure, _ := rate["measure"].(string)
				measures[measure] = true
				// 每一条费率都要带单位、数量、单价，否则控制台只能假设都是 token。
				if rate["measure"] == nil || rate["quantity"] == nil || rate["usd"] == nil {
					t.Fatalf("an applied rate is missing its unit, quantity or price: %v", rate)
				}
			}
			if !measures[tc.wantMeasure] {
				t.Fatalf("the bill was explained with measures %v, want %s", measures, tc.wantMeasure)
			}
			// 明细加起来要等于实际扣的钱。
			if got := numberOrZero(bill["original_cost"]); !nearlyEqual(got, charged) {
				t.Fatalf("the breakdown explains %v but the call was charged %v", got, charged)
			}
		})
	}
}

// TestTheFlatPeakFieldIsActuallyRead 钉住控制台上那两组单价格子里的高峰那一组。
//
// 单价表单分"空闲/高峰"两节，提交的是 input_cost_per_token 和
// input_cost_per_token_peak。改动前计费只读前者，于是高峰时段按空闲价收——那一格
// 填了不读，少收整整一半，而且没有任何地方能看出来。
//
// 断言方式是"落在两档之一，而且和账单说的那一档一致"，因为这次用例跑在哪一分钟
// 不是它能决定的。日历本身由 internal/catalog 的单测覆盖。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestTheFlatPeakFieldIsActuallyRead(t *testing.T) {
	const peakRate = testInputRate * 2
	const peakOut = testOutputRate * 2
	h := newHarness(t)
	admin := h.adminSession()
	tn := h.provision(t, admin, "flatpeak")
	addFlatPricedModel(t, h, admin, "regression-flat-peak", map[string]any{
		"input_cost_per_token":       testInputRate,
		"input_cost_per_token_peak":  peakRate,
		"output_cost_per_token":      testOutputRate,
		"output_cost_per_token_peak": peakOut,
	}, "chat")

	r := h.ok(http.MethodPost, "/v1/chat/completions", tn.key, chatRequest("regression-flat-peak", "which hour"))
	charged := parseFloatOrZero(r.header("x-litellm-response-cost"))
	bill := breakdownOf(t, h, admin, r.header("x-litellm-call-id"))
	window, _ := bill["window"].(string)

	offpeak := expectedCost()
	peak := float64(defaultReply.PromptTokens)*peakRate + float64(defaultReply.CompletionTokens)*peakOut
	switch window {
	case "offpeak":
		if !nearlyEqual(charged, offpeak) {
			t.Fatalf("the bill says off-peak but the call was charged %v, want %v", charged, offpeak)
		}
	case "peak":
		if !nearlyEqual(charged, peak) {
			t.Fatalf("the bill says peak but the call was charged %v, want %v", charged, peak)
		}
	default:
		t.Fatalf("the breakdown recorded window %q, which is neither of the two priced windows", window)
	}
	// 收的那一档必须和时段判定是同一个答案。
	if want := catalog.WindowAt(startOf(t, h, admin, r.header("x-litellm-call-id"))); window != want {
		t.Fatalf("the bill says %q but the call started in %q", window, want)
	}
	// 高峰价是空闲价的两倍，所以"填了不读"会正好落在中间那一档上。上面已经断言
	// 收的钱等于该档的价，这里再点名一次：不能是空闲价的两倍以外的任何一个数。
	if nearlyEqual(charged, peak) == nearlyEqual(charged, offpeak) {
		t.Fatalf("the charge %v is not attributable to either priced window", charged)
	}
}

// TestAnthropicShapedUsageIsBilledWhole 钉住上游用量形状这一处最贵的分歧。
//
// 供应商对"输入 token"的定义不一致：
//
//	OpenAI       prompt_tokens 是整段提示，命中的部分在 prompt_tokens_details 里重述一次
//	Anthropic    input_tokens 只是没命中的那部分，缓存读是**另外一笔**
//
// 用前者的读法读后者，800 个缓存 token 会整个丢掉，而剩下的 200 个反而按缓存价收
// ——一次调用少收一半。这条用例按 Anthropic 的形状让上游改口，断言收的钱等于
// "未命中 × 输入价 + 命中 × 缓存价"。
//
// 它走 /v1/messages，也就是 Anthropic 协议那条入站路径，假上游会按真实上游的事件
// 形状回：真实的上游回答里根本没有 prompt_tokens 这个键，只有 input_tokens 和
// cache_read_input_tokens。改动前假上游对两条路径都回 OpenAI 形状，于是这一整类
// 解析错在套件里无从发生——这正是"账单少了一半而回归全绿"的原因。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestAnthropicShapedUsageIsBilledWhole(t *testing.T) {
	h := newHarness(t)
	admin := h.adminSession()
	tn := h.provision(t, admin, "anthusage")
	addFlatPricedModel(t, h, admin, "regression-anth-usage", map[string]any{
		"input_cost_per_token":        testInputRate,
		"output_cost_per_token":       testOutputRate,
		"cache_read_input_token_cost": cacheReadRate,
	}, "chat")

	// 真实提示是 200 未命中 + 800 命中 = 1000；input_tokens 只算未命中的那部分。
	h.usageOverride("regression-anth-usage", map[string]any{
		"input_tokens":                200,
		"output_tokens":               50,
		"cache_read_input_tokens":     800,
		"cache_creation_input_tokens": 0,
	})

	r := h.ok(http.MethodPost, "/v1/messages", tn.key, map[string]any{
		"model": "regression-anth-usage", "max_tokens": 64,
		"messages": []any{map[string]any{"role": "user", "content": "cached"}},
	})
	charged := parseFloatOrZero(r.header("x-litellm-response-cost"))
	want := 200*testInputRate + 800*cacheReadRate + 50*testOutputRate
	if !nearlyEqual(charged, want) {
		t.Fatalf("an Anthropic-shaped usage was charged %v, want %v (200 uncached + 800 cached)", charged, want)
	}

	// 存下来的用量行要和收的钱说同一件事。cached_tokens 那一列从前只认
	// prompt_tokens_details，于是这一行会显示 0 次命中却按 800 收钱。
	bill := breakdownOf(t, h, admin, r.header("x-litellm-call-id"))
	if cacheCost := numberOrZero(bill["cache_read_cost"]); !nearlyEqual(cacheCost, 800*cacheReadRate) {
		t.Fatalf("the bill shows %v of cache read cost, want %v", cacheCost, 800*cacheReadRate)
	}
	row := logDetail(t, h, admin, r.header("x-litellm-call-id"))
	meta, _ := row["metadata"].(map[string]any)
	if got := numberOrZero(meta["cached_tokens"]); got != 800 {
		t.Fatalf("the log row records %v cached tokens while billing charged for 800", got)
	}
}

// TestTheCatalogPricesEveryMeasureItQuotes 是这张表的守卫。
//
// 它遍历内置价目表里的每一条模型，对每一个它报价了的计量（token / 秒 / 张 / 次）
// 按该计量的用量试算一次，要求**都能算出钱**。
//
// 为什么值得有这么一条：取不到价的调用不报错、状态码 200、账单是零。改动前按秒的
// 146 条费率一条都取不到（市场把限定词写进了变体名，而查找链只会退到空变体），
// 44 个模型的联网搜索被归成了按秒，22 个只有 thinking/text 变体的模型整个算不出钱
// ——26 个百分点的流量在漏，而当时套件里所有用例都是绿的。
//
// 模型名、费率和用量都来自线上读的同一张表，所以这条用例会在价目表刷新导致某个
// 维度再次整体取不到价时变红。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestTheCatalogPricesEveryMeasureItQuotes(t *testing.T) {
	// 固定在一个确定不是高峰也不会是节假日的时刻：周末上午。日历本身由
	// internal/catalog 的单测覆盖，这里只要一个稳定的时刻。
	at := time.Date(2026, 3, 7, 10, 0, 0, 0, time.UTC)
	rows := catalog.CostMap()
	if len(rows) == 0 {
		t.Fatal("the price catalog is empty, so this guard would pass vacuously")
	}

	// 每个计量按什么用量试算。token 有输入和输出两侧，所以分两条。
	probes := []struct {
		measure string
		side    string
		usage   catalog.Usage
	}{
		{"token", "input", catalog.Usage{PromptTokens: 1000}},
		{"token", "output", catalog.Usage{CompletionTokens: 1000}},
		{"second", "output", catalog.Usage{Seconds: 8}},
		{"second", "input", catalog.Usage{Seconds: 8}},
		{"picture", "input", catalog.Usage{Images: 1}},
		{"picture", "output", catalog.Usage{Images: 1}},
		{"query", "output", catalog.Usage{Searches: 1}},
	}

	var quoted, billed, skipped int
	for name, row := range rows {
		// 这一条模型对它报价过的每一个计量各试一次。catalog 的价格行结构在这里
		// 不走导出接口，所以直接读 rates 数组的原始形状。
		measures := quotedMeasures(row)
		if len(measures) == 0 {
			skipped++
			continue
		}
		for _, probe := range probes {
			if !measures[probe.measure+"\x00"+probe.side] {
				continue
			}
			quoted++
			if _, ok := catalog.CostAt(name, probe.usage, at); ok {
				billed++
				continue
			}
			t.Errorf("%s quotes a %s/%s rate but a call reporting that quantity is priced as nothing",
				name, probe.measure, probe.side)
		}
	}
	t.Logf("catalog: %d models, %d skipped (no rates), %d quoted sides, %d of them billable",
		len(rows), skipped, quoted, billed)
	if quoted == 0 {
		t.Fatal("no quoted side was found, so this guard proved nothing")
	}
}

// TestEveryCatalogModelIsReachableByTheNamesDeploymentsUse 是另一个守卫：
// 目录里的每一条模型，用它自己的 id、以及挂上供应商前缀之后，都要能被查到。
//
// 为什么单独有这么一条：部署的公开名通常带供应商前缀（`fenno/<vendor-model-id>`），
// 而价目表按供应商自己的模型 id 收录，别名表也是按那个 id 建的。只给完整名字查别名
// 会漏掉这一整类，于是带前缀的部署取不到目录价——要么运维在每条部署上手填一遍价，
// 要么这些调用记零费用。这一条把"整张表都能被部署名查到"变成一条可断言的约束。
//
// 前缀是从各家真实供应商名里取的：新接一家时如果用了一个新前缀而它查不到，
// 这条会红在接供应商的那一天，而不是红在账单上。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestEveryCatalogModelIsReachableByTheNamesDeploymentsUse(t *testing.T) {
	rows := catalog.CostMap()
	if len(rows) == 0 {
		t.Fatal("the price catalog is empty, so this guard would pass vacuously")
	}
	// 用真实用过的供应商前缀。目录里带前缀的 id 本身也要能查到。
	prefixes := []string{"fenno", "qiniu", "fennoai", "openai", "anthropic"}
	at := time.Date(2026, 3, 7, 10, 0, 0, 0, time.UTC)

	var checked, failed int
	for id := range rows {
		for _, name := range append([]string{id}, prefixedNames(id, prefixes)...) {
			checked++
			// 用这条模型报价了的第一个计量试算；没报价的条目不试。
			usage, ok := probeFor(rows[id])
			if !ok {
				continue
			}
			if _, ok := catalog.CostAt(name, usage, at); !ok {
				failed++
				if failed <= 10 {
					t.Errorf("a deployment named %q cannot be priced from the catalog, though the row for %q exists",
						name, id)
				}
			}
		}
	}
	t.Logf("checked %d deployment-name spellings across %d models; %d unreachable", checked, len(rows), failed)
	if failed > 0 {
		t.Fatalf("%d deployment names could not reach their catalog row", failed)
	}
}

// prefixedNames 给一个目录 id 挂上各个供应商前缀。
// 参数 id（string）：目录里的模型 id；prefixes（[]string）：供应商前缀。
// 返回 []string（[]string）：带前缀的名字。
func prefixedNames(id string, prefixes []string) []string {
	out := make([]string, 0, len(prefixes))
	for _, p := range prefixes {
		out = append(out, p+"/"+id)
	}
	return out
}

// probeFor 给出一个能触发这条模型计费的用量：它有哪个计量就用哪个。
// 参数 row（map[string]any）：价格表里这一条模型的字段。
// 返回 catalog.Usage（catalog.Usage）：试算用的用量；bool（bool）：这一行报了任何计费维度时为真。
func probeFor(row map[string]any) (catalog.Usage, bool) {
	rates, _ := row["rates"].([]any)
	for _, item := range rates {
		rate, ok := item.(map[string]any)
		if !ok {
			continue
		}
		measure, _ := rate["measure"].(string)
		side, _ := rate["side"].(string)
		switch {
		case measure == "token" && side == "input":
			return catalog.Usage{PromptTokens: 1000}, true
		case measure == "token" && side == "output":
			return catalog.Usage{CompletionTokens: 1000}, true
		case measure == "second":
			return catalog.Usage{Seconds: 8}, true
		case measure == "picture":
			return catalog.Usage{Images: 1}, true
		case measure == "query":
			return catalog.Usage{Searches: 1}, true
		}
	}
	return catalog.Usage{}, false
}

// quotedMeasures 列出一条价格行里报价过的 (计量, 侧) 组合，键用 "\x00" 拼起来。
//
// 它读的是行里的原始 JSON 形状而不是导出的结构，因为这个用例要独立的知道"表里有
// 什么"，而不是通过计费路径自己的索引去问——那样两边错在一起就看不出来了。
// 参数 row（map[string]any）：价格表里这一条模型的字段。
// 返回 map[string]bool（map[string]bool）：报价过的 (计量, 侧) 组合。
func quotedMeasures(row map[string]any) map[string]bool {
	out := map[string]bool{}
	rates, _ := row["rates"].([]any)
	for _, item := range rates {
		rate, ok := item.(map[string]any)
		if !ok {
			continue
		}
		measure, _ := rate["measure"].(string)
		side, _ := rate["side"].(string)
		if measure == "" {
			continue
		}
		// 联网搜索的价在表里单位名是 second，但它计的是**次数**，所以按 query
		// 归口。这是计费侧的解释，这里跟着它走，否则这条守卫会把"按秒取不到价"
		// 当成故障——它本来就是次数不是秒数。
		if measure == "second" && strings.TrimSpace(strField(rate, "variant")) == "search" {
			measure = "query"
			side = "output"
		}
		out[measure+"\x00"+side] = true
	}
	return out
}
