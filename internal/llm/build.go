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

// Request 是一次上游调用要决定的全部输入。
//
// Op 与数据面的操作名一致（chat、embeddings、images…）。
// Provider 必须是 ProtocolGroup 里的 136 个包名之一。
// APIBase 与 APIKey 已经由凭证层填好，本函数不把空地址改成官方域名。
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

// Upstream 是发给供应商的 HTTP 请求，还没有拨号。
//
// URL、Header、Body 三者一起构成契约。只改其中一段（例如只换 Base URL、
// 正文仍是 OpenAI chat）对改过鉴权或报文的供应商不算对齐。
type Upstream struct {
	URL    string
	Header http.Header
	Body   []byte
}

// Build 按协议组生成上游请求。
//
// OpenAI 与只改 Base URL、Bearer 密钥的供应商走 github.com/openai/openai-go。
// Gemini 与 Vertex 的 generateContent 走 google.golang.org/genai。
// Azure 仍用 OpenAI SDK 的 JSON，但 URL 是部署路径，鉴权头是 api-key。
// Anthropic、Cohere、Bedrock 各自有路径和正文，不投稿到 /chat/completions。
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
		// openai_byte_compatible、openai_native、openai_delta。
		return buildOpenAIWire(ctx, in)
	}
}

var (
	errUnknownProvider    = errString("provider_not_implemented")
	errDeprecatedProvider = errString("deprecated_provider")
)

type errString string

func (e errString) Error() string { return string(e) }

// ChatProbeUsesFixture 表示标准测试上游能回答这次聊天。
//
// 这些供应商的聊天 URL 以 /chat/completions、/v1/messages 或 :generateContent 结尾。
// 其余协议组必须看 Build 的 URL 和正文，不能靠同一条 OpenAI 路径判成功。
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

func buildAzure(ctx context.Context, in Request) (Upstream, error) {
	// Azure OpenAI 的路径把模型放进部署名，鉴权是 api-key，不是 Bearer 打到 /chat/completions。
	body, err := openAIChatBody(ctx, in.APIBase, in.APIKey, in.Model, in.Body)
	if err != nil {
		return Upstream{}, err
	}
	h := http.Header{}
	h.Set("api-key", in.APIKey)
	h.Set("Content-Type", "application/json")
	// LiteLLM 1.102.0 的 AZURE_DEFAULT_API_VERSION。部署路径之外必须带这个查询参数。
	u := Endpoint(in.Op, "azure", in.APIBase, in.Model)
	if !strings.Contains(u, "api-version=") {
		u += "?api-version=2025-02-01-preview"
	}
	return Upstream{URL: u, Header: h, Body: body}, nil
}

func buildAzureAI(ctx context.Context, in Request) (Upstream, error) {
	// Foundry 不是公有 Azure 部署路径。聊天走 /openai/v1/chat/completions，并带 azure-ai 头。
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

func buildAnthropic(in Request) (Upstream, error) {
	// Messages API：路径 {api_base}/v1/messages。
	// LiteLLM 把字符串 content 收成 [{type:text,text}]，头是 x-api-key 与 anthropic-version: 2023-06-01。
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

// RealtimeClientSecretsURL 是 LiteLLM OpenAIRealtimeHTTPConfig.get_complete_url。
// 自定义 api_base 若以 /v1 结尾，先去掉再接上 /v1/realtime/client_secrets。
func RealtimeClientSecretsURL(apiBase string) string {
	base := strings.TrimRight(apiBase, "/")
	base = strings.TrimSuffix(base, "/v1")
	return base + "/v1/realtime/client_secrets"
}

func buildCohere(in Request) (Upstream, error) {
	if in.Op == OpRerank {
		return buildCohereRerank(in)
	}
	// LiteLLM 的 Cohere v2 在调用方给了 api_base 时，把这个地址当作完整 URL，不再追加 /v2/chat。
	// 正文是 messages 数组，另加 request-source。未提供 api_base 时才用官方 https://api.cohere.com/v2/chat。
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

// buildCohereRerank 对齐 LiteLLM CohereRerankV2Config。
// 给了 api_base 且不以 /v2/rerank 结尾时接上这段路径；没给时用 https://api.cohere.ai/v2/rerank。
// 未传 return_documents 时 LiteLLM 默认 true。鉴权只有 Bearer，没有聊天那条 request-source。
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

func buildBedrock(in Request) (Upstream, error) {
	// Bedrock InvokeModel 用模型 ID 放进路径，并用 AWS SigV4 头，而不是 Bearer + /chat/completions。
	// 这里组装签名头的凭证范围；不连接 AWS。
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

func buildSearch(in Request) (Upstream, error) {
	// 搜索供应商收 query，不收 chat messages。
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

func buildEmbedHost(in Request) (Upstream, error) {
	// DashScope / Jina / Voyage 的嵌入路径不是 OpenAI /embeddings。
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

func buildOwn(in Request) (Upstream, error) {
	// 这些包的主操作不是 OpenAI Chat。ollama 用 /api/chat，其余用 /invoke。
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

func buildShared(in Request) (Upstream, error) {
	// base_llm / custom_httpx / pass_through 不是可调用的供应商 URL。
	return Upstream{}, errUnknownProvider
}

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

func captureOpenAIChat(ctx context.Context, base, apiKey, model string, body map[string]any) (Upstream, error) {
	raw, err := openAIChatBody(ctx, base, apiKey, model, body)
	if err != nil {
		return Upstream{}, err
	}
	// URL 与头也从同一次 SDK 调用取出，避免手写路径和 SDK 正文各走各的。
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
	// 用已经叠过 stream 等字段的正文，URL 和鉴权头仍以 SDK 为准。
	return Upstream{URL: cap.req.URL.String(), Header: cap.req.Header.Clone(), Body: raw}, nil
}

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
	// SDK 的聊天参数不包含 stream。流式开关从原请求叠回去，否则上游看不到 stream。
	if v, ok := body["stream"]; ok {
		doc["stream"] = v
	}
	if v, ok := body["temperature"]; ok {
		doc["temperature"] = v
	}
	if v, ok := body["max_tokens"]; ok {
		doc["max_tokens"] = v
	}
	// 调用方的 tools 是 OpenAI 工具调用，不是已经去掉的工具管理栏目。
	if v, ok := body["tools"]; ok {
		doc["tools"] = v
	}
	if v, ok := body["tool_choice"]; ok {
		doc["tool_choice"] = v
	}
	return json.Marshal(doc)
}

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
		// Vertex 用项目和区域。SDK 不允许项目和 API key 同时出现，密钥放在请求头里。
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
	// 自定义 api_base 时，LiteLLM 的 Gemini URL 是 {api_base}/models/{model}:generateContent，不插入 v1beta。
	// SDK 仍负责正文和 x-goog-api-key。Vertex 在空路径的 api_base 上接 /v1/projects/...，与 SDK 一致。
	if in.Provider == "gemini" {
		up.URL = strings.TrimRight(in.APIBase, "/") + "/models/" + in.Model + ":generateContent"
	}
	// LiteLLM 把 maxOutputTokens 写成 generationConfig.max_output_tokens。SDK 用驼峰，这里改成 LiteLLM 的正文。
	up.Body = geminiLiteLLMBody(up.Body)
	return up, nil
}

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

func cloneMap(in map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range in {
		out[k] = v
	}
	return out
}

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

func str(v any) string {
	s, _ := v.(string)
	return s
}
