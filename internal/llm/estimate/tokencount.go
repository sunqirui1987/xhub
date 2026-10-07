// Package estimate counts tokens and lists the OpenAI parameters a model claims to support.
package estimate

import (
	"strings"

	"github.com/sunqirui1987/xhub/internal/logx"
	"github.com/tiktoken-go/tokenizer"
	"sync"
)

var logTraceOnceTokencount sync.Once

// CountTokens matches the local LiteLLM token_counter for OpenAI chat models. A prompt is counted as plain text. Each message adds tokens_per_message, then the role and content, and the total adds 3 reply-priming tokens. The returned tokenizer type is the openai_tokenizer selected by LiteLLM.
// 参数 model（string）：发给上游或对外展示的模型名；prompt（string）：提示 token 数，用来估价；messages（[]map[string]any）：计数令牌使用的map[string]any。
// 返回 int（int）：估算的 token 数，供预算和 TPM 预检；string（string）：实际参与计数的模型名，已去掉供应商标前缀；error（error）：失败原因。nil 表示计数完成。
// 调用：gateway/tokens.go
// 测试：无直接单测
func CountTokens(model, prompt string, messages []map[string]any) (int, string, error) {
	logTraceOnceTokencount.Do(func() { logx.Trace("enter estimate.CountTokens") })

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

// OpenAISupportedParams matches OpenAIGPTConfig.get_supported_openai_params. gpt-4 and gpt-3.5-turbo-1 6k omit response_format. OpenAI models listed in the catalog also include user.
// 参数 model（string）：对外模型名，用来选部署和记用量；catalog（bool）：为真时走目录这一支。为假时保持原来的路径。
// 返回 []string（[]string）：打开AISupported参数。没有匹配时为空切片。
// 调用：gateway/tokens.go
// 测试：无直接单测
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
// 参数 requestModel（string）：模型名。对外名用来选部署，上游名写进转发正文；deploymentModel（string）：模型名。对外名用来选部署，上游名写进转发正文。
// 返回 string（string）：去掉供应商标前缀后的模型名，和本地 token 计数一致。
// 调用：gateway/tokens.go
// 测试：无直接单测
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
