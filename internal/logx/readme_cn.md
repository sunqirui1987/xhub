# 运行日志与秘密脱敏

[English](readme.md) · [全功能实现说明](../../docs/development/implementation.md)

## 职责与实现契约

log.go 提供 Trace/Debug/Info/Error 等 fmt 风格包装，底层使用标准 logger。写出前过滤 Bearer、sk- 风格 token 和 URL 用户凭据，减少诊断信息泄漏。
运行日志用于排查请求阶段和依赖错误；它不是 IAM 的持久审计，也不是 spend log。禁止把完整环境变量、连接串、用户 prompt 或上游秘密串作为普通日志参数；模式脱敏不能保证任意秘密格式都会识别。
没有 HTTP 接口。files_test 检查日志使用相关约束。添加日志要保留调用 ID 和明确失败阶段，避免输出真实凭据；需要审计的读取和写入走业务审计链。

## 源码职责与入口

### log.go

- [`func Trace(format string, args ...any)`](log.go) — Trace records a step inside a call, such as entering the data plane.
- [`func Debug(format string, args ...any)`](log.go) — Debug records a decision that is useful when a call did not do what the operator expected, such as skipping a deployment.
- [`func Info(format string, args ...any)`](log.go) — Info records a normal outcome, such as the process listening or one HTTP request finishing.
- [`func Error(format string, args ...any)`](log.go) — Error records a failure. The process keeps running unless the caller exits after this returns.

## 对外 HTTP 边界

无本目录直接登记的 HTTP 路由。导出的 Go API 由上层调用；运行时目录调度或调用宿主的入口应沿依赖链追踪。

## 验证与维护入口

| 测试文件 | 场景入口 |
| --- | --- |
| [files_test.go](files_test.go) | `TestEveryServerFileUsesLeveledLogger`, `TestRedactHidesBearerAndKey`, `TestRedactHidesCaseInsensitiveKeysAndURLCredentials` |

```bash
go test ./internal/logx -count=1
```

数据库验收设置 XHUB_REGRESSION_STRICT=1 并检查跳过项；Redis 和真实供应商需单独配置。接口、字段或行为改变后同步本说明及相关功能文档。
