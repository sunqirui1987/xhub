# router

## Purpose

`router` orders the deployments that share one public model name. A model such as `gpt-4o-mini` may have several upstreams. This package decides the order. It does not send HTTP. After `Pick` returns, `EncodeRequest` and `DecodeResponse` hand the body to `llm`.

## Features

- `All` returns every deployment whose `model_name` matches the alias, including wildcard patterns.
- `Order` sorts those deployments with a strategy and a `State` snapshot of cooldown, latency, and load.
- `Pick` returns the first deployment that is still usable.
- `ValidateStrategy` rejects a strategy name the router does not implement.
- `DeploymentID` is the stable id used as the Redis cooldown key.
- A deployment that is cooling down is skipped when another deployment is still available.

## How another package uses it

Import `github.com/sunqirui1987/xhub/internal/router`.

```go
if err := router.ValidateStrategy(strategy); err != nil {
    return err
}
st := router.State{Cooldown: cooledIDs, Latency: latencyMS}
chosen := router.Pick(cfg.ModelList, "gpt-4o-mini", strategy, st)
if chosen == nil {
    // no deployment available
    return
}
body, err := router.EncodeRequest("chat", provider, publicJSON, realModel)
```

Strategies include `simple-shuffle`, `least-busy`, `latency-based-routing`, and `usage-based-routing`. Pass the names the settings page stores. An empty strategy is filled by `config.Load` before the process serves traffic.

## What this package does not do

It does not record a failure. After a failed attempt the data plane calls `live.RecordFailure`, then asks `Pick` again with an updated `State`.

中文使用说明见同目录的 readme_cn.md。
