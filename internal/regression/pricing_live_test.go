package regression

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/sunqirui1987/xhub/internal/catalog"
	"github.com/sunqirui1987/xhub/internal/config"
)

// 这个文件拿**真实模型和真实上游回答**来验计量，供应商从环境变量发现。
//
// 为什么需要它：假上游能证明网关自己的接线对，但它证明不了"网关读得懂供应商
// 真正发回来的那个东西"。而计量出错的方式恰恰在这里——供应商对同一件事有不同的
// 拼法，读错一种不报错，只会让账单少一块。所以这一组必须对着真供应商跑。
//
// 供应商不写死在代码里。加一家只要多设一组变量，套件自己带上它跑；格式见
// harness_test.go 的 liveVendor。关着时整组跳过。
//
// 挑的是配置里给的模型、很短的提示、max_tokens 很小，但每一次都是真的花钱。

// liveModelDeployment 把一条真实模型做成部署。
//
// 部署上**不写单价**是有意的：这一组要证明的是"网关按公布的费率把这次调用算出来了"，
// 所以价必须来自内置价目表。部署上写死单价的话，价目表读不到也会绿。
//
// 参数 vendor（liveVendor）：供应商配置；model（string）：供应商认的模型名。
// 返回 config.ModelEntry（config.ModelEntry）：装进模型表的部署。
func liveModelDeployment(vendor liveVendor, model string) config.ModelEntry {
	return config.ModelEntry{
		// 对外名字带供应商标识，日志里不会有歧义。
		ModelName: vendor.ID + "/" + model,
		LiteLLMParams: map[string]any{
			"model":               model,
			"api_key":             vendor.Key,
			"api_base":            vendor.Base,
			"custom_llm_provider": vendor.Protocol,
			"timeout":             90,
		},
		ModelInfo: map[string]any{"endpoint_types": []string{"chat"}, "transport": "adapted"},
	}
}

