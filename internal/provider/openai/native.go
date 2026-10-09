package openai

import (
	"encoding/json"
	"github.com/sunqirui1987/xhub/internal/provider"
	"math"
	"strconv"
	"strings"
)

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
		{"openai_audio_translation", "audio_translation", "openai-audio-translation", "audio_transcription", "/v1/audio/translations", "openai", provider.AuthConfig{Header: "Authorization", Prefix: "Bearer"}, nil},
		{"openai_audio_transcription", "audio_transcription", "openai-audio-transcription", "audio_transcription", "/v1/audio/transcriptions", "openai", provider.AuthConfig{Header: "Authorization", Prefix: "Bearer"}, nil},
		{"openai_moderations", "moderation", "openai-moderations", "moderation", "/v1/moderations", "openai", provider.AuthConfig{Header: "Authorization", Prefix: "Bearer"}, nil},
		{"rerank", "rerank", "rerank", "rerank", "/v1/rerank", "openai", provider.AuthConfig{Header: "Authorization", Prefix: "Bearer"}, nil},
	} {
		provider.RegisterTransport(provider.Transport{ID: spec.id, Label: spec.endpoint, Kind: provider.KindBypass, EndpointID: spec.endpoint, Protocol: spec.protocol, Family: spec.family, ModelField: "model", Auth: spec.auth, Headers: spec.headers, ResponseUsage: provider.NativeUsage(spec.protocol), Actions: []provider.Action{{Name: "create", Method: "POST", PublicPath: nativePublicPath(spec.endpoint, spec.prefix, spec.path), UpstreamPath: spec.path}}})
	}
}

// nativePublicPath 返回已登记操作的公开路径；参数为端点、供应商前缀与标准路径。
// 返回：Bypass 对话/图片保留独立路径，标准媒体使用原厂路径。调用：注册，无外部副作用。
func nativePublicPath(endpoint, prefix, path string) string {
	if len(endpoint) >= 7 && endpoint[:7] == "bypass:" {
		return "/bypass/" + prefix + path
	}
	return path
}

// init 登记独立图片操作、视频任务与 Gemini/Vertex 原生执行协议。
// 参数：无；返回：无。调用：包初始化；仅登记固定协议，不从部署或请求读取任意路径。
// Vertex 凭据地址必须包含项目、区域及 publishers/google，密钥为有效 OAuth Bearer token。
func init() {
	for _, spec := range []struct{ id, endpoint, protocol, family, path string }{
		{"openai_image_generation", "image_generation", "openai-images", "image", "/v1/images/generations"},
		{"openai_image_edit", "image_edit", "openai-images", "image", "/v1/images/edits"},
		{"openai_videos", "video", "openai-videos", "video", "/v1/videos"},
	} {
		tr := provider.Transport{ID: spec.id, Label: spec.endpoint, Kind: provider.KindBypass, EndpointID: spec.endpoint, Protocol: spec.protocol, Family: spec.family, ModelField: "model", Auth: provider.AuthConfig{Header: "Authorization", Prefix: "Bearer"}, ResponseUsage: provider.NativeUsage(spec.protocol), Actions: []provider.Action{{Name: "create", Method: "POST", PublicPath: spec.path, UpstreamPath: spec.path}}}
		if spec.protocol == "openai-videos" {
			tr.TaskID = "id"
			tr.Actions = append(tr.Actions, provider.Action{Name: "get", Method: "GET", PublicPath: spec.path + "/{id}", UpstreamPath: spec.path + "/{id}"}, provider.Action{Name: "content", Method: "GET", PublicPath: spec.path + "/{id}/content", UpstreamPath: spec.path + "/{id}/content"})
			tr.Billing = &provider.TaskBilling{Context: videoContext, Usage: videoUsage}
		}
		provider.RegisterTransport(tr)
	}
	for _, protocol := range []string{"gemini", "vertex"} {
		public, upstream := "/v1beta/models/{model}", "/v1beta/models/{model}"
		auth := provider.AuthConfig{Header: "x-goog-api-key"}
		if protocol == "vertex" {
			public, upstream = "/vertex/v1/models/{model}", "/models/{model}"
			auth = provider.AuthConfig{Header: "Authorization", Prefix: "Bearer"}
		}
		tr := provider.Transport{ID: protocol + "_generate_content", Label: protocol, EndpointID: protocol, Protocol: protocol, Kind: provider.KindBypass, Family: "chat", Auth: auth, ResponseUsage: provider.NativeUsage(protocol)}
		for _, operation := range []string{"generateContent", "streamGenerateContent", "countTokens"} {
			tr.Actions = append(tr.Actions, provider.Action{Name: "create", Method: "POST", PublicPath: public + ":" + operation, UpstreamPath: upstream + ":" + operation})
		}
		// 同一上游配置额外登记独立 Bypass 路径，保留原厂扩展字段。
		for _, action := range append([]provider.Action(nil), tr.Actions...) {
			if protocol == "gemini" {
				action.PublicPath = "/bypass/gemini" + action.PublicPath
			} else {
				action.PublicPath = "/bypass/vertex" + strings.TrimPrefix(action.PublicPath, "/vertex")
			}
			tr.Actions = append(tr.Actions, action)
		}
		provider.RegisterTransport(tr)
	}
}

// videoContext 保存创建时间以外的最小计价上下文；参数为请求字段，返回未知实测用量的上下文。
// 调用：视频创建，实际 seconds 只能从已完成响应读取，不使用请求时长预扣。
func videoContext(request map[string]any) provider.TaskContext { return provider.TaskContext{} }

// videoUsage 仅提取成功视频的实测秒数；参数为响应和固定上下文，返回事实或 nil。
// 调用：轮询结算；未完成、失败或非法秒数时不产生账单，任务由数据面隔离并去重。
func videoUsage(doc map[string]any, ctx provider.TaskContext) map[string]any {
	if doc["status"] != "completed" || doc["error"] != nil {
		return nil
	}
	var seconds float64
	var err error
	// OpenAI seconds 可为字符串；仅接受有限非负数，防止格式错误被计量层当成免费成功。
	switch value := doc["seconds"].(type) {
	case string:
		seconds, err = strconv.ParseFloat(value, 64)
	case json.Number:
		seconds, err = value.Float64()
	case float64:
		seconds = value
	case int:
		seconds = float64(value)
	default:
		return nil
	}
	if err != nil || math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds < 0 {
		return nil
	}
	return map[string]any{"seconds": seconds}
}
