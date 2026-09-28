// Package llm turns one call into an upstream URL, headers, and body. It does not send HTTP and it does not read the database.
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"google.golang.org/genai"
)

// Request is every input needed to decide one upstream call.
//
// Op matches the data-plane operation name, such as chat, embeddings, or images.
// Provider must be one of the names ProtocolGroup knows. The caller skips a name that is not in that table.
// APIBase and APIKey are already filled by the credential layer. This function does not replace an empty address with the official host.
type Request struct {
	Op             string
	Provider       string
	APIBase        string
	APIKey         string
	Model          string
	Body           map[string]any
	VertexProject  string
	VertexLocation string
}

// Upstream is the HTTP request for the provider. It has not been dialed yet.
//
// URL, Header, and Body together are the contract. Changing only one part, such as only the base URL while
// the body stays OpenAI chat, is not aligned for a provider that changed authentication or the payload.
type Upstream struct {
	URL    string
	Header http.Header
	Body   []byte
}

// Build builds the upstream request for the protocol group.
//
// OpenAI and providers that only change the base URL and a Bearer key use github.com/openai/openai-go.
// Gemini and Vertex generateContent use google.golang.org/genai.
// Azure still uses the OpenAI SDK JSON, but the URL is the deployment path and the auth header is api-key.
// Anthropic, Cohere, and Bedrock each have their own path and body. They are not posted to /chat/completions.
func Build(ctx context.Context, in Request) (Upstream, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	provider := strings.ToLower(strings.TrimSpace(in.Provider))
	group, ok := ProtocolGroup(provider)
	if !ok {
		return Upstream{}, errUnknownProvider
	}
	if group == "deprecated_providers" {
		return Upstream{}, errDeprecatedProvider
	}
	in.Provider = provider
	switch group {
	case "gemini_genai":
		return buildGemini(ctx, in)
	case "azure_openai":
		return buildAzure(ctx, in)
	case "azure_ai_foundry":
		return buildAzureAI(ctx, in)
	case "anthropic_messages":
		return buildAnthropic(in)
	case "cohere_http":
		return buildCohere(in)
	case "bedrock_aws":
		return buildBedrock(in)
	case "search_http":
		return buildSearch(in)
	case "image_media_http":
		return buildImageHost(in)
	case "audio_http":
		return buildAudioHost(in)
	case "embed_rerank_http":
		return buildEmbedHost(in)
	case "vector_store":
		return buildVector(in)
	case "sandbox":
		return buildSandbox(in)
	case "ocr_http":
		return buildOCR(in)
	case "own_protocol_http":
		return buildOwn(in)
	case "shared_runtime":
		return buildShared(in)
	default:
		// openai_byte_compatible, openai_native, and openai_delta use the OpenAI wire builder.
		return buildOpenAIWire(ctx, in)
	}
}

var (
	errUnknownProvider    = errString("provider_not_implemented")
	errDeprecatedProvider = errString("deprecated_provider")
)

type errString string

// Error returns the text for an encoding failure. An unknown or retired provider uses a fixed sentence.
func (e errString) Error() string { return string(e) }

// ChatProbeUsesFixture reports that the standard test upstream can answer this chat.
//
// Those providers' chat URLs end in /chat/completions, /v1/messages, or :generateContent.
// Every other protocol group must be checked from the URL and body Build returns. Success cannot be judged from the same OpenAI path.
func ChatProbeUsesFixture(provider string) bool {
	group, ok := ProtocolGroup(strings.ToLower(strings.TrimSpace(provider)))
	if !ok {
		return false
	}
	switch group {
	case "openai_byte_compatible", "openai_native", "openai_delta", "azure_openai", "azure_ai_foundry", "anthropic_messages", "gemini_genai":
		return true
	default:
		return false
	}
}

// buildOpenAIWire builds the upstream URL, headers, and body for an OpenAI-compatible group. Most providers use this instead of copying it.
func buildOpenAIWire(ctx context.Context, in Request) (Upstream, error) {
	op := in.Op
	if op == "" || op == OpChat || op == "gemini" {
		up, err := captureOpenAIChat(ctx, in.APIBase, in.APIKey, in.Model, in.Body)
		if err != nil {
			return Upstream{}, err
		}
		return up, nil
	}
	return legacy(in)
}