// TestLiveRealModelsAreBilledFromTheCatalog 是这一组的核心：真实模型、真实回答、
// 真实计费，配置里的每一条模型都过一遍。
//
// 每一条断言四件事，缺一件就不是"完整"：
//
//  1. 供应商回了正数用量——否则后面几条都是空跑。
//  2. 网关按内置价目表算出了非零金额，也就是这条模型在目录里查得到。
//  3. 账单里留下了这次用到的费率，而且数量等于上游报的数量。
//  4. 金额和费率能对上：数量 × 单价 = 收到的那笔钱。
//
// 第 3、4 条是"计量准不准"真正的判据。只看响应头有金额不够——金额可能来自另一条
// 路径，而费率快照说明的是计费读了哪张表、用了哪一档。
// 参数 t（*testing.T）：当前测试。
// 返回：无。未开 live 模式或没配供应商时跳过。
func TestLiveRealModelsAreBilledFromTheCatalog(t *testing.T) {
	for _, vendor := range liveCredentials(t) {
		for _, model := range vendor.Models {
			public := vendor.ID + "/" + model
			t.Run(public, func(t *testing.T) {
				h := newHarness(t, liveModelDeployment(vendor, model))
				h.live = true
				admin := h.adminSession()
				tn := h.provision(t, admin, "live-"+slugOf(public))

				r := h.liveCallFor(t, tn.key, public, vendor.Protocol, "Reply with the single word ok.")
				usage, _ := r.json()["usage"].(map[string]any)
				if usage == nil {
					t.Fatalf("the vendor answer carried no usage, so there is nothing to measure: %s", r.describe())
				}
				upstreamPrompt, upstreamCompletion := liveCounts(usage)
				if upstreamPrompt <= 0 || upstreamCompletion <= 0 {
					t.Fatalf("the vendor reported prompt=%d completion=%d: %s",
						upstreamPrompt, upstreamCompletion, truncate(string(mustJSON(usage)), 300))
				}

				charged := parseFloatOrZero(r.header("x-litellm-response-cost"))
				if charged <= 0 {
					t.Fatalf("a real call to %s was recorded at zero cost; the built-in catalog has no usable rate for %q",
						public, model)
				}

				// 账单必须留下费率，而且数量就是上游报的那个数。
				bill := breakdownOf(t, h, admin, r.header("x-litellm-call-id"))
				if bill["source"] != "snapshot" {
					t.Fatalf("source = %v; a priced call must record the rates it used", bill["source"])
				}
				applied, _ := bill["applied"].([]any)
				if len(applied) == 0 {
					t.Fatalf("the breakdown named no applied rate: %s", truncate(string(mustJSON(bill)), 400))
				}
				expected := liveUsageCounts(usage)
				var billedInput, billedCachedInput, billedCacheRead, billedCacheWrite, billedCompletion, sum float64
				var hasInput, hasCachedInput, hasCacheRead, hasCacheWrite bool
				for _, item := range applied {
					rate, _ := item.(map[string]any)
					measure, _ := rate["measure"].(string)
					side, _ := rate["side"].(string)
					if measure != "token" {
						continue
					}
					quantity := numberOrZero(rate["quantity"])
					switch side {
					case "input":
						if rate["variant"] == "cached" {
							billedCachedInput += quantity
							hasCachedInput = true
						} else {
							billedInput += quantity
							hasInput = true
						}
					case "cache_read":
						billedCacheRead += quantity
						hasCacheRead = true
					case "cache_write":
						billedCacheWrite += quantity
						hasCacheWrite = true
					case "output":
						billedCompletion += quantity
					}
					sum += quantity * numberOrZero(rate["usd"])
				}
				// A catalog may price cache hits as an input variant, as a separate
				// cache_read side, or not price them separately at all. In every
				// case the input-side quantities must account for the full prompt once.
				if hasInput {
					wantInput := expected.prompt
					if hasCachedInput || hasCacheRead {
						wantInput -= expected.cacheRead
					}
					if billedInput != float64(wantInput) {
						t.Fatalf("the bill accounts for uncached input=%v, want %d from prompt=%d cache_read=%d", billedInput, wantInput, expected.prompt, expected.cacheRead)
					}
				}
				if hasCachedInput && billedCachedInput != float64(expected.cacheRead) {
					t.Fatalf("the bill accounts for cached input=%v but the vendor reported %d", billedCachedInput, expected.cacheRead)
				}
				if hasCacheRead && billedCacheRead != float64(expected.cacheRead) {
					t.Fatalf("the bill accounts for cache_read=%v but the vendor reported %d", billedCacheRead, expected.cacheRead)
				}
				if hasCacheWrite && billedCacheWrite != float64(expected.cacheWrite) {
					t.Fatalf("the bill accounts for cache_write=%v but the vendor reported %d", billedCacheWrite, expected.cacheWrite)
				}
				if billedCompletion != float64(upstreamCompletion) {
					t.Fatalf("the bill accounts for %v completion tokens but the vendor reported %d", billedCompletion, upstreamCompletion)
				}
				if !nearlyEqual(sum, charged) {
					t.Fatalf("the applied rates add up to %v but the call was charged %v", sum, charged)
				}

				// 用量行里的 token 数也要是上游报的那些，不能是估的。
				row := logDetail(t, h, admin, r.header("x-litellm-call-id"))
				if got := numberOrZero(row["prompt_tokens"]); got != float64(expected.prompt) {
					t.Fatalf("the log row records %v prompt tokens while the vendor reported %d", got, upstreamPrompt)
				}
				if got := numberOrZero(row["completion_tokens"]); got != float64(upstreamCompletion) {
					t.Fatalf("the log row records %v completion tokens while the vendor reported %d", got, upstreamCompletion)
				}
				t.Logf("%s: vendor reported prompt=%d completion=%d, charged %v from the built-in catalog",
					public, expected.prompt, expected.completion, charged)
			})
		}
	}
}

