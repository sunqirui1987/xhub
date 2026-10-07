package regression

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/sunqirui1987/xhub/internal/config"
)

// chained 是一条回归链路用的租户：组织、团队、项目、用户，以及挂在项目上的密钥。
// 额度链路要五层都能记账，所以密钥必须挂上项目，否则项目花费永远不动。
type chained struct {
	provisionedTenant
	keyID string
}

// scopeMoney 是同一次调用会推动的五张花费计数。
type scopeMoney struct {
	user, key, project, team, org float64
}

// deployment 造一条对话部署。upstream 是 litellm 的 model 字段，例如 openai/cheap。
// extra 覆盖权重、单价、延迟这些路由要用的参数。
func deployment(public, upstream string, extra map[string]any) config.ModelEntry {
	params := map[string]any{
		"model":                 upstream,
		"api_key":               "sk-fake",
		"custom_llm_provider":   "openai",
		"input_cost_per_token":  testInputRate,
		"output_cost_per_token": testOutputRate,
	}
	for k, v := range extra {
		params[k] = v
	}
	return config.ModelEntry{
		ModelName:     public,
		LiteLLMParams: params,
		ModelInfo:     map[string]any{"mode": "chat"},
	}
}

// chatRequest 是一次对话补全。每次内容都换掉，避免被响应缓存当成同一次调用。
func chatRequest(model, content string) map[string]any {
	return map[string]any{
		"model":    model,
		"messages": []any{map[string]any{"role": "user", "content": content}},
	}
}

// openScope 建组织、团队、项目、用户，再发一把挂在项目上的个人密钥。
func (h *harness) openScope(t *testing.T, admin, name string) chained {
	t.Helper()
	tn := h.newTenant(t, admin, name)
	session := h.signIn(t, tn)
	project := h.ok(http.MethodPost, "/project/new", admin, map[string]any{
		"team_id":       tn.teamID,
		"project_alias": name + "-project",
	})
	tn.projectID = firstString(project.json(), "project_id", "id")
	if tn.projectID == "" {
		t.Fatalf("project/new returned no id: %s", project.describe())
	}
	secret, keyID := h.issueKey(t, session, tn.teamID, tn.projectID, name+"-key")
	tn.keySecret = secret
	return chained{provisionedTenant: provisionedTenant{tenant: tn, session: session, key: secret}, keyID: keyID}
}

// issueKey 发一把密钥，返回明文和 id。projectID 为空时不挂项目。
func (h *harness) issueKey(t *testing.T, session, teamID, projectID, alias string) (string, string) {
	t.Helper()
	body := map[string]any{"key_alias": alias, "team_id": teamID}
	if projectID != "" {
		body["project_id"] = projectID
	}
	r := h.ok(http.MethodPost, "/key/generate", session, body)
	secret := firstString(r.json(), "key", "token")
	keyID := firstString(r.json(), "token_id", "key_id")
	if !strings.HasPrefix(secret, "sk-") || keyID == "" {
		t.Fatalf("key/generate returned no usable secret: %s", r.describe())
	}
	return secret, keyID
}

// setProjectBudget 设置项目额度。
func (h *harness) setProjectBudget(t *testing.T, admin, projectID string, budget float64) {
	t.Helper()
	h.ok(http.MethodPost, "/project/update", admin, map[string]any{
		"project_id": projectID, "max_budget": budget,
	})
}

// setKeyBudget 设置密钥额度。
func (h *harness) setKeyBudget(t *testing.T, admin, secret string, budget float64) {
	t.Helper()
	h.ok(http.MethodPost, "/key/update", admin, map[string]any{
		"key": secret, "max_budget": budget,
	})
}

// setRouter 写入路由设置。返回的是这次更新的响应，调用方决定它该不该成功。
func (h *harness) setRouter(admin string, patch map[string]any) reply {
	return h.do(http.MethodPost, "/config/update", admin, map[string]any{"router_settings": patch})
}

