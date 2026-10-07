# llm

把一次公开请求变成上游要的字节，再把上游字节变回公开形状。数据面调用这个包。这个包不选部署，也不监听。

## 地址

只有部署没写 `api_base` 时才用 `DefaultAPIBase`。`openai` 是 `https://api.openai.com/v1`。`anthropic` 是 `https://api.anthropic.com`。不认识的供应商返回空串。`Endpoint` 不会补上这个默认值。空地址保持为空，这样没配好的部署不会悄悄去打官方主机。调用方应该改成认证失败。

`Endpoint(op, provider, apiBase, model)` 去掉末尾斜杠再拼操作路径。OpenAI 兼容的供应商共用 `/chat/completions`、`/embeddings` 和其他默认分支。Azure 插入 `/openai/deployments/<model>`。Anthropic 聊天是 `{base}/v1/messages`。规则折进这一个函数，对应各供应商的 `get_complete_url`。

`PassthroughURL` 按 LiteLLM `_join_url_paths` 把子路径接到 `api_base` 上，并拒绝 `..`。`include` 为假或子路径为空时，`PassthroughSubpath` 原样返回 `base`。

## 正文

`Encode` 写出某个操作的上游正文。公开的 `model` 是路由用的别名，所以会改成部署上的真实模型名。其他字段保留。Anthropic Messages 在缺少 `max_tokens` 时填 256，因为该接口没有这个字段会返回 400。`StripProxyParams` 在供应商看到正文之前删掉 LiteLLM 代理字段（`litellm.utils.filter_out_litellm_params`）。

`Decode` 把调用方的别名写回 `model`。音频字节没有 JSON 模型字段，原样返回。图像、重排和转写结果上的模型字段属于结果本身，不覆盖。Messages 操作再把聊天补全收成 Anthropic 消息。

`Hydrate` 把凭据值抄到部署的参数表上。空白的凭据字段不会覆盖已经写好的值。`api_key` 是例外：凭据上非空的密钥会替换控制台留下的掩码 `*****`。以 `os.environ/` 开头的字符串在灌入时用 `os.Getenv` 展开，不是在保存凭据时展开。变量不存在则变成空串。`custom_llm_provider` 会转成小写。

`Build` 是 `Serve` 用的聊天编码器：OpenAI 线路、Anthropic Messages（`x-api-key`、`anthropic-version: 2023-06-01`）、Gemini、Cohere，以及其他协议组。`RealtimeClientSecretsURL` 去掉末尾斜杠和末尾 `/v1`，再接上 `/v1/realtime/client_secrets`。

## 这个包不做什么

它不记账、不缓存、不钉会话。它不登记七牛或火山的内容生成路径。那些留在 `ServeBypass`，只替换模型字段后原样转发，不会重编码成聊天。

English notes are in `readme.md` in this directory.
