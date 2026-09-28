# cache

## 这个模块做什么

`cache` 是进程内的响应缓存。重复的推理请求可以从内存里回答，不必再次调用供应商。条目数量有上限，读和写都会复制字节，调用方改自己的切片不会改到缓存内容。

## 功能

- 固定容量的 LRU。满了之后丢掉最久未用的条目。
- `Get` 返回存储字节的副本。
- `Set` 保存你传入值的副本。
- `Flush` 清空全部条目。
- `Key` 用请求的若干部分拼出一个稳定的缓存键。

## 其它包怎么用

导入 `github.com/sunqirui1987/xhub/internal/cache`。

```go
c := cache.New()
key := cache.Key("chat", "gpt-4o-mini", string(body))
if hit, ok := c.Get(key); ok {
    w.Write(hit)
    return
}
// 调用上游之后：
c.Set(key, responseBody)
```

网关进程在 `Server` 上放一份 `DualCache`，通过 `ResponseCache` 交出去。请求处理里应该用这一份，这样大家共用同一个缓存。只有在网关进程之外、需要单独缓存时才自己调用 `New`。

## 这个包不做什么

它不连接 Redis，也不决定哪条路由允许缓存。那个策略留在调用方。
