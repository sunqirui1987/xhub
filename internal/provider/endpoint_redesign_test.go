package provider_test

import (
	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/provider"
	"strings"
	"testing"
)

// TestEndpointRedesignContract 校验三类原生接口及 Bypass 目录、独立图片操作和移除旧 ID。
// 参数 t 为测试上下文；前置注册目录，返回无；无网络与持久数据，无需清理。
func TestEndpointRedesignContract(t *testing.T) {
	for _, old := range []string{"image", "bypass:fal-video"} {
		if provider.KnownEndpoint(old) {
			t.Fatalf("旧端点仍被接受: %s", old)
		}
	}
	for _, e := range provider.EndpointTypes() {
		if e.Category != "openai" && e.Category != "vertex" && e.Category != "claude" && e.Category != "bypass" {
			t.Fatalf("缺少分类: %+v", e)
		}
	}
	for _, tc := range []struct {
		transport, endpoint string
		valid               bool
	}{
		{"openai_image_generation", "image_generation", true}, {"openai_image_generation", "image_edit", false},
		{"openai_image_edit", "image_edit", true}, {"openai_image_edit", "image", false},
		{"gemini_generate_content", "gemini", true}, {"gemini_generate_content", "vertex", true},
		{"vertex_generate_content", "vertex", true}, {"openai_videos", "video", true},
	} {
		m := config.ModelEntry{ModelName: "alias", LiteLLMParams: map[string]any{"model": "real-model", "custom_llm_provider": "custom"}, ModelInfo: map[string]any{"transport": tc.transport, "endpoint_types": []string{tc.endpoint}}}
		if err := provider.ValidateDeployment(m); (err == nil) != tc.valid {
			t.Errorf("%+v: %v", tc, err)
		}
		if tc.valid && len(provider.DeploymentEndpoints(m)) == 0 {
			t.Errorf("未公开已实现端点: %s", tc.endpoint)
		}
	}
}

// TestGoogleNativeActions 验证带操作后缀的模型路径及独立动作解析。
// 参数 t 为测试上下文；前置注册原生协议；验证正常、空别名、错误操作与模型替换，无数据清理。
func TestGoogleNativeActions(t *testing.T) {
	for _, protocol := range []string{"gemini", "vertex"} {
		root := "/v1beta/models/"
		if protocol == "vertex" {
			root = "/vertex/v1/models/"
		}
		for _, op := range []string{"generateContent", "streamGenerateContent", "countTokens"} {
			hit, ok := provider.Match("POST", root+"alias:"+op, nil)
			if !ok || hit.Names["model"] != "alias" {
				t.Fatalf("未命中原生操作: %s %+v", op, hit)
			}
			m := config.ModelEntry{ModelName: "alias", LiteLLMParams: map[string]any{"model": "real"}, ModelInfo: map[string]any{"transport": protocol + "_generate_content", "endpoint_types": []string{protocol}}}
			resolved, err := provider.ResolveHit(hit, m)
			if err != nil || resolved.Names["model"] != "real" || resolved.Action.UpstreamPath[len(resolved.Action.UpstreamPath)-len(op):] != op {
				t.Fatalf("操作或模型错配: %+v %v", resolved, err)
			}
		}
		for _, path := range []string{root + ":generateContent", root + "alias:unknown", root + "../:generateContent"} {
			if _, ok := provider.Match("POST", path, nil); ok {
				t.Fatalf("非法路径被接受: %s", path)
			}
		}
	}
}

// TestGoogleAliasValidation 验证所有对话协议的公开别名支持路径和冒号；空段、点段提供中文原因，上游 ID 不受限制。
// 参数 t 为测试上下文，返回无；只读注册表，无持久数据及清理。
func TestGoogleAliasValidation(t *testing.T) {
	for _, transport := range []string{"gemini_generate_content", "vertex_generate_content", "bypass_openai_chat", "bypass_openai_responses", "bypass_anthropic_messages"} {
		for _, name := range []string{"alias", "alias name", "", ".", "..", "group/model", "alias:op", "group/model:latest", "名称/版本：最新", "group//model", "group/../model", "/model", "model/"} {
			m := config.ModelEntry{ModelName: name, LiteLLMParams: map[string]any{"model": "group/real:latest"}, ModelInfo: map[string]any{"transport": transport, "endpoint_types": []string{"chat"}}}
			valid := name == "alias" || name == "alias name" || name == "group/model" || name == "alias:op" || name == "group/model:latest" || name == "名称/版本：最新"
			if err := provider.ValidateDeployment(m); (err == nil) != valid {
				t.Errorf("别名 %q 校验错误: %v", name, err)
			} else if !valid && !strings.Contains(err.Error(), "对外模型名称") {
				t.Errorf("别名 %q 缺少中文校验说明: %v", name, err)
			}
			if !valid && len(provider.DeploymentEndpoints(m)) > 0 {
				t.Errorf("非法别名 %q 公布了不可调用路径", name)
			}
		}
	}
}

// TestGooglePathAliases 验证 Google 模型占位符解析完整路径和版本后缀，拒绝空段、点段和未知操作。
// 参数 t 为单测上下文；前置编译时注册，返回无；只读目录，无网络和持久数据清理。
func TestGooglePathAliases(t *testing.T) {
	for _, root := range []string{"/v1beta/models/", "/vertex/v1/models/", "/bypass/gemini/v1beta/models/", "/bypass/vertex/v1/models/"} {
		for _, op := range []string{"generateContent", "streamGenerateContent", "countTokens"} {
			for _, name := range []string{"group/model:latest", "group/sub/model", "model:generateContent", "名称/版本：最新"} {
				hit, ok := provider.Match("POST", root+name+":"+op, nil)
				if !ok || hit.Names["model"] != name {
					t.Errorf("完整模型名解析失败 %s%s:%s：%+v", root, name, op, hit)
				}
			}
		}
		for _, name := range []string{"", ".", "..", "group//model", "group/../model", "/model"} {
			if _, ok := provider.Match("POST", root+name+":generateContent", nil); ok {
				t.Errorf("非法模型路径被接受 %s%s", root, name)
			}
		}
		if _, ok := provider.Match("POST", root+"group/model:unknown", nil); ok {
			t.Errorf("未知 Google 操作被接受 %s", root)
		}
	}
	// 多段解析仅用于末尾模型，不得放宽已登记的异步任务 ID。
	if _, ok := provider.Match("GET", "/queue/bytedance/seedance-2.0/requests/group/task", nil); ok {
		t.Fatal("Fal 任务 ID 被错误扩展为多段路径")
	}
}
