package regression

import (
	"net/http"
	"strings"
	"testing"
)

// 这一组测日志和用量：一次调用之后，控制台能看到哪些视图，以及这些视图之间
// 是不是自洽。
//
// consistency_test.go 核的是一次调用的四个数字（返回值、计费头、日志行、用量
// 计数）。这一组换一个尺度：同一批调用，在不同的**视图**下加总起来是不是同一个数。
// 日志页、按模型聚合、按密钥聚合、按天聚合——它们读的是同一批行，所以对不上就是
// 报表在骗人。
//
// 这一组还会跨身份核对可见范围：租户只该看见自己的日志，平台管理员看见全部的。

// TestEveryLogViewAgreesOnTheSameTraffic 证明同一批流量在所有视图下加总一致。
// 控制台的有几张表读的是同一批用量行，把总数对齐是它们唯一该有的共识。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestEveryLogViewAgreesOnTheSameTraffic(t *testing.T) {
	h := newHarness(t, chatDeployment("regression-views"))
	admin := h.adminSession()
	tn := h.provision(t, admin, "views")

	// 发几次内容各不相同的调用，免得被缓存合并掉其中一部分。
	const calls = 3
	for i := 0; i < calls; i++ {
		h.ok(http.MethodPost, "/v1/chat/completions", tn.key, map[string]any{
			"model": "regression-views",
			"messages": []any{map[string]any{
				"role": "user", "content": "view probe " + string(rune('a'+i)),
			}},
		})
	}
	h.flushSpend()

	// 日志页：行数就是调用次数。
	logs := h.spendLogs(t, admin)
	rows := logsFor(logs, "regression-views")
	if len(rows) != calls {
		t.Fatalf("the log page shows %d rows for %d calls", len(rows), calls)
	}

	// 每一次调用的总费用，就是日志页上这几行之和。
	var logTotal float64
	for _, row := range rows {
		spend, _ := floatField(row, "spend")
		logTotal += spend
	}

	// 按模型聚合：同一个模型名下的花费必须和日志页加总一致。
	byModel := h.ok(http.MethodGet, "/global/spend/models", admin, nil)
	modelRows := rowsOf(byModel, "data", "models")
	modelRow := findBy(modelRows, "model", "regression-views")
	if modelRow == nil {
		t.Fatalf("the model view did not total this model: %s", truncate(string(mustJSON(modelRows)), 300))
	}
	modelSpend := firstFloat(modelRow, "spend", "total_spend")
	if !nearlyEqual(modelSpend, logTotal) {
		t.Fatalf("model view total %v disagrees with the log rows %v", modelSpend, logTotal)
	}

	// 用户自己的用量计数器，也必须等于同一批调用的总和。
	user, err := h.db.GetUser(t.Context(), tn.userID)
	if err != nil {
		t.Fatalf("read user: %v", err)
	}
	if !nearlyEqual(user.Spend, logTotal) {
		t.Fatalf("the user counter %v disagrees with the log rows %v", user.Spend, logTotal)
	}

	// 按密钥聚合：/spend/keys 用密钥别名做键，所以这里按别名找。
	keys := h.ok(http.MethodGet, "/spend/keys", admin, nil)
	keyRows := rowsOf(keys, "data", "keys")
	keyRow := findBy(keyRows, "key_alias", "views-key")
	if keyRow == nil {
		t.Fatalf("the key view did not total this tenant's key: %s", truncate(string(mustJSON(keyRows)), 300))
	}
	keySpend, _ := floatField(keyRow, "spend")
	if !nearlyEqual(keySpend, logTotal) {
		t.Fatalf("the key view total %v disagrees with the log rows %v", keySpend, logTotal)
	}
}

