package regression

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/gateway"
	"github.com/sunqirui1987/xhub/internal/iam"
	"github.com/sunqirui1987/xhub/internal/live"
	"github.com/sunqirui1987/xhub/internal/store"
	"github.com/sunqirui1987/xhub/internal/testsupport"
)

// 这个文件是整套回归测试的底座。每个测试用例都从 newHarness 起一个真实的网关，
// 后续所有断言都通过 h.do / h.ok 走 HTTP，不去直接调内部函数、也不直接查表——
// 唯一的例外在 seedCredential 和 putGuardrail 里，原因写在各自的注释上。
//
// 三条贯穿全文件的原则：
//
//  1. 除了上游供应商，边界上不放替身。真实路由、真实 PostgreSQL、真实密钥、
//     真实用量写入、真实额度判定、真实护栏。
//  2. 配置写在 Go 里，不读 configs/config.yaml。别人改自己的配置不该影响回归。
//  3. 断言要能"被感知"。只断言 200 的用例，在"网关把请求全发错地方"这种故障
//     下照样绿，所以这里会去核对上游收到的正文、上游被调了几次、日志里的钱和
//     token。

// upstreamReply 是假供应商回答的内容。它带 usage 块，这样网关的 token 统计
// 有真东西可读——否则"费用也和响应一致"这类断言就是空跑。
type upstreamReply struct {
	PromptTokens     int    // 提示 token 数，写进假响应的 usage
	CompletionTokens int    // 完成 token 数，写进假响应的 usage
	Content          string // 回答正文，用来核对是不是真的转回来了
}

// defaultReply 是每一次假补全的回答。数字选得小且固定，因为计费断言要拿它算钱。
var defaultReply = upstreamReply{PromptTokens: 11, CompletionTokens: 5, Content: "regression-ok"}

// 假供应商回答的字段名照抄真实上游，值可以是构造的。
//
// 字段名必须真：形状错了，用例就测不到真实路径上会发生的错。这里的两套形状是
// 从两家在跑的供应商实际抓下来的（2026-10 抓取）：
//
//	api.qnaigc.com/v1/chat/completions   OpenAI 形状，多两个 details 对象
//	api.fenno.ai/v1/messages             Anthropic 形状，用量在另一套字段上
//
// 从前这里对两条路径都回 OpenAI 形状。那正是"计量准不准"这件事一直没被测到的
// 原因：真实 Messages 上游报的是 input_tokens 加一笔独立的 cache_read_input_tokens，
// 而假上游报的是 prompt_tokens，于是整类解析错——缓存那一半被丢掉、剩下的一半
// 按缓存价收——在套件里根本无从发生。
//
// 值仍然由 defaultReply 固定，因为计费断言要拿它算钱；构造的是数字，不是形状。

// openAIUsage 是 OpenAI 兼容上游（qiniu 那一路）回的 usage。
//
// 真实回答里 prompt_tokens_details 和 completion_tokens_details 是存在的空对象，
// 缓存命中时会带 cached_tokens。空对象和缺键在解析上是两件事，所以这里照抄。
// 参数 override（map[string]any）：要替换成的 usage。空时用 defaultReply 的数字。
// 返回 map[string]any（map[string]any）：写进回答的 usage 对象。
func openAIUsage(override map[string]any) map[string]any {
	if override != nil {
		out := map[string]any{}
		for k, v := range override {
			out[k] = v
		}
		if _, set := out["total_tokens"]; !set {
			out["total_tokens"] = asIntAny(out["prompt_tokens"]) + asIntAny(out["completion_tokens"])
		}
		if _, set := out["prompt_tokens_details"]; !set {
			out["prompt_tokens_details"] = map[string]any{}
		}
		if _, set := out["completion_tokens_details"]; !set {
			out["completion_tokens_details"] = map[string]any{}
		}
		return out
	}
	return map[string]any{
		"prompt_tokens":             defaultReply.PromptTokens,
		"completion_tokens":         defaultReply.CompletionTokens,
		"total_tokens":              defaultReply.PromptTokens + defaultReply.CompletionTokens,
		"prompt_tokens_details":     map[string]any{},
		"completion_tokens_details": map[string]any{},
	}
}

// anthropicUsage 是 Anthropic 形状上游（fenno 那一路）回的 usage。
//
// 这一套字段名和 OpenAI 的不是一套：input_tokens 只是没命中缓存的那部分，
// cache_read_input_tokens 是**另外一笔**，cache_creation 是个嵌套对象，
// 还有 output_tokens_details、service_tier、inference_geo 这些计费不读但确实
// 在回答里的键。
//
// 参数 override（map[string]any）：要替换成的 usage。给了 input_tokens 这类键时按
// Anthropic 的语义读：input_tokens 不加缓存读，除非调用方自己写了缓存那一笔。
// 返回 map[string]any（map[string]any）：写进回答的 usage 对象。
func anthropicUsage(override map[string]any) map[string]any {
	if override != nil {
		out := map[string]any{}
		for k, v := range override {
			out[k] = v
		}
		if _, set := out["input_tokens"]; !set {
			out["input_tokens"] = 0
		}
		if _, set := out["output_tokens"]; !set {
			out["output_tokens"] = 0
		}
		if _, set := out["cache_creation_input_tokens"]; !set {
			out["cache_creation_input_tokens"] = 0
		}
		if _, set := out["cache_read_input_tokens"]; !set {
			out["cache_read_input_tokens"] = 0
		}
		out["cache_creation"] = map[string]any{
			"ephemeral_5m_input_tokens": 0, "ephemeral_1h_input_tokens": 0,
		}
		if _, set := out["output_tokens_details"]; !set {
			out["output_tokens_details"] = map[string]any{"thinking_tokens": 0}
		}
		if _, set := out["service_tier"]; !set {
			out["service_tier"] = "standard"
		}
		if _, set := out["inference_geo"]; !set {
			out["inference_geo"] = "global"
		}
		return out
	}
	return map[string]any{
		"input_tokens":                defaultReply.PromptTokens,
		"output_tokens":               defaultReply.CompletionTokens,
		"cache_creation_input_tokens": 0,
		"cache_read_input_tokens":     0,
		"cache_creation": map[string]any{
			"ephemeral_5m_input_tokens": 0, "ephemeral_1h_input_tokens": 0,
		},
		"output_tokens_details": map[string]any{"thinking_tokens": 0},
		"service_tier":          "standard",
		"inference_geo":         "global",
	}
}

// chatAnswer 是 OpenAI 兼容上游的一次补全回答，字段照抄 qiniu 的形状。
// 参数 model（string）：上游模型名；override（map[string]any）：要替换的 usage。
// 返回 map[string]any（map[string]any）：回答对象。
func chatAnswer(model string, override map[string]any) map[string]any {
	return map[string]any{
		"id": "chatcmpl-regression", "object": "chat.completion", "created": 1, "model": model,
		"choices": []any{map[string]any{
			"index": 0, "finish_reason": "stop",
			"message": map[string]any{"role": "assistant", "content": defaultReply.Content},
		}},
		"usage": openAIUsage(override),
	}
}

// anthropicAnswer 是 Anthropic 形状上游的一次问答回答，字段照抄 fenno 的形状。
// 参数 model（string）：上游模型名；override（map[string]any）：要替换的 usage。
// 返回 map[string]any（map[string]any）：回答对象。
func anthropicAnswer(model string, override map[string]any) map[string]any {
	return map[string]any{
		"id": "msg_regression", "type": "message", "role": "assistant", "model": model,
		"content":            []any{map[string]any{"type": "text", "text": defaultReply.Content, "citations": []any{}}},
		"stop_reason":        "end_turn",
		"stop_sequence":      nil,
		"stop_details":       nil,
		"context_management": map[string]any{"applied_edits": []any{}},
		"usage":              anthropicUsage(override),
	}
}

