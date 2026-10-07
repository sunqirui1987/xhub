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
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/sunqirui1987/xhub/internal/auth"
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
	raw, _ := io.ReadAll(r.Body)
	var body map[string]any
	if len(bytes.TrimSpace(raw)) > 0 {
		if json.Unmarshal(raw, &body) != nil {
			httpx.WriteTypedError(w, r.URL.Path, 400, "invalid_request", "invalid json")
			return
		}
	}
	if hit.Action.Name == "create" || hit.Type.ModelField != "" && hit.Action.Name == "" {
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
	field := hit.Type.ModelField
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
	dep, ok := pickDeployment(h, hit, alias)
	if !ok {
		httpx.WriteTypedError(w, r.URL.Path, 400, "invalid_request", "model not found: "+alias)
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
	body[field] = provider.OfficialID(hit.Type.StripPrefix, dep.ParamString("model", alias))
	payload, err := json.Marshal(body)
	if err != nil {
		httpx.WriteTypedError(w, r.URL.Path, 400, "invalid_request", "invalid json")
		return
	}
	url := withQuery(base+provider.Expand(hit.Action.UpstreamPath, nil), r)
	respBody, status, err := forwardOfficial(h, r, http.MethodPost, url, key, payload, r.Header)
	if err != nil {
		logx.Error("bypass forward path=%s err=%s", r.URL.Path, safeErr(err))
		httpx.WriteTypedError(w, r.URL.Path, 502, "upstream_error", "upstream request failed")
		return
	}
	var doc map[string]any
	_ = json.Unmarshal(respBody, &doc)
	depID := router.DeploymentID(dep)
	if taskID := provider.ReadTaskID(doc, hit.Type.TaskID); taskID != "" {
		h.PinOfficial(taskID, depID)
	}
	plan := h.PlanRoute(r, alias, body, principal)
	h.RememberExchange(callID, r, raw, respBody)
	h.AnnotateCall(callID, dataplaneNote(hit, dep, plan.SessionID, depID, time.Since(start)))
	h.RecordSpend(w, principal, callID, alias, hit.Type.ID+":"+hit.Action.Name, nil, start, false, status, depID)
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
	if id != "" {
		pinned := h.OfficialDeployment(id)
		if pinned == "" {
			httpx.WriteTypedError(w, r.URL.Path, 404, "not_found", "unknown task")
			return
		}
		dep, ok = h.FindDeployment(pinned)
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
	respBody, status, err := forwardOfficial(h, r, r.Method, url, key, nil, r.Header)
	if err != nil {
		httpx.WriteTypedError(w, r.URL.Path, 502, "upstream_error", "upstream request failed")
		return
	}
	var doc map[string]any
	_ = json.Unmarshal(respBody, &doc)
	usage := usageFromOfficial(doc)
	if id != "" && usage != nil && !h.OfficialBilled(id) {
		h.MarkOfficialBilled(id)
	} else {
		usage = nil
	}
	depID := router.DeploymentID(dep)
	plan := h.PlanRoute(r, dep.ModelName, nil, principal)
	h.RememberExchange(callID, r, nil, respBody)
	h.AnnotateCall(callID, dataplaneNote(hit, dep, plan.SessionID, depID, time.Since(start)))
	h.RecordSpend(w, principal, callID, dep.ModelName, hit.Type.ID+":"+hit.Action.Name, usage, start, false, status, depID)
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
		name = hit.Type.StripPrefix
	}
	if name == "" {
		name = hit.Type.ID
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
	return provider.Includes(dep, hit.Type.ID)
}

// pickDeployment 在匹配当前端点类型的部署里按路由策略选出一条。没有可选部署时返回假。
// 参数 alias：请求正文模型字段的值，要和部署的 ModelName 一致。
// 返回：选中的部署。没有匹配时 ok 为 false，调用方写 400。
// 调用：serveBypassCreate。测试：逻辑测试按模型名打到对应密钥。
func pickDeployment(h Bypass, hit provider.Hit, alias string) (config.ModelEntry, bool) {
	list := eligible(h, hit)
	pool := router.Order(list, alias, h.GatewayConfig().RouterSettings.RoutingStrategy, h.RouteState())
	pool, paused := dropPaused(pool)
	if len(pool) == 0 {
		_ = paused
		return config.ModelEntry{}, false
	}
	return pool[0], true
}

// eligible 列出模型表里属于当前端点类型、或名字与自定义路径一致的部署。
// 参数 h、hit：读模型表和当前匹配。
// 返回：未再过滤暂停状态的部署。暂停由 pickDeployment 和 oneUpstream 去掉。
// 调用：pickDeployment、oneUpstream。无单独测试。
func eligible(h Bypass, hit provider.Hit) []config.ModelEntry {
	var out []config.ModelEntry
	for _, m := range h.Models() {
		if hit.DeploymentName != "" {
			if m.ModelName == hit.DeploymentName {
				out = append(out, m)
			}
			continue
		}
		if provider.Includes(m, hit.Type.ID) {
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
	list, _ := dropPaused(eligible(h, hit))
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
		base = trimBase(hit.Type.APIBase)
	}
	if base == "" {
		base = provider.APIBase(hit.Type.StripPrefix, "")
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
// Host 和 Content-Length 不转发。其余入站头原样复制。响应最多读 8 MiB。
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
		if strings.EqualFold(k, "Authorization") || strings.EqualFold(k, "Host") || strings.EqualFold(k, "Content-Length") {
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
	resp, err := h.HTTPClient().Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	out, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	return out, resp.StatusCode, err
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