// TestLiveVendorUsageFieldsAreUnderstood 证明网关读得懂每一家真正发回来的那套字段名。
//
// 供应商对"输入 token"的定义不一致，这是最贵的一处不一致：
//
//	OpenAI 形状    prompt_tokens 是整段提示，命中部分在 prompt_tokens_details 里重述
//	Anthropic 形状 input_tokens 只是没命中的那部分，cache_read_input_tokens 另算一笔
//
// 读错一种不报错，只是账单少一块。这条用例把真实回答里的**字段名**逐个列出来，
// 要求网关的归一化认得它们，并把提示侧读成"整段提示"。
//
// 它不写死任何一家供应商：字段集合由真实回答决定，所以新接一家时如果它用的是
// 第三套拼法，这条会红在这里而不是红在账单上。
// 参数 t（*testing.T）：当前测试。
// 返回：无。未开 live 模式或没配供应商时跳过。
func TestLiveVendorUsageFieldsAreUnderstood(t *testing.T) {
	for _, vendor := range liveCredentials(t) {
		model := vendor.Models[0]
		public := vendor.ID + "/" + model
		t.Run(public, func(t *testing.T) {
			h := newHarness(t, liveModelDeployment(vendor, model))
			h.live = true
			admin := h.adminSession()
			tn := h.provision(t, admin, "live-fields-"+slugOf(public))

			r := h.liveCallFor(t, tn.key, public, vendor.Protocol, "Reply with the single word ok.")
			usage, _ := r.json()["usage"].(map[string]any)
			if usage == nil {
				t.Fatalf("the vendor answer carried no usage: %s", r.describe())
			}
			t.Logf("%s answers with usage fields: %v", public, sortedKeys(usage))

			// 上游报的提示数（按它自己那套字段读）。
			upstreamPrompt, upstreamCompletion := liveCounts(usage)

			// 网关归一化之后必须至少把上游报的提示数认全，不能少。
			// 少了就说明这家用的拼法没被认出来——那正是账单变少的原因。
			billed := catalog.NormalizeUsage(usage)
			expected := liveUsageCounts(usage)
			if billed.PromptTokens != expected.prompt {
				t.Fatalf("the gateway normalized the vendor's %d prompt tokens to %d; "+
					"the vendor's field names are not fully understood: %v",
					upstreamPrompt, billed.PromptTokens, sortedKeys(usage))
			}
			if billed.CompletionTokens != expected.completion {
				t.Fatalf("the gateway normalized the vendor's %d completion tokens to %d",
					upstreamCompletion, billed.CompletionTokens)
			}
			// 提示侧含缓存读，所以 = 未命中 + 命中；而缓存读不该超过总数。
			if billed.CachedTokens != expected.cacheRead {
				t.Fatalf("the normalized cache read is %d, vendor reported %d", billed.CachedTokens, expected.cacheRead)
			}
			if billed.CacheWriteTokens != expected.cacheWrite {
				t.Fatalf("the normalized cache write is %d, vendor reported %d", billed.CacheWriteTokens, expected.cacheWrite)
			}
		})
	}
}

