// Package catalog loads the built-in model price map and lists models by provider.
//
// The price map is generated from the configured price feed
// (显式配置的价格源) and
// embedded as publicdata/pricedata.json. There is no LiteLLM price file and no
// sample_spec row: every key in the map is a real model.
package catalog

import (
	"os"
	"strings"
	"sync"
	"time"

	"github.com/sunqirui1987/xhub/internal/logx"
)

// The price map and the per-provider index, parsed once at startup.
// Wildcard expansion (openai/*) reads the per-provider sets built from each
// row's litellm_provider field.
var (
	modelCostMu          sync.RWMutex
	modelCostMapValue    any
	modelCostMapLoadedAt string
	modelsByProvider     map[string][]string
)

var logTraceOnceModelCost sync.Once

// init loads the embedded data this package depends on. A parse failure falls back to an empty map so the process can still start.
// 参数：无。
// 调用：Go 在载入这个包时自动执行。
// 测试：无直接单测
// 返回：无。嵌入的价格表已载入，并记下了载入时刻。解析失败时表为空，进程仍能启动。
func init() {
	logTraceOnceModelCost.Do(func() { logx.Trace("enter catalog.init") })

	loadPriceDocument()
	modelCostMapLoadedAt = time.Now().UTC().Format(time.RFC3339)
}

// providerModels returns the model ids indexed for that provider. A missing provider returns nil.
// 调用：ProviderModels。
// 测试：无直接单测
// 参数 provider（string）：供应商标识，例如 openai 或 volcengine。
// 返回 []string（[]string）：这个供应商在价格表里的模型 id。没有这个供应商时为 nil。
func providerModels(provider string) []string {
	modelCostMu.RLock()
	defer modelCostMu.RUnlock()
	return modelsByProvider[provider]
}

// modelCostMapCount is the number of models in the loaded price map.
// 参数：无。
// 调用：Count。
// 测试：无直接单测
// 返回 int（int）：价格表里的模型条数。零表示还没有载入。
func modelCostMapCount() int {
	modelCostMu.RLock()
	defer modelCostMu.RUnlock()
	raw, ok := modelCostMapValue.(map[string]any)
	if !ok {
		return 0
	}
	return len(raw)
}

// localCostMapForced reports whether only the built-in price map may be used.
// 参数：无。
// 返回 bool（bool）：环境变量 LITELLM_LOCAL_MODEL_COST_MAP 为 true 时返回真。
// 调用：EnvForced。
// 测试：无直接单测
func localCostMapForced() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("LITELLM_LOCAL_MODEL_COST_MAP")), "true")
}

// knownLLMProviders holds the prefixes that name a supplier rather than an
// organization id. While expanding a wildcard, only a known prefix is stripped
// and then rejoined with the caller's prefix, so org ids like meta-llama/ stay.
// Provider packages add to this set when they register.
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
	// Suppliers the embedded price catalog names.
	"kling": {}, "vidu": {}, "byteplus": {}, "meituan": {}, "stepfun": {}, "arcee_ai": {},
}

// Raw returns the raw JSON object of the built-in price map.
// 参数：无。
// 调用：gateway/catalog.go
// 测试：无直接单测
// 返回 any（any）：any，供调用方继续使用。
func Raw() any {
	modelCostMu.RLock()
	defer modelCostMu.RUnlock()
	return modelCostMapValue
}

// TokenRates returns the per-token input and output prices for one model in the built-in price map. The name is matched as stored, and if that misses, the provider prefix before the first slash is dropped. A missing name or a row with no per-token prices returns ok false.
// 参数 model（string）：对外模型名，用来选部署和记用量。
// 调用：catalog/cost.go、gateway/usage/reports.go
// 测试：cost_breakdown_test.go、match_test.go、seedance_test.go
// 返回 input（float64）：输入侧费用；output（float64）：输出侧费用；ok（bool）：真表示找到了可用结果。
func TokenRates(model string) (input, output float64, ok bool) {
	modelCostMu.RLock()
	defer modelCostMu.RUnlock()
	model = strings.TrimSpace(model)
	if model == "" {
		return 0, 0, false
	}
	// 和 CostAt 走同一份候选键。只看写下来的第一个键会停在没有单价的空壳上，
	// 价目表里后面那条同名模型的每 token 价就读不到了。
	row, found := priceRowForLocked(model)
	if !found {
		return 0, 0, false
	}
	return tokenRatesFrom(row)
}

// priceRow reads one object from the price map. A missing key or a non-object returns ok false.
// 返回 map[string]any（map[string]any）：价格表里这个键对应的对象。原键没有时再试小写。都没有时为 nil；bool（bool）：找到对象时为真。
// 调用：仅在 model_cost.go 内使用
// 测试：无直接单测
// 参数 raw（map[string]any）：原始文本或 JSON 字节；key（string）：要读取的字段名或映射键。
func priceRow(raw map[string]any, key string) (map[string]any, bool) {
	row, ok := raw[key].(map[string]any)
	if ok {
		return row, true
	}
	lower := strings.ToLower(key)
	if lower == key {
		return nil, false
	}
	row, ok = raw[lower].(map[string]any)
	return row, ok
}

