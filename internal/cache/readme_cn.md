# 进程内响应缓存

[English](readme.md) · [全功能实现说明](../../docs/development/implementation.md)

## 职责与实现契约

cache.go 实现容量为 8192 的并发 LRU。New 无容量参数；Get 返回字节副本，Set 复制输入，避免调用方修改共享缓存。Key 把各字符串以零字节分隔后计算 SHA256，调用方负责提供完整身份和行为上下文。
缓存只保存完整成功响应，由 dataplane 决定是否读取和写入。流式请求不写此缓存；失败不得复用为成功。缓存键包含调用身份、操作、公开模型、正文、配置及模板等摘要，避免跨租户或改配置后错误命中。
这里没有 TTL、磁盘持久化或 Redis 后端。Flush 清除这个对象中的条目，不重置预算、Redis 热状态或数据库日志。多实例有各自缓存，不能把一次清空当作全局失效。

## 源码职责与入口

### cache.go

公开类型：`DualCache`.

- [`func New() *DualCache`](cache.go) — New builds an in-process cache of 8192 entries. It panics if that capacity is illegal, which would be a programming error.
- [`func Key(parts ...string) string`](cache.go) — Key hashes the tenant, operation, model, and body into a cache key. Parts are separated by a zero byte so "ab"+"c" and "a"+"bc" do not collide.
- [`func (c *DualCache) Get(key string) ([]byte, bool)`](cache.go) — Get returns a copy of the cached bytes. Changing the slice does not change the cache. A miss returns ok false.
- [`func (c *DualCache) Set(key string, value []byte)`](cache.go) — Set stores a copy of value. Later changes to the caller's slice do not change the cached bytes.
- [`func (c *DualCache) Flush()`](cache.go) — Flush removes every cached response. It does not write spend or talk to Redis.

## 对外 HTTP 边界

无本目录直接登记的 HTTP 路由。导出的 Go API 由上层调用；运行时目录调度或调用宿主的入口应沿依赖链追踪。

## 依赖关系

[internal/logx](../logx/readme_cn.md).

## 验证与维护入口

| 测试文件 | 场景入口 |
| --- | --- |
| [cache_test.go](cache_test.go) | `TestKeySeparatesPartsAndCacheCopiesBytes` |

```bash
go test ./internal/cache -count=1
```

数据库验收设置 XHUB_REGRESSION_STRICT=1 并检查跳过项；Redis 和真实供应商需单独配置。接口、字段或行为改变后同步本说明及相关功能文档。
