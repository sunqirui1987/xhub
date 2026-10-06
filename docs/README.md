# XHub 文档

`docs/` 分三层。看文档前先确认自己在哪一层——这三层回答的是不同问题，混着读会得到互相矛盾的结论。

| 目录 | 回答什么 | 可信度 |
| --- | --- | --- |
| [`current/`](current/) | 这个系统**现在**怎么运行 | 按代码写，每篇带证据文件；与代码冲突时以代码为准 |
| [`design/`](design/) | 我们**打算**把它做成什么样 | 方案，不是承诺；大部分尚未实现 |
| [`testdata/`](testdata/) | 测试用的静态基线数据 | 数据，不是文档 |

要判断「这条规则现在生效吗」，只读 `current/`。`design/` 里写着的事情，多数还没有写进代码。

## current/ — 现在运行的行为

这几篇描述的是已实现并跑在网关里的规则。每篇开头列出证据文件，改动那一块代码的人应该同时改这里。

| 文档 | 解决的问题 |
| --- | --- |
| [14 团队管理](current/14-team-management.md) | 团队、成员、项目和密钥现在能怎么操作 |
| [15 用户管理](current/15-user-management.md) | 怎么建账号、账号属于哪里、忘记密码怎么办 |
| [16 权限](current/16-permissions.md) | 平台管理员、组织管理员、团队管理员、成员各自看见什么、能做什么 |
| [17 验证数据](current/17-demo-data.md) | 可选的一套验证租户。不是启动时的默认数据 |
| [18 测试架构](current/18-test-architecture.md) | 四层测试各自验证什么、怎么跑、依赖什么 |

根目录的 [README](../README.md) 里的「权限模型」一节也是这一层，讲整体设计取舍。

`current/` 与 `design/` 冲突时以 `current/` 为准。这不是偏好问题：`current/` 是照着跑起来的代码写的，`design/` 是照着想要的目标写的。

## design/ — 目标设计（多数未实现）

这一层来自 2026-10-02 的一次整体审查，给出的是一套目标架构和分阶段实施计划。**它描述的不是当前行为。** 每篇顶部都有同样的提示。

优先级判断：审查给每个缺陷标了 P0/P1。P0 表示可能破坏权限或账务一致性，应当在生产开放前阻断；P1 表示能力、统计或协议语义不完整，应当限定功能范围。风险分级不等于这些问题都已经在生产环境发生过。

| 文档 | 解决的问题 | 现在是否已实现 |
| --- | --- | --- |
| [00 范围与现状](design/00-scope-and-status.md) | 审查证据、缺陷严重程度、已实现与待实现边界 | 缺陷表逐条标注 |
| [01 系统模型](design/01-system-model.md) | 身份、模型、部署、协议、用量和账务的边界 | 部分 |
| [02 身份与权限](design/02-identity-and-authorization.md) | 会话撤销、能力授权、多租户隔离、故障拒绝 | 大部分已实现，见 current/16 |
| [03 模型目录与可用模型](design/03-model-catalog-and-availability.md) | 我的模型、大模型广场、供应商与模型的区别 | 部分 |
| [04 请求生命周期](design/04-request-lifecycle.md) | 请求、尝试、终态及流式异常 | 否 |
| [05 用量事件与日志](design/05-usage-events-and-logging.md) | 所有结果可追踪，使用量不再固定为聊天 token | 部分 |
| [06 计费与结算](design/06-billing-and-settlement.md) | 预占、价格快照、事务与幂等扣费 | 否 |
| [07 缓存与幂等](design/07-cache-and-idempotency.md) | 命中不重复收生成费用，重试不重复创建任务 | 部分 |
| [08 协议适配器](design/08-protocol-adapters.md) | JSON、SSE、multipart、二进制、WebSocket 与异步任务 | 部分 |
| [09 异步媒体与 Seedance](design/09-async-media-and-seedance.md) | 提交、查询、取消、回调、下载和结算闭环 | 否 |
| [10 扩展契约](design/10-extension-contract.md) | 扩展钩子、失败策略与权限边界 | 否 |
| [11 可观测性与报表](design/11-observability-and-reporting.md) | 历史归属、指标口径、时区、对账与运行手册 | 部分 |
| [12 实施迁移](design/12-migration-plan.md) | 阶段依赖、旧数据迁移、发布与回滚 | 否 |
| [13 测试与验收](design/13-test-and-acceptance-plan.md) | 最坏情况、并发故障、发布门槛 | 部分是验收目标 |
| [权限重构方案](design/permissions-plan.md) | 角色矩阵与全新建库的 DDL 草案 | 大部分已实现，见 current/16 |

