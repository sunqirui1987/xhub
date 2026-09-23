# 路由设置

- Route: `/router-settings`
- Nav: SETTINGS
- Status: `specified`

## 目的

配置网关如何把同一 `model_group` 打到多个 deployment：routing strategy、重试、tag filtering、fallback 链、routing groups、Anthropic prompt cache。仅 admin。

## 布局

登录后控制台壳。全宽，线型 Tab `mx-8 mt-4`，内容 `px-8 py-6`。

### 顶栏

无独立标题。Tab 即页头。Loadbalancing Tab 内分组标题 Routing Settings / Reliability。

### 筛选

无资源筛选。Strategy 下拉来自 `GET /router/settings` 的 `fields[].options`。

### 表

- Loadbalancing：表单，不是资源表。
- Routing Groups：组列表。
- Fallbacks：主模型 → 备用模型链（provider logo + 名），行 Edit / Test / Delete。
- Prompt Caching：开关 + TTL 下拉。
- General：动态设置表，列 Setting（名+描述）、Value（按 field_type 控件）、Status（In DB / In Config / Not Set）、Action（Update / 垃圾桶 reset）。

### Tab

默认 `loadbalancing`：

| Tab | 内容 |
|---|---|
| Loadbalancing | `routing_strategy` 下拉（选项来自后端，含 simple-shuffle / least-busy / lowest-latency / latency-based-routing / lowest-cost 等）、`enable_tag_filtering`、latency-based 时 TTL/buffer、ReliabilityRetries（num_retries、timeout、allowed_fails 等） |
| Routing Groups | 组 CRUD |
| Fallbacks | fallback 图编辑 |
| Prompt Caching | `enable_anthropic_prompt_caching` 立即保存；TTL 仅在开启后可改，空 = 5m default |
| General | `GET /config/list?config_type=general_settings` 动态行，排除 TypedDictionary 与 prompt_caching 字段 |

Tab `keepMounted`。

### 抽屉

无。

### 模态

Fallbacks：Add / Edit 模型链；Delete 确认；Test 用 openai 客户端打一枪。General 无模态，行内 Update。

### URL

`/router-settings`。Tab 不写入 URL。

## 交互

进入 Loadbalancing：`GET /get/config/callbacks` 取 `router_settings`（去掉 `model_group_retry_policy`），`GET /router/settings` 取字段元数据与 strategy 说明。改 strategy/tag/retries 后 Save 走 `POST /config/update`（callbacks payload 含 router_settings）。Prompt Caching 开关立即 `POST /config/field/update` 或 `POST /config/field/delete`（清空 TTL）。General 每行 Update 写该 `field_name`；垃圾桶 reset 回 `field_default_value`，状态 Not Set。Fallbacks 保存同样走 config update。未登录 `accessToken` 空则整页 `null`。

## 操作 → 契约

| 用户动作 | Method Path | 成功后 UI |
|---|---|---|
| 读 router 当前值 | `GET /get/config/callbacks` | 填 Loadbalancing |
| 字段元数据 | `GET /router/settings` | 动态 label/options |
| 保存 router / fallback | `POST /config/update` | toast，表单保持 |
| 读 general | `GET /config/list?config_type=general_settings` | General 表 |
| 更新单字段 | `POST /config/field/update` | Status → In DB |
| 重置字段 | `POST /config/field/delete` | Status → Not Set |

## 字段

| 字段 | 含义 |
|---|---|
| `routing_strategy` | 策略名 |
| `routing_strategy_args.ttl` / `lowest_latency_buffer` | latency-based 参数 |
| `enable_tag_filtering` | 按请求 tag 过滤 deployment |
| `num_retries` | 重试次数 |
| `timeout` | 秒 |
| `allowed_fails` | 冷却阈值 |
| `enable_anthropic_prompt_caching` | 自动 Anthropic cache |
| `anthropic_prompt_caching_ttl` | cache 寿命，默认 5m |
| `stored_in_db` | true=In DB，false=In Config，null=Not Set |
| fallback 图 | `{ [primary_model]: string[] }` 列表 |

## 状态

loading：等 callbacks + router fields。empty：无 fallback 行。forbidden：非 admin。Select 清空走 reset。Update 失败保持原值。窄屏 General 表横向滚动。

## 验收

能改 strategy 并保存；fallback 链增删；Prompt Caching 开关立即生效；General 行 Update/Reset 改 Status 徽章；桌面与窄屏；不得 mock 表格。
