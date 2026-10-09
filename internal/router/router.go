// Package router orders deployments that share one model name. A deployment that is cooling down is skipped when another one is still available.
package router

import (
	"encoding/json"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/sunqirui1987/xhub/internal/catalog"
	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/llm"
	"github.com/sunqirui1987/xhub/internal/logx"
	"sync"
)

var logTraceOnceRouter sync.Once

// All returns every deployment under one public model name, before a strategy orders them.
// 调用：仅在 router.go 内使用
// 测试：无直接单测
// 参数 list（[]config.ModelEntry）：候选部署列表，后面按策略挑一条；alias（string）：对外模型名。
// 返回：这个对外名下的全部部署，此时还没有按策略排序。
func All(list []config.ModelEntry, alias string) []config.ModelEntry {
	logTraceOnceRouter.Do(func() { logx.Trace("enter router.All") })

	return matchDeployments(list, alias)
}

// State is the runtime snapshot the router sees. The zero value uses only in-process Busy and does not read cooldown or latency.
// State is the Redis-backed view of deployments. Zero values keep the old
// in-process behavior (busy map only).
type State struct {
	Busy     map[string]int
	Cooldown map[string]bool
	Latency  map[string]float64
	Usage    map[string]float64
	// Now is the instant a time-aware strategy should price at. A window-priced
	// deployment costs twice as much during its peak hours, so the cost strategy
	// needs to know which hour it is choosing for. Zero means time.Now(), which
	// is right for a live request and wrong for a test.
	Now time.Time
	// Splits holds the weighted-split cursors, keyed by deployment id. It is the
	// router's only mutable state: every other strategy is a pure function of the
	// pool, while an even split has to remember how far it got last time.
	Splits *SplitState
	// SplitScope 按模板版本、公开模型和入口隔离轮询。
	SplitScope string
}

// instant returns the time this routing decision is being made for.
// 参数 st（State）：此刻的冷却、延迟、用量和并发，用来排序。
// 返回 time.Time（time.Time）：要用来判断时段和比价的时刻。
// 调用：comparableRate。
// 测试：无直接单测
func (st State) instant() time.Time {
	if st.Now.IsZero() {
		return time.Now()
	}
	return st.Now
}

// comparableRate is the per-token price the cost strategy compares deployments
// on. A deployment with no stated rate comes back as +Inf so it sorts last
// instead of winning by declaration order.
//
// Rate tables take precedence over flat fields, as they do during settlement.
// Explicit zero prices remain comparable; peak fields use the decision time.
//
// 参数 e（config.ModelEntry）：一条部署；st（State）：此刻的冷却、延迟、用量和并发，用来排序。
// 返回 float64（float64）：可比价的每 token 输入价。没有写价时是正无穷。
// 调用：Pick 的 cost 分支。
// 测试：无直接单测
func comparableRate(e config.ModelEntry, st State) float64 {
	usage := catalog.Usage{PromptTokens: 1}
	charge, ok := catalog.CostFromRates(deploymentRates(e), usage, st.instant())
	if !ok {
		charge, ok = catalog.CostFromFlatOrRates(func(field string) (float64, bool) {
			return costParamFloat(e, field)
		}, usage, st.instant())
	}
	if !ok || charge.Input < 0 || math.IsNaN(charge.Input) || math.IsInf(charge.Input, 0) {
		return math.Inf(1)
	}
	return charge.Input
}

// costParamFloat reads a deployment's flat price. It differs from paramFloat in
// reporting whether the key was there at all, because the cost strategy has to
// tell "priced at zero" apart from "not priced".
// 参数 e（config.ModelEntry）：一条部署；key（string）：上游或调用方的密钥。空串表示还不能转发或还没有密钥。
// 返回 float64（float64）：读到的小数。缺失时为 0；bool（bool）：部署上写了这个价时为真。
// 调用：comparableRate。
// 测试：无直接单测
func costParamFloat(e config.ModelEntry, key string) (float64, bool) {
	if e.LiteLLMParams == nil {
		return 0, false
	}
	v, ok := e.LiteLLMParams[key]
	if !ok || v == nil {
		return 0, false
	}
	var rate float64
	switch t := v.(type) {
	case float64:
		rate = t
	case float32:
		rate = float64(t)
	case int:
		rate = float64(t)
	case int64:
		rate = float64(t)
	case json.Number:
		var err error
		rate, err = t.Float64()
		if err != nil {
			return 0, false
		}
	case string:
		var err error
		rate, err = strconv.ParseFloat(strings.TrimSpace(t), 64)
		if err != nil {
			return 0, false
		}
	default:
		return 0, false
	}
	return rate, rate >= 0 && !math.IsNaN(rate) && !math.IsInf(rate, 0)
}

