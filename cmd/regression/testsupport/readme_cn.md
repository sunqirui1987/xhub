# 隔离数据库测试支持

[English](readme.md) · [全功能实现说明](../../../docs/development/implementation.md)

## 职责与实现契约

postgres.go 的 DatabaseURL 读取 XHUB_TEST_DATABASE_URL，未设置时返回本地 Compose 默认连接，不是空字符串。Postgres(t,prefix) 创建独立 schema，返回带 search_path 的连接 URL，并通过 t.Cleanup 删除。
数据库不可达时通常 Skip；XHUB_REGRESSION_STRICT=1 时 Fatal。数据库已连通但 schema 创建失败属于真实测试失败。用例必须在各自 schema 中打开 IAM/Store，避免读写开发数据。强制结束进程可能跳过 cleanup，应按本次 prefix 清理泄漏 schema。
该包不导入 iam/gateway，避免测试依赖循环。连接字符串和环境凭据不要写入失败报告。

## 源码职责与入口

### postgres.go

- [`func DatabaseURL() string`](postgres.go) — DatabaseURL returns the DSN the suites should connect to.
- [`func Postgres(t *testing.T, prefix string) string`](postgres.go) — Postgres returns a DSN whose search_path points at a schema created for this one test, and registers the cleanup that drops it. A private schema is what makes these suites safe to run against a database a gateway is also using: the test writes real accounts, real keys and real usage rows, and without the schema it would read the developer's own rows and leave its own behind. The prefix names the caller in the schema, so a schema leaked by a killed test run can be traced back to the package that made it. The test skips when the database is unreachable. It fails only when the database answers and the fixture cannot be built, because that is a real error rather than a missing dependency.

## 对外 HTTP 边界

无本目录直接登记的 HTTP 路由。导出的 Go API 由上层调用；运行时目录调度或调用宿主的入口应沿依赖链追踪。

## 验证与维护入口

| 测试文件 | 场景入口 |
| --- | --- |

```bash
go test ./cmd/regression/testsupport -count=1
```

数据库验收设置 XHUB_REGRESSION_STRICT=1 并检查跳过项；Redis 和真实供应商需单独配置。接口、字段或行为改变后同步本说明及相关功能文档。
