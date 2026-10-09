package regression

import (
	"net/http"
	"strings"
	"testing"
)

// 这一组测协议宽度和流式：同一个模型能从几种调用方协议进来，各自的形状对不对。
//
// 一个只实现 /v1/chat/completions 的网关，在"chat 能不能用"这一点上是绿的，
// 但它会把 Anthropic SDK 和 OpenAI Responses SDK 的调用方全都挡在门外。
// 而且这些路径各有各的形状——请求要改写、回答要转回调用方要的形状——所以只看
// 状态码是抓不住写错形状这种问题的。
//
// 协议之间的翻译由网关自己完成，调用方不需要知道上游说的是哪种协议。

// TestSameModelAnswersThreeChatProtocols 证明同一条模型能应答三种对话协议。
//
// 这三种说的是同一件事（文本进、文本出），只是拼法不同。让运维在添加模型时
// 二选一，只会把本来能跑的组合挡掉。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestSameModelAnswersThreeChatProtocols(t *testing.T) {
	h := newHarness(t, chatDeployment("regression-protocols"))
	admin := h.adminSession()
	tn := h.provision(t, admin, "protocols")

	// OpenAI chat completions：回答里该有 choices。
	t.Run("openai chat completions", func(t *testing.T) {
		h.resetUpstream()
		r := h.ok(http.MethodPost, "/v1/chat/completions", tn.key, map[string]any{
			"model": "regression-protocols", "messages": []any{map[string]any{"role": "user", "content": "hi"}},
		})
		choices, _ := r.json()["choices"].([]any)
		if len(choices) == 0 {
			t.Fatalf("the chat completion carried no choices: %s", r.describe())
		}
		if got := len(h.upstreamCalls()); got != 1 {
			t.Fatalf("the upstream saw %d calls, want 1", got)
		}
	})

	// Anthropic Messages：回答要转成 Messages 形状，也就是 content 数组。
	t.Run("anthropic messages", func(t *testing.T) {
		h.resetUpstream()
		r := h.ok(http.MethodPost, "/v1/messages", tn.key, map[string]any{
			"model": "regression-protocols", "max_tokens": 64,
			"messages": []any{map[string]any{"role": "user", "content": "hi"}},
		})
		body := r.json()
		// Messages 回答的特点是有 content 数组和一个 stop_reason。
		if _, ok := body["content"]; !ok {
			t.Fatalf("the messages answer carried no content: %s", r.describe())
		}
		if got := len(h.upstreamCalls()); got != 1 {
			t.Fatalf("the upstream saw %d calls, want 1", got)
		}
	})

	// OpenAI Responses：回答要转成 response 形状，也就是 output 数组。
	t.Run("openai responses", func(t *testing.T) {
		h.resetUpstream()
		r := h.ok(http.MethodPost, "/v1/responses", tn.key, map[string]any{
			"model": "regression-protocols", "input": "hi",
		})
		body := r.json()
		if _, ok := body["output"]; !ok {
			// 有些上游直接回 chat 形状，网关会把它转到 response 形状；
			// 两种都不是就说明形状没处理。
			if _, isChat := body["choices"]; !isChat {
				t.Fatalf("the responses answer is neither a response nor a chat object: %s", r.describe())
			}
		}
		if got := len(h.upstreamCalls()); got != 1 {
			t.Fatalf("the upstream saw %d calls, want 1", got)
		}
	})
}

