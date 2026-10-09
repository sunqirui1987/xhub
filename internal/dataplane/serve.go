// serve.go is the adapted inference loop: chat, embeddings, images, audio,
// rerank, and the other catalog operations. Bypass does not enter this file.
//
// Serve order: identity, body, model name, budget, guardrail, extensions,
// cache, route, then one deployment at a time. A deployment without a key
// is skipped. A 5xx or 429 tries the next attempt and then the next
// deployment. The first response that is not one of those is the one that
// is logged and returned.
//
// Before RecordSpend the loop calls RememberExchange and AnnotateCall so the
// usage row has headers, bodies, provider, TTFT, session, and deployment.

package dataplane

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/sunqirui1987/xhub/internal/cache"
	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/httpx"
	"github.com/sunqirui1987/xhub/internal/llm"
	"github.com/sunqirui1987/xhub/internal/logx"
	"github.com/sunqirui1987/xhub/internal/plugin"
	"github.com/sunqirui1987/xhub/internal/provider"
	"github.com/sunqirui1987/xhub/internal/router"
)

var (
	errUpstreamStatus = errors.New("upstream_error")
	errEmptyUpstream  = errors.New("empty upstream stream")
)

// dropDisabled removes deployments the dashboard has disabled.
// 参数 pool：已经按模型名匹配过的部署。
// 返回 active：model_info.disabled 不为 true 的部署。disabled：被拿掉的个数。
// 调用：Serve、pickDeployment。
// 测试：disabled_test.go。
func dropDisabled(pool []config.ModelEntry) ([]config.ModelEntry, int) {
	active := make([]config.ModelEntry, 0, len(pool))
	disabled := 0
	for _, dep := range pool {
		if dep.Disabled() {
			disabled++
			continue
		}
		active = append(active, dep)
	}
	return active, disabled
}

