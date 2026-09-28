// Package llm describes the operation, address, key, and body of one upstream call.
package llm

import (
	"context"
	"encoding/json"
	"github.com/sunqirui1987/xhub/internal/logx"
	"net/http"
	"strings"
	"sync"
)

// Operation names match the data-plane op. An empty string and "chat" both mean chat completions.
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

var logTraceOnceCall sync.Once

// Headers is the minimum header set sent upstream.
//
// The key is placed only in Authorization and not in the body, so a log or a cache key does not expand it again.
// Content-Type is fixed as JSON. Multipart requests such as audio should later use the matching provider,
// and this function does not guess that shape.
func Headers(apiKey string) http.Header {
	logTraceOnceCall.Do(func() { logx.Trace("enter llm.Headers") })

	h := make(http.Header)
	h.Set("Authorization", "Bearer "+apiKey)
	h.Set("Content-Type", "application/json")
	return h
}

// DefaultAPIBase is the official root used when a deployment does not set api_base.
// OpenAI chat joins /chat/completions onto https://api.openai.com/v1. An unknown provider returns an empty string.
func DefaultAPIBase(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "openai":
		return "https://api.openai.com/v1"
	case "anthropic":
		return "https://api.anthropic.com"
	case "groq":
		return "https://api.groq.com/openai/v1"
	case "mistral":
		return "https://api.mistral.ai/v1"
	case "deepseek":
		return "https://api.deepseek.com"
	case "xai":
		return "https://api.x.ai/v1"
	default:
		return ""
	}
}

// Endpoint returns the full URL for this operation.
//
// apiBase must already be resolved by the credential layer. This function does not replace an empty address with api.openai.com,
// or an unconfigured deployment would quietly call the official host. The caller should return
// an authentication error when the address or the key is empty.
//
// Path rules match each LiteLLM provider's get_complete_url, folded into one table:
// OpenAI-compatible providers share the default branch, and Azure, Anthropic, Gemini, and Vertex have their own branches.
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
		// Gemini has no Anthropic Messages path. The messages API still calls generateContent,
		// and Decode turns the candidates into the Messages shape.
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

// chatEndpoint is the provider path for chat completions.
// Every OpenAI-compatible provider uses {base}/chat/completions. The model name is in the body, not in the path.
func chatEndpoint(provider, base, model string) string {
	switch provider {
	case "anthropic":
		return base + "/v1/messages"
	case "azure":
		return base + "/openai/deployments/" + model + "/chat/completions"
	case "gemini":
		return base + "/v1beta/models/" + model + ":generateContent"
	case "vertex_ai":
		// The project and region come from the deployment. This is only the path shape without those parameters. The placeholder project name was removed.
		return base + "/v1/projects/vertex-project/locations/us-central1/publishers/google/models/" + model + ":generateContent"
	default:
		return base + "/chat/completions"
	}
}

// Encode turns the public request into the upstream body.
//
// The public model is the routing alias. The upstream wants the real model name from the deployment, so model is overwritten here.
// Other fields are kept as they are. Temperature, tools, and the stream flag come from the caller. This function does not fill defaults,
// except Anthropic Messages, which fills max_tokens with 256 when it is missing. That field is required by the Messages API,
// and the upstream returns 400 without it.
func Encode(op, provider string, body map[string]any, model string) ([]byte, error) {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if provider == "gemini" || provider == "vertex_ai" {
		// The body is produced by google.golang.org/genai. See Build.
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
	// Proxy fields do not enter the provider body. The rule matches litellm.utils.filter_out_litellm_params.
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

// shapeResponsesMessages turns type=text inside chat messages into the Responses API input_text.
// Assistant messages stay as they are. The rule lives in responses.ShapePromptManagedMessage.
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

// Decode turns an upstream response into the public shape and sets model back to the alias the caller used.
//
// Audio bytes have no JSON model field and are returned unchanged. The model field on images, rerank, and transcriptions
// belongs to the result and is not overwritten with the alias. Other JSON objects set model to the alias.
// A Messages operation then turns a chat.completion into an Anthropic message object.
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

// decodeMessages turns an OpenAI chat.completion into an Anthropic Messages object.
// Existing content is not overwritten, so an upstream body that is already a Messages shape is not replaced by chat fields.
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
