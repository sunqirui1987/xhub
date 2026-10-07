# live

`live` is the Redis client for the hot path. A request updates counters here. It does not insert a PostgreSQL row. `dataplane/live.go` reads this client for routing state, and `dataplane.Flush` drains the spend queue into `iam.DB`.

`Open(url)` parses a Redis URL and pings it. Ping failure closes the client and returns the error. `gateway` leaves `Server.Redis` nil when `redis_url` is empty. A nil `*Client` makes the methods below no-ops or empty maps; the request then writes spend straight to PostgreSQL inside the request.

## Two different ids

Do not mix these.

| Caller | Value passed as `id` | Key shape |
| --- | --- | --- |
| Router cooldown, latency, route TPM, spend | deployment id `api_base\|model` from `router.DeploymentID` | `xhub:fails:`, `xhub:cooldown:`, `xhub:latency:`, `xhub:routetpm:`, `xhub:spend:` |
| Key RPM / TPM | virtual-key token hash `Principal.Hash` (`auth.go`, field comment on `Principal.Hash`) | `xhub:rpm:<hash>:<unixMinute>`, `xhub:tpm:<hash>:<unixMinute>` |

`limits.go` `enforceRedisRateLimits` calls `HitRPM(p.Hash)` and `HitTPM(p.Hash, est)`. It does not pass `api_base|model`.

## Routing counters (deployment id)

- `RecordFailure(id, allowed, cooldown)` increments `xhub:fails:<id>`. `allowed < 1` returns immediately and writes nothing. Once the count exceeds `allowed`, it sets `xhub:cooldown:<id>` to `"1"` for `cooldown`. `RecordFailure` is reached from `dataplane.RecordFailure` after a 5xx or 429. `allowed_fails` and `cooldown_time` come from the router settings document; the code default cooldown is one minute when the document has no duration.
- `Cooled(ids)` returns which of those ids still have `xhub:cooldown:`. Redis down means an empty map, and the router treats that as no cooldown.
- `AddLatency` pushes milliseconds onto the left of `xhub:latency:<id>`, keeps 20 samples, and expires the key after one hour. `Latencies` returns the average. An id with no sample is omitted.
- `AddUsage` adds tokens to `xhub:routetpm:<id>` for the current minute. This is the router's usage signal (`router.State.Usage`). It is not the key TPM limit.

## Key rate limits (token hash)

`minuteBucket` is `time.Now().Unix()/60`. It cannot fail.

`HitRPM(id)` calls `bump("xhub:rpm:"+id, 1)`. With the hash from `limits.go` the stem is `xhub:rpm:<tokenHash>`. `bump` then stores `xhub:rpm:<tokenHash>:<minute>` and sets a two-minute expiry, so the next minute starts at zero.

`HitTPM(id, tokens)` uses the stem `xhub:tpm:<tokenHash>` and adds `tokens` (the estimate `limits.go` already computed).

`bump` returns the new counter. A nil client returns `0, nil`. A Redis `INCRBY` error returns `0` and the error. `limits.go` turns that error into HTTP 503 `rate_limit_unavailable`. A limit of 0, or a count above the key's RPM/TPM limit, is HTTP 429.

## Spend queue

`ChargeSpend` / `EnqueueSpend` add a dollar delta to `xhub:spend:<id>` and the id to the set `xhub:spend:ids`. Spend log rows are pushed on the list `xhub:spendlog`.

`PeekSpend` and `PeekLogs` read without deleting. `TakeSpend` reads and clears. `AckSpend` and `AckFlushed` subtract a delta only after PostgreSQL has committed it, so a failed flush can be retried from the same Redis values. `DrainLogs` pops from the head; `n < 1` returns nil.

`GetString` / `SetString` are the generic string used by chat affinity and official task pins (`deployment_affinity:v1:…`, `official_task:v1:…`, `official_billed:v1:…`). Those key names are chosen by `gateway/affinity.go`, not by this package.

## What this package does not do

It does not check a budget and it does not write `usage_events`. The flush that turns a queued row into a billed event is `dataplane.Flush` calling `iam.DB.RecordUsage`. In-process in-flight counts are `internal/hooks`, keyed by key id, not by these Redis keys.

中文说明见同目录 `readme_cn.md`。
