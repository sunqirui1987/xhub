// 模型的创建、更新、删除和屏蔽。数据库里的模型会覆盖同名 YAML。
package models

import (
	"net/http"
	"strings"
	"time"

	"github.com/sunqirui1987/xhub/internal/catalog"
	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/httpx"
	"github.com/sunqirui1987/xhub/internal/store"
)

// 模型的对外 JSON。参数里的密钥会被遮罩。
func Public(m config.ModelEntry) map[string]any {
	info := m.ModelInfo
	if info == nil {
		info = map[string]any{}
	}
	if info["id"] == nil || str(info["id"]) == "" {
		info["id"] = m.ModelName
	}
	params := redactLiteLLMParams(m.LiteLLMParams)
	blocked := false
	if v, ok := info["blocked"].(bool); ok {
		blocked = v
	}
	// 配置文件里的模型没有 db_model。控制台据此禁用删除和保存。
	if _, ok := info["db_model"].(bool); !ok {
		info["db_model"] = false
	}
	return map[string]any{
		"model_name":     m.ModelName,
		"litellm_params": params,
		"model_info":     info,
		"blocked":        blocked,
	}
}

// 遮罩 litellm 参数中的密钥字段。
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

// 创建数据库模型。同名 YAML 模型之后以数据库行为准。
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
	if str(info["id"]) == "" {
		info["id"] = "model_" + httpx.CallID()[:12]
	}
	// 页面创建的模型进数据库。重启后 LoadStored 会把它加回来，并标成 db_model。
	info["db_model"] = true
	if str(info["created_at"]) == "" {
		info["created_at"] = time.Now().UTC().Format(time.RFC3339)
	}
	entry := config.ModelEntry{ModelName: name, LiteLLMParams: params, ModelInfo: info}
	if err := s.DB().UpsertProxyModel(proxyModel(entry)); err != nil {
		httpx.WriteError(w, 500, "internal", err.Error())
		return
	}
	s.LockModels()
	*s.ModelTable() = append(*s.ModelTable(), entry)
	s.UnlockModels()
	httpx.WriteJSON(w, 200, Public(entry))
}

// 更新数据库模型。只改请求里出现的字段。
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
	m.ModelInfo["db_model"] = true
	if err := s.DB().UpsertProxyModel(proxyModel(m)); err != nil {
		httpx.WriteError(w, 500, "internal", err.Error())
		return
	}
	(*s.ModelTable())[i] = m
	httpx.WriteJSON(w, 200, Public(m))
}

// 删除数据库模型。删不掉只存在于 YAML 的模型。
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
	if err := s.DB().DeleteProxyModel(str(m.ModelInfo["id"])); err != nil {
		httpx.WriteError(w, 500, "internal", err.Error())
		return
	}
	*s.ModelTable() = append((*s.ModelTable())[:i], (*s.ModelTable())[i+1:]...)
	out := Public(m)
	out["id"] = id
	out["deleted"] = true
	httpx.WriteJSON(w, 200, out)
}

// 屏蔽模型。
func Block(s Host, w http.ResponseWriter, r *http.Request) {
	setBlocked(s, w, r, true)
}

// 取消屏蔽模型。
func Unblock(s Host, w http.ResponseWriter, r *http.Request) {
	setBlocked(s, w, r, false)
}

// 设置模型屏蔽标志。
func setBlocked(s Host, w http.ResponseWriter, r *http.Request, blocked bool) {
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
	if m.ModelInfo == nil {
		m.ModelInfo = map[string]any{}
	}
	if !modelIsDB(m) {
		httpx.WriteError(w, 400, "invalid_request", "Config models cannot be paused from the dashboard.")
		return
	}
	m.ModelInfo["blocked"] = blocked
	(*s.ModelTable())[i] = m
	if err := s.DB().UpsertProxyModel(proxyModel(m)); err != nil {
		httpx.WriteError(w, 500, "internal", err.Error())
		return
	}
	httpx.WriteJSON(w, 200, Public(m))
}

// 返回价格表来源、是否强制内置表、加载时间和模型数量。
func CostMapSource(s Host, w http.ResponseWriter, r *http.Request) {
	if s.RequireManage(w, r) == nil {
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{
		"source":          "local",
		"url":             nil,
		"is_env_forced":   catalog.EnvForced(),
		"fallback_reason": nil,
		"loaded_at":       catalog.LoadedAt(),
		"source_revision": nil,
		"etag":            nil,
		"model_count":     catalog.Count(),
	})
}

// 返回一个对外模型名下的部署信息。
func GroupInfo(s Host, w http.ResponseWriter, r *http.Request) {
	if s.RequireManage(w, r) == nil {
		return
	}
	groups := map[string][]string{}
	s.LockModels()
	list := append([]config.ModelEntry(nil), (*s.ModelTable())...)
	s.UnlockModels()
	for _, m := range list {
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
	data := []map[string]any{}
	for name, providers := range groups {
		mode := "chat"
		for _, m := range list {
			if m.ModelName == name && m.ModelInfo != nil {
				if v := str(m.ModelInfo["mode"]); v != "" {
					mode = v
				}
				break
			}
		}
		data = append(data, map[string]any{
			"model_group": name,
			"providers":   providers,
			"mode":        mode,
		})
	}
	httpx.WriteJSON(w, 200, map[string]any{"data": data})
}

// 模型是否来自数据库而不是 YAML。
func modelIsDB(m config.ModelEntry) bool {
	if m.ModelInfo == nil {
		return false
	}
	v, ok := m.ModelInfo["db_model"].(bool)
	return ok && v
}

// 把配置里的模型收成可以入库的一行。
func proxyModel(m config.ModelEntry) store.ProxyModel {
	id := ""
	if m.ModelInfo != nil {
		id = str(m.ModelInfo["id"])
	}
	return store.ProxyModel{ID: id, ModelName: m.ModelName, Params: m.LiteLLMParams, Info: m.ModelInfo}
}

// loadStoredState 把数据库里的模型并进本次进程的模型表。
// 配置文件里的模型不在这张表里，所以重启后仍然只来自 yaml，页面上删不掉。
func LoadStored(s Host) {
	if s.DB() == nil {
		return
	}
	rows, err := s.DB().ListProxyModels()
	if err != nil {
		return
	}
	for _, row := range rows {
		if row.Info == nil {
			row.Info = map[string]any{}
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
}

// findByID 按 id 找模型。请求路径上调用方必须已持有模型锁；启动合并时还没有并发请求。
func findByID(list []config.ModelEntry, id string) (int, config.ModelEntry, bool) {
	for i, m := range list {
		if m.ModelInfo != nil && str(m.ModelInfo["id"]) == id {
			return i, m, true
		}
	}
	return -1, config.ModelEntry{}, false
}

// findModel 按对外名字或 id 找模型。请求路径上调用方必须已持有模型锁。
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
