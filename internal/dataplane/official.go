// official.go is the bypass forwarder. The gateway calls ServeBypass only
// after an endpoint type of kind bypass has matched. This file does not
// encode the body into chat completions.
//
// Create replaces the model field, forwards the rest, and pins the task id
// to that deployment for seven days. A later get or list must resolve to a
// deployment that selected the same endpoint type, so a Volcengine task id
// is not fetched from Qiniu. Usage on a follow-up response is billed once.

package dataplane

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/sunqirui1987/xhub/internal/auth"
	"github.com/sunqirui1987/xhub/internal/catalog"
	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/httpx"
	"github.com/sunqirui1987/xhub/internal/logx"
	"github.com/sunqirui1987/xhub/internal/provider"
	"github.com/sunqirui1987/xhub/internal/router"
)

// ServeBypass 转发一次已经匹配到的官方调用。正文只改模型字段，状态码和响应字节原样返回。
// usage 在后续查询第一次出现时扣一次。
// 参数 h：Bypass 接口。w：调用方响应。r：入站请求。hit：路径匹配到的端点类型和动作。
// 返回：无。错误和上游正文都写在 w 上。
// 调用：gateway/bypass.go serveBypass。测试：bypass_logic_test.go TestEndpointAndModelLogic。
func ServeBypass(h Bypass, w http.ResponseWriter, r *http.Request, hit provider.Hit) {
	start := time.Now()
	callID := httpx.CallID()
	httpx.SetCallID(w, callID)
	p := h.RequireLLMPrincipal(w, r)
	if p == nil {
		return
	}
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		httpx.WriteTypedError(w, r.URL.Path, 400, "invalid_request", "could not read request body")
		return
	}
	var body map[string]any
	if len(bytes.TrimSpace(raw)) > 0 {
		if json.Unmarshal(raw, &body) != nil {
			httpx.WriteTypedError(w, r.URL.Path, 400, "invalid_request", "invalid json")
			return
		}
	}
	if hit.Action.Name == "create" || hit.Transport.ModelField != "" && hit.Action.Name == "" {
		serveBypassCreate(h, w, r, hit, p, body, raw, callID, start)
		return
	}
	id := hit.Names["id"]
	if id == "" {
		id = hit.Names["task_id"]
	}
	if hit.Action.TaskQuery != "" {
		id = strings.TrimSpace(r.URL.Query().Get(hit.Action.TaskQuery))
	}
	serveBypassFollow(h, w, r, hit, id, callID, start, p)
}

