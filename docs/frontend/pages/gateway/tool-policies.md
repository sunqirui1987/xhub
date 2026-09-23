# Tool Policies

- Route: `/tool-policies`
- Nav: AI GATEWAY
- Status: `specified`

## 目的

哪些主体能调哪些工具。

## 布局

- **顶栏 CTA**：标题 Tool Policies。无创建按钮（工具靠调用自动发现）。
- **筛选**：表搜索 + 列筛选（tool name、team、key…）。
- **表/卡片/Tab**：概览四张 MetricCard：New Today、Total Tools Discovered、Blocked Tools、Active Teams。表列 Discovered、Tool Name、Input Policy、Output Policy、# Calls、Team Name、Key Hash、Key Name、User Agent。点 Tool Name 进 `ToolDetail`（allow/block 与主体覆盖、恢复默认）。
- **抽屉/模态**：无独立创建模态。详情内改 policy / 清 override。
- **URL query**：无。详情为组件内 view state。

## 交互

- 进页 `GET /v1/tool/list` 填表；options `GET /v1/tool/policy/options`。行内改 Input/Output Policy → `POST /v1/tool/policy`，行即时更新。点名进详情；清覆盖 `DELETE /v1/tool/{tool_name}/overrides` 恢复默认。
- **view-only**：无 `viewToolPolicies` 时整页文案「Tool Policies is only available to admin users.」不拉列表。有权限才渲染表与写 policy。
- **loading**：表 loading。
- **empty**：`No tools discovered` / Make a chat completion that returns tool_calls to start auto-discovery. 筛选无匹配 `No matching tools`。
- **forbidden**：同上 admin-only 文案。
- 写失败 toast Failed to update input/output policy，可改后重试；超时锁定原 payload。

## 操作 → 契约

| 用户动作 | Method Path | 成功后 UI |
|---|---|---|
| 列出工具 | `GET /v1/tool/list` | 表 |
| 策略选项 | `GET /v1/tool/policy/options` | 下拉 |
| 覆盖 | `DELETE /v1/tool/{tool_name}/overrides` | 恢复默认 |

## 字段

| 字段 | 含义 |
|---|---|
| `tool_name` | 工具名 |
| `allowed` | 是否允许 |
| `blocked_tools` | 黑名单 |
| `input_policy` | 入站策略 |
| `output_policy` | 出站策略 |

## 验收

桌面与窄屏走通上表每一行动作；不得 mock 表格。
