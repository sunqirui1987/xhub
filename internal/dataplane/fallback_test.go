package dataplane

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/sunqirui1987/xhub/internal/auth"
	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/gateway/prefs"
	"github.com/sunqirui1987/xhub/internal/provider"
	"github.com/sunqirui1987/xhub/internal/router"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestFallbackClassification 验证限流/服务错误与专用错误分类；成功、普通4xx和非法JSON不触发，无外部状态。
func TestFallbackClassification(t *testing.T) {
	for _, tc := range []struct {
		status     int
		body, want string
	}{
		{500, "", "general"}, {429, "", "general"}, {400, "invalid-json", ""},
		{400, "{}", ""}, {200, "{\"error\":\"context_length_exceeded\"}", ""},
		{400, "{\"error\":{\"code\":\"context_length_exceeded\"}}", "context"},
		{400, "{\"error\":{\"message\":\"maximum context length exceeded\"}}", "context"},
		{403, "{\"error\":{\"code\":\"content_policy_violation\"}}", "content"},
		{401, "{\"error\":{\"code\":\"invalid_api_key\"}}", ""},
	} {
		if got := fallbackKind(tc.status, []byte(tc.body)); got != tc.want {
			t.Fatalf("状态%d 正文%s: %q != %q", tc.status, tc.body, got, tc.want)
		}
	}
}

// TestFallbackQueue 验证有序目标、去重、空池跳过、禁用和错误传播；前置内存路由，断言无共享配置修改，无需清理。
func TestFallbackQueue(t *testing.T) {
	settings := prefs.BuiltinSettings()
	settings.ModelFallbacks = map[string]router.FallbackPolicy{"a": {Fallbacks: []string{"missing", "b", "c"}}, "b": {Fallbacks: []string{"c", "a"}}}
	models := []config.ModelEntry{{ModelName: "b", LiteLLMParams: map[string]any{"deployment_id": "b"}}, {ModelName: "c", LiteLLMParams: map[string]any{"deployment_id": "c"}}}
	q := newFallbackQueue(settings, "a", false)
	for _, want := range []string{"b", "c", ""} {
		pool, _ := q.next("general", models, router.State{})
		if want == "" {
			if len(pool) != 0 {
				t.Fatal("重复模型被再次调用")
			}
		} else if len(pool) != 1 || pool[0].ModelName != want {
			t.Fatalf("回退顺序 %s: %v", want, pool)
		}
	}
	if pool, _ := newFallbackQueue(settings, "a", true).next("general", models, router.State{}); len(pool) != 0 {
		t.Fatal("禁用回退未生效")
	}
	if pool, _ := newFallbackQueue(settings, "a", false).next("content", models, router.State{}); len(pool) != 0 {
		t.Fatal("专用类别误用通用策略")
	}
	settings.ModelDefaults = map[string]any{"b": map[string]any{"allocations": "bad"}}
	_, result := newFallbackQueue(settings, "a", false).next("general", models, router.State{})
	if result.Err == nil {
		t.Fatal("目标权重错误未传播")
	}
}

// TestFallbackNativePayload 验证网关开关剥离及大整数原样转发；前置原生 JSON，检查未知字段和模型替换，无资源清理。
func TestFallbackNativePayload(t *testing.T) {
	b, err := parseBypassBody([]byte("{\"model\":\"a\",\"disable_fallbacks\":true,\"value\":9007199254740993}"), "application/json")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := b.Payload("model", "b", false)
	if err != nil || strings.Contains(string(raw), "disable_fallbacks") || !strings.Contains(string(raw), "9007199254740993") {
		t.Fatalf("原生正文损坏: %s %v", raw, err)
	}
	raw, err = b.Payload("", "unused", false)
	var fields map[string]json.RawMessage
	if err != nil || json.Unmarshal(raw, &fields) != nil || fields[""] != nil || fields["disable_fallbacks"] != nil || string(fields["model"]) != `"a"` {
		t.Fatalf("无模型字段时只应剥离开关: %s %v", raw, err)
	}
}

