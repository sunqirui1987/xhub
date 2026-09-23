# 旧用量视图

- Route: `/old-usage`
- Nav: DEVELOPER TOOLS
- Status: `specified`

## 目的

旧仪表盘布局看 spend / activity。数据仍走全局 spend API。页顶 `DeprecationBanner`（The old Usage page）。新用量在 Observability `/usage`。只读。

## 布局

登录后控制台壳。`p-8` 全宽。Admin / Admin Viewer 比内部用户多三个顶层 Tab。

### 顶栏

无独立标题按钮。Tab 即导航。日期范围（AdvancedDatePicker）、Team/Tag 多选出现在各子视图卡片内。

### 筛选

All Up → Cost：日期、可选 key。Team Based Usage：团队。Tag Based Usage：tag 多选（`all-tags` 或具体 tag，选项 `GET` tag names）。Customer Usage：end-user。Activity：日期。筛选不写 URL。

### 表

All Up / Cost：月度 spend 卡、按 provider 饼图、Top Keys（`/global/spend/keys?limit=5`）、Top Models（`/global/spend/models?limit=5`）、日 spend 图。Activity：请求数/token 面积图、按模型活动。Team：团队柱。Customer：end-user 表。Tag：tag spend 表。数字用 `MoneyCell`。

### Tab

顶层默认 `all-up`：

| Tab | 谁可见 |
|---|---|
| All Up | 所有登录角色 |
| Team Based Usage | Admin / Admin Viewer |
| Customer Usage | 同上 |
| Tag Based Usage | 同上 |

All Up 内嵌：Cost / Activity。

### 抽屉

无。Top Key 行不打开控制台 Key 抽屉。

### 模态

无写模态。

### URL

`/old-usage`。Tab 不写入 URL。

## 交互

进入拉 `GET /global/spend/logs` 等。切 Tab 拉对应接口。改日期重拉 activity/provider。Admin 才能看跨团队/客户/tag。内部用户只见 All Up。数字必须来自 spend API，禁止 mock。窄屏双列图改单列。

## 操作 → 契约

| 用户动作 | Method Path | 成功后 UI |
|---|---|---|
| 全局 spend 日志 | `GET /global/spend/logs` | Cost 卡/图 |
| Top keys | `GET /global/spend/keys?limit=5` | Top Key 表 |
| Top models | `GET /global/spend/models?limit=5` | Top Model |
| Provider | `GET /global/spend/provider` | 饼图 |
| 活动 | `GET /global/activity` | Activity 图 |
| 按模型活动 | `GET /global/activity/model` | 分模型 |
| 团队 spend | `GET /global/spend/teams` | Team Tab |
| Tag spend | `GET /global/spend/tags` | Tag Tab |
| Tag 名 | tag names 列表接口 | Tag 下拉 |

## 字段

| 字段 | 含义 |
|---|---|
| `start_date` / `end_date` | 区间 |
| `spend` | USD |
| `api_requests` / `total_tokens` | 活动 |
| `api_key` | 哈希/别名，非明文 |
| `team_id` / tag / end_user | 维度 |

## 状态

loading：图卡骨架。empty：区间无数据空图。forbidden：非 Admin 不渲染跨租户 Tab（不是假装空表）。超时 Banner。窄屏图堆叠。

## 验收

All Up Cost/Activity 有真实 spend 数字；Admin 能切 Team/Customer/Tag；内部用户看不到后三个 Tab；桌面与窄屏；不得 mock 表格。
