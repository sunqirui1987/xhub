# Redis 热状态与可靠用量队列

[English](readme.md) · [全功能实现说明](../../docs/development/implementation.md)

## 职责与实现契约

redis.go 保存部署失败计数、冷却、延迟、RPM/TPM、热支出、粘性及待落库日志。New/运行方法在没有 Redis 时按各自降级规则处理，不能把降级解释成共享状态仍被保证。
请求即时更新热支出用于预算判断；耐久账务仍由 PostgreSQL 事务承担。队列 Peek 与 Ack 分离：消费者只有落库成功后确认，重复交付由 IAM request_id 去重。队列满或 Redis 故障需要明确日志和失败/降级语义。
Cooled 查询故障按空结果处理，但数据库身份读取故障不能沿用这种开放降级。冷却计数使用可区分部署的标识。测试要连接真实 Redis，核验计数、TTL、队列顺序与确认；未连接时相应用例跳过，不能算作 Redis 验收。

## 源码职责与入口

### redis.go

公开类型：`Client`, `SpendLog`.

- [`func SpendRef(kind, id string) string`](redis.go) — SpendRef 拼出热花费计数的 Redis 键，按密钥、团队、用户、组织或项目分开。
- [`func Open(url string) (*Client, error)`](redis.go) — Open parses the Redis URL and pings it. On failure it closes the client and returns the error.
- [`func (c *Client) Close() error`](redis.go) — Close closes Redis. A nil client is not an error.
- [`func (c *Client) RecordFailure(id string, allowed int, cooldown time.Duration) error`](redis.go) — RecordFailure increments the failure count. After allowed failures it writes the cooldown key. An allowed below 1 does nothing.
- [`func (c *Client) Cooled(ids []string) map[string]bool`](redis.go) — Cooled reports which deployment ids are still cooling down. If Redis is unavailable it returns an empty map and the caller treats that as no cooldown.
- [`func (c *Client) AddLatency(id string, ms float64) error`](redis.go) — AddLatency pushes one latency onto the left of the list, keeps the latest 20, and expires the key after one hour.
- [`func (c *Client) Latencies(ids []string) map[string]float64`](redis.go) — Latencies returns the average of recent latencies for each deployment. An id with no sample is omitted.
- [`func (c *Client) AddUsage(id string, tokens int) error`](redis.go) — AddUsage adds tokens to the current minute bucket.
- [`func (c *Client) Usages(ids []string) map[string]float64`](redis.go) — Usages returns each deployment's token usage for the current minute.
- [`func (c *Client) HitRPM(id string) (int64, error)`](redis.go) — HitRPM increments the current minute's request count and returns the new value.
- [`func (c *Client) HitTPM(id string, tokens int) (int64, error)`](redis.go) — HitTPM adds tokens to the current minute and returns the new value.
- [`func (c *Client) ChargeSpend(id string, usd float64) error`](redis.go) — ChargeSpend adds a dollar delta to hot spend and puts the id in the set waiting to be flushed.
- [`func (c *Client) HotSpend(id string) float64`](redis.go) — HotSpend is the spend delta not yet flushed to PostgreSQL. A missing id returns 0.
- [`func (c *Client) PeekSpend() map[string]float64`](redis.go) — PeekSpend reads hot spend with GET and does not subtract or delete it. After a failed flush the same deltas can still be read.
- [`func (c *Client) AckSpend(deltas map[string]float64) error`](redis.go) — AckSpend subtracts a delta only after PostgreSQL has committed it. It decrements the Redis counters for the map it is given and does not read the database itself.
- [`func (c *Client) ClearSpendQueue() error`](redis.go) — ClearSpendQueue drops spend waiting to be flushed. Use it only from a test or an explicit reset.
- [`func (c *Client) TakeSpend() map[string]float64`](redis.go) — TakeSpend reads and clears spend waiting to be flushed. Unlike Peek, a later failure no longer finds the delta in Redis.
- [`func (c *Client) EnqueueSpend(row SpendLog) error`](redis.go) — EnqueueSpend publishes a log and its hot budget deltas atomically. RequestID must identify one immutable event globally (including the tenant namespace). Replays are successful no-ops, even after AckFlushed; the first payload wins. Dedup identities have no TTL. Redis persistence/retention is required until PostgreSQL commits; this method does not wait for an fsync or replica quorum.
- [`func (c *Client) EnqueueLog(row SpendLog) error`](redis.go) — 把一条花费日志放进 Redis 队列，等刷写进 PostgreSQL。客户端为空时直接返回。
- [`func (c *Client) PeekLogs(n int) (rows []SpendLog, raw []string)`](redis.go) — PeekLogs 查看花费队列头部的若干条，不删除。客户端为空或没有日志时两个返回值都是 nil。
- [`func (c *Client) AckFlushed(deltas map[string]float64, n int, head string) error`](redis.go) — AckFlushed acknowledges spend deltas and the written log prefix together after the database transaction succeeds. AckFlushed subtracts hot spend and trims the log prefix in one Redis script. If the queue head is no longer the batch that was peeked, it does nothing, so a retry cannot subtract twice after a successful ack and cannot drop the logs first.
- [`func (c *Client) AckLogs(n int) error`](redis.go) — AckLogs 从花费队列头部丢掉已经刷进数据库的 n 条。
- [`func (c *Client) DrainLogs(n int) []SpendLog`](redis.go) — DrainLogs 从队列头部弹出最多 n 条花费日志。弹出后队列里不再保留它们。
- [`func (c *Client) GetString(ctx context.Context, key string) (string, bool)`](redis.go) — GetString reads one Redis string. A missing key returns ok false.
- [`func (c *Client) SetString(ctx context.Context, key, value string, ttl time.Duration)`](redis.go) — SetString stores one Redis string with a TTL. A nil client does nothing.

## 对外 HTTP 边界

无本目录直接登记的 HTTP 路由。导出的 Go API 由上层调用；运行时目录调度或调用宿主的入口应沿依赖链追踪。

## 依赖关系

[internal/logx](../logx/readme_cn.md).

## 验证与维护入口

| 测试文件 | 场景入口 |
| --- | --- |
| [redis_test.go](redis_test.go) | `TestEnqueueSpendConcurrentDedupAndAck`, `TestEnqueueSpendRequiresIdentity`, `TestEnqueueSpendRejectsInvalidStateWithoutPartialWrites`, `TestPeekLogsStopsAtCorruptEvent`, `TestConcurrentTokenCounterHasExpiry` |

```bash
go test ./internal/live -count=1
```

数据库验收设置 XHUB_REGRESSION_STRICT=1 并检查跳过项；Redis 和真实供应商需单独配置。接口、字段或行为改变后同步本说明及相关功能文档。