// harness 是一个已经跑起来的网关，外加测试需要用到的所有把手：往里发请求的
// httptest server、真身份库 db、框架记录库 store、假供应商、以及假供应商收到过
// 的每一次请求。
type harness struct {
	t       *testing.T
	server  *httptest.Server // 待测网关，测试通过它发 HTTP
	gw      *gateway.Server  // 同一个网关的内部句柄，用来调 FlushSpend 之类
	db      *iam.DB          // 真实身份库，私有 schema
	store   *store.Store     // 真实框架记录库，同一个 schema
	prices  *httptest.Server // 假供应商：所有部署都指向它
	replies []upstreamCall   // 假供应商收到过的请求，断言用
	master  string           // 本进程的 master key
	failing bool             // 置位后假供应商回 500，用来测失败路径
	mu      sync.Mutex       // 保护 replies、failing 和 rules，回退用例会并发打上游
	rules   map[string]upstreamRule
	live    bool // 上游是真实供应商。假服务器看不到这些请求，断言改看费用、日志和拒绝。
}

// upstreamRule 按上游看见的模型名改这一次的回答。status 大于等于 400 时直接回那个
// 状态码；hold 非空时先卡住，用来让另一条部署变成"更空闲"；before 在回写之前运行，
// 用来在第一次失败和换部署之间把额度花到顶。
type upstreamRule struct {
	status int
	hold   <-chan struct{}
	before func()
	// usage 非空时替换这一次回答里的 usage，用来制造缓存命中、秒数、图片数
	// 这些默认形状没有的计量。计费认的就是这些量。
	usage map[string]any
}

// upstreamCall 是假供应商收到的一次请求。测试拿它来证明两件事：
// 护栏在访问上游之前就把请求拦下了（这里应该是空的），以及 Bypass 按端点类型
// 的约定改写了正文（这里的 body 应该是改过的样子）。
type upstreamCall struct {
	Path string         // 上游收到的路径
	Body map[string]any // 上游收到的正文
}

// TestMain 把 gin 切到测试模式，避免每个用例都打一遍启动横幅。
func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	os.Exit(m.Run())
}

// newHarness 起一个网关，挂在私有 schema 上，背后放一个假供应商。
//
// 参数 models 是要装进模型表的部署。传进来的部署如果没写 api_base，会被统一
// 指向本地假供应商；写了的一样保留，这正好让 --live 那组真实调用可以覆盖掉。
//
// 配置写在代码里而不是读 configs/config.yaml，这样"测什么"由测试自己钉住：
// 开发者改自己的配置，不会改变回归跑的内容。
//
// 参数 t（*testing.T）：当前测试；models（...config.ModelEntry）：装进模型表的部署。
// 返回 *harness（*harness）：已经可以发请求和查结果的网关句柄。
func newHarness(t *testing.T, models ...config.ModelEntry) *harness {
	t.Helper()
	return openHarness(t, true, models...)
}

// openHarness 和 newHarness 一样，只是可以关掉花费日志里的请求正文。
// 产品默认不存正文，管理员开关能把它打开；套件里大多数用例要核对日志，所以 newHarness 仍然打开。
func openHarness(t *testing.T, storePrompts bool, models ...config.ModelEntry) *harness {
	t.Helper()
	h := &harness{t: t, master: "sk-regression-" + strconv.FormatInt(time.Now().UnixNano(), 36)}

	h.prices = httptest.NewServer(http.HandlerFunc(h.serveUpstream))
	t.Cleanup(h.prices.Close)

	// 没写 api_base 的部署会去连真实供应商。这里统一指向假供应商，这样请求
	// 一步都不出本机，断言考的是网关而不是供应商的可用性。
	for i := range models {
		if models[i].LiteLLMParams == nil {
			models[i].LiteLLMParams = map[string]any{}
		}
		if stringField(models[i].LiteLLMParams, "api_base") == "" {
			models[i].LiteLLMParams["api_base"] = h.prices.URL + "/v1"
		}
		if stringField(models[i].LiteLLMParams, "api_key") == "" {
			models[i].LiteLLMParams["api_key"] = "sk-fake-upstream"
		}
	}

	// 每个测试一个独立 schema，结束就删。所以这套测试和你正在跑的网关可以
	// 共用同一个数据库，不会互相看见对方的账号、密钥和用量。
	dsn := testsupport.Postgres(t, "regression")
	db, err := iam.Open(t.Context(), dsn)
	if err != nil {
		t.Fatalf("open iam: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Migrate(t.Context()); err != nil {
		t.Fatalf("migrate iam: %v", err)
	}
	h.db = db

	st, err := store.Open(dsn)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	if st.Engine != nil {
		t.Cleanup(func() { _ = st.Engine.Close() })
	}
	h.store = st

	// Redis 是可选的。设了 XHUB_REGRESSION_REDIS_URL 才走真实的 Redis
	// 热花费和限流路径；不设就走进程内滑窗，断言照样成立。
	cfg := &config.Config{
		ModelList:      models,
		RouterSettings: config.RouterSettings{RoutingStrategy: "simple-shuffle", NumRetries: 0, Timeout: 30},
		GeneralRaw:     map[string]any{},
		GeneralSettings: config.GeneralSettings{
			MasterKey:               h.master,
			DatabaseURL:             dsn,
			RedisURL:                os.Getenv("XHUB_REGRESSION_REDIS_URL"),
			StorePromptsInSpendLogs: storePrompts,
		},
	}
	for _, model := range models {
		var timeout float64
		switch value := model.LiteLLMParams["timeout"].(type) {
		case float64:
			timeout = value
		case int:
			timeout = float64(value)
		case string:
			timeout, _ = strconv.ParseFloat(value, 64)
		}
		if timeout > cfg.RouterSettings.Timeout {
			cfg.RouterSettings.Timeout = timeout
		}
	}
	h.gw = gateway.New(cfg, st, db)
	h.server = httptest.NewServer(h.gw.Handler())
	t.Cleanup(func() {
		// Wait for streaming handlers before draining their final events. Otherwise
		// the next private schema can ingest this harness's shared Redis queue.
		h.server.Close()
		h.flushSpend()
		_ = h.gw.Live.Close()
	})
	return h
}

// serveUpstream 是假供应商。它回答网关可能拨的每一条路径，并把这次请求记下来，
// 好让测试断言"到底发出去了什么"。
//
// 它刻意只认识路径、不认识供应商：chat 按 OpenAI 形状回，embedding 回向量，
// contents/generations 回一个任务 id（Seedance 那形状，用量要等查询才有）。
//
// 参数 w（http.ResponseWriter）：写响应；r（*http.Request）：上游收到的请求。
// 返回：无。响应写进 w，请求记进 h.replies。
func (h *harness) serveUpstream(w http.ResponseWriter, r *http.Request) {
	raw, _ := io.ReadAll(r.Body)
	var body map[string]any
	_ = json.Unmarshal(raw, &body)

	model := "regression-model"
	if s, ok := body["model"].(string); ok && s != "" {
		model = s
	}
	h.mu.Lock()
	h.replies = append(h.replies, upstreamCall{Path: r.URL.Path, Body: body})
	rule := h.rules[model]
	failing := h.failing
	h.mu.Unlock()
	if rule.before != nil {
		rule.before()
	}
	if rule.hold != nil {
		<-rule.hold
	}
	w.Header().Set("Content-Type", "application/json")

	// 按模型名注入的状态优先于全局失败开关，这样同一对外名下的两条部署可以一个失败、一个成功。
	if rule.status >= 400 {
		w.WriteHeader(rule.status)
		writeJSON(w, map[string]any{"error": map[string]any{"message": "regression upstream status", "type": "api_error"}})
		return
	}
	// 失败注入：用来证明失败的调用会记账、但不扣费。
	if failing {
		w.WriteHeader(http.StatusInternalServerError)
		writeJSON(w, map[string]any{"error": map[string]any{"message": "regression upstream failure", "type": "api_error"}})
		return
	}

	// 流式：真的按 SSE 回，因为流式的计费和形状是另一条路径——
	// 一个只回 JSON 的假供应商会让流式用例假通过。两条协议的事件形状不同，
	// 所以按上游路径分流，和真实上游一样。
	if stream, _ := body["stream"].(bool); stream {
		h.serveStream(w, model, strings.Contains(r.URL.Path, "/messages"), rule.usage)
		return
	}

	switch {
	case strings.Contains(r.URL.Path, "/embeddings"):
		writeJSON(w, map[string]any{
			"object": "list", "model": model,
			"data":  []any{map[string]any{"object": "embedding", "index": 0, "embedding": []float64{0.1, 0.2}}},
			"usage": map[string]any{"prompt_tokens": 4, "total_tokens": 4},
		})
	case strings.Contains(r.URL.Path, "contents/generations"):
		// Seedance 的形状：先给一个任务 id，还没有用量。用量要等查询那一次。
		writeJSON(w, map[string]any{"id": "regression-task-1", "status": "queued"})
	case strings.Contains(r.URL.Path, "/messages"):
		// Messages 端点的上游是 Anthropic 形状，它的用量字段和 OpenAI 的不是一套。
		writeJSON(w, anthropicAnswer(model, rule.usage))
	default:
		writeJSON(w, chatAnswer(model, rule.usage))
	}
}

// serveStream 按 SSE 回一次流式补全。分几个 chunk 发出去，最后补一个带 usage 的
// chunk 和终止行——真实供应商就是这么发的，而 usage 只出现在流的尾巴上，
// 这正是流式计费容易漏掉的地方。
//
// 两种上游的流是两种协议，所以分流。Anthropic 的流是一条事件序列
// （message_start / content_block_delta / message_delta / message_stop），
// usage 在 message_start 里先报一次、在 message_delta 里带上最终输出数再报一次；
// 计费要认的是后一次那个数。OpenAI 的流是 data: 行，usage 只在最后一个 chunk 上。
//
// 参数 w（http.ResponseWriter）：写响应；model（string）：本次请求的模型名；
// anthropic（bool）：按 Anthropic 事件流回，还是按 OpenAI data: 行回；
// override（map[string]any）：要替换的 usage。空时用 defaultReply 的数字。
// 返回：无。
func (h *harness) serveStream(w http.ResponseWriter, model string, anthropic bool, override map[string]any) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	flusher, _ := w.(http.Flusher)
	flush := func() {
		if flusher != nil {
			flusher.Flush()
		}
	}
	if anthropic {
		h.serveAnthropicStream(w, model, override, flush)
		return
	}

	chunks := []map[string]any{
		{"id": "chatcmpl-regression", "object": "chat.completion.chunk", "created": 1, "model": model,
			"choices": []any{map[string]any{"index": 0, "delta": map[string]any{"role": "assistant"}, "finish_reason": nil}}},
		{"id": "chatcmpl-regression", "object": "chat.completion.chunk", "created": 1, "model": model,
			"choices": []any{map[string]any{"index": 0, "delta": map[string]any{"content": defaultReply.Content}, "finish_reason": nil}}},
	}
	for _, chunk := range chunks {
		_, _ = io.WriteString(w, "data: "+string(mustJSON(chunk))+"\n\n")
		flush()
	}

	// 最后一个 chunk 带 usage，和真实供应商一样。
	final := map[string]any{
		"id": "chatcmpl-regression", "object": "chat.completion.chunk", "created": 1, "model": model,
		"choices": []any{map[string]any{"index": 0, "delta": map[string]any{}, "finish_reason": "stop"}},
		"usage":   openAIUsage(override),
	}
	_, _ = io.WriteString(w, "data: "+string(mustJSON(final))+"\n\n")
	_, _ = io.WriteString(w, "data: [DONE]\n\n")
	flush()
}

