# Chat · 日志

- Route: `/chat/logs`
- Nav: CHAT
- Status: `specified`

## 目的

当前用户自己的请求日志。固定 `user_id=自己`，无跨用户/团队筛选，不是 Observability `/logs` 全量审计。

## 布局

Chat 壳主区 `py-8 px-8`。

### 顶栏

标题区 + 时间范围按钮组：24h / 7d / 30d（默认 24h）。

### 筛选

只有时间范围。无 request_id 搜索、无跨用户、无模型多选（管理 Logs 才有）。改范围重置到第 1 页。

### 表

列：Time（`MMM D, HH:mm:ss`）、Model、Status（Success 绿 / Failure 红）、Tokens、Duration、Cost。行可点。分页 50，底栏 `N requests · Page x of y`。空：`No logs for this period`。错：`Failed to load your logs` + Retry。Loading：8 行骨架。

### Tab

无。详情 Dialog 内 Request / Response 两块，不是抽屉 Tab。

### 抽屉

无。管理 Logs 用抽屉；本页用 Dialog。

### 模态

Request details：标题 + `request_id` mono；四格 Model/Cost/Tokens/Duration；Request JSON（`proxy_server_request` 或 `messages`）；Response JSON。加载骨架。无 Routing/Guardrails 管理 Tab。

### URL

`/chat/logs`。时间范围与页码不写入 URL。

## 交互

进入 `GET /spend/logs/ui?user_id=&sort_by=startTime&sort_order=desc`。点行 `GET /spend/logs/ui/{request_id}`。翻页 keepPreviousData。关 Dialog 清选中。数字来自 SpendLogs，禁止 mock。

## 操作 → 契约

| 用户动作 | Method Path | 成功后 UI |
|---|---|---|
| 列出 | `GET /spend/logs/ui` | 表 |
| 详情 | `GET /spend/logs/ui/{request_id}` | Dialog JSON |

## 字段

| 字段 | 含义 |
|---|---|
| `request_id` | 调用 id |
| `model` | 别名 |
| `status` | success / failure |
| `spend` | USD |
| `total_tokens` / `prompt_tokens` / `completion_tokens` | token |
| `startTime` / `endTime` / `request_duration_ms` | 时延 |
| `user_id` | 固定当前用户 |

## 状态

loading：骨架。empty：虚线空态。error：Retry。forbidden：未进 Chat 壳。详情缺报文：`Not available`。窄屏表横向滚。

## 验收

只出现当前用户行；无跨用户筛选；点行看到该 request 的 request/response；桌面与窄屏；不得 mock 表格。
