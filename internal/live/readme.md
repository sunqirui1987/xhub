# Redis hot state and usage queue

[简体中文](readme_cn.md) · [Feature implementation reference](../../docs/development/implementation.md)

## Responsibilities and behavior

redis.go stores failures, cooldowns, latency, RPM/TPM, hot spend, affinity, and pending logs. Methods have explicit behavior when Redis is absent; degraded operation does not retain distributed guarantees.
Hot spend supports immediate budget checks, while PostgreSQL owns durable accounting. Queue peeking and acknowledgment are separate; consumers acknowledge after commit, and IAM request_id deduplicates redelivery. Queue pressure and Redis failure need observable behavior.
Cooldown read failures yield no cooled entries; that policy must not be copied to identity database errors. Shared state uses deployment-specific identifiers. Real Redis tests verify counters, expiry, queue order, and acknowledgment; skipped Redis cases are unverified.

## Source responsibilities and entry points

### redis.go

Exported types: `Client`, `SpendLog`.

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

## External HTTP boundary

This directory registers no direct HTTP route. Higher layers call its Go API; trace catalog dispatch or host calls through the dependency chain.

## Dependencies

[internal/logx](../logx/readme.md).

## Verification and maintenance

| Test file | Scenario entry points |
| --- | --- |
| [redis_test.go](redis_test.go) | `TestEnqueueSpendConcurrentDedupAndAck`, `TestEnqueueSpendRequiresIdentity`, `TestEnqueueSpendRejectsInvalidStateWithoutPartialWrites`, `TestPeekLogsStopsAtCorruptEvent`, `TestConcurrentTokenCounterHasExpiry` |

```bash
go test ./internal/live -count=1
```

Use XHUB_REGRESSION_STRICT=1 for database acceptance and inspect skips. Redis and live providers require separate configuration. Update this reference and feature documentation after contract changes.
