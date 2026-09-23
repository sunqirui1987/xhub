# 预算

- Route: `/budgets`
- Nav: ACCESS CONTROL
- Status: `specified`

## 目的

可复用预算对象（spend / TPM / RPM），挂到 customer（也可被 org/team/key 引用）。Admin Viewer 只读；proxy admin 可写。

## 布局

**顶栏** PageHeader 图标 Wallet、标题 Budgets、副标题「Spend, TPM and RPM limits you can assign to customers.」。`canModify`（proxy admin）时主按钮 Create Budget。line Tab：Budgets、Examples。

**筛选** Budgets Tab 表 Filter Drawer（服务端过滤，filterFn 只为挡住 TanStack 误删）：Reset（budget_duration 多选，含 Not set，与具体 7d/30d 互斥）、Max Budget（min/max 或 Unlimited only）、Created（from/to）。工具条搜索。列菜单默认隐藏 Reset、Created（`BUDGET_TABLE_HIDDEN_COLUMNS`），打开后才显示。

**表** 列：Budget ID（可复制、不截断）、Max Budget（空为 Unlimited，0 也显示）、TPM、RPM（空为 n/a）、Reset（duration 文案，未设为 Not set）、Created。`canModify` 时行末 ⋯：Edit budget、分隔线、Delete budget。无行点击进详情页。分页默认 50。

**Tab** Budgets（表 + 创建/编辑模态挂载点）、Examples。Examples 只读代码：Assign Budget to Customer、Test it (Curl)、Test it (OpenAI SDK)。Examples 不打预算 API。

**抽屉** 无详情抽屉。Filter Drawer 只筛选。

**模态** Create（BudgetModal）：max_budget、soft_budget、tpm、rpm、budget_duration 等。EditBudgetModal 预填。Delete Budget? 确认展示 Budget ID、Max Budget、TPM、RPM。无密钥明文。

**URL** Tab、筛选、分页不进 query。无 `budget=` 详情深链。

## 交互

点 Create Budget → 创建模态；成功新行 + toast。点 ⋯ Edit budget → 编辑模态；保存后该行数字更新。点 ⋯ Delete budget → 确认；成功 toast「Budget deleted.」行消失。点 Budget ID 复制，不打开抽屉。Admin Viewer 无 Create、无 ⋯，仍可筛选与看 Examples。改 Reset/Max Budget/Created 后服务端重拉，空草稿不作为有效筛选。

empty：无查询「No budgets yet」——Create a budget to set spend, TPM and RPM limits for customers；有查询「No matching budgets」——No budget matches your search or filters。forbidden：403「You do not have access to budgets」——Ask a proxy admin to grant you the admin viewer role；其它错误「Could not load budgets」+ `error.message`。禁止把 403 画成「还没有预算」。

## 操作 → 契约

| 用户动作 | Method Path | 成功后 UI |
|---|---|---|
| 列出 | `GET /management/v1/budgets`（兼容 `GET /budget/list`） | 表 |
| 创建 | `POST /budget/new` | 新行 |
| 更新 | `POST /budget/update` | 行内数字更新 |
| 删除 | `POST /budget/delete` | 行消失 |

## 字段

| 字段 | 含义 |
|---|---|
| `budget_id` | id |
| `max_budget` | USD 上限 |
| `soft_budget` | 告警阈值（创建表单，默认列不展示） |
| `tpm_limit` | TPM |
| `rpm_limit` | RPM |
| `budget_duration` | 周期如 7d / 30d，表头 Reset |
| `created_at` | 创建时间 |

## 状态

loading：表骨架。empty / 403 / 其它错误见交互。校验失败贴在 Create/Edit 字段旁。删除 confirmLoading。超时锁定原 payload。列隐藏状态属于表偏好，刷新可回到默认四列（ID / Max / TPM / RPM）。

## 验收

桌面与窄屏走通上表每一行动作；不得 mock 表格。Admin Viewer 必须看得见表、看不见写入口。403 文案不得与 empty 相同。