// serveBypassCreate 处理创建。按模型字段找勾了该类型的部署，换成官方模型 id 和上游密钥。
// 创建响应里的任务 id 钉住这条部署。这次不按 usage 扣费。
//
// 参数 h（Bypass）：官方转发需要的能力，不含聊天缓存和护栏；w（http.ResponseWriter）：创建结果或错误写在这里；r（*http.Request）：入站请求，用来读路径和头；hit（provider.Hit）：这次匹配到的端点类型和动作。
// 参数 principal（*auth.Principal）：已通过鉴权的调用方。body（map[string]any）：解析后的 JSON。raw（[]byte）：原始正文。
// 参数 callID（string）：用量行 id。start（time.Time）：请求开始时间。
// 返回：无。
// 调用：ServeBypass 在动作名是 create 时。测试：逻辑测试里的火山、七牛和 Suno 创建。
func serveBypassCreate(h Bypass, w http.ResponseWriter, r *http.Request, hit provider.Hit, principal *auth.Principal, body map[string]any, raw []byte, callID string, start time.Time) {
	field := hit.Transport.ModelField
	if field == "" {
		field = "model"
	}
	alias, _ := body[field].(string)
	alias = strings.TrimSpace(alias)
	if alias == "" {
		httpx.WriteTypedError(w, r.URL.Path, 400, "invalid_request", field+" required")
		return
	}
	if !h.EnforceIdentityLimits(w, r.URL.Path, principal, alias, EstimateTokens(body)) {
		return
	}
	settings := h.RouteSettingsFor(principal)
	if settings.Err != nil {
		httpx.WriteTypedError(w, r.URL.Path, 503, "unavailable", "route template unavailable")
		return
	}
	settings = settings.ForModel(alias)
	if settings.Err != nil {
		httpx.WriteTypedError(w, r.URL.Path, 400, "invalid_request", settings.Err.Error())
		return
	}
	if err := router.ValidateStrategy(settings.Strategy()); err != nil {
		httpx.WriteTypedError(w, r.URL.Path, 400, "invalid_request", err.Error())
		return
	}
	list := eligible(h, hit)
	if router.IsSplitStrategy(settings.Strategy()) {
		list = router.ApplyWeights(list, settings.WeightOverrides())
	}
	list, _ = dropDisabled(list)
	pool := router.Order(list, alias, settings.Strategy(), h.RouteState())
	if len(pool) == 0 {
		httpx.WriteTypedError(w, r.URL.Path, 400, "invalid_request", "model not found: "+alias)
		return
	}
	var dep config.ModelEntry
	var respBody []byte
	var status int
	var selectedID string
	tried := false
	originalHit := hit
	for _, rawDep := range pool {
		candidate, err := h.AttachCredential(rawDep)
		if err != nil {
			continue
		}
		candidateHit := provider.ApplyOverride(candidate, originalHit)
		hit = candidateHit
		base, key := bypassAuth(candidateHit, candidate)
		if base == "" || key == "" {
			continue
		}
		body[field] = provider.OfficialID(candidateHit.Transport.StripPrefix, candidate.ParamString("model", alias))
		payload, err := json.Marshal(body)
		if err != nil {
			httpx.WriteTypedError(w, r.URL.Path, 400, "invalid_request", "invalid json")
			return
		}
		url := withQuery(base+provider.Expand(candidateHit.Action.UpstreamPath, nil), r)
		for attempt := 0; attempt < settings.Retries(); attempt++ {
			if tried {
				fresh, err := h.ResolveRequest(r)
				if err != nil || fresh == nil {
					httpx.WriteTypedError(w, r.URL.Path, 401, "invalid_api_key", "invalid api key")
					return
				}
				principal = fresh
				if !h.EnforceIdentityLimits(w, r.URL.Path, principal, alias, EstimateTokens(body)) {
					return
				}
			}
			tried = true
			ctx, cancel := context.WithTimeout(r.Context(), time.Duration(settings.TimeoutSeconds()*float64(time.Second)))
			respBody, status, err = forwardOfficial(h, r.WithContext(ctx), http.MethodPost, url, key, payload, r.Header)
			cancel()
			dep = candidate
			selectedID = router.CooldownID(rawDep)
			if err != nil {
				logx.Error("bypass forward path=%s err=%s", r.URL.Path, safeErr(err))
				h.NoteFailure(router.CooldownID(rawDep), settings)
				// A lost create response may already have created a task. Do not
				// replay an ambiguous non-idempotent request.
				httpx.WriteTypedError(w, r.URL.Path, 502, "upstream_error", "upstream request failed")
				return
			}
			if status >= 500 || status == http.StatusTooManyRequests {
				h.NoteFailure(router.CooldownID(rawDep), settings)
				if r.Context().Err() != nil {
					return
				}
				continue
			}
			hit = candidateHit
			goto forwarded
		}
	}
	if !tried {
		httpx.WriteTypedError(w, r.URL.Path, 401, "upstream_auth", "This model has no upstream API key configured.")
		return
	}
forwarded:
	var doc map[string]any
	_ = json.Unmarshal(respBody, &doc)
	depID := selectedID
	if taskID := provider.ReadTaskID(doc, hit.Transport.TaskID); taskID != "" && status < 400 {
		h.PinOfficial(officialTaskScope(principal, hit.Transport.ID, taskID), selectedID)
	}
	plan := h.PlanRoute(r, alias, body, principal)
	h.RememberExchange(callID, r, raw, respBody)
	h.AnnotateCall(callID, dataplaneNote(hit, dep, plan.SessionID, depID, time.Since(start)))
	h.RecordSpend(w, principal, callID, alias, hit.Transport.ID+":"+hit.Action.Name, nil, start, false, status, depID)
	writeThrough(w, status, respBody)
}