// serveAnthropicStream 按 Anthropic 的事件序列回一次流。
//
// 顺序和字段名照抄真实上游：ping、message_start（先报一次输入用量）、
// content_block_start、content_block_delta、content_block_stop、
// message_delta（带最终输出用量）、message_stop。
//
// 两边都报用量是有意的：中间那一版 message_delta 的 usage 里 output_tokens 还是
// 很小的数，最终那一版才是完整的。只认第一次拿到的 usage 会少记输出 token。
//
// 参数 w（http.ResponseWriter）：写响应；model（string）：本次请求的模型名；
// override（map[string]any）：要替换的 usage；flush（func()）：把这一段推给调用方。
// 返回：无。
func (h *harness) serveAnthropicStream(w http.ResponseWriter, model string, override map[string]any, flush func()) {
	event := func(name string, doc map[string]any) {
		_, _ = io.WriteString(w, "event: "+name+"\ndata: "+string(mustJSON(doc))+"\n\n")
		flush()
	}
	event("ping", map[string]any{"type": "ping"})

	// 第一版用量：输入已经定了，输出只有第一个 token。
	first := anthropicUsage(override)
	event("message_start", map[string]any{
		"type": "message_start",
		"message": map[string]any{
			"model": model, "id": "msg_regression", "type": "message", "role": "assistant",
			"content": []any{}, "stop_reason": nil, "stop_sequence": nil, "stop_details": nil,
			"usage": first,
		},
	})
	event("content_block_start", map[string]any{
		"type": "content_block_start", "index": 0,
		"content_block": map[string]any{"type": "text", "text": ""},
	})
	event("content_block_delta", map[string]any{
		"type": "content_block_delta", "index": 0,
		"delta": map[string]any{"type": "text_delta", "text": defaultReply.Content},
	})
	event("content_block_stop", map[string]any{"type": "content_block_stop", "index": 0})

	// 最终用量：输出这一侧到这一刻才是完整的。
	final := anthropicUsage(override)
	event("message_delta", map[string]any{
		"type": "message_delta",
		"delta": map[string]any{
			"stop_reason": "end_turn", "stop_sequence": nil, "stop_details": nil,
		},
		"usage":              final,
		"context_management": map[string]any{"applied_edits": []any{}},
	})
	event("message_stop", map[string]any{"type": "message_stop"})
}

// upstreamCalls 返回假供应商收到过的请求的副本。返回副本而不是切片本身，是为了
// 让调用方拿到之后不会因为后续请求追加而被改掉。
// 参数：无。返回 []upstreamCall（[]upstreamCall）：到此为止收到的请求。
func (h *harness) upstreamCalls() []upstreamCall {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]upstreamCall{}, h.replies...)
}

// resetUpstream 忘掉已经记录的上游请求，让后面的断言只看这之后发生的调用。
// 参数：无。返回：无。只清空记录，不影响已经发出去的请求。
func (h *harness) resetUpstream() {
	h.mu.Lock()
	h.replies = nil
	h.mu.Unlock()
}

// failUpstream 让假供应商改回 500，用来测"失败的调用记了账但没扣费"。
// 参数 fail（bool）：true 表示回 500；false 表示恢复正常回答。
// 返回：无。只改假供应商的行为。
func (h *harness) failUpstream(fail bool) {
	h.mu.Lock()
	h.failing = fail
	h.mu.Unlock()
}

