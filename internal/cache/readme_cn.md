# cache

进程内的上游响应 LRU。容量是常量 `cacheEntries` = 8192。`New` 用 `hashicorp/golang-lru` 建表，只有库拒绝这个容量时才 panic，常量本身不会。网关在进程上留一份，通过 `ResponseCache` 交给 `dataplane.Serve`。

`Get` 和 `Set` 都会复制字节。调用方后来改自己的缓冲区，不会改到缓存里的值；改 `Get` 返回的切片，也不会改缓存。

`Key(parts...)` 是各段用一个零字节拼起来之后的 SHA-256，再编成十六进制。零字节用来避免 `"ab"+"c"` 和 `"a"+"bc"` 撞车。`Serve` 的调用是 `cache.Key(租户, op, 对外名, 原始正文)`。调用方有虚拟密钥时租户是 `Principal.Hash`，否则是空串。`op` 是数据面操作名（`chat`、`embedding` 等）。`alias` 是对外模型名。`rawBody` 是请求的原始字节。流式响应不调用 `Get`。命中由 `Host.WriteCacheHit` 写回，记账金额是 0，token 数保留。

`Flush` 对 LRU 调用 `Purge`。它不写花费，也不访问 Redis。单次请求的路径上不会在写完响应之后调用 `Flush`；它给测试，以及给想把进程缓存倒空的操作路径。

## 这个包不做什么

它不缓存官方 bypass 的响应。`ServeBypass` 从不调用 `Get`。它不设 TTL。一条记录只在 LRU 淘汰或 `Flush` 时离开。它也不另存一份提示词；正文已经在哈希里。

English notes are in `readme.md` in this directory.
