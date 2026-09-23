package server

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/sunqirui1987/xhub/internal/auth"
	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/httpx"
)

// LiteLLM litellm.constants.DEFAULT_MODEL_CREATED_AT_TIME
const defaultModelCreatedAt int64 = 1677610602

const (
	allProxyModels  = "all-proxy-models"
	allTeamModels   = "all-team-models"
	noDefaultModels = "no-default-models"
)

func (s *Server) listModels(w http.ResponseWriter, r *http.Request) {
	httpx.SetCallID(w, httpx.CallID())
	p, err := s.resolve(r)
	if err != nil || (!p.CanLLM(s.Cfg) && !p.CanManage()) {
		if err != nil {
			httpx.WriteTypedError(w, r.URL.Path, 401, "invalid_api_key", "Authentication Error, No api key passed in.")
			return
		}
		httpx.WriteTypedError(w, r.URL.Path, 401, "invalid_api_key", "Not allowed to access LLM endpoints")
		return
	}
	scope := r.URL.Query().Get("scope")
	if scope != "" && scope != "expand" {
		httpx.WriteError(w, 400, "invalid_request", fmt.Sprintf("Invalid scope parameter. Only 'expand' is currently supported. Received: %s", scope))
		return
	}
	names := s.availableModelNames(r, p)
	data := make([]map[string]any, 0, len(names))
	for _, name := range names {
		data = append(data, map[string]any{
			"id": name, "object": "model", "created": defaultModelCreatedAt, "owned_by": "openai",
		})
	}
	httpx.WriteJSON(w, 200, map[string]any{"object": "list", "data": data})
}

func (s *Server) availableModelNames(r *http.Request, p *auth.Principal) []string {
	q := r.URL.Query()
	includeGroups := queryBool(q.Get("include_model_access_groups"))
	onlyGroups := queryBool(q.Get("only_model_access_groups"))
	returnWild := queryBool(q.Get("return_wildcard_routes"))
	teamID := strings.TrimSpace(q.Get("team_id"))
	scope := q.Get("scope")

	s.mu.Lock()
	list := append([]config.ModelEntry(nil), s.Cfg.ModelList...)
	s.mu.Unlock()

	proxyNames := proxyModelNames(list)
	groups := modelAccessGroups(list)

	var keyModels, teamModels []string
	adminExpand := scope == "expand" && hasAdminModelView(p)
	if !adminExpand && p != nil && p.Key != nil {
		if teamID != "" {
			if team, err := s.Store.GetTeam(teamID); err == nil {
				teamModels = team.Models()
			}
		} else {
			keyModels = p.Key.Models()
			if p.Key.TeamID != "" {
				if team, err := s.Store.GetTeam(p.Key.TeamID); err == nil {
					teamModels = team.Models()
				}
			}
		}
		keyModels = expandGrantedModels(keyModels, teamModels, proxyNames, groups, includeGroups, true)
		teamModels = expandGrantedModels(teamModels, teamModels, proxyNames, groups, includeGroups, false)
	}

	var base []string
	switch {
	case len(keyModels) > 0:
		base = keyModels
	case len(teamModels) > 0:
		base = teamModels
	default:
		base = append([]string{}, proxyNames...)
		if includeGroups {
			base = append(base, groupKeys(groups)...)
		}
	}
	base = dedupeModels(base)
	if onlyGroups {
		var out []string
		for _, model := range base {
			if _, ok := groups[model]; ok {
				out = append(out, model)
			}
		}
		return out
	}
	return dedupeModels(expandWildcardNames(base, list, returnWild))
}

func hasAdminModelView(p *auth.Principal) bool {
	if p == nil {
		return false
	}
	if p.Master || p.Role == "proxy_admin" || p.Role == "proxy_admin_viewer" {
		return true
	}
	return false
}

