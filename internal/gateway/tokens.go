// Package gateway counts tokens locally and lists the OpenAI parameters a model supports. A failed count returns an error instead of pretending the count is zero.
package gateway

import (
	"encoding/json"
	"net/http"

	"github.com/sunqirui1987/xhub/internal/httpx"
	"github.com/sunqirui1987/xhub/internal/llm/estimate"
)

// tokenCounter serves POST /utils/token_counter.
// A body without prompt, messages, or contents returns 400. The count matches the local tokenizer and does not call a provider.
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
	total, kind, err := estimate.CountTokens(used, prompt, msgs)
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

// modelUsedForCount chooses the request model name or the deployment model name for the count.
func (s *Server) modelUsedForCount(requestModel string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, m := range s.Cfg.ModelList {
		if m.ModelName != requestModel {
			continue
		}
		return estimate.ModelUsedForCount(requestModel, str(m.LiteLLMParams["model"]))
	}
	return requestModel
}

// supportedOpenAIParams serves GET /utils/supported_openai_params with a model query.
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
	_, known := modelCostMap()[used]
	body := map[string]any{
		"supported_openai_params": estimate.OpenAISupportedParams(used, known),
	}
	if saved, err := s.Store.GetKV("transform", "last"); err == nil {
		body["last_transform"] = saved
	}
	httpx.WriteJSON(w, 200, body)
}

// transformRequest stores the rewritten request so the transform page can read it back from the parameter list.
func (s *Server) transformRequest(w http.ResponseWriter, r *http.Request) {
	httpx.SetCallID(w, httpx.CallID())
	if s.requireMixed(w, r) == nil {
		return
	}
	body := readMap(r)
	model := str(body["model"])
	if model == "" {
		httpx.WriteError(w, 400, "invalid_request", "model required")
		return
	}
	out := map[string]any{
		"model":       model,
		"transformed": true,
		"name":        str(body["name"]),
		"request":     body,
	}
	raw, _ := json.Marshal(out)
	if err := s.Store.PutKV("transform", "last", string(raw)); err != nil {
		httpx.WriteError(w, 500, "internal", err.Error())
		return
	}
	httpx.WriteJSON(w, 200, out)
}
