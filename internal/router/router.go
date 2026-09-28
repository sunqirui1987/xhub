// Package router orders deployments that share one model name. A deployment that is cooling down is skipped when another one is still available.
package router

import (
	"regexp"
	"sort"
	"strings"

	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/llm"
	"github.com/sunqirui1987/xhub/internal/logx"
	"sync"
)

var logTraceOnceRouter sync.Once

// All returns every deployment under one public model name, before a strategy orders them.
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
}

// Order sorts usable deployments into attempt order for a strategy. A cooling deployment is not placed first when another deployment exists.
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

// Pick returns the first deployment from Order. It returns nil when no deployment is usable.
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

// matchDeployments finds deployments whose public model name matches, including wildcards. It does not sort them.
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

// patternSpecificity ranks a pattern higher when it has fewer wildcards and a longer literal.
func patternSpecificity(pattern string) (int, int) {
	complexity := 0
	for _, c := range "*+?\\^$|()" {
		complexity += strings.Count(pattern, string(c))
	}
	return len(pattern), complexity
}

// wildcardRegexp compiles a model wildcard into a regular expression. An illegal pattern becomes an expression that matches nothing.
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
func DeploymentID(e config.ModelEntry) string {
	return e.ParamString("api_base", "") + "|" + e.ParamString("model", e.ModelName)
}

// paramFloat reads a float from deployment parameters. A missing value returns fallback.
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
func AdapterURL(provider, apiBase, realModel string) string {
	return AdapterURLOp("chat", provider, apiBase, realModel)
}

// AdapterURLOp returns the full URL for an operation and a provider. The rules live in internal/llm.Endpoint.
func AdapterURLOp(op, provider, apiBase, realModel string) string {
	return llm.Endpoint(op, provider, apiBase, realModel)
}

// ValidateStrategy accepts the strategy names from the catalog and the hyphenated spellings the gateway config already uses.
// An unknown name returns an error and is not treated as simple-shuffle.
func ValidateStrategy(strategy string) error {
	if _, ok := strategyKind(strategy); !ok {
		return errUnknownStrategy
	}
	return nil
}

var errUnknownStrategy = strategyError("unknown routing strategy")

type strategyError string

// Error returns the text for an unknown routing strategy.
func (e strategyError) Error() string { return string(e) }

// strategyKind folds a strategy alias into the internal name. Hyphens become underscores first. An unrecognized name returns ok false.
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