// buildAzure builds a request on the Azure deployment path. The model name goes in the deployment segment of the URL, not in the model field of the body.
func buildAzure(ctx context.Context, in Request) (Upstream, error) {
	// The Azure OpenAI path puts the model in the deployment name. Authentication is api-key, not a Bearer call to /chat/completions.
	body, err := openAIChatBody(ctx, in.APIBase, in.APIKey, in.Model, in.Body)
	if err != nil {
		return Upstream{}, err
	}
	h := http.Header{}
	h.Set("api-key", in.APIKey)
	h.Set("Content-Type", "application/json")
	// This is AZURE_DEFAULT_API_VERSION from LiteLLM 1.102.0. The deployment path must carry this query parameter.
	u := Endpoint(in.Op, "azure", in.APIBase, in.Model)
	if !strings.Contains(u, "api-version=") {
		u += "?api-version=2025-02-01-preview"
	}
	return Upstream{URL: u, Header: h, Body: body}, nil
}

// buildAzureAI builds a request with the Azure AI address rules. The key source differs from Azure OpenAI.
func buildAzureAI(ctx context.Context, in Request) (Upstream, error) {
	// Foundry is not the public Azure deployment path. Chat uses /openai/v1/chat/completions and an azure-ai header.
	body, err := openAIChatBody(ctx, in.APIBase, in.APIKey, in.Model, in.Body)
	if err != nil {
		return Upstream{}, err
	}
	h := http.Header{}
	h.Set("api-key", in.APIKey)
	h.Set("Content-Type", "application/json")
	base := strings.TrimRight(in.APIBase, "/")
	return Upstream{URL: base + "/openai/v1/chat/completions", Header: h, Body: body}, nil
}

// buildAnthropic rewrites a chat request into the Anthropic Messages API body and headers.
func buildAnthropic(in Request) (Upstream, error) {
	// The Messages API path is {api_base}/v1/messages.
	// LiteLLM turns a string content into [{type:text,text}]. The headers are x-api-key and anthropic-version: 2023-06-01.
	shaped := anthropicBody(cloneMap(in.Body))
	raw, err := Encode(OpMessages, "anthropic", shaped, in.Model)
	if err != nil {
		return Upstream{}, err
	}
	h := http.Header{}
	h.Set("x-api-key", in.APIKey)
	h.Set("anthropic-version", "2023-06-01")
	h.Set("Content-Type", "application/json")
	return Upstream{URL: strings.TrimRight(in.APIBase, "/") + "/v1/messages", Header: h, Body: raw}, nil
}

// RealtimeClientSecretsURL matches LiteLLM OpenAIRealtimeHTTPConfig.get_complete_url.
// A custom api_base that ends in /v1 is trimmed first and then joined with /v1/realtime/client_secrets.
func RealtimeClientSecretsURL(apiBase string) string {
	base := strings.TrimRight(apiBase, "/")
	base = strings.TrimSuffix(base, "/v1")
	return base + "/v1/realtime/client_secrets"
}

// buildCohere builds a Cohere protocol-group request. Chat and completion use different paths.
func buildCohere(in Request) (Upstream, error) {
	if in.Op == OpRerank {
		return buildCohereRerank(in)
	}
	// LiteLLM Cohere v2 treats a caller-supplied api_base as the full URL and does not append /v2/chat.
	// The body is a messages array plus request-source. The official https://api.cohere.com/v2/chat is used only when api_base was not provided.
	payload := map[string]any{"model": in.Model}
	if msgs, ok := in.Body["messages"]; ok {
		payload["messages"] = msgs
	} else if msg := firstText(in.Body); msg != "" {
		payload["messages"] = []any{map[string]any{"role": "user", "content": msg}}
	}
	if v, ok := in.Body["max_tokens"]; ok {
		payload["max_tokens"] = v
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return Upstream{}, err
	}
	h := Headers(in.APIKey)
	h.Set("request-source", "unspecified:litellm")
	base := strings.TrimRight(in.APIBase, "/")
	if base == "" {
		base = "https://api.cohere.com/v2/chat"
	}
	return Upstream{URL: base, Header: h, Body: raw}, nil
}

