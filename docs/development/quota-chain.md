# 额度与分钟限额：实现链路和验证

面向维护组织、团队、个人和 API Key 的开发者。用户设置方法见 [额度管理帮助](../quota-management.md)。金额、RPM、TPM 使用同一条唯一归属链，三个维度分别校验。

## 不变量

1. 个人最多属于一个团队，API Key 从个人分配；服务 Key 使用团队业务账号承接个人层。
2. 固定分配保留未使用容量，留空共享上级未保留容量，零禁用。共享中间层透传固定后代，不能扩大上级容量。
3. 管理请求的金额、RPM、TPM、角色和其他字段整体提交；分配失败全部回滚。
4. 分钟限额全部层、全部维度通过后才原子计数。限流拒绝不扣量，不调用供应商。
5. 退出团队解除后续归属和未用保留，不退回旧团队历史账单或当分钟计数。独立个人 Key 保留，显式绑定原团队的 Key 撤销，不使用 BudgetDetached。

## 页面到持久化

| 层级 | 页面实现（frontend/src/components 下） | 真实接口 |
| --- | --- | --- |
| 组织 | organization/org-create/OrgCreateDialog.tsx、org-settings/OrgSettingsForm.tsx | POST /organization/new、PATCH /organization/update |
| 团队 | Teams.tsx、team/TeamInfo.tsx | POST /team/new、POST /team/update |
| 个人 | CreateUserButton.tsx、用户详情 UserAccountEditor.tsx、team/TeamMemberTab.tsx | POST /user/new、POST /user/update、POST /team/member_update |
| API Key | organisms/create_key_button.tsx、templates/key_edit_view.tsx、templates/key_info_view.tsx | POST /key/generate、POST /key/update |

页面共用 `shared/QuotaGuide.tsx`。团队成员入口修改同一份个人限额，不建立第二份团队内个人配置。文案和业务错误通过 i18n 翻译。

保存链路：权限判断 → `internal/httpx/budget.go` 输入校验 → 可空字段转换 → IAM 事务写入 → `validateQuota` → 提交。组织/团队/用户路由在 `internal/gateway/identity/{handlers,members}.go`，Key 路由在 `internal/gateway/keys/{generate,admin}.go`；IAM 写入在 `internal/iam/{users,teams,keys}.go`。

创建时未设置为 null；更新时未提交字段保留原值，显式 null 清空。RPM/TPM 只接受 0–2,147,483,647 的整数，非法类型、负数、分数和溢出不能被解析为不限额。

`internal/iam/db.go` 的 `tx` 在行锁之前取得当前 schema 的 PostgreSQL 事务级 advisory lock，串行化管理写入和结算。两个管理员同时分配兄弟容量时，后一个事务读取前一个提交结果。锁范围是 schema，不是单个组织；吞吐需要另行评估。

`internal/iam/quota.go` 的 `validateQuota` 从变更节点向上校验，递归检查下级。RPM/TPM 使用 `rateAllocated`，金额使用 `quotaUnused`。超配返回 400 `quota_allocation_exceeded` 或 `rate_allocation_exceeded`，事务回滚；页面保留输入供修正。修改限额的权限不会额外授予账号角色或模型权限。

## 配置公式

对某个 RPM 或 TPM 维度，固定上限为 L(v)，向父级占用为 A(v)：

```text
共享节点：A(v) = Σ A(直接孩子)
固定节点：先要求 Σ A(直接孩子) ≤ L(v)，再令 A(v) = L(v)
```

这保证 Key 总分配不超过个人、个人总分配不超过团队、团队总分配不超过组织；共享中间层不能隐藏固定后代。

金额额外考虑历史消费 U(v)。固定节点要求 `U(v) + Σ 下级未用保留 ≤ L(v)`，向父级保留 `max(0, L(v) - U(v))`；共享节点透传下级未用保留。删除或缩小分配只释放未用部分，不退回账单。金额为累计 USD，不自动按月重置。

## 请求、计数、供应商和账单

1. `internal/gateway/limits.go` 的 `enforceIdentityLimits` 检查身份、金额、团队/组织状态与模型权限。`IAM.QuotaPath` 校验金额链和兄弟保留，写入主体的 `BillingTeamID`、`BillingOrgID`。
2. `enforceRateLimits` 调用 `IAM.RatePlan`。`internal/iam/rates.go` 的 `buildRatePlan` 校验目标到根的祖先，拒绝缺上级、多团队、循环和 ID 错配；没有团队的个人是合法独立根。
3. 计划包含当前根下的兄弟及后代，保护固定保留，排除无关组织和独立根，减少 Redis 参数和计数读取。数据库 `quotaTree` 仍读取全树，尚未改成数据库按组织查询。
4. 有 Redis 时，`internal/live/rates.go` 的 `AdmitRatePlan` 用 Lua 原子检查并计数；无 Redis 时，在网关互斥锁内调用 `internal/live/rate_plan.go` 的 `CheckRatePlan` 并计数。两者按 UTC 自然分钟计数。
5. 放行后调用供应商；`internal/gateway/spend.go` 在响应后沿确定的计费归属记录费用，热消费及持久化账单用于后续金额检查。