// moneyOf 读五层当前花费。拒绝之后这五个数都不该动。
func (h *harness) moneyOf(t *testing.T, c chained) scopeMoney {
	t.Helper()
	h.flushSpend()
	var m scopeMoney
	user, err := h.db.GetUser(t.Context(), c.userID)
	if err != nil {
		t.Fatalf("read user spend: %v", err)
	}
	key, err := h.db.GetKey(t.Context(), c.keyID)
	if err != nil {
		t.Fatalf("read key spend: %v", err)
	}
	project, err := h.db.GetProject(t.Context(), c.projectID)
	if err != nil {
		t.Fatalf("read project spend: %v", err)
	}
	team, err := h.db.GetTeam(t.Context(), c.teamID)
	if err != nil {
		t.Fatalf("read team spend: %v", err)
	}
	org, err := h.db.GetOrg(t.Context(), c.orgID)
	if err != nil {
		t.Fatalf("read org spend: %v", err)
	}
	m.user, m.key, m.project, m.team, m.org = user.Spend, key.Spend, project.Spend, team.Spend, org.Spend
	return m
}

func (m scopeMoney) same(other scopeMoney) bool {
	return nearlyEqual(m.user, other.user) && nearlyEqual(m.key, other.key) &&
		nearlyEqual(m.project, other.project) && nearlyEqual(m.team, other.team) &&
		nearlyEqual(m.org, other.org)
}

func (m scopeMoney) grewBy(before scopeMoney, cost float64) bool {
	return nearlyEqual(m.user-before.user, cost) && nearlyEqual(m.key-before.key, cost) &&
		nearlyEqual(m.project-before.project, cost) && nearlyEqual(m.team-before.team, cost) &&
		nearlyEqual(m.org-before.org, cost)
}

// successRows 返回这个模型成功的用量行。没有日志时是空切片，不把"还没调用"当成失败。
func (h *harness) successRows(t *testing.T, admin, model string) []map[string]any {
	t.Helper()
	h.flushSpend()
	r := h.do(http.MethodGet, "/spend/logs/ui?page=1&page_size=200", admin, nil)
	if r.status < 200 || r.status >= 300 {
		t.Fatalf("spend logs -> %d: %s", r.status, r.text())
	}
	var out []map[string]any
	for _, row := range rowsOf(r, "data", "logs") {
		if stringField(row, "model") == model && stringField(row, "status") == "success" {
			out = append(out, row)
		}
	}
	return out
}

// upstreamSince 返回从 mark 之后上游看见的模型名，顺序就是拨号顺序。
func (h *harness) upstreamSince(mark int) []string {
	calls := h.upstreamCalls()
	if mark > len(calls) {
		mark = len(calls)
	}
	out := make([]string, 0, len(calls)-mark)
	for _, call := range calls[mark:] {
		out = append(out, stringField(call.Body, "model"))
	}
	return out
}

