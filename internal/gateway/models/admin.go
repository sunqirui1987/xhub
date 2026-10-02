// Package models creates, updates, deletes, and blocks models. A model stored in the database overrides YAML with the same name.
package models

import (
	"net/http"
	"strings"
	"time"

	"github.com/sunqirui1987/xhub/internal/catalog"
	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/httpx"
	"github.com/sunqirui1987/xhub/internal/logx"
	"github.com/sunqirui1987/xhub/internal/store"
	"sync"
)

var logTraceOnceAdmin sync.Once

// Public is the model JSON shown to callers. Secrets inside the parameters are masked.
func Public(m config.ModelEntry) map[string]any {
	logTraceOnceAdmin.Do(func() { logx.Trace("enter models.Public") })

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
	// A model from the config file has no db_model. The dashboard uses that to disable delete and save.
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

// redactLiteLLMParams masks secret fields inside litellm parameters.
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

// New creates a database model. A YAML model with the same name is then overridden by the database row.
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
	// A model created from the page is stored in the database. After a restart LoadStored adds it back and marks it db_model.
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

// Update changes a database model. Only fields present on the request are changed.
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

// Delete removes a database model. A model that exists only in YAML cannot be deleted.
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

// Block marks a model blocked.
func Block(s Host, w http.ResponseWriter, r *http.Request) {
	setBlocked(s, w, r, true)
}

// Unblock clears a model block.
func Unblock(s Host, w http.ResponseWriter, r *http.Request) {
	setBlocked(s, w, r, false)
}

// setBlocked sets the model blocked flag.
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

// CostMapSource returns the price-map source, whether the built-in map is forced, the load time, and the model count.
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

// GroupInfo returns the deployments under one public model name.
func GroupInfo(s Host, w http.ResponseWriter, r *http.Request) {
	if s.RequireManage(w, r) == nil {
		return
	}
	s.LockModels()
	list := append([]config.ModelEntry(nil), (*s.ModelTable())...)
	s.UnlockModels()
	httpx.WriteJSON(w, 200, map[string]any{"data": playgroundGroups(list)})
}

// playgroundGroups is the model list behind the playground endpoint picker.
// Provider shells such as fennoai and qiniu are credentials, not models. A mode copied from the provider wire is reported as chat, matching a hand-added model that leaves mode empty.
func playgroundGroups(list []config.ModelEntry) []map[string]any {
	groups := map[string][]string{}
	order := []string{}
	for _, m := range list {
		if providerShell(m.ModelInfo) || m.ModelName == "" {
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
		mode := "chat"
		for _, m := range list {
			if m.ModelName != name || providerShell(m.ModelInfo) {
				continue
			}
			if m.ModelInfo != nil {
				if v := str(m.ModelInfo["mode"]); v != "" {
					mode = v
				}
			}
			break
		}
		data = append(data, map[string]any{
			"model_group": name,
			"providers":   groups[name],
			"mode":        mode,
		})
	}
	return data
}

// modelIsDB reports whether the model came from the database rather than YAML.
func modelIsDB(m config.ModelEntry) bool {
	if m.ModelInfo == nil {
		return false
	}
	v, ok := m.ModelInfo["db_model"].(bool)
	return ok && v
}

// proxyModel turns a configured model into a row that can be stored.
func proxyModel(m config.ModelEntry) store.ProxyModel {
	id := ""
	if m.ModelInfo != nil {
		id = str(m.ModelInfo["id"])
	}
	return store.ProxyModel{ID: id, ModelName: m.ModelName, Params: m.LiteLLMParams, Info: m.ModelInfo}
}

// LoadStored merges database models into this process's model table.
// Entries that exist only in the config file are not in this table, so after a restart they still come only from YAML and cannot be deleted from the page.
func LoadStored(s Host) {
	if s.DB() == nil {
		return
	}
	dropProviderShells(s)
	rows, err := s.DB().ListProxyModels()
	if err != nil {
		return
	}
	for _, row := range rows {
		if clearCopiedMode(&row) {
			if err := s.DB().UpsertProxyModel(row); err != nil {
				logx.Error("builtin model %s: %v", row.ID, err)
			}
		}
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
	SeedBuiltins(s)
}

// findByID finds a model by id. On a request path the caller must already hold the model lock. The startup merge has no concurrent requests.
func findByID(list []config.ModelEntry, id string) (int, config.ModelEntry, bool) {
	for i, m := range list {
		if m.ModelInfo != nil && str(m.ModelInfo["id"]) == id {
			return i, m, true
		}
	}
	return -1, config.ModelEntry{}, false
}

// findModel finds a model by public name or id. On a request path the caller must already hold the model lock.
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
