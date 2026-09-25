// 估算 token，并列出某个模型声明支持的 OpenAI 参数。
package estimate

import (
	"strings"

	"github.com/tiktoken-go/tokenizer"
)

// CountTokens 对齐 LiteLLM token_counter 对 OpenAI 聊天模型的本地计数。
// prompt 走纯文本。messages 每条先加 tokens_per_message，再数 role 和 content，最后加 3 个回复起始 token。
// tokenizer_type 与 LiteLLM _select_tokenizer 的 openai_tokenizer 相同。
func CountTokens(model, prompt string, messages []map[string]any) (int, string, error) {
	codec, err := tokenizer.ForModel(tokenizer.Model(model))
	if err != nil {
		codec, err = tokenizer.Get(tokenizer.Cl100kBase)
		if err != nil {
			return 0, "", err
		}
	}
	count := func(text string) int {
		n, err := codec.Count(text)
		if err != nil {
			return 0
		}
		return n
	}
	if prompt != "" || messages == nil {
		return count(prompt), "openai_tokenizer", nil
	}
	n := 0
	for _, msg := range messages {
		n += 3
		if role, ok := msg["role"].(string); ok {
			n += count(role)
		}
		if content, ok := msg["content"].(string); ok {
			n += count(content)
		}
		if name, ok := msg["name"].(string); ok && name != "" {
			n += count(name) + 1
		}
	}
	n += 3
	return n, "openai_tokenizer", nil
}

// OpenAISupportedParams 对齐 OpenAIGPTConfig.get_supported_openai_params。
// gpt-4 与 gpt-3.5-turbo-16k 没有 response_format。目录里的 OpenAI 模型多一个 user。
func OpenAISupportedParams(model string, catalog bool) []string {
	params := []string{
		"frequency_penalty",
		"logit_bias",
		"logprobs",
		"top_logprobs",
		"max_tokens",
		"max_completion_tokens",
		"modalities",
		"prediction",
		"n",
		"presence_penalty",
		"seed",
		"stop",
		"stream",
		"stream_options",
		"temperature",
		"top_p",
		"tools",
		"tool_choice",
		"function_call",
		"functions",
		"max_retries",
		"extra_headers",
		"parallel_tool_calls",
		"audio",
		"web_search_options",
		"service_tier",
		"safety_identifier",
		"prompt_cache_key",
		"prompt_cache_retention",
		"store",
	}
	if model != "gpt-3.5-turbo-16k" && model != "gpt-4" {
		params = append(params, "response_format")
	}
	if catalog {
		params = append(params, "user")
	}
	return params
}

// ModelUsedForCount 在部署的 model 带供应商前缀时去掉第一段，和 LiteLLM token_counter 一样。
func ModelUsedForCount(requestModel, deploymentModel string) string {
	model := strings.TrimSpace(deploymentModel)
	if model == "" {
		return requestModel
	}
	if i := strings.Index(model, "/"); i > 0 {
		return model[i+1:]
	}
	return model
}
