# 内置公开数据资源

[English](readme.md) · [全功能实现说明](../../../docs/development/implementation.md)

## 职责与实现契约

本目录有 pricedata.json、cn_holidays.json、autorouter_presets.json，由父包 embed.go 嵌入到可执行文件。pricedata 是默认价格目录；cn_holidays 为时段价格计算提供日历；autorouter_presets 是保留的数据资源，不代表已移除的自动路由功能重新生效。
资源没有自己的 Go 包或 HTTP handler。读取、校验、覆盖和公开展示由 catalog 与 gateway/models 完成。修改 JSON 后必须校验语法、模型标识、计价单位和明确零价；价格单价的展示尺度与后端基础单位要一致。
文件内容进入构建产物，编辑源文件不会自动改变已经运行的二进制。运行时覆盖和价格源刷新是另一条链路。日历改变需验证边界日期；价格改变需验证 CostAt 和历史快照隔离。

## 源码职责与入口

此目录由资源、子包或测试文件组成，没有独立生产 Go 实现。

资源与持久化定义：[autorouter_presets.json](autorouter_presets.json), [cn_holidays.json](cn_holidays.json), [pricedata.json](pricedata.json).

## 对外 HTTP 边界

无本目录直接登记的 HTTP 路由。导出的 Go API 由上层调用；运行时目录调度或调用宿主的入口应沿依赖链追踪。

## 验证与维护入口

当前目录没有直接测试文件；上层集成测试仅证明被执行的链路，不代表所有内部失败分支均已覆盖。

```bash
go test ./internal/catalog -count=1
```

数据库验收设置 XHUB_REGRESSION_STRICT=1 并检查跳过项；Redis 和真实供应商需单独配置。接口、字段或行为改变后同步本说明及相关功能文档。
