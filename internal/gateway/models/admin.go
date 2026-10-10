// Package models creates, updates, deletes, and disables models. A model stored in the database overrides YAML with the same name.
package models

import (
	"maps"
	"net/http"
	"strings"
	"time"

	"github.com/sunqirui1987/xhub/internal/catalog"
	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/httpx"
	"github.com/sunqirui1987/xhub/internal/logx"
	"github.com/sunqirui1987/xhub/internal/provider"
	"github.com/sunqirui1987/xhub/internal/store"
	"sync"
)

var logTraceOnceAdmin sync.Once

// Public is the model JSON shown to callers. Secrets inside the parameters are masked.
// 参数 m（config.ModelEntry）：一条模型部署，含对外名、供应商参数和价格覆盖。
// 返回 map[string]any（map[string]any）：公开的字段表。缺键表示上游或库里没有这个字段。
// 调用：gateway/models/list.go、gateway/wire.go
// 测试：无直接单测
func Public(m config.ModelEntry) map[string]any {
	logTraceOnceAdmin.Do(func() { logx.Trace("enter models.Public") })

	info := maps.Clone(m.ModelInfo)
	if info == nil {
		info = map[string]any{}
	}
	if info["id"] == nil || str(info["id"]) == "" {
		info["id"] = m.ModelName
	}
	stripModelOwnership(info)
	params := redactLiteLLMParams(m.LiteLLMParams)

	if err := provider.ValidateDeployment(m); err != nil {
		info["unavailable_reason"] = err.Error()
	}
	info["disabled"] = m.Disabled()
	// A model from the config file has no db_model. The dashboard uses that to disable delete and save.
	if _, ok := info["db_model"].(bool); !ok {
		info["db_model"] = false
	}
	return map[string]any{
		"model_name":     m.ModelName,
		"litellm_params": params,
		"model_info":     info,
	}
}

// redactLiteLLMParams masks secret fields inside litellm parameters.
// 参数 in（map[string]any）：打码LiteLLMParams读到的 JSON 对象。缺键表示没有该字段。
// 返回 map[string]any（map[string]any）：打码LiteLLMParams的字段表。缺键表示上游或库里没有这个字段。
// 调用：仅在 admin.go 内使用
// 测试：无直接单测
func redactLiteLLMParams(in map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range in {
		lk := strings.ToLower(k)
		if s, ok := v.(string); ok && s != "" && (strings.Contains(lk, "key") || strings.Contains(lk, "secret") || strings.Contains(lk, "password") || strings.Contains(lk, "token")) {
			out[k] = "*****"
			continue
		}
		out[k] = v
	}
	return out
}

