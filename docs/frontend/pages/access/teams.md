# 团队

- Route: `/teams`
- Nav: ACCESS CONTROL
- Status: `specified`

## 目的

团队、成员、模型白名单、预算。管理员与 org_admin 可建团；成员看自己的团。删团会删关联 Key 与为该团创建的模型。

## 布局

**顶栏** 无选中团队时：PageHeader 图标 Users、标题 Teams、副标题「Manage teams, members, and their access to models and budgets」。有建团权限时主按钮 Create Team。line Tab 嵌在 Header：Your Teams、Available Teams；proxy admin 另有 Default Team Settings。选中团队后顶栏换成「Back to Teams」+ `team_alias` + 可复制 `team_id`，不再显示 Create。

**筛选** Your Teams 表工具条：搜索（防抖，可按 team_id 前缀）+ Filter Drawer（Organization、Team alias、Team ID）+ Export CSV。改筛选/排序回第一页。Available Teams 与 Default Team Settings 不用这套抽屉。

**表** Your Teams 列：Team（alias 为主标题，有 alias 时 subtitle 为 team_id，点击进详情）、Organization（链到组织详情，无组织为 —）、Resources（members / models / keys 三枚计数徽章）、Spend / Budget（进度条）、Created。默认隐藏 Members、Models、Rate Limits（TPM/RPM 两行）、Updated。行末 ⋯：Edit team（仅 Admin）、Copy team ID、Delete team（仅 Admin，分隔线后危险项）。Available Teams 是可加入团队表，列与行菜单不同。

**Tab** 列表：Your Teams / Available Teams / Default Team Settings（admin）。详情（TeamInfoView）：Overview、My User、Virtual Keys；管理员或 team admin 另有 Members、Member Permissions、Settings。从行菜单 Edit 进详情时默认 Settings，否则 Overview。非管理员看不到 Members / Settings。

**抽屉** 无右侧 Sheet。详情是整页替换。Filter Drawer 是筛选项，不是团队详情。

**模态** Create Team Dialog：必填 team_alias；organization（org_admin 仅能选自己管理的组织，仅一个时预填；proxy admin 可建无组织团）；models（可空，表示模型来自访问组）；max_budget、budget_duration、tpm/rpm；可折叠 Additional / MCP / Agent / Search tool / Skills、model aliases、router settings、logging。Delete Team 确认框展示 Team ID / Name / Keys / Members；有 Key 时警告删团会删 Key 与模型；需输入 team_alias 才确认。Members Tab 另有 Edit Member / 移除成员确认。

**URL** `?team=<team_id>` 打开详情（push）；Back 清参数。列表 Tab、筛选、分页不进 URL。

## 交互

列表只读加行菜单。Create / Edit / Delete / 加成员是写。点 Team 名 → `?team=` 详情 Overview。点 ⋯ Edit team → 详情 Settings。点 Copy team ID → 剪贴板 toast，不跳页。点 Delete team → 确认框；成功后行消失、toast。点 Create Team → 模态；成功后关模态、Your Teams 出现新行。点 Organization 单元格 → 组织详情（离开本页）。Export CSV 按当前筛选下载，表不变。Available Teams 加入走 `/team/available` 口径，成功后该团出现在 Your Teams。

empty：Your Teams 无团「还没有团队」+ Create（有权限时）；有筛选「没有匹配的团队」。Available Teams 空「没有可加入的团队」，不是建团 CTA。forbidden：无建团权限藏 Create 与行内 Edit/Delete，Copy team ID 仍在。内部用户只列出自己所在团。越权 API 说明，不把别人的团画成空。

## 操作 → 契约

| 用户动作 | Method Path | 成功后 UI |
|---|---|---|
| 列出 | `GET /team/list` | Your Teams 表 |
| 可加入 | `GET /team/available` | Available Teams |
| 创建 | `POST /team/new` | 关模态，新行 |
| 加成员 | `POST /team/member_add` | Members 表更新 |
| 更新 | `POST /team/update` | Settings 保存 |
| 删除 | `POST /team/delete` | 行消失，关联 Key 删除 |

## 字段

| 字段 | 含义 |
|---|---|
| `team_alias` | 名称 |
| `team_id` | id |
| `organization_id` | 组织 |
| `members_with_roles` | 成员与角色（admin/user） |
| `models` | 模型白名单 |
| `max_budget` | 预算 |
| `spend` | 已花费 |
| `tpm_limit` / `rpm_limit` | 速率 |
| `blocked` | 是否停用 |

## 状态

loading：表骨架，Resources 徽章三枚 skeleton。empty / forbidden 见交互，禁止互扮。校验失败贴在 Create / Settings 字段旁（team_alias 必填、secret_manager_settings 须为合法 JSON）。删团确认加载中锁按钮。超时锁定原 payload。

## 验收

桌面与窄屏走通上表每一行动作；不得 mock 表格。删团必须展示 Key 数量警告并要求输入 alias。