// Serve runs one inference. It picks deployments with the routing strategy, encodes the upstream request, and tries the next deployment after a failure.
// Serve 跑一次适配推理。预算或护栏拒绝时响应已经写好，函数直接返回。
// 扩展按注册顺序在缓存和上游之前运行。重试次数小于 1 时按 1 次。没有 api_base
// 时拒绝调用。没有密钥或地址的部署跳过。
//
// 参数 h：Adapted，不含官方任务钉。w：调用方响应。r：入站请求，正文只读一次。
// 参数 op：目录操作名，例如 chat、embedding。
// 返回：无。成功、失败和拒绝都写在 w 上。
// 调用：gateway/limits.go dataPlane。测试：failure_log_test.go 的 TestServeLogs*。
func Serve(h Adapted, w http.ResponseWriter, r *http.Request, op string) {
	traceHop(r.URL.Path)
	logx.Trace("process path=%s step=start op=%s", r.URL.Path, op)
	start := time.Now()
	callID := httpx.CallID()
	httpx.SetCallID(w, callID)
	p := h.RequireLLMPrincipal(w, r)
	if p == nil {
		logx.Debug("process path=%s step=auth ok=false", r.URL.Path)
		return
	}
	logx.Debug("process path=%s step=auth ok=true kind=%s", r.URL.Path, p.Kind)
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		logx.Error("process path=%s step=body reason=invalid body", r.URL.Path)
		httpx.WriteTypedError(w, r.URL.Path, 400, "invalid_request", "invalid body")
		return
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		logx.Error("process path=%s step=body reason=invalid json", r.URL.Path)
		httpx.WriteTypedError(w, r.URL.Path, 400, "invalid_request", "invalid json")
		return
	}
	alias, _ := body["model"].(string)
	if alias == "" {
		logx.Error("process path=%s step=model reason=required", r.URL.Path)
		httpx.WriteTypedError(w, r.URL.Path, 400, "invalid_request", "model required")
		return
	}
	est := EstimateTokens(body)
	if !h.EnforceIdentityLimits(w, r.URL.Path, p, alias, est) {
		logx.Debug("process path=%s step=limits refused model=%s", r.URL.Path, alias)
		return
	}
	if op == "chat" || op == "responses" || op == "messages" || op == "" {
		if blocked, msg := h.GuardrailBlocks(callID, body); blocked {
			logx.Error("process path=%s step=guardrail blocked model=%s", r.URL.Path, alias)
			// The refusal is still a request. Record it so the logs drawer can
			// show which guardrail stopped the call.
			h.RememberExchange(callID, r, raw, nil)
			h.RecordSpend(w, p, callID, alias, op, nil, start, false, http.StatusBadRequest, "")
			httpx.WriteTypedError(w, r.URL.Path, 400, "guardrail_failed", msg)
			return
		}
		// Guardrails can redact the parsed request. Cache and exchange logs must
		// describe the same request that the encoder sends upstream.
		raw, err = json.Marshal(body)
		if err != nil {
			httpx.WriteTypedError(w, r.URL.Path, 400, "invalid_request", "invalid guarded request")
			return
		}
	}
	// Budget was already checked against the whole hierarchy by
	// EnforceIdentityLimits. This only bounds in-flight calls for one key.
	done := h.HookEngine().Begin(p.KeyID)
	if done != nil {
		defer done()
	}
	if refused := applyExtensions(w, r, h, op, alias); refused {
		logx.Debug("process path=%s step=extension refused model=%s", r.URL.Path, alias)
		return
	}

	cfg := h.GatewayConfig()
	// The settings this request runs under, resolved once. Its scope was already
	// picked out by the identity chain during the budget check, so this reads
	// one template row rather than walking anything.
	//
	// Every read below - the strategy, the retry count, the cooldown thresholds -
	// goes through this value rather than through cfg.RouterSettings, which is
	// the process-global document. That is what makes a scope that selected a
	// template and one that did not run the same code with different inputs.
	routeCfg := h.RouteSettingsFor(p)
	if routeCfg.Err != nil {
		httpx.WriteTypedError(w, r.URL.Path, 503, "unavailable", "route template unavailable")
		return
	}
	routeCfg = routeCfg.ForEndpoint(alias, provider.EndpointForOp(op))
	if routeCfg.Err != nil {
		httpx.WriteTypedError(w, r.URL.Path, 400, "invalid_request", routeCfg.Err.Error())
		return
	}
	tenant := p.Hash
	if tenant == "" {
		tenant = "user:" + p.UserID
	}
	plan := h.PlanRoute(r, alias, body, p)
	if plan.Err != nil {
		httpx.WriteTypedError(w, r.URL.Path, 400, "invalid_request", plan.Err.Error())
		return
	}
	var dialogue llm.Dialogue
	if callerProtocol(op) != "" {
		dialogue, err = llm.ParseDialogue(callerProtocol(op), dialogueRequestBody(body))
		if err != nil {
			httpx.WriteTypedError(w, r.URL.Path, 400, "unsupported_capability", err.Error())
			return
		}
	}
	cacheScope, err := json.Marshal(map[string]any{
		"models": cfg.ModelList, "router": routeCfg.Settings,
		"session": plan.SessionID, "query": r.URL.RawQuery,
	})
	if err != nil {
		httpx.WriteTypedError(w, r.URL.Path, 503, "unavailable", "invalid route configuration")
		return
	}
	ck := cache.Key(tenant, op, alias, string(raw), string(cacheScope))
	stream, _ := body["stream"].(bool)
	if !stream {
		if hit, ok := h.ResponseCache().Get(ck); ok {
			elapsed := time.Since(start)
			pt, ct := bodyUsage(hit)
			logx.Debug("process path=%s step=cache hit=true model=%s", r.URL.Path, alias)
			logMetrics(r.URL.Path, alias, true, pt, ct, elapsed, elapsed)
			h.RememberExchange(callID, r, raw, hit)
			h.AnnotateCall(callID, CallNote{SessionID: plan.SessionID, CacheKey: ck, CacheHit: true})
			h.WriteCacheHit(w, p, callID, alias, ck, op, hit, start)
			return
		}
		logx.Trace("process path=%s step=cache hit=false model=%s", r.URL.Path, alias)
	}

	if err := router.ValidateStrategy(routeCfg.Strategy()); err != nil {
		logx.Error("process path=%s step=strategy model=%s", r.URL.Path, alias)
		httpx.WriteTypedError(w, r.URL.Path, 400, "invalid_request", err.Error())
		return
	}
	// 权重仅取自解析后的模板，旧部署权重不会参与任何策略。
	models := cfg.ModelList
	if router.IsSplitStrategy(routeCfg.Strategy()) {
		models = router.ApplyWeights(models, routeCfg.WeightOverrides())
	}
	models, disabled := dropDisabled(models)
	// 实际请求与预览使用同一兼容性判断，策略只处理已经兼容的候选。
	if callerProtocol(op) != "" {
		models, _ = provider.Candidates(models, provider.EndpointForOp(op), &dialogue)
	} else {
		models, _ = provider.Candidates(models, provider.EndpointForOp(op), nil)
	}
	state := h.RouteState()
	state.SplitScope = routeCfg.CursorScope(alias, provider.EndpointForOp(op))
	pool := router.Schedule(models, alias, routeCfg.Strategy(), state, plan.Pinned)
	if plan.Required && (state.Cooldown[plan.Pinned] || len(pool) == 0 || router.CooldownID(pool[0]) != plan.Pinned) {
		httpx.WriteTypedError(w, r.URL.Path, 409, "continuation_unavailable", "original deployment is unavailable")
		return
	}
	if plan.Required {
		pool = pool[:1]
	}
	logx.Debug("process path=%s step=route model=%s deployments=%d stream=%t", r.URL.Path, alias, len(pool), stream)
	if len(pool) == 0 {
		if disabled > 0 {
			logx.Error("process path=%s step=route model=%s reason=disabled", r.URL.Path, alias)
			httpx.WriteTypedError(w, r.URL.Path, 400, "model_disabled", "model is disabled")
			return
		}
		logx.Error("model %s %s model not found: %s", r.Method, r.URL.Path, alias)
		httpx.WriteTypedError(w, r.URL.Path, 400, "invalid_request", "model not found: "+alias)
		return
	}
	// Retries already clamps to at least one, so a template that leaves the key
	// out cannot turn a request into one that is never attempted.
	attempts := routeCfg.Retries()

	var lastErr error
	var lastStatus int
	var lastProvider string
	triedHTTP := false
	missingCredential := false
	// unimplementedProvider records that a deployment was skipped because the
	// gateway has no protocol implementation for its provider name. It is
	// tracked separately from lastProvider, which is set before the provider is
	// checked and therefore cannot tell the two failures apart: an operator who
	// misspelled a provider name used to be told their API key was missing.

	for _, rawDep := range pool {
		if triedHTTP {
			// A retry is a new request against the upstream, so the credential
			// is resolved again: a key revoked between attempts must not carry
			// the retry, and the hierarchy is re-checked before spend is added.
			p2, err := h.ResolveRequest(r)
			if err != nil || p2 == nil || !p2.CanInfer() {
				httpx.WriteTypedError(w, r.URL.Path, 401, "invalid_api_key", "Authentication Error, No api key passed in.")
				return
			}
			p = p2
			if !h.EnforceIdentityLimits(w, r.URL.Path, p, alias, est) {
				return
			}
		}
		dep, credentialErr := h.AttachCredential(rawDep)
		if credentialErr != nil {
			missingCredential = true
			logx.Debug("skip deployment path=%s model=%s reason=credential_unavailable", r.URL.Path, alias)
			continue
		}
		upstreamModel := dep.ParamString("model", alias)
		supplier := dep.ParamString("custom_llm_provider", "")
		realModel := upstreamModel
		lastProvider = supplier
		var built llm.Upstream
		var err error
		apiBase := dep.ParamString("api_base", "")
		did := router.CooldownID(rawDep)
		if callerProtocol(op) != "" {
			built, err = buildDialogue(dep, dialogue)
			if errors.Is(err, errDialogueCredentials) {
				missingCredential = true
				logx.Debug("skip deployment path=%s provider=%s model=%s reason=missing api key or api base", r.URL.Path, supplier, realModel)
				continue
			}
			if err != nil {
				lastErr = err
				continue
			}
		} else {
			built, err = buildRegisteredOperation(dep, body, op)
			if errors.Is(err, errDialogueCredentials) {
				missingCredential = true
				continue
			}
			if err != nil {
				lastErr = err
				continue
			}
		}

		for try := 0; try < attempts; try++ {
			if try > 0 {
				fresh, err := h.ResolveRequest(r)
				if err != nil || fresh == nil || !fresh.CanInfer() {
					httpx.WriteTypedError(w, r.URL.Path, 401, "invalid_api_key", "invalid api key")
					return
				}
				p = fresh
				if !h.EnforceIdentityLimits(w, r.URL.Path, p, alias, est) {
					return
				}
			}
			triedHTTP = true
			logx.Trace("process path=%s step=attempt n=%d provider=%s model=%s base_host=%s", r.URL.Path, try+1, supplier, realModel, baseHost(apiBase))
			h.IncBusy(did)
			// The timeout belongs to this request's document, not to the process
			// client. The shared client is copied so its Timeout of 0 lets the
			// context be the only deadline; otherwise a template that says 10
			// seconds still waits for the timeout frozen at startup.
			attemptCtx, cancelAttempt := context.WithTimeout(r.Context(), time.Duration(routeCfg.TimeoutSeconds()*float64(time.Second)))
			req, err := http.NewRequestWithContext(attemptCtx, http.MethodPost, built.URL, bytes.NewReader(built.Body))
			if err != nil {
				cancelAttempt()
				h.DecBusy(did)
				lastErr = err
				logx.Error("upstream request path=%s provider=%s model=%s err=%s", r.URL.Path, supplier, realModel, safeErr(err))
				continue
			}
			for key, values := range built.Header {
				req.Header[key] = values
			}
			client := *h.HTTPClient()
			client.Timeout = 0
			// 明确地址不跟随重定向，防止凭据被转发到未配置的连接。
			client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
			resp, err := client.Do(req)
			if err != nil {
				cancelAttempt()
				h.DecBusy(did)
				lastErr = err
				h.NoteFailure(router.CooldownID(rawDep), routeCfg)
				logx.Error("upstream dial path=%s provider=%s model=%s base_host=%s err=%s", r.URL.Path, supplier, realModel, baseHost(apiBase), safeErr(err))
				continue
			}
			if resp.StatusCode >= 500 || resp.StatusCode == http.StatusTooManyRequests {
				_, _ = io.ReadAll(resp.Body)
				resp.Body.Close()
				cancelAttempt()
				h.DecBusy(did)
				h.NoteFailure(router.CooldownID(rawDep), routeCfg)
				lastStatus = resp.StatusCode
				lastErr = errUpstreamStatus
				logx.Error("upstream status path=%s provider=%s model=%s base_host=%s status=%d", r.URL.Path, supplier, realModel, baseHost(apiBase), resp.StatusCode)
				continue
			}

			h.NoteLatency(did, float64(time.Since(start).Milliseconds()))
			h.SetChatHeaders(w, p, alias, apiBase)
			if stream {
				var wrote bool
				var usage map[string]any
				var ttft time.Duration
				var streamed []byte
				var streamErr error
				if callerProtocol(op) != "" && resp.StatusCode < 400 {
					execution, _ := provider.Execution(dep)
					wrote, usage, ttft, streamed, streamErr = pipeDialogue(w, resp, start, alias, execution.Protocol, callerProtocol(op))
				} else {
					wrote, usage, ttft, streamed, streamErr = pipeStream(w, resp, start)
				}
				h.DecBusy(did)
				cancelAttempt()
				if streamErr != nil {
					lastErr = streamErr
					h.NoteFailure(did, routeCfg)
					logx.Error("upstream stream path=%s provider=%s model=%s base_host=%s err=%s", r.URL.Path, supplier, realModel, baseHost(apiBase), safeErr(streamErr))
					if wrote {
						h.RememberExchange(callID, r, raw, streamed)
						h.AnnotateCall(callID, CallNote{TTFTMs: ttftMillis(ttft), Provider: supplier, CacheKey: ck, SessionID: plan.SessionID})
						h.RecordSpend(w, p, callID, alias, op, usage, start, false, http.StatusBadGateway, did)
						return
					}
					continue
				}
				if wrote {
					if callerProtocol(op) != "" {
						usage = llm.NormalizeDialogueUsage(usage)
					} else {
						usage = completeUsage(usage, body, streamed)
					}
					pt, ct := usageCounts(usage)
					logMetrics(r.URL.Path, alias, false, pt, ct, ttft, time.Since(start))
					depID := did
					h.RememberExchange(callID, r, raw, streamed)
					h.AnnotateCall(callID, CallNote{
						TTFTMs: ttftMillis(ttft), Provider: supplier, CacheKey: ck,
						SessionID: plan.SessionID,
					})
					if resp.StatusCode < 400 {
						h.CommitRoute(plan, depID, responseID(streamed))
					}
					h.RecordSpend(w, p, callID, alias, op, usage, start, false, resp.StatusCode, depID)
					return
				}
				lastErr = errEmptyUpstream
				logx.Error("upstream stream path=%s provider=%s model=%s base_host=%s err=%s", r.URL.Path, supplier, realModel, baseHost(apiBase), safeErr(errEmptyUpstream))
				continue
			}
			respBody, readErr := io.ReadAll(resp.Body)
			resp.Body.Close()
			cancelAttempt()
			h.DecBusy(did)
			if readErr != nil {
				lastErr = readErr
				h.NoteFailure(did, routeCfg)
				continue
			}
			if callerProtocol(op) != "" && resp.StatusCode < 400 {
				execution, _ := provider.Execution(dep)
				result, conversionErr := llm.ParseDialogueResult(execution.Protocol, respBody)
				if conversionErr != nil {
					lastErr = conversionErr
					continue
				}
				respBody, conversionErr = llm.EncodeDialogueResult(result, callerProtocol(op), alias)
				if conversionErr != nil {
					lastErr = conversionErr
					continue
				}
			}
			elapsed := time.Since(start)
			pt, ct := bodyUsage(respBody)
			logx.Info("process path=%s step=upstream status=%d provider=%s model=%s", r.URL.Path, resp.StatusCode, supplier, realModel)
			logMetrics(r.URL.Path, alias, false, pt, ct, elapsed, elapsed)
			depID := did
			h.RememberExchange(callID, r, raw, respBody)
			h.AnnotateCall(callID, CallNote{
				TTFTMs: ttftMillis(elapsed), Provider: supplier, CacheKey: ck,
				SessionID: plan.SessionID,
			})
			if resp.StatusCode < 400 {
				h.CommitRoute(plan, depID, responseID(respBody))
			}
			h.WriteChatJSON(w, p, callID, alias, ck, op, "", respBody, resp.StatusCode, start, depID)
			return
		}
	}

	// Every terminal failure below is still a request the caller made. It is
	// recorded the way a 400 already is: a row with the status, no tokens and no
	// charge. Without this the failure is visible only in the process log, so a
	// customer reporting "my calls fail" leaves nothing to look at in the console.
	//
	// The three cases are told apart on purpose. Two of them used to share the
	// 401 "no upstream API key configured" message, which sent an operator who had
	// misspelled a provider name, or who had picked one the gateway cannot encode
	// a request for, looking for a credential that was never the problem.
	if !triedHTTP {
		switch {
		case missingCredential:
			logx.Error("dataplane path=%s status=401 code=authentication_error provider=%s", r.URL.Path, lastProvider)
			h.RecordSpend(w, p, callID, alias, op, nil, start, false, http.StatusUnauthorized, "")
			httpx.WriteTypedError(w, r.URL.Path, 401, "authentication_error", "This model has no upstream API key configured.")
		case lastErr != nil:
			// The provider name is known but the request could not be encoded for
			// it. Reporting this as a missing credential would be a lie, and the
			// encode error is the only thing that says what to fix.
			logx.Error("dataplane path=%s status=400 code=provider_not_implemented provider=%s err=%s",
				r.URL.Path, lastProvider, safeErr(lastErr))
			h.RecordSpend(w, p, callID, alias, op, nil, start, false, http.StatusBadRequest, "")
			detail := safeErr(lastErr)

			httpx.WriteTypedError(w, r.URL.Path, 400, "execution_error",
				"The request could not be encoded for "+lastProvider+": "+detail)
		default:
			logx.Error("dataplane path=%s status=400 code=provider_not_implemented", r.URL.Path)
			h.RecordSpend(w, p, callID, alias, op, nil, start, false, http.StatusBadRequest, "")
			httpx.WriteTypedError(w, r.URL.Path, 400, "execution_error", "execution_error")
		}
		return
	}
	if lastStatus > 0 {
		logx.Error("dataplane path=%s status=502 code=upstream_error detail=upstream %d", r.URL.Path, lastStatus)
		h.RecordSpend(w, p, callID, alias, op, nil, start, false, http.StatusBadGateway, "")
		httpx.WriteTypedError(w, r.URL.Path, 502, "upstream_error", "upstream "+strconv.Itoa(lastStatus))
		return
	}
	if lastErr != nil {
		logx.Error("dataplane path=%s status=502 code=upstream_error detail=%s", r.URL.Path, safeErr(lastErr))
		h.RecordSpend(w, p, callID, alias, op, nil, start, false, http.StatusBadGateway, "")
		httpx.WriteTypedError(w, r.URL.Path, 502, "upstream_error", safeErr(lastErr))
		return
	}
	logx.Error("dataplane path=%s status=502 code=upstream_error detail=all deployments failed", r.URL.Path)
	h.RecordSpend(w, p, callID, alias, op, nil, start, false, http.StatusBadGateway, "")
	httpx.WriteTypedError(w, r.URL.Path, 502, "upstream_error", "all deployments failed")
}

