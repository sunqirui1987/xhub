package dataplane

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sunqirui1987/xhub/internal/auth"
	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/llm"
	"github.com/sunqirui1987/xhub/internal/provider"
	"github.com/sunqirui1987/xhub/internal/router"
)

// diagnosticDeployment 构造隔离部署；参数为公开名、部署 ID、传输和暂停标志，返回独立配置。
// 单元测试调用，不访问外部服务；地址故意含凭据和查询参数以检验日志投影。
func diagnosticDeployment(name, id, transport string, disabled bool) config.ModelEntry {
	return config.ModelEntry{ModelName: name, LiteLLMParams: map[string]any{
		"model": "real", "api_key": "arbitrary-private-secret", "api_base": "https://user:private-password@upstream.example/v1?token=private-query",
		"custom_llm_provider": "custom",
	}, ModelInfo: map[string]any{"id": id, "transport": transport, "disabled": disabled}}
}

// TestRouteDiagnosisStages 验证正常、空目录、无效声明、暂停、冷却、零权重及组/通配符边界。
// 前置独立内存目录；验证分类、计数及无共享配置改写；日志恢复由 captureLog 清理，无外部数据。
func TestRouteDiagnosisStages(t *testing.T) {
	good := diagnosticDeployment("gpt-5.6-sol", "live", "bypass_openai_chat", false)
	bad := diagnosticDeployment("gpt-5.6-sol", "bad", "missing-transport", false)
	paused := diagnosticDeployment("gpt-5.6-sol", "paused", "bypass_openai_chat", true)
	other := diagnosticDeployment("other", "other", "bypass_openai_chat", true)
	wildcard := diagnosticDeployment("gpt-*", "wild", "bypass_openai_chat", false)
	settings := router.BuiltinSettings().ForModel("gpt-5.6-sol")
	for _, tc := range []struct {
		name                                   string
		list                                   []config.ModelEntry
		state                                  router.State
		group                                  bool
		matched, compatible, available, status int
		kind, message                          string
	}{
		{name: "healthy", list: []config.ModelEntry{good, bad, other}, matched: 2, compatible: 1, available: 1},
		{name: "empty", status: 400, kind: "invalid_request", message: "model not found"},
		{name: "unrelated disabled", list: []config.ModelEntry{other}, status: 400, kind: "invalid_request", message: "model not found"},
		{name: "matching disabled", list: []config.ModelEntry{paused}, status: 400, kind: "model_disabled", message: "model is disabled"},
		{name: "invalid transport", list: []config.ModelEntry{bad}, matched: 1, status: 400, kind: "model_unavailable", message: "exists"},
		{name: "cooldown", list: []config.ModelEntry{good}, state: router.State{Cooldown: map[string]bool{router.CooldownID(good): true}}, matched: 1, compatible: 1, status: 503, kind: "model_unavailable", message: "cooling down"},
		{name: "zero weight stale positive", list: []config.ModelEntry{good}, state: router.State{Allocations: map[string]float64{"live": 0, "deleted": 100}}, matched: 1, compatible: 1, status: 503, kind: "model_unavailable", message: "no allocated traffic"},
		{name: "wildcard", list: []config.ModelEntry{wildcard}, matched: 1, compatible: 1, available: 1},
		{name: "group", list: []config.ModelEntry{good}, group: true, matched: 1, compatible: 1, available: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := settings
			alias := "gpt-5.6-sol"
			if tc.group {
				alias = "group"
				s.RoutingGroups = []router.Group{{Name: alias, Models: []string{good.ModelName}}}
			}
			if tc.state.Allocations != nil {
				s.Policy.Allocations = []router.Allocation{{DeploymentID: "live", Weight: 0}, {DeploymentID: "deleted", Weight: 100}}
			}
			active, _ := dropDisabled(tc.list)
			eligible, _ := provider.Candidates(active, "chat", nil)
			d := diagnoseRoute(tc.list, eligible, alias, "chat", s, tc.state, nil, false)
			if d.matched != tc.matched || d.compatible != tc.compatible || d.available != tc.available {
				t.Fatalf("阶段计数错误: %+v", d)
			}
			if tc.status != 0 {
				status, kind, message := d.failure(alias)
				if status != tc.status || kind != tc.kind || !strings.Contains(message, tc.message) {
					t.Fatalf("空路由分类错误: %d %s %s", status, kind, message)
				}
			}
			if tc.name == "zero weight stale positive" && len(d.stale) != 1 {
				t.Fatalf("没有识别过期部署权重: %+v", d.stale)
			}
			if !paused.Disabled() || wildcard.ParamString("model", "") != "real" {
				t.Fatal("诊断修改了原目录")
			}
		})
	}
}

