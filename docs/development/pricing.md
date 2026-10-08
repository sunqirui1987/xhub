# 计价与用量

[开发指南](README.md) · [路由](routing.md) · [运行限制](runtime.md)

## 价格来源与单位

一次调用先尝试部署价格，再按上游模型 ID 和公开模型名查询目录。部署费率表和旧扁平单价都走同一计价实现。缺失价格与显式零价不同：查不到有效费率不能解释为供应商免费，运营需要核对部署价格。

`catalog.Rate` 用 `rates[]` 表达价格：

| 字段 | 含义 |
| --- | --- |
| `measure` | `token`、`second`、`picture`、`query` |
| `side` | 输入、输出、缓存读写、批量输入输出等侧 |
| `variant` | uncached/cached、分辨率、思考模式等限定词 |
| `window` | `peak`、`offpeak`、`all` |
| `unit_size` | 来源报价覆盖的基础单位数量 |
| `usd` | 已换算的每个基础单位美元单价 |

**计算使用 `quantity × usd`，不能再除一次 `unit_size`。** 控制台按每百万 token 展示时，显示单位与底层每 token 单价之间也要明确转换。旧扁平字段保留用于兼容读取，可能显示最低档；有时段的实际调用按当前适用费率计算。

示例：来源报价 $2 / 1,000 tokens，存储 `unit_size = 1000`、`usd = 0.002`。用量 1,500 tokens 的费用是 $3。

## 用量与时段

`CostAt(model, usage, startedAt)` 用调用开始时刻确定窗口。`Usage` 包含输入、输出、缓存读写 token，以及图片数、秒数和搜索次数。适配器支持和真实返回的计量单位决定哪些维度可用。

显式报告的 0 不触发估算，缺失字段才补估算；输出上限 `max_tokens` 不能当作已消耗输入。缓存读 token 是输入 token 的子集，命中部分按缓存价，其余按非缓存输入价，不能重复累计。非 token 数量不能伪装成 token。

对含 peak/offpeak 费率的模型，窗口使用 `Asia/Shanghai`。通常工作日的 `[09:00,12:00)` 和 `[14:00,18:00)` 是高峰；周末和中国法定节假日是空闲，调休工作日按节假日数据覆盖。整点边界用半开区间，跨窗口调用仍以开始时刻为准。

节假日来自 `internal/catalog/publicdata/cn_holidays.json`，需要维护年份。费率优先选适用时段，再退到 `all`；变体缺少准确匹配时可能退到可用候选，`fallback` 标记随快照保留。不能把这些降级候选当作已经测量到的模态或分辨率。

## 历史价格快照

`Charge.Window` 和 `Charge.Applied` 保存本次使用的费率、数量及降级标记；`price_snapshot` 随 usage 入库。日志详情优先读快照，模型改价不重写历史金额。没有快照的旧记录退回重算，并标明 `source: "recomputed"`。

输入输出费用、缓存字段的显示词由 `frontend/src/lib/rateDisplay.ts` 统一。控制台当前可编辑扁平价格字段，完整 `rates[]` 表需要配置或接口维护。

## 持久化链路

1. `gateway.RecordSpend` 根据开始时刻和用量计算费用、价格快照及历史归属。失败和完整响应缓存命中的本地新增费用为零，供应商实际费用仍需对账。
2. Redis 可用时 `EnqueueSpend` 的 Lua 同时更新热支出、追加队列和登记 `request_id`；重复身份不会二次入队或增加热支出，首次 payload 生效。Redis 不可用时尝试 PostgreSQL 即时写入。
3. 刷写顺序是 `PeekLogs → IAM.RecordUsage → AckFlushed`。PostgreSQL 事务插入 usage、日志、每日聚合和主体支出。`request_id` 唯一约束与 `ON CONFLICT DO NOTHING` 使只有新增事件增加费用。非法批在写入前拒绝，整批失败回滚。
4. 数据库提交后才确认 Redis。确认失败不会在 PostgreSQL 二次记账，但数据库支出与尚未扣除的热支出可能暂时重复显示。

历史身份随 usage 记录保留，不从今天的密钥归属补出旧事实。会话汇总按调用方和 session 收窄，同名 session 不应合并不同调用方的记录；旧数据缺少调用方标识时无法恢复这一边界。

## 官方任务结算

每次查询都有独立 callID，临时日志和元数据不共享。成功终态且有正用量时，最终账务身份使用调用方、transport、task ID 和部署范围的稳定摘要；重复完成查询经 Redis 和 PostgreSQL 去重。创建、等待、失败或零用量查询不推断生成费。任务查询不重复增加同步生成的路由 TPM。

结算不依赖预先写入的 billed 标记；第一次持久化失败，后续完成查询仍可重试。重复完成查询折叠为一条账务记录，不能据此统计全部 poll 次数。

## 不能保证的部分

- 当前没有预算预占或结算账本，并发、流式或长任务可能超过预算。金额使用 `float64`，没有统一固定精度会计规则或退款调整分录。
- `RecordSpend` 不向 HTTP 调用方返回持久化确认；Redis 和 PostgreSQL 都写入失败时，已成功的上游响应仍可能返回。需要运行监控和供应商对账。
- Redis 去重状态不自动过期，会持续增长；队列恢复依赖 Redis 持久化与部署方式。没有数据库 outbox 或跨存储事务。坏队首可能阻断刷新，多个刷新者仍需要协调批次。
- 图片数、秒数、搜索次数可参与计价，但 `usage_events` 没有独立完整的多维数量列；部分证据只在价格快照中。缓存创建 token 没有独立报表列。
- 没有逐尝试费用账本、完整 unknown/partial 用量状态，也没有独立的供应商成本与客户收费账本。成功响应费用不能核准所有失败重试和中断输出的供应商收费。

## 代码与验证

| 行为 | 实现入口 | 相关测试 |
| --- | --- | --- |
| 费率解析、窗口、计算 | `internal/catalog/rates.go`、`cost_at.go`、`holiday.go` | `cost_at_test.go`、`holiday_test.go`、`usage_regression_test.go` |
| 部署价格与快照 | `internal/gateway/spend.go`、`internal/gateway/usage/reports.go` | `internal/gateway/usage/cost_breakdown_test.go`、`internal/regression/pricing_test.go` |
| 数据库去重与事务 | `internal/iam/usage.go` | `usage_idempotency_test.go` |
| Redis 入队与确认 | `internal/live/redis.go` | `redis_test.go` |
| 官方任务结算 | `internal/dataplane/official.go`、`internal/gateway/spend.go` | `official_settlement_test.go` |
| 完整调用计价 | `internal/regression` | `pricing_test.go` |

相关测试入口存在不代表本次修改已经执行；检查报告应列出实际运行结果。