// deploymentRates reads a rate table typed on a deployment's litellm_params.
// 参数 e（config.ModelEntry）：一条部署。
// 返回 []catalog.Rate（[]catalog.Rate）：部署自己写的费率表。没写时为空。
// 调用：comparableRate。
// 测试：无直接单测
func deploymentRates(e config.ModelEntry) []catalog.Rate {
	if e.LiteLLMParams == nil {
		return nil
	}
	raw, ok := e.LiteLLMParams["rates"]
	if !ok || raw == nil {
		return nil
	}
	rates, _ := catalog.DecodeRates(raw)
	return rates
}

// Order sorts usable deployments into attempt order for a strategy. A cooling deployment is not placed first when another deployment exists.
// 调用：dataplane/official.go、dataplane/serve.go
// 测试：无直接单测
// 参数 list（[]config.ModelEntry）：候选部署列表，后面按策略挑一条；alias（string）：对外模型名；strategy（string）：路由策略名，决定多条部署谁先被尝试；st（State）：此刻的冷却、延迟、用量和并发，用来排序。
// 返回：尝试顺序。还有其它部署可用时，处于冷却的不会排在第一。
func Order(list []config.ModelEntry, alias, strategy string, st State) []config.ModelEntry {
	pool := All(list, alias)
	if IsSplitStrategy(strategy) {
		pool = splitCandidates(pool, st)
	} else {
		pool = openCandidates(pool, st)
	}
	first := Pick(list, alias, strategy, st)
	if first == nil {
		if IsSplitStrategy(strategy) {
			return nil
		}
		return pool
	}
	fid := CooldownID(*first)
	out := []config.ModelEntry{*first}
	for _, e := range pool {
		if CooldownID(e) != fid {
			out = append(out, e)
		}
	}
	return out
}

// Pick returns the first deployment from Order. It returns nil when no deployment is usable.
// 调用：仅在 router.go 内使用
// 测试：无直接单测
// 参数 list（[]config.ModelEntry）：候选部署列表，后面按策略挑一条；alias（string）：对外模型名；strategy（string）：路由策略名，决定多条部署谁先被尝试；st（State）：此刻的冷却、延迟、用量和并发，用来排序。
// 返回：Order 的第一条。没有可用部署时为 nil。
func Pick(list []config.ModelEntry, alias, strategy string, st State) *config.ModelEntry {
	pool := matchDeployments(list, alias)
	if IsSplitStrategy(strategy) {
		return pickSplit(pool, st)
	}
	if len(pool) == 0 {
		return nil
	}
	if len(st.Cooldown) > 0 {
		open := make([]config.ModelEntry, 0, len(pool))
		for _, e := range pool {
			if !st.Cooldown[CooldownID(e)] {
				open = append(open, e)
			}
		}
		if len(open) > 0 {
			pool = open
		}
	}
	kind, ok := strategyKind(strategy)
	if !ok {
		return nil
	}
	switch kind {
	case "busy":
		best := 0
		bestN := 1 << 30
		for i, e := range pool {
			n, _ := stateInt(st.Busy, e)
			if n < bestN {
				bestN = n
				best = i
			}
		}
		return &pool[best]
	case "cost":
		// Cheapest first. A deployment that typed its own rate table is compared
		// by the rate it would charge right now, which is why the price is read
		// through the same resolver billing uses: a deployment whose peak rate is
		// high should stop looking cheap during peak hours.
		//
		// A deployment with no stated price is treated as the dearest rather than
		// falling back to its index. The old fallback was the index, which made an
		// unpriced deployment win a cost comparison by accident of declaration
		// order - a silent tie-break, not a stated one.
		best := 0
		bestC := math.Inf(1)
		for i, e := range pool {
			c := comparableRate(e, st)
			if c < bestC {
				bestC = c
				best = i
			}
		}
		return &pool[best]
	case "latency":
		best := 0
		bestC := 1e99
		for i, e := range pool {
			c := paramFloat(e, "latency_ms", float64(i))
			if v, ok := stateFloat(st.Latency, e); ok {
				c = v
			}
			if c < bestC {
				bestC = c
				best = i
			}
		}
		return &pool[best]
	case "tpm":
		best := 0
		bestC := 1e99
		for i, e := range pool {
			c := paramFloat(e, "tpm", float64(i))
			if v, ok := stateFloat(st.Usage, e); ok {
				c = v
			}
			if c < bestC {
				bestC = c
				best = i
			}
		}
		return &pool[best]
	case "tag":
		for i, e := range pool {
			if e.ParamString("tag", "") != "" {
				return &pool[i]
			}
		}
		return &pool[0]
	case "split":
		return pickSplit(pool, st)
	default:
		// 非加权策略不读取历史部署权重，同优先级采用稳定声明顺序。
		return &pool[0]
	}

}