// diagnosticHost 用独立设置与运行状态驱动真实 Serve，复用无持久化的单元测试宿主。
type diagnosticHost struct {
	*logHost
	settings router.RouteSettings
	state    router.State
}

// RouteSettingsFor 返回测试的请求规则快照；参数身份由 Serve 提供，返回设置，不写数据。
func (h *diagnosticHost) RouteSettingsFor(*auth.Principal) router.RouteSettings { return h.settings }

// RouteState 返回测试冷却状态；Serve 调用，无参数，返回只读快照，无外部副作用。
func (h *diagnosticHost) RouteState() router.State { return h.state }

// TestServeRouteDiagnosisLogs 验证用户复现的零权重/过期 ID 场景，接口返回503并关联安全日志。
// 前置独立配置和内存日志；同时覆盖不兼容请求/无效协议失败，恢复日志输出，无真实凭据及上游调用。
func TestServeRouteDiagnosisLogs(t *testing.T) {
	for _, mode := range []string{"zero", "invalid", "capability", "fallback"} {
		t.Run(mode, func(t *testing.T) {
			dep := diagnosticDeployment("gpt-5.6-sol", "live", "bypass_openai_chat", false)
			s := router.BuiltinSettings()
			wantStatus, wantReason := 503, "deployment has no allocated traffic"
			if mode == "zero" {
				s.ModelDefaults = map[string]any{dep.ModelName: map[string]any{"allocations": []any{map[string]any{"deployment_id": "live", "weight": 0}, map[string]any{"deployment_id": "deleted", "weight": 100}}}}
			} else if mode == "invalid" {
				dep.ModelInfo["transport"] = "missing-transport"
				wantStatus, wantReason = 400, "model_info.transport must select a registered transport"
			} else if mode == "capability" {
				dep.ModelInfo["transport"] = "bypass_anthropic_messages"
				wantStatus, wantReason = 400, "strict"
			} else {
				s.ModelFallbacks = map[string]router.FallbackPolicy{dep.ModelName: {Fallbacks: []string{"fallback"}}}
				dep.ModelName = "fallback"
				dep.ModelInfo["transport"] = "missing-transport"
				wantStatus, wantReason = 400, "fallback_visited=1"
			}
			h := &diagnosticHost{logHost: newLogHost(&config.Config{ModelList: []config.ModelEntry{dep}}, nil), settings: s}
			buf := captureLog(t)
			req := chatRequest(t, "gpt-5.6-sol", false)
			if mode == "capability" {
				req = httptest.NewRequest(http.MethodPost, "/chat/completions", strings.NewReader(`{"model":"gpt-5.6-sol","messages":[{"role":"user","content":"hi"}],"tools":[{"type":"function","function":{"name":"f","strict":true,"parameters":{"type":"object"}}}]}`))
			}
			w := httptest.NewRecorder()
			Serve(h, w, req, "chat")
			if w.Code != wantStatus {
				t.Fatalf("%s 分类错误: %d %s", mode, w.Code, w.Body.String())
			}
			for _, fragment := range []string{"step=route_diagnosis", "call_id=\"" + w.Header().Get("x-litellm-call-id") + "\"", wantReason, "source=\"builtin\""} {
				if !strings.Contains(buf.String(), fragment) {
					t.Fatalf("缺失路由解释 %q: %s", fragment, buf.String())
				}
			}
			for _, secret := range []string{"arbitrary-private-secret", "private-password", "private-query", "https://"} {
				if strings.Contains(buf.String(), secret) {
					t.Fatalf("诊断泄漏敏感字段 %q", secret)
				}
			}
		})
	}
}

// TestRouteDiagnosisRequestCompatibility 验证对话能力排除不会冒充模型缺失；前置严格工具请求。
// 参数 t 为测试上下文，结果必须保留排除理由，所有对象均在内存中，无数据清理需求。
func TestRouteDiagnosisRequestCompatibility(t *testing.T) {
	dep := diagnosticDeployment("chat", "anthropic", "bypass_anthropic_messages", false)
	dialogue, err := llm.ParseDialogue("openai-chat", map[string]any{"messages": []any{map[string]any{"role": "user", "content": "hi"}}, "tools": []any{map[string]any{"type": "function", "function": map[string]any{"name": "f", "strict": true, "parameters": map[string]any{"type": "object"}}}}})
	if err != nil {
		t.Fatal(err)
	}
	eligible, _ := provider.Candidates([]config.ModelEntry{dep}, "chat", &dialogue)
	d := diagnoseRoute([]config.ModelEntry{dep}, eligible, "chat", "chat", router.BuiltinSettings().ForModel("chat"), router.State{}, &dialogue, true)
	if d.compatible != 0 || len(d.decisions) != 1 || !strings.Contains(d.decisions[0].Reason, "strict") {
		t.Fatalf("严格工具不兼容未解释: %+v", d)
	}
}
