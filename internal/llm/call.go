// 描述一次上游调用要带的操作、地址、密钥和正文。
package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

// 操作名与网关数据面的 op 一致。空字符串和 "chat" 都表示聊天补全。
const (
	OpChat               = "chat"
	OpCompletions        = "completions"
	OpEmbeddings         = "embeddings"
	OpMessages           = "messages"
	OpImages             = "images"
	OpImageEdits         = "images_edits"
	OpAudioSpeech        = "audio_speech"
	OpAudioTranscription = "audio_transcription"
	OpAudioTranslation   = "audio_translation"
	OpModerations        = "moderations"
	OpRerank             = "rerank"
	OpResponses          = "responses"
	OpVideos             = "videos"
)

// Headers 是打到上游的最低请求头。
//
// 密钥只放在 Authorization，不写进请求体，避免被日志或缓存键再次展开。
// Content-Type 固定为 JSON。音频等多部分请求以后要换成对应的 Provider，
// 不在这里猜测。
func Headers(apiKey string) http.Header {
	h := make(http.Header)
	h.Set("Authorization", "Bearer "+apiKey)
	h.Set("Content-Type", "application/json")
	return h
}

// Endpoint 返回这次操作的完整 URL。
//
// apiBase 必须已经由凭证层解析好。本函数不把空地址改成 api.openai.com，
// 否则未配置的部署会悄悄打到官方。调用方在地址或密钥为空时应直接返回
// 认证错误。
//
// 路径规则与 LiteLLM 各供应商的 get_complete_url 对齐，但合并成一张表：
// OpenAI 兼容供应商共用默认分支，Azure / Anthropic / Gemini / Vertex 单独分支。
func Endpoint(op, provider, apiBase, model string) string {
	base := strings.TrimRight(apiBase, "/")
	provider = strings.ToLower(strings.TrimSpace(provider))
	azure := base + "/openai/deployments/" + model
	switch op {
	case OpEmbeddings:
		if provider == "azure" {
			return azure + "/embeddings"
		}
		return base + "/embeddings"
	case OpCompletions:
		if provider == "azure" {
			return azure + "/completions"
		}
		return base + "/completions"
	case OpMessages:
		// Gemini 没有 Anthropic Messages 路径。消息接口仍打 generateContent，
		// 由 Decode 把候选结果收成 Messages 形状。
		if provider == "gemini" || provider == "vertex_ai" {
			return chatEndpoint(provider, base, model)
		}
		return base + "/v1/messages"
	case OpImages:
		if provider == "azure" {
			return azure + "/images/generations"
		}
		return base + "/images/generations"
	case OpImageEdits:
		if provider == "azure" {
			return azure + "/images/edits"
		}
		return base + "/images/edits"
	case OpAudioSpeech:
		return base + "/audio/speech"
	case OpAudioTranscription:
		return base + "/audio/transcriptions"
	case OpAudioTranslation:
		return base + "/audio/translations"
	case OpModerations:
		return base + "/moderations"
	case OpRerank:
		return base + "/rerank"
	case OpResponses:
		return base + "/responses"
	case OpVideos:
		return base + "/videos"
	case "gemini":
		return chatEndpoint(provider, base, model)
	default:
		return chatEndpoint(provider, base, model)
	}
}

// chatEndpoint 是聊天补全的供应商路径。
// OpenAI 兼容供应商一律是 {base}/chat/completions。模型名在请求体里，不在路径里。
func chatEndpoint(provider, base, model string) string {
	switch provider {
	case "anthropic":
		return base + "/v1/messages"
	case "azure":
		return base + "/openai/deployments/" + model + "/chat/completions"
	case "gemini":
		return base + "/v1beta/models/" + model + ":generateContent"
	case "vertex_ai":
		// 项目与区域来自部署。这里只给出未带参数时的路径形状，占位项目名已删除。
		return base + "/v1/projects/vertex-project/locations/us-central1/publishers/google/models/" + model + ":generateContent"
	default:
		return base + "/chat/completions"
	}
}

