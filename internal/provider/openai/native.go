package openai

import "github.com/sunqirui1987/xhub/internal/provider"

// init 注册 Responses、Messages 和图片生成/编辑的原生协议传输。
// 参数：无。返回：无；包加载时写入全局注册表。
// 七牛使用独立供应商路径前缀，认证由协议定义；只替换路由模型，不转换请求参数。
// 调用：Go 包初始化。测试：dataplane/native_bypass_test.go。
func init() {
	for _, spec := range []struct {
		id, endpoint, protocol, family, path, base, prefix string
		auth                                               provider.AuthConfig
		headers                                            map[string]string
	}{
		{"bypass_openai_responses", "bypass:openai-responses", "openai-responses", "chat", "/v1/responses", "https://api.openai.com", "openai", provider.AuthConfig{Header: "Authorization", Prefix: "Bearer"}, nil},
		{"bypass_anthropic_messages", "bypass:anthropic-messages", "anthropic-messages", "chat", "/v1/messages", "https://api.anthropic.com", "anthropic", provider.AuthConfig{Header: "x-api-key"}, map[string]string{"anthropic-version": "2023-06-01"}},
		{"bypass_openai_image_generation", "bypass:openai-images", "openai-images", "image", "/v1/images/generations", "https://api.openai.com", "openai", provider.AuthConfig{Header: "Authorization", Prefix: "Bearer"}, nil},
		{"bypass_openai_image_edit", "bypass:openai-image-edit", "openai-images", "image", "/v1/images/edits", "https://api.openai.com", "openai", provider.AuthConfig{Header: "Authorization", Prefix: "Bearer"}, nil},
	} {
		provider.RegisterTransport(provider.Transport{ID: spec.id, Label: spec.endpoint, Kind: provider.KindBypass, EndpointType: spec.endpoint, Protocol: spec.protocol, Family: spec.family, APIBase: spec.base, StripPrefix: spec.prefix, ModelField: "model", Auth: spec.auth, Headers: spec.headers, SupplierPrefixes: map[string]string{"qiniu": "/bypass/" + spec.prefix}, ResponseUsage: provider.NativeUsage(spec.protocol), Actions: []provider.Action{{Name: "create", Method: "POST", PublicPath: "/bypass/" + spec.prefix + spec.path, UpstreamPath: spec.path}}})
	}
}