// TestFallbackPayloadMultipart 验证无模型字段的文件请求只剥离回退开关，保留文件字节与名称。
// 参数 t 为测试上下文，无返回；前置内存 multipart，不访问上游，不产生需清理的数据。
func TestFallbackPayloadMultipart(t *testing.T) {
	var raw bytes.Buffer
	writer := multipart.NewWriter(&raw)
	writer.WriteField("disable_fallbacks", "true")
	file, err := writer.CreateFormFile("file", "data.bin")
	if err != nil {
		t.Fatal(err)
	}
	data := []byte{0, 255, 13, 10}
	file.Write(data)
	writer.Close()
	body, err := parseBypassBody(raw.Bytes(), writer.FormDataContentType())
	if err != nil {
		t.Fatal(err)
	}
	payload, err := body.Payload("", "unused", false)
	if err != nil {
		t.Fatal(err)
	}
	reader := multipart.NewReader(bytes.NewReader(payload), writer.Boundary())
	part, err := reader.NextPart()
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(part)
	if err != nil || part.FormName() != "file" || part.FileName() != "data.bin" || !bytes.Equal(got, data) {
		t.Fatalf("原生文件损坏: 字段%s 文件%s 正文%v 错误%v", part.FormName(), part.FileName(), got, err)
	}
	if _, err := reader.NextPart(); err != io.EOF {
		t.Fatalf("回退开关未删除: %v", err)
	}
}

// TestFallbackQueueBoundAndCooldown 验证冷却目标跳过和请求最多访问32个模型的边界。
// 参数 t 为测试上下文，无返回；前置内存模型目录和冷却快照，验证链不可无限放大，无持久数据。
func TestFallbackQueueBoundAndCooldown(t *testing.T) {
	settings := prefs.BuiltinSettings()
	settings.ModelFallbacks = map[string]router.FallbackPolicy{}
	models := []config.ModelEntry{}
	for i := 0; i < 40; i++ {
		name := fmt.Sprint(i)
		settings.ModelFallbacks[name] = router.FallbackPolicy{Fallbacks: []string{fmt.Sprint(i + 1)}}
		models = append(models, config.ModelEntry{ModelName: name, LiteLLMParams: map[string]any{"model": name, "deployment_id": name}})
	}
	q := newFallbackQueue(settings, "0", false)
	state := router.State{Cooldown: map[string]bool{router.CooldownID(models[1]): true}}
	count := 0
	for {
		pool, _ := q.next("general", models, state)
		if len(pool) == 0 {
			break
		}
		if pool[0].ModelName == "1" {
			t.Fatal("冷却目标未跳过")
		}
		count++
	}
	if count != 30 || len(q.seen) != 32 {
		t.Fatalf("回退边界错误: 调用%d 已访问%d", count, len(q.seen))
	}
}

// fallbackNativeHost 为原生执行测试提供模型策略；内嵌宿主记录转发与计费，无共享配置修改。
type fallbackNativeHost struct {
	*logicHost
	policy prefs.RouteSettings
}

// RouteSettingsFor 返回原生执行测试指定的配置；参数为鉴权主体，调用方ServeBypass，无持久化副作用。
func (h *fallbackNativeHost) RouteSettingsFor(*auth.Principal) prefs.RouteSettings { return h.policy }