// New 创建数据库部署，并在同一事务中清理因目录变化而失效的默认权重和模板引用。
// 参数 s 提供模型锁、内存目录和配置存储；w 接收 HTTP 状态与 JSON；r 包含部署名称、上游参数和模型信息。
// 返回：无。校验失败写 400，名称冲突写 409，事务失败写 500；只有事务提交后才发布新内存目录。
// 调用场景：POST /model/new 管理接口。副作用是写 proxy_models、可能修复权重配置，并更新进程模型目录。
// 边界：配置文件部署仍由 YAML 管理；任何持久化或清理错误都会回滚，且不会改变内存目录。
func New(s Host, w http.ResponseWriter, r *http.Request) {
	if s.RequireManage(w, r) == nil {
		return
	}
	body := readMap(r)
	name := str(body["model_name"])
	params, _ := body["litellm_params"].(map[string]any)
	if params == nil {
		params = map[string]any{}
	}
	info, _ := body["model_info"].(map[string]any)
	if info == nil {
		info = map[string]any{}
	}
	stripModelOwnership(info)
	if _, ok := info["disabled"].(bool); !ok {
		info["disabled"] = false
	}
	if str(info["id"]) == "" {
		info["id"] = "model_" + httpx.CallID()[:12]
	}
	if err := validateNewDeploymentSurface(params, info); err != nil {
		httpx.WriteError(w, 400, "invalid_request", err.Error())
		return
	}
	if err := validateDeployment(s, name, params, info); err != nil {
		httpx.WriteError(w, 400, "invalid_request", err.Error())
		return
	}
	// 校验与目录修改共用锁，避免模型创建与路由组创建同时占用同一名称。
	s.LockModels()
	defer s.UnlockModels()
	if err := checkRoutingGroupName(s, name); err != nil {
		httpx.WriteError(w, 409, "routing_group_conflict", err.Error())
		return
	}
	// A model created from the page is stored in the database. After a restart LoadStored adds it back and marks it db_model.
	info["db_model"] = true
	if str(info["created_at"]) == "" {
		info["created_at"] = time.Now().UTC().Format(time.RFC3339)
	}
	entry := config.ModelEntry{ModelName: name, LiteLLMParams: params, ModelInfo: info}
	// 同一事务保存部署和有效分配，提交后才发布目录，避免失败时内存与数据库分叉。
	directory := append(append([]config.ModelEntry(nil), (*s.ModelTable())...), entry)
	record := proxyModel(entry)
	if err := s.RecordStore().SaveModelDirectory(&record, "", directory); err != nil {
		httpx.WriteError(w, 500, "internal", err.Error())
		return
	}
	*s.ModelTable() = directory
	httpx.WriteJSON(w, 200, Public(entry))
}

// Update 修改数据库部署，并以修改后的完整目录原子清理改名或部署集合变化产生的失效权重。
// 参数 s 提供模型锁、内存目录和配置存储；w 接收 HTTP 状态与 JSON；r 包含部署 ID 及待覆盖字段。
// 返回：无。不存在、配置部署或非法字段写 400，名称或回退冲突写 409，事务失败写 500。
// 调用场景：POST /model/update/{model_id} 管理接口。成功时持久化部署并替换对应内存条目。
// 边界：遮罩后的密钥不覆盖原值；校验及事务失败时数据库回滚，原内存对象保持不变。
func Update(s Host, w http.ResponseWriter, r *http.Request) {
	if s.RequireManage(w, r) == nil {
		return
	}
	body := readMap(r)
	id := r.PathValue("model_id")
	if id == "" {
		if info, ok := body["model_info"].(map[string]any); ok {
			id = str(info["id"])
		}
	}
	if id == "" {
		id = str(body["id"])
	}
	if id == "" {
		httpx.WriteError(w, 400, "invalid_request", "model_info.id required")
		return
	}
	s.LockModels()
	defer s.UnlockModels()
	i, m, ok := findModel(*s.ModelTable(), id)
	if !ok {
		httpx.WriteError(w, 400, "invalid_request", "model not found")
		return
	}
	if !modelIsDB(m) {
		httpx.WriteError(w, 400, "invalid_request", "Config model cannot be updated. Edit the config file.")
		return
	}
	if params, ok := body["litellm_params"].(map[string]any); ok {
		if _, exists := params["weight"]; exists {
			httpx.WriteError(w, 400, "invalid_request", "deployment weight is obsolete; configure default weights in model management or customer overrides in route templates")
			return
		}
	}
	m.LiteLLMParams = maps.Clone(m.LiteLLMParams)

	m.ModelInfo = maps.Clone(m.ModelInfo)
	if v := str(body["model_name"]); v != "" {
		m.ModelName = v
	}
	if params, ok := body["litellm_params"].(map[string]any); ok {
		if m.LiteLLMParams == nil {
			m.LiteLLMParams = map[string]any{}
		}
		for k, v := range params {
			if secret, ok := v.(string); ok && secret == "*****" {
				continue
			}
			m.LiteLLMParams[k] = v
		}
	}
	if info, ok := body["model_info"].(map[string]any); ok {
		if m.ModelInfo == nil {
			m.ModelInfo = map[string]any{}
		}
		for k, v := range info {
			m.ModelInfo[k] = v
		}
	}
	if disabled, ok := body["disabled"].(bool); ok {
		if m.ModelInfo == nil {
			m.ModelInfo = map[string]any{}
		}
		m.ModelInfo["disabled"] = disabled
	}
	stripModelOwnership(m.ModelInfo)
	if _, ok := m.ModelInfo["disabled"].(bool); !ok {
		m.ModelInfo["disabled"] = m.Disabled()
	}
	m.ModelInfo["db_model"] = true
	// 改名最后一个部署前检查公开名引用，不能让回退策略指向已消失的模型。
	if old := (*s.ModelTable())[i]; old.ModelName != m.ModelName {
		if err := checkRoutingGroupName(s, m.ModelName); err != nil {
			httpx.WriteError(w, 409, "routing_group_conflict", err.Error())
			return
		}
		if err := checkFallbackRemoval(s, old.ModelName, id); err != nil {
			httpx.WriteError(w, 409, "fallback_conflict", err.Error())
			return
		}
	}
	if err := validateDeployment(s, m.ModelName, m.LiteLLMParams, m.ModelInfo); err != nil {
		httpx.WriteError(w, 400, "invalid_request", err.Error())
		return
	}
	// 改名会改变两个公开模型的部署集合，清理默认与模板分配后再更新内存。
	directory := append([]config.ModelEntry(nil), (*s.ModelTable())...)
	directory[i] = m
	record := proxyModel(m)
	if err := s.RecordStore().SaveModelDirectory(&record, "", directory); err != nil {
		httpx.WriteError(w, 500, "internal", err.Error())
		return
	}
	*s.ModelTable() = directory
	httpx.WriteJSON(w, 200, Public(m))
}

