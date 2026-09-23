# Policies

- Route: `/policies`
- Nav: AI GATEWAY
- Status: `specified`

## 目的

版本化策略与绑定。

## 布局

- **顶栏 CTA**：无页级按钮。Policies Tab 内 `+ Add New Policy`；Attachments Tab 内 `+ Add New Attachment`（无策略时 disabled）。
- **筛选**：无列表搜索。策略表按 Name 客户端排序。
- **表/卡片/Tab**：
  - Tab：Templates、Policies、Attachments、Policy Simulator。
  - Policies 表列 Name（version 徽章 / Config 徽章）、Description、Inherits From、Guardrails (Add/Remove)、Model Condition、Created At；admin 行菜单 Edit / Delete。点 Name 进详情。
  - 详情 / Flow Builder：Versions、pipeline 编排、status。Attachments 表绑定 scope（global / teams / keys / models / tags）。Simulator 为 Test pipeline。
- **抽屉/模态**：AddPolicyForm；GuardrailSelectionModal；TemplateParameterModal；AiSuggestionModal；Delete Policy / Delete Attachment。
- **URL query**：无。`showFlowBuilder` 为全页替换。

## 交互

- Templates Use → 可创建 guardrail 再组 policy。Add New Policy → `POST /policies` 或打开 Flow Builder。点 Name → 详情；Edit → Flow Builder `PUT /policies/{id}`。改版本状态 `PUT /policies/{id}/status`。Add Attachment → `POST /policies/attachments`。Simulator 跑 pipeline。Delete 行消失。
- **view-only / 非 admin**：表无 Edit/Delete 行菜单；仍可读 Templates/Policies/Attachments/Simulator。
- **loading**：策略表 loading；Attachments 独立 loading。
- **empty**：`No policies found` / Create a policy to bundle guardrails…
- **forbidden**：无 token 时 Add 按钮 disabled。
- 校验失败可改后重试；超时锁定原 payload。

## 操作 → 契约

| 用户动作 | Method Path | 成功后 UI |
|---|---|---|
| 列出 | `GET /policies/list` | 表 |
| 创建 | `POST /policies` | 新策略 |
| 改状态 | `PUT /policies/{policy_id}/status` | active/disabled |
| 绑定 | `POST /policies/attachments` | 附件表更新 |

## 字段

| 字段 | 含义 |
|---|---|
| `policy_name` | 名称 |
| `version` | 版本 |
| `status` | 状态 |
| `attachment_id` | 绑定 id |

## 验收

桌面与窄屏走通上表每一行动作；不得 mock 表格。
