# 日志与告警

- Route: `/logging-and-alerts`
- Nav: SETTINGS
- Status: `specified`

## 目的

配置 success/failure logging callback（Langfuse 等）、Slack 兼容 webhook 告警、邮件/MS Teams、以及 CloudZero 成本导出。仅 admin。折扣/加价不在本页（见 `/cost-tracking`）。

## 布局

登录后控制台壳。`mx-4` + `p-8` 网格。线型 Tab。

### 顶栏

无独立标题。Logging Callbacks Tab 表上方有 Add 入口（表格工具栏）。

### 筛选

无。Callback 选择器在 Add 模态内 combobox 搜索。

### 表

Logging Callbacks：已启用 callback 名、logo、enabled、行 Edit / Delete / Test（health check）。Alerting Types：固定行（LLM Exceptions、Too Slow、Hanging、Budget、User Spend Thresholds/Anomalies、DB Exceptions、Spend Reports、Outage、Region Outage、Model Deprecation），列开关、显示名、Webhook URL。

### Tab

默认 `logging-callbacks`：

| Tab | 内容 |
|---|---|
| Logging Callbacks | 已配置 callback 表 |
| CloudZero Cost Tracking | CloudZero 集成设置 / 空态 + Create |
| Alerting Types | 上表 + 总 webhook |
| Alerting Settings | 阈值等 `AlertingSettings` |
| Email Alerts | 邮件事件 |
| MS Teams Alerts | Teams webhook |

### 抽屉

无。Edit callback 用 Dialog。

### 模态

- Add Callback：combobox 选 callback（logo+displayName），动态 `dynamic_params`（text/password/number/select，required 标红）。提交 `environment_variables` + `litellm_settings.success_callback`。
- Edit Callback：同字段，预填已有 variables。
- Delete 确认 `DeleteResourceModal`。
- CloudZero Create / Update / Export / dry-run（`/cloudzero/settings`、`/cloudzero/init`、`/cloudzero/export`、`/cloudzero/dry-run`）。

### URL

`/logging-and-alerts`。Tab 不写入 URL。

## 交互

进入拉 `GET /get/config/callbacks`（callbacks、available_callbacks、alerts）与 callback configs。Add/Edit 成功 `POST /config/update` 后重拉列表。Delete `deleteCallback` 后刷新。Test `serviceHealthCheck` toast。Alerting Types 开关只改本地 `activeAlerts`，需在 Alerting Settings 保存才落库（Slack 兼容 webhook，含 catch-all `SLACK_WEBHOOK_URL` 与 `alerts_to_webhook`）。CloudZero 空态点创建；已配置可 export / dry-run。密码型 param 用 password input。

## 操作 → 契约

| 用户动作 | Method Path | 成功后 UI |
|---|---|---|
| 读 callbacks / alerts | `GET /get/config/callbacks` | 表与开关 |
| callback 字段元数据 | callback configs 读接口 | Add 动态表单 |
| 保存 callback | `POST /config/update` | 关模态、表刷新 |
| 删除 callback | delete callback 接口 | 行消失 |
| 测 callback | health check | toast |
| CloudZero 设置 | `GET /cloudzero/settings` | 集成卡或空态 |
| CloudZero 导出 | `POST /cloudzero/export` | 任务状态 |
| CloudZero dry-run | `POST /cloudzero/dry-run` | 预览 |

Vantage 同类接口存在于后端，本页 UI 挂的是 CloudZero Tab。

## 字段

| 字段 | 含义 |
|---|---|
| `callback` / `litellm_callback_name` | langfuse 等 id |
| `ui_callback_name` | 显示名 |
| `dynamic_params` | 按 callback 的表单字段 |
| `environment_variables` | 提交的配置（密码不回明文） |
| `success_callback` | 启用列表 |
| `SLACK_WEBHOOK_URL` | 总 webhook |
| `alerts_to_webhook` | 按 alert 覆盖 URL |
| `active_alerts` | 打开的告警类型 |
| `llm_exceptions` / `budget_alerts` / `outage_alerts` 等 | Alerting Types 行 |

## 状态

loading：callback 表 skeleton。empty：无 callback，点 Add。forbidden：非 admin。Add 未选 callback 或缺 required param：字段红字。删除中锁定。CloudZero 加载失败展示 error.message。窄屏 Tab 横向滚。

## 验收

能增删改测 logging callback；Alerting Types 行与 webhook 可见；CloudZero 空态与导出走真实接口；桌面与窄屏；不得 mock 表格。