// Delete 删除数据库部署，并在同一事务中移除默认权重、历史路由组和客户模板里的悬空部署引用。
// 参数 s 提供模型锁、内存目录和配置存储；w 接收 HTTP 状态与 JSON；r 通过 id 或 model_name 指定部署。
// 返回：无。缺少或找不到 ID、删除配置部署时写 400，仍被回退策略引用时写 409，事务失败写 500。
// 调用场景：POST /model/delete 管理接口。成功提交后才从进程目录删除部署并返回 deleted=true。
// 边界：持久化清理失败会整笔回滚，内存目录不变；配置文件部署必须通过配置文件删除。
func Delete(s Host, w http.ResponseWriter, r *http.Request) {
	if s.RequireManage(w, r) == nil {
		return
	}
	body := readMap(r)
	id := str(body["id"])
	if id == "" {
		id = str(body["model_name"])
	}
	if id == "" {
		httpx.WriteError(w, 400, "invalid_request", "id required")
		return
	}
	s.LockModels()
	defer s.UnlockModels()
	i, m, ok := findModel(*s.ModelTable(), id)
	if !ok {
		httpx.WriteError(w, 400, "invalid_request", "model not found")
		return
	}
	if !modelIsDB(m) {
		httpx.WriteError(w, 400, "invalid_request", "Config model cannot be deleted on the dashboard. Delete it from the config file.")
		return
	}
	// 拒绝移除仍被策略引用的最后一个部署；管理员清除策略后可安全删除。
	if err := checkFallbackRemoval(s, m.ModelName, str(m.ModelInfo["id"])); err != nil {
		httpx.WriteError(w, 409, "fallback_conflict", err.Error())
		return
	}
	// 复制目录，事务同时移除部署及悬空权重；回滚时保留原目录和原分配。
	directory := append([]config.ModelEntry(nil), (*s.ModelTable())[:i]...)
	directory = append(directory, (*s.ModelTable())[i+1:]...)
	if err := s.RecordStore().SaveModelDirectory(nil, str(m.ModelInfo["id"]), directory); err != nil {
		httpx.WriteError(w, 500, "internal", err.Error())
		return
	}
	*s.ModelTable() = directory
	out := Public(m)
	out["id"] = id
	out["deleted"] = true
	httpx.WriteJSON(w, 200, out)
}

