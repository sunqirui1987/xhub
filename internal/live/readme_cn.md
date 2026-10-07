# live

`live` 是热路径上的 Redis 客户端。一次请求在这里改计数，不往 PostgreSQL 插行。`dataplane/live.go` 用它读路由状态，`dataplane.Flush` 把花费队列刷进 `iam.DB`。

`Open(url)` 解析 Redis URL 并 ping。ping 失败会关掉客户端并返回错误。`redis_url` 为空时网关把 `Server.Redis` 留成 nil。nil 的 `*Client` 让下面这些方法变成空操作或空表，花费就在请求里直接写 PostgreSQL。

## 两种 id 不要混

| 调用方 | 传入的 `id` | 键的形状 |
| --- | --- | --- |
| 路由冷却、延迟、路由 TPM、花费 | 部署 id `api_base\|model`，来自 `router.DeploymentID` | `xhub:fails:`、`xhub:cooldown:`、`xhub:latency:`、`xhub:routetpm:`、`xhub:spend:` |
| 密钥 RPM / TPM | 虚拟密钥的令牌哈希 `Principal.Hash`（`auth.go` 里 `Principal.Hash` 的字段说明） | `xhub:rpm:<哈希>:<unix分钟>`、`xhub:tpm:<哈希>:<unix分钟>` |

`limits.go` 的 `enforceRedisRateLimits` 调用的是 `HitRPM(p.Hash)` 和 `HitTPM(p.Hash, est)`。它不传 `api_base|model`。

## 路由计数（部署 id）

- `RecordFailure(id, allowed, cooldown)` 对 `xhub:fails:<id>` 加一。`allowed < 1` 立刻返回，什么都不写。次数超过 `allowed` 之后，把 `xhub:cooldown:<id>` 设成 `"1"`，有效期是 `cooldown`。5xx 或 429 之后由 `dataplane.RecordFailure` 调到这里。`allowed_fails` 和 `cooldown_time` 来自路由设置；文档里没有时长时，代码默认冷却一分钟。
- `Cooled(ids)` 返回这些 id 里哪些还挂着 `xhub:cooldown:`。Redis 不可用时是空表，路由器把空表当成没有冷却。
- `AddLatency` 把毫秒数从左侧推进 `xhub:latency:<id>`，只留 20 条，一小时后过期。`Latencies` 返回平均值。没有样本的 id 不出现。
- `AddUsage` 把 token 加进当前分钟的 `xhub:routetpm:<id>`。这是路由器的用量信号（`router.State.Usage`），不是密钥的 TPM 限额。

## 密钥限流（令牌哈希）

`minuteBucket` 是 `time.Now().Unix()/60`。这一步不会失败。

`HitRPM(id)` 调用 `bump("xhub:rpm:"+id, 1)`。`limits.go` 传入哈希时，前半段是 `xhub:rpm:<令牌哈希>`。`bump` 再写成 `xhub:rpm:<令牌哈希>:<分钟>`，并设两分钟过期，所以下一分钟从 0 开始。

`HitTPM(id, tokens)` 的前半段是 `xhub:tpm:<令牌哈希>`，加上的是 `limits.go` 已经算好的估算 token。

`bump` 返回新的计数。客户端是 nil 时返回 `0, nil`。Redis `INCRBY` 失败时返回 `0` 和 error。`limits.go` 把这个 error 写成 HTTP 503 `rate_limit_unavailable`。限额是 0，或计数超过密钥的 RPM/TPM 限额，是 HTTP 429。

## 花费队列

`ChargeSpend` / `EnqueueSpend` 把美元增量加到 `xhub:spend:<id>`，并把 id 放进集合 `xhub:spend:ids`。花费日志行推进列表 `xhub:spendlog`。

`PeekSpend` 和 `PeekLogs` 只读不删。`TakeSpend` 读完就清。`AckSpend` 和 `AckFlushed` 只在 PostgreSQL 提交成功之后才减掉增量，失败的刷写还能用同一份 Redis 值重试。`DrainLogs` 从队头弹出；`n < 1` 返回 nil。

`GetString` / `SetString` 是通用字符串，给聊天粘滞和官方任务钉用（`deployment_affinity:v1:…`、`official_task:v1:…`、`official_billed:v1:…`）。这些键名是 `gateway/affinity.go` 定的，不是这个包定的。

## 这个包不做什么

它不查预算，也不写 `usage_events`。把队列里的一行变成已计费事件的是 `dataplane.Flush` 调用 `iam.DB.RecordUsage`。进程内的在途次数在 `internal/hooks`，按密钥 id 计，不用这些 Redis 键。

English notes are in `readme.md` in this directory.
