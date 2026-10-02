# 异步媒体任务数据契约

状态：Proposed。schema_version=1。本契约描述网关任务，不假定 Seedance 或其他供应商采用同一字段、URL 或状态枚举。

## 字段

| 字段 | 类型与要求 | 语义 |
| --- | --- | --- |
| task_id / schema_version | ID / 整数，必填 | 网关公开任务 ID 和结构版本 |
| execution_id / submit_request_id | ID，必填 | 业务执行与首次提交入口 |
| owner_snapshot / scope_snapshot | 结构，必填 | 提交主体与租户/组织/团队/项目归属 |
| authorization_snapshot / model_snapshot | 结构，必填 | 提交时能力、作用域、alias、operation 与策略版本 |
| deployment_snapshot | 受控结构，必填 | provider/account、upstream_model、adapter/version、部署版本 |
| credential_reference | 受控 ID/version | 恢复查询所需引用；不保存秘密 |
| external_task_id / provider_idempotency_digest | 受控字符串，可空 | 外部任务映射与稳定提交键关联 |
| input_asset_ids / input_digest | ID 集合 / 摘要 | 输入归属与内容/参数一致性校验 |
| state / version | 枚举 / CAS 整数 | 标准化执行状态与并发保护 |
| last_confirmed_provider_state | 字符串，可空 | 已核验供应商状态映射来源 |
| progress | 数值，可空 | 仅供应商确认时展示，null 不猜为 0 |
| cancellation_state | 枚举 | none、requested、confirmed、unsupported、failed；与执行状态分离 |
| created_at / submitted_at / completed_at | UTC 时间，可空 | 各阶段证据时间，submitted 不代表已完成 |
| last_observed_at / next_poll_at | UTC 时间，可空 | 观测新鲜度和退避调度 |
| lease_owner / lease_until / fencing_token | 内部字段 | worker 调度、接管与拒绝旧 worker 写入 |
| callback_cursor / evidence_digest | 受控字符串 / 摘要，可空 | 事件顺序/去重与观测证据 |
| submission_attempt_ids / usage_ids | ID 集合 | 实际发送意图和资源测量 |
| reservation_id / price_snapshot_id | ID，必填 | 费用上界预占与提交时报价 |
| settlement_state / settlement_id | 枚举 / ID可空 | 账务投影视图，不代替执行状态 |
| output_assets | 受控资产引用集合 | asset_id、MIME、大小、时长、规格、摘要、过期状态 |
| error_code / error_summary | 字符串，可空 | 去敏错误和人工核查原因 |

asset 的 owner/scope、存储定位、保留期与访问状态独立保存。对外 DTO 不返回凭据、内部部署、供应商账户、lease 或长期私有资源链接；下载入口重新授权并生成短期链接或代理。

## 状态与转换

state 为 submission_intent、submitting、queued、running、succeeded、failed、cancelled、unknown。unknown 描述本地不能确认结果，可通过新证据恢复；执行终态为 succeeded/failed/cancelled。queued 可以直接完成，供应商状态映射必须由适配器契约定义。

取消请求更新 cancellation_state=requested，执行仍可 running/succeeded；只有供应商确认才进入 cancelled。取消不受支持返回明确错误；不能为了满足 UI 随意设置终态。已确认终态不被迟到 queued/running 覆盖。供应商若有正式修订机制，保存异常修订证据进入对账流程，不能直接把旧终态覆写。

任务成功但用量未知时 settlement_state=pending_usage；输出 URL 到期只改变资产状态。GET 查询和下载请求各自产生 request 事件，复用 execution/task，不增加生成计量或收费身份。

## 唯一约束与并发

task_id 主键；execution_id 在单任务提交契约中唯一，批量父任务需另行版本化。非空外部映射唯一 (provider, provider_account_id, external_task_id)；供应商 ID 不用于跨账户全局去重。业务幂等约束在 execution 上，冲突摘要返回 409。

回调事件唯一 (provider, account, external_task_id, callback_event_id)，无稳定事件 ID 时使用经验证的序号/摘要方案；不以到达时间假定供应商顺序。查询和回调推进使用同一 version CAS；签名原始字节、时间窗口和重放检查成功后，事件/状态/outbox 同事务提交，才返回回调确认。

worker 接管带 fencing_token，旧持有者的写入被拒绝。但本地 fencing 不能阻止旧 worker 的外部网络副作用；提交只在实际供应商幂等保护下自动重试。过期 lease 允许查询接管，不能证明可安全再次 Submit。结算唯一约束在 [结算契约](settlement.md)。

## 恢复与保证

发送前创建 intent；发送后未保存 external_task_id 时，通过供应商幂等查询或请求查询恢复，不能恢复则 unknown。保留预占，恢复 worker 退避查询并告警；达到调查截止时间不自动记免费失败。

任务查询、取消、下载检查当前读取/操作能力和归属；提交时授权允许后台完成记账，不给被撤销用户新的下载权。凭据轮换时用明确受控替代关系恢复，不尝试任意其他租户凭据。

本地唯一 task/CAS/结算约束在回调与轮询竞争下提供唯一事实；最终完成依赖数据库、worker 和供应商查询最终可用。无供应商幂等/查询时不能同时保证外部绝不重复和自动重试完成，必须显式保留 unknown。

## 敏感字段与验收

不保存参考资源签名 URL、认证头和明文内容到日志；原始回调载荷按受控留存单独加密。验收包括跨租户 ID、提交丢响应、回调重放/乱序、轮询竞争、旧 worker、取消竞态、资产过期、费用修订及崩溃恢复。Seedance 发布还需实际 provider 文档和沙箱证据，见 [异步媒体方案](../09-async-media-and-seedance.md)。