// Disable prevents a stored deployment from being selected at runtime.
// 参数 s（Host）：模型管理宿主；w（http.ResponseWriter）：HTTP 响应；r（*http.Request）：包含部署 id 或名称的请求。
// 返回：无。状态码和模型 JSON 写入响应。
// 调用：由 POST /model/disable 路由调用。
// 测试：admin_test.go、regression/models_test.go。
func Disable(s Host, w http.ResponseWriter, r *http.Request) {
	setDisabled(s, w, r, true)
}

// Enable makes a stored deployment eligible for selection.
// 参数 s（Host）：模型管理宿主；w（http.ResponseWriter）：HTTP 响应；r（*http.Request）：包含部署 id 或名称的请求。
// 返回：无。状态码和模型 JSON 写入响应。
// 调用：由 POST /model/enable 路由调用。
// 测试：admin_test.go、regression/models_test.go。
func Enable(s Host, w http.ResponseWriter, r *http.Request) {
	setDisabled(s, w, r, false)
}

// setDisabled sets the persisted deployment disabled flag.
// 参数 s（Host）：模型管理宿主；w（http.ResponseWriter）：调用方的 HTTP 响应；r（*http.Request）：入站 HTTP 请求；disabled（bool）：要持久化的禁用状态。
// 返回：无。状态码和正文写进调用方的响应。
// 调用：仅在 admin.go 内使用。
// 测试：无直接单测
func setDisabled(s Host, w http.ResponseWriter, r *http.Request, disabled bool) {
	if s.RequireManage(w, r) == nil {
		return
	}
	body := readMap(r)
	id := str(body["id"])
	if id == "" {
		id = str(body["model"])
	}
	if id == "" {
		id = str(body["model_name"])
	}
	if id == "" {
		httpx.WriteError(w, 400, "invalid_request", "model required")
		return
	}
	s.LockModels()
	defer s.UnlockModels()
	i, m, ok := findModel(*s.ModelTable(), id)
	if !ok {
		httpx.WriteError(w, 400, "invalid_request", "model not found")
		return
	}
	if !modelIsDB(m) {
		httpx.WriteError(w, 400, "invalid_request", "Config models cannot be disabled from the dashboard.")
		return
	}
	m.ModelInfo = maps.Clone(m.ModelInfo)
	if m.ModelInfo == nil {
		m.ModelInfo = map[string]any{}
	}
	stripModelOwnership(m.ModelInfo)
	m.ModelInfo["disabled"] = disabled
	if err := s.RecordStore().UpsertProxyModel(proxyModel(m)); err != nil {
		httpx.WriteError(w, 500, "internal", err.Error())
		return
	}
	(*s.ModelTable())[i] = m
	httpx.WriteJSON(w, 200, Public(m))
}

// CostMapSource returns the price-map source, whether the built-in map is forced, the load time, and the model count.
// 参数 s（Host）：费用表来源使用的数据面宿主；w（http.ResponseWriter）：调用方的 HTTP 响应，状态和正文写在这里；r（*http.Request）：入站 HTTP 请求。
// 返回：无。状态码和正文写进调用方的响应。
// 调用：gateway/models/mount.go
// 测试：无直接单测
func CostMapSource(s Host, w http.ResponseWriter, r *http.Request) {
	if s.RequireManage(w, r) == nil {
		return
	}
	// The catalog is generated from the market feed and embedded, then refreshed
	// in place by a reload. source is "market" either way, and generated_at says
	// how old the embedded copy is.
	httpx.WriteJSON(w, 200, map[string]any{
		"source":          "market",
		"url":             catalog.PriceSource(),
		"is_env_forced":   catalog.EnvForced(),
		"fallback_reason": nil,
		"loaded_at":       catalog.LoadedAt(),
		"generated_at":    catalog.PriceGeneratedAt(),
		"source_revision": nil,
		"etag":            nil,
		"model_count":     catalog.Count(),
	})
}