// pickSplit chooses the first deployment by configured traffic share.
//
// A weight is a share, so 3 and 7 mean the same as 30 and 70 - the numbers do
// not have to total 100. Every deployment defaults to 1, so a pool where nobody
// set a weight splits evenly instead of always landing on the first one.
//
// A deployment in cooldown is offered no traffic and its cursor is dropped, so
// its share goes to the others for as long as it is out and it comes back at
// zero rather than immediately claiming everything it accrued while down.
//
// A session pinned to one deployment is applied by the caller before this runs,
// so a pinned request deliberately ignores the split. That is what pinning is
// for, and it means a workload with many pinned sessions will not show exactly
// the configured ratio.
//
// 参数 pool（[]config.ModelEntry）：候选部署列表，后面按策略挑一条；st（State）：此刻的冷却、延迟、用量和并发，用来排序。
// 返回 *config.ModelEntry（*config.ModelEntry）：选中的部署。没有可接流量的部署时为 nil，调用方退回原顺序。
// 调用：Pick 的 split 分支。
// 测试：split_test.go
func pickSplit(pool []config.ModelEntry, st State) *config.ModelEntry {
	if len(pool) == 0 {
		return nil
	}
	ids := make([]string, len(pool))
	weights := make([]float64, len(pool))
	available := make([]bool, len(pool))
	for i, e := range pool {
		ids[i] = CooldownID(e)
		weights[i] = paramFloat(e, "route_template_weight", 1)
		available[i] = !st.Cooldown[ids[i]] && weights[i] > 0 && !math.IsNaN(weights[i]) && !math.IsInf(weights[i], 0)
	}
	// st.Splits is nil when the gateway never installed one. Falling back to the
	// highest weight keeps that case working rather than failing every request.
	if st.Splits == nil {
		best := -1
		bestW := -1.0
		for i, w := range weights {
			if !available[i] {
				continue
			}
			if w > bestW {
				bestW = w
				best = i
			}
		}
		if best < 0 {
			return nil
		}
		return &pool[best]
	}
	picked := st.Splits.Scoped(st.SplitScope).PickWeighted(ids, weights, available)
	if picked < 0 {
		return nil
	}
	return &pool[picked]
}

// splitCandidates returns the retry pool for a weighted split. Cooling deployments and deployments with invalid or nonpositive weights cannot receive either the first attempt or a retry.
// 参数 pool（[]config.ModelEntry）：分流前的候选部署；st（State）：此刻的冷却状态和分流状态。
// 返回 []config.ModelEntry（[]config.ModelEntry）：仍可接收请求的候选部署，顺序与 pool 相同。
// 调用：Order 的分流策略分支。
// 测试：split_test.go、template_regression_test.go。
func splitCandidates(pool []config.ModelEntry, st State) []config.ModelEntry {
	out := make([]config.ModelEntry, 0, len(pool))
	for _, e := range pool {
		w := paramFloat(e, "route_template_weight", 1)
		if w > 0 && !math.IsNaN(w) && !math.IsInf(w, 0) && !st.Cooldown[CooldownID(e)] {
			out = append(out, e)
		}
	}
	return out
}

// openCandidates removes cooling deployments when at least one candidate is open. If every candidate is cooling, it returns the original pool so non-split strategies retain their documented fail-open behavior.
// 参数 pool（[]config.ModelEntry）：同一公开模型名下的候选部署；st（State）：此刻的冷却状态。
// 返回 []config.ModelEntry（[]config.ModelEntry）：有健康候选时只含健康部署；全部冷却时返回原池。
// 调用：Order。
// 测试：template_regression_test.go。
func openCandidates(pool []config.ModelEntry, st State) []config.ModelEntry {
	if len(st.Cooldown) == 0 {
		return pool
	}
	open := make([]config.ModelEntry, 0, len(pool))
	for _, e := range pool {
		if !st.Cooldown[CooldownID(e)] {
			open = append(open, e)
		}
	}
	if len(open) == 0 {
		return pool
	}
	return open
}

