# llm/estimate

本地的 token 估算，以及一个模型声称支持的 OpenAI 参数列表。`gateway/tokens.go` 在 token 计数工具接口上调用它。`dataplane.EstimateTokens` 是上游调用前用来做预算和 TPM 的另一套更小的上界。不要把这两个数当成同一个计数器。

`CountTokens(model, prompt, messages)` 按本地 LiteLLM `token_counter` 的规则计 OpenAI 聊天。提示非空，或者消息列表是 nil，就按纯文本计。否则每条消息加上 `tokens_per_message`，再加上 role 和 content，总数再加 3 个回复起始 token。分词器是 `tokenizer.ForModel`。不认识的模型名退回 `cl100k_base`。成功路径返回的种类字符串是 `openai_tokenizer`。分词器装不上时返回 `0, "", err`。

`ModelUsedForCount` 从部署模型 id 上剥掉一层供应商前缀，所以 `openai/gpt-4o-mini` 按 `gpt-4o-mini` 计。`gateway/tokens.go` 的 `modelUsedForCount` 在请求模型对上某条部署时用该部署的 `litellm_params.model`，否则用请求里的模型名。

`OpenAISupportedParams` 是 `GET /utils/supported_openai_params` 返回的参数列表。`gpt-4` 和 `gpt-3.5-turbo-16k` 不含 `response_format`。出现在目录里的模型还会带上 `user`。

## 这个包不做什么

它不读价格表，也不乘美元。那是 `catalog.Cost`。它也不把一次完成里的工具调用参数文本算进输出 token；已存储调用的输出 token 由 `dataplane/usage.go` 按上游 usage 对象或流式文本补上。

English notes are in `readme.md` in this directory.
