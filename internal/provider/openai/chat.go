// Package openai registers the adapted endpoint types the add-model form already
// offered as health-check modes. Empty mode on an old deployment still means chat.
package openai

import "github.com/sunqirui1987/xhub/internal/provider"

// 进程启动时登记这个目录的供应商、端点类型和模型价格。添加模型时就能选到它们。
// 参数：无。
// 返回：无。chat、completion、embedding 等适配端点已登记。添加模型时就能选到。
// 调用：Go 在载入这个包时自动执行。
// 测试：无直接单测
func init() {
	provider.RegisterType(provider.Type{
		ID: "chat", Kind: provider.KindAdapted, Label: "Chat - /chat/completions", Operation: "chat",
		ModelField: "model",
		Actions:    []provider.Action{{Name: "create", Method: "POST", PublicPath: "/v1/chat/completions", UpstreamPath: "/chat/completions"}},
	})
	provider.RegisterType(provider.Type{
		ID: "completion", Kind: provider.KindAdapted, Label: "Completion - /completions", Operation: "completion",
		ModelField: "model",
		Actions:    []provider.Action{{Name: "create", Method: "POST", PublicPath: "/v1/completions", UpstreamPath: "/completions"}},
	})
	provider.RegisterType(provider.Type{
		ID: "embedding", Kind: provider.KindAdapted, Label: "Embedding - /embeddings", Operation: "embedding",
		ModelField: "model",
		Actions:    []provider.Action{{Name: "create", Method: "POST", PublicPath: "/v1/embeddings", UpstreamPath: "/embeddings"}},
	})
	provider.RegisterType(provider.Type{
		ID: "image_generation", Kind: provider.KindAdapted, Label: "Image Generation - /images/generations", Operation: "image",
		ModelField: "model",
		Actions:    []provider.Action{{Name: "create", Method: "POST", PublicPath: "/v1/images/generations", UpstreamPath: "/images/generations"}},
	})
	provider.RegisterType(provider.Type{
		ID: "audio_speech", Kind: provider.KindAdapted, Label: "Audio Speech - /audio/speech", Operation: "audio_speech",
		ModelField: "model",
		Actions:    []provider.Action{{Name: "create", Method: "POST", PublicPath: "/v1/audio/speech", UpstreamPath: "/audio/speech"}},
	})
	provider.RegisterType(provider.Type{
		ID: "rerank", Kind: provider.KindAdapted, Label: "Rerank - /rerank", Operation: "rerank",
		ModelField: "model",
		Actions:    []provider.Action{{Name: "create", Method: "POST", PublicPath: "/v1/rerank", UpstreamPath: "/rerank"}},
	})
	provider.RegisterType(provider.Type{
		ID: "video_generation", Kind: provider.KindAdapted, Label: "Video Generation - /videos", Operation: "videos",
		ModelField: "model",
		Actions:    []provider.Action{{Name: "create", Method: "POST", PublicPath: "/v1/videos", UpstreamPath: "/videos"}},
	})
}