// TestCallIdIsTheSameEverywhereItAppears 证明同一个 call id 在所有视图里指的是
// 同一次调用。排查问题时人是拿着一个 id 到处点的，两处对不上就等于查不了。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestCallIdIsTheSameEverywhereItAppears(t *testing.T) {
	h := newHarness(t, chatDeployment("regression-callid"))
	admin := h.adminSession()
	tn := h.provision(t, admin, "callid")

	r := h.ok(http.MethodPost, "/v1/chat/completions", tn.key, map[string]any{
		"model": "regression-callid", "messages": []any{map[string]any{"role": "user", "content": "call id"}},
	})
	// 响应头里的 call id 是调用方唯一拿得到的把手。
	callID := r.header("x-litellm-call-id")
	if callID == "" {
		t.Fatalf("the response carried no x-litellm-call-id: %s", r.describe())
	}

	// 用这个 id 能直接查到详情。
	detail := h.ok(http.MethodGet, "/spend/logs/ui/"+callID, admin, nil).json()
	if got := firstString(detail, "request_id", "id"); got != callID {
		t.Fatalf("detail returned request_id=%q, want %q", got, callID)
	}

	// 也出现在列表里，而且是同一行。
	h.flushSpend()
	if findLogByRequestID(h.spendLogs(t, admin), callID) == nil {
		t.Fatalf("call %s is missing from the log list", callID)
	}
}

// TestTenantSeesOnlyItsOwnLogs 证明日志的可见范围受身份限制。
// 日志里带着模型名、密钥别名和花费，串租户看见别人的日志既泄露业务信息，
// 又会让自己的报表数字虚高。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestTenantSeesOnlyItsOwnLogs(t *testing.T) {
	h := newHarness(t, chatDeployment("regression-scope"))
	admin := h.adminSession()
	a := h.provision(t, admin, "scope-a")
	b := h.provision(t, admin, "scope-b")

	// A 发一次，B 发一次，模型名刻意不同，这样能分辨是谁那条。
	h.ok(http.MethodPost, "/v1/chat/completions", a.key, map[string]any{
		"model": "regression-scope", "messages": []any{map[string]any{"role": "user", "content": "from a"}},
	})
	h.ok(http.MethodPost, "/v1/chat/completions", b.key, map[string]any{
		"model": "regression-scope", "messages": []any{map[string]any{"role": "user", "content": "from b"}},
	})
	h.flushSpend()

	// A 看到的每一行，密钥别名都该是 A 自己的那把。
	aRows := rowsOf(h.ok(http.MethodGet, "/spend/logs/ui?page=1&page_size=200", a.session, nil), "data", "logs")
	for _, row := range aRows {
		if alias := logKeyAlias(row); alias != "" && alias != "scope-a-key" {
			t.Fatalf("tenant A sees a row belonging to %q", alias)
		}
	}

	// 平台管理员两边都看得见，否则运维就没法排查了。
	adminRows := rowsOf(h.ok(http.MethodGet, "/spend/logs/ui?page=1&page_size=200", admin, nil), "data", "logs")
	aliases := map[string]bool{}
	for _, row := range adminRows {
		aliases[logKeyAlias(row)] = true
	}
	for _, want := range []string{"scope-a-key", "scope-b-key"} {
		if !aliases[want] {
			t.Fatalf("the platform administrator cannot see %s among %v", want, aliases)
		}
	}
}

// TestDailyActivityCountsTheCalls 证明按天聚合的视图真的把当天的调用算进去了。
// 日活视图是"这个租户今天用了多少"的答案，它漏掉调用比数字偏差更麻烦——
// 会让人以为没人在用。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestDailyActivityCountsTheCalls(t *testing.T) {
	h := newHarness(t, chatDeployment("regression-daily"))
	admin := h.adminSession()
	tn := h.provision(t, admin, "daily")

	const calls = 2
	for i := 0; i < calls; i++ {
		h.ok(http.MethodPost, "/v1/chat/completions", tn.key, map[string]any{
			"model": "regression-daily",
			"messages": []any{map[string]any{
				"role": "user", "content": "daily probe " + string(rune('a'+i)),
			}},
		})
	}
	h.flushSpend()

	activity := h.ok(http.MethodGet, "/user/daily/activity?page=1&page_size=100", admin, nil)

	// 请求数藏在 results[].breakdown.api_keys[<hash>].metrics.api_requests 下面，
	// 所以要一路挖到底，不能只看顶层字段。
	rows := rowsOf(activity, "results", "data", "activity")
	if len(rows) == 0 {
		t.Fatalf("the daily activity view returned no rows: %s", truncate(activity.text(), 300))
	}
	total := 0
	for _, row := range rows {
		total += apiRequestsInBreakdown(row)
	}
	if total == 0 {
		t.Fatalf("the daily view reports zero requests across %d rows: %s",
			len(rows), truncate(activity.text(), 400))
	}
	if total != calls {
		t.Fatalf("the daily view counts %d requests, want %d", total, calls)
	}
}

