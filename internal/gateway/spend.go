// spend.go writes one usage row for every finished inference call, chat and
// bypass included. recordSpend is the only writer. It copies the exchange
// (headers, body, response) and the call note (provider, TTFT, session,
// session) that the handler stored under the call id, then deletes them.
//
// Redis, when Live is set, takes the hot spend update and queues the row.
// Otherwise persistSpend writes PostgreSQL before the response returns.
// A cache hit keeps the token counts and forces the billed amount to zero.
// A status at or above 400 keeps the row for the log and forces the billed
// amount to zero as well.

package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/sunqirui1987/xhub/internal/auth"
	"github.com/sunqirui1987/xhub/internal/authz"
	"github.com/sunqirui1987/xhub/internal/catalog"
	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/gateway/prefs"
	"github.com/sunqirui1987/xhub/internal/iam"
	"github.com/sunqirui1987/xhub/internal/live"
	"github.com/sunqirui1987/xhub/internal/logx"
	"github.com/sunqirui1987/xhub/internal/router"
	"sync"
)

var logTraceOnceSpend sync.Once

// incBusy increments the in-process concurrency count for a deployment.
// 参数 id（string）：部署 id，形状是 api_base|model。空串时计数没有对应的部署。
// 返回：无。该部署的进程内在途数加一。
// 调用：gateway/wire.go
// 测试：无直接单测
func (s *Server) incBusy(id string) {
	logTraceOnceSpend.Do(func() { logx.Trace("enter gateway.incBusy") })

	s.mu.Lock()
	s.Busy[id]++
	s.mu.Unlock()
}

// decBusy decrements the in-process concurrency count for a deployment.
// 参数 id（string）：部署 id，形状是 api_base|model。必须和 incBusy 用同一个 id。
// 返回：无。该部署的进程内在途数减一。必须和 incBusy 成对。
// 调用：gateway/wire.go
// 测试：无直接单测
func (s *Server) decBusy(id string) {
	s.mu.Lock()
	s.Busy[id]--
	s.mu.Unlock()
}

// setChatHeaders sets response headers such as the model name, spend, and latency.
// 参数 w（http.ResponseWriter）：调用方的 HTTP 响应，状态和正文写在这里；p（*auth.Principal）：已经鉴权的调用方，含用户、团队、密钥哈希和别名；alias（string）：对外模型名；apiBase（string）：上游根地址，末尾斜杠会被去掉再拼路径。
// 返回：无。状态码和正文写进调用方的响应。
// 调用：gateway/engine.go、gateway/wire.go
// 测试：无直接单测
func (s *Server) setChatHeaders(w http.ResponseWriter, p *auth.Principal, alias, apiBase string) {
	w.Header().Set("x-litellm-model-name", alias)
	w.Header().Set("x-litellm-model-api-base", apiBase)
	w.Header().Set("x-litellm-version", Version)
	if p.Key != nil {
		if p.Key.TPMLimit != nil {
			w.Header().Set("x-litellm-key-tpm-limit", strconv.Itoa(*p.Key.TPMLimit))
		}
		if p.Key.RPMLimit != nil {
			w.Header().Set("x-litellm-key-rpm-limit", strconv.Itoa(*p.Key.RPMLimit))
		}
		if p.Key.MaxBudget != nil {
			w.Header().Set("x-litellm-key-max-budget", catalog.Format(*p.Key.MaxBudget))
		}
		w.Header().Set("x-litellm-key-spend", catalog.Format(p.Key.Spend))
	}
}

// callCost prices one call at the instant it started.
//
// A rate typed on the deployment wins. Otherwise the price map is read by the
// upstream model id, then by the public name.
//
// The instant matters: a window-priced model costs twice as much inside its
// peak hours, so pricing a call without its start time undercharges every peak
// request by half. start is the request's own start, which is also what the
// usage row records, so a log row can be re-checked later.
//
// 参数 alias（string）：对外模型名；depID（string）：部署 id。空串表示当前没有钉住的部署；usage（catalog.Usage）：这一次调用报出来的用量，含缓存命中和按秒按张的数量；start（time.Time）：调用开始的时刻，时段由它决定。
// 返回 total（float64）：这一次的总费用；input（float64）：输入侧费用；output（float64）：输出侧费用；ok（bool）：真表示找到了可用结果；charge（catalog.Charge）：这次实际用到的费率，用来落 price_snapshot。
// 调用：recordSpend。
// 测试：call_cost_test.go
func (s *Server) callCost(alias, depID string, usage catalog.Usage, start time.Time) (total, input, output float64, ok bool, charge catalog.Charge) {
	if dep, found := s.FindDeployment(depID); found {
		if c, ok := deploymentCost(dep, usage, start); ok {
			return c.Total, c.Input, c.Output, true, c
		}
		if _, hasRates := deploymentRates(dep.LiteLLMParams); hasRates || dep.ModelInfo["pricing_source"] == "manual" {
			return 0, 0, 0, false, catalog.Charge{}
		}
		if id, _ := dep.ModelInfo["base_model"].(string); id != "" {
			c, ok := catalog.CostAt(id, usage, start)
			return c.Total, c.Input, c.Output, ok, c
		}
		if usage.PricingModel != "" {
			c, ok := catalog.CostAt(usage.PricingModel, usage, start)
			return c.Total, c.Input, c.Output, ok, c
		}
		if id := dep.ParamString("model", ""); id != "" && id != alias {
			if c, ok := catalog.CostAt(id, usage, start); ok {
				return c.Total, c.Input, c.Output, true, c
			}
		}
	}
	c, ok := catalog.CostAt(alias, usage, start)
	if !ok {
		return 0, 0, 0, false, catalog.Charge{}
	}
	return c.Total, c.Input, c.Output, true, c
}

