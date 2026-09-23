# 项目

- Route: `/projects`
- Nav: ACCESS CONTROL
- Status: `specified`

## 目的

团队下项目，用于把 Key 按项目归组。`enable_projects_ui` 关闭则侧栏隐藏本页。企业能力独立实现，不复制企业源码。

## 布局

**顶栏** 列表：PageHeader 图标 Folder、标题 Projects、副标题「Manage projects within your teams」、主按钮 Create Project。详情：左 Back 图标、标题 `project_alias`（否则 `project_id`）+ Active/Blocked 徽章、下一行 `ID:` 可复制；右 Edit Project。无列表级 Export。

**筛选** 列表仅搜索框（最多约 400px）：匹配 project_alias、project_id、description、团队 alias。客户端过滤，不打服务端 search。无 Team/Status Filter Drawer。详情无筛选。

**表** 列：ID（等宽，点击进详情）、Name（`project_alias`，空为 —）、Team（alias，加载中 skeleton，未知则等宽 team_id）、Models（outline 徽章显示数量，tooltip 列出或「No models」）、Status（Blocked 红 / Active 绿）、Created、Updated。无行末 ⋯；编辑只在详情。分页客户端，page/page_size 进 URL，默认 10，可选 25/50。

**Tab** 列表无 Tab。详情无页级 Tab：Details 卡（Description、Created/Updated by）、Budget 卡（spend / max_budget 进度）、成员/团队卡、按模型 spend 柱、其下 Project Keys 子表。Keys 列：Key Name（`key_alias`，链到 Key 详情）、Owner、Created、Last Active（空为 Never）。

**抽屉** 无。详情整页替换列表。Filter 只有搜索。

**模态** Create Project Modal（team 必选、alias、models、预算等）。Edit Project Modal（详情 Edit）。无列表行删除菜单；删除若存在走编辑流而非行 ⋯。创建成功关模态、列表出现新行。本页不弹出密钥明文。

**URL** `?project=<project_id>` 详情（push）；Back replace 清除。列表 `page`、`page_size` 同步 query。搜索词不进 URL。

## 交互

列表只读。Create / Edit 是写。点 ID → `?project=` 详情。点 Create Project → 创建模态；成功后刷新 `GET /project/list`。点 Edit Project → 编辑模态；保存后详情字段与 Status 徽章更新。点 Key Name → 虚拟密钥详情（可带该项目过滤），不在本页展示 sk-。点搜索清空 × → 恢复全表。

empty：无搜索「No projects yet」——Create a project to organize keys within your teams；有搜索「No matching projects」——Try a different search term。详情找不到：「Project not found」+ Back，不是空 Keys 表。forbidden：`enable_projects_ui` 关闭则无导航入口。无读写角色 `GET /project/list` 不发；越权用说明，不把空表当成「还没有项目」。内部用户可见性见角色表，Projects 对非 admin 默认可读范围以 API 为准。

## 操作 → 契约

| 用户动作 | Method Path | 成功后 UI |
|---|---|---|
| 列出 | `GET /project/list` | 表 |
| 详情 | `GET /project/info` | 详情卡与 Keys 子表 |
| 创建 | `POST /project/new` | 关模态，新行 |
| 更新 | `POST /project/update` | 详情刷新 |
| 删除 | `POST /project/delete` | 回列表，行消失 |

## 字段

| 字段 | 含义 |
|---|---|
| `project_id` | id |
| `project_alias` | 名称 |
| `team_id` | 所属团队 |
| `models` | 模型集合 |
| `max_budget` | 预算（`litellm_budget_table`） |
| `spend` | 已花费 |
| `blocked` | Active / Blocked |
| `created_at` / `updated_at` | 时间 |

## 状态

loading：列表骨架；详情居中 spinner。empty / not found / forbidden 见交互，三者文案不得互换。校验失败贴在 Create/Edit 字段（team_id 必填）。超时锁定原 payload。窄屏表横向滚，Create 保留。

## 验收

桌面与窄屏走通上表每一行动作；不得 mock 表格。`enable_projects_ui=false` 时侧栏无 Projects。Keys 子表不得出现明文 token。
