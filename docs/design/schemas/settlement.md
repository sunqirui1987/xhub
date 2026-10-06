# 预占、结算和账本数据契约

> **目标设计，不是当前行为。** 见 [docs/current/](../../current/)。

状态：Proposed。schema_version=1；目标为本地事务中的权威财务事实。币种和金额精度明确，不以 float64 或 Redis 热支出充当权威账。

## 对象与字段

| 对象 / 字段 | 类型与要求 | 语义 |
| --- | --- | --- |
| reservation.reservation_id / execution_id | ID，必填 | 一次执行的预算风险预占 |
| reservation.account_scopes / period_ids | 有序 ID 集合，必填 | key/user/project/team/org 及各自预算周期 |
| reservation.currency / reserved_amount | 币种 / 精确金额 | 该执行可收费上界 R；各主体是约束投影，非多次收费 |
| reservation.state / version | 枚举 / CAS 整数 | held、partially_consumed、consumed、released；状态不确定仍 held |
| reservation.bound_evidence | 结构，必填 | 最大 token/数量/时长/重试及报价规则证明 |
| reservation.created_at / lease_until | UTC 时间 / 可空 | lease 是 worker 调度信息，不是免费释放依据 |
| price_snapshot.price_snapshot_id / price_version | ID / 字符串 | 提交时锁定的不可变报价 |
| price_snapshot.lines | 结构数组 | metric、unit、unit_price、quantity_basis、dimensions、包含规则 |
| price_snapshot.currency / rounding_rule | 字符串，必填 | 最小精度、逐行/逐单舍入顺序 |
| price_snapshot.policy_version | 字符串，必填 | 缓存、失败尝试、估算、退款和用户收费规则 |
| settlement.settlement_id / settlement_key | ID / 稳定键，唯一 | 初次结算身份，不随用量/价格修订变化 |
| settlement.charge_identity | 结构，必填 | execution_id、charge_kind、charge_scope |
| settlement.usage_id / price_snapshot_id | ID，可空 / ID | 已采用测量版本及原报价；待用量时 usage 可空 |
| settlement.state | 枚举 | reserved、pending_usage、pending_reconciliation、settled、released、adjusted |
| settlement.provider_cost / customer_charge | 分币种精确金额或 null | unknown 为 null；两者不能相互覆盖 |
| settlement.evidence_digest / settled_at | 摘要 / 时间可空 | 确认事实与首次结算时间 |
| ledger.entry_id / settlement_id | ID，必填 | 不可变金额分录及结算关联 |
| ledger.kind / amount / currency | 枚举 / 精确金额 / 币种 | debit、refund、adjustment、opening_balance；符号规则固定 |
| ledger.account_id / period_id | ID，必填 | 净费用账户与所属周期；约束投影单独存 |
| ledger.adjustment_id / original_entry_id | ID，可空 | 修订/退款的去重身份与原分录关系 |
| ledger.origin / scope_snapshot | 枚举 / 结构 | live、legacy、reconciliation；不可变历史归属 |
| outbox.event_id / aggregate_version | ID / 整数 | 已提交财务事实的可靠发布与投影去重 |

charge_kind 示例为 generation、cache_read；charge_scope 可为 customer 或某个 provider attempt 成本主体。不同供应商 attempt 的成本分开，用户重试收费按锁定政策合并或分项。各主体预算投影记录同一费用的约束，不作为新增收入分录再次汇总。

## 唯一键与原子性

首次结算唯一 (execution_id, charge_kind, charge_scope)；settlement_key 从该身份稳定生成，不包含 usage_version 或 price_version。更新测量只追加 adjustment_id 唯一的差额并沿用原报价；价格纠错需附授权、修正快照与原因，普通改价不重算历史。相同键不同摘要不能静默接受，需核对事实并进入待对账。

ledger.entry_id、adjustment_id、outbox.event_id 各自唯一；每种本次新增金额分录有稳定来源键。消费端只有 INSERT 真正新增后才更新聚合。约束建立在数据库，不依赖 worker 进程锁、Redis 去重或“先查再加”。

单事务锁定 execution、结算、预占和所有相关预算账户，写首次结算/差额、账本、预占变化、权威预算投影、必要任务/执行结算状态和 outbox；失败全部回滚。入口终态可能已经在异步提交时形成，不能为结算重写该终态。投影通知在提交后重放。

## 状态、一致性与边界

reserved → pending_usage/pending_reconciliation → settled 或 released；settled 后追加调整形成 adjusted 视图，原分录仍保留。released 要有确认没有剩余费用风险的证据；超时、进程退出和本地取消都不是充分证据。供应商成本未确定时仍可按政策确认用户收费，但必须保留成本 unknown 和相应风险，不能将其视为零。

准入事务检查 S+H+R≤B，提交后的预算账户维持 S+H≤B；结算实际可收费 C≤R。固定锁顺序并重试死锁；重复结算不再次减少 H 或增加 S。跨周期任务沿用提交时周期，若迁移需显式转移分录。退款是追加负向调整，不删除历史。

追加调整在锁内相对已入账净额计算，并以 adjustment_id 去重；负向退款不得超过可退净额。正向调整先消耗保留风险预占，新增风险需同事务预算校验；若修订超出已确认费用上界而余额不足，进入待对账，按明确政策处理平台承担或后续获准收费，不能直接破坏硬预算。供应商成本事实仍须保留，不以无法向用户收费为由抹除。

充分性：准入在互斥锁内保持预算不变量，结算增量 C−R≤0；唯一来源键让任意重复消息只增加一次金额。下界：无有限 C 上界则任何有限预占均无法保证不越界；供应商无幂等/查询时本地无法保证外部只执行一次，需 pending_reconciliation。

## 敏感字段、历史与验收

账本不保存凭据和内容载荷；快照中的必要个人标识受留存及读取审计控制。旧明细缺失标 legacy，期初余额与明细导入不能重复累计。报表按币种、提交快照、统一对账水位计算，不按当前价格重算。

验收覆盖跨进程/批内重复、同键冲突、事务每步故障、舍入、多主体预算、用量修订差额、未知保留预占以及 Redis 清空后的重建。参见 [账务设计](../06-billing-and-settlement.md)。
