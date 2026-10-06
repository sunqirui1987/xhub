# 04 请求生命周期

> **目标设计，不是当前行为。** 本文描述的是计划要实现的架构。现在实际运行的行为见 [docs/current/](../current/)。两份冲突时，以 `current/` 和代码为准。

状态：数据面有认证、重试、缓存和成功响应记录；尚无覆盖全部失败路径的持久化状态机。证据：internal/dataplane/serve.go、internal/gateway/limits.go、spend.go 和 ingress.go。目标是给每次入口请求和每次外部尝试建立可恢复关联，不把成功支出日志当成请求全集。

## 状态与对象

正常阶段：received → authenticated → authorized → model_resolved → deployment_selected → budget_reserved → upstream_started → upstream_finished → usage_extracted → settled → responded。

阶段不是强制成功顺序：缓存命中跳过上游；预检查失败直接终结；异步提交在 task_accepted 后结束本次入口请求，任务随后独立结算。请求终态为 succeeded / rejected / failed / cancelled / interrupted；计费终态由 settlement_state 单独表达，不能因 HTTP 200 就推断已结算。

request_id 标识一次入口接收；attempt_id 标识一次具体上游发送；task_id 标识持续媒体工作；idempotency_key 标识用户要求只执行一次的业务操作。重复 HTTP 请求可以有不同 request_id，但关联同一 execution_id / task_id 及结算键。

## 接收与持久化边界

入口生成 request_id，写入响应头和追踪上下文。完成身份解析后，将请求接收记录与必要快照持久化；在该记录提交前禁止外部付费工作。认证失败也尝试记录去敏后的拒绝事件，不保存原令牌，未知主体用匿名标记。

数据库不可用时返回 503、request_id，并写运行日志/指标；不能保证此时存在业务事件。唯一终态保证限于已持久化接收的请求，并依赖存储最终恢复及恢复 worker 工作。极端存储永久损毁属于恢复能力之外，需备份策略，不能声称无条件“每个网络请求永不丢失”。

请求起始记录可更新阶段；终态快照提交后不可覆写。补充用量、对账和调整通过追加版本事件/分录实现。[数据结构](schemas/request-event.md) 定义 request、attempt 与最终事件的唯一约束。

## 上游执行与重试

启动前重新校验有效授权、部署/凭据版本和预算预占；持久化 attempt intent 后再发送。每次尝试记录 provider、upstream_model、deployment、操作、时间和失败类别。

只有能确认未产生外部副作用的错误，或供应商支持并实际使用相同幂等键的调用，才能自动重试。连接超时、响应丢失或流中断不等于供应商没执行；无法确认时进入 outcome_unknown，交由查询/对账，不盲目换部署重复生成。

可重试时有总截止时间、次数及累计预占上界；每次新尝试的潜在费用都要计入。不能用“仅最后一次返回成功”抹去前几次已计费用量。网关给用户的计费策略和供应商实际成本分别记录，见 [结算方案](06-billing-and-settlement.md)。

## 流式与取消

首字节前失败可以设置明确 HTTP 错误；发送 SSE/二进制头之后不能改写 HTTP 状态，应发送协议允许的错误事件并终止流，日志记录 interrupted、字节/帧数量和供应商错误。HTTP status=200 与 operation_outcome=failed 可以同时存在，报表以业务终态统计成功。

TTFT 定义为上游发送至首个有效内容事件，而不是任意心跳或响应头；同时记录网关接受至首内容的用户感知时延。客户端断开触发取消，但取消成功只代表本地停止等待，供应商是否停止要独立确认。用量缺失标 unknown；请求断开后仍执行可恢复结算，不能依赖已取消的请求 context 完成持久化。

## 故障恢复与保证

持久化 outbox 驱动恢复任务；worker 按租约和 CAS 领取过期非终态请求/attempt，恢复记录、查询上游和结算。received 且未启动的请求可确定为失败并释放预占；已发送无法确定结果时终结入口为 failed/interrupted，财务状态为 pending_reconciliation，保留预占等待调查。

本地终态 CAS + 唯一 request_id 约束使多个终结者至多生成一个终态。恢复 worker 在存储恢复后持续扫描，保证已接收请求不会因进程退出永久停留在处理中。外部结果未知仍可形成唯一入口终态，但不能伪造完成用量或释放所有费用风险。

## 迁移与验收

先添加接收/attempt 表和统一错误出口，再接入成功、失败、缓存、流式路径；最后用终态事实替换旧成功支出日志。所有 handler 共用 recorder，不复制各自统计代码。

验收包括：认证失败、权限拒绝、路由缺失、限流、预算不足、上游 4xx/5xx、重试、流中断、客户端取消、成功后落库失败和进程在发送前后退出。每个已接收 request_id 最终唯一终态；每次外部发送能找到 attempt intent；外部可能已执行的错误不能无条件重试。
