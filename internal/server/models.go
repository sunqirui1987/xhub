package server

import (
	"net/http"
	"strings"
	"time"

	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/httpx"
	"github.com/sunqirui1987/xhub/internal/store"
)

func (s *Server) modelPublic(m config.ModelEntry) map[string]any {
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

func (s *Server) modelNew(w http.ResponseWriter, r *http.Request) {
	if s.requireManage(w, r) == nil {
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
	// 页面创建的模型进数据库。重启后 loadStoredState 会把它加回来，并标成 db_model。
	info["db_model"] = true
	if str(info["created_at"]) == "" {
		info["created_at"] = time.Now().UTC().Format(time.RFC3339)
	}
	entry := config.ModelEntry{ModelName: name, LiteLLMParams: params, ModelInfo: info}
	if err := s.Store.UpsertProxyModel(proxyModel(entry)); err != nil {
		httpx.WriteError(w, 500, "internal", err.Error())
		return
	}
	s.mu.Lock()
	s.Cfg.ModelList = append(s.Cfg.ModelList, entry)
	s.mu.Unlock()
	httpx.WriteJSON(w, 200, s.modelPublic(entry))
}

func (s *Server) modelUpdate(w http.ResponseWriter, r *http.Request) {
	if s.requireManage(w, r) == nil {
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
	s.mu.Lock()
	defer s.mu.Unlock()
	i, m, ok := s.findModelLocked(id)
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
	if err := s.Store.UpsertProxyModel(proxyModel(m)); err != nil {
		httpx.WriteError(w, 500, "internal", err.Error())
		return
	}
	s.Cfg.ModelList[i] = m
	httpx.WriteJSON(w, 200, s.modelPublic(m))
}

func (s *Server) modelDelete(w http.ResponseWriter, r *http.Request) {
	if s.requireManage(w, r) == nil {
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
	s.mu.Lock()
	defer s.mu.Unlock()
	i, m, ok := s.findModelLocked(id)
	if !ok {
		httpx.WriteError(w, 400, "invalid_request", "model not found")
		return
	}
	if !modelIsDB(m) {
		httpx.WriteError(w, 400, "invalid_request", "Config model cannot be deleted on the dashboard. Delete it from the config file.")
		return
	}
	if err := s.Store.DeleteProxyModel(str(m.ModelInfo["id"])); err != nil {
		httpx.WriteError(w, 500, "internal", err.Error())
		return
	}
	s.Cfg.ModelList = append(s.Cfg.ModelList[:i], s.Cfg.ModelList[i+1:]...)
	out := s.modelPublic(m)
	out["id"] = id
	out["deleted"] = true
	httpx.WriteJSON(w, 200, out)
}

func (s *Server) modelBlock(w http.ResponseWriter, r *http.Request) {
	s.setModelBlocked(w, r, true)
}

func (s *Server) modelUnblock(w http.ResponseWriter, r *http.Request) {
	s.setModelBlocked(w, r, false)
}

func (s *Server) setModelBlocked(w http.ResponseWriter, r *http.Request, blocked bool) {
	if s.requireManage(w, r) == nil {
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
	s.mu.Lock()
	defer s.mu.Unlock()
	i, m, ok := s.findModelLocked(id)
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
	s.Cfg.ModelList[i] = m
	if err := s.Store.UpsertProxyModel(proxyModel(m)); err != nil {
		httpx.WriteError(w, 500, "internal", err.Error())
		return
	}
	httpx.WriteJSON(w, 200, s.modelPublic(m))
}

func (s *Server) modelCostMapSource(w http.ResponseWriter, r *http.Request) {
	if s.requireManage(w, r) == nil {
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{
		"source":          "local",
		"url":             nil,
		"is_env_forced":   localCostMapForced(),
		"fallback_reason": nil,
		"loaded_at":       modelCostMapLoadedAt,
		"source_revision": nil,
		"etag":            nil,
		"model_count":     modelCostMapCount(),
	})
}

func (s *Server) modelGroupInfo(w http.ResponseWriter, r *http.Request) {
	if s.requireManage(w, r) == nil {
		return
	}
	groups := map[string][]string{}
	s.mu.Lock()
	list := append([]config.ModelEntry(nil), s.Cfg.ModelList...)
	s.mu.Unlock()
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

func modelIsDB(m config.ModelEntry) bool {
	if m.ModelInfo == nil {
		return false
	}
	v, ok := m.ModelInfo["db_model"].(bool)
	return ok && v
}

func proxyModel(m config.ModelEntry) store.ProxyModel {
	id := ""
	if m.ModelInfo != nil {
		id = str(m.ModelInfo["id"])
	}
	return store.ProxyModel{ID: id, ModelName: m.ModelName, Params: m.LiteLLMParams, Info: m.ModelInfo}
}

// loadStoredState 把数据库里的模型并进本次进程的模型表。
// 配置文件里的模型不在这张表里，所以重启后仍然只来自 yaml，页面上删不掉。
func (s *Server) loadStoredState() {
	if s.Store == nil {
		return
	}
	rows, err := s.Store.ListProxyModels()
	if err != nil {
		return
	}
	for _, row := range rows {
		if row.Info == nil {
			row.Info = map[string]any{}
		}
		row.Info["id"] = row.ID
		row.Info["db_model"] = true
		if _, _, ok := s.findModelByIDLocked(row.ID); ok {
			continue
		}
		s.Cfg.ModelList = append(s.Cfg.ModelList, config.ModelEntry{
			ModelName: row.ModelName, LiteLLMParams: row.Params, ModelInfo: row.Info,
		})
	}
}

func (s *Server) findModelByIDLocked(id string) (int, config.ModelEntry, bool) {
	for i, m := range s.Cfg.ModelList {
		if m.ModelInfo != nil && str(m.ModelInfo["id"]) == id {
			return i, m, true
		}
	}
	return -1, config.ModelEntry{}, false
}

func (s *Server) findModelLocked(id string) (int, config.ModelEntry, bool) {
	for i, m := range s.Cfg.ModelList {
		if m.ModelName == id {
			return i, m, true
		}
		if m.ModelInfo != nil && str(m.ModelInfo["id"]) == id {
			return i, m, true
		}
	}
	return -1, config.ModelEntry{}, false
}
