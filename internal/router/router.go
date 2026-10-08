// Package router orders deployments that share one model name. A deployment that is cooling down is skipped when another one is still available.
package router

import (
	"math"
	"regexp"
	"sort"
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
// It reads the flat input cost first because that is what the strategy has always
// compared, and a deployment that typed only a rate table falls back to the rate
// table's input side for the window in effect.
//
// 参数 e（config.ModelEntry）：一条部署；st（State）：此刻的冷却、延迟、用量和并发，用来排序。
// 返回 float64（float64）：可比价的每 token 输入价。没有写价时是正无穷。
// 调用：Pick 的 cost 分支。
// 测试：无直接单测
func comparableRate(e config.ModelEntry, st State) float64 {
	if rate, ok := costParamFloat(e, "input_cost_per_token"); ok {
		return rate
	}
	charge, ok := catalog.CostFromRates(deploymentRates(e), catalog.Usage{PromptTokens: 1}, st.instant())
	if !ok || charge.Input <= 0 {
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
	switch t := v.(type) {
	case float64:
		return t, true
	case int:
		return float64(t), true
	default:
		return 0, false
	}
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
	}
	first := Pick(list, alias, strategy, st)
	if first == nil {
		if IsSplitStrategy(strategy) { return nil }
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
			n := st.Busy[DeploymentID(e)]
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
			if st.Latency != nil {
				if v, ok := st.Latency[DeploymentID(e)]; ok {
					c = v
				}
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
			if st.Usage != nil {
				if v, ok := st.Usage[DeploymentID(e)]; ok {
					c = v
				}
			}
			if c < bestC {
				bestC = c
				best = i
			}
		}
		return &pool[best]
	case "tag":
		best := 0
		bestW := -1.0
		for i, e := range pool {
			if e.ParamString("tag", "") == "" && i > 0 {
				continue
			}
			w := paramFloat(e, "weight", 1)
			if w > bestW {
				bestW = w
				best = i
			}
		}
		return &pool[best]
	case "split":
		return pickSplit(pool, st)
	default:
		// simple_shuffle and the other weight strategies pick the highest weight. Callers rely on this stable result.
		best := 0
		bestW := -1.0
		for i, e := range pool {
			w := paramFloat(e, "weight", 1)
			if w > bestW {
				bestW = w
				best = i
			}
		}
		return &pool[best]
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
		weights[i] = paramFloat(e, "weight", 1)
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
		if best < 0 { return nil }
		return &pool[best]
	}
	picked := st.Splits.PickWeighted(ids, weights, available)
	if picked < 0 {
		return nil
	}
	return &pool[picked]
}

// Retry candidates obey the same exclusions as the first weighted choice.
func splitCandidates(pool []config.ModelEntry, st State) []config.ModelEntry {
	out := make([]config.ModelEntry, 0, len(pool))
	for _, e := range pool {
		w := paramFloat(e, "weight", 1)
		if w > 0 && !math.IsNaN(w) && !math.IsInf(w, 0) && !st.Cooldown[CooldownID(e)] {
			out = append(out, e)
		}
	}
	return out
}

// matchDeployments finds deployments whose public model name matches, including wildcards. It does not sort them. matchDeployments prefers an exact model_name. Otherwise it applies LiteLLM wildcard routing (openai/* → openai/<id>) and rewrites litellm_params.model.
// 调用：仅在 router.go 内使用
// 测试：无直接单测
// 参数 list（[]config.ModelEntry）：候选部署列表，后面按策略挑一条；alias（string）：对外模型名。
// 返回：公开名匹配的部署。openai/* 这类通配符会改写成具体的上游模型 id。
func matchDeployments(list []config.ModelEntry, alias string) []config.ModelEntry {
	var exact []config.ModelEntry
	for _, e := range list {
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

// DeploymentID is the deployment identity, shaped as api_base|model parameter. Redis cooldown and usage use this id.
// 调用：dataplane/live.go、dataplane/official.go、dataplane/serve.go、gateway/wire.go
// 测试：bypass_logic_test.go
// 参数 e（config.ModelEntry）：一条部署。用它的 api_base 和 model 参数，model 空则用公开名。
// 返回：api_base|model。Redis 冷却、延迟和用量都用这个 id。两端都空时是 "|"。
func DeploymentID(e config.ModelEntry) string {
	return e.ParamString("api_base", "") + "|" + e.ParamString("model", e.ModelName)
}

// CooldownID isolates failures of named credentials sharing an endpoint. Usage
// and latency retain DeploymentID; no API key is ever included in this ID.
func CooldownID(e config.ModelEntry) string {
	id := DeploymentID(e)
	if name := e.ParamString("litellm_credential_name", ""); name != "" {
		return id + "|credential:" + name
	}
	return id
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

// ApplyWeights copies list and sets weight on the deployments named in overrides.
//
// The key is DeploymentID (api_base|model). A deployment that is not in the map
// keeps the weight already on it, which defaults to 1 inside the split. An empty
// map returns the same slice, so a document that does not configure shares does
// not allocate or change the pool.
//
// The copy matters: ModelList is the process config, and writing weight onto it
// would leak one request's template into the next request.
// 参数 list（[]config.ModelEntry）：候选部署；overrides（map[string]float64）：部署 id 到份额，空表示不改。
// 返回 []config.ModelEntry（[]config.ModelEntry）：带上份额之后的部署。没有覆盖时就是原来的切片。
// 调用：dataplane/serve.go。
// 测试：split_test.go
func ApplyWeights(list []config.ModelEntry, overrides map[string]float64) []config.ModelEntry {
	if len(overrides) == 0 {
		return list
	}
	out := make([]config.ModelEntry, len(list))
	for i, entry := range list {
		weight, ok := overrides[DeploymentID(entry)]
		if !ok {
			out[i] = entry
			continue
		}
		params := make(map[string]any, len(entry.LiteLLMParams)+1)
		for key, value := range entry.LiteLLMParams {
			params[key] = value
		}
		params["weight"] = weight
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

// AdapterURL is the upstream address for chat completions. Other operations use AdapterURLOp.
// 调用：仅在 router.go 内使用
// 测试：无直接单测
// 参数 provider（string）：供应商标识，例如 openai 或 volcengine；apiBase（string）：上游根地址，末尾斜杠会被去掉再拼路径；realModel（string）：发给上游或对外展示的模型名。
// 返回：聊天补全的上游 URL，等价于 AdapterURLOp("chat", ...)。
func AdapterURL(provider, apiBase, realModel string) string {
	return AdapterURLOp("chat", provider, apiBase, realModel)
}

// AdapterURLOp returns the full URL for an operation and a provider. The rules live in internal/llm.Endpoint.
// 调用：仅在 router.go 内使用
// 测试：无直接单测
// 参数 op（string）：操作名，例如 chat；provider（string）：供应商标识，例如 openai 或 volcengine；apiBase（string）：上游根地址，末尾斜杠会被去掉再拼路径；realModel（string）：发给上游或对外展示的模型名。
// 返回：该操作和供应商的完整上游 URL，规则在 llm.Endpoint。
func AdapterURLOp(op, provider, apiBase, realModel string) string {
	return llm.Endpoint(op, provider, apiBase, realModel)
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
