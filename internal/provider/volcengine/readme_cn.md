# 火山方舟官方视频任务传输

[English](readme.md) · [全功能实现说明](../../../docs/development/implementation.md)

## 职责与实现契约

seedance.go 登记 ark_contents_generation，默认 ark.cn-beijing.volces.com。上游路径 /api/v3/contents/generations/tasks 支持创建、列表和按 ID 查询；公共入口由 transport 的路径映射决定。
模型字段与任务 ID 为 model/id，去掉 volcengine 前缀再发送。移除不对应火山账户的输入/输出各 7 USD/百万 token 硬编码价，需要配置实际账户价格。共享 Seedance 提取器只结算成功终态 completion_tokens，并按参考视频和分辨率选档；不能把未定价费用零解释为免费。
供应商注册与模型登记不承担调用身份、预算、错误重试或去重结算；这些在数据面。修改路径、模型前缀或 usage 字段后要跑 seedance_test 与官方任务集成测试；实际费用还需要真实任务终态 usage 证据。

## 源码职责与入口

### seedance.go

内部实现和协议边界见 [seedance.go](seedance.go)。

## 对外 HTTP 边界

无本目录直接登记的 HTTP 路由。导出的 Go API 由上层调用；运行时目录调度或调用宿主的入口应沿依赖链追踪。

## 依赖关系

[internal/logx](../../logx/readme_cn.md), [internal/provider](../readme_cn.md).

## 验证与维护入口

| 测试文件 | 场景入口 |
| --- | --- |
| [seedance_test.go](seedance_test.go) | `TestSeedanceModelsAndPaths` |

```bash
go test ./internal/provider/volcengine -count=1
```

数据库验收设置 XHUB_REGRESSION_STRICT=1 并检查跳过项；Redis 和真实供应商需单独配置。接口、字段或行为改变后同步本说明及相关功能文档。