// TestLiveStreamingIsBilledFromTheRealUsage 证明流式也读得到真实用量。
//
// 流式是另一条记账路径，而且用量的位置和一次返回不同：OpenAI 兼容的流在最后一个
// chunk 上带 usage，Anthropic 的流先嵌在 message_start 的 message 里报一次输入，
// 再在 message_delta 上平铺报一次最终值。只在流的尾巴上找 OpenAI 那个字段，
// Anthropic 那一路就会记成估算值。
//
// 断言读的是**日志行**而不是响应头：流式一旦开始写正文就不能再补计费头了，这是
// 有意为之（见 readme_cn.md 的断言约定）。所以流式要验的是"这一行记了多少钱、
// 多少 token"，响应头上没有金额是正常的，不是漏账。
// 参数 t（*testing.T）：当前测试。
// 返回：无。未开 live 模式或没配供应商时跳过。
func TestLiveStreamingIsBilledFromTheRealUsage(t *testing.T) {
	for _, vendor := range liveCredentials(t) {
		model := vendor.Models[0]
		public := vendor.ID + "/" + model
		t.Run(public, func(t *testing.T) {
			h := newHarness(t, liveModelDeployment(vendor, model))
			h.live = true
			admin := h.adminSession()
			tn := h.provision(t, admin, "live-stream-"+slugOf(public))

			path, body := liveCallShape(public, vendor.Protocol, "Reply with the single word ok.")
			body["stream"] = true
			if vendor.Protocol != "anthropic" {
				body["stream_options"] = map[string]any{"include_usage": true}
			}
			r := h.do(http.MethodPost, path, tn.key, body)
			if r.status != http.StatusOK {
				t.Fatalf("the live vendor refused the stream (%d): %s", r.status, r.describe())
			}
			// 流式正文里必须真的有事件下来，否则"计费对"可能是因为什么都没发生。
			if !strings.Contains(r.text(), "data:") && !strings.Contains(r.text(), "event:") {
				t.Fatalf("the stream carried no events: %s", truncate(r.text(), 300))
			}
			streamUsage := parseStreamUsage(r.text())
			if len(streamUsage) == 0 {
				t.Fatalf("the stream carried no usage event: %s", truncate(r.text(), 500))
			}
			expected := liveUsageCounts(streamUsage)
			if expected.prompt <= 0 || expected.completion <= 0 {
				t.Fatalf("the live stream lacked positive usage: %+v; %s", expected, truncate(r.text(), 500))
			}
			callID := r.header("x-litellm-call-id")
			if callID == "" {
				t.Fatal("the stream carried no call id, so its usage row cannot be found")
			}

			h.flushSpend()
			row := logDetail(t, h, admin, callID)
			spend := numberOrZero(row["spend"])
			if spend <= 0 {
				t.Fatalf("a real streaming call to %s was recorded at zero spend: %s",
					public, truncate(string(mustJSON(row)), 400))
			}
			if got := numberOrZero(row["completion_tokens"]); got != float64(expected.completion) {
				t.Fatalf("a streaming call recorded no completion tokens: %s", truncate(string(mustJSON(row)), 300))
			}
			if got := numberOrZero(row["prompt_tokens"]); got != float64(expected.prompt) {
				t.Fatalf("a streaming call recorded no prompt tokens: %s", truncate(string(mustJSON(row)), 300))
			}
			// 这一行也要能解释那一笔。
			bill := breakdownOf(t, h, admin, callID)
			if bill["source"] != "snapshot" {
				t.Fatalf("a streaming row's cost came from %v, not from the rates it was billed at", bill["source"])
			}
			applied, _ := bill["applied"].([]any)
			var rateSum float64
			for _, item := range applied {
				if rate, ok := item.(map[string]any); ok {
					rateSum += numberOrZero(rate["quantity"]) * numberOrZero(rate["usd"])
				}
			}
			if len(applied) == 0 || !nearlyEqual(rateSum, spend) {
				t.Fatalf("stream applied rates total %v, recorded spend %v: %s", rateSum, spend, truncate(string(mustJSON(bill)), 400))
			}
			t.Logf("%s streaming: spend %v recorded with %v prompt / %v completion tokens",
				public, spend, row["prompt_tokens"], row["completion_tokens"])
		})
	}
}

