# 09 异步媒体与 Seedance 2.0

状态：目录中有 seedance2 系列价格条目（internal/catalog/publicdata/model_cost_map.json），这不证明已实现真实任务协议。本次没有查验具体供应商最新接口或进行付费调用；下文是待实施框架与接入门槛，不提供猜测的 Seedance URL、字段或认证方式。

## 对象和接口

视频生成是有外部副作用的长任务。一次提交 HTTP request 的成功只是接受任务；任务完成和费用结算有各自状态。task_id 对外稳定，external_task_id 保存在受控内部映射中，不能让调用者直接选择任意供应商任务。数据结构见 [任务契约](schemas/async-task.md)。

```go
type AsyncMediaProvider interface {
    Submit(context.Context, MediaRequest) (Task, error)
    Get(context.Context, string) (TaskStatus, error)
    Cancel(context.Context, string) error
    ExtractUsage(TaskStatus) Usage
}
```

接口草案中的 Task/TaskStatus 是标准化内部类型。注册契约声明上传、回调验签、下载、幂等、查询和取消的实际支持情况；不支持 Cancel 必须返回明确错误，不能假装取消成功。

## 状态机

执行状态和 settlement_state 分开保存：

```text
submission_intent → submitting → queued → running → succeeded
                                   └──────────────→ failed / cancelled
submitting 或运行中无法确认结果 → unknown → 查询/人工对账恢复
cancel_requested → 上游确认后 cancelled；也可能继续 running/succeeded
```

queued 可直接进入 succeeded；已确认终态不被迟到 running 覆盖。unknown 不是供应商终态，是本地知识不足，可经新证据恢复。任务执行终态确定后，仍可能 pending_usage 或 pending_reconciliation，不能以“视频生成成功”推断已收费。

建议网关契约为 POST /media/tasks、GET /media/tasks/{task_id}、POST /media/tasks/{task_id}/cancel、GET /media/tasks/{task_id}/outputs/{asset_id}，尚非已实现接口。提交返回 202、task_id、状态查询入口、request_id；业务失败、未知状态、财务待对账均有可读状态。查询和取消检查当前身份、租户、任务归属及相应能力；模型撤销后可按明确策略继续查看已有合法任务，但禁止新提交。跨租户 ID 返回不泄漏存在性的错误。

## 提交流程和费用

1. 校验模型操作权限、输入资源归属、参数与费用上界；同键不同摘要为 409。
2. 同一数据库事务创建 execution、预占、submission_intent、任务和 outbox；重复提交引用同一任务。
3. worker CAS 领取并写 submitting/attempt intent；使用稳定供应商幂等键提交。
4. 持久化 external_task_id 后进入 queued/running，由回调或轮询推进。
5. 成功提取真实时长/数量/规格用量，以提交时价格版本结算；失败/取消核对供应商收费政策后释放未消费预占，必要时结算已发生费用或追加退款。

响应丢失且无供应商幂等/请求查询时标 unknown，保留预占并启动对账。预占过期、进程退出或取消请求发送成功都不能证明外部未执行。实际费用无有限上界的操作不能承诺硬预算，见 [预算证明](06-billing-and-settlement.md)。

## 资产与输出

参考图片、音频和视频先形成有 owner/scope、类型、大小、摘要及保留期的资产。上传权限与生成权限独立校验；上传成功不自动授予其他租户读取。适配器按照已核验供应商要求上传字节或引用资源，不能把所有 provider 都假设成接收 URL。资源拉取执行地址与重定向策略，避免服务器代取不受控内部地址。

输出记录 MIME、时长、规格、摘要、存储位置、来源和过期时间。下载通过受控代理或短期授权 URL，发放前检查权限；日志不保存签名 URL。输出 URL 到期不能把已完成任务改成失败，只标 asset_expired，并按政策重新获取。

## 轮询、回调与恢复

轮询使用数据库 lease_owner/lease_until 与 version CAS，带指数退避、jitter、总查询截止时间和供应商速率限制；租约过期允许接管，但不允许重新 Submit。超出最长等待时间标待核查并告警，不能自动当成免费失败。

回调先核验原始字节签名、签名版本、时间窗口和重放标识，映射受控任务；无签名机制的 provider 不开放可信回调推进，改用已认证查询。回调事件持久化与任务 CAS/outbox 在事务中处理，提交后才应答成功。乱序按供应商事件序号或主动查询纠正，重复回调不重复用量/结算。

回调与轮询同时观察成功时，唯一结算键及终态 CAS 保证本地至多一次结算；状态记录不会倒退。供应商后来修订账单以版本化用量及差额调整处理，不改原事实。查询与下载的请求各自有入口事件，但不能每查一次就重复累计生成费用。

## Seedance 2.0 接入检查点

必须明确接入哪个供应商、服务区域、模型版本和文档版本；同名模型在不同渠道可能有不同协议、收费与限制。逐项取得以下证据：

- 正式文档确认认证、提交/查询/取消/上传/下载方式、任务枚举和错误码。
- 官方计费口径与真实响应对应，确认时长、分辨率、音轨、数量以及失败/取消费用。
- 明确幂等与响应丢失恢复能力；不支持时验证 unknown 流程。
- 授权资产完成一次沙箱生成，核对输出与费用；验证回调重放、轮询竞争及进程崩溃恢复。

发布顺序为 disabled → experimental（限制账户和预算）→ verified。未满足上述条件前，前端不能把价格条目当成可用视频模型。本次文档完成不改变接入状态。

## 验收与下界

同键跨进程并发只创建一个 execution；任务查询/取消/下载不能越权；回调和轮询竞争只产生一份结算；未知费用不提前释放预占。外部发送后本地崩溃且供应商无幂等/查询时，无法同时保证自动完成和绝不重复执行，方案选择保留 unknown 等待证据。数据恢复依赖数据库最终恢复、worker 持续运行及供应商可查询性。
