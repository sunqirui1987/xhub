package server

import (
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"time"
)

// LiteLLM 1.102.0 model_prices_and_context_window.json, parsed once.
// Wildcard expansion (openai/*) reads the same per-provider sets LiteLLM
// builds in litellm._populate_provider_model_sets / models_by_provider.
var (
	modelCostMapValue    any
	modelCostMapLoadedAt string
	modelsByProvider     map[string][]string
)

var bedrockPricingOnly = regexp.MustCompile(`^bedrock/[a-zA-Z0-9_-]+/.+$`)

func init() {
	loadModelCatalog()
	modelCostMapLoadedAt = time.Now().UTC().Format(time.RFC3339)
}

func loadModelCatalog() {
	var raw map[string]any
	if err := json.Unmarshal(modelCostMapJSON, &raw); err != nil {
		modelCostMapValue = map[string]any{}
		modelsByProvider = map[string][]string{}
		return
	}
	modelCostMapValue = raw
	sets := map[string][]string{}
	add := func(set, id string) {
		sets[set] = append(sets[set], id)
	}
	for key, v := range raw {
		info, ok := v.(map[string]any)
		if !ok {
			continue
		}
		prov, _ := info["litellm_provider"].(string)
		mode, _ := info["mode"].(string)
		switch prov {
		case "openai":
			if !isOpenAIFinetuneModel(key) {
				add("openai_chat", key)
			}
		case "text-completion-openai":
			add("openai_text", key)
		case "azure_text":
			add("azure_text", key)
		case "cohere":
			add("cohere", key)
		case "cohere_chat":
			add("cohere_chat", key)
		case "mistral":
			add("mistral", key)
		case "anthropic":
			add("anthropic", key)
		case "openrouter":
			add("openrouter", key)
		case "vercel_ai_gateway":
			add("vercel_ai_gateway", key)
		case "datarobot":
			add("datarobot", key)
		case "vertex_ai-text-models":
			add("vertex_text", key)
		case "vertex_ai-code-text-models":
			add("vertex_code_text", key)
		case "vertex_ai-language-models":
			add("vertex_language", key)
		case "vertex_ai-vision-models":
			add("vertex_vision", key)
		case "vertex_ai-chat-models":
			add("vertex_chat", key)
		case "vertex_ai-code-chat-models":
			add("vertex_code_chat", key)
		case "vertex_ai-embedding-models":
			add("vertex_embedding", key)
		case "vertex_ai-anthropic_models":
			add("vertex_anthropic", strings.ReplaceAll(key, "vertex_ai/", ""))
		case "vertex_ai-llama_models":
			add("vertex_llama", strings.ReplaceAll(key, "vertex_ai/", ""))
		case "vertex_ai-deepseek_models":
			add("vertex_deepseek", strings.ReplaceAll(key, "vertex_ai/", ""))
		case "vertex_ai-mistral_models":
			add("vertex_mistral", strings.ReplaceAll(key, "vertex_ai/", ""))
		case "vertex_ai-ai21_models":
			add("vertex_ai21", strings.ReplaceAll(key, "vertex_ai/", ""))
		case "vertex_ai-image-models":
			add("vertex_image", strings.ReplaceAll(key, "vertex_ai/", ""))
		case "vertex_ai-video-models":
			add("vertex_video", strings.ReplaceAll(key, "vertex_ai/", ""))
		case "vertex_ai-openai_models":
			add("vertex_openai", strings.ReplaceAll(key, "vertex_ai/", ""))
		case "vertex_ai-minimax_models":
			add("vertex_minimax", strings.ReplaceAll(key, "vertex_ai/", ""))
		case "vertex_ai-moonshot_models":
			add("vertex_moonshot", strings.ReplaceAll(key, "vertex_ai/", ""))
		case "vertex_ai-zai_models":
			add("vertex_zai", strings.ReplaceAll(key, "vertex_ai/", ""))
		case "ai21":
			if mode == "chat" {
				add("ai21_chat", key)
			} else {
				add("ai21", key)
			}
		case "nlp_cloud":
			add("nlp_cloud", key)
		case "aleph_alpha":
			add("aleph_alpha", key)
		case "bedrock":
			if mode == "guardrail" || isBedrockPricingOnlyModel(key) {
				continue
			}
			add("bedrock", key)
		case "bedrock_converse":
			add("bedrock_converse", key)
		case "deepinfra":
			add("deepinfra", key)
		case "perplexity":
			add("perplexity", key)
		case "watsonx":
			add("watsonx", key)
		case "gemini":
			add("gemini", key)
		case "fireworks_ai":
			if !strings.Contains(key, "-to-") && !strings.Contains(key, "fireworks-ai-default") {
				add("fireworks_ai", key)
			}
		case "fireworks_ai-embedding-models":
			if !strings.Contains(key, "-to-") {
				add("fireworks_ai_embedding", key)
			}
		case "text-completion-codestral":
			add("text_completion_codestral", key)
		case "text-completion-inception":
			add("text_completion_inception", key)
		case "xai":
			add("xai", key)
		case "zai":
			add("zai", key)
		case "fal_ai":
			add("fal_ai", key)
		case "deepseek":
			add("deepseek", key)
		case "tencent":
			add("tencent", key)
		case "runwayml":
			add("runwayml", key)
		case "meta_llama":
			add("llama", key)
		case "nscale":
			add("nscale", key)
		case "azure_ai":
			add("azure_ai", key)
		case "voyage":
			add("voyage", key)
		case "infinity":
			add("infinity", key)
		case "databricks":
			add("databricks", key)
		case "cloudflare":
			add("cloudflare", key)
		case "codestral":
			add("codestral", key)
		case "friendliai":
			add("friendliai", key)
		case "palm":
			add("palm", key)
		case "groq":
			add("groq", key)
		case "azure":
			add("azure", key)
		case "azure_anthropic":
			add("azure_anthropic", key)
		case "anyscale":
			add("anyscale", key)
		case "cerebras":
			add("cerebras", key)
		case "galadriel":
			add("galadriel", key)
		case "nvidia_nim":
			add("nvidia_nim", key)
		case "nvidia_riva":
			add("nvidia_riva", key)
		case "soniox":
			add("soniox", key)
		case "sambanova":
			add("sambanova", key)
		case "sambanova-embedding-models":
			add("sambanova_embedding", key)
		case "novita":
			add("novita", key)
		case "nebius-chat-models":
			add("nebius", key)
		case "nebius-embedding-models":
			add("nebius_embedding", key)
		case "aiml":
			add("aiml", key)
		case "assemblyai":
			add("assemblyai", key)
		case "jina_ai":
			add("jina_ai", key)
		case "snowflake":
			add("snowflake", key)
		case "gradient_ai":
			add("gradient_ai", key)
		case "featherless_ai":
			add("featherless_ai", key)
		case "deepgram":
			add("deepgram", key)
		case "elevenlabs":
			add("elevenlabs", key)
		case "heroku":
			add("heroku", key)
		case "dashscope":
			add("dashscope", key)
		case "qwencloud":
			add("qwencloud", key)
		case "qwen_ai_platform":
			add("qwen_ai_platform", key)
		case "modelscope":
			add("modelscope", key)
		case "moonshot":
			add("moonshot", key)
		case "publicai":
			add("publicai", key)
		case "darkbloom":
			add("darkbloom", key)
		case "v0":
			add("v0", key)
		case "morph":
			add("morph", key)
		case "lambda_ai":
			add("lambda_ai", key)
		case "inception":
			add("inception", key)
		case "hyperbolic":
			add("hyperbolic", key)
		case "black_forest_labs":
			add("black_forest_labs", key)
		case "recraft":
			add("recraft", key)
		case "cometapi":
			add("cometapi", key)
		case "oci":
			add("oci", key)
		case "volcengine":
			add("volcengine", key)
		case "wandb":
			add("wandb", key)
		case "ovhcloud":
			add("ovhcloud", key)
		case "ovhcloud-embedding-models":
			add("ovhcloud_embedding", key)
		case "lemonade":
			add("lemonade", key)
		case "docker_model_runner":
			add("docker_model_runner", key)
		case "amazon_nova":
			add("amazon_nova", key)
		case "stability":
			add("stability", key)
		case "github_copilot":
			add("github_copilot", key)
		case "chatgpt":
			add("chatgpt", key)
		case "minimax":
			add("minimax", key)
		case "aws_polly":
			add("aws_polly", key)
		case "gigachat":
			add("gigachat", key)
		case "llamagate":
			add("llamagate", key)
		case "reducto":
			add("reducto", key)
		case "bedrock_mantle":
			add("bedrock_mantle", key)
		}
	}
	// Hardcoded seeds LiteLLM keeps outside the price map.
	add("ollama", "llama2")
	add("petals", "petals-team/StableBeluga2")
	add("maritalk", "maritalk")

	u := func(names ...string) []string {
		var out []string
		seen := map[string]struct{}{}
		for _, name := range names {
			for _, id := range sets[name] {
				if _, ok := seen[id]; ok {
					continue
				}
				seen[id] = struct{}{}
				out = append(out, id)
			}
		}
		return out
	}
	modelsByProvider = map[string][]string{
		"openai":                    u("openai_chat", "openai_text"),
		"text-completion-openai":    u("openai_text"),
		"cohere":                    u("cohere", "cohere_chat"),
		"cohere_chat":               u("cohere_chat"),
		"anthropic":                 u("anthropic"),
		"openrouter":                u("openrouter"),
		"vercel_ai_gateway":         u("vercel_ai_gateway"),
		"datarobot":                 u("datarobot"),
		"vertex_ai":                 u("vertex_chat", "vertex_text", "vertex_anthropic", "vertex_vision", "vertex_language", "vertex_deepseek", "vertex_minimax", "vertex_moonshot", "vertex_zai"),
		"ai21":                      u("ai21"),
		"bedrock":                   u("bedrock", "bedrock_converse"),
		"ollama":                    u("ollama"),
		"ollama_chat":               u("ollama"),
		"deepinfra":                 u("deepinfra"),
		"perplexity":                u("perplexity"),
		"maritalk":                  u("maritalk"),
		"watsonx":                   u("watsonx"),
		"gemini":                    u("gemini"),
		"fireworks_ai":              u("fireworks_ai", "fireworks_ai_embedding"),
		"aleph_alpha":               u("aleph_alpha"),
		"text-completion-codestral": u("text_completion_codestral"),
		"text-completion-inception": u("text_completion_inception"),
		"xai":                       u("xai"),
		"zai":                       u("zai"),
		"fal_ai":                    u("fal_ai"),
		"deepseek":                  u("deepseek"),
		"tencent":                   u("tencent"),
		"runwayml":                  u("runwayml"),
		"mistral":                   u("mistral"),
		"azure_ai":                  u("azure_ai"),
		"voyage":                    u("voyage"),
		"infinity":                  u("infinity"),
		"databricks":                u("databricks"),
		"cloudflare":                u("cloudflare"),
		"codestral":                 u("codestral"),
		"nlp_cloud":                 u("nlp_cloud"),
		"friendliai":                u("friendliai"),
		"palm":                      u("palm"),
		"groq":                      u("groq"),
		"azure":                     u("azure", "azure_text"),
		"azure_anthropic":           u("azure_anthropic"),
		"azure_text":                u("azure_text"),
		"anyscale":                  u("anyscale"),
		"cerebras":                  u("cerebras"),
		"galadriel":                 u("galadriel"),
		"nvidia_nim":                u("nvidia_nim"),
		"nvidia_riva":               u("nvidia_riva"),
		"soniox":                    u("soniox"),
		"sambanova":                 u("sambanova", "sambanova_embedding"),
		"novita":                    u("novita"),
		"nebius":                    u("nebius", "nebius_embedding"),
		"aiml":                      u("aiml"),
		"assemblyai":                u("assemblyai"),
		"jina_ai":                   u("jina_ai"),
		"snowflake":                 u("snowflake"),
		"gradient_ai":               u("gradient_ai"),
		"meta_llama":                u("llama"),
		"nscale":                    u("nscale"),
		"featherless_ai":            u("featherless_ai"),
		"deepgram":                  u("deepgram"),
		"elevenlabs":                u("elevenlabs"),
		"heroku":                    u("heroku"),
		"dashscope":                 u("dashscope"),
		"qwencloud":                 u("qwencloud"),
		"qwen_ai_platform":          u("qwen_ai_platform"),
		"modelscope":                u("modelscope"),
		"moonshot":                  u("moonshot"),
		"publicai":                  u("publicai"),
		"darkbloom":                 u("darkbloom"),
		"v0":                        u("v0"),
		"morph":                     u("morph"),
		"lambda_ai":                 u("lambda_ai"),
		"inception":                 u("inception"),
		"hyperbolic":                u("hyperbolic"),
		"black_forest_labs":         u("black_forest_labs"),
		"recraft":                   u("recraft"),
		"cometapi":                  u("cometapi"),
		"oci":                       u("oci"),
		"volcengine":                u("volcengine"),
		"wandb":                     u("wandb"),
		"ovhcloud":                  u("ovhcloud", "ovhcloud_embedding"),
		"lemonade":                  u("lemonade"),
		"clarifai":                  u("clarifai"),
		"amazon_nova":               u("amazon_nova"),
		"stability":                 u("stability"),
		"github_copilot":            u("github_copilot"),
		"chatgpt":                   u("chatgpt"),
		"minimax":                   u("minimax"),
		"aws_polly":                 u("aws_polly"),
		"gigachat":                  u("gigachat"),
		"llamagate":                 u("llamagate"),
		"reducto":                   u("reducto"),
		"bedrock_mantle":            u("bedrock_mantle"),
		"petals":                    u("petals"),
		"docker_model_runner":       u("docker_model_runner"),
	}
}

