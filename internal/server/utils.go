package server

import (
	"net/http"

	"github.com/sunqirui1987/xhub/internal/httpx"
	"github.com/sunqirui1987/xhub/internal/llm"
)

// tokenCounter 是 POST /utils/token_counter。
// 缺 prompt、messages、contents 时返回 400。计数与 LiteLLM 本地 tokenizer 一致，不调用供应商。
func (s *Server) tokenCounter(w http.ResponseWriter, r *http.Request) {
	httpx.SetCallID(w, httpx.CallID())
	if s.requireMixed(w, r) == nil {
		return
	}
	body := readMap(r)
	model := str(body["model"])
	prompt, _ := body["prompt"].(string)
	messages, _ := body["messages"].([]any)
	_, hasContents := body["contents"]
	if prompt == "" && messages == nil && !hasContents {
		httpx.WriteError(w, 400, "invalid_request", "prompt or messages or contents must be provided")
		return
	}
	var msgs []map[string]any
	for _, item := range messages {
		if m, ok := item.(map[string]any); ok {
			msgs = append(msgs, m)
		}
	}
	if messages != nil && msgs == nil {
		msgs = []map[string]any{}
	}
	used := s.modelUsedForCount(model)
	total, kind, err := llm.CountTokens(used, prompt, msgs)
	if err != nil {
		httpx.WriteError(w, 500, "internal", err.Error())
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{
		"total_tokens":   total,
		"request_model":  model,
		"model_used":     used,
		"tokenizer_type": kind,
	})
}

func (s *Server) modelUsedForCount(requestModel string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, m := range s.Cfg.ModelList {
		if m.ModelName != requestModel {
			continue
		}
		return llm.ModelUsedForCount(requestModel, str(m.LiteLLMParams["model"]))
	}
	return requestModel
}

// supportedOpenAIParams 是 GET /utils/supported_openai_params?model=。
func (s *Server) supportedOpenAIParams(w http.ResponseWriter, r *http.Request) {
	httpx.SetCallID(w, httpx.CallID())
	if s.requireMixed(w, r) == nil {
		return
	}
	model := r.URL.Query().Get("model")
	if model == "" {
		httpx.WriteError(w, 400, "invalid_request", "model required")
		return
	}
	used := s.modelUsedForCount(model)
	_, catalog := modelCostMap()[used]
	httpx.WriteJSON(w, 200, map[string]any{
		"supported_openai_params": llm.OpenAISupportedParams(used, catalog),
	})
}

func modelCostMap() map[string]map[string]any {
	if modelCostMapValue == nil {
		return map[string]map[string]any{}
	}
	raw, ok := modelCostMapValue.(map[string]any)
	if !ok {
		return map[string]map[string]any{}
	}
	out := make(map[string]map[string]any, len(raw))
	for k, v := range raw {
		if row, ok := v.(map[string]any); ok {
			out[k] = row
		}
	}
	return out
}