// TestLiveSameVendorViaEitherProtocolIsBilledAlike 证明同一个模型的两种协议被同等计费。
//
// 有的供应商同时提供 Anthropic 协议和 OpenAI 兼容协议，而且两套回答用的是两套用量
// 字段（input_tokens 与 prompt_tokens）。字段名不同，指的钱是同一笔，所以两条路径
// 的单位价必须一致——只接上一条，走另一条的调用就会算错。
//
// 只在配置了两种协议的模型上跑。用户可以用 XHUB_REGRESSION_<ID>_MODELS 加一条
// 走另一种协议的模型来表达这一点；没配就跳过，不硬编供应商。
// 参数 t（*testing.T）：当前测试。
// 返回：无。未开 live 模式或没有两家可比的供应商时跳过。
func TestLiveSameVendorViaEitherProtocolIsBilledAlike(t *testing.T) {
	vendors := liveCredentials(t)
	// 同一家供应商的两种协议，共用同一个上游模型名。
	type pair struct {
		vendor liveVendor
		model  string
	}
	byProtocol := map[string][]pair{}
	for _, vendor := range vendors {
		for _, model := range vendor.Models {
			byProtocol[vendor.Protocol] = append(byProtocol[vendor.Protocol], pair{vendor, model})
		}
	}
	var tested int
	for _, left := range byProtocol["anthropic"] {
		for _, right := range byProtocol["openai"] {
			if normalizeLiveBase(left.vendor.Base) != normalizeLiveBase(right.vendor.Base) || left.model != right.model {
				continue
			}
			tested++
			t.Run(left.vendor.ID+"/"+left.model+" via both protocols", func(t *testing.T) {
				runs := map[string]map[string]any{}
				for _, side := range []pair{left, right} {
					h := newHarness(t, liveModelDeployment(side.vendor, side.model))
					h.live = true
					admin := h.adminSession()
					tn := h.provision(t, admin, "live-both-"+side.vendor.Protocol)

					r := h.liveCallFor(t, tn.key, side.vendor.ID+"/"+side.model, side.vendor.Protocol,
						"Reply with the single word ok.")
					usage, _ := r.json()["usage"].(map[string]any)
					if usage == nil {
						t.Fatalf("%s: the vendor answer carried no usage: %s", side.vendor.Protocol, r.describe())
					}
					bill := breakdownOf(t, h, admin, r.header("x-litellm-call-id"))
					runs[side.vendor.Protocol] = bill
				}
				a, o := runs["anthropic"], runs["openai"]
				if a == nil || o == nil {
					t.Fatalf("one of the two protocols was not priced: %+v", runs)
				}
				if a["source"] != "snapshot" || o["source"] != "snapshot" {
					t.Fatalf("one protocol did not retain its pricing snapshot: %+v", runs)
				}
				if !sameRatePrices(a["applied"], o["applied"]) {
					t.Fatalf("the two protocols used different normalized rates: anthropic=%s openai=%s", truncate(string(mustJSON(a["applied"])), 500), truncate(string(mustJSON(o["applied"])), 500))
				}
			})
		}
	}
	if tested == 0 {
		t.Skip("no vendor is configured with the same model on both protocols")
	}
}

// liveCallFor 按指定协议发一次真实调用。
//
// Anthropic 协议走 /v1/messages，其余走 /v1/chat/completions。回应形状不同是正常的，
// 这里只关心调用成功和它带回来的用量。
// 参数 t（*testing.T）：当前测试；key（string）：调用方的密钥；model（string）：对外模型名；
// protocol（string）：协议标识；prompt（string）：提示词。
// 返回 reply（reply）：上游经网关回来的响应。
func (h *harness) liveCallFor(t *testing.T, key, model, protocol, prompt string) reply {
	t.Helper()
	path, body := liveCallShape(model, protocol, prompt)
	r := h.do(http.MethodPost, path, key, body)
	if r.status != http.StatusOK {
		t.Fatalf("live call failed: status=%d model=%s protocol=%s evidence=%s", r.status, model, protocol, r.describe())
	}
	return r
}

// liveCallShape 给出一次真实调用要走的入站路径和正文。
// 参数 model（string）：对外模型名；protocol（string）：协议标识；prompt（string）：提示词。
// 返回 path（string）：入站路径；body（map[string]any）：请求正文。
func liveCallShape(model, protocol, prompt string) (string, map[string]any) {
	if protocol == "anthropic" {
		return "/v1/messages", map[string]any{
			"model": model, "max_tokens": 16,
			"messages": []any{map[string]any{"role": "user", "content": prompt}},
		}
	}
	return "/v1/chat/completions", map[string]any{
		"model": model, "max_tokens": 16,
		"messages": []any{map[string]any{"role": "user", "content": prompt}},
	}
}

type liveUsage struct {
	prompt, input, cacheRead, cacheWrite, completion int
}