// GroupInfo returns the deployments under one public model name.
// 参数 s（Host）：分组信息使用的数据面宿主；w（http.ResponseWriter）：调用方的 HTTP 响应，状态和正文写在这里；r（*http.Request）：入站 HTTP 请求。
// 返回：无。状态码和正文写进调用方的响应。
// 调用：gateway/models/mount.go
// 测试：无直接单测
func GroupInfo(s Host, w http.ResponseWriter, r *http.Request) {
	if s.RequireManage(w, r) == nil {
		return
	}
	s.LockModels()
	list := append([]config.ModelEntry(nil), (*s.ModelTable())...)
	s.UnlockModels()
	httpx.WriteJSON(w, 200, map[string]any{"data": playgroundGroups(list)})
}

// playgroundGroups 是 Playground 的模型列表；能力从部署的 endpoint_types 读取。
// 参数 list（[]config.ModelEntry）：候选部署列表，后面按策略挑一条。
// 返回 []map[string]any（[]map[string]any）：一组map[string]any。没有匹配时为空切片，不是 nil 分页。
// 调用：仅在 admin.go 内使用
// 测试：builtin_test.go
func playgroundGroups(list []config.ModelEntry) []map[string]any {
	groups := map[string][]string{}
	order := []string{}
	for _, m := range list {
		if providerShell(m.ModelInfo) || m.Disabled() || m.ModelName == "" {
			continue
		}
		if _, ok := groups[m.ModelName]; !ok {
			order = append(order, m.ModelName)
		}
		prov := "openai"
		if p := str(m.LiteLLMParams["custom_llm_provider"]); p != "" {
			prov = p
		} else if raw := str(m.LiteLLMParams["model"]); raw != "" {
			if i := strings.Index(raw, "/"); i > 0 {
				prov = raw[:i]
			}
		}
		groups[m.ModelName] = append(groups[m.ModelName], prov)
	}
	data := make([]map[string]any, 0, len(order))
	for _, name := range order {
		mode := "other"
		for _, m := range list {
			if m.ModelName != name || providerShell(m.ModelInfo) || m.Disabled() {
				continue
			}
			if caps := provider.SelectedCapabilities(m); len(caps) > 0 {
				mode = caps[0]
			}
			break
		}
		data = append(data, map[string]any{
			"model_group": name,
			"providers":   groups[name],
			"mode":        mode,
			"endpoints":   modelEndpointsForAlias(list, name),
		})
	}
	return data
}

// modelIsDB reports whether the model came from the database rather than YAML.
// 参数 m（config.ModelEntry）：一条模型部署，含对外名、供应商参数和价格覆盖。
// 返回 bool（bool）：这条模型来自数据库而不是 YAML 时返回真。
// 调用：gateway/models/list.go
// 测试：无直接单测
func modelIsDB(m config.ModelEntry) bool {
	if m.ModelInfo == nil {
		return false
	}
	v, ok := m.ModelInfo["db_model"].(bool)
	return ok && v
}

// proxyModel turns a configured model into a row that can be stored.
// 参数 m（config.ModelEntry）：一条模型部署，含对外名、供应商参数和价格覆盖。
// 返回 ProxyModel（store.ProxyModel）：可以写入 proxy_models 的一行，含 id、对外名、供应商参数和模型信息。
// 调用：gateway/models/builtin.go
// 测试：无直接单测
func proxyModel(m config.ModelEntry) store.ProxyModel {
	id := ""
	if m.ModelInfo != nil {
		id = str(m.ModelInfo["id"])
	}
	return store.ProxyModel{ID: id, ModelName: m.ModelName, Params: m.LiteLLMParams, Info: m.ModelInfo}
}

