// tokens.go counts tokens locally and lists the OpenAI parameters a model
// supports. A failed count returns an error instead of reporting zero.

package gateway

import (
	"encoding/json"
	"net/http"

	"github.com/sunqirui1987/xhub/internal/httpx"
	"github.com/sunqirui1987/xhub/internal/llm/estimate"
	"github.com/sunqirui1987/xhub/internal/logx"
	"sync"
)

var logTraceOnceTokens sync.Once

// tokenCounter serves POST /utils/token_counter. A body without prompt, messages, or contents returns 400. The count matches the local tokenizer and does not call a provider.
// 参数 w（http.ResponseWriter）：调用方的 HTTP 响应，状态码和正文写在这里；r（*http.Request）：入站 HTTP 请求，用来读路径、头和正文。
// 调用：仅在 tokens.go 内使用
// 测试：无直接单测
// 返回：无。状态码和正文写进调用方的响应。
func (s *Server) tokenCounter(w http.ResponseWriter, r *http.Request) {
	logTraceOnceTokens.Do(func() { logx.Trace("enter gateway.tokenCounter") })

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
// 参数 requestModel（string）：模型名。对外名用来选部署，上游名写进转发正文。
// 返回 string（string）：用来计数的部署上游模型名。没有对上的部署时用请求里的模型名。
// 调用：仅在 tokens.go 内使用
// 测试：无直接单测
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
// 参数 w（http.ResponseWriter）：调用方的 HTTP 响应，状态码和正文写在这里；r（*http.Request）：入站 HTTP 请求，用来读路径、头和正文。
// 调用：仅在 tokens.go 内使用
// 测试：无直接单测
// 返回：无。状态码和正文写进调用方的响应。
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
// 参数 w（http.ResponseWriter）：调用方的 HTTP 响应，状态码和正文写在这里；r（*http.Request）：入站 HTTP 请求，用来读路径、头和正文。
// 调用：仅在 tokens.go 内使用
// 测试：无直接单测
// 返回：无。状态码和正文写进调用方的响应。
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
