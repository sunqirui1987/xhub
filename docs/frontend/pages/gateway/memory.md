# Memory

- Route: `/memory`
- Nav: AI GATEWAY
- Status: `specified`

## 目的

检索与删除 agent memory。

## 布局

- **顶栏 CTA**：标题 Memory。右侧 `New memory`。页顶 `DeprecationBanner`。
- **筛选**：工具栏「Search by key prefix or memory ID…」（防抖 `search` query）；刷新。服务端分页默认 50。
- **表**：列 ID（`memory_id`）、Name（`key`）、Preview（value）、User ID、Team ID、Created。行菜单 View / Edit / Delete。
- **抽屉/模态**：
  - `MemoryDetailDrawer` 只读展示 key/value/metadata。
  - `MemoryEditModal` 兼创建与编辑：key、value、metadata JSON。
  - Delete memory 确认，需输入 key 才可确认。
- **URL query**：无（分页/搜索在组件 state）。

## 交互

- 点 New memory → 创建模态；保存 `POST /v1/memory` → toast Created {key}，表刷新。点 ID 或 View → 抽屉。Edit → PUT 更新。Delete → 确认后 `DELETE /v1/memory/{key}`，行消失。
- **view-only**：无 `viewMemory` 时整页 `AdminOnlyNotice`，不渲染 New memory。有权限用户均可写（本页无 isViewOnly 分支）。
- **loading**：`Loading memories…`。
- **empty**：无数据 `No memories stored yet`；搜索无匹配 `No matching memories`。
- **forbidden**：`Memory is only available to admin users.`
- metadata 非合法 JSON 时 toast「Metadata must be valid JSON」可改后重试；超时锁定原 payload。

## 操作 → 契约

| 用户动作 | Method Path | 成功后 UI |
|---|---|---|
| 列出 | `GET /v1/memory` | 表 |
| 创建 | `POST /v1/memory` | 新行 |
| 更新 | `PUT /v1/memory/{key}` | 保存 |
| 删除 | `DELETE /v1/memory/{key}` | 行消失 |

## 字段

| 字段 | 含义 |
|---|---|
| `memory_id` | 记录 id |
| `key` | 记忆键 |
| `value` | 内容 |
| `agent_id` | 所属 agent（若有） |
| `user_id` | 所属用户 |
| `team_id` | 所属团队 |

## 验收

桌面与窄屏走通上表每一行动作；不得 mock 表格。