// scriptStatus 让某个上游模型固定回一个 HTTP 状态。0 表示恢复成正常的 200。
// 模型名是上游看见的那段，不含 openai/ 前缀。
func (h *harness) scriptStatus(model string, status int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.rules == nil {
		h.rules = map[string]upstreamRule{}
	}
	rule := h.rules[model]
	rule.status = status
	h.rules[model] = rule
}

// onUpstream 在该模型回写之前运行 fn。用来在第一次失败之后、换下一条部署之前把额度花到顶。
func (h *harness) onUpstream(model string, fn func()) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.rules == nil {
		h.rules = map[string]upstreamRule{}
	}
	rule := h.rules[model]
	rule.before = fn
	h.rules[model] = rule
}

// holdUpstream 卡住这个上游模型，直到调用返回的 release。卡住期间这条部署算作在途，
// least-busy 会去选另一条。
func (h *harness) holdUpstream(model string) (release func()) {
	ch := make(chan struct{})
	h.mu.Lock()
	if h.rules == nil {
		h.rules = map[string]upstreamRule{}
	}
	rule := h.rules[model]
	rule.hold = ch
	h.rules[model] = rule
	h.mu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			h.mu.Lock()
			rule := h.rules[model]
			rule.hold = nil
			h.rules[model] = rule
			h.mu.Unlock()
			close(ch)
		})
	}
}

// usageOverride 让某个上游模型回一段指定的 usage，用来测缓存命中和按秒按张的计量。
//
// 假供应商默认回固定的 token 数，那是绝大多数用例需要的形状。计费还认别的量：
// 缓存命中数、秒数、图片数、搜索次数。这些量只有上游会报，所以要让上游改口。
// model 是上游看见的那段名字，不含 openai/ 前缀。
// 参数 model（string）：上游模型名；usage（map[string]any）：要回的 usage 对象；
// 返回：无。nil 表示恢复默认。
func (h *harness) usageOverride(model string, usage map[string]any) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.rules == nil {
		h.rules = map[string]upstreamRule{}
	}
	rule := h.rules[model]
	rule.usage = usage
	h.rules[model] = rule
}

// addFlatPricedModel 通过控制台加一条**用扁平单价字段**定价的部署，返回它的公开名。
//
// 这是控制台真正写出来的形状：单价表单一个格子一个字段，提交的是
// input_cost_per_token、output_cost_per_second 这样的键，而不是一个 rates 数组。
// 所以计费路径上"部署自带价"这一支在真实部署里走的是扁平字段。
//
// 用扁平形式加模型而不是只用 rates，正是为了证明运维在界面上填的价真的被读到：
// 只测 rates 的话，扁平分支断掉了测试也照样绿。
//
// 参数 t（*testing.T）：当前测试；h（*harness）：当前用例的网关；admin（string）：管理员的会话令牌；
// public（string）：公开模型名；params（map[string]any）：要写进 litellm_params 的单价字段；mode（string）：调用方式。
// 返回：无。
func addFlatPricedModel(t *testing.T, h *harness, admin, public string, params map[string]any, mode string) {
	t.Helper()
	litellm := map[string]any{
		"model":               "openai/" + public,
		"api_key":             "sk-fake-upstream",
		"api_base":            h.credentialAPIBase(),
		"custom_llm_provider": "openai",
	}
	for k, v := range params {
		litellm[k] = v
	}
	h.ok(http.MethodPost, "/model/new", admin, map[string]any{
		"model_name":     public,
		"litellm_params": litellm,
		"model_info":     map[string]any{"transport": "adapted", "endpoint_types": []string{mode}},
	})
}

// patchRates 改一条已登记部署的费率表，走控制台的模型更新接口。
//
// 用它而不是直接改内存里的配置，是因为价格在部署上是 litellm_params 的一个键，
// 控制台写进去和配置文件写进去必须被同一段代码读到。直接改内存会绕过这条路径，
// 用例就证明不了"运维在界面上改价会生效"。
//
// 更新接口按部署 id 认行，同名多部署时 id 是唯一的区分方式，所以先把 id 查出来。
// 参数 t（*testing.T）：当前测试；admin（string）：管理员的会话令牌；
// public（string）：公开模型名；rates（[]any）：新的费率表；extra（map[string]any）：要一起写回的其它参数。
// 返回：无。
func (h *harness) patchRates(t *testing.T, admin, public string, rates []any, extra map[string]any) {
	t.Helper()
	id := h.deploymentID(t, admin, public)
	params := map[string]any{
		"model": public, "api_key": "sk-fake", "custom_llm_provider": "openai",
		"rates": rates,
	}
	for k, v := range extra {
		params[k] = v
	}
	h.ok(http.MethodPost, "/model/update", admin, map[string]any{
		"model_name":     public,
		"litellm_params": params,
		"model_info":     map[string]any{"id": id, "transport": "adapted", "endpoint_types": []string{"chat"}},
	})
}

// deploymentID 查一条部署的 id。控制台的更新和删除都按这个 id 认行。
// 参数 t（*testing.T）：当前测试；admin（string）：管理员的会话令牌；public（string）：公开模型名。
// 返回 string（string）：这一行的部署 id。
func (h *harness) deploymentID(t *testing.T, admin, public string) string {
	t.Helper()
	body := h.ok(http.MethodGet, "/v2/model/info?page=1&size=200", admin, nil).json()
	for _, row := range listField(body, "data") {
		if strField(row, "model_name") != public {
			continue
		}
		info, _ := row["model_info"].(map[string]any)
		if id := strField(info, "id"); id != "" {
			return id
		}
		// YAML 里声明、没落库的部署没有 id。这种行不能用控制台改，调用方
		// 需要知道这一点，否则会拿到一个看不懂的 400。
		t.Fatalf("deployment %s is declared in config and cannot be edited from the console", public)
	}
	t.Fatalf("no deployment named %s", public)
	return ""
}

// strField 读一个字符串字段。参数 m（map[string]any）：要读的对象；key（string）：字段名。
// 返回 string（string）：去掉空白的文本。缺失或不是字符串时为空串。
func strField(m map[string]any, key string) string {
	s, _ := m[key].(string)
	return strings.TrimSpace(s)
}

// answerUsage 是这一次补全回答里的 usage。没有覆盖时用固定的默认值：
// 提示 11、完成 5，整个套件的费用断言都按这两个数算。
// 参数 override（map[string]any）：这一次要替换成的 usage。为空时用默认值。
// 返回 map[string]any（map[string]any）：写进回答的 usage 对象。
func answerUsage(override map[string]any) map[string]any {
	if override != nil {
		out := map[string]any{}
		for k, v := range override {
			out[k] = v
		}
		if _, set := out["total_tokens"]; !set {
			out["total_tokens"] = asIntAny(out["prompt_tokens"]) + asIntAny(out["completion_tokens"])
		}
		return out
	}
	return map[string]any{
		"prompt_tokens":     defaultReply.PromptTokens,
		"completion_tokens": defaultReply.CompletionTokens,
		"total_tokens":      defaultReply.PromptTokens + defaultReply.CompletionTokens,
	}
}

// asIntAny 从 usage 里读一个整数，读到别的类型时算 0。
// 参数 v（any）：usage 里的一个值。
// 返回 int（int）：读到的整数。不是数字时为 0。
func asIntAny(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	default:
		return 0
	}
}