// matchDeployments finds deployments whose public model name matches, including wildcards. It does not sort them. matchDeployments prefers an exact model_name. Otherwise it applies LiteLLM wildcard routing (openai/* → openai/<id>) and rewrites litellm_params.model.
// 调用：仅在 router.go 内使用
// 测试：无直接单测
// 参数 list（[]config.ModelEntry）：候选部署列表，后面按策略挑一条；alias（string）：对外模型名。
// 返回：公开名匹配的部署。openai/* 这类通配符会改写成具体的上游模型 id。
func matchDeployments(list []config.ModelEntry, alias string) []config.ModelEntry {
	var exact []config.ModelEntry
	for _, e := range list {
		if e.Disabled() {
			continue
		}
		if e.ModelName == alias {
			exact = append(exact, e)
		}
	}
	if len(exact) > 0 {
		return exact
	}
	byPattern := map[string][]config.ModelEntry{}
	var patterns []string
	for _, e := range list {
		if e.Disabled() {
			continue
		}
		if !llm.IsWildcardModel(e.ModelName) {
			continue
		}
		if _, ok := byPattern[e.ModelName]; !ok {
			patterns = append(patterns, e.ModelName)
		}
		byPattern[e.ModelName] = append(byPattern[e.ModelName], e)
	}
	sort.SliceStable(patterns, func(i, j int) bool {
		li, ci := patternSpecificity(patterns[i])
		lj, cj := patternSpecificity(patterns[j])
		if li != lj {
			return li > lj
		}
		return ci > cj
	})
	for _, pattern := range patterns {
		re := wildcardRegexp(pattern)
		m := re.FindStringSubmatch(alias)
		if m == nil {
			continue
		}
		out := make([]config.ModelEntry, 0, len(byPattern[pattern]))
		for _, e := range byPattern[pattern] {
			cp := e
			params := map[string]any{}
			for k, v := range e.LiteLLMParams {
				params[k] = v
			}
			upstream := e.ParamString("model", e.ModelName)
			params["model"] = applyWildcardModel(upstream, alias, m[1:])
			cp.LiteLLMParams = params
			out = append(out, cp)
		}
		return out
	}
	return nil
}

// patternSpecificity ranks a pattern higher when it has fewer wildcards and a longer literal.
// 调用：仅在 router.go 内使用
// 测试：无直接单测
// 参数 pattern（string）：要匹配的路径模板或正则。
// 返回：字面量长度，以及 *+? 等通配符出现的次数。排序时更长、通配更少的模板优先。
func patternSpecificity(pattern string) (int, int) {
	complexity := 0
	for _, c := range "*+?\\^$|()" {
		complexity += strings.Count(pattern, string(c))
	}
	return len(pattern), complexity
}

// wildcardRegexp compiles a model wildcard into a regular expression. An illegal pattern becomes an expression that matches nothing.
// 调用：仅在 router.go 内使用
// 测试：无直接单测
// 参数 pattern（string）：要匹配的路径模板或正则。
// 返回：把模型通配符编成的正则，* 变成捕获组。模式非法时返回一个什么都不匹配的表达式，而不是 nil。
func wildcardRegexp(pattern string) *regexp.Regexp {
	// re.match: anchored at the start, not the end. QuoteMeta then restore '*'.
	expr := "^" + strings.ReplaceAll(regexp.QuoteMeta(pattern), `\*`, `(.*)`)
	re, err := regexp.Compile(expr)
	if err != nil {
		return regexp.MustCompile(`$^`)
	}
	return re
}

// applyWildcardModel replaces stars in the upstream model name with capture groups from the request.
// 调用：仅在 router.go 内使用
// 测试：无直接单测
// 参数 upstream（string）：部署上的模型模板，可含 *。request（string）：调用方写的模型名，星号比捕获组少时整段改用它。groups（[]string）：请求名里被 * 捕获的片段，按顺序替换模板里的星号。
// 返回：替换后的上游模型 id。模板里没有星号时原样返回 upstream。
func applyWildcardModel(upstream, request string, groups []string) string {
	if !strings.Contains(upstream, "*") {
		return upstream
	}
	if strings.Count(upstream, "*") < len(groups) {
		return request
	}
	for _, g := range groups {
		upstream = strings.Replace(upstream, "*", g, 1)
	}
	return upstream
}

