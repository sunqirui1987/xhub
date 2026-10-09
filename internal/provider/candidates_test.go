package provider_test

import (
	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/llm"
	"github.com/sunqirui1987/xhub/internal/provider"
	"testing"
)

// TestCandidatesUseDeclaredProtocols 验证任意供应商按协议筛选，真实型号保持原样。
// 参数 t 为测试上下文；覆盖正常、严格工具不兼容、禁用、缺配置和未开放入口；纯内存，无清理副作用。
func TestCandidatesUseDeclaredProtocols(t *testing.T) {
	protocols := []string{"bypass_openai_chat", "bypass_openai_responses", "bypass_anthropic_messages"}
	var deps []config.ModelEntry
	for i, profile := range protocols {
		deps = append(deps, config.ModelEntry{ModelName: "shared", LiteLLMParams: map[string]any{"custom_llm_provider": "arbitrary-connection", "model": "org/real/model", "api_base": "https://arbitrary.invalid", "deployment_id": profile}, ModelInfo: map[string]any{"transport": profile, "endpoint_types": []string{"chat", "responses", "messages"}, "index": i}})
	}
	dialogue := &llm.Dialogue{Turns: []llm.Turn{{Role: "user", Text: "hello"}}}
	pool, decisions := provider.Candidates(deps, "chat", dialogue)
	if len(pool) != 3 || len(decisions) != 3 {
		t.Fatalf("同协议兼容池错误: %v", decisions)
	}
	for _, dep := range pool {
		if dep.ParamString("model", "") != "org/real/model" {
			t.Fatal("真实型号被改写")
		}
	}
	dialogue.Tools = []map[string]any{{"name": "f", "parameters": map[string]any{"type": "object"}, "strict": true}}
	pool, decisions = provider.Candidates(deps, "chat", dialogue)
	if len(pool) != 2 || decisions[2].Reason == "" {
		t.Fatalf("严格工具不兼容应明确排除: %+v", decisions)
	}
	for _, mutate := range []func(*config.ModelEntry){
		func(d *config.ModelEntry) { d.ModelInfo["transport"] = "missing" },
		func(d *config.ModelEntry) { d.ModelInfo["endpoint_types"] = []string{"responses"} },
	} {
		dep := deps[0]
		dep.ModelInfo = map[string]any{"transport": protocols[0], "endpoint_types": []string{"chat"}}
		mutate(&dep)
		pool, decisions = provider.Candidates([]config.ModelEntry{dep}, "chat", nil)
		if len(pool) != 0 || decisions[0].Reason == "" {
			t.Fatalf("无效配置未排除: %+v", decisions)
		}
	}
}

// TestDialogueBindingsUseEndpointDirectory 验证统一和原生入口共用目录而不按供应商分支。
// 参数 t 为测试上下文；兼容协议开放多个入口，原生不兼容协议被拒绝；纯注册表读取，无外部数据。
func TestDialogueBindingsUseEndpointDirectory(t *testing.T) {
	dep := config.ModelEntry{ModelName: "shared", LiteLLMParams: map[string]any{"model": "org/model", "custom_llm_provider": "vendor-10000"}, ModelInfo: map[string]any{"transport": "bypass_openai_responses", "endpoint_types": []string{"chat", "responses", "messages", "bypass:openai-responses"}}}
	bindings := provider.DeploymentEndpoints(dep)
	if len(bindings) != 4 {
		t.Fatalf("入口目录投影错误: %+v", bindings)
	}
	for _, binding := range bindings {
		if binding.Path == "" || binding.Transport != "bypass_openai_responses" {
			t.Fatalf("入口缺少路径或执行配置: %+v", binding)
		}
	}
	if provider.AllowsEndpoint(dep, "bypass:openai-chat") == nil {
		t.Fatal("不同原生协议不能互通")
	}
}
