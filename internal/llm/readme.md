# llm

Turns one public request into the bytes an upstream expects, and turns the upstream bytes back into the public shape. The data plane calls this package. This package does not pick a deployment and does not listen.

## Address

`DefaultAPIBase` is used only when the deployment left `api_base` empty. `openai` is `https://api.openai.com/v1`. `anthropic` is `https://api.anthropic.com`. An unknown provider returns an empty string. `Endpoint` does not fill that in. An empty address stays empty so an unconfigured deployment cannot quietly call the official host. The caller should fail authentication instead.

`Endpoint(op, provider, apiBase, model)` trims a trailing slash and joins the operation path. OpenAI-compatible providers share `/chat/completions`, `/embeddings`, and the other default branches. Azure inserts `/openai/deployments/<model>`. Anthropic chat is `{base}/v1/messages`. The rules follow each provider's `get_complete_url`, folded into this one function.

`PassthroughURL` joins a subpath onto an `api_base` the way LiteLLM `_join_url_paths` does, and rejects `..`. `PassthroughSubpath` returns `base` unchanged when `include` is false or the subpath is empty.

## Body

`Encode` writes the upstream body for an operation. The public `model` is the routing alias, so it is overwritten with the deployment's real model name. Other fields are kept. Anthropic Messages fills `max_tokens` with 256 when it is missing, because that API returns 400 without it. `StripProxyParams` deletes LiteLLM proxy fields (`litellm.utils.filter_out_litellm_params`) before the provider sees the body.

`Decode` puts the caller's alias back into `model`. Audio bytes have no JSON model field and are returned unchanged. Image, rerank, and transcription model fields belong to the result and are not overwritten. A Messages operation then turns a chat completion into an Anthropic message.

`Hydrate` copies credential values onto a deployment map. A blank credential field does not overwrite a value already set. `api_key` is the exception: a non-blank credential key replaces a masked `*****` left by the console. Strings that start with `os.environ/` expand through `os.Getenv` at hydrate time, not when the credential is saved. A missing variable becomes empty. `custom_llm_provider` is lowercased.

`Build` is the chat encoder used by `Serve`: OpenAI wire, Anthropic Messages (`x-api-key`, `anthropic-version: 2023-06-01`), Gemini, Cohere, and the other protocol groups. `RealtimeClientSecretsURL` trims a trailing slash and a trailing `/v1`, then appends `/v1/realtime/client_secrets`.

## What this package does not do

It does not bill, cache, or pin sessions. It does not register the Qiniu or Volcengine contents paths. Those stay on `ServeBypass` and are forwarded with the model field replaced, not re-encoded as chat.

中文说明见同目录 `readme_cn.md`。
