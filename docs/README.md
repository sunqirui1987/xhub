# XHub 设计修复方案

本套文档于 2026-10-02 根据当前工作区代码重新编写。旧 docs 文档、生成脚本及缓存已移除；这里记录的是代码审查结论、目标设计和实施验收条件。除文档索引引用迁移外，本次不实现核心修复。

核心判断：当前项目具有身份、模型配置、推理转发、缓存和支出报表的基础，但“存在接口”不能证明“可以生产使用”。用户模型列表尚未完整证明可调用性，用量与结算缺少一致的事实来源，异步媒体与多种传输协议尚未形成完整生命周期。先修权限与账务底座，再扩展 Seedance 2.0。

## 阅读顺序

| 文档 | 解决的问题 |
| --- | --- |
| [00 范围与现状](00-scope-and-status.md) | 审查证据、缺陷严重程度、已实现与待实现边界 |
| [01 系统模型](01-system-model.md) | 身份、模型、部署、协议、用量和账务的边界 |
| [02 身份与权限](02-identity-and-authorization.md) | 会话撤销、能力授权、多租户隔离、故障拒绝 |
| [03 模型目录与可用模型](03-model-catalog-and-availability.md) | 我的模型、大模型广场、供应商与模型的区别 |
| [04 请求生命周期](04-request-lifecycle.md) | 请求、尝试、终态及流式异常 |
| [05 用量事件与日志](05-usage-events-and-logging.md) | 所有结果可追踪，使用量不再固定为聊天 token |
| [06 计费与结算](06-billing-and-settlement.md) | 预占、价格快照、事务与幂等扣费 |
| [07 缓存与幂等](07-cache-and-idempotency.md) | 命中不重复收生成费用，重试不重复创建任务 |
| [08 协议适配器](08-protocol-adapters.md) | JSON、SSE、multipart、二进制、WebSocket 与异步任务 |
| [09 异步媒体与 Seedance](09-async-media-and-seedance.md) | 提交、查询、取消、回调、下载和结算闭环 |
| [10 扩展契约](10-extension-contract.md) | 扩展钩子、失败策略与权限边界 |
| [11 可观测性与报表](11-observability-and-reporting.md) | 历史归属、指标口径、时区、对账与运行手册 |
| [12 实施迁移](12-migration-plan.md) | 阶段依赖、旧数据迁移、发布与回滚 |
| [13 测试与验收](13-test-and-acceptance-plan.md) | 最坏情况、并发故障、发布门槛 |
| [14 团队管理](14-team-management.md) | 当前控制台和网关实际支持的团队、成员、模型和预算 |
| [15 用户管理](15-user-management.md) | 账号角色和团队角色怎么分开，创建用户时该选什么 |
| [16 权限](16-permissions.md) | 平台管理员、组织管理员、团队管理员和成员各自能做什么，列表怎么收窄 |
| [17 验证数据](17-demo-data.md) | 可选的验证租户。不是启动时的默认数据 |

数据契约：[请求事件](schemas/request-event.md)、[用量](schemas/usage.md)、[结算](schemas/settlement.md)、[异步任务](schemas/async-task.md)。

关键决策：[ADR-001 会话与权限](decisions/ADR-001-session-permissions.md)、[ADR-002 可用模型](decisions/ADR-002-available-model-definition.md)、[ADR-003 事实与结算](decisions/ADR-003-request-event-and-settlement.md)、[ADR-004 异步媒体](decisions/ADR-004-async-media-billing.md)、[ADR-005 供应商与模型](decisions/ADR-005-provider-vs-model.md)。

## 本次保留的数据基线

[catalog.json](catalog.json) 保存 LiteLLM 1.102.0 的结构化对照数据，共 779 条 HTTP 路由。保留该数据是为了让现有覆盖检查继续有对照源；不是对旧文档的继续使用，也不是 XHub 支持 779 条生产协议的声明。

该文件的路由、供应商、页面、SDK 方法是静态基线。运行中的可用模型必须由授权与部署状态生成，不能读取这个文件直接作为用户模型列表。其 implementation 字段也不能替代端到端验证；发布状态以本文档的验收门槛及实际检查结果为准。

## 必须成立的约束

1. 用户只能看到和调用自己被授权、已发布且存在有效部署的模型；目录显示不授予调用权限。
2. 身份、权限、预算及强制限流无法确认时，禁止启动新的付费上游工作。
3. 每个已持久化接收的请求最终有唯一终态；每次上游尝试均可追踪，异步任务终态与提交请求分开。
4. 重试、队列重复、回调重复和多个 worker 并发不会导致重复结算。
5. 历史报表使用请求时的归属和价格快照；删密钥、改名字、转团队不改变旧账。
6. 新协议只有在授权、真实调用、用量提取、计费、错误和恢复均通过验收后才标记为生产支持。

这些约束是修复目标，不是对当前实现的保证。故障、未知用量和无法确认上游结果必须显式表示，不能伪装成成功、零费用或空列表。
