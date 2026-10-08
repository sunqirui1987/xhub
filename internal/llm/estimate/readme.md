# Token and cost estimation

[简体中文](readme_cn.md) · [Feature implementation reference](../../../docs/development/implementation.md)

## Responsibilities and behavior

tokencount.go estimates input tokens using tokenizers; cost.go provides TokenCost and related helpers for rates, margins, discounts, and service tiers. Estimates support checks and quotes, not authoritative settlement.
Tokenizer mismatch, multimodal input, hidden reasoning, and cache behavior limit precision. Billing uses actual upstream quantities and catalog prices when available.
There are no direct HTTP routes; gateway token and quote handlers call these helpers. Document supported inputs, unknown models, and units when extending estimation. This directory currently has no direct test file; integration checks do not prove every estimation branch.

## Source responsibilities and entry points

### cost.go

Exported types: `Margin`.

- [`func ApplyDiscount(baseCost float64, provider string, discounts map[string]float64) (final, percent, amount float64)`](cost.go) — ApplyDiscount looks up a rate for the provider in the discount map and subtracts it from the base cost. Discount-map keys are custom_llm_provider. The value is a fraction: 0.05 means subtract 5 percent. A missing provider, or an empty provider name, leaves the cost unchanged. The results are the discounted cost, the discount fraction, and the discount amount.
- [`func ApplyMargin(baseCost float64, provider string, margins map[string]Margin) (final, percent, fixed, total float64)`](cost.go) — ApplyMargin adds a markup after the discount. A provider's own rule wins over the global rule stored under the key global. The results are the marked-up cost, the percent, the fixed amount, and the markup total. With no rule the last three values are 0 and the cost is unchanged.
- [`func TokenCost(promptTokens, completionTokens int, inputRate, outputRate float64) (float64, float64)`](cost.go) — TokenCost is the cost of a text completion when cache tokens are not billed separately. Prompt tokens multiply the input rate and completion tokens multiply the output rate. Rates are dollars per token.
- [`func MapTrafficType(trafficType string) (tier string, known, standard bool)`](cost.go) — MapTrafficType folds a Gemini usageMetadata.trafficType into a billing tier. ON_DEMAND_PRIORITY uses the priority rate. FLEX, BATCH, and ON_DEMAND_FLEX use the flex rate. ON_DEMAND is the standard price. The tier is empty, but known and standard are both true. An empty string or an unknown value sets known to false, and the caller should fall back to the standard price.
- [`func NormalizeServiceTier(serviceTier string, isString bool) (string, bool)`](cost.go) — NormalizeServiceTier reports whether service_tier can be used to look up a price. "auto" is only a routing preference, not a billing tier. A non-string is not one either. Both return ok false, so a later lookup does not call lower on an empty string and mis-read the standard price.

### tokencount.go

- [`func CountTokens(model, prompt string, messages []map[string]any) (int, string, error)`](tokencount.go) — CountTokens matches the local LiteLLM token_counter for OpenAI chat models. A prompt is counted as plain text. Each message adds tokens_per_message, then the role and content, and the total adds 3 reply-priming tokens. The returned tokenizer type is the openai_tokenizer selected by LiteLLM.
- [`func OpenAISupportedParams(model string, catalog bool) []string`](tokencount.go) — OpenAISupportedParams matches OpenAIGPTConfig.get_supported_openai_params. gpt-4 and gpt-3.5-turbo-1 6k omit response_format. OpenAI models listed in the catalog also include user.
- [`func ModelUsedForCount(requestModel, deploymentModel string) string`](tokencount.go) — ModelUsedForCount drops the provider prefix from a deployment model, matching LiteLLM token_counter.

## External HTTP boundary

This directory registers no direct HTTP route. Higher layers call its Go API; trace catalog dispatch or host calls through the dependency chain.

## Dependencies

[internal/logx](../../logx/readme.md).

## Verification and maintenance

There are no direct test files here. Integration tests prove executed paths rather than every internal failure branch.

```bash
go test ./internal/llm/estimate -count=1
```

Use XHUB_REGRESSION_STRICT=1 for database acceptance and inspect skips. Redis and live providers require separate configuration. Update this reference and feature documentation after contract changes.