// deploymentCost prices a call from the prices typed on the deployment itself.
//
// A deployment may carry its own rate table, and it has the same shape as the
// catalog's, so a deployment can express an off-peak price and a peak price
// separately. It is tried before the catalog because an operator who typed a
// price meant it.
//
// The flat fields are still accepted for rows written before the rate table
// existed, and for everything the console writes: the unit-price form submits
// one field per side rather than a table. They go through catalog.RatesFromFlat
// and then the same arithmetic as a rate table, rather than through a second
// copy of the billing rules here. That second copy is what used to happen, and
// it dropped three things an operator had already typed: the peak price, the
// cache-write price, and every measure that is not a token - so a video
// deployment with a per-second price recorded a row of zero spend.
//
// 参数 dep（config.ModelEntry）：这条部署；usage（catalog.Usage）：这一次调用报出来的用量；start（time.Time）：调用开始的时刻。
// 返回 catalog.Charge（catalog.Charge）：按部署自己的费率算出的账单；bool（bool）：部署上写了价时为真。
// 调用：callCost。
// 测试：call_cost_test.go
func deploymentCost(dep config.ModelEntry, usage catalog.Usage, start time.Time) (catalog.Charge, bool) {
	params := dep.LiteLLMParams
	if params == nil {
		return catalog.Charge{}, false
	}
	// A rate table on the deployment is the full form and can carry windows and
	// variants the flat fields have no name for.
	if rates, ok := deploymentRates(params); ok {
		return catalog.CostFromRates(rates, usage, start)
	}
	// Otherwise the flat fields, read through the same biller. A field that is
	// absent stays absent: the difference between "no price" and "a price of
	// zero" decides whether this call is billed at all.
	return catalog.CostFromFlatOrRates(func(field string) (float64, bool) {
		return floatParam(params, field)
	}, usage, start)
}

// deploymentRates reads a rate table typed on a deployment. The shape is the
// same as the catalog's rates array, so one renderer and one billing path
// handle both.
// 参数 params（map[string]any）：部署上的 litellm_params。
// 返回 []catalog.Rate（[]catalog.Rate）：部署自己写的费率表；bool（bool）：写了解析得出来时为真。
// 调用：deploymentCost。
// 测试：call_cost_test.go
func deploymentRates(params map[string]any) ([]catalog.Rate, bool) {
	raw, ok := params["rates"]
	if !ok || raw == nil {
		return nil, false
	}
	return catalog.DecodeRates(raw)
}

// 从 litellm_params 读取一个小数。缺键或类型不符时 ok 为假。
// 参数 params（map[string]any）：部署上的 litellm_params。常见键是 model、api_base、api_key；key（string）：上游或调用方的密钥。空串表示还不能转发或还没有密钥。
// 返回 float64（float64）：小数参数。缺失时为 0，不要把它理解成免费除非调用方另有约定；bool（bool）：litellm_params 里有这个小数键且类型正确时返回真。
// 调用：仅在 spend.go 内使用
// 测试：无直接单测
func floatParam(params map[string]any, key string) (float64, bool) {
	if params == nil {
		return 0, false
	}
	v, ok := params[key]
	if !ok || v == nil {
		return 0, false
	}
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(n), 64)
		return f, err == nil
	default:
		return 0, false
	}
}