// spendLogs 读控制台看的那一页日志。这里读的是控制台真正调的接口，而不是直接
// 查表：如果哪天把控制台读取改坏了，这个用例会红，直接查表就发现不了。
// 参数 t（*testing.T）：当前测试；admin（string）：管理员的会话令牌。
// 返回 []map[string]any（[]map[string]any）：日志页里的每一行。
func (h *harness) spendLogs(t *testing.T, admin string) []map[string]any {
	t.Helper()
	// 先把还压在 Redis 里的花费刷进库，否则刚发生的那次调用可能还没落盘。
	h.flushSpend()
	body := h.ok(http.MethodGet, "/spend/logs/ui?page=1&page_size=200", admin, nil).json()
	rows := listField(body, "data", "logs")
	if rows == nil {
		t.Fatalf("the log page carried no rows: %s", truncate(string(mustJSON(body)), 300))
	}
	return rows
}

// mustJSON 把对象渲染成 JSON，只用来拼失败信息，不参与断言。
// 参数 v（any）：要渲染的对象。返回 []byte（[]byte）：JSON 字节；序列化失败时为空。
func mustJSON(v any) []byte {
	raw, _ := json.Marshal(v)
	return raw
}

// reply 是一次 HTTP 响应。头和正文都留着，因为网关有好几项承诺只写在头里：
// 这一次扣了多少钱、这一次调用的 call id、以及这次是不是命中缓存。
type reply struct {
	status  int         // HTTP 状态码
	body    []byte      // 响应正文原文
	headers http.Header // 响应头，计费、call id、缓存标记都在这里
}

// json 把正文解成对象。正文不是对象时返回空表而不是让测试失败——真正有用的
// 断言是紧接着的那一句，让这里先炸掉只会盖住它。
// 参数：无。返回 map[string]any（map[string]any）：解出来的对象；解不出来时为空表。
func (r reply) json() map[string]any {
	var out map[string]any
	_ = json.Unmarshal(r.body, &out)
	if out == nil {
		out = map[string]any{}
	}
	return out
}

// text 返回响应正文原文，用于拼失败信息。
// 参数：无。返回 string（string）：原始正文。
func (r reply) text() string { return string(r.body) }

// do 发一次 JSON 请求。token 为空时不带 Authorization 头，正好用来测未登录。
// 参数 method/path（string）：方法与路径；token（string）：会话或密钥，空串表示不带头；
// body（any）：请求体，nil 表示不发正文。
// 返回 reply（reply）：状态码、正文和响应头。
func (h *harness) do(method, path, token string, body any) reply {
	return h.doHeaders(method, path, token, body, nil)
}

