// Package estimate counts tokens and lists the OpenAI parameters a model claims to support.
package estimate

import (
	"strings"

	"github.com/tiktoken-go/tokenizer"
)

// CountTokens matches the local LiteLLM token_counter for OpenAI chat models.
// A prompt is counted as plain text. Each message adds tokens_per_message, then the role and content, and the total adds 3 reply-priming tokens.
// The returned tokenizer type is the openai_tokenizer selected by LiteLLM.
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

// OpenAISupportedParams matches OpenAIGPTConfig.get_supported_openai_params.
// gpt-4 and gpt-3.5-turbo-16k omit response_format. OpenAI models listed in the catalog also include user.
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

// ModelUsedForCount drops the provider prefix from a deployment model, matching LiteLLM token_counter.
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
