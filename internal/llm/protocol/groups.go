// 供应商到协议组的对照。不在表里的名字返回空字符串和 false，数据面会跳过该部署，不按 OpenAI 兼容转发。
package protocol

// ProtocolGroup 是 LiteLLM 1.102.0 里 136 个 llms 包的协议归组。
// 分组按报文和鉴权差异，不按 Python 目录。
// 不在这张表里的名字不是已实现的供应商；空字符串也不是。
func ProtocolGroup(provider string) (string, bool) {
	switch provider {
	case "ai21":
		return "openai_byte_compatible", true
	case "aiohttp_openai":
		return "openai_byte_compatible", true
	case "amazon_nova":
		return "openai_byte_compatible", true
	case "baseten":
		return "openai_byte_compatible", true
	case "cerebras":
		return "openai_byte_compatible", true
	case "clarifai":
		return "openai_byte_compatible", true
	case "cloudflare":
		return "openai_byte_compatible", true
	case "codestral":
		return "openai_byte_compatible", true
	case "compactifai":
		return "openai_byte_compatible", true
	case "datarobot":
		return "openai_byte_compatible", true
	case "docker_model_runner":
		return "openai_byte_compatible", true
	case "empower":
		return "openai_byte_compatible", true
	case "featherless_ai":
		return "openai_byte_compatible", true
	case "friendliai":
		return "openai_byte_compatible", true
	case "galadriel":
		return "openai_byte_compatible", true
	case "gdc":
		return "openai_byte_compatible", true
	case "github":
		return "openai_byte_compatible", true
	case "gradient_ai":
		return "openai_byte_compatible", true
	case "heroku":
		return "openai_byte_compatible", true
	case "hyperbolic":
		return "openai_byte_compatible", true
	case "inception":
		return "openai_byte_compatible", true
	case "lambda_ai":
		return "openai_byte_compatible", true
	case "lemonade":
		return "openai_byte_compatible", true
	case "llamafile":
		return "openai_byte_compatible", true
	case "lm_studio":
		return "openai_byte_compatible", true
	case "meta_llama":
		return "openai_byte_compatible", true
	case "moonshot":
		return "openai_byte_compatible", true
	case "morph":
		return "openai_byte_compatible", true
	case "nebius":
		return "openai_byte_compatible", true
	case "novita":
		return "openai_byte_compatible", true
	case "nscale":
		return "openai_byte_compatible", true
	case "oobabooga":
		return "openai_byte_compatible", true
	case "v0":
		return "openai_byte_compatible", true
	case "wandb":
		return "openai_byte_compatible", true
	case "zai":
		return "openai_byte_compatible", true
	case "openai":
		return "openai_native", true
	case "aiml":
		return "openai_delta", true
	case "chatgpt":
		return "openai_delta", true
	case "cometapi":
		return "openai_delta", true
	case "databricks":
		return "openai_delta", true
	case "deepinfra":
		return "openai_delta", true
	case "deepseek":
		return "openai_delta", true
	case "fireworks_ai":
		return "openai_delta", true
	case "github_copilot":
		return "openai_delta", true
	case "groq":
		return "openai_delta", true
	case "hosted_vllm":
		return "openai_delta", true
	case "huggingface":
		return "openai_delta", true
	case "litellm_proxy":
		return "openai_delta", true
	case "manus":
		return "openai_delta", true
	case "minimax":
		return "openai_delta", true
	case "mistral":
		return "openai_delta", true
	case "modelscope":
		return "openai_delta", true
	case "nvidia_nim":
		return "openai_delta", true
	case "openai_like":
		return "openai_delta", true
	case "openrouter":
		return "openai_delta", true
	case "ovhcloud":
		return "openai_delta", true
	case "perplexity":
		return "openai_delta", true
	case "sambanova":
		return "openai_delta", true
	case "sap":
		return "openai_delta", true
	case "snowflake":
		return "openai_delta", true
	case "tencent":
		return "openai_delta", true
	case "together_ai":
		return "openai_delta", true
	case "vercel_ai_gateway":
		return "openai_delta", true
	case "vllm":
		return "openai_delta", true
	case "volcengine":
		return "openai_delta", true
	case "watsonx":
		return "openai_delta", true
	case "xai":
		return "openai_delta", true
	case "azure":
		return "azure_openai", true
	case "azure_ai":
		return "azure_ai_foundry", true
	case "anthropic":
		return "anthropic_messages", true
	case "gemini":
		return "gemini_genai", true
	case "vertex_ai":
		return "gemini_genai", true
	case "bedrock":
		return "bedrock_aws", true
	case "bedrock_mantle":
		return "bedrock_aws", true
	case "sagemaker":
		return "bedrock_aws", true
	case "aws_polly":
		return "bedrock_aws", true
	case "s3_vectors":
		return "bedrock_aws", true
	case "cohere":
		return "cohere_http", true
	case "apiserpent":
		return "search_http", true
	case "brave":
		return "search_http", true
	case "dataforseo":
		return "search_http", true
	case "duckduckgo":
		return "search_http", true
	case "exa_ai":
		return "search_http", true
	case "fastcrw":
		return "search_http", true
	case "firecrawl":
		return "search_http", true
	case "google_pse":
		return "search_http", true
	case "linkup":
		return "search_http", true
	case "nimble":
		return "search_http", true
	case "parallel_ai":
		return "search_http", true
	case "searchapi":
		return "search_http", true
	case "searxng":
		return "search_http", true
	case "serper":
		return "search_http", true
	case "tavily":
		return "search_http", true
	case "tinyfish":
		return "search_http", true
	case "you_com":
		return "search_http", true
	case "black_forest_labs":
		return "image_media_http", true
	case "fal_ai":
		return "image_media_http", true
	case "recraft":
		return "image_media_http", true
	case "runwayml":
		return "image_media_http", true
	case "stability":
		return "image_media_http", true
	case "topaz":
		return "image_media_http", true
	case "xinference":
		return "image_media_http", true
	case "deepgram":
		return "audio_http", true
	case "elevenlabs":
		return "audio_http", true
	case "nvidia_riva":
		return "audio_http", true
	case "scaleway":
		return "audio_http", true
	case "soniox":
		return "audio_http", true
	case "dashscope":
		return "embed_rerank_http", true
	case "infinity":
		return "embed_rerank_http", true
	case "jina_ai":
		return "embed_rerank_http", true
	case "voyage":
		return "embed_rerank_http", true
	case "milvus":
		return "vector_store", true
	case "mongodb":
		return "vector_store", true
	case "pg_vector":
		return "vector_store", true
	case "ragflow":
		return "vector_store", true
	case "valkey":
		return "vector_store", true
	case "e2b":
		return "sandbox", true
	case "opensandbox":
		return "sandbox", true
	case "reducto":
		return "ocr_http", true
	case "a2a":
		return "own_protocol_http", true
	case "bytez":
		return "own_protocol_http", true
	case "gigachat":
		return "own_protocol_http", true
	case "langflow":
		return "own_protocol_http", true
	case "langgraph":
		return "own_protocol_http", true
	case "meta":
		return "own_protocol_http", true
	case "nlp_cloud":
		return "own_protocol_http", true
	case "oci":
		return "own_protocol_http", true
	case "ollama":
		return "own_protocol_http", true
	case "petals":
		return "own_protocol_http", true
	case "predibase":
		return "own_protocol_http", true
	case "replicate":
		return "own_protocol_http", true
	case "triton":
		return "own_protocol_http", true
	case "base_llm":
		return "shared_runtime", true
	case "custom_httpx":
		return "shared_runtime", true
	case "pass_through":
		return "shared_runtime", true
	case "deprecated_providers":
		return "shared_runtime", true
	default:
		return "", false
	}
}
