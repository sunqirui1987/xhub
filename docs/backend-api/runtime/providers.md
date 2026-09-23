# Provider

每个 `llms` 包必须二选一：**已实现适配器**（状态 `adapter`），或运行时返回 **`provider_not_implemented`**。禁止静默按 OpenAI 兼容协议转发。

## URL 规则（adapter 必须遵守）

| Provider | Operation | URL |
|---|---|---|
| openai | chat | `{api_base}/chat/completions` |
| azure | chat | `{api_base}/openai/deployments/{model}/chat/completions` |
| azure | responses | `{api_base}/openai/responses`（默认 version `preview`） |
| anthropic | messages | `{api_base}/v1/messages` |
| gemini | generateContent | `/v1beta/models/{model}:generateContent` |

## 包目录

| package | 状态 |
|---|---|
| `a2a` | `provider_not_implemented` |
| `ai21` | `provider_not_implemented` |
| `aiml` | `provider_not_implemented` |
| `aiohttp_openai` | `provider_not_implemented` |
| `amazon_nova` | `provider_not_implemented` |
| `anthropic` | `adapter` |
| `apiserpent` | `provider_not_implemented` |
| `aws_polly` | `provider_not_implemented` |
| `azure` | `adapter` |
| `azure_ai` | `provider_not_implemented` |
| `base_llm` | `provider_not_implemented` |
| `baseten` | `provider_not_implemented` |
| `bedrock` | `provider_not_implemented` |
| `bedrock_mantle` | `provider_not_implemented` |
| `black_forest_labs` | `provider_not_implemented` |
| `brave` | `provider_not_implemented` |
| `bytez` | `provider_not_implemented` |
| `cerebras` | `provider_not_implemented` |
| `chatgpt` | `provider_not_implemented` |
| `clarifai` | `provider_not_implemented` |
| `cloudflare` | `provider_not_implemented` |
| `codestral` | `provider_not_implemented` |
| `cohere` | `provider_not_implemented` |
| `cometapi` | `provider_not_implemented` |
| `compactifai` | `provider_not_implemented` |
| `custom_httpx` | `provider_not_implemented` |
| `dashscope` | `provider_not_implemented` |
| `databricks` | `provider_not_implemented` |
| `dataforseo` | `provider_not_implemented` |
| `datarobot` | `provider_not_implemented` |
| `deepgram` | `provider_not_implemented` |
| `deepinfra` | `provider_not_implemented` |
| `deepseek` | `provider_not_implemented` |
| `deprecated_providers` | `provider_not_implemented` |
| `docker_model_runner` | `provider_not_implemented` |
| `duckduckgo` | `provider_not_implemented` |
| `e2b` | `provider_not_implemented` |
| `elevenlabs` | `provider_not_implemented` |
| `empower` | `provider_not_implemented` |
| `exa_ai` | `provider_not_implemented` |
| `fal_ai` | `provider_not_implemented` |
| `fastcrw` | `provider_not_implemented` |
| `featherless_ai` | `provider_not_implemented` |
| `firecrawl` | `provider_not_implemented` |
| `fireworks_ai` | `provider_not_implemented` |
| `friendliai` | `provider_not_implemented` |
| `galadriel` | `provider_not_implemented` |
| `gdc` | `provider_not_implemented` |
| `gemini` | `adapter` |
| `gigachat` | `provider_not_implemented` |
| `github` | `provider_not_implemented` |
| `github_copilot` | `provider_not_implemented` |
| `google_pse` | `provider_not_implemented` |
| `gradient_ai` | `provider_not_implemented` |
| `groq` | `provider_not_implemented` |
| `heroku` | `provider_not_implemented` |
| `hosted_vllm` | `provider_not_implemented` |
| `huggingface` | `provider_not_implemented` |
| `hyperbolic` | `provider_not_implemented` |
| `inception` | `provider_not_implemented` |
| `infinity` | `provider_not_implemented` |
| `jina_ai` | `provider_not_implemented` |
| `lambda_ai` | `provider_not_implemented` |
| `langflow` | `provider_not_implemented` |
| `langgraph` | `provider_not_implemented` |
| `lemonade` | `provider_not_implemented` |
| `linkup` | `provider_not_implemented` |
| `litellm_proxy` | `provider_not_implemented` |
| `llamafile` | `provider_not_implemented` |
| `lm_studio` | `provider_not_implemented` |
| `manus` | `provider_not_implemented` |
| `meta` | `provider_not_implemented` |
| `meta_llama` | `provider_not_implemented` |
| `milvus` | `provider_not_implemented` |
| `minimax` | `provider_not_implemented` |
| `mistral` | `provider_not_implemented` |
| `modelscope` | `provider_not_implemented` |
| `mongodb` | `provider_not_implemented` |
| `moonshot` | `provider_not_implemented` |
| `morph` | `provider_not_implemented` |
| `nebius` | `provider_not_implemented` |
| `nimble` | `provider_not_implemented` |
| `nlp_cloud` | `provider_not_implemented` |
| `novita` | `provider_not_implemented` |
| `nscale` | `provider_not_implemented` |
| `nvidia_nim` | `provider_not_implemented` |
| `nvidia_riva` | `provider_not_implemented` |
| `oci` | `provider_not_implemented` |
| `ollama` | `provider_not_implemented` |
| `oobabooga` | `provider_not_implemented` |
| `openai` | `adapter` |
| `openai_like` | `provider_not_implemented` |
| `openrouter` | `provider_not_implemented` |
| `opensandbox` | `provider_not_implemented` |
| `ovhcloud` | `provider_not_implemented` |
| `parallel_ai` | `provider_not_implemented` |
| `pass_through` | `provider_not_implemented` |
| `perplexity` | `provider_not_implemented` |
| `petals` | `provider_not_implemented` |
| `pg_vector` | `provider_not_implemented` |
| `predibase` | `provider_not_implemented` |
| `ragflow` | `provider_not_implemented` |
| `recraft` | `provider_not_implemented` |
| `reducto` | `provider_not_implemented` |
| `replicate` | `provider_not_implemented` |
| `runwayml` | `provider_not_implemented` |
| `s3_vectors` | `provider_not_implemented` |
| `sagemaker` | `provider_not_implemented` |
| `sambanova` | `provider_not_implemented` |
| `sap` | `provider_not_implemented` |
| `scaleway` | `provider_not_implemented` |
| `searchapi` | `provider_not_implemented` |
| `searxng` | `provider_not_implemented` |
| `serper` | `provider_not_implemented` |
| `snowflake` | `provider_not_implemented` |
| `soniox` | `provider_not_implemented` |
| `stability` | `provider_not_implemented` |
| `tavily` | `provider_not_implemented` |
| `tencent` | `provider_not_implemented` |
| `tinyfish` | `provider_not_implemented` |
| `together_ai` | `provider_not_implemented` |
| `topaz` | `provider_not_implemented` |
| `triton` | `provider_not_implemented` |
| `v0` | `provider_not_implemented` |
| `valkey` | `provider_not_implemented` |
| `vercel_ai_gateway` | `provider_not_implemented` |
| `vertex_ai` | `adapter` |
| `vllm` | `provider_not_implemented` |
| `volcengine` | `provider_not_implemented` |
| `voyage` | `provider_not_implemented` |
| `wandb` | `provider_not_implemented` |
| `watsonx` | `provider_not_implemented` |
| `xai` | `provider_not_implemented` |
| `xinference` | `provider_not_implemented` |
| `you_com` | `provider_not_implemented` |
| `zai` | `provider_not_implemented` |
