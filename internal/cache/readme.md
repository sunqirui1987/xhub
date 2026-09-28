# cache

## Purpose

`cache` is the process-local response cache. A repeated inference call can be answered from memory instead of calling the provider again. Entries are capped, and every read and write copies the bytes so a caller cannot change what is stored.

## Features

- Fixed-capacity LRU. The least recently used entry is dropped when the cache is full.
- `Get` returns a copy of the stored bytes.
- `Set` stores a copy of the value you pass.
- `Flush` drops every entry.
- `Key` builds a stable cache key from the parts of a request.

## How another package uses it

Import `github.com/sunqirui1987/xhub/internal/cache`.

```go
c := cache.New()
key := cache.Key("chat", "gpt-4o-mini", string(body))
if hit, ok := c.Get(key); ok {
    w.Write(hit)
    return
}
// call the upstream, then:
c.Set(key, responseBody)
```

The gateway process keeps one `DualCache` on `Server` and exposes it with `ResponseCache`. Other packages should use that instance during a request so they share one cache. Call `New` only when you need an isolated cache, for example in a tool that is not the gateway process.

## What this package does not do

It does not talk to Redis and it does not decide whether a particular route is allowed to be cached. That policy stays with the caller.

中文使用说明见同目录的 readme_cn.md。
