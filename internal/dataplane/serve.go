// 一次推理的发送循环：选部署、编码请求、失败后换下一个。
package dataplane

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/sunqirui1987/xhub/internal/cache"
	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/httpx"
	"github.com/sunqirui1987/xhub/internal/llm"
	"github.com/sunqirui1987/xhub/internal/llm/protocol"
	"github.com/sunqirui1987/xhub/internal/plugin"
	"github.com/sunqirui1987/xhub/internal/router"
)

var (
	errUpstreamStatus = errors.New("upstream_error")
	errEmptyUpstream  = errors.New("empty upstream stream")
)

// Serve 执行一次推理。它按路由策略挑部署，编码上游请求，失败时换下一个部署。
// 身份、预算和限流不在这里实现，失败时 Host 已经写好响应。
// 模型校验通过之后、读缓存和访问上游之前，按注册顺序跑扩展。拒绝则写错误并返回。
// 扩展表为空时这一步直接放行。重试次数小于 1 时按 1 次。凭证或 api_base 为空的部署会被跳过。
func Serve(h Host, w http.ResponseWriter, r *http.Request, op string) {
	start := time.Now()
	callID := httpx.CallID()
	httpx.SetCallID(w, callID)
	p := h.RequireLLMPrincipal(w, r)
	if p == nil {
		return
	}
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		httpx.WriteTypedError(w, r.URL.Path, 400, "invalid_request", "invalid body")
		return
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		httpx.WriteTypedError(w, r.URL.Path, 400, "invalid_request", "invalid json")
		return
	}
	alias, _ := body["model"].(string)
	if alias == "" {
		httpx.WriteTypedError(w, r.URL.Path, 400, "invalid_request", "model required")
		return
	}
	est := EstimateTokens(body)
	if !h.EnforceIdentityLimits(w, r.URL.Path, p, alias, est) {
		return
	}
	if op == "chat" || op == "" {
		if blocked, msg := h.GuardrailBlocks(body); blocked {
			httpx.WriteTypedError(w, r.URL.Path, 400, "guardrail_failed", msg)
			return
		}
	}
	done, reason := h.HookEngine().Begin(p.Key)
	if reason == "budget" {
		httpx.WriteTypedError(w, r.URL.Path, 429, "budget_exceeded", "Budget has been exceeded")
		return
	}
	if reason == "parallel" {
		httpx.WriteTypedError(w, r.URL.Path, 429, "rate_limit", "max_parallel_requests exceeded")
		return
	}
	if done != nil {
		defer done()
	}
	if refused := applyExtensions(w, r, h, op, alias); refused {
		return
	}

	cfg := h.GatewayConfig()
	tenant := ""
	if p.Key != nil {
		tenant = p.Hash
	}
	ck := cache.Key(tenant, op, alias, string(raw))
	stream, _ := body["stream"].(bool)
	if !stream {
		if hit, ok := h.ResponseCache().Get(ck); ok {
			h.WriteCacheHit(w, p, callID, alias, ck, hit, start)
			return
		}
	}

	if err := router.ValidateStrategy(cfg.RouterSettings.RoutingStrategy); err != nil {
		httpx.WriteTypedError(w, r.URL.Path, 400, "invalid_request", err.Error())
		return
	}
	pool := router.Order(cfg.ModelList, alias, cfg.RouterSettings.RoutingStrategy, h.RouteState())
	if len(pool) == 0 {
		httpx.WriteTypedError(w, r.URL.Path, 400, "invalid_request", "model not found: "+alias)
		return
	}
	attempts := cfg.RouterSettings.NumRetries
	if attempts < 1 {
		attempts = 1
	}

	var lastErr error
	var lastStatus int
	var lastProvider string
	triedHTTP := false
	missingCredential := false

	for di, rawDep := range pool {
		if di > 0 {
			p2, err := h.ResolveRequest(r)
			if err != nil || p2 == nil || !p2.CanLLM(cfg) {
				httpx.WriteTypedError(w, r.URL.Path, 401, "invalid_api_key", "Authentication Error, No api key passed in.")
				return
			}
			p = p2
			if !h.EnforceIdentityLimits(w, r.URL.Path, p, alias, est) {
				return
			}
		}
		dep := h.AttachCredential(rawDep)
		upstreamModel := dep.ParamString("model", alias)
		provider, realModel := config.SplitProviderModel(upstreamModel)
		if custom := dep.ParamString("custom_llm_provider", ""); custom != "" {
			provider = custom
		}
		provider = strings.ToLower(strings.TrimSpace(provider))
		lastProvider = provider
		if _, ok := protocol.ProtocolGroup(provider); !ok || provider == "" {
			continue
		}
		apiBase := trimBase(dep.ParamString("api_base", ""))
		apiKey := dep.ParamString("api_key", "")
		if apiKey == "" || apiBase == "" {
			missingCredential = true
			continue
		}
		did := dep.ParamString("api_base", "") + "|" + upstreamModel
		built, err := llm.Build(r.Context(), llm.Request{
			Op: op, Provider: provider, APIBase: apiBase, APIKey: apiKey, Model: realModel, Body: body,
			VertexProject:  dep.ParamString("vertex_project", ""),
			VertexLocation: dep.ParamString("vertex_location", ""),
		})
		if err != nil {
			lastErr = err
			continue
		}

		for try := 0; try < attempts; try++ {
			triedHTTP = true
			h.IncBusy(did)
			req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, built.URL, bytes.NewReader(built.Body))
			if err != nil {
				h.DecBusy(did)
				lastErr = err
				continue
			}
			for key, values := range built.Header {
				req.Header[key] = values
			}
			resp, err := h.HTTPClient().Do(req)
			if err != nil {
				h.DecBusy(did)
				lastErr = err
				continue
			}
			if resp.StatusCode >= 500 || resp.StatusCode == http.StatusTooManyRequests {
				_, _ = io.ReadAll(resp.Body)
				resp.Body.Close()
				h.DecBusy(did)
				h.NoteFailure(router.DeploymentID(rawDep))
				lastStatus = resp.StatusCode
				lastErr = errUpstreamStatus
				continue
			}

			h.NoteLatency(router.DeploymentID(rawDep), float64(time.Since(start).Milliseconds()))
			h.SetChatHeaders(w, p, alias, apiBase)
			if stream {
				wrote, usage := pipeStream(w, resp)
				h.DecBusy(did)
				if wrote {
					if usage == nil {
						usage = map[string]any{"prompt_tokens": EstimateTokens(body), "completion_tokens": 0}
					}
					h.RecordSpend(w, p, callID, alias, usage, start, false, router.DeploymentID(rawDep))
					return
				}
				lastErr = errEmptyUpstream
				continue
			}
			respBody, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			h.DecBusy(did)
			h.WriteChatJSON(w, p, callID, alias, ck, op, provider, respBody, resp.StatusCode, start, router.DeploymentID(rawDep))
			return
		}
	}

	if !triedHTTP {
		if lastProvider != "" || missingCredential {
			httpx.WriteTypedError(w, r.URL.Path, 401, "authentication_error", "Authentication Error, No api key passed in.")
			return
		}
		httpx.WriteTypedError(w, r.URL.Path, 400, "provider_not_implemented", "provider_not_implemented")
		return
	}
	if lastStatus > 0 {
		httpx.WriteTypedError(w, r.URL.Path, 502, "upstream_error", "upstream "+strconv.Itoa(lastStatus))
		return
	}
	if lastErr != nil {
		httpx.WriteTypedError(w, r.URL.Path, 502, "upstream_error", lastErr.Error())
		return
	}
	httpx.WriteTypedError(w, r.URL.Path, 502, "upstream_error", "all deployments failed")
}

