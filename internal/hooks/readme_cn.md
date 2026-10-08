# 请求在途计数

[English](readme.md) · [全功能实现说明](../../docs/development/implementation.md)

## 职责与实现契约

hooks.go 的 Engine 为 least-busy 等策略提供进程内在途计数。New 创建引擎，Begin(keyID) 增加计数并返回释放函数。释放使用 sync.Once，调用多次不会重复减；空标识是无操作。
网关必须在请求生命周期结束时 defer 释放，包括发送错误、超时和流中断。计数是运行态提示，不是身份授权、限流或账务计数，也不存储请求正文。
此包没有外部 HTTP 接口或数据库。多进程的计数不会自动汇总；跨实例负载选择依赖其它共享状态。验证应覆盖并发 Begin、双重释放、空标识以及结束后计数恢复。

## 源码职责与入口

### hooks.go

公开类型：`Engine`.

- [`func New() *Engine`](hooks.go) — New returns an empty gate. In-flight calls are counted by key id.
- [`func (e *Engine) Begin(keyID string) func()`](hooks.go) — Begin reserves one in-flight slot and returns the release function. An empty key id is not counted, so a caller without a key still gets a release.

## 对外 HTTP 边界

无本目录直接登记的 HTTP 路由。导出的 Go API 由上层调用；运行时目录调度或调用宿主的入口应沿依赖链追踪。

## 依赖关系

[internal/logx](../logx/readme_cn.md).

## 验证与维护入口

| 测试文件 | 场景入口 |
| --- | --- |
| [hooks_test.go](hooks_test.go) | `TestReleaseIsIdempotentWithOtherCallsInFlight` |

```bash
go test ./internal/hooks -count=1
```

数据库验收设置 XHUB_REGRESSION_STRICT=1 并检查跳过项；Redis 和真实供应商需单独配置。接口、字段或行为改变后同步本说明及相关功能文档。