// doHeaders 和 do 一样，额外带上调用方要钉住的头，例如会话粘滞用的 X-Session-Id。
func (h *harness) doHeaders(method, path, token string, body any, headers map[string]string) reply {
	h.t.Helper()
	var reader io.Reader
	if body != nil {
		// Business live calls should stay small even when the caller is one of the
		// broader workflow tests rather than the pricing-specific helpers.
		// Simulated requests keep their original body exactly.
		if h.live && strings.HasPrefix(path, "/v1/") {
			if object, ok := body.(map[string]any); ok {
				copyBody := make(map[string]any, len(object)+1)
				for k, v := range object {
					copyBody[k] = v
				}
				if _, exists := copyBody["max_tokens"]; !exists {
					copyBody["max_tokens"] = 16
				}
				body = copyBody
			}
		}
		raw, err := json.Marshal(body)
		if err != nil {
			h.t.Fatalf("marshal body for %s %s: %v", method, path, err)
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, h.server.URL+path, reader)
	if err != nil {
		h.t.Fatalf("build request %s %s: %v", method, path, err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	client := *h.server.Client()
	client.Timeout = 60 * time.Second
	if h.live {
		client.Timeout = 110 * time.Second
	}
	resp, err := client.Do(req)
	if err != nil {
		h.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		h.t.Fatalf("read response %s %s: %v", method, path, err)
	}
	return reply{status: resp.StatusCode, body: raw, headers: resp.Header.Clone()}
}

// ok 发一次请求，非 2xx 就让测试失败。用在"这一步本来就该成功"的地方：失败时
// 直接把响应贴出来，省得下一步断言报一个不知所云的错。
// 参数 method/path（string）：方法与路径；token（string）：会话或密钥；body（any）：请求体。
// 返回 reply（reply）：响应。非 2xx 时已经让当前测试失败。
func (h *harness) ok(method, path, token string, body any) reply {
	h.t.Helper()
	r := h.do(method, path, token, body)
	if r.status < 200 || r.status >= 300 {
		h.t.Fatalf("%s %s -> %d: %s", method, path, r.status, r.text())
	}
	return r
}

// adminSession 确保平台管理员存在并登录，返回它的会话令牌。
// 会话走真实的 /v2/login，刻意不用 master key：master key 只能进引导和应急路径，
// 不是账号密码。
// 参数：无。返回 string（string）：管理员的会话令牌。
func (h *harness) adminSession() string {
	h.t.Helper()
	const (
		email    = "regression-admin@example.com"
		password = "regression-admin-password"
	)
	if _, err := h.db.EnsureAdmin(h.t.Context(), email, "Regression Admin", password); err != nil {
		h.t.Fatalf("seed admin: %v", err)
	}
	return h.login(email, password)
}

// login 登录并返回会话令牌。登录响应里 "token" 是 JWT、"key" 是会话 id；
// 数据面和管理路由都认 JWT，所以优先取它。
// 参数 username/password（string）：账号与密码。返回 string（string）：会话令牌。
func (h *harness) login(username, password string) string {
	h.t.Helper()
	r := h.ok(http.MethodPost, "/v2/login", "", map[string]any{"username": username, "password": password})
	body := r.json()
	for _, field := range []string{"token", "key"} {
		if s, ok := body[field].(string); ok && s != "" {
			return s
		}
	}
	h.t.Fatalf("login returned no token: %s", r.text())
	return ""
}

// writeJSON 是假供应商用的编码器。
// 参数 w（http.ResponseWriter）：写响应；body（any）：要写的内容。返回：无。
func writeJSON(w http.ResponseWriter, body any) {
	raw, _ := json.Marshal(body)
	_, _ = w.Write(raw)
}

// spentOver 一行说明某个范围的额度已经用尽：上限是多少，就已经花了多少。
// 参数 ceiling（float64）：这个范围的上限。返回 float64（float64）：原样返回上限，
// 表示"已经达到上限"，因为判定是 spent + hot >= ceiling。
func spentOver(ceiling float64) float64 { return ceiling }

// stringField 从解出来的对象里读一个去掉首尾空白的字符串。
// 参数 body（map[string]any）：对象；key（string）：字段名。
// 返回 string（string）：去掉空白后的文本；缺字段或不是字符串时为空串。
func stringField(body map[string]any, key string) string {
	s, _ := body[key].(string)
	return strings.TrimSpace(s)
}

// floatField 从解出来的对象里读一个小数，同时接受 JSON 表示整数时用的写法。
// 参数 body（map[string]any）：对象；key（string）：字段名。
// 返回 float64（float64）：读到的数；bool（bool）：这个字段是数字时为真。
func floatField(body map[string]any, key string) (float64, bool) {
	switch v := body[key].(type) {
	case float64:
		return v, true
	case int:
		return float64(v), true
	case json.Number:
		f, err := v.Float64()
		return f, err == nil
	default:
		return 0, false
	}
}

// listField 按字段名取出嵌套的列表，容忍控制台接口用过的几种信封写法。
// 参数 body（map[string]any）：对象；keys（...string）：按优先顺序尝试的字段名。
// 返回 []map[string]any（[]map[string]any）：列表里的每一行；都没有时返回 nil。
func listField(body map[string]any, keys ...string) []map[string]any {
	for _, key := range keys {
		items, ok := body[key].([]any)
		if !ok {
			continue
		}
		out := make([]map[string]any, 0, len(items))
		for _, item := range items {
			if row, ok := item.(map[string]any); ok {
				out = append(out, row)
			}
		}
		return out
	}
	return nil
}

// rowsOf 取出响应里的行，同时接受两种信封：带字段名的（{teams: [...]}）和
// 裸数组（[...]）。控制台接口两种都在用，例如 /team/list 一直返回裸数组，
// 而 /v2/team/list 返回带 total 和分页的对象。
// 参数 r（reply）：要读的响应；keys（...string）：带字段名信封下按优先顺序尝试的字段名。
// 返回 []map[string]any（[]map[string]any）：解出来的行；两种形状都不是时返回 nil。
func rowsOf(r reply, keys ...string) []map[string]any {
	body := r.json()
	if rows := listField(body, keys...); rows != nil {
		return rows
	}
	// 裸数组：整体解一次。
	var items []any
	if json.Unmarshal(r.body, &items) != nil {
		return nil
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if row, ok := item.(map[string]any); ok {
			out = append(out, row)
		}
	}
	return out
}

// findBy 返回第一个该字段等于 want 的行。
// 参数 rows（[]map[string]any）：候选行；field（string）：要比的字段；want（string）：期望值。
// 返回 map[string]any（map[string]any）：命中的那一行；没有时返回 nil。
func findBy(rows []map[string]any, field, want string) map[string]any {
	for _, row := range rows {
		if stringField(row, field) == want {
			return row
		}
	}
	return nil
}

// namesOf 把每一行的某个字符串字段收集起来并排序，用来做集合级断言。
// 参数 rows（[]map[string]any）：候选行；field（string）：要收集的字段。
// 返回 []string（[]string）：排好序的字段值，跳过空值。
func namesOf(rows []map[string]any, field string) []string {
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		if name := stringField(row, field); name != "" {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

// liveVendor 是一家要真连的供应商，从环境变量发现，不写死在代码里。
//
// 为什么是环境变量而不是一张 Go 里的表：供应商会一直加，而且加一家不该需要改
// 回归代码。每多一家，运维只要多设一组变量，套件自己就会把它带上跑。写死的表
// 每加一家都要动代码，于是"没覆盖到"和"没人改代码"变成同一件事。
//
// 约定（<ID> 大写，非字母数字一律换成下划线）：
//
//	XHUB_REGRESSION_<ID>_KEY    必需。缺了这家就整组跳过，不静默当成通过
//	XHUB_REGRESSION_<ID>_BASE   必需。chat 部署的根地址，OpenAI 兼容的带 /v1
//	XHUB_REGRESSION_<ID>_MODELS 必需。逗号分隔的模型名，必须是真实供应商接受的名称
//	XHUB_REGRESSION_<ID>_PROTOCOL 可选。anthropic 或 openai，缺省 openai
//	XHUB_REGRESSION_<ID>_BYPASS_BASE 可选。Bypass 端点要的裸主机名，缺省由 BASE 去掉 /v1
//
// 例：
//
//	XHUB_REGRESSION_FENNO_KEY=sk-...
//	XHUB_REGRESSION_FENNO_BASE=https://api.fenno.ai
//	XHUB_REGRESSION_FENNO_PROTOCOL=anthropic
//	XHUB_REGRESSION_FENNO_MODELS=claude-haiku-4-5
//	XHUB_REGRESSION_QINIU_KEY=sk-...
//	XHUB_REGRESSION_QINIU_BASE=https://api.qnaigc.com/v1
//	XHUB_REGRESSION_QINIU_MODELS=deepseek-v3,kimi-k2
type liveVendor struct {
	// ID is the variable-name stem, e.g. FENNO. It is also what the deployment's
	// public model name is prefixed with, so a log row names its supplier.
	ID string
	// Key is the credential. Never read from a file, only from the environment.
	Key string
	// Base is the chat root. The protocol adapter appends the operation path.
	Base string
	// BypassBase is the bare host for bypass endpoints, which carry their own path.
	BypassBase string
	// Protocol is "anthropic" or "openai"; it decides which upstream path and which
	// usage field names the vendor answers with.
	Protocol string
	// Models are the models to exercise against this vendor.
	Models []string
}

// liveVendorEnvPrefix is the stem every variable shares.
const liveVendorEnvPrefix = "XHUB_REGRESSION_"

// liveVendors discovers the vendors to test against from the environment.
//
// A vendor with no key is left out rather than failing the run: the suite has to
// be usable with one vendor configured. A key with no base is a configuration
// mistake and fails loudly, because a half-configured vendor would otherwise
// look like a vendor that passes.
//
// 参数 t（*testing.T）：当前测试。
// 返回 []liveVendor（[]liveVendor）：配置完整的供应商，按 ID 排序。没有时为空。
// 调用：本文件的 live 用例和 pricing_live_test.go。
// 测试：由 live 那两条用例直接覆盖。
func liveVendors(t *testing.T) []liveVendor {
	t.Helper()
	ids := liveVendorIDs()
	vendors := make([]liveVendor, 0, len(ids))
	for _, id := range ids {
		key := strings.TrimSpace(os.Getenv(liveVendorEnvPrefix + id + "_KEY"))
		if key == "" {
			continue
		}
		base := strings.TrimSpace(os.Getenv(liveVendorEnvPrefix + id + "_BASE"))
		if base == "" {
			t.Fatalf("%s_KEY is set but %s_BASE is not; a vendor needs both its credential and its base URL",
				liveVendorEnvPrefix+id, liveVendorEnvPrefix+id)
		}
		models := splitList(os.Getenv(liveVendorEnvPrefix + id + "_MODELS"))
		if len(models) == 0 {
			t.Fatalf("%s_KEY is set but %s_MODELS is empty; a live vendor needs at least one model to exercise",
				liveVendorEnvPrefix+id, liveVendorEnvPrefix+id)
		}
		bypass := strings.TrimSpace(os.Getenv(liveVendorEnvPrefix + id + "_BYPASS_BASE"))
		if bypass == "" {
			bypass = strings.TrimSuffix(strings.TrimSuffix(base, "/"), "/v1")
		}
		protocol := strings.ToLower(strings.TrimSpace(os.Getenv(liveVendorEnvPrefix + id + "_PROTOCOL")))
		if protocol == "" {
			protocol = "openai"
		}
		if protocol != "openai" && protocol != "anthropic" {
			t.Fatalf("%s_PROTOCOL must be openai or anthropic", liveVendorEnvPrefix+id)
		}
		vendors = append(vendors, liveVendor{
			ID: id, Key: key, Base: base, BypassBase: bypass,
			Protocol: protocol, Models: models,
		})
	}
	if len(vendors) == 0 {
		t.Fatalf("live mode is on but no vendor is configured; set %s<ID>_KEY, _BASE and _MODELS",
			liveVendorEnvPrefix)
	}
	return vendors
}

// liveVendorIDs lists the vendor stems the environment mentions.
//
// It reads the *_KEY variables rather than the *_BASE ones, because the key is
// what makes a vendor callable; discovering from the base would pick up a vendor
// with no credential and then fail on a missing key.
// 参数：无。
// 返回 []string（[]string）：变量名里出现的供应商标识，排好序。
// 调用：liveVendors。
// 测试：由 live 那两条用例直接覆盖。
func liveVendorIDs() []string {
	suffix := "_KEY"
	seen := map[string]bool{}
	for _, entry := range os.Environ() {
		name, _, ok := strings.Cut(entry, "=")
		if !ok || !strings.HasPrefix(name, liveVendorEnvPrefix) || !strings.HasSuffix(name, suffix) {
			continue
		}
		id := strings.TrimSuffix(strings.TrimPrefix(name, liveVendorEnvPrefix), suffix)
		if id == "" {
			continue
		}
		seen[id] = true
	}
	out := make([]string, 0, len(seen))
	for id := range seen {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// splitList splits a comma-separated variable into trimmed, non-empty entries.
// 参数 raw（string）：变量原文，例如 "deepseek-v3, kimi-k2"。
// 返回 []string（[]string）：去掉空白和空项之后的条目。
// 调用：liveVendors。
// 测试：由 live 那两条用例直接覆盖。
func splitList(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

// liveEnabled 报告这次要不要调真实供应商。默认关着，理由有三个：花钱、受供应商
// 抖动影响、以及"别人家服务挂了"这种事教不会我们任何东西。
// 参数：无。返回 bool（bool）：环境变量打开时为真。
func liveEnabled() bool { return liveModeEnabled() }

// liveCredentials 是开着 live 模式时的一次前置检查。
//
// 供应商从环境里发现（见 liveVendor），所以这里不再逐个点名。它保留下来是因为
// 现有的 live 用例需要一个"开了 live 却没配任何供应商就失败"的入口——静默跳过
// 看起来和通过一模一样。
// 参数 t（*testing.T）：当前测试。
// 返回 []liveVendor（[]liveVendor）：配置完整的供应商。未开 live 模式时整个用例跳过。
func liveCredentials(t *testing.T) []liveVendor {
	t.Helper()
	if !liveEnabled() {
		t.Skip("live vendor calls are off; set XHUB_REGRESSION_LIVE=1 to enable")
	}
	return liveVendors(t)
}

// recordSpendFor 直接写一条用量记录。额度测试用它把某个范围的花费预置到位，
// 而不是真去发一百次贵请求。
//
// 它走的是网关自己用的 iam.RecordUsage，所以额度判定读到的那些计数，和一次真实
// 调用会推动的计数完全是同一个。
//
// 参数 rec（iam.UsageRecord）：要写入的用量。空着的字段会填上不影响断言的默认值。
// 返回：无。写库失败时让当前测试失败。
func (h *harness) recordSpendFor(rec iam.UsageRecord) {
	h.t.Helper()
	if rec.RequestID == "" {
		rec.RequestID = "seed-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	if rec.Model == "" {
		rec.Model = "regression-seed"
	}
	if rec.CallType == "" {
		rec.CallType = "chat"
	}
	if rec.Status == "" {
		rec.Status = "success"
	}
	if rec.TS.IsZero() {
		rec.TS = time.Now().UTC()
	}
	if err := h.db.RecordUsage(h.t.Context(), []iam.UsageRecord{rec}); err != nil {
		h.t.Fatalf("seed usage %s: %v", rec.RequestID, err)
	}
}

// flushSpend 把还压在 Redis 里的花费推进 PostgreSQL，这样测试不用等后台的
// 刷盘循环就能读到自己刚刚造成的那一行。
// 参数：无。返回：无。没有 Redis 时它什么都不用做。
func (h *harness) flushSpend() {
	h.t.Helper()
	h.gw.FlushSpend()
}

// credentialAPIBase 是这套测试的假供应商地址。从内置目录加进来的部署会被指向
// 它，而不是指向真实供应商。
// 参数：无。返回 string（string）：假供应商的 /v1 根地址。
func (h *harness) credentialAPIBase() string { return h.prices.URL + "/v1" }

// seedCredential 写入一条供应商凭据，内容就是内置流程会写的那种，只是地址指向
// 假供应商。
//
// 为什么走 store 而不是走 HTTP 路由：那条路由把供应商自己的根地址写死在代码里，
// 而一套每次都去连真实供应商的回归测试既不划算也不确定。路由本身由
// TestProviderSetupAddsFennoaiAndQiniu 覆盖。
//
// 参数 provider（string）：供应商标识，例如 fennoai。
// 返回：无。写库失败时让当前测试失败。
func (h *harness) seedCredential(provider string) {
	h.t.Helper()
	base := h.credentialAPIBase()
	raw, err := json.Marshal(map[string]any{
		"credential_name": provider,
		"credential_info": map[string]any{
			"custom_llm_provider": "openai",
			"builtin":             provider,
			"api_base":            base,
		},
		"credential_values": map[string]any{"api_key": "sk-fake-upstream", "api_base": base},
	})
	if err != nil {
		h.t.Fatalf("marshal credential %s: %v", provider, err)
	}
	if err := h.store.PutKV("credentials", provider, string(raw)); err != nil {
		h.t.Fatalf("store credential %s: %v", provider, err)
	}
}

// clearAPIKey 把某条部署的 api_key 清掉，用来测"真的没有凭据"这一支。
//
// newHarness 会给没写 api_key 的部署补一把假密钥，免得每条测试都自己写一遍；
// 而"凭据缺失"恰恰是必须测到的一支，所以这里提供一个反过来的把手。
//
// 参数 name（string）：部署的对外模型名。
// 返回：无。找不到这条部署时让当前测试失败。
func (h *harness) clearAPIKey(name string) {
	h.t.Helper()
	cfg := h.gw.Cfg
	for i := range cfg.ModelList {
		if cfg.ModelList[i].ModelName != name {
			continue
		}
		delete(cfg.ModelList[i].LiteLLMParams, "api_key")
		return
	}
	h.t.Fatalf("no deployment named %q to clear the key from", name)
}

// redisClient 返回热花费客户端；没配 Redis 时返回 nil。额度测试拿它证明
// Redis 那一侧的计数也在干活。
// 参数：无。返回 *live.Client（*live.Client）：Redis 客户端，没配时为 nil。
func (h *harness) redisClient() *live.Client { return h.gw.Live }

// tenant 是一条完整的归属链：组织、团队、用户、密钥。额度可以设在其中任何一层，
// 这正是额度测试要变化的那一维。
type tenant struct {
	orgID     string // 组织 id
	teamID    string // 团队 id
	projectID string // 项目 id，密钥挂在项目上时才有
	userID    string // 用户 id
	email     string // 用户登录邮箱
	password  string // 用户登录密码
	keySecret string // 密钥明文，只在键范围预置花费时用到
}

// newTenant 在一个新组织、新团队里建一个用户。每一层都走 HTTP 接口创建，所以
// 这个夹具证明的是操作员真正会用的那些路由，而不是绕过它们去直接写库。
// 参数 t（*testing.T）：当前测试；admin（string）：管理员会话；name（string）：这批资源的名字前缀。
// 返回 tenant（tenant）：建好的归属链，含各层 id。任何一层没拿到 id 就失败。
func (h *harness) newTenant(t *testing.T, admin, name string) tenant {
	t.Helper()
	tn := tenant{
		email:    name + "-regression@example.com",
		password: name + "-regression-password",
	}

	org := h.ok(http.MethodPost, "/organization/new", admin, map[string]any{
		"organization_alias": name + "-org",
	})
	tn.orgID = firstString(org.json(), "organization_id", "id")
	if tn.orgID == "" {
		t.Fatalf("organization/new returned no id: %s", org.describe())
	}

	team := h.ok(http.MethodPost, "/team/new", admin, map[string]any{
		"team_alias":      name + "-team",
		"organization_id": tn.orgID,
	})
	tn.teamID = firstString(team.json(), "team_id", "id")
	if tn.teamID == "" {
		t.Fatalf("team/new returned no id: %s", team.describe())
	}

	user := h.ok(http.MethodPost, "/user/new", admin, map[string]any{
		"user_email":      tn.email,
		"user_alias":      name + "-user",
		"password":        tn.password,
		"user_role":       "internal_user",
		"team_id":         tn.teamID,
		"organization_id": tn.orgID,
	})
	tn.userID = firstString(user.json(), "user_id", "id")
	if tn.userID == "" {
		t.Fatalf("user/new returned no id: %s", user.describe())
	}
	return tn
}

// signIn 让这个租户登录，返回它的会话令牌。
// 参数 t（*testing.T）：当前测试；tn（tenant）：要登录的租户。返回 string（string）：会话令牌。
func (h *harness) signIn(t *testing.T, tn tenant) string {
	t.Helper()
	return h.login(tn.email, tn.password)
}

// provisionedTenant 是已经登录、也已经有密钥的租户——每一个额度用例和端点用例
// 都从这里开始。
type provisionedTenant struct {
	tenant         // 内嵌，所以 tn.userID、tn.teamID 直接用
	session string // 该用户的会话令牌
	key     string // 该用户个人密钥的明文
}

// provision 建租户、登录、发一把个人密钥。
// 参数 t（*testing.T）：当前测试；admin（string）：管理员会话；name（string）：名字前缀。
// 返回 provisionedTenant（provisionedTenant）：可以直接拿去发请求的租户。
func (h *harness) provision(t *testing.T, admin, name string) provisionedTenant {
	t.Helper()
	tn := h.newTenant(t, admin, name)
	session := h.signIn(t, tn)
	key := h.keyFor(t, session, tn.teamID, name+"-key")
	return provisionedTenant{tenant: tn, session: session, key: key}
}

// keyFor 给会话的主人发一把个人密钥，返回密钥明文。
// 每一个密钥都必须带团队，个人密钥也一样，所以团队是传进来的而不是推出来的。
// 参数 t（*testing.T）：当前测试；session（string）：会话令牌；teamID（string）：归属团队；
// alias（string）：密钥别名。返回 string（string）：密钥明文。
func (h *harness) keyFor(t *testing.T, session, teamID, alias string) string {
	t.Helper()
	return h.keyWith(t, session, map[string]any{"key_alias": alias, "team_id": teamID})
}

// keyWith 用显式正文发一把密钥，给那些需要指定团队归属、项目、或者密钥自身额度的用例用。
// 参数 t（*testing.T）：当前测试；session（string）：会话令牌；body（map[string]any）：签发正文。
// 返回 string（string）：密钥明文。拿不到就以 sk- 开头的明文时失败。
func (h *harness) keyWith(t *testing.T, session string, body map[string]any) string {
	t.Helper()
	r := h.ok(http.MethodPost, "/key/generate", session, body)
	secret := firstString(r.json(), "key", "token")
	if !strings.HasPrefix(secret, "sk-") {
		t.Fatalf("key/generate returned no usable secret: %s", r.describe())
	}
	return secret
}

// modelKey 发一把只允许调用指定模型的密钥，用来证明模型允许名单和额度是两件
// 独立的事。
// 参数 t（*testing.T）：当前测试；session/teamID/alias（string）：会话、团队、别名；
// models（...string）：允许调用的模型。返回 string（string）：密钥明文。
func (h *harness) modelKey(t *testing.T, session, teamID, alias string, models ...string) string {
	return h.keyWith(t, session, map[string]any{
		"key_alias": alias, "team_id": teamID, "models": models,
	})
}

// setOrgBudget 设置组织额度。组织更新路由是 PATCH，跟用户和团队那两个不一样，
// 所以这里的动词是写死的而不是想当然的 POST。
// 参数 t（*testing.T）：当前测试；admin/orgID（string）：管理员会话与组织 id；
// budget（float64）：新的上限。返回：无。设置失败时通过 h.ok 让测试失败。
func (h *harness) setOrgBudget(t *testing.T, admin, orgID string, budget float64) {
	t.Helper()
	h.ok(http.MethodPatch, "/organization/update", admin, map[string]any{
		"organization_id": orgID, "max_budget": budget,
	})
}

// setTeamBudget 设置团队额度。
// 参数 t（*testing.T）：当前测试；admin/teamID（string）：管理员会话与团队 id；
// budget（float64）：新的上限。返回：无。设置失败时通过 h.ok 让测试失败。
func (h *harness) setTeamBudget(t *testing.T, admin, teamID string, budget float64) {
	t.Helper()
	h.ok(http.MethodPost, "/team/update", admin, map[string]any{
		"team_id": teamID, "max_budget": budget,
	})
}

// setUserBudget 设置个人额度。
// 参数 t（*testing.T）：当前测试；admin/userID（string）：管理员会话与用户 id；
// budget（float64）：新的上限。返回：无。设置失败时通过 h.ok 让测试失败。
func (h *harness) setUserBudget(t *testing.T, admin, userID string, budget float64) {
	t.Helper()
	h.ok(http.MethodPost, "/user/update", admin, map[string]any{
		"user_id": userID, "max_budget": budget,
	})
}

// spendOn 在指定范围记一笔花费。额度判定读的那些计数正是 iam.RecordUsage 维护的，
// 所以这样预置走的是一个真实调用会走的同一条路，只是不用真的调付费上游。
//
// 参数 t（*testing.T）：当前测试；tn（tenant）：这笔花费挂在谁身上；
// scope（string）：user、key、project、team 或 org；amount（float64）：金额。
// 返回：无。范围名不认识时让当前测试失败。
func (h *harness) spendOn(t *testing.T, tn tenant, scope string, amount float64) {
	t.Helper()
	rec := iam.UsageRecord{
		RequestID: "seed-" + scope + "-" + strconv.FormatInt(time.Now().UnixNano(), 36),
		Cost:      amount,
	}
	switch scope {
	case "user":
		rec.UserID = tn.userID
	case "team":
		rec.TeamID = tn.teamID
	case "org":
		rec.OrganizationID = tn.orgID
	case "project":
		rec.ProjectID = tn.projectID
	case "key":
		// 键范围要的是密钥行的 id。这里用密钥明文去找——调用方手上也只有这个。
		rec.KeyHash = tn.keySecret
	default:
		t.Fatalf("unknown budget scope %q", scope)
	}
	h.recordSpendFor(rec)
}

// boolField 从解出来的对象里读一个布尔值。
// 参数 body（map[string]any）：对象；key（string）：字段名。
// 返回 bool（bool）：字段为真时为真；缺字段或类型不符时为假。
func boolField(body map[string]any, key string) bool {
	b, _ := body[key].(bool)
	return b
}

// firstString 返回若干候选字段里第一个非空的字符串。控制台接口对 id 用过好几种
// 拼法，这样写能把差异吸收掉。
// 参数 body（map[string]any）：对象；keys（...string）：按优先顺序尝试的字段名。
// 返回 string（string）：第一个非空值；都没有时为空串。
func firstString(body map[string]any, keys ...string) string {
	for _, key := range keys {
		if s := stringField(body, key); s != "" {
			return s
		}
	}
	return ""
}

// describe 把一次响应压成一行，拼失败信息用。正文太长时截断，免得把整个断言
// 的输出淹掉。
// 参数：无。返回 string（string）：形如 status=400 body={...} 的一行。
func (r reply) describe() string {
	return fmt.Sprintf("status=%d body=%s", r.status, truncate(r.text(), 400))
}

// truncate 把字符串截到 n 个字节，超长时补一个省略号。
// 参数 s（string）：原串；n（int）：保留的字节数。返回 string（string）：截断后的串。
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
