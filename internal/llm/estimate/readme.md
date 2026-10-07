# llm/estimate

Local token estimates and the OpenAI parameter list a model claims to support. `gateway/tokens.go` calls this for `GET` token-count utilities. `dataplane.EstimateTokens` is a separate, smaller upper bound used before the upstream call for budget and TPM. Do not treat the two numbers as the same counter.

`CountTokens(model, prompt, messages)` follows the local LiteLLM `token_counter` for OpenAI chat. A non-empty prompt, or a nil message list, is counted as plain text. Otherwise each message adds `tokens_per_message`, then the role and the content, and the total adds 3 reply-priming tokens. The tokenizer is `tokenizer.ForModel`. An unknown model name falls back to `cl100k_base`. The returned kind string is `openai_tokenizer` on the success path. A tokenizer that cannot be loaded returns `0, "", err`.

`ModelUsedForCount` strips one provider prefix from a deployment model id so `openai/gpt-4o-mini` is counted as `gpt-4o-mini`. `gateway/tokens.go` `modelUsedForCount` uses the deployment's `litellm_params.model` when the request model matches a deployment, and otherwise the request model itself.

`OpenAISupportedParams` lists the parameters `GET /utils/supported_openai_params` returns. `gpt-4` and `gpt-3.5-turbo-16k` omit `response_format`. Models that appear in the catalog also include `user`.

## What this package does not do

It does not read the price map and it does not multiply by dollars. That is `catalog.Cost`. It does not count tool-call argument text inside a finished completion; output tokens for a stored call are filled in `dataplane/usage.go` from the upstream usage object or from streamed text.

中文说明见同目录 `readme_cn.md`。