// serveBypassFollow 处理查询和列表。id 非空时走任务钉，并要求部署仍属于这个端点类型。
// id 为空时是列表：只有一个上游密钥才转发，多个则写 400 和模型名。
// 参数 id：路径或查询串里的任务 id，列表时为空。其余参数与创建相同。
// 返回：无。
// 调用：ServeBypass。测试：逻辑测试的重复扣费、跨供应商 404、列表和 Suno 查询参数。
func serveBypassFollow(h Bypass, w http.ResponseWriter, r *http.Request, hit provider.Hit, id, callID string, start time.Time, principal *auth.Principal) {
	var dep config.ModelEntry
	var ok bool
	scopedID := officialTaskScope(principal, hit.Transport.ID, id)
	var depID string
	if id != "" {
		pinned := h.OfficialDeployment(scopedID)
		if pinned == "" {
			httpx.WriteTypedError(w, r.URL.Path, 404, "not_found", "unknown task")
			return
		}
		dep, ok = h.FindDeployment(pinned)
		depID = pinned
		if !ok || !sameEndpoint(dep, hit) {
			httpx.WriteTypedError(w, r.URL.Path, 404, "not_found", "unknown task")
			return
		}
	} else {
		var names []string
		dep, names, ok = oneUpstream(h, hit)
		if !ok {
			if len(names) == 0 {
				httpx.WriteTypedError(w, r.URL.Path, 400, "invalid_request", "model not found")
				return
			}
			httpx.WriteTypedError(w, r.URL.Path, 400, "invalid_request", "model required: "+strings.Join(names, ", "))
			return
		}
	}
	if depID == "" {
		depID = router.CooldownID(dep)
	}
	if !h.EnforceIdentityLimits(w, r.URL.Path, principal, dep.ModelName, 0) {
		return
	}
	settings := h.RouteSettingsFor(principal)
	if settings.Err != nil {
		httpx.WriteTypedError(w, r.URL.Path, 503, "unavailable", "route template unavailable")
		return
	}
	hit = provider.ApplyOverride(dep, hit)
	dep, err := h.AttachCredential(dep)
	if err != nil {
		httpx.WriteTypedError(w, r.URL.Path, 401, "upstream_auth", "This model has no upstream API key configured.")
		return
	}
	base, key := bypassAuth(hit, dep)
	if base == "" || key == "" {
		httpx.WriteTypedError(w, r.URL.Path, 401, "upstream_auth", "This model has no upstream API key configured.")
		return
	}
	url := withQuery(base+provider.Expand(hit.Action.UpstreamPath, hit.Names), r)
	ctx, cancel := context.WithTimeout(r.Context(), time.Duration(settings.TimeoutSeconds()*float64(time.Second)))
	defer cancel()
	respBody, status, err := forwardOfficial(h, r.WithContext(ctx), r.Method, url, key, nil, r.Header)
	if err != nil {
		httpx.WriteTypedError(w, r.URL.Path, 502, "upstream_error", "upstream request failed")
		return
	}
	var doc map[string]any
	_ = json.Unmarshal(respBody, &doc)
	usage := usageFromOfficial(doc)
	settlementID := ""
	if id != "" && status >= 200 && status < 300 && positiveOfficialUsage(usage) && !officialPending(doc) {
		// Re-submit on every completed poll. The durable request_id constraint,
		// not a process-local marker written before persistence, claims the bill.
		settlementID = officialSettlementID(scopedID, depID)
	} else {
		usage = nil
	}
	plan := h.PlanRoute(r, dep.ModelName, nil, principal)
	h.RememberExchange(callID, r, nil, respBody)
	note := dataplaneNote(hit, dep, plan.SessionID, depID, time.Since(start))
	note.SettlementID = settlementID
	note.SkipRouteUsage = true
	h.AnnotateCall(callID, note)
	h.RecordSpend(w, principal, callID, dep.ModelName, hit.Transport.ID+":"+hit.Action.Name, usage, start, false, status, depID)
	writeThrough(w, status, respBody)
}

// 参数 depID：api_base|model。elapsed：从请求开始到上游响应结束。
// 返回：CallNote。TTFTMs 在耗时大于 0 时至少为 1 毫秒。
// 调用：创建和查询在 RecordSpend 之前。测试：逻辑测试断言 Provider 为 volcengine。
// sameEndpoint reports that the pinned deployment is the one this path belongs to.
// A task id created on Volcengine is not a Qiniu task.
// dataplaneNote 组装 Bypass 写入用量行的备注。供应商优先用部署上的 custom_llm_provider，
// 否则用端点类型的前缀，再否则用类型 id。
// 参数 hit：端点类型。dep：选中的部署。sessionID：PlanRoute 给出的会话，可空。
// 参数 depID：api_base|model。elapsed：从请求开始到上游响应结束。
// 返回：CallNote。TTFTMs 在耗时大于 0 时至少为 1 毫秒。
// 调用：创建和查询在 RecordSpend 之前。测试：逻辑测试断言 Provider 为 volcengine。
func dataplaneNote(hit provider.Hit, dep config.ModelEntry, sessionID, depID string, elapsed time.Duration) CallNote {
	name := dep.ParamString("custom_llm_provider", "")
	if name == "" {
		name = hit.Transport.StripPrefix
	}
	if name == "" {
		name = hit.Transport.ID
	}
	return CallNote{
		TTFTMs:       ttftMillis(elapsed),
		Provider:     name,
		SessionID:    sessionID,
		DeploymentID: depID,
	}
}

