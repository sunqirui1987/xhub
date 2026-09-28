# router

## 这个模块做什么

`router` 给共用一个对外模型名的部署排序。`gpt-4o-mini` 这样的名字可以背后有多个上游。这个包决定顺序。它不发送 HTTP。`Pick` 返回之后，`EncodeRequest` 和 `DecodeResponse` 把正文交给 `llm`。

## 功能

- `All` 返回 `model_name` 与别名匹配的全部分部署，包括通配符。
- `Order` 用策略和一份 `State` 快照排序。快照里有冷却、延迟和负载。
- `Pick` 返回第一个还能用的部署。
- `ValidateStrategy` 拒绝路由不认识的策略名。
- `DeploymentID` 是 Redis 冷却键用的稳定 id。
- 一个部署在冷却中时，只要还有别的部署可用，就会被跳过。

## 其它包怎么用

导入 `github.com/sunqirui1987/xhub/internal/router`。

```go
if err := router.ValidateStrategy(strategy); err != nil {
    return err
}
st := router.State{Cooldown: cooledIDs, Latency: latencyMS}
chosen := router.Pick(cfg.ModelList, "gpt-4o-mini", strategy, st)
if chosen == nil {
    // 没有可用部署
    return
}
body, err := router.EncodeRequest("chat", provider, publicJSON, realModel)
```

策略包括 `simple-shuffle`、`least-busy`、`latency-based-routing` 和 `usage-based-routing`。传入设置页保存的名字。空策略会在进程接流量之前由 `config.Load` 填上。

## 这个包不做什么

它不记录失败。一次尝试失败后，数据面调用 `live.RecordFailure`，再用更新过的 `State` 重新 `Pick`。
