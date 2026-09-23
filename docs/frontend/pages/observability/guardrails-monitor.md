# Guardrails 监控

- Route: `/guardrails-monitor`
- Nav: OBSERVABILITY
- Status: `specified`

## 目的

按时间窗看各 guardrail 的评估次数、拦截、时延与成本。只读监控；配置仍在 `/guardrails`。无 `viewGuardrailUsage` 整页 AdminOnlyNotice。

## 布局

**顶栏** 概览：PageHeader 图标 HeartPulse、标题 Guardrails Monitor、副标题「Monitor guardrail performance across all requests」。右侧 utilities：AdvancedDatePicker（默认近 7 天，无时刻）+ Export Data（`title="Coming soon"`，禁用预期）。详情：左「Back to Overview」，右日期控件仍可用，标题旁 status 点（healthy / warning / critical）与 provider 徽章，设置齿轮打开 Evaluation Settings。

**筛选** 页级只有日期窗，无 Team/Key Filter Drawer。表头可排 Requests、Fail Rate、Avg. latency added、Cost（默认 Fail Rate 降序）。详情 Logs Tab 可按 action（blocked / passed / flagged）收窄，不是页级筛选。

**表** 概览指标卡：Total Evaluations、Blocked Requests、Pass Rate、Avg. latency added、Guardrail Cost（可打开 CalcPopover 看「各 guardrail 成本之和」；无单价的 usage units 用警告三角排除）、Active Guardrails。其下 ScoreChart。表列：Status 点、Guardrail（可点名称）、Provider 色徽章（Bedrock / Google Cloud / 网关 / Custom）、Requests、Fail Rate（>15% 红、>5% 黄，trend ↑↓）、Avg. latency added、Usage Units、Cost。

**Tab** 详情才有 Overview / Logs。Overview：Requests Evaluated、Fail Rate、Avg. latency added、GuardrailUsageBreakdown、同一套 LogViewer。Logs：完整日志表。概览页无 Tab。

**抽屉** 无右侧 Sheet。点 Guardrail 名称进入整页详情（替换概览），不是抽屉。日志行的 snippet / reason 在详情页 LogViewer 内展开。

**模态** EvaluationSettingsModal（概览齿轮与详情齿轮）。Export 未实现，点击不打开下载框。无删除确认。

**URL** `?guardrail=<id>` 进详情（history push）；Back 用 replace 清参数。日期不进 URL。无 `log_id`。

## 交互

只读。改日期重拉 overview / detail / logs。点 Guardrail 名称 → `setSelectedGuardrailId` → 详情。点 Back → 回概览。点 Cost 旁公式 → CalcPopover，不改数据。点齿轮 → Evaluation Settings，保存后关模态，表刷新评估口径。Export Data 保持 Coming soon。

empty：窗口内无评估时指标为 0、表「该窗口没有 guardrail 评估」，不是「还没有 Guardrail」（配置页才是）。详情 404：「Failed to load guardrail details」+ Back，不是空表。forbidden：无 `viewGuardrailUsage` 整页 AdminOnlyNotice（「Guardrails Monitor」），不渲染指标卡，不把无权限画成 0 evaluations。

## 操作 → 契约

| 用户动作 | Method Path | 成功后 UI |
|---|---|---|
| 概览 | `GET /guardrails/usage/overview` | 卡片、图、表 |
| 日志 | `GET /guardrails/usage/logs` | 详情 LogViewer |
| 详情 | `GET /guardrails/usage/detail/{guardrail_id}` | 详情指标与 breakdown |

## 字段

| 字段 | 含义 |
|---|---|
| `guardrail_id` | id |
| `guardrail_name` | 显示名 |
| `action` | blocked / passed / flagged |
| `requestsEvaluated` | 评估次数 |
| `failRate` | 拦截率 % |
| `avgLatency` | 附加时延 ms |
| `cost` | 该 guardrail 计价后 USD |
| `usageUnits` | 按 counter 的用量 |

## 状态

loading：概览表骨架；详情未到时居中 spinner。empty：零评估空表，文案绑定时间窗。forbidden：AdminOnlyNotice，禁止空表冒充。未计价 units 用警告三角，Cost 列显示已计价部分，不把缺失单价当成 $0。超时保留当前窗，不把另一窗的 failRate 画进来。

## 验收

桌面与窄屏走通上表每一行动作；不得 mock 表格。无权限必须 Notice，不能进详情深链内容。
