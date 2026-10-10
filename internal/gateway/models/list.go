// Package models lists the models a caller may see and expands wildcards such as openai/*.
package models

import (
	"fmt"
	"github.com/sunqirui1987/xhub/internal/pagination"
	"net/http"
	"strconv"
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
	noDefaultModels = "no-default-models"
)

var logTraceOnceList sync.Once

// List serves GET /v1/models. Either an inference identity or a management identity is accepted. scope accepts only empty or expand; any other value returns 400. created uses the fixed LiteLLM default time, not the time the model was stored.
// 参数 s（Host）：列出使用的数据面宿主；w（http.ResponseWriter）：调用方的 HTTP 响应，状态和正文写在这里；r（*http.Request）：入站 HTTP 请求。
// 返回：无。状态码和正文写进调用方的响应。
// 调用：gateway/keys/generate.go、gateway/keys/mount.go、gateway/models/mount.go、gateway/prefs/mount.go
// 测试：无直接单测
func List(s Host, w http.ResponseWriter, r *http.Request) {
	logTraceOnceList.Do(func() { logx.Trace("enter models.List") })

	httpx.SetCallID(w, httpx.CallID())
	p, err := s.Resolve(r)
	if err != nil || (!s.AllowLLM(p) && !p.PlatformAdmin()) {
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

// availableNames computes visible model names from the caller's permitted set. A platform administrator with scope=expand sees every enabled deployment. only_model_access_groups returns an empty list because model-owned access groups no longer exist.
// 参数 s（Host）：可用名称使用的数据面宿主；r（*http.Request）：入站 HTTP 请求；p（*auth.Principal）：已经鉴权的调用方，含用户、团队、密钥哈希和别名。
// 返回 []string（[]string）：可用名称。没有匹配时为 nil 或空切片，调用方按长度判断。
// 调用：仅在 list.go 内使用
// 测试：无直接单测
func availableNames(s Host, r *http.Request, p *auth.Principal) []string {
	q := r.URL.Query()
	onlyGroups := queryBool(q.Get("only_model_access_groups"))
	returnWild := queryBool(q.Get("return_wildcard_routes"))
	scope := q.Get("scope")
	// A session browses one team's catalog; a key is bound to its own team and
	// ignores this parameter.
	teamID := q.Get("team_id")
	ctx := r.Context()

	s.LockModels()
	list := append([]config.ModelEntry(nil), (*s.ModelTable())...)
	s.UnlockModels()

	proxyNames := proxyModelNames(list)
	adminExpand := scope == "expand" && hasAdminModelView(p)
	var base []string
	if adminExpand {
		base = append([]string{}, proxyNames...)
	} else {
		for _, name := range proxyNames {
			if AllowsModel(s, ctx, p, teamID, name) {
				base = append(base, name)
			}
		}
	}
	if returnWild && !adminExpand && p != nil && p.Key != nil {
		for _, granted := range p.Key.Models {
			if !strings.Contains(granted, "*") {
				continue
			}
			for _, name := range proxyNames {
				if AllowsModel(s, ctx, p, teamID, name) && wildcardMatches(granted, name) {
					base = append(base, granted)
					break
				}
			}
		}
	}
	base = dedupeModels(base)
	if onlyGroups {
		return []string{}
	}
	return dedupeModels(expandWildcardNames(base, list, returnWild))
}

// 判断模型名是否匹配通配符。星号可以在末尾。
// 参数 pattern（string）：要匹配的路径模板或正则；name（string）：通配Matches要查找或展示的名称。空串表示还没有命名。
// 返回 bool（bool）：名字匹配这个通配模式时为真。单独一个星号匹配全部。
// 调用：仅在 list.go 内使用
// 测试：无直接单测
func wildcardMatches(pattern, name string) bool {
	if pattern == "*" {
		return true
	}
	prefix, ok := strings.CutSuffix(pattern, "*")
	return ok && strings.HasPrefix(name, prefix)
}

// hasAdminModelView reports that the platform administrator may bypass the key model restriction when scope=expand. An ordinary identity may not, and a key never does, whatever role its owner holds.
// 参数 p（*auth.Principal）：已经解析的调用方，含用户、团队和密钥。
// 返回 bool（bool）：平台管理员在 scope=expand 时可以绕过密钥的模型限制时返回真。普通身份和密钥都不可以。
// 调用：仅在 list.go 内使用
// 测试：无直接单测
func hasAdminModelView(p *auth.Principal) bool {
	return p.IsMaster() || p.PlatformAdmin()
}

// queryBool treats the query values 1, true, yes, and on as true, ignoring case. Everything else, including an empty string, is false.
// 参数 v（string）：查询布尔使用的值。空串表示调用方没有提供这项。
// 返回 bool（bool）：查询值是 1、true、yes 或 on（忽略大小写）时返回真。空串和其他值都是假。
// 调用：仅在 list.go 内使用
// 测试：无直接单测
func queryBool(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

// proxyModelNames returns public model names in config order. A name whose deployments are all disabled is omitted.
// 参数 list（[]config.ModelEntry）：候选部署列表，后面按策略挑一条。
// 返回 []string（[]string）：proxy模型名称。没有匹配时为空切片。
// 调用：仅在 list.go 内使用
// 测试：list_test.go
func proxyModelNames(list []config.ModelEntry) []string {
	total := map[string]int{}
	disabled := map[string]int{}
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
		if m.Disabled() {
			disabled[m.ModelName]++
		}
	}
	var out []string
	for _, name := range order {
		if total[name] > 0 && disabled[name] == total[name] {
			continue
		}
		out = append(out, name)
	}
	return out
}

// 判断这条配置是不是不能当模型调用的条目。
// 参数 entry（config.ModelEntry）：一条模型部署，含对外名、供应商参数和价格覆盖。
// 返回 bool（bool）：这条配置不是可调用模型时为真，例如空名或供应商壳。
// 调用：gateway/models/available.go
// 测试：list_test.go
func nonModelEntry(entry config.ModelEntry) bool {
	return strings.TrimSpace(entry.ModelName) == "" ||
		providerShell(entry.ModelInfo)
}

// providerShell reports a builtin provider row. It is a credential shell, not a model the console should list.
// 参数 info（map[string]any）：一行价格或模型字段。缺键表示价目表没有这项。
// 返回 bool（bool）：这是内置供应商的凭据壳，不是控制台应列出的模型时返回真。
// 调用：gateway/models/admin.go
// 测试：无直接单测
func providerShell(info map[string]any) bool {
	if info == nil {
		return false
	}
	role, _ := info["role"].(string)
	return role == "provider"
}

// stripModelOwnership removes obsolete deployment ownership fields. Team model allowlists live in the identity module.
// 参数 info（map[string]any）：要清理的 model_info；nil 可安全处理。
// 返回：无。直接删除模型上的用户、团队、组织和访问组字段。
// 调用：模型创建、更新、读取和数据库加载路径。
// 测试：admin_test.go、list_test.go。
func stripModelOwnership(info map[string]any) {
	for _, key := range []string{"team_id", "teamId", "organization_id", "organizationId", "org_id", "orgId", "user_id", "userId", "access_groups", "model_access_group"} {
		delete(info, key)
	}
}

// expandWildcardNames expands model names that contain * into concrete models from the price map. When returnWild is true the wildcard text is kept. With no matching deployment it looks up the built-intable by provider prefix.
// 参数 models（[]string）：模型列表。空切片表示没有可处理的项；list（[]config.ModelEntry）：候选部署列表，后面按策略挑一条；returnWild（bool）：为真时走returnWild这一支。为假时保持原来的路径。
// 返回 []string（[]string）：展开通配名称。没有匹配时为空切片。
// 调用：仅在 list.go 内使用
// 测试：无直接单测
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

// deploymentsNamed 收集公开名等于给定名字的部署，不展开通配符。
// 参数 list（[]config.ModelEntry）：当前模型表。name（string）：公开模型名，不在这里展开通配符。
// 返回：公开名等于 name 的部署。
// 调用：模型列表按公开名收集部署。
// 测试：无直接单测
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
// 参数 wildcard（string）：wildcard；litellmModel（string）：litellm model。
// 返回 []string（[]string）：价格表里这个前缀展开出的模型 id。不是已知供应商的组织 id 前缀得到空切片。
// 调用：仅在 list.go 内使用
// 测试：无直接单测
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
// 参数 in（[]string）：内列表。空切片表示没有可处理的项。
// 返回 []string（[]string）：去重模型。没有匹配时为空切片。
// 调用：仅在 list.go 内使用
// 测试：无直接单测
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

// Info serves GET /v2/model/info, the deployment list behind the console's Models and Endpoints page and its auto-router lookups. The page reads this route and nothing else, so without a handler it fell
//
//	through to the catalog's generic key-value store and answered an empty list — a page that looked like an empty deployment table rather than a missing endpoint. Each row is the deployment JSON the page renders, filtered to the models the caller may actually use so the table never offers a model the gateway would refuse. A platform administrator sees every deployment, matching the list route, where
//	a missing grant means unrestricted rather than denied.
//
// 参数 s（Host）：信息使用的数据面宿主；w（http.ResponseWriter）：调用方的 HTTP 响应，状态和正文写在这里；r（*http.Request）：入站 HTTP 请求。
// 返回：无。状态码和正文写进调用方的响应。
// 调用：dataplane/log.go、dataplane/serve.go、gateway/engine.go、gateway/keys/generate.go
// 测试：files_test.go
func Info(s Host, w http.ResponseWriter, r *http.Request) {
	httpx.SetCallID(w, httpx.CallID())
	p, err := s.Resolve(r)
	if err != nil {
		httpx.WriteTypedError(w, r.URL.Path, 401, "invalid_api_key", "Authentication Error, No api key passed in.")
		return
	}
	// The page is management UI: an inference key has no business enumerating
	// deployments, and its own list route answers a different shape.
	if !p.PlatformAdmin() {
		httpx.WriteError(w, 403, "forbidden", "this credential cannot list deployments")
		return
	}

	s.LockModels()
	list := append([]config.ModelEntry(nil), (*s.ModelTable())...)
	s.UnlockModels()

	modelID := strings.TrimSpace(r.URL.Query().Get("modelId"))
	search := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("search")))
	rows := make([]map[string]any, 0, len(list))
	for _, m := range list {
		if m.ModelName == "" || providerShell(m.ModelInfo) {
			continue
		}
		id := str(m.ModelInfo["id"])
		if id == "" {
			id = m.ModelName
		}
		if modelID != "" && id != modelID {
			continue
		}
		if search != "" && !strings.Contains(strings.ToLower(id+" "+m.ModelName+" "+m.ParamString("model", "")+" "+m.ParamString("litellm_credential_name", "")+" "+m.ParamString("custom_llm_provider", "")), search) {
			continue
		}
		row := Public(m)
		// The page switches on these two fields to decide which rows it may
		// edit, and they are not part of the stored record.
		row["id"] = str(m.ModelInfo["id"])
		row["db_model"] = modelIsDB(m)
		rows = append(rows, row)
	}

	// The page paginates, so the envelope carries the counts it reads. The rows
	// are already narrowed, so the totals describe what this caller can see.
	page, size := queryIntDefault(r, "page", 1), queryIntDefault(r, "size", 50)
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 50
	}
	total := len(rows)
	// 超大页码返回空页，避免整数溢出造成切片崩溃。
	start, end := pagination.Bounds(total, page, size)
	pages := 1
	if total > 0 {
		pages = (total + size - 1) / size
	}
	httpx.WriteJSON(w, 200, map[string]any{
		"data":         rows[start:end],
		"total_count":  total,
		"current_page": page,
		"total_pages":  pages,
		"size":         size,
	})
}

// queryIntDefault reads an integer query parameter, falling back to def when it is absent or unparsable.
// 参数 r（*http.Request）：入站 HTTP 请求；name（string）：查询参数名；def（int）：参数缺失或不是整数时用的默认值。
// 返回 int（int）：查询参数解析出的整数。缺失或无法解析时返回 def，不是固定的 0。
// 调用：仅在 list.go 内使用
// 测试：无直接单测
func queryIntDefault(r *http.Request, name string, def int) int {
	raw := strings.TrimSpace(r.URL.Query().Get(name))
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return def
	}
	return n
}
