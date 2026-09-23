# Chat · 用量

- Route: `/chat/usage`
- Nav: CHAT
- Status: `specified`

## 目的

当前用户日聚合 spend / 请求 / token。不是 Observability `/usage` 全局仪表盘，无团队/模型下钻。

## 布局

Chat 壳主区 `py-8 px-8`。

### 顶栏

`Your Usage` + `Spend and request activity`。右：7d / 30d / 90d（默认 30d）。

### 筛选

仅时间范围按钮。无 Team/模型筛选。

### 表

无明细表。四张卡：Total Spend、API Requests、Tokens Used（含 in/out）、Success Rate（失败数红字）。多日时两根 sparkline：Daily Spend、Daily Requests。

### Tab

无。

### 抽屉

无。

### 模态

无。

### URL

`/chat/usage`。时间范围不写入 URL。

## 交互

进入 `GET /user/daily/activity/aggregated?start_date=&end_date=`（带当前 user）。切 7d/30d/90d 重查。`total_api_requests===0` 空态 `No usage data for this period`。数字与 SpendLogs 一致。

## 操作 → 契约

| 用户动作 | Method Path | 成功后 UI |
|---|---|---|
| 日聚合 | `GET /user/daily/activity/aggregated` | 卡 + sparkline |

## 字段

| 字段 | 含义 |
|---|---|
| `date` | 日 |
| `metrics.spend` | USD |
| `metrics.prompt_tokens` / `completion_tokens` / `total_tokens` | token |
| `metrics.api_requests` / `successful_requests` / `failed_requests` | 次数 |
| `metadata.total_*` | 区间合计 |

## 状态

loading：四卡骨架。empty：虚线空态。forbidden：未进 Chat 壳。窄屏两列卡可堆叠为更窄栅格。

## 验收

切时间范围数字变化；无跨用户数据；空区间空态不是假 0 图冒充有流量；桌面与窄屏。