func isOpenAIFinetuneModel(key string) bool {
	return strings.HasPrefix(key, "ft:") && strings.Count(key, ":") <= 1
}

func isBedrockPricingOnlyModel(key string) bool {
	if strings.Contains(key, "month-commitment") {
		return true
	}
	return bedrockPricingOnly.MatchString(key)
}

func providerModels(provider string) []string {
	return modelsByProvider[provider]
}

// modelCostMapCount 是当前加载的价格表条目数。sample_spec 是字段说明，不计入模型。
func modelCostMapCount() int {
	raw, ok := modelCostMapValue.(map[string]any)
	if !ok {
		return 0
	}
	n := len(raw)
	if _, ok := raw["sample_spec"]; ok {
		n--
	}
	return n
}

func localCostMapForced() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("LITELLM_LOCAL_MODEL_COST_MAP")), "true")
}

// knownLLMProviders is LiteLLM's LlmProviders values. A leading segment is
// stripped only when it is one of these, so org ids like meta-llama/ stay.
var knownLLMProviders = map[string]struct{}{
	"openai": {}, "chatgpt": {}, "openai_like": {}, "jina_ai": {}, "xai": {}, "zai": {},
	"custom_openai": {}, "text-completion-openai": {}, "cohere": {}, "cohere_chat": {},
	"clarifai": {}, "anthropic": {}, "anthropic_text": {}, "bytez": {}, "replicate": {},
	"reducto": {}, "runwayml": {}, "aws_polly": {}, "huggingface": {}, "together_ai": {},
	"openrouter": {}, "datarobot": {}, "vertex_ai": {}, "vertex_ai_beta": {}, "gemini": {},
	"ai21": {}, "baseten": {}, "black_forest_labs": {}, "azure": {}, "azure_text": {},
	"azure_ai": {}, "sagemaker": {}, "sagemaker_chat": {}, "sagemaker_nova": {},
	"bedrock": {}, "vllm": {}, "nlp_cloud": {}, "petals": {}, "oobabooga": {},
	"ollama": {}, "ollama_chat": {}, "deepinfra": {}, "perplexity": {}, "mistral": {},
	"milvus": {}, "groq": {}, "a2a": {}, "gigachat": {}, "nvidia_nim": {}, "nvidia_riva": {},
	"soniox": {}, "cerebras": {}, "ai21_chat": {}, "volcengine": {}, "codestral": {},
	"text-completion-codestral": {}, "dashscope": {}, "qwencloud": {}, "qwen_ai_platform": {},
	"modelscope": {}, "moonshot": {}, "publicai": {}, "v0": {}, "morph": {}, "lambda_ai": {},
	"inception": {}, "text-completion-inception": {}, "deepseek": {}, "sambanova": {},
	"maritalk": {}, "voyage": {}, "cloudflare": {}, "xinference": {}, "fireworks_ai": {},
	"friendliai": {}, "featherless_ai": {}, "watsonx": {}, "watsonx_text": {}, "triton": {},
	"predibase": {}, "databricks": {}, "empower": {}, "github": {}, "ragflow": {},
	"compactifai": {}, "docker_model_runner": {}, "custom": {}, "litellm_proxy": {},
	"hosted_vllm": {}, "tencent": {}, "llamafile": {}, "lm_studio": {}, "galadriel": {},
	"nebius": {}, "infinity": {}, "deepgram": {}, "elevenlabs": {}, "novita": {},
	"aiohttp_openai": {}, "langfuse": {}, "humanloop": {}, "topaz": {}, "sap": {},
	"assemblyai": {}, "charity_engine": {}, "github_copilot": {}, "snowflake": {},
	"gradient_ai": {}, "meta_llama": {}, "nscale": {}, "pg_vector": {}, "s3_vectors": {},
	"valkey": {}, "mongodb": {}, "helicone": {}, "hyperbolic": {}, "recraft": {},
	"fal_ai": {}, "stability": {}, "heroku": {}, "aiml": {}, "cometapi": {}, "oci": {},
	"auto_router": {}, "vercel_ai_gateway": {}, "dotprompt": {}, "manus": {}, "wandb": {},
	"ovhcloud": {}, "scaleway": {}, "lemonade": {}, "amazon_nova": {}, "a2a_agent": {},
	"langgraph": {}, "langflow": {}, "minimax": {}, "synthetic": {}, "apertis": {},
	"nano-gpt": {}, "poe": {}, "chutes": {}, "neosantara": {}, "parasail": {},
	"xiaomi_mimo": {}, "tensormesh": {}, "libertai": {}, "pinstripes": {}, "cognition": {},
	"scx-ai": {}, "darkbloom": {}, "meta": {}, "litellm_agent": {}, "cursor": {},
	"bedrock_mantle": {}, "gdc": {},
}