// sameEndpoint 确认钉住的部署和当前路径属于同一类端点。火山任务不能走到七牛路径。
// 参数 dep：钉上找回的部署。hit：当前路径匹配到的类型。自定义路径还比对部署名。
// 返回：true 才继续用这行部署转发。
// 调用：serveBypassFollow。测试：逻辑测试用 cgt- 打七牛路径得到 404。
func sameEndpoint(dep config.ModelEntry, hit provider.Hit) bool {
	if hit.DeploymentName != "" {
		return dep.ModelName == hit.DeploymentName
	}
	return provider.Includes(dep, hit.Transport.ID)
}

// eligible 列出模型表里属于当前端点类型、或名字与自定义路径一致的部署。
// 参数 h、hit：读模型表和当前匹配。
// 返回：未再过滤暂停状态的部署。暂停由创建和 oneUpstream 去掉。
// 调用：serveBypassCreate、oneUpstream。测试：bypass_logic_test.go。
func eligible(h Bypass, hit provider.Hit) []config.ModelEntry {
	var out []config.ModelEntry
	for _, m := range h.Models() {
		if hit.DeploymentName != "" {
			if m.ModelName == hit.DeploymentName {
				out = append(out, m)
			}
			continue
		}
		if provider.Includes(m, hit.Transport.ID) {
			out = append(out, m)
		}
	}
	return out
}

// oneUpstream 为没有任务 id 的列表选部署。按 api_base 加 api_key 分组。 只有一组时返回那条部署。多组不能合成一家的官方响应。
// 参数 h（Bypass）：h；hit（provider.Hit）：hit。
// 调用：仅在 official.go 内使用
// 测试：无直接单测
func oneUpstream(h Bypass, hit provider.Hit) (config.ModelEntry, []string, bool) {
	list, _ := dropDisabled(eligible(h, hit))
	seen := map[string]config.ModelEntry{}
	var names []string
	for _, m := range list {
		names = append(names, m.ModelName)
		attached, err := h.AttachCredential(m)
		if err != nil {
			continue
		}
		base, key := bypassAuth(hit, attached)
		id := base + "|" + key
		if _, ok := seen[id]; !ok {
			seen[id] = attached
		}
	}
	if len(seen) == 1 {
		for _, dep := range seen {
			return dep, nil, true
		}
	}
	return config.ModelEntry{}, names, false
}

// bypassAuth 决定这次转发的根地址和密钥。部署上的 api_base 优先，否则用端点类型的默认根地址， 再否则用供应商登记的默认根地址。 返回 base：去掉末尾斜杠的根地址。key：api _key。任一为空时调用方写 401。 调用：创建、查询、列表分组。测试：逻辑测试把 api_base 指到 httptest，并检查 Bearer。
// 参数 hit（provider.Hit）：这次路由命中的端点类型。Type 决定上游动作，路径参数在 Params 里；dep（config.ModelEntry）：一条模型部署，含对外名、供应商参数和价格覆盖。
// 返回 string（string）：去掉末尾斜杠的上游根地址。部署、端点类型、供应商默认都空时为空串，调用方写 401；string（string）：部署上的 api_key。空串时调用方写 401。
// 调用：仅在 official.go 内使用
// 测试：无直接单测
func bypassAuth(hit provider.Hit, dep config.ModelEntry) (string, string) {
	base := trimBase(dep.ParamString("api_base", ""))
	if base == "" {
		base = trimBase(hit.Transport.APIBase)
	}
	if base == "" {
		base = provider.APIBase(hit.Transport.StripPrefix, "")
	}
	return base, dep.ParamString("api_key", "")
}

// withQuery 把入站查询串接到上游 URL 后面。没有查询串时原样返回。
// 参数 url：根地址加上游路径。r：入站请求。
// 返回：可能带 ? 的完整 URL。
// 调用：创建和查询。测试：逻辑测试断言 page_size 和 taskId 被保留。
func withQuery(url string, r *http.Request) string {
	if r.URL.RawQuery == "" {
		return url
	}
	return url + "?" + r.URL.RawQuery
}