金额耗尽返回 429 `budget_exceeded`，分钟超限返回 429 `rate_limit`。分钟计划或 Redis 故障返回 503 `rate_limit_unavailable`，停止调用，不降级放开限制。成功准入占用分钟计数，之后供应商失败不会自动退回这次计数。

## 固定保留与调用次序的保证

当前分钟剩余保留 R(v)：固定节点为 `max(0, L(v) - U(v))`，共享节点为直接孩子 R 的总和。RPM 请求增量 d=1，TPM 增量为非负入站估算。调用路径上的固定祖先检查：

```text
当前用量 + 不能由本次请求使用的下级保留 + d ≤ 本层上限
```

`credit` 是路径内最近固定节点的剩余保留，经过共享层继续传递。在祖先处从路径孩子的保留中扣除 credit，避免请求被自己的保留拦截；兄弟保留不能扣除，到达下一个固定祖先后更新 credit。

在配置/归属不变、初始用量为零且分配合法的前提下，任何只消耗自身可用容量的顺序都足够：固定池消费 d，使祖先用量增加 d、该池保留减少 d，二者之和不增加；共享池只占未保留余量。逐层维持上式，兄弟不能抢走固定池。归纳每次原子成功准入，所有固定祖先用量不超过上限。

匹配上界：组织 5，两个固定池各 2，共享余量 1，总容量恰为 5；各池第一个超配请求都必须拒绝，根不能接受第六个单位请求。`TestRatePlanAllOrders` 对 RPM/TPM 分别枚举 `5!/(2!×2!×1!)=30` 条完整顺序，并在每个前缀尝试三个下一请求，验证容量内充分放行和超配拒绝。小规模穷举是例证，一般保证来自不变量及原子提交。

## 测试分层与运行

| 层级 | 测试入口 | 核心保证 |
| --- | --- | --- |
| 输入单元 | internal/httpx/{budget,rates}_test.go | 缺省/null/零、负数、分数、溢出、非法类型 |
| IAM 单元及数据库 | internal/iam/{quota,rates}_test.go | 分配递归、真实并发分配回滚、唯一团队、退出释放、计划异常 |
| 纯准入单元 | internal/live/rate_plan_test.go | 两维度各 30 种顺序，容量内放行与匹配上界 |
| 计数及并发 | internal/live/rates_test.go、internal/gateway/rates_test.go | 真实临时 Redis、本地计数、拒绝不扣量、分钟恢复、故障、100 并发仅放行 7 |
| 后台 regression | cmd/regression/{quota_hierarchy,rate_hierarchy,budget_chain,teamless_keys}_test.go | 真实 HTTP/数据库/本地供应商，汇总、共享、权限、退出历史；混合字段失败金额/角色全回滚 |
| 验收脚本回归 | e2e/test_real_dataset.py、cmd/regression/real_dataset_limits_test.go | 实际执行 Python 限额验收；已消费且有固定分配的基线保持不变，五预算及八分钟限制拦截/恢复对账，正常与异常路径逆序清理 |
| 页面测试 | 配套 payload/schema、表单、QuotaGuide、HTTP 客户端测试 | 可空字段、三项限额、双语说明及错误、保存转换 |
| 浏览器 E2E | frontend/e2e/{rate-hierarchy,quota-hierarchy,personal-keys}.spec.ts | 页面操作→真实持久化→真实数据面放行/429；输入保留、修正、双语、唯一团队、退出、清空及删除 |

数据库测试使用隔离 schema，临时 Redis、本地供应商和数据库由测试清理。E2E 使用稳定角色、标签；自建对象在 finally 删除，脚本结束清理 schema，不覆盖用户现有数据。

仓库根目录运行：

```bash
go test ./internal/iam ./internal/httpx ./internal/live ./internal/authz ./internal/gateway/identity ./internal/gateway/keys -count=1 -json
go test -race ./internal/live ./internal/gateway -run 'TestRate|TestHierarchicalLocalRates' -count=1 -json
bash scripts/regression.sh --verbose 'TestRate|TestQuotaHierarchy|TestBudget|TestTeamless|TestPersonalKey'
bash scripts/e2e.sh e2e/rate-hierarchy.spec.ts e2e/quota-hierarchy.spec.ts e2e/personal-keys.spec.ts
```

前端用 `npm run test:unit`、`test:component`、`test:integration` 的指定文件参数。需要本地 PostgreSQL、Redis 测试环境与 Playwright；E2E 脚本包含生产构建和 TypeScript 校验。不能把 DB 不可达导致的 skip 当验证通过。

## 运行边界

- 金额调用前检查、响应后结算，没有预占最终费用；最终价格和并发在途请求可能超余额。分钟原子计数不等同于最终金额硬余额保证。
- TPM 是入站估算，包含文本与请求结构开销，不含最终输出；真实供应商分词、异步任务与账单需要外部验证。
- 多实例需共用 Redis；进程内计数只约束本进程，重启清零。已覆盖真实 Redis 原子与失败路径，未覆盖 Redis Cluster 和大规模负载。
- 配置/金额读取不是一次跨数据库与 Redis 的原子快照；并发归属/分配修改与准入之间仍有窗口。全树 DB 读取和 schema 级事务锁未做大规模性能验证。
- 当分钟调配不清用量，合法配置也可能暂时无法调用。历史多团队、超配数据由管理员修正，不猜归属或自动抬高上限。