func queryBool(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func proxyModelNames(list []config.ModelEntry) []string {
	total := map[string]int{}
	blocked := map[string]int{}
	var order []string
	seen := map[string]struct{}{}
	for _, m := range list {
		if _, ok := seen[m.ModelName]; !ok && m.ModelName != "" {
			order = append(order, m.ModelName)
			seen[m.ModelName] = struct{}{}
		}
		if m.ModelName == "" {
			continue
		}
		total[m.ModelName]++
		if modelBlocked(m) {
			blocked[m.ModelName]++
		}
	}
	var out []string
	for _, name := range order {
		if total[name] > 0 && blocked[name] == total[name] {
			continue
		}
		out = append(out, name)
	}
	return out
}

func modelBlocked(m config.ModelEntry) bool {
	if m.ModelInfo == nil {
		return false
	}
	b, _ := m.ModelInfo["blocked"].(bool)
	return b
}

func modelAccessGroups(list []config.ModelEntry) map[string][]string {
	groups := map[string][]string{}
	for _, m := range list {
		if m.ModelInfo == nil {
			continue
		}
		for _, g := range stringList(m.ModelInfo["access_groups"]) {
			groups[g] = append(groups[g], m.ModelName)
		}
	}
	return groups
}

func groupKeys(groups map[string][]string) []string {
	out := make([]string, 0, len(groups))
	for k := range groups {
		out = append(out, k)
	}
	return out
}

func stringList(v any) []string {
	switch t := v.(type) {
	case []string:
		return t
	case []any:
		var out []string
		for _, x := range t {
			if s, ok := x.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

// expandGrantedModels mirrors get_key_models / get_team_models.
// keyPass distinguishes the all-team-models sentinel, which only keys honor
// before falling through to the proxy list.
func expandGrantedModels(granted, teamModels, proxy []string, groups map[string][]string, include, keyPass bool) []string {
	if len(granted) == 0 {
		return nil
	}
	all := append([]string{}, granted...)
	if keyPass && containsStr(all, allTeamModels) {
		all = append([]string{}, teamModels...)
		if containsStr(all, allTeamModels) {
			all = removeStr(all, allTeamModels)
			all = append(all, proxy...)
			if include {
				all = append(all, groupKeys(groups)...)
			}
		}
	}
	if containsStr(all, allProxyModels) {
		all = append([]string{}, proxy...)
		if include {
			all = append(all, groupKeys(groups)...)
		}
	}
	return modelsFromAccessGroups(all, groups, include, proxy)
}

func modelsFromAccessGroups(all []string, groups map[string][]string, include bool, proxy []string) []string {
	deployed := map[string]struct{}{}
	for _, name := range proxy {
		deployed[name] = struct{}{}
	}
	var kept []string
	for _, model := range all {
		_, isGroup := groups[model]
		_, isDeployed := deployed[model]
		if !isGroup || include || isDeployed {
			kept = append(kept, model)
		}
	}
	var members []string
	for _, model := range all {
		if ms, ok := groups[model]; ok {
			members = append(members, ms...)
		}
	}
	return append(kept, members...)
}

func expandWildcardNames(models []string, list []config.ModelEntry, returnWild bool) []string {
	var out []string
	var extra []string
	for _, model := range models {
		if !strings.Contains(model, "*") {
			out = append(out, model)
			continue
		}
		if returnWild {
			out = append(out, model)
		}
		deps := deploymentsNamed(list, model)
		if len(deps) == 0 {
			extra = append(extra, knownModelsFromWildcard(model, "")...)
			continue
		}
		for _, dep := range deps {
			extra = append(extra, knownModelsFromWildcard(model, dep.ParamString("model", ""))...)
		}
	}
	return append(out, extra...)
}

func deploymentsNamed(list []config.ModelEntry, name string) []config.ModelEntry {
	var out []config.ModelEntry
	for _, m := range list {
		if m.ModelName == name {
			out = append(out, m)
		}
	}
	return out
}

func knownModelsFromWildcard(wildcard, litellmModel string) []string {
	toExpand := wildcard
	if wildcard == "*" && strings.Contains(litellmModel, "*") && strings.Contains(litellmModel, "/") {
		toExpand = litellmModel
	}
	prefix, suffix, ok := strings.Cut(toExpand, "/")
	if !ok {
		return nil
	}
	provider := prefix
	if litellmModel != "" {
		if p, _, found := strings.Cut(litellmModel, "/"); found && p != "" {
			provider = p
		}
	}
	models := providerModels(provider)
	if len(models) == 0 {
		return nil
	}
	models = append([]string{}, models...)
	if suffix != "*" {
		modelPrefix := strings.ReplaceAll(suffix, "*", "")
		partial := false
		for _, m := range models {
			if strings.HasPrefix(m, modelPrefix) {
				partial = true
				break
			}
		}
		if partial {
			filtered := make([]string, 0, len(models))
			for _, m := range models {
				if strings.HasPrefix(m, modelPrefix) {
					filtered = append(filtered, m)
				}
			}
			models = filtered
		} else {
			for i, m := range models {
				models[i] = modelPrefix + m
			}
		}
	}
	out := make([]string, 0, len(models))
	for _, model := range models {
		if !strings.HasPrefix(model, prefix) {
			leading, rest, hasSlash := strings.Cut(model, "/")
			if hasSlash {
				if _, known := knownLLMProviders[leading]; known {
					model = prefix + "/" + rest
				} else {
					model = prefix + "/" + model
				}
			} else {
				model = prefix + "/" + model
			}
		}
		out = append(out, model)
	}
	return out
}

func dedupeModels(in []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, model := range in {
		if model == "" || model == noDefaultModels {
			continue
		}
		if _, ok := seen[model]; ok {
			continue
		}
		seen[model] = struct{}{}
		out = append(out, model)
	}
	return out
}

func containsStr(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

func removeStr(list []string, drop string) []string {
	out := make([]string, 0, len(list))
	for _, s := range list {
		if s != drop {
			out = append(out, s)
		}
	}
	return out
}