func liveUsageCounts(usage map[string]any) liveUsage {
	var out liveUsage
	out.input = firstLiveInt(usage, "prompt_tokens", "input_tokens")
	out.completion = firstLiveInt(usage, "completion_tokens", "output_tokens")
	if hasLiveValue(usage, "cache_read_input_tokens", "cache_read_tokens") {
		out.cacheRead = firstLiveInt(usage, "cache_read_input_tokens", "cache_read_tokens")
		out.prompt = out.input + out.cacheRead
	} else {
		out.cacheRead = firstLiveInt(usage, "cached_tokens")
		for _, key := range []string{"prompt_tokens_details", "input_tokens_details"} {
			if details, ok := usage[key].(map[string]any); ok && !hasLiveValue(usage, "cached_tokens") {
				if value, present := details["cached_tokens"]; present && value != nil {
					out.cacheRead = asIntAny(value)
					break
				}
			}
		}
		out.prompt = out.input
	}
	out.cacheWrite = firstLiveInt(usage, "cache_creation_input_tokens", "cache_write_tokens")
	return out
}

func hasLiveValue(usage map[string]any, keys ...string) bool {
	for _, key := range keys {
		if value, ok := usage[key]; ok && value != nil {
			return true
		}
	}
	return false
}

func firstLiveInt(usage map[string]any, keys ...string) int {
	for _, key := range keys {
		if value, ok := usage[key]; ok && value != nil {
			return asIntAny(value)
		}
	}
	return 0
}

// liveCounts 从供应商回的真实 usage 里读提示和完成 token 数。
//
// 两套拼法都读：OpenAI 的 prompt_tokens/completion_tokens，和 Anthropic 的
// input_tokens/output_tokens。这是测试自己的读法，故意和网关的归一化分开写，
// 这样网关读错了这里不会跟着错。
// 参数 usage（map[string]any）：供应商回的 usage 对象。
// 返回 prompt（int）：提示 token 数；completion（int）：完成 token 数。
func liveCounts(usage map[string]any) (prompt, completion int) {
	counts := liveUsageCounts(usage)
	return counts.prompt, counts.completion
}

func parseStreamUsage(raw string) map[string]any {
	merged := map[string]any{}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" || payload == "[DONE]" {
			continue
		}
		var event map[string]any
		if json.Unmarshal([]byte(payload), &event) != nil {
			continue
		}
		usage, _ := event["usage"].(map[string]any)
		if usage == nil {
			if message, ok := event["message"].(map[string]any); ok {
				usage, _ = message["usage"].(map[string]any)
			}
		}
		if usage == nil {
			if delta, ok := event["delta"].(map[string]any); ok {
				usage, _ = delta["usage"].(map[string]any)
			}
		}
		if usage == nil {
			if response, ok := event["response"].(map[string]any); ok {
				usage, _ = response["usage"].(map[string]any)
			}
		}
		for key, value := range usage {
			merged[key] = value
		}
	}
	return merged
}

func sameRatePrices(left, right any) bool {
	leftRates, lok := left.([]any)
	rightRates, rok := right.([]any)
	if !lok || !rok {
		return false
	}
	keys := func(rates []any) []string {
		out := make([]string, 0, len(rates))
		for _, v := range rates {
			m, _ := v.(map[string]any)
			if m["measure"] != "token" || (m["side"] != "input" && m["side"] != "output") || m["variant"] == "cached" {
				continue
			}
			out = append(out, fmt.Sprintf("%v|%v|%v|%v", m["measure"], m["side"], m["unit_size"], m["usd"]))
		}
		return out
	}
	leftKeys, rightKeys := keys(leftRates), keys(rightRates)
	if len(leftKeys) == 0 || len(rightKeys) == 0 {
		return false
	}
	sort.Strings(leftKeys)
	sort.Strings(rightKeys)
	return strings.Join(leftKeys, "\n") == strings.Join(rightKeys, "\n")
}

func normalizeLiveBase(base string) string {
	base = strings.TrimSuffix(strings.TrimSpace(base), "/")
	return strings.TrimSuffix(base, "/v1")
}

