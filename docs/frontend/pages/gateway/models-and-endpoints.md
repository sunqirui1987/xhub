# 模型与端点

- Route: `/models-and-endpoints`
- Nav: AI GATEWAY
- Status: `specified`

## 目的

登记 deployment、凭证、健康与 fallback。

## 布局

- **顶栏 CTA**：标题 Model Management。右侧刷新图标 + Last Refreshed 时间。无独立「Create」按钮——创建在 Add Model Tab。
- **筛选**（All Models）：搜索 Public Model Name；Team 下拉（Personal / 具体 team）；View mode Current Team Models / All Available Models；Filters 抽屉：Public Model Name、Model Access Group（含 wildcard）；Model Settings 按钮打开设置模态。
- **表/卡片/Tab**：
  - 默认 All Models / Your Models（非 admin）。列 Model ID、Model Information、Credentials、Created By、Updated At、Costs、Team ID、Model Access Group、Source、Actions（Pause/Delete，view-only 隐藏）。
  - 权限可见 Tab：Add Model（可创建时）、Auto-Routers（Beta）、LLM Credentials、Pass-Through Endpoints、Health Status、Model Retry Settings、Model Group Alias、Model Access Group Budgets（Beta）、Price Data Reload。view-only admin 只保留读侧（All Models + Health Status）。
  - Add Model 向导：provider → `litellm_params.model` / `api_base` / `api_key`（或凭证引用）→ rpm/tpm/timeout → 测试连接。
- **抽屉/模态**：删除确认；Model Settings；点行打开 `ModelInfoView`；`?team=` 打开 Team 详情。
- **URL query**：`?model=<id>` 模型详情；`?team=<id>` 团队详情；`?model_group=` 过滤公共模型名。

## 交互

- 点刷新 → invalidate `models/list`。点行 Model ID → `?model=` 详情。Add Model 提交 `POST /model/new` → 出现在表中且 Chat 可用。行 Pause / Delete → 更新或 `POST /model/delete`，行状态变或消失。Health Tab 测连接 → 健康徽章。
- **view-only**：隐藏 Add Model、LLM Credentials、Pass-Through、Retry、Alias、Budgets、Price Data；表 Actions 无 Pause/Delete；Team 详情 `editTeam=false`。
- **loading**：模型表骨架 / Health 加载。
- **empty**：`No models found` — No models match your search or filters. Try resetting them.
- **forbidden**：无创建权时不出现 Add Model Tab（内部用户可被 UI setting `disable_model_add_for_internal_users` 关掉）。
- 校验失败可改后重试；超时锁定原 payload。

## 操作 → 契约

| 用户动作 | Method Path | 成功后 UI |
|---|---|---|
| 列出 | `GET /v2/model/info` | 表 |
| 新建 | `POST /model/new` | 出现在表中且 Chat 可用 |
| 更新 | `POST /model/update` | 刷新 |
| 删除 | `POST /model/delete` | 行消失 |
| 测连接 | `POST /health/test_connection` | 健康徽章 |
| 凭证 | `GET /credentials` | 凭证抽屉 |

## 字段

| 字段 | 含义 |
|---|---|
| `model_name` | 客户端别名 |
| `litellm_params.model` | 上游模型 |
| `api_base` | 上游地址 |
| `api_key` | 环境变量或凭证引用 |
| `rpm` | 每分钟请求 |
| `tpm` | 每分钟 token |
| `timeout` | 秒 |

## 验收

桌面与窄屏走通上表每一行动作；不得 mock 表格。