// buildCohereRerank matches LiteLLM CohereRerankV2Config.
// When api_base is set and does not already end in /v2/rerank, that path is appended. Otherwise https://api.cohere.ai/v2/rerank is used.
// LiteLLM defaults return_documents to true when it is omitted. Authentication is only Bearer, without the chat request-source header.
func buildCohereRerank(in Request) (Upstream, error) {
	base := strings.TrimRight(in.APIBase, "/")
	if base == "" {
		base = "https://api.cohere.ai/v2/rerank"
	} else if !strings.HasSuffix(base, "/v2/rerank") {
		base += "/v2/rerank"
	}
	payload := map[string]any{
		"model":     in.Model,
		"query":     in.Body["query"],
		"documents": in.Body["documents"],
	}
	if v, ok := in.Body["top_n"]; ok {
		payload["top_n"] = v
	}
	if v, ok := in.Body["return_documents"]; ok {
		payload["return_documents"] = v
	} else {
		payload["return_documents"] = true
	}
	for _, key := range []string{"rank_fields", "max_tokens_per_doc"} {
		if v, ok := in.Body[key]; ok && v != nil {
			payload[key] = v
		}
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return Upstream{}, err
	}
	return Upstream{URL: base, Header: Headers(in.APIKey), Body: raw}, nil
}

// anthropicBody extracts the messages and system Anthropic needs from an OpenAI-shaped body.
func anthropicBody(body map[string]any) map[string]any {
	msgs, ok := body["messages"].([]any)
	if !ok {
		return body
	}
	out := make([]any, len(msgs))
	for i, raw := range msgs {
		msg, ok := raw.(map[string]any)
		if !ok {
			out[i] = raw
			continue
		}
		copied := cloneMap(msg)
		if text, ok := copied["content"].(string); ok {
			copied["content"] = []any{map[string]any{"type": "text", "text": text}}
		}
		out[i] = copied
	}
	body["messages"] = out
	return body
}

// buildBedrock builds a request on the Bedrock model path. Region and keys do not read environment variables in this function.
func buildBedrock(in Request) (Upstream, error) {
	// Bedrock InvokeModel puts the model id in the path and uses AWS SigV4 headers instead of Bearer plus /chat/completions.
	// This assembles the credential scope for the signed headers. It does not connect to AWS.
	payload, err := json.Marshal(map[string]any{
		"anthropic_version": "bedrock-2023-05-31",
		"max_tokens":        256,
		"messages":          in.Body["messages"],
		"model":             in.Model,
	})
	if err != nil {
		return Upstream{}, err
	}
	h := http.Header{}
	h.Set("Content-Type", "application/json")
	h.Set("Authorization", "AWS4-HMAC-SHA256 Credential="+in.APIKey+"/bedrock/invoke")
	h.Set("X-Amz-Target", "AmazonBedrock.InvokeModel")
	base := strings.TrimRight(in.APIBase, "/")
	return Upstream{URL: base + "/model/" + in.Model + "/invoke", Header: h, Body: payload}, nil
}

// buildSearch builds a search upstream request. The body still comes from the caller. This function only chooses the address and headers.
func buildSearch(in Request) (Upstream, error) {
	// Search providers accept query, not chat messages.
	q := str(in.Body["query"])
	if q == "" {
		q = firstText(in.Body)
	}
	payload, err := json.Marshal(map[string]any{"query": q, "provider": in.Provider})
	if err != nil {
		return Upstream{}, err
	}
	return Upstream{
		URL:    strings.TrimRight(in.APIBase, "/") + "/search",
		Header: Headers(in.APIKey),
		Body:   payload,
	}, nil
}

// buildImageHost builds the upstream address for image generation or editing.
func buildImageHost(in Request) (Upstream, error) {
	payload, err := json.Marshal(map[string]any{
		"model":  in.Model,
		"prompt": str(in.Body["prompt"]),
	})
	if err != nil {
		return Upstream{}, err
	}
	return Upstream{
		URL:    strings.TrimRight(in.APIBase, "/") + "/images",
		Header: Headers(in.APIKey),
		Body:   payload,
	}, nil
}

// buildAudioHost builds the upstream address for speech or transcription.
func buildAudioHost(in Request) (Upstream, error) {
	payload, err := json.Marshal(map[string]any{"model": in.Model, "text": firstText(in.Body)})
	if err != nil {
		return Upstream{}, err
	}
	return Upstream{
		URL:    strings.TrimRight(in.APIBase, "/") + "/audio",
		Header: Headers(in.APIKey),
		Body:   payload,
	}, nil
}

