// 按调用方身份列出可见模型，并展开 openai/* 这类通配符。
package models

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/sunqirui1987/xhub/internal/auth"
	"github.com/sunqirui1987/xhub/internal/catalog"
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

// listModels GET /v1/models。推理身份或管理身份都可以。scope 只接受空或 expand，其它值返回 400。created 使用固定的 LiteLLM 默认时间，不是模型入库时间。
func List(s Host, w http.ResponseWriter, r *http.Request) {
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

// availableModelNames 按密钥和团队的允许列表计算可见模型名。管理员在 scope=expand 时忽略密钥限制，看到全部未全部屏蔽的部署。only_model_access_groups 只留访问组名。
func availableNames(s Host, r *http.Request, p *auth.Principal) []string {
	q := r.URL.Query()
	includeGroups := queryBool(q.Get("include_model_access_groups"))
	onlyGroups := queryBool(q.Get("only_model_access_groups"))
	returnWild := queryBool(q.Get("return_wildcard_routes"))
	teamID := strings.TrimSpace(q.Get("team_id"))
	scope := q.Get("scope")

	s.LockModels()
	list := append([]config.ModelEntry(nil), (*s.ModelTable())...)
	s.UnlockModels()

	proxyNames := proxyModelNames(list)
	groups := modelAccessGroups(list)

	var keyModels, teamModels []string
	adminExpand := scope == "expand" && hasAdminModelView(p)
	if !adminExpand && p != nil && p.Key != nil {
		if teamID != "" {
			if team, err := s.DB().GetTeam(teamID); err == nil {
				teamModels = team.Models()
			}
		} else {
			keyModels = p.Key.Models()
			if p.Key.TeamID != "" {
				if team, err := s.DB().GetTeam(p.Key.TeamID); err == nil {
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

// hasAdminModelView 主密钥、proxy_admin 和 proxy_admin_viewer 可以在 scope=expand 时越过密钥的模型限制。普通身份不行。
func hasAdminModelView(p *auth.Principal) bool {
	if p == nil {
		return false
	}
	if p.Master || p.Role == "proxy_admin" || p.Role == "proxy_admin_viewer" {
		return true
	}
	return false
}

// queryBool 查询参数 1、true、yes、on（不区分大小写）为真。其余，包括空串，都是假。
func queryBool(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

// proxyModelNames 对外模型名，保持配置里的出现顺序。某个名字下的部署全部 blocked 时，这个名字不出现。
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

// modelBlocked 读 model_info.blocked。字段缺失或不是布尔时视为未屏蔽，不修改配置。
func modelBlocked(m config.ModelEntry) bool {
	if m.ModelInfo == nil {
		return false
	}
	b, _ := m.ModelInfo["blocked"].(bool)
	return b
}

// modelAccessGroups 从各部署的 model_info.access_groups 建访问组到模型名的映射。没有该字段的部署被跳过。
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

// groupKeys 返回访问组名字。map 迭代顺序不稳定，调用方需要稳定顺序时要再排序。
func groupKeys(groups map[string][]string) []string {
	out := make([]string, 0, len(groups))
	for k := range groups {
		out = append(out, k)
	}
	return out
}

// stringList 把 []string 或 []any 收成字符串列表。非字符串元素和空串被丢掉。其它类型返回 nil。
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

// expandGrantedModels 展开 all-proxy-models 和 all-team-models。只有密钥上的 all-team-models 会先落到团队列表，再落到全部部署。空授权返回 nil，表示调用方改用全部代理模型。
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

// modelsFromAccessGroups 把授权里的访问组展开成成员模型名。include 为假时，未部署的组名本身被丢掉，成员模型仍然加入。
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

// expandWildcardNames 把带 * 的模型名展开成价格表里的具体模型。returnWild 为真时保留通配符原文。没有匹配部署时按供应商前缀查内置表。
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

// deploymentsNamed 返回对外名字恰好等于 name 的部署。通配符不在这里展开。
func deploymentsNamed(list []config.ModelEntry, name string) []config.ModelEntry {
	var out []config.ModelEntry
	for _, m := range list {
		if m.ModelName == name {
			out = append(out, m)
		}
	}
	return out
}

// knownModelsFromWildcard 用 openai/* 这类前缀到内置价格表里取模型。组织 id 前缀（不是已知供应商）会保留在模型名里。没有斜杠的通配符返回 nil。
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

// dedupeModels 去掉空名、重复名和 no-default-models。保留第一次出现的顺序。
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

// containsStr 列表里是否有完全相等的字符串。不忽略大小写。
func containsStr(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

// removeStr 去掉所有等于 drop 的项，保留其余顺序。
func removeStr(list []string, drop string) []string {
	out := make([]string, 0, len(list))
	for _, s := range list {
		if s != drop {
			out = append(out, s)
		}
	}
	return out
}