// preferDeployment 把钉住的部署移到候选列表的第一位。id 为空或不在列表里时顺序不变。
// 参数 pool：路由排序后的部署。id：deployment_affinity 里存的 CooldownID。
// 返回：同一批部署，钉住的那条在下标 0。
// 调用：Serve 在 PlanRoute 给出 Pinned 之后。测试：prefer_test.go TestPreferDeployment*。
func preferDeployment(pool []config.ModelEntry, id string) []config.ModelEntry {
	if id == "" {
		return pool
	}
	for i, entry := range pool {
		if router.CooldownID(entry) != id {
			continue
		}
		if i == 0 {
			return pool
		}
		out := make([]config.ModelEntry, 0, len(pool))
		out = append(out, entry)
		out = append(out, pool[:i]...)
		out = append(out, pool[i+1:]...)
		return out
	}
	return pool
}

// applyExtensions 按注册顺序跑推理前扩展。任一扩展拒绝时响应已写好，并返回真。
// 参数 w、r：写拒绝和读取路径。h：提供扩展注册表。op、alias：传给扩展的操作和模型名。
// 返回：true 表示调用方应停止，不再访问上游。
// 调用：Serve。测试夹具的扩展注册表是空的，所以现有测试走 false 这条。
func applyExtensions(w http.ResponseWriter, r *http.Request, h Adapted, op, alias string) bool {
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