// Encode 把对外请求收成上游请求体。
//
// 对外的 model 是路由别名。上游要的是部署里的真实模型名，所以这里覆盖 model。
// 其余字段原样保留：温度、工具、流式开关都由调用方传入，本函数不补默认值，
// 除了 Anthropic Messages 在缺少 max_tokens 时补 256。那是 Messages API 的必填项，
// 缺了上游会直接 400。
func Encode(op, provider string, body map[string]any, model string) ([]byte, error) {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if provider == "gemini" || provider == "vertex_ai" {
		// 正文由 google.golang.org/genai 生成，见 Build。
		up, err := Build(context.Background(), Request{
			Op: op, Provider: provider, APIBase: "https://generativelanguage.googleapis.com",
			APIKey: "builder", Model: model, Body: body,
			VertexProject: "vertex-project", VertexLocation: "us-central1",
		})
		if err != nil {
			return nil, err
		}
		return up.Body, nil
	}
	out := map[string]any{}
	for k, v := range body {
		out[k] = v
	}
	// 代理字段不进供应商正文。规则与 litellm.utils.filter_out_litellm_params 相同。
	StripProxyParams(out)
	out["model"] = model
	if op == OpResponses {
		shapeResponsesMessages(out)
	}
	if op == OpMessages {
		if _, ok := out["max_tokens"]; !ok {
			out["max_tokens"] = 256
		}
	}
	return json.Marshal(out)
}

// shapeResponsesMessages 把聊天消息里的 type=text 收成 Responses API 的 input_text。
// 助手消息保持原样。规则在 responses.ShapePromptManagedMessage。
func shapeResponsesMessages(body map[string]any) {
	msgs, ok := body["messages"].([]any)
	if !ok {
		return
	}
	shaped := make([]any, len(msgs))
	for i, raw := range msgs {
		msg, ok := raw.(map[string]any)
		if !ok {
			shaped[i] = raw
			continue
		}
		shaped[i] = ShapeResponsesMessage(msg)
	}
	body["messages"] = shaped
}

// Decode 把上游响应收成对外形状，并把 model 改回调用方使用的别名。
//
// 音频二进制没有 JSON 模型字段，原样返回。图像、重排和转写的 model 字段
// 属于结果本身，不用别名覆盖。其余 JSON 对象把 model 改成别名。
// Messages 操作再把 chat.completion 收成 Anthropic 的 message 对象。
func Decode(op, provider, alias string, raw []byte) []byte {
	if op == OpAudioSpeech {
		return raw
	}
	provider = strings.ToLower(strings.TrimSpace(provider))
	if (provider == "gemini" || provider == "vertex_ai") && (op == OpChat || op == OpCompletions || op == OpMessages || op == OpEmbeddings) {
		return decodeGemini(op, alias, raw)
	}
	var doc map[string]any
	if json.Unmarshal(raw, &doc) != nil {
		return raw
	}
	if op != OpImages && op != OpImageEdits && op != OpRerank && op != OpAudioTranscription {
		doc["model"] = alias
	}
	if op == OpMessages {
		decodeMessages(doc)
	}
	encoded, err := json.Marshal(doc)
	if err != nil {
		return raw
	}
	return encoded
}

// decodeMessages 把 OpenAI chat.completion 收成 Anthropic Messages 的对象。
// content 已存在时不覆盖，避免上游本来就是 Messages 形状时被聊天字段冲掉。
func decodeMessages(doc map[string]any) {
	if _, ok := doc["content"]; !ok {
		text := ""
		if choices, ok := doc["choices"].([]any); ok && len(choices) > 0 {
			if choice, ok := choices[0].(map[string]any); ok {
				if msg, ok := choice["message"].(map[string]any); ok {
					text = textOf(msg["content"])
				} else if t, ok := choice["text"].(string); ok {
					text = t
				}
			}
		}
		doc["content"] = []any{map[string]any{"type": "text", "text": text}}
	}
	delete(doc, "choices")
	delete(doc, "object")
	delete(doc, "created")
	delete(doc, "system_fingerprint")
	doc["type"] = "message"
	if _, ok := doc["role"]; !ok {
		doc["role"] = "assistant"
	}
	if _, ok := doc["stop_reason"]; !ok {
		doc["stop_reason"] = "end_turn"
	}
	if usage, ok := doc["usage"].(map[string]any); ok {
		if _, has := usage["input_tokens"]; !has {
			usage["input_tokens"] = usage["prompt_tokens"]
			usage["output_tokens"] = usage["completion_tokens"]
		}
	}
}
