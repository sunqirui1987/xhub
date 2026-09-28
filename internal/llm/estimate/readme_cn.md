# llm/estimate

## 这个模块做什么

`llm/estimate` 用 token 数和供应商折扣给一次调用计价，并估算一段提示会用掉多少 token。找不到价格时不能当成 0。token 计数失败要返回错误，不能假装提示是空的。

## 功能

- `TokenCost` 用每 token 单价乘提示 token 和补全 token。
- `ApplyDiscount` 和 `ApplyMargin` 用控制台保存的供应商表调整基价。`Margin` 带百分比和可选的固定费用。
- `CountTokens` 按模型、原始提示或聊天消息列表估算 token 数。
- `OpenAISupportedParams` 列出某个模型声明支持的 OpenAI 参数。
- `ModelUsedForCount` 在请求别名和上游模型不同时，选出用来计数的部署模型。
- `MapTrafficType` 和 `NormalizeServiceTier` 把成本视图认识的档位字符串规范化。

## 其它包怎么用

导入 `github.com/sunqirui1987/xhub/internal/llm/estimate`。

```go
n, _, err := estimate.CountTokens("gpt-4o-mini", "", messages)
if err != nil {
    return err
}
input, output := estimate.TokenCost(n, completion, inputRate, outputRate)
final, percent, amount := estimate.ApplyDiscount(input+output, "openai", discounts)
```

单价来自 `catalog.CostMap`，不是这个包。把单价传进来。价格表没有这个模型时，不要用 0 去调用 `TokenCost`。

## 这个包不做什么

它不写花费日志。算出来的数字由 `store` 和 `live` 保存。