// recordSpend records this call's spend. With Redis it updates the hot path and queues a log instead of writing PostgreSQL inside the request.
// 参数 w（http.ResponseWriter）：调用方的 HTTP 响应，状态和正文写在这里；p（*auth.Principal）：已经鉴权的调用方，含用户、团队、密钥哈希和别名；callID（string）：这一次调用的 id，用来把请求、响应和用量记在同一行；alias（string）：对外模型名；op（string）：操作名，例如 chat；usage（map[string]any）：用量对象。字段可能是 prompt_tokens，也可能是 input_tokens；start（time.Time）：时间范围的起点。零值表示不限制开始；cacheHit（bool）：为真时走缓存命中这一支。为假时保持原来的路径；status（int）：HTTP 状态码；depID（string）：部署 id。空串表示当前没有钉住的部署。
// 返回：无。状态码和正文写进调用方的响应。
// 调用：gateway/wire.go
// 测试：无直接单测
func (s *Server) recordSpend(w http.ResponseWriter, p *auth.Principal, callID, alias, op string, usage map[string]any, start time.Time, cacheHit bool, status int, depID string) {
	if usage == nil {
		usage = map[string]any{}
	}
	billed := catalog.NormalizeUsage(usage)
	pt, ct := billed.PromptTokens, billed.CompletionTokens
	// The start instant decides the billing window. A window-priced model costs
	// twice as much inside its peak hours, so this is the difference between
	// charging the published rate and charging half of it.
	total, in, out, okc, charge := s.callCost(alias, depID, billed, start)
	// A cache hit is a request fact, but it is not a second upstream generation.
	// Keep the token metadata for observability while making the billable delta
	// explicitly zero. This must happen before both the Redis and PostgreSQL
	// persistence paths so the two paths cannot disagree.
	if cacheHit {
		total, in, out = 0, 0, 0
		charge = catalog.Charge{}
	}
	if status >= 400 {
		total, in, out = 0, 0, 0
		charge = catalog.Charge{}
	}
	// LiteLLM stores response_cost or 0.0. Unknown models still get a row, with spend 0.
	spend := 0.0
	if okc {
		spend = total
		w.Header().Set("x-litellm-response-cost", catalog.Format(total))
		w.Header().Set("x-litellm-response-cost-original", catalog.Format(total))
		w.Header().Set("x-litellm-response-cost-input", catalog.Format(in))
		w.Header().Set("x-litellm-response-cost-output", catalog.Format(out))
		if p != nil && p.Key != nil {
			shown := p.Key.Spend + total
			if s.Live != nil {
				shown = p.Key.Spend + s.Live.HotSpend(live.SpendRef("key", p.Hash)) + total
			}
			w.Header().Set("x-litellm-key-spend", catalog.Format(shown))
		}
	}
	hash := ""
	ownerType := ""
	teamID, userID, orgID, projectID := "", "", "", ""
	if p != nil {
		hash = p.Hash
		ownerType = p.OwnerType
		userID = p.UserID
		if p.Key != nil {
			teamID, projectID = p.Key.TeamID, deref(p.Key.ProjectID)
			// A key carries no organization of its own; its team's is the
			// billing scope above it, snapshotted onto the usage row.
			orgID = s.teamOrg(p.Key.TeamID)
			if p.Key.UserID != nil {
				userID = *p.Key.UserID
			}
		} else if p.Kind == authz.KindSession && userID != "" && s.IAM != nil {
			// A console session has no key. Leaving the owner blank makes the
			// store call it a service log with no team, which neither the caller
			// nor their organization administrator is allowed to read.
			memberships, err := s.IAM.MemberTeams(context.Background(), userID)
			if err != nil {
				logx.Error("usage team lookup failed user=%s err=%v", userID, err)
			}
			ownerType, teamID, orgID = sessionLogBinding(ownerType, teamID, orgID, memberships)
		}
	}
	end := time.Now()
	tokens := pt + ct
	if tokens == 0 {
		tokens = asInt(usage["total_tokens"])
	}
	ex := s.takeExchange(callID)
	callType := strings.TrimSpace(op)
	if callType == "" {
		callType = "chat"
	}
	rowStatus := "success"
	if status >= 400 {
		rowStatus = "error"
		// Failed gateway requests are observations, not billable completions.
		// Keep the response usage for diagnostics but never turn an upstream 4xx/5xx
		// into spend or route-usage.
		spend = 0
		tokens = 0
	}
	note := s.takeNote(callID)
	if !cacheHit && tokens > 0 && !note.SkipRouteUsage {
		s.noteUsage(depID, tokens)
	}
	keyID := ""
	if p != nil {
		keyID = p.KeyID
	}
	row := live.SpendLog{
		RequestID: callID, CallType: callType, Model: alias, APIKey: hash,
		KeyID:  keyID,
		Prompt: pt, Completion: ct, Spend: spend, SpendValid: true,
		Start: start.UTC().Format(time.RFC3339Nano), End: end.UTC().Format(time.RFC3339Nano),
		CacheHit: cacheHit || note.CacheHit, Status: rowStatus, OwnerType: ownerType,
		TeamID: teamID, UserID: userID, OrgID: orgID, ProjectID: projectID,
		Messages: ex.messages, Response: ex.response, ProxyRequest: ex.proxy,
		TTFTMs: note.TTFTMs, Provider: note.Provider, CacheKey: note.CacheKey,
		SessionID: note.SessionID, CachedTokens: cachedColumn(usage),
		Guardrail:     s.takeGuardrail(callID),
		PriceSnapshot: catalog.SnapshotUsage(charge, billed, okc),
	}
	if note.SettlementID != "" {
		row.RequestID = note.SettlementID
	}
	if p != nil && p.Key != nil {
		row.KeyHash = p.Key.TokenHash
		row.KeyAlias = p.Key.Name
	}
	if teamID != "" && s.IAM != nil {
		if team, err := s.IAM.GetTeam(context.Background(), teamID); err == nil && team != nil {
			row.TeamAlias = team.Name
		}
	}
	if s.Live != nil && s.Live.EnqueueSpend(row) == nil {
		return
	}
	s.persistSpend(row, spend, ex, start, end)
}

