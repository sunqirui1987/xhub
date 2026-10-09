package openai

import "github.com/sunqirui1987/xhub/internal/provider"

// init 注册 Responses、Messages 和图片生成/编辑的原生协议传输。
// 参数：无。返回：无；包加载时写入全局注册表。
// 认证由原厂协议定义；只替换路由模型，不转换请求参数。
// 调用：Go 包初始化。测试：dataplane/native_bypass_test.go。
func init() {
	for _, spec := range []struct {
		id, endpoint, protocol, family, path, prefix string
		auth                                         provider.AuthConfig
		headers                                      map[string]string
	}{
		{"bypass_openai_chat", "bypass:openai-chat", "openai-chat", "chat", "/v1/chat/completions", "openai", provider.AuthConfig{Header: "Authorization", Prefix: "Bearer"}, nil},
		{"bypass_openai_responses", "bypass:openai-responses", "openai-responses", "chat", "/v1/responses", "openai", provider.AuthConfig{Header: "Authorization", Prefix: "Bearer"}, nil},
		{"bypass_anthropic_messages", "bypass:anthropic-messages", "anthropic-messages", "chat", "/v1/messages", "anthropic", provider.AuthConfig{Header: "x-api-key"}, map[string]string{"anthropic-version": "2023-06-01"}},
		{"bypass_openai_image_generation", "bypass:openai-images", "openai-images", "image", "/v1/images/generations", "openai", provider.AuthConfig{Header: "Authorization", Prefix: "Bearer"}, nil},
		{"bypass_openai_image_edit", "bypass:openai-image-edit", "openai-images", "image", "/v1/images/edits", "openai", provider.AuthConfig{Header: "Authorization", Prefix: "Bearer"}, nil},
		{"openai_completions", "completion", "openai-completions", "completion", "/v1/completions", "openai", provider.AuthConfig{Header: "Authorization", Prefix: "Bearer"}, nil},
		{"openai_embeddings", "embedding", "openai-embeddings", "embedding", "/v1/embeddings", "openai", provider.AuthConfig{Header: "Authorization", Prefix: "Bearer"}, nil},
		{"openai_audio_speech", "audio_speech", "openai-audio-speech", "audio_speech", "/v1/audio/speech", "openai", provider.AuthConfig{Header: "Authorization", Prefix: "Bearer"}, nil},
		{"openai_audio_transcription", "audio_transcription", "openai-audio-transcription", "audio_transcription", "/v1/audio/transcriptions", "openai", provider.AuthConfig{Header: "Authorization", Prefix: "Bearer"}, nil},
		{"openai_moderations", "moderation", "openai-moderations", "moderation", "/v1/moderations", "openai", provider.AuthConfig{Header: "Authorization", Prefix: "Bearer"}, nil},
		{"rerank", "rerank", "rerank", "rerank", "/v1/rerank", "openai", provider.AuthConfig{Header: "Authorization", Prefix: "Bearer"}, nil},
	} {
		provider.RegisterTransport(provider.Transport{ID: spec.id, Label: spec.endpoint, Kind: provider.KindBypass, EndpointID: spec.endpoint, Protocol: spec.protocol, Family: spec.family, ModelField: "model", Auth: spec.auth, Headers: spec.headers, ResponseUsage: provider.NativeUsage(spec.protocol), Actions: []provider.Action{{Name: "create", Method: "POST", PublicPath: "/bypass/" + spec.prefix + spec.path, UpstreamPath: spec.path}}})
	}
}
