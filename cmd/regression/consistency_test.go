package regression

import (
	"net/http"
	"strconv"
	"strings"
	"testing"
)

// TestResponseUsageAndLogsAgree 是发布必须依赖的一致性核对。
//
// 只发一次补全，然后把四样东西互相印证：
//
//   - 响应正文里的 usage 块；
//   - 同一个响应上的 x-litellm-response-cost 头；
//   - 控制台读的那一行用量日志；
//   - 额度判定读的那几个用量计数。
//
// 其中任何一样和其余几个对不上，客户就会被按一个和 API 告诉他的用量不同的数字
// 收钱。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestResponseUsageAndLogsAgree(t *testing.T) {
	h := newHarness(t, chatDeployment("regression-agree"))
	admin := h.adminSession()
	tn := h.provision(t, admin, "agree")
	h.flushSpend()

	r := h.ok(http.MethodPost, "/v1/chat/completions", tn.key, map[string]any{
		"model": "regression-agree", "messages": []any{map[string]any{"role": "user", "content": "count me"}},
	})
	body := r.json()

	usage, ok := body["usage"].(map[string]any)
	if !ok {
		t.Fatalf("the response carried no usage block: %s", r.describe())
	}
	prompt, _ := floatField(usage, "prompt_tokens")
	completion, _ := floatField(usage, "completion_tokens")
	if prompt != float64(defaultReply.PromptTokens) || completion != float64(defaultReply.CompletionTokens) {
		t.Fatalf("response usage prompt=%v completion=%v, want %d/%d",
			prompt, completion, defaultReply.PromptTokens, defaultReply.CompletionTokens)
	}

	// 网关按价目表给这次调用定价。这个头就是告诉调用方扣了多少钱的那句话。
	headerCost := r.header("x-litellm-response-cost")
	if headerCost == "" {
		t.Fatal("the response carried no x-litellm-response-cost header")
	}

	h.flushSpend()
	logs := h.spendLogs(t, admin)
	if len(logs) == 0 {
		t.Fatal("no spend log was written for the call")
	}
	row := newestLogFor(logs, "regression-agree")
	if row == nil {
		t.Fatalf("no spend log named regression-agree: %v", logModels(logs))
	}

	// 日志里的 token 数和响应正文里的应该是同一个。
	logPrompt, _ := floatField(row, "prompt_tokens")
	logCompletion, _ := floatField(row, "completion_tokens")
	if logPrompt != prompt || logCompletion != completion {
		t.Fatalf("log tokens %v/%v disagree with the response %v/%v", logPrompt, logCompletion, prompt, completion)
	}

	// 钱也要一样。日志里的 spend 是开发票依据的数字，所以它必须等于调用方
	// 已经看到的那个头。
	logSpend, _ := floatField(row, "spend")
	headerValue := parseFloatOrZero(headerCost)
	if !nearlyEqual(logSpend, headerValue) {
		t.Fatalf("log spend %v disagrees with the response header %v", logSpend, headerValue)
	}
	if logSpend <= 0 {
		t.Fatalf("the call was priced at zero: %v", logSpend)
	}
}

// TestUsageCountersMatchTheBill 证明额度判定读的那些计数，推动的幅度正好等于日志里
// 记的扣费。额度判定是拿累计花费和上限比的，所以一个和账目走散的计数，
// 要么让租户花超，要么提前把他拦下。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestUsageCountersMatchTheBill(t *testing.T) {
	h := newHarness(t, chatDeployment("regression-counters"))
	admin := h.adminSession()
	tn := h.provision(t, admin, "counters")
	h.flushSpend()

	r := h.ok(http.MethodPost, "/v1/chat/completions", tn.key, map[string]any{
		"model": "regression-counters", "messages": []any{map[string]any{"role": "user", "content": "count me"}},
	})
	charged := parseFloatOrZero(r.header("x-litellm-response-cost"))
	if charged <= 0 {
		t.Fatalf("the call was priced at zero, so the counters prove nothing: %s", r.describe())
	}

	h.flushSpend()
	user, err := h.db.GetUser(t.Context(), tn.userID)
	if err != nil {
		t.Fatalf("read user: %v", err)
	}
	if !nearlyEqual(user.Spend, charged) {
		t.Fatalf("user spend %v, want the charged %v", user.Spend, charged)
	}
	team, err := h.db.GetTeam(t.Context(), tn.teamID)
	if err != nil {
		t.Fatalf("read team: %v", err)
	}
	if !nearlyEqual(team.Spend, charged) {
		t.Fatalf("team spend %v, want the charged %v", team.Spend, charged)
	}
	org, err := h.db.GetOrg(t.Context(), tn.orgID)
	if err != nil {
		t.Fatalf("read org: %v", err)
	}
	if !nearlyEqual(org.Spend, charged) {
		t.Fatalf("org spend %v, want the charged %v", org.Spend, charged)
	}
}

