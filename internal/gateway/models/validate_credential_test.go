package models

import "testing"

// TestCredentialProtocolMatches 验证正常协议、OpenAI 兼容别名、身份不能替代协议、空字段和不兼容输入。
// 参数 t：单测上下文；返回：无。仅构造内存凭据，无外部依赖或清理副作用。
func TestCredentialProtocolMatches(t *testing.T) {
	for _, tc := range []struct {
		name, builtin, protocol, fallback, current string
		want                                       bool
	}{
		{"relay", "", "openai", "", "openai", true},
		{"custom-openai", "", "custom_openai", "", "openai", true},
		{"openai-custom", "", "openai", "", "custom_openai", true},
		{"native", "", "volcengine", "", "volcengine", true},
		{"legacy", "qiniu", "openai", "", "qiniu", false},
		{"qiniu", "", "openai", "", "qiniu", false},
		{"fallback", "qiniu", "", "openai", "qiniu", false},
		{"normalized", "qiniu", " OpenAI ", "", " OpenAI ", true},
		{"compatible-chat", "qiniu", "qiniu", "", "openai", false},
		{"empty-credential", "", "", "", "openai", true},
		{"empty-model", "", "openai", "", "", true},
		{"ordinary-openai", "", "openai", "", "qiniu", false},
		{"wrong-builtin", "fenno", "openai", "", "qiniu", false},
		{"qiniu", "fenno", "openai", "", "qiniu", false},
		{"wrong-wire", "qiniu", "anthropic", "openai", "qiniu", false},
		{"wrong-target", "qiniu", "openai", "", "volcengine", false},
		{"wrong-fallback", "qiniu", "", "anthropic", "qiniu", false},
	} {
		t.Run(tc.name+"/"+tc.builtin, func(t *testing.T) {
			record := map[string]any{
				"credential_info":   map[string]any{"builtin": tc.builtin, "custom_llm_provider": tc.protocol},
				"credential_values": map[string]any{"custom_llm_provider": tc.fallback},
			}
			if got := credentialProtocolMatches(tc.name, record, tc.current); got != tc.want {
				t.Fatalf("凭据 %s 协议 %q 到部署 %q 的兼容结果=%v，预期=%v", tc.name, tc.protocol, tc.current, got, tc.want)
			}
		})
	}
	if !credentialProtocolMatches("missing", nil, "openai") {
		t.Fatal("缺字段凭据应保持空协议的历史兼容行为")
	}
}

// TestOpenAICompatibleProtocol 验证目录发现只接受 OpenAI 线协议及其 Custom 表单别名。
// 参数 t：单测上下文；返回：无。覆盖大小写与空白的正常输入，以及 Custom、Anthropic 和空值失败输入。
func TestOpenAICompatibleProtocol(t *testing.T) {
	for _, tc := range []struct {
		protocol string
		want     bool
	}{
		{"openai", true},
		{" CUSTOM_OPENAI ", true},
		{"custom", false},
		{"anthropic", false},
		{"", false},
	} {
		if got := openAICompatibleProtocol(tc.protocol); got != tc.want {
			t.Fatalf("协议 %q 的 OpenAI 兼容结果=%v，预期=%v", tc.protocol, got, tc.want)
		}
	}
}
