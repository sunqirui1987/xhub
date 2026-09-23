# 成本跟踪

- Route: `/cost-tracking`
- Nav: SETTINGS
- Status: `specified`

## 目的

按供应商配置折扣、加价（百分比或固定额），以及拦截无定价模型的请求。定价计算器给所有角色估成本。CloudZero/Vantage 导出不在本页（在 `/logging-and-alerts` CloudZero Tab）。折扣/加价仅 proxy_admin 可改，自动保存。

## 布局

登录后控制台壳。`p-8`：标题 `Cost Tracking Settings` + DocsMenu（custom pricing / spend tracking 外链）。其下若干折叠卡。

### 顶栏

标题 + 说明「Configure cost discounts and margins… Changes are saved automatically。」无全局 Save。

### 筛选

无。Add 模态内供应商下拉。计算器内模型 SearchSelect。

### 表

- Provider Discounts：供应商、折扣百分比、行内改值、Remove。
- Fee/Price Margin：供应商、percentage 和/或 fixed_amount、Remove。
- Pricing Calculator：多行模型 + input/output tokens + 请求量，结果区成本。

### Tab

Discounts 折叠内：Discounts / Test It（How it works 说明）。页级无顶 Tab。其余折叠：Fee/Price Margin、Block Unpriced Models、Pricing Calculator（默认展开）。

### 抽屉

无。

### 模态

- Add Provider Discount：供应商 + 折扣百分比。
- Add Provider Margin：供应商 + marginType percentage/fixed + 对应数值。
- 移除确认 AlertDialog（discount 或 margin）。

非 admin 不渲染折扣/加价/拦截折叠，只留计算器。

### URL

`/cost-tracking`。折叠与计算器行不写入 URL。

## 交互

进入并行 `GET /config/cost_discount_config`、`GET /config/cost_margin_config`、`GET /config/block_requests_for_models_without_pricing`，以及 `GET /model_group/info` 供计算器。改折扣单元格立即 PATCH 折扣配置。Add 成功关模态。Remove 确认后从 map 删键并 PATCH。Block Unpriced Switch 立即 PATCH，开启后无定价模型请求 403。计算器改模型/token 防抖估成本。非 admin 看不到写折叠。

## 操作 → 契约

| 用户动作 | Method Path | 成功后 UI |
|---|---|---|
| 读折扣 | `GET /config/cost_discount_config` | 折扣表 |
| 保存折扣 | `PATCH /config/cost_discount_config` | 表更新 |
| 读加价 | `GET /config/cost_margin_config` | 加价表 |
| 保存加价 | `PATCH /config/cost_margin_config` | 表更新 |
| 读拦截开关 | `GET /config/block_requests_for_models_without_pricing` | Switch |
| 写拦截开关 | 同路径 PATCH/PUT | Switch 跟上 |
| 模型列表 | `GET /model_group/info` | 计算器下拉 |
| 估成本 | 成本估算接口（按模型 token） | 结果区 |

## 字段

| 字段 | 含义 |
|---|---|
| 折扣 map `[provider]=ratio` | 0–1 或百分比展示 |
| `percentage` / `fixed_amount` | 加价 |
| `blockUnpriced` / `enabled` | 无定价则 403 |
| `model` | 计算器模型组 |
| `input_tokens` / `output_tokens` | 单次 token |
| `num_requests_per_day` / `per_month` | 量 |

## 状态

loading：`Loading configuration...`。empty：`No provider discounts configured` + 主按钮。forbidden：非 admin 无写区。移除中按钮锁定。计算器无模型时空下拉。窄屏折叠全宽。

## 验收

Admin 能加/改/删折扣与加价且自动保存；拦截开关立刻打对应配置；非 admin 只能用计算器；桌面与窄屏；不得 mock 表格。