// usageOf answers what the upstream reported, for the parts of the usage row
// that are not the charge: the cached-token column the console displays.
//
// The billable quantities come from catalog.NormalizeUsage, which is also what
// the charge is computed from. This used to be a second reader of the same
// object, and the two disagreed: the billing path resolved the Anthropic shape
// (a cache read beside a prompt count that excludes it) while the stored column
// read only prompt_tokens_details, so a row could be charged for 800 cached
// tokens and display 0.
//
// 参数 usage（map[string]any）：上游报出来的用量对象。
// 返回 *int（*int）：要写进 cached_tokens 列的命中数。上游没报这个数时为 nil，不写这一列。
// 调用：recordSpend。
// 测试：call_cost_test.go
func cachedColumn(usage map[string]any) *int {
	if usage == nil {
		return nil
	}
	// Absent and zero are different here. Zero written into the column says the
	// upstream reported a cache and nothing hit it; nil says it did not report
	// one, and the console shows that as no figure rather than as a miss.
	if !reportsCachedTokens(usage) {
		return nil
	}
	n := catalog.NormalizeUsage(usage).CachedTokens
	return &n
}

// reportsCachedTokens answers whether the usage object carries a cached-prompt
// count at all, in any of the spellings the providers use.
// 参数 usage（map[string]any）：上游报出来的用量对象。
// 返回 bool（bool）：上游报了这个数时为真。
// 调用：cachedColumn。
// 测试：call_cost_test.go
func reportsCachedTokens(usage map[string]any) bool {
	for _, key := range []string{"cached_tokens", "cache_read_input_tokens", "cache_read_tokens"} {
		if _, ok := usage[key]; ok {
			return true
		}
	}
	for _, key := range []string{"prompt_tokens_details", "input_tokens_details"} {
		if details, ok := usage[key].(map[string]any); ok {
			if _, ok := details["cached_tokens"]; ok {
				return true
			}
		}
	}
	return false
}

// persistSpend writes spend to PostgreSQL immediately. Requests take this path when Redis is not configured.
// 参数 row（live.SpendLog）：从用量或目录读出的SpendLog；spend（float64）：这一行要累加的费用，单位是美元；ex（promptExchange）：persist花费使用的promptExchange；start（time.Time）：时间范围的起点。零值表示不限制开始；end（time.Time）：时间范围的终点。零值表示直到现在。
// 返回：无。这条花费已写入 PostgreSQL。没配库时直接返回，不写。
// 调用：仅在 spend.go 内使用
// 测试：log_completeness_test.go
func (s *Server) persistSpend(row live.SpendLog, spend float64, ex promptExchange, start, end time.Time) {
	if s.IAM == nil {
		return
	}
	// The event, its stored bodies, the daily roll-up and all five billing scopes
	// commit together and deduplicate by request_id.
	rec := usageFromSpend(row, spend, ex.messages, ex.response, ex.proxy, start, end)
	if err := s.IAM.RecordUsage(context.Background(), []iam.UsageRecord{rec}); err != nil {
		logx.Error("persist spend failed: %v", err)
	}
}

// sessionLogBinding marks a console call as that person's own log. When they belong to one team, the row is filed there so the team and its organization administrator can read it. More than one membership leaves the team blank: the caller still sees the row, and a guess would file it under the wrong team.
// 参数 ownerType（string）：会话日志Binding使用的归属类型。空串表示调用方没有提供这项；teamID（string）：团队 id。空串表示没有指定团队；orgID（string）：组织 id。空串表示不按组织过滤；memberships（[]iam.Membership）：会话日志Binding使用的Membership。
// 返回 string（string）：归属类型。入参为空时按个人密钥；string（string）：要写入用量行的团队 id。只属于一个团队且入参为空时用那个团队；string（string）：要写入用量行的组织 id。跟团队一起补上，已经有值时不改。
// 调用：仅在 spend.go 内使用
// 测试：spend_session_test.go
func sessionLogBinding(ownerType, teamID, orgID string, memberships []iam.Membership) (string, string, string) {
	if ownerType == "" {
		ownerType = iam.OwnerPersonal
	}
	if teamID == "" && len(memberships) == 1 {
		teamID = memberships[0].TeamID
		if orgID == "" {
			orgID = memberships[0].OrganizationID
		}
	}
	return ownerType, teamID, orgID
}

// teamOrg returns the organization that owns a team, for the ownership snapshot written onto a usage row. A lookup failure records an empty organization rather than dropping the row: the spend itself is
//
//	already known and must be billed.
//
// 参数 teamID（string）：团队 id。空串表示没有指定团队。
// 返回 string（string）：这个团队所属的组织 id，写进用量行的归属快照。库没有或查不到时为空串。
// 调用：仅在 spend.go 内使用
// 测试：无直接单测
func (s *Server) teamOrg(teamID string) string {
	if s.IAM == nil || teamID == "" {
		return ""
	}
	team, err := s.IAM.GetTeam(context.Background(), teamID)
	if err != nil {
		logx.Error("usage organization lookup failed team=%s err=%v", teamID, err)
		return ""
	}
	return team.OrganizationID
}