// tokenRatesFrom reads input_cost_per_token and output_cost_per_token. Both missing returns ok false. A missing side is zero.
// 参数 row（map[string]any）：价格表里这一条模型的字段。
// 调用：仅在 model_cost.go 内使用
// 测试：无直接单测
// 返回 input（float64）：输入侧费用；output（float64）：输出侧费用；ok（bool）：真表示找到了可用结果。
func tokenRatesFrom(row map[string]any) (input, output float64, ok bool) {
	in, inOK := floatField(row["input_cost_per_token"])
	out, outOK := floatField(row["output_cost_per_token"])
	if !inOK && !outOK {
		return 0, 0, false
	}
	return in, out, true
}

// floatField reads a JSON number. A missing or non-numeric value returns ok false.
// 调用：仅在 model_cost.go 内使用
// 测试：无直接单测
// 参数 v（any）：待转换或待保存的值。
// 返回 float64（float64）：JSON 数字转成的小数。不是数字时为 0；bool（bool）：值是 float64、int 或 int64 时为真。
func floatField(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	default:
		return 0, false
	}
}

// CostMap turns the price map into a model-name to fields map. An entry that is not an object is dropped.
// 参数：无。
// 返回 map[string]map[string]any（map[string]map[string]any）：模型名到价格字段的表。不是对象的条目会被丢掉。还没载入时为空表。
// 调用：gateway/catalog.go、gateway/models/available.go、gateway/models/builtin.go、gateway/public_hub.go
// 测试：无直接单测
func CostMap() map[string]map[string]any {
	modelCostMu.RLock()
	defer modelCostMu.RUnlock()
	raw, ok := modelCostMapValue.(map[string]any)
	if !ok {
		return map[string]map[string]any{}
	}
	out := make(map[string]map[string]any, len(raw))
	for id, v := range raw {
		row, ok := v.(map[string]any)
		if ok {
			out[id] = row
		}
	}
	return out
}

// Count is the number of models in the price map.
// 参数：无。
// 调用：gateway/catalog.go、gateway/models/admin.go、iam/teams.go、iam/usage_read.go
// 测试：authz_test.go、usage_idempotency_test.go
// 返回 int（int）：价格表里的模型条数。
func Count() int { return modelCostMapCount() }

// LoadedAt is the UTC time, in RFC3339, when the process loaded this built-in price map.
// 参数：无。
// 调用：gateway/catalog.go、gateway/models/admin.go
// 测试：无直接单测
// 返回 string（string）：内置价格表的载入时刻，UTC 的 RFC3339。还没载入时为空串。
func LoadedAt() string {
	modelCostMu.RLock()
	defer modelCostMu.RUnlock()
	return modelCostMapLoadedAt
}

// MarkReloaded records that the price catalog was refreshed and returns the
// number of models now in it. The rows themselves come from ReloadFromMarket;
// this only stamps the time the console displays.
// 参数：无。
// 调用：gateway/models/cost_reload.go
// 测试：无直接单测
// 返回 int（int）：价格表里的模型条数。
func MarkReloaded() int {
	modelCostMu.Lock()
	defer modelCostMu.Unlock()
	modelCostMapLoadedAt = time.Now().UTC().Format(time.RFC3339)
	raw, ok := modelCostMapValue.(map[string]any)
	if !ok {
		return 0
	}
	return len(raw)
}

// EnvForced reports that LITELLM_LOCAL_MODEL_COST_MAP is set to true, ignoring case. When it is true the gateway uses only the built-in map and does not try a remote price source.
// 参数：无。
// 返回 bool（bool）：环境变量 LITELLM_LOCAL_MODEL_COST_MAP 为 true 时返回真。
// 调用：gateway/catalog.go、gateway/models/admin.go
// 测试：无直接单测
func EnvForced() bool { return localCostMapForced() }

// ProviderModels returns the model ids for one provider in the built-in map. An unknown provider returns nil.
// 调用：gateway/catalog.go、gateway/models/list.go
// 测试：无直接单测
// 参数 provider（string）：供应商标识，例如 openai 或 volcengine。
// 返回 []string（[]string）：这个供应商在价格表里的模型 id。没有这个供应商时为 nil。
func ProviderModels(provider string) []string { return providerModels(provider) }

// KnownProvider reports that this prefix is a supplier name rather than an organization id. While expanding a wildcard, only a known provider prefix is stripped and then joined with the caller's prefix.
// 返回 bool（bool）：这个前缀是供应商标识而不是组织 id 时返回真。通配符展开时只剥这种前缀。
// 调用：gateway/models/list.go
// 测试：无直接单测
// 参数 name（string）：名称，用来查找或展示这一项。
func KnownProvider(name string) bool {
	_, ok := knownLLMProviders[name]
	return ok
}
