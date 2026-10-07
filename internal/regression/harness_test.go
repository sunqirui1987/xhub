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
			StorePromptsInSpendLogs: true,
		},
	}
	h.gw = gateway.New(cfg, st, db)
	h.server = httptest.NewServer(h.gw.Handler())
	t.Cleanup(h.server.Close)
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
	// 一个只回 JSON 的假供应商会让流式用例假通过。
	if stream, _ := body["stream"].(bool); stream {
		h.serveStream(w, model)
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
	default:
		writeJSON(w, map[string]any{
			"id": "chatcmpl-regression", "object": "chat.completion", "created": 1, "model": model,
			"choices": []any{map[string]any{
				"index": 0, "finish_reason": "stop",
				"message": map[string]any{"role": "assistant", "content": defaultReply.Content},
			}},
			"usage": map[string]any{
				"prompt_tokens":     defaultReply.PromptTokens,
				"completion_tokens": defaultReply.CompletionTokens,
				"total_tokens":      defaultReply.PromptTokens + defaultReply.CompletionTokens,
			},
		})
	}
}

// serveStream 按 SSE 回一次流式补全。分几个 chunk 发出去，最后补一个带 usage 的
// chunk 和终止行——真实供应商就是这么发的，而 usage 只出现在流的尾巴上，
// 这正是流式计费容易漏掉的地方。
// 参数 w（http.ResponseWriter）：写响应；model（string）：本次请求的模型名。
// 返回：无。
func (h *harness) serveStream(w http.ResponseWriter, model string) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	flusher, _ := w.(http.Flusher)

	chunks := []map[string]any{
		{"id": "chatcmpl-regression", "object": "chat.completion.chunk", "created": 1, "model": model,
			"choices": []any{map[string]any{"index": 0, "delta": map[string]any{"role": "assistant"}, "finish_reason": nil}}},
		{"id": "chatcmpl-regression", "object": "chat.completion.chunk", "created": 1, "model": model,
			"choices": []any{map[string]any{"index": 0, "delta": map[string]any{"content": defaultReply.Content}, "finish_reason": nil}}},
	}
	for _, chunk := range chunks {
		_, _ = io.WriteString(w, "data: "+string(mustJSON(chunk))+"\n\n")
		if flusher != nil {
			flusher.Flush()
		}
	}

	// 最后一个 chunk 带 usage，和真实供应商一样。
	final := map[string]any{
		"id": "chatcmpl-regression", "object": "chat.completion.chunk", "created": 1, "model": model,
		"choices": []any{map[string]any{"index": 0, "delta": map[string]any{}, "finish_reason": "stop"}},
		"usage": map[string]any{
			"prompt_tokens":     defaultReply.PromptTokens,
			"completion_tokens": defaultReply.CompletionTokens,
			"total_tokens":      defaultReply.PromptTokens + defaultReply.CompletionTokens,
		},
	}
	_, _ = io.WriteString(w, "data: "+string(mustJSON(final))+"\n\n")
	_, _ = io.WriteString(w, "data: [DONE]\n\n")
	if flusher != nil {
		flusher.Flush()
	}
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
	resp, err := h.server.Client().Do(req)
	if err != nil {
		h.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
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

// liveKeys 是真实调用要用到的供应商密钥。
// 只从环境变量读，绝不写进仓库。
type liveKeys struct {
	fenno string // fennoai 的密钥，来自 XHUB_REGRESSION_FENNO_KEY
	qiniu string // qiniu 的密钥，来自 XHUB_REGRESSION_QINIU_KEY
}

// liveEnabled 报告这次要不要调真实供应商。默认关着，理由有三个：花钱、受供应商
// 抖动影响、以及"别人家服务挂了"这种事教不会我们任何东西。
// 参数：无。返回 bool（bool）：环境变量打开时为真。
func liveEnabled() bool { return liveModeEnabled() }

// liveCredentials 读真实调用的密钥。开了 live 模式却没给密钥就直接失败，而不是
// 悄悄跳过——静默跳过看起来和通过一模一样。
// 参数 t（*testing.T）：当前测试。
// 返回 liveKeys（liveKeys）：两个供应商的密钥。未开 live 模式时整个用例跳过。
func liveCredentials(t *testing.T) liveKeys {
	t.Helper()
	keys := liveKeys{
		fenno: strings.TrimSpace(os.Getenv("XHUB_REGRESSION_FENNO_KEY")),
		qiniu: strings.TrimSpace(os.Getenv("XHUB_REGRESSION_QINIU_KEY")),
	}
	if !liveEnabled() {
		t.Skip("live vendor calls are off; set XHUB_REGRESSION_LIVE=1 to enable")
	}
	if keys.fenno == "" || keys.qiniu == "" {
		t.Fatal("live mode needs XHUB_REGRESSION_FENNO_KEY and XHUB_REGRESSION_QINIU_KEY")
	}
	return keys
}

// liveChatBase 给出 chat 部署要连的真实根地址。协议适配会把 "/chat/completions"
// 接到这个地址后面，所以 OpenAI 兼容的供应商要自带 "/v1"。
// Bypass 不一样：整条路径由端点类型自己给，所以另有一个 liveBypassBase。
// 参数 provider（string）：供应商标识。返回 string（string）：该供应商的 chat 根地址。
func liveChatBase(provider string) string {
	switch provider {
	case "fennoai":
		return "https://api.fenno.ai/v1"
	case "qiniu":
		return "https://api.qnaigc.com/v1"
	default:
		return ""
	}
}

// liveBypassBase 给出 Bypass 部署要连的真实根地址：光秃秃的主机名，因为剩下的
// 路径由端点类型带。
// 参数 provider（string）：供应商标识。返回 string（string）：该供应商的 Bypass 根地址。
func liveBypassBase(provider string) string {
	switch provider {
	case "fennoai":
		return "https://api.fenno.ai"
	case "qiniu":
		return "https://api.qnaigc.com"
	default:
		return ""
	}
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