// usageFromSpend 把一条热花费和暂存的请求、响应正文收成可以写入 PostgreSQL 的用量行。
// 参数 row（live.SpendLog）：Redis 队列里的花费日志，含密钥、团队、模型和 token；spend（float64）：这条日志的美元费用；messages（string）：已打码的请求 JSON。没开正文保存时为空串；response（string）：已打码的响应 JSON。没开正文保存时为空串；proxy（string）：发给上游的请求 JSON。没开正文保存时为空串；start（time.Time）：请求开始时间；end（time.Time）：请求结束时间，用来算耗时。
// 返回 UsageRecord（iam.UsageRecord）：准备写入 usage_events 的一行。费用、正文和归属都来自上面的参数。
// 调用：persistSpend。
// 测试：无直接单测
func usageFromSpend(row live.SpendLog, spend float64, messages, response, proxy string, start, end time.Time) iam.UsageRecord {
	return iam.UsageRecord{
		RequestID: row.RequestID, TS: start, KeyID: row.KeyID, OwnerType: row.OwnerType,
		UserID: row.UserID, TeamID: row.TeamID, ProjectID: row.ProjectID, OrganizationID: row.OrgID,
		Model: row.Model, CallType: row.CallType, Status: row.Status,
		PromptTokens: row.Prompt, CompletionTokens: row.Completion, Cost: spend,
		DurationMS:  int(end.Sub(start).Milliseconds()),
		RequestBody: messages, ResponseBody: response, ProxyRequest: proxy,
		EndedAt: end, TTFTMs: row.TTFTMs, CacheHit: row.CacheHit,
		KeyHash: row.KeyHash, KeyAlias: row.KeyAlias, TeamAlias: row.TeamAlias,
		Provider: row.Provider, CachedTokens: row.CachedTokens,
		SessionID: row.SessionID, CacheKey: row.CacheKey, Guardrail: row.Guardrail,
		PriceSnapshot: row.PriceSnapshot,
	}
}

// deref 读取可选字符串。nil 指针当成空串，避免把没有填写的列写成 "<nil>"。
// 参数 s（*string）：用量行上的可选文本，例如团队别名。nil 表示这一列没有值。
// 返回 string（string）：指针指向的字符串。指针为 nil 时为空串。
// 调用：仅在 spend.go 内使用。
// 测试：无直接单测
func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// promptExchange is the request and response saved on one spend log. Empty strings mean prompt storage was off.
type promptExchange struct {
	messages string
	response string
	proxy    string
}

// promptsEnabled reports whether new spend logs should keep the request and response. The YAML flag wins when it is set. A database override can turn the same key on later.
// 参数：无。
// 返回 bool（bool）：新的花费日志要保存请求和响应正文时返回真。YAML 已设置时以 YAML 为准。
// 调用：仅在 spend.go 内使用
// 测试：无直接单测
func (s *Server) promptsEnabled() bool {
	if s == nil || s.Cfg == nil {
		return false
	}
	if s.Cfg.GeneralSettings.StorePromptsInSpendLogs {
		return true
	}
	if s.Store == nil {
		return false
	}
	switch v := prefs.MergedGeneral(s)["store_prompts_in_spend_logs"].(type) {
	case bool:
		return v
	case string:
		return strings.EqualFold(v, "true")
	default:
		return false
	}
}

// rememberExchange keeps one call's headers and bodies until recordSpend writes the row.
// 参数 callID（string）：这一次调用的 id，用来把请求、响应和用量记在同一行；r（*http.Request）：入站 HTTP 请求，用来读路径、头和正文；reqBody（[]byte）：remember交换要读的原始字节；respBody（[]byte）：remember交换要读的原始字节。
// 返回：无。这次调用的头和正文已留到写用量行时再用。没开提示词记录或 callID 为空时不留。
// 调用：gateway/wire.go
// 测试：无直接单测
func (s *Server) rememberExchange(callID string, r *http.Request, reqBody, respBody []byte) {
	if s == nil || callID == "" || !s.promptsEnabled() {
		return
	}
	messages, response, proxy := promptJSON(r, reqBody, respBody)
	s.mu.Lock()
	if s.exchanges == nil {
		s.exchanges = map[string]promptExchange{}
	}
	s.exchanges[callID] = promptExchange{messages: messages, response: response, proxy: proxy}
	s.mu.Unlock()
}

