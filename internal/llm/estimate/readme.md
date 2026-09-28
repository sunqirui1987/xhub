# llm/estimate

## Purpose

`llm/estimate` prices a call from token counts and provider discounts, and it estimates how many tokens a prompt will use. A missing price must not become zero. A failed token count returns an error instead of pretending the prompt was empty.

## Features

- `TokenCost` multiplies prompt and completion tokens by per-token rates.
- `ApplyDiscount` and `ApplyMargin` adjust a base cost with the provider maps the dashboard stores. `Margin` carries a percent and an optional fixed fee.
- `CountTokens` estimates a token count for a model, a raw prompt, or a chat message list.
- `OpenAISupportedParams` lists the OpenAI parameters a model claims to support.
- `ModelUsedForCount` picks the deployment model when the request alias and the upstream model differ.
- `MapTrafficType` and `NormalizeServiceTier` normalize the tier strings the cost view understands.

## How another package uses it

Import `github.com/sunqirui1987/xhub/internal/llm/estimate`.

```go
n, _, err := estimate.CountTokens("gpt-4o-mini", "", messages)
if err != nil {
    return err
}
input, output := estimate.TokenCost(n, completion, inputRate, outputRate)
final, percent, amount := estimate.ApplyDiscount(input+output, "openai", discounts)
```

Rates come from `catalog.CostMap`, not from this package. Pass the rates in. If the map has no row for the model, do not call `TokenCost` with zeros.

## What this package does not do

It does not write a spend log. `store` and `live` persist the number you compute.

中文使用说明见同目录的 readme_cn.md。