// TestNativeDedicatedFallbackChain 验证仅配置专用列表时，第一目标5xx后继续剩余目标或目标自身的通用链。
// 参数 t 为测试上下文，无返回；本地HTTP模拟上下文和内容错误，验证普通4xx终止和异步禁用，Cleanup关闭服务。
func TestNativeDedicatedFallbackChain(t *testing.T) {
	for _, tc := range []struct {
		name, code         string
		child, background  bool
		middleStatus, want int
		models             string
	}{
		{"context-pending", "context_length_exceeded", false, false, 500, 200, "a,b,c"},
		{"content-child", "content_policy_violation", true, false, 500, 200, "a,b,c"},
		{"ordinary-terminal", "context_length_exceeded", false, false, 400, 400, "a,b"},
		{"background-disabled", "context_length_exceeded", false, true, 500, 400, "a"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := []string{}
			up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var doc map[string]any
				json.NewDecoder(r.Body).Decode(&doc)
				model, _ := doc["model"].(string)
				calls = append(calls, model)
				w.Header().Set("Content-Type", "application/json")
				switch model {
				case "a":
					w.WriteHeader(400)
					fmt.Fprintf(w, "{\"error\":{\"code\":%q}}", tc.code)
				case "b":
					w.WriteHeader(tc.middleStatus)
					io.WriteString(w, "{\"error\":{\"code\":\"api_error\"}}")
				default:
					io.WriteString(w, "{\"id\":\"ok\",\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}")
				}
			}))
			t.Cleanup(up.Close)
			settings := prefs.BuiltinSettings()
			targets := []string{"b", "c"}
			if tc.child {
				targets = targets[:1]
			}
			p := router.FallbackPolicy{ContextWindow: targets}
			if tc.code == "content_policy_violation" {
				p = router.FallbackPolicy{ContentPolicy: targets}
			}
			settings.ModelFallbacks = map[string]router.FallbackPolicy{"a": p, "b": {Fallbacks: []string{"c"}}}
			h := &fallbackNativeHost{logicHost: officialHost(up, deployment("a", "a", "key", up.URL, "bypass_openai_responses", nil), deployment("b", "b", "key", up.URL, "bypass_openai_responses", nil), deployment("c", "c", "key", up.URL, "bypass_openai_responses", nil)), policy: settings}
			req := httptest.NewRequest("POST", "/bypass/openai/v1/responses", strings.NewReader(fmt.Sprintf("{\"model\":\"a\",\"input\":\"hi\",\"background\":%t}", tc.background)))
			hit, ok := provider.Match("POST", req.URL.Path, h.models)
			if !ok {
				t.Fatal("原生入口未匹配")
			}
			w := httptest.NewRecorder()
			ServeBypass(h, w, req, hit)
			if w.Code != tc.want || strings.Join(calls, ",") != tc.models {
				t.Fatalf("状态%d 调用%v 正文%s", w.Code, calls, w.Body.String())
			}
		})
	}
}

// fallbackServeHost 为执行单测提供请求私有策略和可观察结果；内存配置不修改共享宿主。
type fallbackServeHost struct {
	*logHost
	settings prefs.RouteSettings
	deny     string
	required bool
}

// RouteSettingsFor 返回单测指定策略；参数为已鉴权主体，无存储或副作用。
func (h *fallbackServeHost) RouteSettingsFor(*auth.Principal) prefs.RouteSettings { return h.settings }

// WriteChatJSON 记录真实HTTP响应；参数与执行宿主一致，返回无，写入测试响应而不结算。
func (h *fallbackServeHost) WriteChatJSON(w http.ResponseWriter, _ *auth.Principal, _, _, _, _, _ string, body []byte, status int, _ time.Time, _ string) {
	w.WriteHeader(status)
	w.Write(body)
}

// EnforceIdentityLimits 模拟单个目标被拒绝；参数为目标模型，返回许可并在拒绝时写403，无外部权限数据。
func (h *fallbackServeHost) EnforceIdentityLimits(w http.ResponseWriter, _ string, _ *auth.Principal, model string, _ int) bool {
	if model == h.deny {
		w.WriteHeader(403)
		return false
	}
	return true
}

// PlanRoute 返回单测固定部署约束；参数为请求及身份，返回固定计划，无外部会话写入。
func (h *fallbackServeHost) PlanRoute(*http.Request, string, map[string]any, *auth.Principal) RoutePlan {
	if h.required {
		return RoutePlan{Required: true, Pinned: router.CooldownID(h.cfg.ModelList[0])}
	}
	return RoutePlan{}
}