// 取出并删掉这次调用暂存的请求和响应，供记用量时写入日志。
// 参数 callID（string）：这一次调用的 id，用来把请求、响应和用量记在同一行。
// 返回 promptExchange（promptExchange）：取出并删掉这次调用暂存的请求和响应，供记用量时写入日志。没有命中时为零值。
// 调用：仅在 spend.go 内使用
// 测试：无直接单测
func (s *Server) takeExchange(callID string) promptExchange {
	if s == nil {
		return promptExchange{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	ex := s.exchanges[callID]
	delete(s.exchanges, callID)
	return ex
}

// promptJSON builds the three documents the log drawer reads: messages, response, and proxy_server_request. Authorization and API key headers are replaced so the stored log does not keep a credential.
// 参数 r（*http.Request）：入站 HTTP 请求，用来读路径、头和正文；reqBody（[]byte）：输入JSON要读的原始字节；respBody（[]byte）：输入JSON要读的原始字节。
// 返回 messages（string）：打码后的请求 JSON，日志抽屉的请求面板读它；response（string）：打码后的响应 JSON；proxy（string）：打码后发给上游的请求 JSON。三段都去掉了 Authorization 和 API 密钥。
// 调用：仅在 spend.go 内使用
// 测试：prompt_log_test.go
func promptJSON(r *http.Request, reqBody, respBody []byte) (messages, response, proxy string) {
	reqDoc := jsonDocument(reqBody)
	respDoc := jsonDocument(respBody)
	if reqDoc == nil && len(reqBody) > 0 {
		reqDoc = map[string]any{"body": string(reqBody)}
	}
	if respDoc == nil && len(respBody) > 0 {
		respDoc = map[string]any{"body": string(respBody)}
	}
	if assembled := assembleLoggedResponse(respBody, respDoc); assembled != nil {
		respDoc = assembled
	}
	messagesDoc := reqDoc
	if m, ok := reqDoc.(map[string]any); ok {
		if msgs, exists := m["messages"]; exists {
			messagesDoc = msgs
		}
	}
	headers := map[string]string{}
	method, path := "", ""
	if r != nil {
		method = r.Method
		path = r.URL.RequestURI()
		for k, vals := range r.Header {
			headers[k] = strings.Join(vals, ", ")
		}
		redactHeaders(headers)
	}
	proxyDoc := map[string]any{
		"method":  method,
		"url":     path,
		"headers": headers,
		"body":    reqDoc,
	}
	return mustJSON(messagesDoc), mustJSON(respDoc), mustJSON(proxyDoc)
}

// assembleLoggedResponse turns a stored event stream into the final response document the log drawer can read. A chat or Responses stream is not one JSON value, so it used to be kept as a raw body and the output panel stayed empty.
// 参数 raw（[]byte）：原始文本或 JSON 字节；parsed（any）：assembleLogged响应接到的动态值。类型在函数体内收窄。
// 返回 any（any）：assembleLogged响应。没有合格值时为 nil。
// 调用：仅在 spend.go 内使用
// 测试：prompt_response_test.go
func assembleLoggedResponse(raw []byte, parsed any) any {
	text := ""
	switch doc := parsed.(type) {
	case map[string]any:
		if body, ok := doc["body"].(string); ok {
			text = body
		}
	}
	if text == "" && looksLikeEventStream(raw) {
		text = string(raw)
	}
	if text == "" {
		return nil
	}
	var completed map[string]any
	var outputText strings.Builder
	var chat strings.Builder
	var reasoning strings.Builder
	tools := map[int]*streamTool{}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		line = strings.TrimPrefix(line, "data:")
		line = strings.TrimSpace(line)
		if line == "" || line == "[DONE]" {
			continue
		}
		var doc map[string]any
		if json.Unmarshal([]byte(line), &doc) != nil {
			continue
		}
		if doc["type"] == "response.completed" {
			if resp, ok := doc["response"].(map[string]any); ok {
				completed = resp
			}
		}
		if doc["type"] == "response.output_text.delta" {
			if delta, ok := doc["delta"].(string); ok {
				outputText.WriteString(delta)
			}
		}
		if choices, ok := doc["choices"].([]any); ok && len(choices) > 0 {
			choice, _ := choices[0].(map[string]any)
			if delta, ok := choice["delta"].(map[string]any); ok {
				if part, ok := delta["content"].(string); ok {
					chat.WriteString(part)
				}
				if part, ok := delta["reasoning_content"].(string); ok {
					reasoning.WriteString(part)
				}
				appendStreamTools(tools, delta["tool_calls"])
			}
			if msg, ok := choice["message"].(map[string]any); ok {
				if part, ok := msg["content"].(string); ok && part != "" {
					chat.Reset()
					chat.WriteString(part)
				}
			}
		}
	}
	if completed != nil {
		return completed
	}
	if outputText.Len() > 0 {
		return map[string]any{
			"output": []any{map[string]any{
				"type": "message", "role": "assistant",
				"content": []any{map[string]any{"type": "output_text", "text": outputText.String()}},
			}},
		}
	}
	if chat.Len() > 0 || reasoning.Len() > 0 || len(tools) > 0 {
		msg := map[string]any{"role": "assistant", "content": chat.String()}
		if reasoning.Len() > 0 {
			msg["reasoning_content"] = reasoning.String()
		}
		if calls := streamToolCalls(tools); len(calls) > 0 {
			msg["tool_calls"] = calls
		}
		return map[string]any{"choices": []any{map[string]any{"message": msg}}}
	}
	return nil
}

type streamTool struct {
	id   string
	name string
	args strings.Builder
}

// 把流式工具调用增量按 index 拼进已有的工具调用。
// 参数 dst（map[int]*streamTool）：追加流工具使用的map[int]*streamTool；raw（any）：JSON 里读出的动态值。数字、字符串和对象都要接住，类型不符时按零值而不是 panic。
// 返回：无。流式工具调用增量已按 index 拼进已有的工具调用。
// 调用：仅在 spend.go 内使用
// 测试：无直接单测
func appendStreamTools(dst map[int]*streamTool, raw any) {
	list, _ := raw.([]any)
	for _, item := range list {
		call, _ := item.(map[string]any)
		idx := 0
		switch n := call["index"].(type) {
		case float64:
			idx = int(n)
		case int:
			idx = n
		}
		tool := dst[idx]
		if tool == nil {
			tool = &streamTool{}
			dst[idx] = tool
		}
		if id, ok := call["id"].(string); ok && id != "" {
			tool.id = id
		}
		fn, _ := call["function"].(map[string]any)
		if name, ok := fn["name"].(string); ok && name != "" {
			tool.name = name
		}
		if args, ok := fn["arguments"].(string); ok {
			tool.args.WriteString(args)
		}
	}
}

// 把拼好的流式工具调用收成响应里的 tool_calls 数组。
// 参数 tools（map[int]*streamTool）：流工具Calls使用的map[int]*streamTool。
// 返回 []any（[]any）：把拼好的流式工具调用收成响应里的 tool_calls 数组。没有行时为空切片。
// 调用：仅在 spend.go 内使用
// 测试：无直接单测
func streamToolCalls(tools map[int]*streamTool) []any {
	if len(tools) == 0 {
		return nil
	}
	indexes := make([]int, 0, len(tools))
	for idx := range tools {
		indexes = append(indexes, idx)
	}
	sort.Ints(indexes)
	out := make([]any, 0, len(indexes))
	for _, idx := range indexes {
		tool := tools[idx]
		out = append(out, map[string]any{
			"id":   tool.id,
			"type": "function",
			"function": map[string]any{
				"name":      tool.name,
				"arguments": tool.args.String(),
			},
		})
	}
	return out
}

// 粗略判断正文是不是 SSE，避免把事件流当成普通 JSON 解析。
// 参数 raw（[]byte）：原始正文。可能是 JSON，也可能是 SSE，由调用方按内容解析。
// 返回 bool（bool）：正文像 SSE：含 data:，并且像事件流或聊天块时为真。
// 调用：仅在 spend.go 内使用
// 测试：无直接单测
func looksLikeEventStream(raw []byte) bool {
	s := string(raw)
	return strings.Contains(s, "data:") && (strings.Contains(s, "event:") || strings.Contains(s, "\"choices\""))
}

// 把正文解析成 JSON。空正文或解析失败时为 nil。
// 参数 raw（[]byte）：原始正文。可能是 JSON，也可能是 SSE，由调用方按内容解析。
// 返回 any（any）：解析后的 JSON。空正文或解析失败时为 nil。
// 调用：仅在 spend.go 内使用
// 测试：无直接单测
func jsonDocument(raw []byte) any {
	if len(raw) == 0 {
		return nil
	}
	var v any
	if json.Unmarshal(raw, &v) != nil {
		return nil
	}
	return v
}

// 把值序列化成 JSON 文本。nil 或失败时为空串。
// 参数 v（any）：JSON 里读出的动态值。数字、字符串和对象都要接住，类型不符时按零值而不是 panic。
// 返回 string（string）：序列化后的 JSON 文本。值是 nil 或序列化失败时为空串。
// 调用：仅在 spend.go 内使用
// 测试：无直接单测
func mustJSON(v any) string {
	if v == nil {
		return ""
	}
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}

// 就地打码敏感头。Authorization、Cookie 和各类 api-key 变成星号。
// 参数 h（map[string]string）：字符串到字符串的表。缺键表示这项没有填，不要补成空 JSON。
// 返回：无。Authorization、Cookie 和各类 api-key 已就地换成星号。
// 调用：仅在 spend.go 内使用
// 测试：无直接单测
func redactHeaders(h map[string]string) {
	for k, v := range h {
		if sensitiveHeader(k) {
			h[k] = "***"
			continue
		}
		h[k] = redactSecretText(v)
	}
}

// 判断这个头是不是凭证或 Cookie。
// 参数 name（string）：HTTP 头名称。
// 返回 bool（bool）：这个头会携带凭证或 Cookie，必须打码时为真。
// 调用：仅在 spend.go 内使用
// 测试：无直接单测
func sensitiveHeader(name string) bool {
	switch strings.ToLower(name) {
	case "authorization", "proxy-authorization", "cookie", "set-cookie", "x-api-key", "api-key", "x-litellm-api-key", "x-goog-api-key":
		return true
	default:
		return false
	}
}

var promptSecret = regexp.MustCompile(`sk-[A-Za-z0-9_\-]+`)

// 把文本里的密钥样式替换成星号。
// 参数 s（string）：即将写入日志的一行，可能含 bearer、sk- 或上游 URL。
// 返回 string（string）：打码后的文本。看起来像密钥的片段换成星号。
// 调用：仅在 spend.go 内使用
// 测试：无直接单测
func redactSecretText(s string) string {
	return promptSecret.ReplaceAllString(s, "***")
}

// writeCacheHit returns a cached body and records a cache-hit spend log.
// 参数 w（http.ResponseWriter）：调用方的 HTTP 响应，状态和正文写在这里；p（*auth.Principal）：已经鉴权的调用方，含用户、团队、密钥哈希和别名；callID（string）：这一次调用的 id，用来把请求、响应和用量记在同一行；alias（string）：对外模型名；ck（string）：写入缓存命中使用的ck。空串表示调用方没有提供这项；op（string）：操作名，例如 chat；hit（[]byte）：缓存命中的响应正文；start（time.Time）：时间范围的起点。零值表示不限制开始。
// 返回：无。状态码和正文写进调用方的响应。
// 调用：gateway/wire.go
// 测试：无直接单测
func (s *Server) writeCacheHit(w http.ResponseWriter, p *auth.Principal, callID, alias, ck, op string, hit []byte, start time.Time) {
	w.Header().Set("cache_hit", "true")
	w.Header().Set("x-litellm-cache-hit", "true")
	w.Header().Set("x-litellm-cache-key", ck)
	w.Header().Set("x-litellm-model-name", alias)
	w.Header().Set("x-litellm-version", Version)
	var parsed map[string]any
	var usage map[string]any
	if json.Unmarshal(hit, &parsed) == nil {
		usage, _ = parsed["usage"].(map[string]any)
	}
	// Always write the cache-hit request log, even when the cached response has
	// no usage object. Missing usage is different from a missing request fact;
	// recordSpend will keep the row and, because cacheHit is true, charge zero.
	s.recordSpend(w, p, callID, alias, op, usage, start, true, http.StatusOK, "")
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("x-litellm-response-duration-ms", strconv.FormatInt(time.Since(start).Milliseconds(), 10))
	w.WriteHeader(200)
	_, _ = w.Write(hit)
}

// writeChatJSON writes the upstream JSON back to the client and records the spend.
// 参数 w（http.ResponseWriter）：调用方的 HTTP 响应，状态和正文写在这里；p（*auth.Principal）：已经鉴权的调用方，含用户、团队、密钥哈希和别名；callID（string）：这一次调用的 id，用来把请求、响应和用量记在同一行；alias（string）：对外模型名；ck（string）：写入对话JSON使用的ck。空串表示调用方没有提供这项；op（string）：操作名，例如 chat；provider（string）：供应商标识，例如 openai 或 volcengine；respBody（[]byte）：写入对话JSON要读的原始字节；status（int）：HTTP 状态码；start（time.Time）：时间范围的起点。零值表示不限制开始；depID（string）：部署 id。空串表示当前没有钉住的部署。
// 返回：无。状态码和正文写进调用方的响应。
// 调用：gateway/wire.go
// 测试：无直接单测
func (s *Server) writeChatJSON(w http.ResponseWriter, p *auth.Principal, callID, alias, ck, op, provider string, respBody []byte, status int, start time.Time, depID string) {
	if op == "audio_speech" {
		// Speech responses do not carry chat-token usage. Do not manufacture
		// 8/2/10 tokens: that corrupts both usage reports and billing.
		s.recordSpend(w, p, callID, alias, op, nil, start, false, status, depID)
		if w.Header().Get("Content-Type") == "" {
			w.Header().Set("Content-Type", "audio/mpeg")
		}
		w.Header().Set("x-litellm-response-duration-ms", strconv.FormatInt(time.Since(start).Milliseconds(), 10))
		w.WriteHeader(status)
		_, _ = w.Write(respBody)
		return
	}
	respBody = router.DecodeResponse(op, provider, alias, respBody)
	var parsed map[string]any
	if json.Unmarshal(respBody, &parsed) == nil {
		if op == "chat" || op == "" || op == "completions" || op == "embeddings" || op == "messages" || op == "responses" || op == "moderations" || op == "videos" {
			parsed["model"] = alias
		}
		if op == "chat" || op == "" {
			if _, ok := parsed["system_fingerprint"]; !ok {
				parsed["system_fingerprint"] = nil
			}
			if _, ok := parsed["object"]; !ok {
				parsed["object"] = "chat.completion"
			}
			if _, ok := parsed["created"]; !ok {
				parsed["created"] = time.Now().UTC().Unix()
			}
		}
		if op == "embeddings" {
			if _, ok := parsed["object"]; !ok {
				parsed["object"] = "list"
			}
			if _, ok := parsed["data"]; !ok {
				parsed["data"] = []any{}
			}
		}
		if op == "completions" {
			if _, ok := parsed["object"]; !ok {
				parsed["object"] = "text_completion"
			}
		}
		usage, _ := parsed["usage"].(map[string]any)
		if usage == nil {
			if um, ok := parsed["usageMetadata"].(map[string]any); ok {
				usage = map[string]any{
					"prompt_tokens":     um["promptTokenCount"],
					"completion_tokens": um["candidatesTokenCount"],
					"total_tokens":      um["totalTokenCount"],
				}
			}
		}
		// Some providers omit usage (especially non-chat operations). Keep the
		// request log, but do not turn an unknown measurement into fake tokens.
		// Providers that do return usage are normalized below.
		if usage == nil {
			usage = map[string]any{}
		}
		s.recordSpend(w, p, callID, alias, op, usage, start, false, status, depID)
		respBody, _ = json.Marshal(parsed)
		if status >= 200 && status < 300 {
			s.Cache.Set(ck, respBody)
		}
	} else {
		s.recordSpend(w, p, callID, alias, op, nil, start, false, status, depID)
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("x-litellm-response-duration-ms", strconv.FormatInt(time.Since(start).Milliseconds(), 10))
	w.WriteHeader(status)
	_, _ = w.Write(respBody)
}
