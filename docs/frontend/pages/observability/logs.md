# 请求日志

- Route: `/logs`
- Nav: OBSERVABILITY
- Status: `specified`

## 目的

单请求与 session 明细。默认看 Request Logs；有权限时兼看 Audit Logs、Deleted Keys、Deleted Teams。详情在右侧抽屉，不离开表。

## 布局

**顶栏** 页级无线 Tab：Request Logs（默认）、Audit Logs（`viewAuditLogs`）、Deleted Keys、Deleted Teams（`viewDeletedTeams`）。无 Create。Request Logs 工具条：时间快捷（含 Custom Range）、Live Tail 开关、Hide Health Checks 开关、Reset Filters。Live Tail 开且在第一页时每 15s 刷新，顶一条绿 banner「Auto-refreshing every 15 seconds」+ Stop。时间与开关存 sessionStorage，刷新保留，不进 URL。

**筛选** DataTable 工具条搜索（防抖，写入 `search`）+ Filter Drawer。抽屉字段：Team ID（可搜索团队）、Status（All / Success / Failure）、Cache（All / Cache Hit / Cache Miss）、Key Alias（按团队分页搜）、User ID（当前时间窗内内部用户）、End User（当前窗 end user）、Error Code（预置 + 自定义）、Error Message、Key Hash、Session ID、Model（模型 id）、Public model / search tool。改任一筛选或排序都回到第一页并清 session cursor。Spend / token 通过列排序与 Cost/Tokens 列表达，不是独立数字输入。

**表** Request Logs 列：Time、Type（LLM / MCP / Agent / Batch，或多调用 session 计数徽章）、Status（Success / Failure；batch 部分失败为 `n/m succeeded`）、Session ID、Request ID（batch 显示 batch id +「batch cost」）、Cost（多调用显示 session total，MCP 花费另注）、Duration (s)、TTFT (s)、Team Name、Key Hash、Key Alias、Model、Tokens（total 与 prompt+completion）、Internal User、End User、Tags。默认按 Time 降序。行点击打开该请求抽屉。Audit Logs / Deleted Keys / Deleted Teams 各用自己的表，不共用这些列。

**Tab** 页级见顶栏。抽屉内 Request / Response 子 Tab，Formatted / JSON 切换。Session 模式左侧事件树（LLM / Agent / MCP / Auto-Router 图标），可按 duration 排序；J/K 上下、Escape 关闭。失败行顶部 Request Failed 条。有 routing_decision 显示 Routing 卡；有 cost_breakdown 显示 Cost；有 guardrail_information 显示 Guardrails；另有 Tools、Vector Store、Eval、Metadata。

**抽屉** 右侧 Sheet。打开条件：`log_id` 命中一行，或 `session_id` 指向多调用 session。头：request_id 可复制、模型、状态、花费。点 Key Hash 列不关日志抽屉，另开该哈希的 Key Info（只读前缀，不展示明文）。点 Session ID 以 session 模式打开，侧栏拉 `GET /spend/logs/session/ui`（page_size 100，最多 50 页）。

**模态** Request Logs 无创建模态。无密钥明文弹窗。Deleted Keys / Deleted Teams 的恢复或确认走各自资源确认框，不混进 Request Logs。

**URL** `?log_id=` 打开单请求；`?session_id=` 打开 session（可与 `log_id` 同在，侧栏高亮该请求）。关抽屉清这两个参数。页级 Tab、时间窗、Live Tail、筛选不进 URL。

## 交互

Request Logs 只读。筛选、排序、Live Tail、Hide Health Checks 只改查询。写操作不在本 Tab（Audit / Deleted 另计）。点行 → `openLog(request_id)` → 抽屉 + `?log_id=`。点 Session ID → `openSession` → 侧栏事件。点 Key Hash → `GET /key/info` 填 Key Info，列表仍只显示哈希。Reset Filters 清抽屉筛选、搜索、时间回到近 24h，Live Tail 默认开。

请求/响应体：`messages` 与 `response` 皆空且非失败时，抽屉内显示「Request/Response Data Not Available」，引导在 `proxy_config.yaml` 设 `general_settings.store_model_in_db` 与 `store_prompts_in_spend_logs`，或到 **Admin Settings → Logging Settings** 打开同一开关。该开关只影响之后的新请求，已写入的行不会补上正文。正文仍按角色脱敏。

empty：无筛选时「No requests yet」——经本网关代理的请求会出现在这里；有筛选时「No matching requests」——该时间窗没有命中筛选的请求。forbidden：无 `viewAuditLogs` 不出现 Audit Logs Tab（不是空审计表）；无 `viewDeletedTeams` 同理。内部用户只看见自己的 spend 日志。未就绪（缺 token/role）整页 spinner，不闪空表。

## 操作 → 契约

| 用户动作 | Method Path | 成功后 UI |
|---|---|---|
| 列表 | `GET /spend/logs/ui` | 填表（`group_by_session=true`） |
| 详情 | `GET /spend/logs/ui/{request_id}` | 抽屉正文（懒加载 messages/response） |
| Session | `GET /spend/logs/session/ui` | 抽屉侧栏事件 |
| 内部用户选项 | `GET /spend_logs/users` | User ID 筛选 |
| End user 选项 | `GET /spend_logs/end_users` | End User 筛选 |
| Key 信息 | `GET /key/info` | Key Info 面板 |

## 字段

| 字段 | 含义 |
|---|---|
| `request_id` | 调用 id |
| `session_id` | 多调用会话 |
| `model` | 别名 |
| `spend` | USD |
| `prompt_tokens` | 输入 |
| `completion_tokens` | 输出 |
| `status` | Success / Failure |
| `startTime` | 开始 |
| `request_duration_ms` | 耗时 |
| `cache_hit` | 缓存命中 |
| `call_type` | llm / mcp / agent / batch |

## 状态

loading：表骨架；抽屉正文未到时「Loading request & response data...」，不把缺正文误报成未开启 store_prompts。empty：见上，空表不是 403。forbidden：按能力藏 Tab；越权列表 API 用说明，不假装零请求。Live Tail 仅第一页轮询。429 / 超时锁定当前页，不丢已开抽屉的 `log_id`。

## 验收

桌面与窄屏走通上表每一行动作；不得 mock 表格。未开 `store_prompts_in_spend_logs` 时抽屉必须出现配置说明，而不是空白 JSON。