// assertBilled 走完一次成功调用，并核对上游模型、响应金额、日志和五层花费是同一笔钱。
// want 是这次新增的上游模型序列；nil 表示只要求至少拨了一次。
func (h *harness) assertBilled(t *testing.T, c chained, admin, model, content string, want []string) float64 {
	t.Helper()
	beforeMoney := h.moneyOf(t, c)
	beforeLogs := len(h.successRows(t, admin, model))
	mark := len(h.upstreamCalls())
	r := h.ok(http.MethodPost, "/v1/chat/completions", c.key, chatRequest(model, content))
	if got := stringField(r.json(), "id"); got == "" {
		t.Fatalf("a successful call returned no id: %s", r.describe())
	}
	gotModels := h.upstreamSince(mark)
	if h.live {
		// 真实供应商不经过假服务器，拨号次数从花费和日志看，不从假上游的记录看。
	} else if want == nil {
		if len(gotModels) == 0 {
			t.Fatal("the call never reached the upstream")
		}
	} else if strings.Join(gotModels, ",") != strings.Join(want, ",") {
		t.Fatalf("upstream models %v, want %v", gotModels, want)
	}
	cost := parseFloatOrZero(r.header("x-litellm-response-cost"))
	if cost <= 0 {
		t.Fatalf("the call was not priced: %s", r.describe())
	}
	rows := h.successRows(t, admin, model)
	if len(rows) != beforeLogs+1 {
		t.Fatalf("success logs went from %d to %d, want one new row", beforeLogs, len(rows))
	}
	// 日志页不一定按时间追加，用这次响应的 call id 对上那一行。
	callID := r.header("x-litellm-call-id")
	row := findLogByRequestID(rows, callID)
	if row == nil {
		t.Fatalf("no success log for call %s", callID)
	}
	logged, _ := floatField(row, "spend")
	if !nearlyEqual(logged, cost) {
		t.Fatalf("log spend %v disagrees with the response cost %v", logged, cost)
	}
	if stringField(row, "project_id") != c.projectID {
		t.Fatalf("log project_id %q, want %s", stringField(row, "project_id"), c.projectID)
	}
	after := h.moneyOf(t, c)
	if !after.grewBy(beforeMoney, cost) {
		t.Fatalf("scopes grew user=%v key=%v project=%v team=%v org=%v, want each to grow by %v",
			after.user-beforeMoney.user, after.key-beforeMoney.key, after.project-beforeMoney.project,
			after.team-beforeMoney.team, after.org-beforeMoney.org, cost)
	}
	return cost
}

// assertBudgetStop 断言这次调用在拨上游之前被某一层额度拒绝，五层花费和成功日志都不变。
func (h *harness) assertBudgetStop(t *testing.T, c chained, admin, model, content, scope string) {
	t.Helper()
	beforeMoney := h.moneyOf(t, c)
	beforeLogs := len(h.successRows(t, admin, model))
	mark := len(h.upstreamCalls())
	r := h.do(http.MethodPost, "/v1/chat/completions", c.key, chatRequest(model, content))
	if r.status != http.StatusTooManyRequests {
		t.Fatalf("a %s at its ceiling answered %d, want 429: %s", scope, r.status, r.describe())
	}
	message := errorMessage(r)
	if !strings.Contains(message, scope) || !strings.Contains(message, "budget has been exceeded") {
		t.Fatalf("the refusal named %q, want %s budget has been exceeded", message, scope)
	}
	if got := h.upstreamSince(mark); len(got) != 0 {
		t.Fatalf("an over-budget call reached the upstream: %v", got)
	}
	if after := h.moneyOf(t, c); !after.same(beforeMoney) {
		t.Fatalf("the refused call moved spend from %+v to %+v", beforeMoney, after)
	}
	if got := len(h.successRows(t, admin, model)); got != beforeLogs {
		t.Fatalf("the refused call added a success log: %d -> %d", beforeLogs, got)
	}
}

// assertStopped 断言调用被拒绝且没有新增上游。用在停用、过期、模型名单这些不是 429 的拒绝上。
func (h *harness) assertStopped(t *testing.T, c chained, model, content string) reply {
	t.Helper()
	before := h.moneyOf(t, c)
	mark := len(h.upstreamCalls())
	r := h.do(http.MethodPost, "/v1/chat/completions", c.key, chatRequest(model, content))
	if r.status < 300 {
		t.Fatalf("the call was allowed: %s", r.describe())
	}
	if got := h.upstreamSince(mark); len(got) != 0 {
		t.Fatalf("a refused call reached the upstream: %v", got)
	}
	if after := h.moneyOf(t, c); !after.same(before) {
		t.Fatalf("a refused call moved spend from %+v to %+v", before, after)
	}
	return r
}

// waitUpstream 等到假上游记下这个模型，或超时。least-busy 要等第一条请求真的占住部署。
func (h *harness) waitUpstream(t *testing.T, model string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		for _, call := range h.upstreamCalls() {
			if stringField(call.Body, "model") == model {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("upstream never saw %s: %v", model, h.upstreamSince(0))
}