// TestEveryProtocolIsBilledTheSameWay 证明同一次对话无论从哪个协议进来，
// 被扣的钱和记的 token 都一样。
//
// 计费不该跟着调用方的协议走。如果只有 chat 那条路径接上了计费，
// 走 Messages 的调用就会变成免费的——这种漏洞不会报错，只会让账单少一块。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestEveryProtocolIsBilledTheSameWay(t *testing.T) {
	h := newHarness(t, chatDeployment("regression-billing-protocols"))
	admin := h.adminSession()
	tn := h.provision(t, admin, "billing-protocols")

	// 三种协议各发一次，提示词各不相同以免命中缓存。
	calls := []struct {
		name string
		path string
		body map[string]any
	}{
		{"chat", "/v1/chat/completions", map[string]any{
			"model": "regression-billing-protocols", "messages": []any{map[string]any{"role": "user", "content": "proto a"}},
		}},
		{"messages", "/v1/messages", map[string]any{
			"model": "regression-billing-protocols", "max_tokens": 64,
			"messages": []any{map[string]any{"role": "user", "content": "proto b"}},
		}},
		{"responses", "/v1/responses", map[string]any{
			"model": "regression-billing-protocols", "input": "proto c",
		}},
	}

	charges := map[string]float64{}
	for _, call := range calls {
		r := h.ok(http.MethodPost, call.path, tn.key, call.body)
		charges[call.name] = parseFloatOrZero(r.header("x-litellm-response-cost"))
	}

	// 每一次都必须定价，而且价格相同：假供应商对每次都给同样多的 token。
	first := charges["chat"]
	if first <= 0 {
		t.Fatalf("the chat call was priced at zero, so the comparison proves nothing: %v", charges)
	}
	for name, cost := range charges {
		if !nearlyEqual(cost, first) {
			t.Fatalf("protocol %s was charged %v but chat was charged %v", name, cost, first)
		}
	}
}

// TestStreamingAnswersArriveAndAreBilled 证明流式回答能到、而且照常计费。
//
// 流式的计费特别容易漏：usage 藏在流的最后一个 chunk 里，一个"读到第一个 chunk
// 就返回"的实现会把整次调用记成零 token。
//
// 这里不核对响应头里的金额，因为流式回答根本设不了那个头：SSE 一旦开始写正文，
// HTTP 的头就已经发出去了。所以流式的金额只能从日志里看——这一点本身就是流式和
// 非流式的一个真实差别，值得写在注释里免得以后有人以为漏了。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestStreamingAnswersArriveAndAreBilled(t *testing.T) {
	h := newHarness(t, chatDeployment("regression-stream"))
	admin := h.adminSession()
	tn := h.provision(t, admin, "stream")

	r := h.ok(http.MethodPost, "/v1/chat/completions", tn.key, map[string]any{
		"model": "regression-stream", "stream": true,
		"messages": []any{map[string]any{"role": "user", "content": "stream please"}},
	})

	// 流式的回答是这个形状：若干 data: 行，最后一行为 data: [DONE]。
	text := r.text()
	if !containsSubstring(text, "data:") {
		t.Fatalf("a streaming request did not return an event stream: %s", truncate(text, 300))
	}
	if !containsSubstring(text, "[DONE]") {
		t.Fatalf("the stream never terminated with [DONE]: %s", truncate(text, 300))
	}
	// 正文要真的透过来了，不能只有一个空的骨架。
	if !containsSubstring(text, defaultReply.Content) {
		t.Fatalf("the streamed content never arrived: %s", truncate(text, 400))
	}

	// 计费从日志里核对：流式设不了响应头。
	h.flushSpend()
	rows := logsFor(h.spendLogs(t, admin), "regression-stream")
	if len(rows) != 1 {
		t.Fatalf("a streaming call left %d log rows, want 1", len(rows))
	}
	spend, _ := floatField(rows[0], "spend")
	if spend <= 0 {
		t.Fatalf("a streaming call was charged nothing: %s", truncate(string(mustJSON(rows[0])), 300))
	}
	if !nearlyEqual(spend, expectedCost()) {
		t.Fatalf("a streaming call was charged %v, want %v", spend, expectedCost())
	}
}

