package dataplane

import (
	"encoding/json"
	"github.com/sunqirui1987/xhub/internal/provider"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sunqirui1987/xhub/internal/auth"
	"github.com/sunqirui1987/xhub/internal/catalog"
	"github.com/sunqirui1987/xhub/internal/config"
)

// TestFalForwardingQueueIsolationAndSettlement 验证 Fal 创建、网关查询地址改写、任务身份隔离和成功结果唯一结算。
// 参数 t：Go 测试上下文。返回：无；断言失败时报告协议或计费契约回归。
// 调用：go test；使用本地注册表、价格目录或假上游，不创建真实付费任务。
func TestFalForwardingQueueIsolationAndSettlement(t *testing.T) {
	const model = "fal-ai/kling-video/v2.5-turbo/pro/text-to-video"
	const queue = "/queue/fal-ai/kling-video/requests/task-1"
	var path, query, authHeader, body string
	var polls int
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path, query, authHeader = r.URL.Path, r.URL.RawQuery, r.Header.Get("Authorization")
		raw, _ := io.ReadAll(r.Body)
		body = string(raw)
		if r.Method == http.MethodPost {
			io.WriteString(w, "{\"request_id\":\"task-1\",\"status\":\"IN_QUEUE\",\"response_url\":\"https://supplier.example/leave-gateway\",\"status_url\":\"https://supplier.example/status\",\"cancel_url\":\"https://supplier.example/cancel\"}")
			return
		}
		polls++
		if polls == 1 {
			w.WriteHeader(202)
			io.WriteString(w, "{\"status\":\"IN_PROGRESS\",\"result\":{\"video\":{\"url\":\"https://example.org/video.mp4\",\"duration\":5.5}}}")
			return
		}
		if polls == 2 {
			io.WriteString(w, "{\"status\":\"COMPLETED\",\"detail\":{\"message\":\"failed\"},\"result\":{\"video\":{\"url\":\"https://example.org/video.mp4\",\"duration\":5.5}}}")
			return
		}
		if strings.HasSuffix(path, "/status") {
			io.WriteString(w, "{\"status\":\"COMPLETED\",\"result\":{\"video\":{\"url\":\"https://example.org/video.mp4\",\"duration\":5.5}}}")
			return
		}
		io.WriteString(w, "{\"video\":{\"url\":\"https://example.org/video.mp4\",\"duration\":5.5}}")
	}))
	defer up.Close()
	dep := deployment("kling", "qiniu/"+model, "supplier-key", up.URL, "qiniu_fal_kling", nil)
	h := &logicHost{cfg: &config.Config{}, client: up.Client(), models: []config.ModelEntry{dep}, pins: map[string]string{}}
	create := h.call(t, "POST", "/queue/"+model+"?fal_webhook=https%3A%2F%2Fexample.org%2Fhook", "{\"model\":\"kling\",\"prompt\":\"apple\",\"duration\":\"10\"}")
	if create.Code != 200 || authHeader != "Key supplier-key" || path != "/queue/"+model || !strings.Contains(query, "fal_webhook=") {
		t.Fatalf("forwarding %d %s %s %s", create.Code, authHeader, path, query)
	}
	var doc map[string]any
	json.Unmarshal([]byte(body), &doc)
	if doc["model"] != nil || doc["prompt"] != "apple" || doc["duration"] != "10" {
		t.Fatal("Fal JSON changed", doc)
	}
	json.Unmarshal(create.Body.Bytes(), &doc)
	if doc["response_url"] != queue || doc["status_url"] != queue+"/status" || doc["cancel_url"] != "" {
		t.Fatal("unsafe queue links", doc)
	}
	ctx := h.OfficialContext(officialTaskScope(&auth.Principal{UserID: "test-user"}, "qiniu_fal_kling", "task-1"))
	if ctx.Model != model || ctx.Variant != "pro_norefv_v_duration" || ctx.StartedAt.IsZero() {
		t.Fatal(ctx)
	}
	for i := 0; i < 2; i++ {
		h.call(t, "GET", queue+"/status", "")
		if h.spend[len(h.spend)-1].usage != nil || h.notes[len(h.notes)-1].SettlementID != "" {
			t.Fatal("pending/failure billed")
		}
	}
	for _, p := range []string{queue + "/status", queue, queue} {
		h.call(t, "GET", p, "")
	}
	settlement := h.notes[len(h.notes)-1].SettlementID
	if settlement == "" {
		t.Fatal("no settlement")
	}
	for i := len(h.spend) - 3; i < len(h.spend); i++ {
		u := catalog.NormalizeUsage(h.spend[i].usage)
		if u.Seconds != 5.5 || u.OutputVariant != "pro_norefv_v_duration" || u.PricingModel != "qiniu/"+model || h.notes[i].SettlementID != settlement {
			t.Fatal(u, h.notes[i])
		}
	}
	before := polls
	wrong := h.call(t, "GET", "/queue/fal-ai/vidu/requests/task-1", "")
	if wrong.Code != 404 || polls != before {
		t.Fatal("cross-family task escaped pin")
	}
	// The same task ID with another identity has no pin.
	hitScope := officialTaskScope(&auth.Principal{UserID: "other"}, "qiniu_fal_kling", "task-1")
	if h.OfficialDeployment(hitScope) != "" {
		t.Fatal("cross-caller pin")
	}
	// Standard Fal has no body model and routes by its registered endpoint ID.
	h.models[0].ModelName = "qiniu/" + model
	if r := h.call(t, "POST", "/queue/"+model, "{\"prompt\":\"apple\"}"); r.Code != 200 {
		t.Fatal(r.Code, r.Body.String())
	}
	// The editor defaults to a public name without the supplier prefix.
	h.models[0].ModelName = model
	if r := h.call(t, "POST", "/queue/"+model, "{\"prompt\":\"apple\"}"); r.Code != 200 {
		t.Fatal("public path alias failed", r.Code, r.Body.String())
	}
	h.models[0].ModelName = "kling"
	second := h.models[0]
	second.ModelName = "another-kling"
	h.models = append(h.models, second)
	if r := h.call(t, "POST", "/queue/"+model, "{\"prompt\":\"apple\"}"); r.Code != 400 {
		t.Fatal("ambiguous alias guessed", r.Code)
	}
	h.models = h.models[:1]
	// An alias bound to a different endpoint must not change the requested path model.
	h.models[0].ModelName = "kling"
	h.models[0].LiteLLMParams["model"] = "qiniu/fal-ai/kling-video/v2.5-turbo/standard/image-to-video"
	if r := h.call(t, "POST", "/queue/"+model, "{\"model\":\"kling\",\"prompt\":\"apple\"}"); r.Code != 400 {
		t.Fatal("alias changed endpoint", r.Code)
	}
}