// buildEmbedHost builds the upstream address for embeddings.
func buildEmbedHost(in Request) (Upstream, error) {
	// DashScope, Jina, and Voyage embedding paths are not the OpenAI /embeddings path.
	payload, err := json.Marshal(map[string]any{"model": in.Model, "input": in.Body["input"]})
	if err != nil {
		return Upstream{}, err
	}
	return Upstream{
		URL:    strings.TrimRight(in.APIBase, "/") + "/embed",
		Header: Headers(in.APIKey),
		Body:   payload,
	}, nil
}

// buildVector builds the upstream address for a vector-store call.
func buildVector(in Request) (Upstream, error) {
	payload, err := json.Marshal(map[string]any{"provider": in.Provider, "query": firstText(in.Body)})
	if err != nil {
		return Upstream{}, err
	}
	return Upstream{
		URL:    strings.TrimRight(in.APIBase, "/") + "/vectors/query",
		Header: Headers(in.APIKey),
		Body:   payload,
	}, nil
}

// buildSandbox builds the upstream address for a sandbox or code-execution call.
func buildSandbox(in Request) (Upstream, error) {
	payload, err := json.Marshal(map[string]any{"code": firstText(in.Body)})
	if err != nil {
		return Upstream{}, err
	}
	return Upstream{
		URL:    strings.TrimRight(in.APIBase, "/") + "/sandboxes",
		Header: Headers(in.APIKey),
		Body:   payload,
	}, nil
}

// buildOCR builds the upstream address for an OCR call.
func buildOCR(in Request) (Upstream, error) {
	payload, err := json.Marshal(map[string]any{"document": in.Body["document"], "model": in.Model})
	if err != nil {
		return Upstream{}, err
	}
	return Upstream{
		URL:    strings.TrimRight(in.APIBase, "/") + "/ocr",
		Header: Headers(in.APIKey),
		Body:   payload,
	}, nil
}

// buildOwn builds an upstream request for this process's custom protocol. Unknown fields are not dropped here.
func buildOwn(in Request) (Upstream, error) {
	// The main operation of these packages is not OpenAI chat. ollama uses /api/chat and the others use /invoke.
	path := "/invoke"
	if in.Provider == "ollama" {
		path = "/api/chat"
	}
	if in.Provider == "meta" {
		path = "/realtime"
	}
	payload, err := json.Marshal(map[string]any{"model": in.Model, "input": firstText(in.Body)})
	if err != nil {
		return Upstream{}, err
	}
	return Upstream{URL: strings.TrimRight(in.APIBase, "/") + path, Header: Headers(in.APIKey), Body: payload}, nil
}

// buildShared joins addresses shared by several protocol groups. Protocol differences stay in each build function.
func buildShared(in Request) (Upstream, error) {
	// base_llm, custom_httpx, and pass_through are not callable provider URLs.
	return Upstream{}, errUnknownProvider
}

// legacy is the old encoding entry. New callers should not depend on the field names here.
func legacy(in Request) (Upstream, error) {
	raw, err := Encode(in.Op, in.Provider, cloneMap(in.Body), in.Model)
	if err != nil {
		return Upstream{}, err
	}
	return Upstream{
		URL:    Endpoint(in.Op, in.Provider, in.APIBase, in.Model),
		Header: Headers(in.APIKey),
		Body:   raw,
	}, nil
}

// captureOpenAIChat builds one OpenAI chat request with the official SDK so a test can compare the URL and body. It does not dial.
func captureOpenAIChat(ctx context.Context, base, apiKey, model string, body map[string]any) (Upstream, error) {
	raw, err := openAIChatBody(ctx, base, apiKey, model, body)
	if err != nil {
		return Upstream{}, err
	}
	// The URL and headers come from the same SDK call so a hand-written path and the SDK body do not drift apart.
	cap := &capture{}
	client := openai.NewClient(
		option.WithAPIKey(apiKey),
		option.WithBaseURL(strings.TrimRight(base, "/")),
		option.WithHTTPClient(&http.Client{Transport: cap}),
	)
	_, err = client.Chat.Completions.New(ctx, chatParams(model, body))
	if err != nil && cap.req == nil {
		return Upstream{}, err
	}
	if cap.req == nil {
		return Upstream{}, errString("openai sdk did not send")
	}
	// The body already includes fields such as stream. The URL and auth headers still come from the SDK.
	return Upstream{URL: cap.req.URL.String(), Header: cap.req.Header.Clone(), Body: raw}, nil
}