// TestServeModelFallback 验证实际HTTP执行顺序、禁用开关、继续请求和权限拒绝。
// 前置本地上游和内存路由，所有错误与成功均走真实HTTP，服务由Cleanup关闭。
func TestServeModelFallback(t *testing.T) {
	for _, tc := range []struct {
		name, denied      string
		disable, required bool
		want              int
		calls             int
	}{{"enabled", "", false, false, 200, 2}, {"disabled", "", true, false, 502, 1}, {"continuation", "", false, true, 502, 1}, {"denied", "b", false, false, 403, 1}} {
		t.Run(tc.name, func(t *testing.T) {
			calls := []string{}
			up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var b map[string]any
				json.NewDecoder(r.Body).Decode(&b)
				model, _ := b["model"].(string)
				calls = append(calls, model)
				if b["disable_fallbacks"] != nil {
					t.Error("开关发送给了上游")
				}
				if model == "a" {
					w.WriteHeader(500)
					io.WriteString(w, "{}")
					return
				}
				io.WriteString(w, "{\"id\":\"ok\",\"choices\":[{\"message\":{\"role\":\"assistant\",\"content\":\"ok\"},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":1,\"completion_tokens\":1}}")
			}))
			t.Cleanup(up.Close)
			cfg := chatConfig(config.ModelEntry{ModelName: "a", LiteLLMParams: map[string]any{"model": "a", "api_base": up.URL, "api_key": "sk-local", "deployment_id": "a"}}, config.ModelEntry{ModelName: "b", LiteLLMParams: map[string]any{"model": "b", "api_base": up.URL, "api_key": "sk-local", "deployment_id": "b"}})
			settings := prefs.BuiltinSettings()
			settings.ModelFallbacks = map[string]router.FallbackPolicy{"a": {Fallbacks: []string{"b"}}}
			h := &fallbackServeHost{logHost: newLogHost(cfg, up.Client()), settings: settings, deny: tc.denied, required: tc.required}
			req := chatRequest(t, "a", false)
			if tc.disable {
				req.Body = io.NopCloser(strings.NewReader("{\"model\":\"a\",\"messages\":[{\"role\":\"user\",\"content\":\"hi\"}],\"disable_fallbacks\":true}"))
			}
			w := httptest.NewRecorder()
			Serve(h, w, req, "chat")
			if w.Code != tc.want || len(calls) != tc.calls {
				t.Fatalf("%s: 状态%d 调用%v 正文%s", tc.name, w.Code, calls, w.Body.String())
			}
		})
	}
}

// TestFallbackStreamAlreadyWritten 验证配置回退后流式中断仍不切换；前置分段错误流，断言只发送一次。
// 使用内存RoundTripper，响应和缓存随测试结束释放，不依赖外部供应商。
func TestFallbackStreamAlreadyWritten(t *testing.T) {
	attempts := 0
	client := &http.Client{Transport: auditRoundTripper(func(*http.Request) (*http.Response, error) {
		attempts++
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(&fragmentedErrorReader{parts: [][]byte{[]byte("data: {\"choices\":[{\"delta\":{\"content\":\"partial\"}}]}\n\n")}, err: errors.New("reset")})}, nil
	})}
	cfg := chatConfig(config.ModelEntry{ModelName: "a", LiteLLMParams: map[string]any{"model": "a", "api_base": "https://example.invalid", "api_key": "local"}}, config.ModelEntry{ModelName: "b", LiteLLMParams: map[string]any{"model": "b", "api_base": "https://example.invalid", "api_key": "local"}})
	settings := prefs.BuiltinSettings()
	settings.ModelFallbacks = map[string]router.FallbackPolicy{"a": {Fallbacks: []string{"b"}}}
	h := &fallbackServeHost{logHost: newLogHost(cfg, client), settings: settings}
	w := httptest.NewRecorder()
	Serve(h, w, chatRequest(t, "a", true), "chat")
	if attempts != 1 || !strings.Contains(w.Body.String(), "partial") {
		t.Fatalf("输出后重试: 次数%d 正文%s", attempts, w.Body.String())
	}
}