// WeightID is the template-weight identity. A deployment id wins
// because it names one configured row even when several suppliers share a
// pricing record, endpoint, and upstream model. Database-backed deployments
// keep that stable id in model_info.id; configuration files may provide it as
// litellm_params.deployment_id. A configured pricing_id is the next-best
// stable identity. Rows without either stable ID have no template-weight identity.
// 参数 e（config.ModelEntry）：候选部署。
// 返回 string：部署或定价的稳定权重身份；没有稳定 ID 时返回空串。
// 调用：模板权重匹配与部署筛选。
// 测试：router_test.go
func WeightID(e config.ModelEntry) string {
	if id := strings.TrimSpace(e.ParamString("deployment_id", "")); id != "" {
		return "deployment:" + id
	}
	if id, _ := e.ModelInfo["id"].(string); strings.TrimSpace(id) != "" {
		return "deployment:" + strings.TrimSpace(id)
	}
	if id := strings.TrimSpace(e.ParamString("pricing_id", "")); id != "" {
		return "pricing:" + id
	}
	if id, _ := e.ModelInfo["pricing_id"].(string); strings.TrimSpace(id) != "" {
		return "pricing:" + strings.TrimSpace(id)
	}
	return ""
}

// CooldownID is the runtime deployment identity. It isolates cooldown, busy,
// latency, usage, session pinning, and billing state. Existing parameter IDs
// keep their precedence and key shape. Database-backed rows use model_info.id,
// only when neither parameter ID is configured, so
// otherwise identical database deployments still have independent runtime state.
// No API key is included.
// 参数 e（config.ModelEntry）：一条部署；命名凭证从 litellm_credential_name 读取；稳定身份优先从参数 pricing_id、deployment_id 读取，再使用 model_info.id。
// 返回 string（string）：运行时部署 id；稳定身份追加为 |pricing:<id>，命名凭证追加为 |credential:<name>。
// 调用：路由冷却、分流、运行指标、会话钉住和调用记账。
// 测试：runtime_identity_test.go、template_regression_test.go。
func CooldownID(e config.ModelEntry) string {
	id := e.ParamString("api_base", "") + "|" + e.ParamString("model", e.ModelName)
	stable := e.ParamString("pricing_id", "")
	if stable == "" {
		stable = e.ParamString("deployment_id", "")
	}
	if stable == "" {
		if modelID, _ := e.ModelInfo["id"].(string); strings.TrimSpace(modelID) != "" {
			stable = strings.TrimSpace(modelID)
		}
	}
	if stable != "" {
		id += "|pricing:" + stable
	}
	if name := e.ParamString("litellm_credential_name", ""); name != "" {
		return id + "|credential:" + name
	}
	return id
}

// stateInt reads runtime integer state by credential-aware identity.
// 参数 values（map[string]int）：按运行时部署 id 保存的整数状态；e（config.ModelEntry）：要查找的部署。
// 返回 int（int）：找到的状态值，未找到时为 0；bool（bool）：是否找到对应状态。
// 调用：Pick 的 least-busy 分支。
// 测试：template_regression_test.go。
func stateInt(values map[string]int, e config.ModelEntry) (int, bool) {
	value, ok := values[CooldownID(e)]
	return value, ok
}

// stateFloat reads runtime floating-point state by credential-aware identity.
// 参数 values（map[string]float64）：按运行时部署 id 保存的浮点状态；e（config.ModelEntry）：要查找的部署。
// 返回 float64（float64）：找到的状态值，未找到时为 0；bool（bool）：是否找到对应状态。
// 调用：Pick 的 latency 和 tpm 分支。
// 测试：template_regression_test.go。
func stateFloat(values map[string]float64, e config.ModelEntry) (float64, bool) {
	value, ok := values[CooldownID(e)]
	return value, ok
}