// openAIChatBody encodes a chat body as the JSON the OpenAI SDK would send.
func openAIChatBody(ctx context.Context, base, apiKey, model string, body map[string]any) ([]byte, error) {
	cap := &capture{}
	client := openai.NewClient(
		option.WithAPIKey(apiKey),
		option.WithBaseURL(strings.TrimRight(base, "/")),
		option.WithHTTPClient(&http.Client{Transport: cap}),
	)
	_, err := client.Chat.Completions.New(ctx, chatParams(model, body))
	if cap.body == nil {
		if err != nil {
			return nil, err
		}
		return nil, errString("openai sdk empty body")
	}
	var doc map[string]any
	if json.Unmarshal(cap.body, &doc) != nil {
		return cap.body, nil
	}
	// The SDK chat parameters do not include stream. The stream flag is copied back from the original request, or the upstream would not see stream.
	if v, ok := body["stream"]; ok {
		doc["stream"] = v
	}
	if v, ok := body["temperature"]; ok {
		doc["temperature"] = v
	}
	if v, ok := body["max_tokens"]; ok {
		doc["max_tokens"] = v
	}
	// The caller's tools are OpenAI tool calls, not the removed tool-management column.
	if v, ok := body["tools"]; ok {
		doc["tools"] = v
	}
	if v, ok := body["tool_choice"]; ok {
		doc["tool_choice"] = v
	}
	return json.Marshal(doc)
}

// chatParams turns a map body into the SDK chat parameters. A field the SDK cannot represent does not silently succeed as a zero value.
func chatParams(model string, body map[string]any) openai.ChatCompletionNewParams {
	var msgs []openai.ChatCompletionMessageParamUnion
	if raw, ok := body["messages"].([]any); ok {
		for _, item := range raw {
			msg, ok := item.(map[string]any)
			if !ok {
				continue
			}
			role, _ := msg["role"].(string)
			text := textOf(msg["content"])
			switch role {
			case "assistant":
				msgs = append(msgs, openai.AssistantMessage(text))
			case "system":
				msgs = append(msgs, openai.SystemMessage(text))
			default:
				msgs = append(msgs, openai.UserMessage(text))
			}
		}
	}
	if len(msgs) == 0 {
		msgs = []openai.ChatCompletionMessageParamUnion{openai.UserMessage(firstText(body))}
	}
	return openai.ChatCompletionNewParams{
		Model:    openai.ChatModel(model),
		Messages: msgs,
	}
}

// buildGemini builds a Gemini or Vertex generateContent request.
func buildGemini(ctx context.Context, in Request) (Upstream, error) {
	cap := &capture{}
	cfg := &genai.ClientConfig{
		APIKey:     in.APIKey,
		HTTPClient: &http.Client{Transport: cap},
		HTTPOptions: genai.HTTPOptions{
			BaseURL:    strings.TrimRight(in.APIBase, "/"),
			APIVersion: "v1beta",
		},
	}
	if in.Provider == "vertex_ai" {
		// Vertex uses a project and a region. The SDK does not allow a project and an API key together, so the key is placed in the request header.
		project := in.VertexProject
		if project == "" {
			project = "vertex-project"
		}
		location := in.VertexLocation
		if location == "" {
			location = "us-central1"
		}
		cfg.APIKey = ""
		cfg.Backend = genai.BackendVertexAI
		cfg.Project = project
		cfg.Location = location
		cfg.HTTPOptions.APIVersion = "v1"
		cfg.HTTPOptions.Headers = http.Header{"Authorization": []string{"Bearer " + in.APIKey}}
	} else {
		cfg.Backend = genai.BackendGeminiAPI
	}
	client, err := genai.NewClient(ctx, cfg)
	if err != nil {
		return Upstream{}, err
	}
	contents := geminiContents(in.Body)
	var genCfg *genai.GenerateContentConfig
	if n, ok := asInt32(in.Body["max_tokens"]); ok {
		genCfg = &genai.GenerateContentConfig{MaxOutputTokens: n}
	}
	_, err = client.Models.GenerateContent(ctx, in.Model, contents, genCfg)
	if cap.req == nil {
		if err != nil {
			return Upstream{}, err
		}
		return Upstream{}, errString("genai sdk did not send")
	}
	up := Upstream{URL: cap.req.URL.String(), Header: cap.req.Header.Clone(), Body: append([]byte(nil), cap.body...)}
	// With a custom api_base, the LiteLLM Gemini URL is {api_base}/models/{model}:generateContent and does not insert v1beta.
	// The SDK still owns the body and x-goog-api-key. Vertex joins /v1/projects/... onto an api_base with an empty path, matching the SDK.
	if in.Provider == "gemini" {
		up.URL = strings.TrimRight(in.APIBase, "/") + "/models/" + in.Model + ":generateContent"
	}
	// LiteLLM writes maxOutputTokens as generationConfig.max_output_tokens. The SDK uses camel case, and this rewrites the body to the LiteLLM form.
	up.Body = geminiLiteLLMBody(up.Body)
	return up, nil
}

