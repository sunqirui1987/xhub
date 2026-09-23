package router

import (
	"regexp"
	"sort"
	"strings"

	"github.com/sunqirui1987/xhub/internal/config"
)

func All(list []config.ModelEntry, alias string) []config.ModelEntry {
	return matchDeployments(list, alias)
}

func Order(list []config.ModelEntry, alias, strategy string, busy map[string]int) []config.ModelEntry {
	pool := All(list, alias)
	first := Pick(list, alias, strategy, busy)
	if first == nil {
		return pool
	}
	fid := depID(*first)
	out := []config.ModelEntry{*first}
	for _, e := range pool {
		if depID(e) != fid {
			out = append(out, e)
		}
	}
	return out
}

func Pick(list []config.ModelEntry, alias, strategy string, busy map[string]int) *config.ModelEntry {
	pool := matchDeployments(list, alias)
	if len(pool) == 0 {
		return nil
	}
	switch strategy {
	case "least-busy":
		best := 0
		bestN := 1 << 30
		for i, e := range pool {
			n := busy[depID(e)]
			if n < bestN {
				bestN = n
				best = i
			}
		}
		return &pool[best]
	case "lowest-cost":
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
	default:
		// simple-shuffle and others: first matching (stable for tests); weight if present
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
		if !strings.Contains(e.ModelName, "*") {
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

func patternSpecificity(pattern string) (int, int) {
	complexity := 0
	for _, c := range "*+?\\^$|()" {
		complexity += strings.Count(pattern, string(c))
	}
	return len(pattern), complexity
}

func wildcardRegexp(pattern string) *regexp.Regexp {
	// re.match: anchored at the start, not the end. QuoteMeta then restore '*'.
	expr := "^" + strings.ReplaceAll(regexp.QuoteMeta(pattern), `\*`, `(.*)`)
	re, err := regexp.Compile(expr)
	if err != nil {
		return regexp.MustCompile(`$^`)
	}
	return re
}

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

func depID(e config.ModelEntry) string {
	return e.ParamString("api_base", "") + "|" + e.ParamString("model", e.ModelName)
}

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

func AdapterURL(provider, apiBase, realModel string) string {
	return AdapterURLOp("chat", provider, apiBase, realModel)
}

func AdapterURLOp(op, provider, apiBase, realModel string) string {
	base := strings.TrimRight(apiBase, "/")
	azurePrefix := base + "/openai/deployments/" + realModel
	switch op {
	case "embeddings":
		if provider == "azure" {
			return azurePrefix + "/embeddings"
		}
		return base + "/embeddings"
	case "completions":
		if provider == "azure" {
			return azurePrefix + "/completions"
		}
		return base + "/completions"
	case "messages":
		if provider == "gemini" || provider == "vertex_ai" {
			return AdapterURLOp("chat", provider, apiBase, realModel)
		}
		return base + "/v1/messages"
	case "images":
		if provider == "azure" {
			return azurePrefix + "/images/generations"
		}
		return base + "/images/generations"
	case "images_edits":
		if provider == "azure" {
			return azurePrefix + "/images/edits"
		}
		return base + "/images/edits"
	case "audio_speech":
		return base + "/audio/speech"
	case "audio_transcription":
		return base + "/audio/transcriptions"
	case "audio_translation":
		return base + "/audio/translations"
	case "moderations":
		return base + "/moderations"
	case "rerank":
		return base + "/rerank"
	case "responses":
		return base + "/responses"
	case "videos":
		return base + "/videos"
	case "gemini":
		return AdapterURLOp("chat", provider, apiBase, realModel)
	default:
		switch provider {
		case "anthropic":
			return base + "/v1/messages"
		case "azure":
			return azurePrefix + "/chat/completions"
		case "gemini":
			return base + "/v1beta/models/" + realModel + ":generateContent"
		case "vertex_ai":
			return base + "/v1/projects/x/locations/us/publishers/google/models/" + realModel + ":generateContent"
		default:
			return base + "/chat/completions"
		}
	}
}

func KnownAdapter(provider string) bool {
	// Every LiteLLM 1.102.0 llms package is an adapter: OpenAI-compatible HTTP
	// by default, with provider-specific URL/body in AdapterURLOp/EncodeRequest.
	return strings.TrimSpace(provider) != ""
}