// TestPromptStorageFollowsTheSwitch 证明花费日志默认不留请求正文，管理员打开开关之后才留。
// 关掉之后下一笔又不再留。开关只影响新日志，已经写下的正文还在。
func TestPromptStorageFollowsTheSwitch(t *testing.T) {
	h := openHarness(t, false, chatDeployment("regression-prompts"))
	admin := h.adminSession()
	tn := h.provision(t, admin, "prompts")

	off := h.ok(http.MethodPost, "/v1/chat/completions", tn.key, map[string]any{
		"model":    "regression-prompts",
		"messages": []any{map[string]any{"role": "user", "content": "secret-while-off"}},
	})
	h.flushSpend()
	if body := string(mustJSON(logDetail(t, h, admin, off.header("x-litellm-call-id"))["messages"])); strings.Contains(body, "secret-while-off") {
		t.Fatalf("the prompt was stored while the switch was off: %s", body)
	}

	turnedOn := h.do(http.MethodPost, "/config/update", admin, map[string]any{
		"general_settings": map[string]any{"store_prompts_in_spend_logs": true},
	})
	if turnedOn.status != http.StatusOK {
		t.Fatalf("turn prompt storage on: %d %s", turnedOn.status, turnedOn.text())
	}
	on := h.ok(http.MethodPost, "/v1/chat/completions", tn.key, map[string]any{
		"model":    "regression-prompts",
		"messages": []any{map[string]any{"role": "user", "content": "secret-while-on"}},
	})
	h.flushSpend()
	stored := string(mustJSON(logDetail(t, h, admin, on.header("x-litellm-call-id"))))
	if !strings.Contains(stored, "secret-while-on") || !strings.Contains(stored, "regression-ok") {
		t.Fatalf("the open switch did not keep the prompt and the answer: %s", stored)
	}

	turnedOff := h.do(http.MethodPost, "/config/update", admin, map[string]any{
		"general_settings": map[string]any{"store_prompts_in_spend_logs": false},
	})
	if turnedOff.status != http.StatusOK {
		t.Fatalf("turn prompt storage off: %d %s", turnedOff.status, turnedOff.text())
	}
	again := h.ok(http.MethodPost, "/v1/chat/completions", tn.key, map[string]any{
		"model":    "regression-prompts",
		"messages": []any{map[string]any{"role": "user", "content": "secret-while-off-again"}},
	})
	h.flushSpend()
	if body := string(mustJSON(logDetail(t, h, admin, again.header("x-litellm-call-id"))["messages"])); strings.Contains(body, "secret-while-off-again") {
		t.Fatalf("the prompt was stored after the switch was turned off: %s", body)
	}
}

func logDetail(t *testing.T, h *harness, admin, callID string) map[string]any {
	t.Helper()
	if callID == "" {
		t.Fatal("the response carried no call id")
	}
	return h.ok(http.MethodGet, "/spend/logs/ui/"+callID, admin, nil).json()
}