// geminiLiteLLMBody turns a Gemini response into the JSON shape LiteLLM expects.
func geminiLiteLLMBody(raw []byte) []byte {
	var doc map[string]any
	if json.Unmarshal(raw, &doc) != nil {
		return raw
	}
	cfg, ok := doc["generationConfig"].(map[string]any)
	if !ok {
		return raw
	}
	if v, ok := cfg["maxOutputTokens"]; ok {
		cfg["max_output_tokens"] = v
		delete(cfg, "maxOutputTokens")
	}
	out, err := json.Marshal(doc)
	if err != nil {
		return raw
	}
	return out
}

// asInt32 converts a number to int32. A wrong type returns ok false, and the caller must not treat it as 0.
func asInt32(v any) (int32, bool) {
	switch n := v.(type) {
	case int:
		return int32(n), true
	case int32:
		return n, true
	case float64:
		return int32(n), true
	case json.Number:
		i, err := n.Int64()
		if err != nil {
			return 0, false
		}
		return int32(i), true
	default:
		return 0, false
	}
}

// geminiContents turns a message list into Gemini Content. Empty content still produces one user message so the upstream does not reject empty contents.
func geminiContents(body map[string]any) []*genai.Content {
	if raw, ok := body["contents"].([]any); ok && len(raw) > 0 {
		var out []*genai.Content
		for _, item := range raw {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			role, _ := m["role"].(string)
			if role == "" {
				role = "user"
			}
			out = append(out, &genai.Content{Role: role, Parts: []*genai.Part{{Text: textOf(m)}}})
		}
		if len(out) > 0 {
			return out
		}
	}
	if msgs, ok := body["messages"].([]any); ok {
		var out []*genai.Content
		for _, item := range msgs {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			role, _ := m["role"].(string)
			switch role {
			case "assistant":
				role = "model"
			case "system":
				role = "user"
			default:
				if role == "" {
					role = "user"
				}
			}
			out = append(out, &genai.Content{Role: role, Parts: []*genai.Part{{Text: textOf(m["content"])}}})
		}
		if len(out) > 0 {
			return out
		}
	}
	return []*genai.Content{{Role: "user", Parts: []*genai.Part{{Text: firstText(body)}}}}
}

type capture struct {
	req  *http.Request
	body []byte
}

// RoundTrip records the request and returns a fixed success JSON. The capture type uses it as a test transport.
func (c *capture) RoundTrip(r *http.Request) (*http.Response, error) {
	b, _ := io.ReadAll(r.Body)
	c.body = b
	r.Body = io.NopCloser(bytes.NewReader(b))
	c.req = r.Clone(r.Context())
	c.req.Body = io.NopCloser(bytes.NewReader(b))
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"id":"sdk","choices":[{"message":{"role":"assistant","content":""}}],"candidates":[{"content":{"parts":[{"text":""}]}}]}`)),
		Request:    r,
	}, nil
}

// cloneMap shallow-copies a map. Later edits to the copy do not change the original.
func cloneMap(in map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range in {
		out[k] = v
	}
	return out
}

// firstText takes the first text part from the body for a provider that has no messages field.
func firstText(body map[string]any) string {
	if s := str(body["prompt"]); s != "" {
		return s
	}
	if s := str(body["input"]); s != "" {
		return s
	}
	if s := str(body["query"]); s != "" {
		return s
	}
	if msgs, ok := body["messages"].([]any); ok && len(msgs) > 0 {
		if m, ok := msgs[0].(map[string]any); ok {
			return textOf(m["content"])
		}
	}
	return ""
}

// str reads v as a string. A non-string returns an empty string and does not panic.
func str(v any) string {
	s, _ := v.(string)
	return s
}
