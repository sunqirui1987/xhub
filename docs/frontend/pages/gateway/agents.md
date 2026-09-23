# Agents

- Route: `/agents`
- Nav: AI GATEWAY
- Status: `specified`

## 目的

创建 Agent、发现 Agent Card、绑定虚拟 Key。

## 布局

- **顶栏 CTA**：标题 Agents。说明文案 + 「Why do agents need keys?」提示。Admin 显示 `Add New Agent`。
- **筛选**：表工具栏搜索「Search agents by name, ID, or description...」；Health Check 开关（再拉列表带 health）。
- **表/卡片/Tab**：表列 Agent Name、Agent ID、Spend (USD)、Model、Created、Status（Active / Needs Setup）。Admin 行菜单 Delete。点 Agent ID 进详情。
  - 详情：Back to Agents。Tab Overview（Agent Card 字段、Skills、成本 `AgentCostView`、Virtual Keys 子表）、Settings（仅 admin：Edit Settings、Agent Card Discovery、Rate Limits、MCP Servers）。
- **抽屉/模态**：Add Agent 多步向导（Configure → Entitlements → Governance → Agent Management → Ready），含 Agent Card 发现与绑定 Key；创建成功可用 `CreatedKeyDisplay` 展示新 Key。Delete Agent 确认框。
- **URL query**：无。选中 agent 为组件内 state。

## 交互

- 点 Add New Agent → 向导；提交 `POST /v1/agents`（可顺带 `POST /key/generate` 绑定）→ 关闭并向表插入。点 Agent ID → Overview；admin Edit Settings → `PATCH /v1/agents/{agent_id}`。行 Delete → 确认后 `DELETE`，行消失。Health Check 开 → 列表带健康。Make Public 在表单 `litellm_params.make_public`（亦可 `POST /v1/agents/make_public`）。
- **view-only / 非 admin**：隐藏 Add New Agent、行 Delete、详情 Settings Tab。仍可读表与 Overview。
- **loading**：`Loading agents…`。
- **empty**：无数据 `No agents yet` / Add an agent…；搜索无匹配 `No matching agents`。
- **forbidden**：非 admin 无写 CTA；请求失败 toast。
- 校验失败可改后重试；超时锁定原 payload。

## 操作 → 契约

| 用户动作 | Method Path | 成功后 UI |
|---|---|---|
| 列出 | `GET /v1/agents` | 表 |
| 详情 | `GET /v1/agents/{agent_id}` | Overview |
| 创建 | `POST /v1/agents` | 新行 |
| 更新 | `PATCH /v1/agents/{agent_id}` | 保存 |
| 删除 | `DELETE /v1/agents/{agent_id}` | 移除 |
| 公开 | `POST /v1/agents/make_public` | is_public |

## 字段

| 字段 | 含义 |
|---|---|
| `agent_id` | id |
| `agent_name` | 名称 |
| `litellm_params` | 模型参数 |
| `is_public` / `make_public` | 是否公开 |

## 验收

桌面与窄屏走通上表每一行动作；不得 mock 表格。
