# 请求、执行与事件数据契约

状态：Proposed。schema_version=1；这是目标数据契约，不是当前 Go struct 或已发布接口。时间均为 UTC，标识为服务端生成的不可猜测 ID；面向用户的读取必须经过作用域授权。

## 对象与字段

| 对象 / 字段 | 类型与要求 | 语义 |
| --- | --- | --- |
| request.request_id | ID，主键 | 一次入口请求；HTTP 重放产生新 ID |
| request.execution_id | ID，可空 | 共享的业务执行；预检查失败可以未创建 |
| request.schema_version | 整数，必填 | 记录的解释版本 |
| request.principal_snapshot | 结构，必填 | kind、subject_id、session_id/key_id、认证方式；匿名拒绝用明确 anonymous |
| request.scope_snapshot | 结构，必填 | tenant/org/team/project/user 的 ID 和当时显示名；未解析字段注明 unknown |
| request.authorization_snapshot | 结构，可空 | action、decision、reason_code、policy/permission_version、准入线性化时间 |
| request.model_snapshot | 结构，可空 | alias、公开名称、operation、model_version；部署和供应商实际选择在 attempt |
| request.received_at / accepted_at | 时间；后者可空 | 网络接收与持久化接收提交时间分别记录 |
| request.phase / version | 枚举 / 单调整数 | 可更新处理中阶段和 CAS 版本 |
| request.terminal_status | 枚举，可空 | succeeded、rejected、failed、cancelled、interrupted |
| request.http_status / delivery_outcome | 整数可空 / 枚举 | 实际发送状态码；not_started、partial、completed、unknown |
| request.error | 结构，可空 | 稳定 error_code、去敏摘要、retryability；不保存原认证头 |
| request.cache_status | 枚举 | miss、hit、bypass、not_applicable；重放另行标记 |
| request.replay_of_execution_id | ID，可空 | 返回已有结果，不能另建生成收费身份 |
| request.terminal_event_id | ID，可空，唯一 | 指向不可变入口终态事实 |
| execution.execution_id | ID，主键 | 一个逻辑生成/工作；可关联多个 request 和 attempts |
| execution.idempotency_scope / key_digest | 结构 / 摘要，可空 | tenant、subject、operation；原始业务键按策略保护 |
| execution.payload_digest | 摘要，必填 | 规范化输入和影响执行的参数摘要；不含明文内容 |
| execution.policy_snapshot / price_snapshot_id | 结构 / ID | 重试、用户收费、预算与报价版本 |
| attempt.attempt_id / execution_id | ID，主键 / 外键 | 一次具体上游尝试 |
| attempt.ordinal / intent_at | 整数 / 时间 | 在发送前持久化的尝试序号与意图 |
| attempt.deployment_snapshot | 结构，必填 | deployment_id/version、provider、upstream_model、adapter/version、operation、transport |
| attempt.credential_reference | 受控 ID/version | 只保留可审计引用，不保存秘密 |
| attempt.provider_request_id | 字符串，可空 | 供应商局部标识，非全局唯一键 |
| attempt.outcome | 枚举 | not_sent、in_progress、succeeded、failed、cancelled、interrupted、unknown |
| attempt.sent_at / first_content_at / finished_at | 时间，可空 | 无法证实已发送时不伪造 sent_at，保留 unknown |
| event.event_id / subject_id / sequence | ID / ID / 整数 | 追加事实，subject_type 为 request、execution、attempt 或 task |
| event.kind / occurred_at / recorded_at | 枚举 / 时间 / 时间 | 事件类型、事实时间与落库时间分开 |
| event.payload / evidence_digest | 受控结构 / 摘要 | schema_version 决定结构；用量及结算保存引用 |

内部 deployment、credential、session 引用仅服务端可见；对外 DTO 使用字段白名单。快照保留历史归属，当前实体删除不得使旧记录无法查询或改变归属。合规脱敏以独立策略和审计执行。

## 状态与唯一约束

处理中 phase 按 [请求生命周期](../04-request-lifecycle.md) 推进；缓存和异步接受可以跳过不适用阶段。入口终态不可覆写。异步 submit 的 succeeded 仅表示提交操作完成，不代表生成任务成功；task 状态另存。财务状态来自结算投影，不混进 terminal_status。

数据库约束：request_id、execution_id、attempt_id、event_id 为各表主键；attempt 唯一 (execution_id, ordinal)；event 唯一 (subject_type, subject_id, sequence)；每个 request 最多一条 terminal event（独立终态表或等效唯一约束）。存在幂等键时唯一 (idempotency_scope, key_digest)，冲突后核对 payload_digest；不同内容返回 409，不覆盖原执行。

依赖读错用 error 明确表达，不能补空授权。过期非终态由恢复 worker CAS 终结。事务将终态事件、状态投影和必要 outbox 一起写入；结算以 [结算契约](settlement.md) 的事务边界为准。

## 一致性与保证

一个请求可以没有用量、多次 attempt 可以产生多份供应商成本，但 HTTP 重放不再生成同一业务工作。一份 task 的后续查询是新 request，不能当成新视频生成。业务结果与客户端交付分离：上游成功而客户端断线时保留已执行事实并标交付异常。

本地唯一约束/CAS 保证至多一个终态。持久化接收后恢复扫描在数据库最终恢复、worker 最终运行时补齐终态；入口持久化之前的故障只有运行日志，不能承诺完整业务事件。供应商未知结果保留 pending_reconciliation，不用入口失败推断无费用。

## 敏感字段与读取

禁止 Authorization、Cookie、API key、签名 URL 和完整 prompt 默认入表。显示名与错误文本限制长度并转义；详细内容另存受控加密载荷，保留期独立。按 tenant 和已授权 self/team/all scope 查询，数据库分页；request_id 本身不是读取凭证。

契约验收见 [测试计划](../13-test-and-acceptance-plan.md)。
