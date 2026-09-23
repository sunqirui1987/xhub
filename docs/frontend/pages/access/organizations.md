# 组织

- Route: `/organizations`
- Nav: ACCESS CONTROL
- Status: `specified`

## 目的

组织与成员，挂模型与预算对象。非 premium 部署整页说明并藏表。Admin 与 Org Admin 可创建。

## 布局

**顶栏** premium 且角色为 Admin 或 Org Admin 时左上「+ Create New Organization」。无 PageHeader 图标行。选中组织后顶栏由 OrganizationInfoView 的关闭/返回接管，创建按钮仍在页外层。非 premium：仅一段说明 + 试用链接，无按钮、无表。

**筛选** 未选组织时：Search by Organization Name（`org_alias`）+ Filters 按钮 + Reset。展开后增加 Search by Organization ID（`org_id`）。筛选作为 query 传 `GET /organization/list`。表上提示「Click on an organization ID to view its details.」

**表** 列：Organization ID（等宽，点击进详情）、Organization Name、Created、Spend (USD)、Budget (USD)（来自 `litellm_budget_table.max_budget`，空为 Unlimited）、Models、TPM / RPM Limits（两行，空为 Unlimited）、Members（`N Members`）。行末 ⋯ 仅 `userRole === "Admin"`：Edit、Delete。Org Admin 无行菜单，仍可点 ID 进详情。

**Tab** 详情：Overview、Members、Settings。列表无 Tab。Members 可加/改/删成员（`org_admin` 等）。Settings 编 alias、models、budget。从 ⋯ Edit 进入时 `editOrg=true`，直接可编。

**抽屉** 无 Sheet。详情整页替换表（仍留在 `/organizations?org=`）。

**模态** OrgCreateDialog。Delete Organization? 确认（Organization ID，不可撤销）。无密钥明文。Members 加成员可能另开表单/对话框。

**URL** `?org=<organization_id>` 详情（push）；关闭清参数。筛选不进 URL。

## 交互

点 Organization ID → 详情 Overview。点 ⋯ Edit → 详情并进入编辑。点 ⋯ Delete → 确认；成功 toast「Organization deleted successfully」并失效列表。点 Create → 创建对话框；成功后新行。点 Members 加成员 → `POST /organization/member_add`，计数更新。Reset Filters 清空 alias 与 id。

empty：无筛选「No organizations yet」——Create an organization to group teams, models, and budgets；有筛选「No matching organizations」——Try a different name or ID。非 premium 不是 empty，是功能说明。forbidden：非 Admin 藏行菜单 Delete/Edit；非 Admin/Org Admin 藏 Create。无 premium 不发 list 假装空表。越权 API 用说明。

## 操作 → 契约

| 用户动作 | Method Path | 成功后 UI |
|---|---|---|
| 列出 | `GET /organization/list` | 表 |
| 创建 | `POST /organization/new` | 新行 |
| 加成员 | `POST /organization/member_add` | Members 更新 |
| 更新 | `PATCH /organization/update` | Settings 保存 |
| 删除 | `DELETE /organization/delete` | 行消失 |

## 字段

| 字段 | 含义 |
|---|---|
| `organization_id` | id |
| `organization_alias` | 名称 |
| `budget_id` | 预算对象 |
| `max_budget` | 来自 budget table |
| `models` | 模型 |
| `tpm_limit` / `rpm_limit` | 速率 |
| `members` | 成员列表 |
| `spend` | 已花费 |

## 状态

loading：表骨架。empty / 非 premium / forbidden 三套文案不得互换。校验失败贴在创建/Settings 字段旁。删除 confirmLoading 锁按钮。超时锁定原 payload。

## 验收

桌面与窄屏走通上表每一行动作；不得 mock 表格。非 premium 必须说明而非空表。行菜单仅 Admin。
