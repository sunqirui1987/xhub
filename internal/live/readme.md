# live

## Purpose

`live` is the Redis client for facts that change on every request: cooldown, latency samples, requests and tokens per minute, and spend that has not been written to PostgreSQL yet. The inference request itself does not write the database. A later flush does.

## Features

- `Open` connects with a Redis URL. `Close` closes it.
- `RecordFailure` and `Cooled` implement the cooldown window. `allowed` is how many failures are tolerated before cooling. A cooldown of zero is stored as one minute by the caller that interprets it.
- `AddLatency` / `Latencies` and `AddUsage` / `Usages` feed the router.
- `HitRPM` and `HitTPM` increment the current minute bucket and return the new total.
- `ChargeSpend`, `PeekSpend`, `TakeSpend`, and `AckSpend` move dollar deltas.
- `EnqueueLog`, `PeekLogs`, `AckLogs`, and `DrainLogs` move spend-log rows. `AckFlushed` acknowledges a flush that also applied spend deltas.

## How another package uses it

Import `github.com/sunqirui1987/xhub/internal/live`.

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

The gateway stores this client on `Server.Live` and exposes it with `Redis`. When Redis is not configured, that value is nil and the data plane keeps the request path in memory only. Do not call methods on a nil client.

## What this package does not do

It does not open PostgreSQL and it does not choose which deployment handles a model. The router reads the maps this client returns.

中文使用说明见同目录的 readme_cn.md。