配套：

- 数据契约：[请求事件](design/schemas/request-event.md)、[用量](design/schemas/usage.md)、[结算](design/schemas/settlement.md)、[异步任务](design/schemas/async-task.md)。
- 关键决策：[ADR-001 会话与权限](design/decisions/ADR-001-session-permissions.md)、[ADR-002 可用模型](design/decisions/ADR-002-available-model-definition.md)、[ADR-003 事实与结算](design/decisions/ADR-003-request-event-and-settlement.md)、[ADR-004 异步媒体](design/decisions/ADR-004-async-media-billing.md)、[ADR-005 供应商与模型](design/decisions/ADR-005-provider-vs-model.md)。

### 已落地的部分

审查提出的事情有一部分后来真的做了，读 `design/` 时容易误以为全都还没做。目前确认已实现的：

- **能力级授权**：`internal/authz/decide.go` 的 `Action` 集合与 `decide`，每个动作单独判定，不再散落 `role == "admin"` 字符串比较。
- **可撤销会话**：会话记录保存签发时的 `session_version`，角色每次请求从用户行读取；禁用、删除、改密码都会递增版本，使该用户已开的会话立刻失效。
- **按作用域收窄的列表**：由 `internal/gateway/visibility_chain_test.go` 和 `frontend/e2e/visibility-chain.spec.ts` 逐层验证。
- **用量写入的幂等**：`internal/iam/usage.go` 的 `RecordUsage` 在同一事务内用 `ON CONFLICT (request_id) DO NOTHING` 判断是否新事件，只有新事件才累加汇总与作用域支出。

仍然没有实现的：预算预占与结算账本（**没有** reservation/ledger；当前只有 `RecordUsage` 直接写用量与支出）、不可变的请求/尝试/结算事件、统一多维 usage、通用传输层、异步媒体状态机与 Seedance 验收。

## testdata/ — 测试基线数据

[catalog.json](testdata/catalog.json) 保存 LiteLLM 1.102.0 的结构化对照数据，共 779 条 HTTP 路由。

它放在 `docs/` 下面只是历史原因，实际上是测试夹具，被三处读取：`internal/gateway/catalog_reads_test.go`、`e2e/livesweep/main.go`、`frontend/e2e/coverage.spec.ts`。

保留它是因为覆盖检查需要一份固定的对照源；这不代表 XHub 支持 779 条生产协议。运行中的可用模型必须由授权与部署状态生成，不能读这个文件直接当作用户模型列表。它的 `implementation` 字段也不能替代端到端验证。

## 必须成立的约束

这六条是目标，不是对当前实现的保证。故障、未知用量和无法确认的上游结果必须显式表示，不能伪装成成功、零费用或空列表。

1. 用户只能看到和调用自己被授权、已发布且存在有效部署的模型；目录显示不授予调用权限。
2. 身份、权限、预算及强制限流无法确认时，禁止启动新的付费上游工作。
3. 每个已持久化接收的请求最终有唯一终态；每次上游尝试均可追踪，异步任务终态与提交请求分开。
4. 重试、队列重复、回调重复和多个 worker 并发不会导致重复结算。
5. 历史报表使用请求时的归属和价格快照；删密钥、改名字、转团队不改变旧账。
6. 新协议只有在授权、真实调用、用量提取、计费、错误和恢复均通过验收后才标记为生产支持。
