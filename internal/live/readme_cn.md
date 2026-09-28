# live

## 这个模块做什么

`live` 是热路径上的 Redis 客户端。冷却、延迟样本、每分钟请求数和 token 数，以及还没写进 PostgreSQL 的花费，都放在这里。推理请求本身不写数据库，稍后的刷写才写。

## 功能

- `Open` 用 Redis URL 连接。`Close` 关闭连接。
- `RecordFailure` 和 `Cooled` 实现冷却窗口。`allowed` 是冷却前允许的失败次数。调用方把 0 分钟的冷却理解成 1 分钟。
- `AddLatency` / `Latencies` 和 `AddUsage` / `Usages` 给路由器用。
- `HitRPM` 和 `HitTPM` 增加当前这一分钟的计数，并返回新的合计。
- `ChargeSpend`、`PeekSpend`、`TakeSpend`、`AckSpend` 搬运美元增量。
- `EnqueueLog`、`PeekLogs`、`AckLogs`、`DrainLogs` 搬运花费日志。`AckFlushed` 确认一次同时写了花费增量的刷写。

## 其它包怎么用

导入 `github.com/sunqirui1987/xhub/internal/live`。

```go
client, err := live.Open(cfg.GeneralSettings.RedisURL)
if err != nil {
    log.Fatal(err)
}
defer client.Close()

_ = client.RecordFailure(deploymentID, 3, time.Minute)
cooled := client.Cooled([]string{deploymentID})
_ = client.ChargeSpend(keyHash, 0.002)
```

网关把这个客户端放在 `Server.Live`，用 `Redis` 交出去。没有配置 Redis 时这个值是 nil，数据面只在内存里走请求路径。不要对 nil 客户端调用方法。

## 这个包不做什么

它不打开 PostgreSQL，也不决定哪个部署处理某个模型。路由读取这个客户端返回的表。