// TestNativeGroupMemberFallback 验证组名原生调用使用实际成员的回退链；参数t为测试上下文，无返回。
// 前置本地上游及单成员组；覆盖成功回退、显式禁用、异步禁用和目标错误，Cleanup关闭HTTP服务。
func TestNativeGroupMemberFallback(t *testing.T) {
	for _, tc := range []struct {
		name                 string
		disabled, background bool
		targetStatus, want   int
		calls                string
	}{
		{"member-chain", false, false, 200, 200, "a,b"},
		{"disabled", true, false, 200, 500, "a"},
		{"background", false, true, 200, 500, "a"},
		{"target-error", false, false, 400, 400, "a,b"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := []string{}
			up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body map[string]any
				json.NewDecoder(r.Body).Decode(&body)
				model, _ := body["model"].(string)
				calls = append(calls, model)
				w.Header().Set("Content-Type", "application/json")
				status := tc.targetStatus
				if model == "a" {
					status = 500
				}
				w.WriteHeader(status)
				if status >= 400 {
					io.WriteString(w, "{\"error\":{\"code\":\"api_error\"}}")
				} else {
					io.WriteString(w, "{\"id\":\"ok\",\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}")
				}
			}))
			t.Cleanup(up.Close)
			settings := prefs.BuiltinSettings()
			settings.RoutingGroups = []router.Group{{Name: "group", Models: []string{"a"}, Strategy: "simple-shuffle"}}
			settings.ModelFallbacks = map[string]router.FallbackPolicy{"a": {Fallbacks: []string{"b"}}}
			h := &fallbackNativeHost{logicHost: officialHost(up,
				deployment("a", "a", "key", up.URL, "bypass_openai_responses", nil),
				deployment("b", "b", "key", up.URL, "bypass_openai_responses", nil)), policy: settings}
			req := httptest.NewRequest("POST", "/bypass/openai/v1/responses", strings.NewReader(fmt.Sprintf("{\"model\":\"group\",\"input\":\"hi\",\"background\":%t,\"disable_fallbacks\":%t}", tc.background, tc.disabled)))
			hit, ok := provider.Match("POST", req.URL.Path, h.models)
			if !ok {
				t.Fatal("原生入口未匹配")
			}
			w := httptest.NewRecorder()
			ServeBypass(h, w, req, hit)
			if w.Code != tc.want || strings.Join(calls, ",") != tc.calls {
				t.Fatalf("组成员回退: 状态%d 调用%v 正文%s", w.Code, calls, w.Body.String())
			}
		})
	}
}

// TestFallbackGroupSource 验证组链优先、显式禁用和每次实际成员更新；仅内存队列，测试后自动释放。
func TestFallbackGroupSource(t *testing.T) {
	settings := prefs.BuiltinSettings()
	settings.ModelFallbacks = map[string]router.FallbackPolicy{"a": {Fallbacks: []string{"backup-a"}}, "b": {ContextWindow: []string{"backup-b"}}}
	q := newFallbackQueue(settings, "group", false)
	q.useDeployment("a")
	q.useDeployment("b")
	if q.current != "b" {
		t.Fatalf("成员源粘住: %s", q.current)
	}
	settings.ModelFallbacks["group"] = router.FallbackPolicy{}
	q = newFallbackQueue(settings, "group", false)
	q.useDeployment("b")
	if q.current != "group" {
		t.Fatal("显式组禁用没有优先")
	}
	settings.ModelFallbacks["group"] = router.FallbackPolicy{ContextWindow: []string{"backup-b"}}
	if !hasFallbackTargets(settings.ModelFallbacks["group"]) || hasFallbackTargets(router.FallbackPolicy{}) {
		t.Fatal("专用错误缓冲判定错误")
	}
}
