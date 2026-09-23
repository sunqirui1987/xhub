# 工作流运行

- Route: `/workflows`
- Nav: AI GATEWAY
- Status: `specified`

## 目的

查看 durable workflow 运行。

## 布局

- **顶栏 CTA**：标题 Workflow Runs，副标题 Durable state tracking…。无创建按钮（只读）。页顶 `DeprecationBanner`。
- **筛选**：工具栏「Search runs…」；刷新；Filters 抽屉：Status（pending / running / paused / completed / failed）、Type。客户端分页 50/100。
- **表/卡片/Tab**：列 Run（状态点 + title + 短 `run_id`）、Type、Status、Created。点行打开右侧 Sheet。
- **抽屉/模态**：详情 Sheet：Metadata 卡（status、created、pr_url、worktree、session…）；可折叠 Events 时间线 / Gantt；Messages 对话。无写模态。
- **URL query**：无。

## 交互

- 进页拉 `GET /v1/workflows/runs?limit=100`。点行 → Sheet 并行拉 events 与 messages。刷新重拉列表。筛选/搜索只过滤当前页数据。
- **view-only**：本页对有 `viewWorkflowRuns` 的角色只读；无 mutating CTA。
- **loading**：列表 `Loading workflow runs…`；详情 Sheet 中央 spinner。
- **empty**：表 `No workflow runs yet`；时间线 `No events recorded`。
- **forbidden**：无 `viewWorkflowRuns` → `AdminOnlyNotice`：「Workflow Runs is only available to admin users.」且不发 `/v1/workflows` 请求。
- 校验失败可改后重试；超时锁定原 payload。

## 操作 → 契约

| 用户动作 | Method Path | 成功后 UI |
|---|---|---|
| 列出 | `GET /v1/workflows/runs` | 表 |
| 事件 | `GET /v1/workflows/runs/{run_id}/events` | 时间线 |
| 消息 | `GET /v1/workflows/runs/{run_id}/messages` | 对话 |
| 更新状态 | `PATCH /v1/workflows/runs/{run_id}` | status 变化 |

## 字段

| 字段 | 含义 |
|---|---|
| `run_id` | 运行 id |
| `status` | pending/running/paused/completed/failed |
| `workflow_type` | 类型 |
| `started_at` / `created_at` | 开始时间 |

## 验收

桌面与窄屏走通上表每一行动作；不得 mock 表格。
