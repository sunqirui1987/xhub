// Package models lists the models a caller may see and expands wildcards such as openai/*.
package models

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/sunqirui1987/xhub/internal/auth"
	"github.com/sunqirui1987/xhub/internal/catalog"
	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/httpx"
	"github.com/sunqirui1987/xhub/internal/logx"
	"sync"
)

// LiteLLM litellm.constants.DEFAULT_MODEL_CREATED_AT_TIME
const defaultModelCreatedAt int64 = 1677610602

const (
	allProxyModels  = "all-proxy-models"
	allTeamModels   = "all-team-models"
	noDefaultModels = "no-default-models"
)

var logTraceOnceList sync.Once

// List serves GET /v1/models. Either an inference identity or a management identity is accepted. scope accepts only empty or expand; any other value returns 400. created uses the fixed LiteLLM default time, not the time the model was stored.
func List(s Host, w http.ResponseWriter, r *http.Request) {
	logTraceOnceList.Do(func() { logx.Trace("enter models.List") })

	httpx.SetCallID(w, httpx.CallID())
	p, err := s.Resolve(r)
	if err != nil || (!s.AllowLLM(p) && !p.CanManage()) {
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
	names := availableNames(s, r, p)
	data := make([]map[string]any, 0, len(names))
	for _, name := range names {
		data = append(data, map[string]any{
			"id": name, "object": "model", "created": defaultModelCreatedAt, "owned_by": "openai",
		})
	}
	httpx.WriteJSON(w, 200, map[string]any{"object": "list", "data": data})
}

// availableNames computes visible model names from the key and team allow-lists. An admin with scope=expand ignores the key restriction and sees every deployment that is not fully blocked. only_model_access_groups keeps only access-group names.
func availableNames(s Host, r *http.Request, p *auth.Principal) []string {
	q := r.URL.Query()
	includeGroups := queryBool(q.Get("include_model_access_groups"))
	onlyGroups := queryBool(q.Get("only_model_access_groups"))
	returnWild := queryBool(q.Get("return_wildcard_routes"))
	scope := q.Get("scope")

	s.LockModels()
	list := append([]config.ModelEntry(nil), (*s.ModelTable())...)
	s.UnlockModels()

	proxyNames := proxyModelNames(list)
	groups := modelAccessGroups(list)

	adminExpand := scope == "expand" && hasAdminModelView(p)
	var base []string
	if adminExpand {
		base = append([]string{}, proxyNames...)
	} else {
		for _, name := range proxyNames {
			if AllowsModel(s, p, name) {
				base = append(base, name)
			}
		}
		if includeGroups {
			for group, members := range groups {
				for _, member := range members {
					if AllowsModel(s, p, member) {
						base = append(base, group)
						break
					}
				}
			}
		}
	}
	if returnWild && !adminExpand && p != nil && p.Key != nil {
		for _, granted := range p.Key.Models() {
			if !strings.Contains(granted, "*") {
				continue
			}
			for _, name := range proxyNames {
				if AllowsModel(s, p, name) && wildcardMatches(granted, name) {
					base = append(base, granted)
					break
				}
			}
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

func wildcardMatches(pattern, name string) bool {
	if pattern == "*" {
		return true
	}
	prefix, ok := strings.CutSuffix(pattern, "*")
	return ok && strings.HasPrefix(name, prefix)
}

// hasAdminModelView reports that the master key, proxy_admin, and proxy_admin_viewer may bypass the key model restriction when scope=expand. An ordinary identity may not.
func hasAdminModelView(p *auth.Principal) bool {
	if p == nil {
		return false
	}
	if p.Kind == "master" || p.Role == "proxy_admin" || p.Role == "proxy_admin_viewer" {
		return true
	}
	return false
}

// queryBool treats the query values 1, true, yes, and on as true, ignoring case. Everything else, including an empty string, is false.
func queryBool(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

// proxyModelNames returns public model names in config order. A name whose deployments are all blocked is omitted.
func proxyModelNames(list []config.ModelEntry) []string {
	total := map[string]int{}
	blocked := map[string]int{}
	var order []string
	seen := map[string]struct{}{}
	for _, m := range list {
		if nonModelEntry(m) {
			continue
		}
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

// nonModelProvider identifies provider credentials that can appear in the
// deployment table but are not selectable models. Keep this name-based guard
// for older configurations that predate model_info.role: provider.
func nonModelProvider(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "qiniu", "fennoai":
		return true
	default:
		return false
	}
}

func nonModelEntry(entry config.ModelEntry) bool {
	return strings.TrimSpace(entry.ModelName) == "" ||
		nonModelProvider(entry.ModelName) ||
		providerShell(entry.ModelInfo)
}

// providerShell reports a builtin provider row. It is a credential shell, not a model the console should list.
func providerShell(info map[string]any) bool {
	if info == nil {
		return false
	}
	role, _ := info["role"].(string)
	return role == "provider"
}

// modelBlocked reads model_info.blocked. A missing or non-boolean field counts as not blocked and does not change the config.
func modelBlocked(m config.ModelEntry) bool {
	if m.ModelInfo == nil {
		return false
	}
	b, _ := m.ModelInfo["blocked"].(bool)
	return b
}

// modelAccessGroups maps access-group names to model names from each deployment's model_info.access_groups. A deployment without that field is skipped.
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

// groupKeys returns the access-group names. Map iteration order is not stable, so a caller that needs a stable order must sort.
func groupKeys(groups map[string][]string) []string {
	out := make([]string, 0, len(groups))
	for k := range groups {
		out = append(out, k)
	}
	return out
}

// stringList turns a []string or []any into a string list. Non-string elements and empty strings are dropped. Any other type returns nil.
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

// expandGrantedModels expands all-proxy-models and all-team-models. Only all-team-models on a key falls through to the team list and then to every deployment. An empty grant returns nil, which tells the caller to use every proxy model.
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

// modelsFromAccessGroups expands access groups in a grant into member model names. When include is false, an undeployed group name itself is dropped, but member models are still added.
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

// expandWildcardNames expands model names that contain * into concrete models from the price map. When returnWild is true the wildcard text is kept. With no matching deployment it looks up the built-in table by provider prefix.
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

// deploymentsNamed returns deployments whose public name equals name. Wildcards are not expanded here.
func deploymentsNamed(list []config.ModelEntry, name string) []config.ModelEntry {
	var out []config.ModelEntry
	for _, m := range list {
		if m.ModelName == name {
			out = append(out, m)
		}
	}
	return out
}

// knownModelsFromWildcard looks up models for a prefix such as openai/* in the built-in price map. An organization-id prefix that is not a known provider stays on the model name. A wildcard with no slash returns nil.
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
	models := catalog.ProviderModels(provider)
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
				if catalog.KnownProvider(leading) {
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

// dedupeModels drops empty names, duplicates, and no-default-models. The first occurrence keeps its order.
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

// containsStr reports whether the list has a string that is exactly equal. Case is not ignored.
func containsStr(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

// removeStr removes every item equal to drop and keeps the order of the rest.
func removeStr(list []string, drop string) []string {
	out := make([]string, 0, len(list))
	for _, s := range list {
		if s != drop {
			out = append(out, s)
		}
	}
	return out
}
