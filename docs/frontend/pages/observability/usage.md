# 用量

- Route: `/usage`
- Nav: OBSERVABILITY
- Status: `specified`

## 目的

按日/模型/团队/客户/标签/Agent 聚合的 spend 与 token。只读仪表盘；导出与 Ask AI 不改 spend 数据。

## 布局

**顶栏** 左为 Usage View 选择器（图标 + 标题「Usage View」+ 下拉），右为 AdvancedDatePicker。默认窗口近 7 天。管理员在 Global Usage 下另有「Filter by user」下拉，可把全站视图收窄到某一内部用户。右上在 Global / Your Usage 内层 Tab 旁放 Ask AI、Export Data。日期切换立刻进入 isDateChanging，卡片数字必须带当前窗口 stamp，禁止把上一窗口的数画到新窗口。

**筛选** 视图选择器决定整页内容，不是表内 Filter Drawer。选项：Global Usage（非管理员标签为 Your Usage）、Your Usage（仅管理员）、Organization Usage（`viewOrganizationUsage`）、Team Usage、Customer Usage（仅管理员）、Tag Usage（管理员或内部用户）、Agent Usage (A2A)（`viewAgentUsage`）、User Usage（仅管理员）、User Agent Activity（仅管理员）。Tag Usage 顶部可关的 info banner 说明可复用凭证会以 `Credential: ` 前缀出现。Customer Usage 的实体列表来自客户账号（alias 或 user_id），不是内部用户。

**表** Global / Your Usage 的 Cost Tab 不是行表：指标卡（Total / Successful / Failed Requests、Average Cost per Request、Total Tokens，点 Tokens 卡展开 Input / Output / Cache Read / Cache Write）+ Daily Spend 柱图 + 管理员 Gateway Requests by Endpoint 堆叠柱 + Top Virtual Keys + Top Public Model Names / Top Models（5/10/25 条与 groups/models 切换）+ Spend by Provider。Endpoint Activity Tab 才是按 `breakdown.endpoints` 聚合的柱图、折线与明细表。Entity 视图（Team / Organization / Customer / Tag / Agent / User）走 EntityUsage：实体多选 + 趋势 + 明细。User Agent Activity 走独立活动日志。

**Tab** 仅 Global Usage 与 Your Usage 有内层 Tab：Cost、Model Activity、Key Activity、MCP Server Activity、Endpoint Activity。切 Tab 不丢日期与用户筛选。Organization / Team / Customer / Tag / Agent / User / User Agent Activity 无这组内层 Tab。Cost Optimization、Old Usage 不在本页。

**抽屉** 无行详情抽屉。Ask AI 打开右侧 UsageAIChatPanel，对话走用量问答，不打开单请求日志。点 Top Key 可进 Key Activity 口径，不打开密钥明文。

**模态** EntityUsageExportModal（Export Data）按当前日期窗口导出聚合，默认 entityType=team。CloudZeroExportModal 组件存在但不从本页主按钮打开。导出不改库。无创建/删除确认框。

**URL** 当前实现不把视图、日期、用户写入 query。刷新回到 Global（或非管理员 Your Usage）与默认 7 天。深链应后续补 `view` / `from` / `to` / `user`。

## 交互

本页以只读为主。改日期、视图、用户、Top N、groups/models、展开 token 分解只触发重新拉取，不写 spend。Ask AI 与 Export 是旁路：前者 `POST /usage/ai/chat`，后者下载当前窗口聚合。无 Create / Delete / Block。

点 Usage View → Customer Usage：整页换成按客户账号的 EntityUsage，实体下拉来自客户列表；空客户时筛选读 loading 而非「零客户」。点 Endpoint Activity：用同一份日活的 `breakdown.endpoints` 画柱/线/表，不另打独立 endpoint API。点 Successful / Failed Requests 旁 Info：管理员看到网关计数（部署级、与 spend 日志独立）；无网关表时 Failed 提示含路由失败与工具失败。点 Export Data → 导出模态，确认后下载，表与图不变。点 Ask AI → 右侧面板，不关图表。

empty：选定窗口无日活时图表空态 + 指标卡为 0，文案按视图区分（Global「该窗口无请求」/ Customer「没有客户 spend」/ Tag「该窗口无标签」），禁止假装成权限错误。forbidden：非管理员看不到 Customer / User / User Agent Activity / 部署级网关计数；无 `viewOrganizationUsage` 时 Organization 选项消失，若 URL 残留则同帧回落 Global；无 `viewAgentUsage` 时 Agent Usage 不出现。内部用户只看见自己的 Your Usage 数据。

## 操作 → 契约

| 用户动作 | Method Path | 成功后 UI |
|---|---|---|
| 日活聚合 | `GET /user/daily/activity/aggregated` | 填卡片、图、Top 表 |
| 日活分页回退 | `GET /user/daily/activity` | 聚合失败时分页填同一套 UI |
| 网关请求计数 | `GET /gateway/daily/activity` | 管理员 Successful/Failed 与按 endpoint 柱 |
| 标签列表 | `GET /tag/list` | Tag Usage 实体下拉 |
| Ask AI | `POST /usage/ai/chat` | 右侧对话 |
| 团队日活 | `GET /team/daily/activity` | Team Usage |
| 组织日活 | `GET /organization/daily/activity` | Organization Usage |

## 字段

| 字段 | 含义 |
|---|---|
| `start_date` | 窗口起 |
| `end_date` | 窗口止 |
| `spend` | USD |
| `prompt_tokens` | 输入 token |
| `completion_tokens` | 输出 token |
| `total_tokens` | 总 token |
| `api_requests` | 请求数 |
| `successful_requests` | 成功 |
| `failed_requests` | 失败 |
| `cache_read_input_tokens` | 缓存读 |
| `cache_creation_input_tokens` | 缓存写 |

## 状态

loading：日期切换用 ChartLoader，卡片不闪成空。empty：窗口内无日活，指标为 0，不假装无权限。forbidden：按能力藏视图与网关计数，不把别人的 spend 画进 Your Usage。分页回退显示 PaginationStatusAlerts（进度、取消）。超时保留上一窗口 stamp 的数，不把过期窗口画进新日期。

## 验收

桌面与窄屏走通上表每一行动作；不得 mock 表格。Customer Usage 与 Endpoint Activity 必须真实存在且可切换。