// TestCacheHitIsFreeAndLoggedAsOne 钉住缓存的约定：第二次一样的调用由缓存应答，
// 被标成命中，不扣任何钱，但仍然留下一行日志。
//
// 悄悄把这行丢掉的缓存会让这次请求不可见；对命中仍然扣费的缓存等于重复收费。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestCacheHitIsFreeAndLoggedAsOne(t *testing.T) {
	h := newHarness(t, chatDeployment("regression-cache"))
	admin := h.adminSession()
	tn := h.provision(t, admin, "cache")
	h.flushSpend()
	h.resetUpstream()

	request := map[string]any{
		"model": "regression-cache", "messages": []any{map[string]any{"role": "user", "content": "same question"}},
	}
	first := h.ok(http.MethodPost, "/v1/chat/completions", tn.key, request)
	if misses := len(h.upstreamCalls()); misses != 1 {
		t.Fatalf("the first call made %d upstream requests, want 1", misses)
	}
	firstCost := parseFloatOrZero(first.header("x-litellm-response-cost"))

	second := h.ok(http.MethodPost, "/v1/chat/completions", tn.key, request)
	if got := len(h.upstreamCalls()); got != 1 {
		t.Fatalf("the second identical call reached the upstream (%d requests total); the cache did not answer it", got)
	}
	if !isCacheHit(second) {
		t.Fatalf("the second call was not marked as a cache hit: %s", second.describe())
	}
	// 缓存命中是一次请求事实，不是第二次生成，所以不计费。响应头自己就说清了
	// 这一点。
	if got := parseFloatOrZero(second.header("x-litellm-response-cost")); got != 0 {
		t.Fatalf("a cache hit was charged %v", got)
	}

	h.flushSpend()
	logs := h.spendLogs(t, admin)
	rows := logsFor(logs, "regression-cache")
	if len(rows) != 2 {
		t.Fatalf("expected a row for each of the two calls, got %d", len(rows))
	}
	hits := 0
	var total float64
	for _, row := range rows {
		if cacheHit(row) {
			hits++
			if spend, _ := floatField(row, "spend"); spend != 0 {
				t.Fatalf("the cache-hit row was charged %v", spend)
			}
			continue
		}
		spend, _ := floatField(row, "spend")
		total += spend
	}
	if hits != 1 {
		t.Fatalf("expected exactly one row marked as a cache hit, got %d", hits)
	}
	// 租户只为未命中那次付钱，不是两次都付。
	if !nearlyEqual(total, firstCost) {
		t.Fatalf("charged %v across both rows, want only the miss at %v", total, firstCost)
	}
	user, err := h.db.GetUser(t.Context(), tn.userID)
	if err != nil {
		t.Fatalf("read user: %v", err)
	}
	if !nearlyEqual(user.Spend, firstCost) {
		t.Fatalf("user spend %v, want only the miss at %v", user.Spend, firstCost)
	}
}

// TestFailedCallIsLoggedButNotCharged 证明上游失败会被记成一条不带钱的观测记录。
// 为 4xx 或 5xx 扣费这类问题，通常是客户先发现、运维后知道。
//
// 这一条对应的修复见 internal/dataplane/serve.go 里终局失败那几个分支：
// 它们原本只在进程日志里留痕，控制台里什么都看不到。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestFailedCallIsLoggedButNotCharged(t *testing.T) {
	h := newHarness(t, chatDeployment("regression-fail"))
	admin := h.adminSession()
	tn := h.provision(t, admin, "failure")
	h.flushSpend()
	h.failUpstream(true)
	t.Cleanup(func() { h.failUpstream(false) })

	r := h.do(http.MethodPost, "/v1/chat/completions", tn.key, map[string]any{
		"model": "regression-fail", "messages": []any{map[string]any{"role": "user", "content": "will fail"}},
	})
	if r.status < 400 {
		t.Fatalf("the upstream was told to fail but the gateway answered %d: %s", r.status, r.describe())
	}

	h.flushSpend()
	rows := logsFor(h.spendLogs(t, admin), "regression-fail")
	if len(rows) == 0 {
		t.Fatal("a failed call left no log row, so the failure is invisible")
	}
	for _, row := range rows {
		if spend, _ := floatField(row, "spend"); spend != 0 {
			t.Fatalf("a failed call was charged %v", spend)
		}
		if status := stringField(row, "status"); status != "error" {
			t.Fatalf("failed row has status %q, want error", status)
		}
	}
	user, err := h.db.GetUser(t.Context(), tn.userID)
	if err != nil {
		t.Fatalf("read user: %v", err)
	}
	if user.Spend != 0 {
		t.Fatalf("a failed call moved the user's spend to %v", user.Spend)
	}
}