// IsSplitStrategy reports whether strategy divides traffic by weight.
// Hyphens and underscores are the same name. Anything else, including
// simple-shuffle, is not a split: simple-shuffle still picks the heaviest
// deployment, and treating it as a split would change that.
// 参数 strategy（string）：路由策略名。
// 返回 bool（bool）：这个名字是按权重分流时为真。
// 调用：dataplane/serve.go，只在这时把文档里的份额写进部署。
// 测试：split_test.go
func IsSplitStrategy(strategy string) bool {
	switch strings.ReplaceAll(strings.TrimSpace(strategy), "-", "_") {
	case "weighted_split", "weighted_round_robin", "traffic_split":
		return true
	default:
		return false
	}
}

// ApplyWeights 从模板构造私有部署副本；未配置使用 1，明确 0 排除部署。
// 参数 list：候选；overrides：稳定部署 ID 到权重。返回副本，不读取旧 weight，不修改配置。
// 调用：统一、原生与预览；测试：unified_policy_test.go。
func ApplyWeights(list []config.ModelEntry, overrides map[string]float64) []config.ModelEntry {
	out := make([]config.ModelEntry, len(list))
	for i, entry := range list {
		weight := 1.0
		if v, ok := overrides[WeightID(entry)]; ok {
			weight = v
		}
		params := make(map[string]any, len(entry.LiteLLMParams)+1)
		for key, value := range entry.LiteLLMParams {
			params[key] = value
		}

		params["route_template_weight"] = weight
		entry.LiteLLMParams = params
		out[i] = entry
	}
	return out
}

// paramFloat reads a float from deployment parameters. A missing value returns fallback.
//
// 参数 e：一条部署。key：LiteLLMParams 里的数字字段。fallback：缺值时的结果。
// 返回：读到的浮点数，或 fallback。
// 调用：仅 router.go。测试：无直接单测。
func paramFloat(e config.ModelEntry, key string, fallback float64) float64 {
	if e.LiteLLMParams == nil {
		return fallback
	}
	v, ok := e.LiteLLMParams[key]
	if !ok {
		return fallback
	}
	switch t := v.(type) {
	case float64:
		return t
	case int:
		return float64(t)
	default:
		return fallback
	}
}

// ValidateStrategy accepts the strategy names from the catalog and the hyphenated spellings the gateway config already uses. An unknown name returns an error and is not treated as simple-shuffle.
// 返回 error（error）：失败原因，nil 表示这一步成功。
// 调用：dataplane/serve.go
// 测试：无直接单测
// 参数 strategy（string）：路由策略名，决定多条部署谁先被尝试。
func ValidateStrategy(strategy string) error {
	if _, ok := strategyKind(strategy); !ok {
		return errUnknownStrategy
	}
	return nil
}

var errUnknownStrategy = strategyError("unknown routing strategy")

type strategyError string

// Error returns the text for an unknown routing strategy.
// 参数：无。
// 调用：ValidateStrategy 在策略名不认识时返回它，经 error 接口读取。
// 测试：无直接单测
// 返回：未知策略的说明文本，给 ValidateStrategy 的调用方。
func (e strategyError) Error() string { return string(e) }

// strategyKind folds a strategy alias into the internal name. Hyphens become underscores first. An unrecognized name returns ok false.
// 调用：仅在 router.go 内使用
// 测试：无直接单测
// 参数 strategy（string）：路由策略名，决定多条部署谁先被尝试。
// 返回：内部策略名（weight、busy、cost、latency）以及这个名字是否被承认。不认识时 ok 为 false，不会当成 simple-shuffle。
func strategyKind(strategy string) (string, bool) {
	s := strings.ReplaceAll(strings.TrimSpace(strategy), "-", "_")
	switch s {
	case "", "simple_shuffle", "base_routing_strategy", "adaptive_router", "auto_router", "complexity_router", "quality_router":
		return "weight", true
	case "least_busy":
		return "busy", true
	case "lowest_cost", "budget_limiter", "savings_baseline":
		return "cost", true
	case "lowest_latency", "lar1_routing", "latency_based_routing":
		return "latency", true
	case "lowest_tpm_rpm", "lowest_tpm_rpm_v2", "usage_based_routing", "usage_based_routing_v2":
		return "tpm", true
	case "cost_based_routing":
		return "cost", true
	case "tag_based_routing":
		return "tag", true
	case "weighted_split", "weighted_round_robin", "traffic_split":
		// Its own kind rather than a change to "weight". simple_shuffle already
		// routes here, and six other aliases share it; making "weight" mean a
		// ratio would change all of them and break a pinned regression case.
		return "split", true
	default:
		return "", false
	}
}