// forwardOfficial 向供应商发一次 HTTP。Authorization 换成这条部署的密钥。
// Host、网关身份和逐跳头不转发。响应最多读 8 MiB，超出返回错误。
//
// 参数 method、url：上游方法和地址。apiKey：Bearer 密钥。body：创建时的 JSON，查询时为 nil。
// 参数 in：入站头。
// 返回：响应字节、状态码、拨号或读取错误。
// 调用：创建和查询。测试：逻辑测试的假上游记录方法、路径、头和正文。
func forwardOfficial(h Bypass, r *http.Request, method, url, apiKey string, body []byte, in http.Header) ([]byte, int, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(r.Context(), method, url, reader)
	if err != nil {
		return nil, 0, err
	}
	for k, vals := range in {
		switch strings.ToLower(k) {
		case "authorization", "host", "content-length", "cookie", "x-api-key", "api-key", "x-litellm-api-key", "connection", "proxy-connection", "keep-alive", "proxy-authorization", "proxy-authenticate", "te", "trailer", "transfer-encoding", "upgrade":
			continue
		}
		connectionHeader := false
		for _, named := range strings.Split(in.Get("Connection"), ",") {
			if strings.EqualFold(strings.TrimSpace(named), k) {
				connectionHeader = true
				break
			}
		}
		if connectionHeader {
			continue
		}
		for _, v := range vals {
			req.Header.Add(k, v)
		}
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	if body != nil && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}
	client := *h.HTTPClient()
	if _, deadline := r.Context().Deadline(); deadline {
		client.Timeout = 0
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	out, err := io.ReadAll(io.LimitReader(resp.Body, (8<<20)+1))
	if err == nil && len(out) > 8<<20 {
		err = fmt.Errorf("official response exceeds 8 MiB")
	}
	return out, resp.StatusCode, err
}

// officialTaskScope 隔离不同调用方和不同供应商端点的任务编号。
// 参数 p、transport、id：调用方、端点类型和上游任务编号。
// 返回：固定长度、不含凭证的任务范围键。
// 调用：创建和查询。测试：official_template_test.go。
func officialTaskScope(p *auth.Principal, transport, id string) string {
	caller := ""
	if p != nil {
		if p.Hash != "" {
			caller = "key:" + p.Hash
		} else {
			caller = "user:" + p.UserID
		}
	}
	raw, _ := json.Marshal([]string{caller, transport, id})
	sum := sha256.Sum256(raw)
	return "v2:" + hex.EncodeToString(sum[:])
}

// officialSettlementID includes the pinned credential/deployment as well as
// the caller, transport and task scope. Zero/progress polls keep their own IDs.
// 参数 scopedID：隔离调用主体和任务的身份。depID：钉住的部署身份。
// 返回：稳定的持久层结算 request_id。
// 调用：serveBypassFollow。测试：official_settlement_test.go。
func officialSettlementID(scopedID, depID string) string {
	raw, _ := json.Marshal([]string{scopedID, depID})
	sum := sha256.Sum256(raw)
	return "official-settlement:" + hex.EncodeToString(sum[:])
}

// positiveOfficialUsage 判断规范化后是否有正用量。
// 参数 usage：上游用量对象。返回：任一计费维度为正时为真。
// 调用：serveBypassFollow。测试：official_settlement_test.go。
func positiveOfficialUsage(usage map[string]any) bool {
	u := catalog.NormalizeUsage(usage)
	return u.PromptTokens > 0 || u.CompletionTokens > 0 || u.CachedTokens > 0 ||
		u.CacheWriteTokens > 0 || u.Images > 0 || u.Seconds > 0 || u.Searches > 0
}

// officialPending 判断显式未完成或失败的任务，避免把进度用量当作最终账单。
// 参数 doc：上游任务响应。返回：任务尚未成功完成时为真。
// 调用：serveBypassFollow。测试：official_template_test.go。
func officialPending(doc map[string]any) bool {
	status, _ := doc["status"].(string)
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "queued", "pending", "running", "processing", "submitted", "failed", "error", "cancelled", "canceled":
		return true
	}
	return false
}

// writeThrough 把上游状态码和正文写给调用方，Content-Type 固定为 application/json。
// status 为 0 时改成 502。
// 参数 w：调用方响应。status、body：forwardOfficial 的结果。
// 返回：无。
// 调用：创建和查询的成功路径。无单独测试。
func writeThrough(w http.ResponseWriter, status int, body []byte) {
	if status == 0 {
		status = http.StatusBadGateway
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

// usageFromOfficial 从官方响应里取出 usage 对象。没有用量字段时返回 nil，创建请求因此不计费。
// 参数 doc：已解析的上游响应。
// 返回：usage 映射，或 nil。
// 调用：serveBypassFollow。测试：逻辑测试第一次查询带 completion_tokens，创建响应没有 usage。
func usageFromOfficial(doc map[string]any) map[string]any {
	usage, _ := doc["usage"].(map[string]any)
	if len(usage) == 0 {
		return nil
	}
	return usage
}