// TestLogDetailMatchesTheRowInTheList 证明日志的两个读法给的是同一件事，这样点进
// 详情不会显示出一次和列表里不同的调用。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestLogDetailMatchesTheRowInTheList(t *testing.T) {
	h := newHarness(t, chatDeployment("regression-detail"))
	admin := h.adminSession()
	tn := h.provision(t, admin, "detail")
	h.flushSpend()

	r := h.ok(http.MethodPost, "/v1/chat/completions", tn.key, map[string]any{
		"model": "regression-detail", "messages": []any{map[string]any{"role": "user", "content": "detail"}},
	})
	callID := r.header("x-litellm-call-id")
	if callID == "" {
		t.Fatalf("the response carried no call id, so nothing can be looked up: %s", r.describe())
	}
	h.flushSpend()

	detail := logDetail(t, h, admin, callID)
	if got := firstString(detail, "request_id", "id"); got != callID {
		t.Fatalf("detail returned request_id=%q, want %q", got, callID)
	}
	listRow := findLogByRequestID(h.spendLogs(t, admin), callID)
	if listRow == nil {
		t.Fatalf("the listed logs do not contain call %s", callID)
	}
	for _, field := range []string{"model", "prompt_tokens", "completion_tokens", "spend"} {
		detailValue, _ := floatField(detail, field)
		listValue, _ := floatField(listRow, field)
		if detailValue != listValue {
			t.Fatalf("field %s: detail=%v list=%v", field, detailValue, listValue)
		}
	}
}

// header 读一个响应头。reply 把整个头表留着，所以这里直接取。
// 参数 name（string）：头名。返回 string（string）：头的值；没有时为空串。
func (r reply) header(name string) string {
	if r.headers == nil {
		return ""
	}
	return r.headers.Get(name)
}

// isCacheHit 判断这个响应是不是网关从缓存里答的。
// 参数 r（reply）：要判断的响应。返回 bool（bool）：响应头标了命中时为真。
func isCacheHit(r reply) bool {
	return strings.EqualFold(r.header("cache_hit"), "true") || strings.EqualFold(r.header("x-litellm-cache-hit"), "true")
}

// cacheHit 判断这一行日志是不是记的缓存命中。
// 控制台那几个读法里，这个字段出现过布尔和字符串两种写法，这里都认。
// 参数 row（map[string]any）：一行日志。返回 bool（bool）：标了命中时为真。
func cacheHit(row map[string]any) bool {
	if b, ok := row["cache_hit"].(bool); ok {
		return b
	}
	if s, ok := row["cache_hit"].(string); ok {
		return strings.EqualFold(s, "true")
	}
	return false
}

// logsFor 取某个模型的全部日志行。
// 参数 logs（[]map[string]any）：日志行；model（string）：模型名。
// 返回 []map[string]any（[]map[string]any）：匹配的行。
func logsFor(logs []map[string]any, model string) []map[string]any {
	var out []map[string]any
	for _, row := range logs {
		if stringField(row, "model") == model {
			out = append(out, row)
		}
	}
	return out
}

// newestLogFor 取某个模型的最后一行，也就是最近一次。
// 参数 logs（[]map[string]any）：日志行；model（string）：模型名。
// 返回 map[string]any（map[string]any）：最近那一行；没有时返回 nil。
func newestLogFor(logs []map[string]any, model string) map[string]any {
	rows := logsFor(logs, model)
	if len(rows) == 0 {
		return nil
	}
	return rows[len(rows)-1]
}

// findLogByRequestID 取某一次调用的那一行。
// 参数 logs（[]map[string]any）：日志行；requestID（string）：调用 id。
// 返回 map[string]any（map[string]any）：命中那一行；没有时返回 nil。
func findLogByRequestID(logs []map[string]any, requestID string) map[string]any {
	for _, row := range logs {
		if got := firstString(row, "request_id", "id"); got == requestID {
			return row
		}
	}
	return nil
}

// logModels 把一页日志里出现过的模型列出来，拼失败信息用。
// 参数 logs（[]map[string]any）：日志行。返回 []string（[]string）：排好序的模型名。
func logModels(logs []map[string]any) []string { return namesOf(logs, "model") }

// parseFloatOrZero 从响应头或 JSON 字符串里读一个小数。
// 参数 raw（string）：要解析的文本。返回 float64（float64）：读到的数；解析不了时为 0。
func parseFloatOrZero(raw string) float64 {
	out, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil {
		return 0
	}
	return out
}

// nearlyEqual 比较两个金额。两边都是拿同一批 float64 单价、经不同路径算出来的，
// 所以严格相等会因为浮点表示而失败，而不是因为真的不一致。
// 参数 a/b（float64）：要比较的两个金额。返回 bool（bool）：差值在容差以内时为真。
func nearlyEqual(a, b float64) bool {
	const epsilon = 1e-12
	diff := a - b
	if diff < 0 {
		diff = -diff
	}
	return diff <= epsilon
}