// LoadStored 在启动阶段把数据库部署合并进进程目录，并按最终完整目录清理历史权重引用。
// 参数 s 提供尚未对外服务的模型目录和配置存储；调用方必须保证此时没有并发请求修改目录。
// 返回：无。成功时数据库部署带上稳定 ID 和 db_model 标记；读取失败时保留配置目录并直接返回。
// 调用场景：gateway.Server 启动。副作用包括补齐旧部署字段、修复默认权重和模板中的悬空部署 ID。
// 边界：单行字段迁移失败会记录日志并继续装载；历史权重清理失败只记录错误，事务会回滚且不阻止服务启动。
func LoadStored(s Host) {
	if s.RecordStore() == nil {
		return
	}
	rows, err := s.RecordStore().ListProxyModels()
	if err != nil {
		return
	}
	for _, row := range rows {
		if row.Info == nil {
			row.Info = map[string]any{}
		}
		changed := false
		if _, ok := row.Info["disabled"].(bool); !ok {
			row.Info["disabled"] = false
			changed = true
		}
		before := len(row.Info)
		stripModelOwnership(row.Info)
		changed = changed || len(row.Info) != before
		if changed {
			if err := s.RecordStore().UpsertProxyModel(row); err != nil {
				logx.Error("stored model %s: %v", row.ID, err)
			}
		}
		row.Info["id"] = row.ID
		row.Info["db_model"] = true
		if _, _, ok := findByID(*s.ModelTable(), row.ID); ok {
			continue
		}
		*s.ModelTable() = append(*s.ModelTable(), config.ModelEntry{
			ModelName: row.ModelName, LiteLLMParams: row.Params, ModelInfo: row.Info,
		})
	}
	// 启动时目录已经包含 YAML 与数据库部署，可据此一次性删除停机期间遗留的部署引用。
	if err := s.RecordStore().SaveModelDirectory(nil, "", append([]config.ModelEntry(nil), (*s.ModelTable())...)); err != nil {
		logx.Error("clean stored model weights: %v", err)
	}
}

// findByID 在当前模型表里按部署 id 查找。找不到时返回假。
// 参数 list（[]config.ModelEntry）：当前模型表。id（string）：部署 id，形状是 api_base|model。
// 返回：下标、那一行部署，以及是否找到。调用方需要已持有模型锁。
// 调用：仅在 admin.go 内使用。
// 测试：无直接单测
func findByID(list []config.ModelEntry, id string) (int, config.ModelEntry, bool) {
	for i, m := range list {
		if m.ModelInfo != nil && str(m.ModelInfo["id"]) == id {
			return i, m, true
		}
	}
	return -1, config.ModelEntry{}, false
}

// findModel 先按部署 id 查找，再按公开模型名查找。
// 参数 list（[]config.ModelEntry）：当前模型表。id（string）：公开名或部署 id。
// 返回：下标、那一行部署，以及是否找到。请求路径上调用方需要已持有模型锁。
// 调用：仅在 admin.go 内使用。
// 测试：无直接单测
func findModel(list []config.ModelEntry, id string) (int, config.ModelEntry, bool) {
	for i, m := range list {
		if m.ModelName == id {
			return i, m, true
		}
		if m.ModelInfo != nil && str(m.ModelInfo["id"]) == id {
			return i, m, true
		}
	}
	return -1, config.ModelEntry{}, false
}

// modelEndpointsForAlias 合并同一公开模型别名下所有部署的端点绑定。
// 一个别名可以同时挂在适配对话、原生 Responses 或 Fal 视频 bypass 上，
// 所以不能只取第一条部署。provider.MergeEndpoints 会按公开协议和传输去重。
// 参数 list（[]config.ModelEntry）：当前模型部署表；alias（string）：公开模型名。
// 返回 []provider.EndpointBinding（[]provider.EndpointBinding）：该别名的端点绑定。
// 调用：Admin/Playground 模型列表。
// 测试：admin_test.go、bindings_test.go。
func modelEndpointsForAlias(list []config.ModelEntry, alias string) []provider.EndpointBinding {
	out := []provider.EndpointBinding{}
	for _, m := range list {
		if m.ModelName == alias && !providerShell(m.ModelInfo) {
			out = provider.MergeEndpoints(out, provider.DeploymentEndpoints(m))
		}
	}
	return out
}
