# 11 可观测性与报表

> **目标设计，不是当前行为。** 本文描述的是计划要实现的架构。现在实际运行的行为见 [docs/current/](../current/)。两份冲突时，以 `current/` 和代码为准。

状态：已有支出日志及每日活动报表；internal/gateway/usage/activity.go 的 loadActivity 忽略 ListSpendLogs/ListKeys 错误，selectActivity 用当前 key 补历史归属、以当前 catalog.CostMap 推 provider，再把全部记录载入进程聚合。这会混淆历史身份、数据失败与真实零值，规模增大时也不能稳定分页。

## 事实与口径

请求量来自唯一入口终态事实；上游尝试量来自 attempt；生成任务量来自唯一 execution/task；费用来自已提交账本分录。一次请求多次尝试、一次任务多次查询、多个幂等重放请求分别计入相应指标，不能统称 api_requests 后求同一个成功率。

| 指标 | 分母/来源 | 规则 |
| --- | --- | --- |
| 入口成功率 | 已终结入口请求 | rejected/failed/interrupted 独立，异步 accepted 不等于生成成功 |
| 上游成功率 | 实际发送 attempt | 跳过部署不计发送，未知单列 |
| 视频完成率 | 已确认任务终态 | unknown 与进行中另列，不从分母无声删除 |
| 生成用量 | 测量事实 | 按单位和质量分组，未知不当成 0 |
| 用户净收费 | customer_charge 账本 | 收费减退款，分币种 |
| 供应商成本 | provider_cost 事实/分录 | 与用户收费分开，未知成本另列 |
| 缓存命中率 | 具备缓存资格的请求 | 网关响应缓存与供应商 prompt cache 分开 |

TTFT 以首有效内容为准，时延区分入口、上游、排队、完整任务及下载。异步 Submit HTTP 202 的几百毫秒不作为视频生成耗时。成功 HTTP 200 的断流以业务 interrupted 统计。

## 历史快照与查询

请求/执行时保存 key_id/key_hash/key_alias、user_id/team_id/organization_id/project_id、provider/model_alias/upstream_model/operation、price_version/unit/unit_quantity、status/error_code/cache_status。名称显示使用 snapshot，当前名称可作独立字段，不能重写历史分组。密钥删除、改名、团队转移不移动旧账；历史数据访问仍按当前授予的报告作用域和历史记录归属过滤。

报表请求必须经服务端能力授权，客户端 user_id/team_id 只是范围收窄参数。个人、团队和管理员报表采用不同 scope，禁止任意查询参数扩大访问范围。内容日志与财务报告权限分开。

数据库进行筛选、分页和聚合；稳定排序键使用时间 + 唯一 ID，日志默认游标分页。日期输入明确 IANA 时区，换算成 UTC 半开区间 [start,end)，支持夏令时，拒绝非法时区和倒置区间。聚合响应带 as_of、source_watermark、schema_version、unknown_count、pending_amount，区分实时近似投影和账本确定值。大规模导出限制范围并使用有权限的异步导出任务。

同一费用投影在 key/user/team/org 下是多维切片，不可把这些行相加当成总费用。request/attempt/usage 多对多联接必须先按事实键聚合，避免行乘积重复累计。分页总数与记录来自同一快照/水位，防止并发写入导致口径漂移。

## 运行指标与对账

监控至少覆盖授权依赖错误、会话撤销命中、可用模型无 witness、接收记录/终态落库失败、活跃预占、超龄 unknown、结算冲突、outbox backlog/age、投影 lag、usage 缺失、任务轮询 lease、回调验签失败及 transport 断流。以低基数 model/provider/operation/status 作指标标签，request_id/task_id 放日志与追踪，避免高基数指标失控。

对账按币种/周期检查：账本净额与权威聚合相等；每个结算对应已确认 execution 和价格快照；每个预占在活动任务或待核查列表中有归属；Redis 投影与数据库版本一致；供应商账单差异生成调查及追加 adjustment，不能手改历史金额。

运行处置：授权数据库不可用停止新付费准入；outbox 延迟优先修复投影而不重扣款；unknown 超龄查询供应商并记录证据；回调签名失败保留去敏事件并改用可信查询；usage 不合法暂停结算并调查。告警阈值和 SLO 由上线容量测量确定，不在设计阶段把任意数字当作既成承诺。

## 故障、迁移与验收

数据库查询错误返回 503/report_unavailable 和 request_id，不能返回成功空列表。授权无法确认则拒绝读取；可选缓存报告必须带陈旧标识且满足同样权限检查。

新旧报表在固定水位比较请求数、费用、单位与未知数；旧缺失维度标 legacy/unknown，不用当前 key/价格推造历史事实。验收：删 key/改价/转团队前后旧报表不变；跨租户读取失败；午夜/夏令时边界无重漏；失败请求与异步查询不重复统计生成；账本对账差异可定位到分录。风险是旧数据无法还原，该信息下界必须通过未知标识呈现。
