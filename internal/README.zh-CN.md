# 后端模块总览

[English](README.md) · [全功能实现说明](../docs/development/implementation.md)

## 职责与实现契约

本目录实现 XHub 的身份、管理控制面、模型数据面、路由、计价、持久化和跨模块回归。cmd 负责组合和启动，internal 的包负责可测试的业务行为；Go 的 internal 导入规则限制外部项目直接依赖这些实现。对外稳定边界是 HTTP 协议和配置文档，不是所有导出的 Go 符号。
一次推理先识别会话或虚拟密钥，从数据库恢复组织、团队、项目和所有者，再检查动作、模型范围、预算与限流。控制面解析本次路由模板，数据面选择可用部署、构造协议并发送；成功响应提取 usage、保存价格快照和调用归属，最后提交成功缓存及粘性状态。
身份数据和账务在 iam，框架配置及模型部署在 store，Redis 热状态在 live，进程内响应缓存在 cache。五个费用作用域是同一次消费的视图，不能相加成为总消费。流输出后禁止拼接第二个供应商响应；数据库提交前不能确认队列。

## 子目录与协作边界

| 目录 | 职责 |
| --- | --- |
| [auth](auth/readme_cn.md) | 调用身份解析 |
| [authz](authz/readme_cn.md) | 动作授权与查询范围 |
| [cache](cache/readme_cn.md) | 进程内响应缓存 |
| [catalog](catalog/readme_cn.md) | 模型目录、费率与用量归一化 |
| [config](config/readme_cn.md) | 启动配置与模型定义 |
| [dataplane](dataplane/readme_cn.md) | 推理执行与结算数据面 |
| [gateway](gateway/readme_cn.md) | HTTP 网关与模块组合 |
| [hooks](hooks/readme_cn.md) | 请求在途计数 |
| [httpx](httpx/readme_cn.md) | HTTP 错误与模块协议 |
| [iam](iam/readme_cn.md) | 身份、授权资源与事务账务 |
| [live](live/readme_cn.md) | Redis 热状态与可靠用量队列 |
| [llm](llm/readme_cn.md) | 上游请求构造与协议适配 |
| [logx](logx/readme_cn.md) | 运行日志与秘密脱敏 |
| [plugin](plugin/readme_cn.md) | 上游调用扩展注册 |
| [provider](provider/readme_cn.md) | 供应商能力与传输登记 |
| [providerconfig](providerconfig/readme_cn.md) | 真实供应商测试配置解析与权重周期校验 |
| [regression](regression/readme_cn.md) | 跨模块业务回归 |
| [router](router/readme_cn.md) | 部署过滤、排序与加权分流 |
| [store](store/readme_cn.md) | 框架配置与模型存储 |
| [testsupport](testsupport/readme_cn.md) | 隔离数据库测试支持 |

## 源码职责与入口

此目录由资源、子包或测试文件组成，没有独立生产 Go 实现。

## 对外 HTTP 边界

无本目录直接登记的 HTTP 路由。导出的 Go API 由上层调用；运行时目录调度或调用宿主的入口应沿依赖链追踪。

## 验证与维护入口

当前目录没有直接测试文件；上层集成测试仅证明被执行的链路，不代表所有内部失败分支均已覆盖。

```bash
go test ./internal/... -count=1
```

数据库验收设置 XHUB_REGRESSION_STRICT=1 并检查跳过项；Redis 和真实供应商需单独配置。接口、字段或行为改变后同步本说明及相关功能文档。
