# 访问组

- Route: `/access-groups`
- Nav: ACCESS CONTROL
- Status: `specified`

## 目的

把模型、MCP Server、Agent 打成一组，再挂到团队与虚拟 Key。不是「模型白名单 + budget 子资源」单列页。Admin Viewer 只读。

## 布局

**顶栏** 列表：PageHeader 图标 Boxes、标题 Access Groups、副标题「Manage resource permissions for your organization」。`canModify`（proxy admin）时 Create Access Group。详情：Back、标题 `access_group_name`、下一行可复制 ID；右 Edit Access Group（详情页按钮；Viewer 进详情后若无写权限应不可保存）。

**筛选** 列表搜索（最多约 400px）：name、id、description 客户端过滤。无 Organization Filter Drawer。详情无搜索。

**表** 列：ID（等宽，点击进详情）、Name、Resources（Models / MCP Servers / Agents 三枚计数徽章）、Created、Updated。`canModify` 时行末 ⋯ 仅「Delete access group」，无 Edit 项（编辑走详情顶栏）。分页 10/25/50。

**Tab** 列表无 Tab。详情：Group Details 卡（Description、Created/Updated by）；Attached Keys / Attached Teams 两卡（超过 5 条 Show Less / View All，徽章链到 Key / Team）；其下 Models / MCP Servers / Agents 资源 Tab，空时各有 emptyMessage。

**抽屉** 无。详情整页替换。选中态在组件 state，不是 nuqs。

**模态** AccessGroupCreateDialog：name、description、models / MCP / agents 多选（无 budget 字段）。AccessGroupEditModal：同上并可改 assigned keys/teams。Delete Access Group 确认：ID、Name、Description。无密钥明文。

**URL** 详情 id 不进 query（与 teams/projects/users 不同）。刷新从详情掉回列表。后续若补深链应对齐 `?group=`。

## 交互

点 ID → 详情。点 Create Access Group → 创建对话框；`POST /v1/access_group` 成功后新行。点 Edit Access Group → 编辑模态；`PUT /v1/access_group/{id}` 成功后详情徽章与资源 Tab 更新。点 ⋯ Delete → 确认；`DELETE /v1/access_group/{id}` 成功关框、行消失。点 Attached Key / Team 徽章 → 对应详情。Viewer 无 Create、无 ⋯、详情不可写。

empty：无搜索「No access groups yet」——Create an access group to manage resource permissions for your organization；有搜索「No matching access groups」——Try a different search term。详情 404：「Access group not found」+ Back。资源 Tab 空：「No keys attached」/「No teams attached」/ 对应 emptyMessage，不是整页 empty。forbidden：无写权限藏 Create 与删除菜单，列表仍可读。越权 GET 用说明，不把无权限画成「还没有访问组」。

## 操作 → 契约

| 用户动作 | Method Path | 成功后 UI |
|---|---|---|
| 列出 | `GET /v1/access_group` | 表 |
| 详情 | `GET /v1/access_group/{id}` | 详情卡与资源 Tab |
| 创建 | `POST /v1/access_group` | 新组 |
| 更新 | `PUT /v1/access_group/{id}` | 详情刷新 |
| 删除 | `DELETE /v1/access_group/{id}` | 行消失 |

## 字段

| 字段 | 含义 |
|---|---|
| `access_group_id` | id |
| `access_group_name` | 名称 |
| `description` | 说明 |
| `access_model_names` | 模型 |
| `access_mcp_server_ids` | MCP |
| `access_agent_ids` | Agent |
| `assigned_key_ids` | 已挂 Key |
| `assigned_team_ids` | 已挂团队 |
| `created_at` / `updated_at` | 时间 |

## 状态

loading：列表骨架；详情 spinner。empty / not found / forbidden 见交互。校验失败贴在 Create/Edit（name 必填）。删除 confirmLoading。超时锁定原 payload。本页不展示、不轮换虚拟 Key 明文。

## 验收

桌面与窄屏走通上表每一行动作；不得 mock 表格。Resources 列必须同时反映 models / MCP / agents 计数。Viewer 只读。
