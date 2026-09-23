# Chat · 密钥

- Route: `/chat/api-keys`
- Nav: CHAT
- Status: `specified`

## 目的

终端用户只看**自己的**虚拟 Key（`GET /key/list` 带当前 `user_id`）。无 Team/User 全局筛选，无 Create Key。Premium 可 Rotate，轮换成功后新 `sk-` **只展示一次**。

## 布局

Chat 壳（左栏 Chats/Integrations/… + 本页主区 `py-8 px-8`）。不是管理 `/api-keys`。

### 顶栏

`Your API Keys` + 说明 View your virtual keys and spend；premium 追加 grace period 说明。无 Create。

### 筛选

无。列表固定 `user_id=当前用户`，page_size=100。

### 表

列：Key（`key_name` 掩码 `sk-…` + 可选 `key_alias`）、Spend（`$x.xx`，有预算则 `/ $max`）、Expires（Never / UTC / Expired 红徽章）、Created（相对时间）。Premium 最右 Rotate。空态：`No keys found`。Loading：5 行骨架。

### Tab

无。左栏「API Keys」是路由高亮。

### 抽屉

无。不打开管理端 Key Info 抽屉。

### 模态

Rotate Key（仅 premium）：

1. 表单：Key Alias 只读；Max Budget、TPM、RPM；Expire Key（`30s|30m|24h|2d|1w|1mo`）；Grace Period。过期 Key 必须填 duration。
2. 成功：警告条 `Save this key now; you will not see it again` + 新 key 明文 + Copy。关闭后列表仍只显示掩码，不再回显明文。

### URL

`/chat/api-keys`。无 query。

## 交互

进入 `GET /key/list`（organization/team 空，`userID=自己`）。点 Rotate 开模态。校验 duration/grace 正则。提交 `POST /key/{token}/regenerate`，响应 `key` 填明文区，invalidate 列表。Copy 后按钮变 Copied。Close 丢弃明文。非 premium 无 Rotate 列。不提供删除/拉黑/跨用户筛选。

## 操作 → 契约

| 用户动作 | Method Path | 成功后 UI |
|---|---|---|
| 列出自己的 Key | `GET /key/list` | 表 |
| 轮换（premium） | `POST /key/{key}/regenerate` | 模态展示一次新明文 |

无 `POST /key/generate`。无 `GET /key/info` 抽屉。

## 字段

| 字段 | 含义 |
|---|---|
| `key_name` | 掩码展示 |
| `key_alias` | 显示名 |
| `spend` / `max_budget` | 已花费 / 上限 USD |
| `expires` / `duration` | 过期 |
| `tpm_limit` / `rpm_limit` | 速率 |
| `grace_period` | 旧 key 宽限期 |
| `token` | 轮换用 id，非明文 |

## 状态

loading：骨架表。empty：虚线空态。forbidden：未进 Chat 壳。校验失败贴在 duration/grace 下。轮换失败 toast，模态不关。密钥明文只展示一次。

## 验收

只出现当前用户 Key；无全局 User 筛选；premium 轮换后明文只出现在该模态；关闭后再打开没有明文；桌面与窄屏。
