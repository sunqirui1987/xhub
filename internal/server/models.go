package server

import (
	"net/http"
	"strings"

	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/httpx"
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
	s.mu.Lock()
	s.Cfg.ModelList = append(s.Cfg.ModelList, config.ModelEntry{ModelName: name, LiteLLMParams: params, ModelInfo: info})
	s.mu.Unlock()
	httpx.WriteJSON(w, 200, s.modelPublic(config.ModelEntry{ModelName: name, LiteLLMParams: params, ModelInfo: info}))
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
	if v := str(body["model_name"]); v != "" {
		m.ModelName = v
	}
	if params, ok := body["litellm_params"].(map[string]any); ok {
		if m.LiteLLMParams == nil {
			m.LiteLLMParams = map[string]any{}
		}
		for k, v := range params {
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
	m.ModelInfo["blocked"] = blocked
	s.Cfg.ModelList[i] = m
	httpx.WriteJSON(w, 200, s.modelPublic(m))
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
