# Process-local response cache

[简体中文](readme_cn.md) · [Feature implementation reference](../../docs/development/implementation.md)

## Responsibilities and behavior

cache.go implements a concurrent 8192-entry LRU. New takes no capacity argument. Get and Set copy bytes so callers cannot mutate shared entries. Key hashes zero-separated string components with SHA256; callers supply complete identity and behavior context.
The data plane decides which complete successful responses are cacheable. Streaming and failed executions do not create success entries. Cache identity includes caller, operation, public model, body, deployment configuration, and routing context.
There is no TTL, persistent storage, or Redis backend here. Flush clears this object only, without resetting budgets, queues, or logs. Each process has its own cache and invalidation boundary.

## Source responsibilities and entry points

### cache.go

Exported types: `DualCache`.

- [`func New() *DualCache`](cache.go) — New builds an in-process cache of 8192 entries. It panics if that capacity is illegal, which would be a programming error.
- [`func Key(parts ...string) string`](cache.go) — Key hashes the tenant, operation, model, and body into a cache key. Parts are separated by a zero byte so "ab"+"c" and "a"+"bc" do not collide.
- [`func (c *DualCache) Get(key string) ([]byte, bool)`](cache.go) — Get returns a copy of the cached bytes. Changing the slice does not change the cache. A miss returns ok false.
- [`func (c *DualCache) Set(key string, value []byte)`](cache.go) — Set stores a copy of value. Later changes to the caller's slice do not change the cached bytes.
- [`func (c *DualCache) Flush()`](cache.go) — Flush removes every cached response. It does not write spend or talk to Redis.

## External HTTP boundary

This directory registers no direct HTTP route. Higher layers call its Go API; trace catalog dispatch or host calls through the dependency chain.

## Dependencies

[internal/logx](../logx/readme.md).

## Verification and maintenance

| Test file | Scenario entry points |
| --- | --- |
| [cache_test.go](cache_test.go) | `TestKeySeparatesPartsAndCacheCopiesBytes` |

```bash
go test ./internal/cache -count=1
```

Use XHUB_REGRESSION_STRICT=1 for database acceptance and inspect skips. Redis and live providers require separate configuration. Update this reference and feature documentation after contract changes.
