# 成本优化

- Route: `/cost-optimization`
- Nav: OBSERVABILITY
- Status: `specified`
- 实验性仪表盘（页内 info 条，反馈链到讨论区）

## 目的

把 prompt 压缩、prompt 缓存、自动路由与 shadow eval 的节省画在一页。Auto-Router 的创建入口在 Models + Endpoints 的 Auto-Routers Tab，本页只看效果与评测。

## 布局

**顶栏** PageHeader：图标 PiggyBank、标题 Cost Optimization、副标题说明压缩/缓存，并指向 Models + Endpoints。其下 line Tab。无 Create Budget 类主按钮。日期由各 Tab 内 AdvancedDatePicker 控制（Overall 与 Auto-Router 共用 `useDailyActivityRange`，默认近 30 天）。分页回退时 PaginationStatusAlerts 在 Tab 内容之上。

**筛选** Overall：日期 + 累积/按日（Savings Accumulation）。Auto-Router：日期 + 路由器下拉（All routers 或某一 auto-router）。Prompt Compression / Caching 无实体筛选。Shadow eval 的目标、流量百分比、方向在启动表单里，不在页级 Filter Drawer。

**表** Overall：SavingsTiles、按驱动甜甜圈（只画正节省）、节省面积/柱、工具 spend 柱（需 `viewProxyWideCostData`）。Prompt Compression：已有压缩 guardrail 卡 + Name / API base / defaultOn 表单。Prompt Caching：设置面板 + CacheLeakageCard。Auto-Router：英雄卡（Total estimated savings、Actual auto-router spend、最高档基线）、分档 turns 条、Bucket 表（Bucket / Turns / Hit rate）、Cache hit rate、其下 Shadow eval 任务表（状态徽章 running/completed/stopped、目标、router、win rate、花费）。

**Tab** Overall（默认，value=`usage`）。有 `viewProxyWideCostData` 才出现 Prompt Compression、Prompt Caching、Auto-Router。切走过的 Tab keepMounted。无 `viewProxyWideCostData` 时只留 Overall，压缩/缓存/路由评测对内部用户隐藏。

**抽屉** 无通用行抽屉。Shadow eval 行展开同页 Slice 表（Judged turns、Router wins、对臂 wins、Ties、Judge confidence、Router cost），不跳路由。

**模态** 无全屏创建模态。Shadow eval 用页内 StartForm（目标 key/router、shadow_percentage、direction、max_turns/max_budget）。Stop 在行内，不另开确认框以外的模态。Prompt Compression 提交是表单，不是 Dialog。

**URL** Tab 与日期不进 query。刷新回 Overall 与默认 30 天。任务 id 不进 URL。

## 交互

Overall 与 Auto-Router 的图表只读。Prompt Compression 的 Add、Prompt Caching 的设置字段、Shadow eval 的 Start/Stop 是写操作。点 Overall 看累积节省；负的 auto-router 冷缓存写入不进甜甜圈，但合计仍带符号。点 Auto-Router 路由器下拉 → `GET /auto_router/benchmarks?api_key=` 重算英雄卡与分档。点 Start shadow eval → `POST /auto_router/shadow_eval/start`，running 行每 15s 轮询。点 Stop → `POST /auto_router/shadow_eval/{job_id}/stop`，徽章变 stopped。Compression 表单校验 Name、API base；成功后刷新压缩 guardrail 列表。

empty：无日活时 Overall 图为空、节省为 $0，文案「该窗口没有可计算的节省」，不是「还没有密钥」。无 auto-router 时 Auto-Router Tab 居中「还没有 Auto-Router，请到 Models + Endpoints 创建」。无 shadow eval 任务时 StartForm 下方空表，不是错误。forbidden：无 `viewProxyWideCostData` 只见 Overall，工具 spend 与压缩/缓存/Auto-Router Tab 不出现；403 用说明，不画成 $0 节省。

## 操作 → 契约

| 用户动作 | Method Path | 成功后 UI |
|---|---|---|
| 日活（节省序列） | `GET /user/daily/activity/aggregated` | Overall 图与 tiles |
| benchmarks | `GET /auto_router/benchmarks` | Auto-Router 英雄卡与分档表 |
| shadow 列表 | `GET /auto_router/shadow_eval` | 任务表，running 轮询 |
| 启动 shadow eval | `POST /auto_router/shadow_eval/start` | 新 running 行 |
| 停止 | `POST /auto_router/shadow_eval/{job_id}/stop` | status=stopped |
| 压缩规则列表 | `GET /guardrails/list` | Compression 卡 |
| 新增压缩 | `POST /guardrails` | 新卡，表单清空 |

## 字段

| 字段 | 含义 |
|---|---|
| `job_id` | shadow 任务 |
| `routing_strategy` | 策略 |
| `savings` / `saved_spend` | 相对最高档的估计节省 USD |
| `shadow_percentage` | 镜像流量百分比 |
| `direction` | forward / reverse |
| `status` | running / completed / stopped |
| `hit_rate_pct` | 路由缓存命中 |

## 状态

loading：日活与 benchmarks 用卡片骨架，不闪 $0。empty：见上，空节省 ≠ 无权限。forbidden：藏 Tab 与工具 spend。校验失败贴在 Compression / StartForm 字段旁，可改后重试。超时锁定原 payload（Start 按钮不丢已填目标）。实验条常驻，不替代错误 banner。

## 验收

桌面与窄屏走通上表每一行动作；不得 mock 表格。无 `viewProxyWideCostData` 时不得露出 Auto-Router 或 Shadow eval。