func TestLiveUsageOracleHonorsExplicitZeroAndNullAliases(t *testing.T) {
	got := liveUsageCounts(map[string]any{
		"prompt_tokens": nil, "input_tokens": float64(9),
		"completion_tokens": float64(0), "output_tokens": float64(4),
		"cached_tokens":         nil,
		"prompt_tokens_details": map[string]any{"cached_tokens": float64(2)},
	})
	if got.prompt != 9 || got.completion != 0 || got.cacheRead != 2 {
		t.Fatalf("live usage oracle did not preserve null fallback and explicit zero: %+v", got)
	}
}

func TestLiveUsageOracleDistinguishesNestedSubsetFromAnthropicCache(t *testing.T) {
	openAI := liveUsageCounts(map[string]any{
		"prompt_tokens":         float64(10),
		"prompt_tokens_details": map[string]any{"cached_tokens": float64(3)},
	})
	if openAI.prompt != 10 || openAI.cacheRead != 3 {
		t.Fatalf("OpenAI cached tokens are a subset of prompt tokens: %+v", openAI)
	}
	anthropic := liveUsageCounts(map[string]any{
		"input_tokens": float64(7), "cache_read_input_tokens": float64(3),
	})
	if anthropic.prompt != 10 || anthropic.cacheRead != 3 {
		t.Fatalf("Anthropic cache reads add to input tokens: %+v", anthropic)
	}
}

func TestParseStreamUsageMergesResponseUsageEvents(t *testing.T) {
	got := parseStreamUsage("data: {\"response\":{\"usage\":{\"input_tokens\":7}}}\n" +
		"data: {\"response\":{\"usage\":{\"output_tokens\":2}}}\n")
	if got["input_tokens"] != float64(7) || got["output_tokens"] != float64(2) {
		t.Fatalf("stream usage events were not merged: %#v", got)
	}
}

func TestSameRatePricesHandlesUnequalRateListLengths(t *testing.T) {
	left := []any{map[string]any{"measure": "token", "side": "input", "usd": float64(1)}}
	right := []any{
		map[string]any{"measure": "token", "side": "input", "usd": float64(1)},
		map[string]any{"measure": "token", "side": "output", "usd": float64(2)},
	}
	if sameRatePrices(left, right) {
		t.Fatal("different rate lists compared equal")
	}
}

func TestSameRatePricesRequiresComparableTokenRates(t *testing.T) {
	input := map[string]any{"measure": "token", "side": "input", "unit_size": float64(1), "usd": float64(1)}
	output := map[string]any{"measure": "token", "side": "output", "unit_size": float64(1), "usd": float64(2)}
	if sameRatePrices([]any{}, []any{}) || sameRatePrices([]any{map[string]any{"measure": "query"}}, []any{map[string]any{"measure": "query"}}) {
		t.Fatal("missing token-rate evidence compared equal")
	}
	if !sameRatePrices([]any{input, output}, []any{output, input}) {
		t.Fatal("the same input and output prices should compare equal regardless of order")
	}
	if sameRatePrices([]any{input}, []any{map[string]any{"measure": "token", "side": "input", "unit_size": float64(1), "usd": float64(3)}}) {
		t.Fatal("a changed input rate compared equal")
	}
}

// slugOf 把一个对外模型名收成能当变量名用的短标识。
// 参数 name（string）：对外模型名。
// 返回 string（string）：去掉斜杠和点之后的短标识。
func slugOf(name string) string {
	return strings.NewReplacer("/", "-", ".", "-", "_", "-").Replace(name)
}

// sortedKeys 列出一个对象的键，排好序，用来把"供应商回了哪套字段"写进日志和失败信息。
// 参数 m（map[string]any）：要读的对象。
// 返回 []string（[]string）：排好序的键。
func sortedKeys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// nowUTC 给出一个固定时刻，用于计费守卫的试算。取的是确定不在高峰的周末，
// 避免守卫因为跑在高峰时段而看到不同的价。
// 参数：无。
// 返回 time.Time（time.Time）：固定的试算时刻。
func nowUTC() time.Time {
	return time.Date(2026, 3, 7, 10, 0, 0, 0, time.UTC)
}