// TestSpendCalculateUsesThePriceCatalog 钉住估算接口的定价来源。
//
// 它从生成的价格目录里取费率，不看某条部署上另填的单价。这是一个刻意的设计：
// 估算接口回答的是"这个模型大概多少钱"，而部署上的单价是运维的私人安排。
//
// 这条测试同时把这个差别说清楚，免得有人以为两边必然相等。真正的等价关系不在
// 这里，而在 consistency_test.go 里：那次核的是"同一个定价来源下，四个数字一致"。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestSpendCalculateUsesThePriceCatalog(t *testing.T) {
	h := newHarness(t, chatDeployment("regression-calc"))
	admin := h.adminSession()

	// 挑一条生成目录里确实有费率、而且是对话类的模型。
	const catalogModel = "claude-4.1-opus"

	estimate := h.ok(http.MethodPost, "/spend/calculate", admin, map[string]any{
		"model": catalogModel,
		"completion_response": map[string]any{
			"model": catalogModel,
			"usage": map[string]any{"prompt_tokens": 1000, "completion_tokens": 1000},
		},
	}).json()
	estimated, ok := floatField(estimate, "cost")
	if !ok {
		t.Fatalf("the estimate carried no cost: %s", mustJSON(estimate))
	}
	if estimated <= 0 {
		t.Fatalf("the catalog has no rate for %s, so the estimate is zero: %s", catalogModel, mustJSON(estimate))
	}

	// 同一个模型、同一批 token，算第二次必须一样：这个接口不能有随机性。
	again := h.ok(http.MethodPost, "/spend/calculate", admin, map[string]any{
		"model": catalogModel,
		"completion_response": map[string]any{
			"model": catalogModel,
			"usage": map[string]any{"prompt_tokens": 1000, "completion_tokens": 1000},
		},
	}).json()
	if repeat, _ := floatField(again, "cost"); !nearlyEqual(repeat, estimated) {
		t.Fatalf("the estimate is not stable: %v then %v", estimated, repeat)
	}

	// 而一个不在目录里的模型应该老老实实说零，不能编一个数出来。
	unknown := h.ok(http.MethodPost, "/spend/calculate", admin, map[string]any{
		"model": "definitely-not-in-the-catalog",
		"completion_response": map[string]any{
			"model": "definitely-not-in-the-catalog",
			"usage": map[string]any{"prompt_tokens": 1000, "completion_tokens": 1000},
		},
	}).json()
	if cost, _ := floatField(unknown, "cost"); cost != 0 {
		t.Fatalf("an unpriced model was estimated at %v instead of zero", cost)
	}
}

// firstFloat 按顺序读第一个读得到的数字字段。
// 参数 row（map[string]any）：对象；fields（...string）：按优先顺序尝试的字段名。
// 返回 float64（float64）：读到的数；都读不到时为 0。
func firstFloat(row map[string]any, fields ...string) float64 {
	for _, field := range fields {
		if v, ok := floatField(row, field); ok {
			return v
		}
	}
	return 0
}

// logKeyAlias 从一行日志里取出密钥别名。日志把密钥信息放在 metadata 下面，
// 所以不能只看顶层字段。
// 参数 row（map[string]any）：一行日志。
// 返回 string（string）：密钥别名；没有时为空串。
func logKeyAlias(row map[string]any) string {
	if meta, ok := row["metadata"].(map[string]any); ok {
		if alias := stringField(meta, "user_api_key_alias"); alias != "" {
			return alias
		}
	}
	// 有些视图把别名提到顶层，这里退一步认它。
	return firstString(row, "user_api_key_alias", "key_alias")
}

// apiRequestsInBreakdown 从一行日活里挖出请求数。这个数字藏在
// results[].breakdown.api_keys[<hash>].metrics.api_requests 下面，所以要逐层下去，
// 把各个密钥的请求数加起来。
// 参数 row（map[string]any）：一行日活。
// 返回 int（int）：这一行的请求数；挖不到时为 0。
func apiRequestsInBreakdown(row map[string]any) int {
	total := 0
	breakdown, _ := row["breakdown"].(map[string]any)
	keys, _ := breakdown["api_keys"].(map[string]any)
	for _, raw := range keys {
		entry, _ := raw.(map[string]any)
		metrics, _ := entry["metrics"].(map[string]any)
		if n, ok := floatField(metrics, "api_requests"); ok {
			total += int(n)
		}
	}
	// 顶层也报一次总数，两种写法都认。
	if total == 0 {
		if n, ok := floatField(row, "api_requests"); ok {
			total = int(n)
		}
	}
	if total == 0 {
		if meta, ok := row["metadata"].(map[string]any); ok {
			if n, ok := floatField(meta, "total_api_requests"); ok {
				total = int(n)
			}
		}
	}
	return total
}