// applyExtensions 在上游 HTTP 之前按注册顺序跑扩展。拒绝时已经写好响应并返回 true。
// 状态码缺省 400，错误码缺省 extension_refused。没有注册项时返回 false。
func applyExtensions(w http.ResponseWriter, r *http.Request, h Host, op, alias string) bool {
	d := h.Extensions().Run(plugin.Call{Op: op, Model: alias, Path: r.URL.Path})
	for k, v := range d.Header {
		w.Header().Set(k, v)
	}
	if !d.Refuse {
		return false
	}
	status := d.Status
	if status == 0 {
		status = 400
	}
	code := d.Code
	if code == "" {
		code = "extension_refused"
	}
	msg := d.Message
	if msg == "" {
		msg = "extension refused the call"
	}
	httpx.WriteTypedError(w, r.URL.Path, status, code, msg)
	return true
}

// EstimateTokens 用请求体长度粗算预扣 token。它不是分词器，只给预算和 TPM 一个上界。
func EstimateTokens(body map[string]any) int {
	n := 32
	if mt := asInt(body["max_tokens"]); mt > 0 {
		n += mt
	} else {
		n += 64
	}
	switch t := body["messages"].(type) {
	case []any:
		for _, m := range t {
			if mm, ok := m.(map[string]any); ok {
				n += len(textOf(mm["content"])) / 4
			}
		}
	}
	if p, ok := body["prompt"].(string); ok {
		n += len(p) / 4
	}
	if p, ok := body["input"].(string); ok {
		n += len(p) / 4
	}
	return n
}

// 把值当成字符串。不是字符串时返回空串。
func textOf(v any) string {
	s, _ := v.(string)
	return s
}

// 把 JSON 数字收成 int。float64 会截断小数。其它类型返回 0。
func asInt(v any) int {
	switch t := v.(type) {
	case float64:
		return int(t)
	case int:
		return t
	default:
		return 0
	}
}

// 去掉 api_base 末尾的斜杠，避免和端点路径拼出双斜杠。
func trimBase(s string) string {
	for len(s) > 0 && s[len(s)-1] == '/' {
		s = s[:len(s)-1]
	}
	return s
}

// 把上游 SSE 原样写给客户端，并尽量从流里抽出 usage。还没写出任何字节时返回 wrote=false。
func pipeStream(w http.ResponseWriter, resp *http.Response) (bool, map[string]any) {
	defer resp.Body.Close()
	buf := make([]byte, 4096)
	flusher, _ := w.(http.Flusher)
	wrote := false
	var pending []byte
	var usage map[string]any
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			if !wrote {
				w.Header().Set("Content-Type", "text/event-stream")
				w.WriteHeader(resp.StatusCode)
				wrote = true
			}
			_, _ = w.Write(buf[:n])
			if flusher != nil {
				flusher.Flush()
			}
			pending = append(pending, buf[:n]...)
			usage = streamUsage(pending, usage)
			if len(pending) > 1<<20 {
				pending = pending[len(pending)-4096:]
			}
		}
		if err != nil {
			return wrote, usage
		}
	}
}

// 从已经收到的 SSE 缓冲里找最后一次 usage。没有则沿用 prev。
func streamUsage(raw []byte, prev map[string]any) map[string]any {
	for _, line := range bytes.Split(raw, []byte("\n")) {
		line = bytes.TrimSpace(line)
		line = bytes.TrimPrefix(line, []byte("data:"))
		line = bytes.TrimSpace(line)
		if len(line) == 0 || bytes.Equal(line, []byte("[DONE]")) {
			continue
		}
		var doc map[string]any
		if json.Unmarshal(line, &doc) != nil {
			continue
		}
		if u, ok := doc["usage"].(map[string]any); ok {
			prev = u
		}
	}
	return prev
}
