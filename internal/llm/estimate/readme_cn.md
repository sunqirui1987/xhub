# Token 与费用估算

[English](readme.md) · [全功能实现说明](../../../docs/development/implementation.md)

## 职责与实现契约

tokencount.go 使用 tokenizer 对文本、消息等输入估算 token；cost.go 根据模型价格、倍率、折扣和服务档位提供 TokenCost 等费用辅助。估算为预检查和报价服务，不代替供应商返回的真实 usage。
模型 tokenizer 不完全对应时只能近似；图像、音频、隐藏推理或缓存命中并不由普通文本计数完整涵盖。正式结算优先采用真实上游数量和 catalog 费率。
函数没有独立 HTTP 路由，由 gateway/tokens 和 usage 的报价能力使用。新增估算支持需说明支持输入、缺失模型处理、输出单位以及与历史账务的区别。此目录当前没有直接测试文件，调用集成验证不能宣称覆盖每个估算分支。

## 源码职责与入口

### cost.go

公开类型：`Margin`.

- [`func ApplyDiscount(baseCost float64, provider string, discounts map[string]float64) (final, percent, amount float64)`](cost.go) — ApplyDiscount looks up a rate for the provider in the discount map and subtracts it from the base cost. Discount-map keys are custom_llm_provider. The value is a fraction: 0.05 means subtract 5 percent. A missing provider, or an empty provider name, leaves the cost unchanged. The results are the discounted cost, the discount fraction, and the discount amount.
- [`func ApplyMargin(baseCost float64, provider string, margins map[string]Margin) (final, percent, fixed, total float64)`](cost.go) — ApplyMargin adds a markup after the discount. A provider's own rule wins over the global rule stored under the key global. The results are the marked-up cost, the percent, the fixed amount, and the markup total. With no rule the last three values are 0 and the cost is unchanged.
- [`func TokenCost(promptTokens, completionTokens int, inputRate, outputRate float64) (float64, float64)`](cost.go) — TokenCost is the cost of a text completion when cache tokens are not billed separately. Prompt tokens multiply the input rate and completion tokens multiply the output rate. Rates are dollars per token.
- [`func MapTrafficType(trafficType string) (tier string, known, standard bool)`](cost.go) — MapTrafficType folds a Gemini usageMetadata.trafficType into a billing tier. ON_DEMAND_PRIORITY uses the priority rate. FLEX, BATCH, and ON_DEMAND_FLEX use the flex rate. ON_DEMAND is the standard price. The tier is empty, but known and standard are both true. An empty string or an unknown value sets known to false, and the caller should fall back to the standard price.
- [`func NormalizeServiceTier(serviceTier string, isString bool) (string, bool)`](cost.go) — NormalizeServiceTier reports whether service_tier can be used to look up a price. "auto" is only a routing preference, not a billing tier. A non-string is not one either. Both return ok false, so a later lookup does not call lower on an empty string and mis-read the standard price.

### tokencount.go

- [`func CountTokens(model, prompt string, messages []map[string]any) (int, string, error)`](tokencount.go) — CountTokens matches the local LiteLLM token_counter for OpenAI chat models. A prompt is counted as plain text. Each message adds tokens_per_message, then the role and content, and the total adds 3 reply-priming tokens. The returned tokenizer type is the openai_tokenizer selected by LiteLLM.
- [`func OpenAISupportedParams(model string, catalog bool) []string`](tokencount.go) — OpenAISupportedParams matches OpenAIGPTConfig.get_supported_openai_params. gpt-4 and gpt-3.5-turbo-1 6k omit response_format. OpenAI models listed in the catalog also include user.
- [`func ModelUsedForCount(requestModel, deploymentModel string) string`](tokencount.go) — ModelUsedForCount drops the provider prefix from a deployment model, matching LiteLLM token_counter.

## 对外 HTTP 边界

无本目录直接登记的 HTTP 路由。导出的 Go API 由上层调用；运行时目录调度或调用宿主的入口应沿依赖链追踪。

## 依赖关系

[internal/logx](../../logx/readme_cn.md).

## 验证与维护入口

当前目录没有直接测试文件；上层集成测试仅证明被执行的链路，不代表所有内部失败分支均已覆盖。

```bash
go test ./internal/llm/estimate -count=1
```

数据库验收设置 XHUB_REGRESSION_STRICT=1 并检查跳过项；Redis 和真实供应商需单独配置。接口、字段或行为改变后同步本说明及相关功能文档。