// TestFalAuthorizationDoesNotFollowRedirects 验证原生请求不会跟随重定向而把供应商密钥转发到其他目的地。
// 参数 t：Go 测试上下文。返回：无；断言失败时报告协议或计费契约回归。
// 调用：go test；使用本地注册表、价格目录或假上游，不创建真实付费任务。
func TestFalAuthorizationDoesNotFollowRedirects(t *testing.T) {
	reached := false
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { reached = true }))
	defer destination.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, destination.URL, 307) }))
	defer source.Close()
	request := httptest.NewRequest("GET", "/queue/", nil)
	response, err := forwardOfficial(officialHost(source), request, "GET", source.URL, "secret", nil, request.Header, provider.Transport{Auth: provider.AuthConfig{Header: "Authorization", Prefix: "Key"}}, nil)
	status := response.StatusCode
	if err != nil || status != 307 || reached {
		t.Fatal("followed Fal redirect", status, err, reached)
	}
}

// TestFalCreateDoesNotReplayAmbiguousServerFailure 验证创建任务的未知服务端失败不会切换部署或重放请求。
// 参数 t：Go 测试上下文。返回：无；断言失败时报告协议或计费契约回归。
// 调用：go test；使用本地注册表、价格目录或假上游，不创建真实付费任务。
func TestFalCreateDoesNotReplayAmbiguousServerFailure(t *testing.T) {
	const model = "fal-ai/kling-video/v2.5-turbo/pro/text-to-video"
	const response = `{"request_id":"possibly-created","error":"response failed after acceptance"}`
	calls := 0
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, response)
	}))
	defer up.Close()
	first := deployment("kling", "qiniu/"+model, "first-key", up.URL, "qiniu_fal_kling", nil)
	second := deployment("kling", "qiniu/"+model, "second-key", up.URL, "qiniu_fal_kling", nil)
	h := &logicHost{
		cfg: &config.Config{}, client: up.Client(), models: []config.ModelEntry{first, second},
		pins: map[string]string{}, settings: map[string]any{"num_retries": 3.},
	}
	r := h.call(t, "POST", "/queue/"+model, `{"prompt":"apple"}`)
	if calls != 1 || r.Code != http.StatusInternalServerError || r.Body.String() != response {
		t.Fatalf("ambiguous creation replayed or hidden: calls=%d status=%d body=%s", calls, r.Code, r.Body.String())
	}
	if len(h.pins) != 0 || len(h.facts) != 0 {
		t.Fatal("failed response pinned a task")
	}
}

// TestFalCreateBusinessFailureDoesNotPinTask 验证 HTTP 成功但业务失败的响应不会登记可查询任务。
// 参数 t：Go 测试上下文。返回：无；断言失败时报告协议或计费契约回归。
// 调用：go test；使用本地注册表、价格目录或假上游，不创建真实付费任务。
func TestFalCreateBusinessFailureDoesNotPinTask(t *testing.T) {
	const model = "fal-ai/kling-video/v2.5-turbo/pro/text-to-video"
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"request_id":"failed-task","detail":{"message":"generation rejected"}}`)
	}))
	defer up.Close()
	dep := deployment("kling", "qiniu/"+model, "supplier-key", up.URL, "qiniu_fal_kling", nil)
	h := &logicHost{cfg: &config.Config{}, client: up.Client(), models: []config.ModelEntry{dep}, pins: map[string]string{}}
	r := h.call(t, "POST", "/queue/"+model, `{"prompt":"apple"}`)
	if r.Code != http.StatusOK || len(h.pins) != 0 || len(h.facts) != 0 {
		t.Fatal("business failure pinned a task", r.Code, h.pins, h.facts)
	}
	r = h.call(t, "GET", "/queue/fal-ai/kling-video/requests/failed-task", "")
	if r.Code != http.StatusNotFound {
		t.Fatal("failed task remained queryable", r.Code)
	}
}