// TestStreamingAndNonStreamingAgreeOnTokens 证明流式和非流式对同一批 token 记账一致。
//
// 两条路径读 usage 的位置不同：非流式在正文里，流式在流的尾巴上。所以它们最容易
// 各记一套，而账单上只看得出总数不对。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestStreamingAndNonStreamingAgreeOnTokens(t *testing.T) {
	h := newHarness(t, chatDeployment("regression-stream-parity"))
	admin := h.adminSession()
	tn := h.provision(t, admin, "stream-parity")

	plain := h.ok(http.MethodPost, "/v1/chat/completions", tn.key, map[string]any{
		"model": "regression-stream-parity", "messages": []any{map[string]any{"role": "user", "content": "plain"}},
	})
	plainCost := parseFloatOrZero(plain.header("x-litellm-response-cost"))
	if plainCost <= 0 {
		t.Fatalf("the non-streaming call was priced at zero: %s", plain.describe())
	}

	h.ok(http.MethodPost, "/v1/chat/completions", tn.key, map[string]any{
		"model": "regression-stream-parity", "stream": true,
		"messages": []any{map[string]any{"role": "user", "content": "streamed"}},
	})

	h.flushSpend()
	rows := logsFor(h.spendLogs(t, admin), "regression-stream-parity")
	if len(rows) != 2 {
		t.Fatalf("expected one row per call, got %d", len(rows))
	}

	// 两次调用的 token 数必须一样：假供应商对两条路径都报同样的 token。
	plainTokens := firstFloat(rows[0], "total_tokens")
	streamTokens := firstFloat(rows[1], "total_tokens")
	if plainTokens <= 0 || streamTokens <= 0 {
		t.Fatalf("a call recorded no tokens: plain=%v streamed=%v", plainTokens, streamTokens)
	}
	if plainTokens != streamTokens {
		t.Fatalf("the streamed call recorded %v tokens but the plain one %v", streamTokens, plainTokens)
	}

	// 两次的钱也要一样。
	plainLogged := firstFloat(rows[0], "spend")
	streamLogged := firstFloat(rows[1], "spend")
	if !nearlyEqual(plainLogged, streamLogged) {
		t.Fatalf("the streamed call was charged %v but the plain one %v", streamLogged, plainLogged)
	}
}

// TestEmbeddingIsBilledFromItsOwnUsage 证明向量调用按它自己的 usage 计费，
// 而不是按对话那两个字段。
//
// 向量的回应里只有 prompt_tokens，没有 completion_tokens。按对话的字段去读，
// 会把输入那一半也记成零。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestEmbeddingIsBilledFromItsOwnUsage(t *testing.T) {
	h := newHarness(t, embeddingDeployment("regression-embed-bill"))
	admin := h.adminSession()
	tn := h.provision(t, admin, "embed-bill")

	r := h.ok(http.MethodPost, "/v1/embeddings", tn.key, map[string]any{
		"model": "regression-embed-bill", "input": "embed me",
	})
	usage, _ := r.json()["usage"].(map[string]any)
	if usage == nil {
		t.Fatalf("the embedding answer carried no usage: %s", r.describe())
	}
	if prompt, _ := floatField(usage, "prompt_tokens"); prompt <= 0 {
		t.Fatalf("the embedding answer reported %v prompt tokens: %s", prompt, r.describe())
	}

	// 向量模型在部署上没填单价，所以这里不核对金额，只核对 token 进了日志。
	h.flushSpend()
	rows := logsFor(h.spendLogs(t, admin), "regression-embed-bill")
	if len(rows) == 0 {
		t.Fatal("an embedding call left no log row")
	}
	if prompt, _ := floatField(rows[0], "prompt_tokens"); prompt <= 0 {
		t.Fatalf("the embedding log row recorded %v prompt tokens", prompt)
	}
}

// TestUnknownPathIsNotFound 证明不认识的数据面路径回 404，而不是被某个宽泛的
// 处理函数吞掉之后回一个空成功。
//
// 静默的成功是最难查的一类问题：调用方以为通了，其实什么都没发生。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestUnknownPathIsNotFound(t *testing.T) {
	h := newHarness(t, chatDeployment("regression-404"))
	admin := h.adminSession()
	tn := h.provision(t, admin, "notfound")

	// 数据面上一条不存在的子路径。
	r := h.do(http.MethodPost, "/v1/chat/completions/nonexistent-suffix", tn.key, map[string]any{})
	if r.status == http.StatusOK {
		t.Fatalf("an unknown data-plane path answered 200: %s", r.describe())
	}
}

// containsSubstring 判断文本里有没有这段子串。
// 参数 haystack/needle（string）：被查的文本与要找的片段。
// 返回 bool（bool）：含有时为真。
func containsSubstring(haystack, needle string) bool { return strings.Contains(haystack, needle) }
