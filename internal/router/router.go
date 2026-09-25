// 在同一个模型名的多个部署之间排序。冷却中的部署只要还有别的可用就会被跳过。
package router

import (
	"regexp"
	"sort"
	"strings"

	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/llm"
)

// 同一个对外模型名下的全部部署，尚未按策略排序。
func All(list []config.ModelEntry, alias string) []config.ModelEntry {
	return matchDeployments(list, alias)
}

// 路由器看到的运行时状态。零值表示只用进程内 Busy，不读冷却和延迟。
// State is the Redis-backed view of deployments. Zero values keep the old
// in-process behavior (busy map only).
type State struct {
	Busy     map[string]int
	Cooldown map[string]bool
	Latency  map[string]float64
	Usage    map[string]float64
}

// 按策略把可用部署排成尝试顺序。冷却中的部署只要还有其它部署就不会排在前面。
func Order(list []config.ModelEntry, alias, strategy string, st State) []config.ModelEntry {
	pool := All(list, alias)
	first := Pick(list, alias, strategy, st)
	if first == nil {
		return pool
	}
	fid := DeploymentID(*first)
	out := []config.ModelEntry{*first}
	for _, e := range pool {
		if DeploymentID(e) != fid {
			out = append(out, e)
		}
	}
	return out
}

// Order 的第一个部署。没有可用部署时返回 nil。
func Pick(list []config.ModelEntry, alias, strategy string, st State) *config.ModelEntry {
	pool := matchDeployments(list, alias)
	if len(pool) == 0 {
		return nil
	}
	if len(st.Cooldown) > 0 {
		open := make([]config.ModelEntry, 0, len(pool))
		for _, e := range pool {
			if !st.Cooldown[DeploymentID(e)] {
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
		best := 0
		bestC := 1e99
		for i, e := range pool {
			c := paramFloat(e, "input_cost_per_token", float64(i))
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
	default:
		// simple_shuffle 以及按权重选的策略：权重最大者。测试依赖这个稳定结果。
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

// 找出对外模型名能匹配上的部署，含通配符。还不排序。
// matchDeployments prefers an exact model_name. Otherwise it applies LiteLLM
// wildcard routing (openai/* → openai/<id>) and rewrites litellm_params.model.
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

// 通配符越少、字面段越长越优先。
func patternSpecificity(pattern string) (int, int) {
	complexity := 0
	for _, c := range "*+?\\^$|()" {
		complexity += strings.Count(pattern, string(c))
	}
	return len(pattern), complexity
}

// 把模型通配符编译成正则。非法模式得到不会匹配的表达式。
func wildcardRegexp(pattern string) *regexp.Regexp {
	// re.match: anchored at the start, not the end. QuoteMeta then restore '*'.
	expr := "^" + strings.ReplaceAll(regexp.QuoteMeta(pattern), `\*`, `(.*)`)
	re, err := regexp.Compile(expr)
	if err != nil {
		return regexp.MustCompile(`$^`)
	}
	return re
}

// 用请求里的捕获组替换上游模型名中的星号。
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

// 部署身份，格式是 api_base|模型参数。Redis 的冷却和用量都用这个 id。
func DeploymentID(e config.ModelEntry) string {
	return e.ParamString("api_base", "") + "|" + e.ParamString("model", e.ModelName)
}

// 从部署参数取浮点数。缺失时用 fallback。
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

// AdapterURL 是聊天补全的上游地址。其他操作用 AdapterURLOp。
func AdapterURL(provider, apiBase, realModel string) string {
	return AdapterURLOp("chat", provider, apiBase, realModel)
}

// AdapterURLOp 按操作和供应商给出完整 URL。规则在 internal/llm.Endpoint。
func AdapterURLOp(op, provider, apiBase, realModel string) string {
	return llm.Endpoint(op, provider, apiBase, realModel)
}

// ValidateStrategy 只接受 catalog 里的 15 个策略名，以及网关配置里已经在用的连字符写法。
// 不认识的名字返回错误，不再当成 simple-shuffle。
func ValidateStrategy(strategy string) error {
	if _, ok := strategyKind(strategy); !ok {
		return errUnknownStrategy
	}
	return nil
}

var errUnknownStrategy = strategyError("unknown routing strategy")

type strategyError string

// 未知路由策略的错误文本。
func (e strategyError) Error() string { return string(e) }

// 把策略别名收成内部名字。连字符会先变成下划线。认不出时 ok 为 false。
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
	default:
		return "", false
	}
}
