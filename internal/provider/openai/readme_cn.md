# OpenAI 供应商占位边界

[English](readme.md) · [全功能实现说明](../../../docs/development/implementation.md)

## 职责与实现契约

chat.go 保留 OpenAI 供应商包边界，当前不通过 init 注册一组端点类型。OpenAI 的适配请求、usage 和流处理在 llm 与 dataplane，模型能力来自 provider 能力表。
这个目录没有独立管理 API，也不能按历史说明宣称登记七种类型。chat_test 验证当前边界。若以后增加供应商登记，应明确哪些是能力、哪些是传输，并由 provider/all 装配。
阅读聊天实现时应从 gateway ingress → dataplane Serve → llm Build/Endpoint 跟踪，而不是在这个占位目录寻找完整 HTTP 客户端。

## 源码职责与入口

### chat.go

内部实现和协议边界见 [chat.go](chat.go)。

## 对外 HTTP 边界

无本目录直接登记的 HTTP 路由。导出的 Go API 由上层调用；运行时目录调度或调用宿主的入口应沿依赖链追踪。

## 依赖关系

[internal/logx](../../logx/readme_cn.md).

## 验证与维护入口

| 测试文件 | 场景入口 |
| --- | --- |
| [chat_test.go](chat_test.go) | `TestAdaptedTypesAreNotRegistered`, `TestCapabilityTableDescribesTheAdaptedEntrypoints`, `TestRegisterTransportRejectsAnEntryWithNoActions` |

```bash
go test ./internal/provider/openai -count=1
```

数据库验收设置 XHUB_REGRESSION_STRICT=1 并检查跳过项；Redis 和真实供应商需单独配置。接口、字段或行为改变后同步本说明及相关功能文档。
