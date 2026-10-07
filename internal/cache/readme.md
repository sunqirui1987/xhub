# cache

In-process LRU of upstream response bodies. Capacity is the constant `cacheEntries` = 8192. `New` builds it with `hashicorp/golang-lru` and panics only if that library rejects the capacity, which the constant does not. The gateway keeps one cache on the server and passes it to `dataplane.Serve` as `ResponseCache`.

`Get` and `Set` copy the byte slice. A caller that later mutates its buffer does not change the stored value, and a caller that mutates the slice returned by `Get` does not change the cache.

`Key(parts...)` is SHA-256 of the parts joined by a zero byte, hex encoded. The zero byte is what stops `"ab"+"c"` colliding with `"a"+"bc"`. `Serve` calls it as `cache.Key(tenant, op, alias, rawBody)`. Tenant is `Principal.Hash` when the caller has a virtual key, otherwise the empty string. `op` is the data-plane operation (`chat`, `embedding`, …). `alias` is the public model name. `rawBody` is the exact request bytes. Stream responses never call `Get`. A hit is written back by `Host.WriteCacheHit` and the billed amount is 0; token counts on that row stay.

`Flush` calls `Purge` on the LRU. It does not write spend and it does not talk to Redis. Nothing in the request path calls `Flush` after a single response; it is for tests and for an operator path that wants the process cache empty.

## What this package does not do

It does not cache official bypass responses. `ServeBypass` never calls `Get`. It does not set a TTL. An entry leaves only when the LRU evicts it or `Flush` runs. It does not store prompt text under a separate key; the body is already inside the hash.

中文说明见同目录 `readme_cn.md`。
