# 10 扩展契约

> **目标设计，不是当前行为。** 本文描述的是计划要实现的架构。现在实际运行的行为见 [docs/current/](../current/)。两份冲突时，以 `current/` 和代码为准。

状态：internal/plugin/registry.go 的 Extension 仅有 Name 和 BeforeUpstream(Call) Decision；没有 context、截止时间、响应/用量/结算阶段或版本。当前 Call 只含 Op、Model、Path。注册器可以作为原型，但不能承担完整权限或账务扩展。

## 生命周期与所有权

| hook | 可观察输入 | 允许的输出 |
| --- | --- | --- |
| OnRequestReceived | 去敏元数据与 request_id | 受限标签、额外参数验证 |
| OnAuthenticated | 不可变身份/租户及权限版本 | 拒绝、审计标签 |
| BeforeRoute | 已授权模型、操作与候选集合 | 在候选范围内排序/收窄 |
| BeforeUpstream | 已预占执行与去敏请求 | 拒绝、schema 内的参数转换 |
| AfterUpstream | 供应商结果与 attempt ID | 受约束错误/内容转换 |
| OnUsageExtracted | 测量项及证据 | 校验意见、额外版本化测量 |
| BeforeSettlement | 用量、价格快照与账务计划 | 拒绝或受控收费策略建议 |
| AfterSettlement | 已提交 settlement ID | 异步通知意图 |
| OnError | 去敏错误、阶段与关联 ID | 审计/告警意图 |

这些是目标钩子，并非实现声明。缓存路径跳过上游钩子，但仍执行权限及适用计费钩子；异步任务在任务 worker 上触发相应阶段。AfterSettlement 通过事务 outbox 至少一次投递，消费端以 settlement/event ID 去重。

身份、能力、候选授权集合、预算及结算键只能由核心服务控制。插件不得改 owner、补充未授权候选、读出凭据、直接写账本或把未知 usage 改成实测零。BeforeSettlement 的建议由 BillingService 按已发布规则验证，金额变化必须有 policy_version 和审计证据。已提交分录只能追加调整。

## 接口与执行约束

每个扩展声明稳定名称、契约版本、实现版本、阶段、输入/输出 schema、执行顺序、timeout、资源限制、fail_policy、需要访问的数据及外部副作用。使用 context.Context 与类型化只读输入，返回受限 Decision；不把数据库连接、可写身份或上游凭据传给扩展。

顺序按明确阶段和优先级固定，禁止依赖偶然注册顺序。候选收窄或参数变化后再次校验能力与费用上界。Header 输出走允许列表，不得设置 Authorization、Cookie 或覆盖 request_id 等权威字段。未知扩展版本或必需阶段缺失在启动/发布校验时拒绝。

记录 hook 名称/版本、时长、超时、结果、失败策略和 decision 摘要；敏感载荷不进入审计。外部通知不能在账务事务中阻塞，使用 outbox 和稳定幂等键。

## 失败策略

权限、强制治理、价格与账务校验失败或超时采用 fail_closed：上游前禁止付费发送；上游后暂停不确定结算并对账，不能把已经发生的工作抹掉。可选遥测可明确配置 fail_open，同时增加降级指标和告警，不能默认为所有扩展放行。OnError 本身失败要限制递归并记录核心日志。

超时 context 不能强制停止不合作的进程内 Go 扩展；仅开启 goroutine 并超时返回无法限制后台资源或外部副作用。进程内扩展必须可信且遵守取消契约；不可信/可上传扩展需要独立进程或其他受限运行环境。最坏情况保证以可强制的隔离边界为准，不能宣称 context 等价于沙箱。

## 迁移与验收

先将 BeforeUpstream 包装到版本化契约，明确原有扩展失败语义；加入记录和总时间预算，再接其他阶段。收费策略初期只做影子验证，不能和旧计费同时扣款。

验收包括：插件试图拓宽授权被拒绝；超时/异常遵循策略；多个 hook 不覆盖核心响应头；重复 AfterSettlement 无重复通知副作用；参数变化后费用仍不超过预占；不可合作扩展被隔离。风险是将扩展方便性当成允许越权，因此核心不变量必须独立于插件成立。
