# 用量日志、活动与费用报表

[English](readme.md) · [全功能实现说明](../../../docs/development/implementation.md)

## 职责与实现契约

reports.go 提供费用与日志查询，activity.go/entity_activity.go 按日期和作用域聚合，benchmarks.go 保留对应实现，mount.go 登记当前可调用入口。LogsScope 区分个人自有日志、管理团队的服务密钥日志与平台全量；特权正文读取需审计。
分页默认 50、最大 200。会话汇总先按密钥识别调用方，再使用用户；没有调用方身份的记录分别展示，不根据相同 session ID 合并。当前页会话汇总与事件总数不能混淆。每个 HTTP 调用有独立 callID；同一 session 不意味着只记一笔。
费用以 USD、token 以原始整数返回。明确零费用调用仍计请求与 token；历史 provider 和价格快照优先。浏览器 timezone offset 按西区为正解释。/spend/calculate 是当前报价，未知价格的零估算不承诺免费。prompt logging 控制正文、头和响应写入；脱敏在写入时进行，读取仍受权限限制。

## 源码职责与入口

### activity.go

- [`func UserDailyActivity(s Host, w http.ResponseWriter, r *http.Request)`](activity.go) — UserDailyActivity is GET /user/daily/activity. It groups stored usage into the daily rollup the usage page reads, newest day first and paged. The entity breakdown is per user, which is what "用户用量" renders, and the scope is the caller's own rows plus the teams they oversee.
- [`func UserDailyActivityAggregated(s Host, w http.ResponseWriter, r *http.Request)`](activity.go) — UserDailyActivityAggregated is GET /user/daily/activity/aggregated. The same rollup in a single response, which is what "你的用量" and the global view load first. A user id on the query narrows to that account inside the scope.
- [`func GatewayDailyActivity(s Host, w http.ResponseWriter, r *http.Request)`](activity.go) — GatewayDailyActivity is GET /gateway/daily/activity. Request counts come from the same usage rows, split by outcome and route. The scope decides whose calls are counted: a platform administrator seesthe gateway, anyone else sees only their own calls and the calls inside the teams they belong to.

### benchmarks.go

- [`func Benchmarks(s Host, w http.ResponseWriter, r *http.Request)`](benchmarks.go) — Benchmarks returns auto-router benchmark rows. With no sample every count is 0, and latency and hit rate are not invented.

### codec.go

内部实现和协议边界见 [codec.go](codec.go)。

### entity_activity.go

- [`func TeamDailyActivity(s Host, w http.ResponseWriter, r *http.Request)`](entity_activity.go) — TeamDailyActivity is GET /team/daily/activity. The breakdown is per team, paged by day, and limited to the teams the caller may see.
- [`func TeamDailyActivityAggregated(s Host, w http.ResponseWriter, r *http.Request)`](entity_activity.go) — TeamDailyActivityAggregated is GET /team/daily/activity/aggregated. The usage page loads this first for "团队用量". One response covers the whole range.
- [`func OrganizationDailyActivity(s Host, w http.ResponseWriter, r *http.Request)`](entity_activity.go) — OrganizationDailyActivity is GET /organization/daily/activity. The breakdown is per organization, for "组织用量".
- [`func TeamSpendByUser(s Host, w http.ResponseWriter, r *http.Request)`](entity_activity.go) — TeamSpendByUser is GET /team/spend/by_user. Each row is one user inside one team, still inside the caller's usage scope.

### host.go

公开类型：`Host`.

内部实现和协议边界见 [host.go](host.go)。

### mount.go

- [`func Module(h mountHost) httpx.Module`](mount.go) — Module is spend reports, activity summaries, and health probes.

### reports.go

- [`func LogsV2(s Host, w http.ResponseWriter, r *http.Request)`](reports.go) — LogsV2 is the paged spend-log API behind GET /spend/logs/ui. Every row is narrowed by the caller's log scope, so a member sees their own calls and a team administrator additionally sees their team's service keys. The response shape is the one the console's table reads: data plus the paging metadata.
- [`func SessionLogs(s Host, w http.ResponseWriter, r *http.Request)`](reports.go) — SessionLogs lists every call in one caller's session. The list route folds a caller/session pair into a single row; this route is what the drawer opens when that row is clicked. api_key takes precedence over user_id, matching the grouping rule used by collapseSessions.
- [`func LogByID(s Host, w http.ResponseWriter, r *http.Request)`](reports.go) — LogByID serves GET /spend/logs/ui/{request_id} and returns one log with its stored request and response bodies. The row must first be visible through the caller's log scope. Only then is the body read
- [`func Activity(s Host, w http.ResponseWriter, r *http.Request)`](reports.go) — Activity returns the global usage time series. Platform administrators only.
- [`func ActivityModel(s Host, w http.ResponseWriter, r *http.Request)`](reports.go) — ActivityModel returns global usage split by model.
- [`func ActivityCacheHits(s Host, w http.ResponseWriter, r *http.Request)`](reports.go) — ActivityCacheHits returns the global cache-hit summary. Cache hits are not a stored dimension of the roll-up, so the counters are reported as zero rather than guessed from a field that does not exist.
- [`func SpendLogs(s Host, w http.ResponseWriter, r *http.Request)`](reports.go) — SpendLogs returns spend per day. Platform administrators only.
- [`func SpendKeys(s Host, w http.ResponseWriter, r *http.Request)`](reports.go) — SpendKeys totals spend by key. Platform administrators only.
- [`func SpendModels(s Host, w http.ResponseWriter, r *http.Request)`](reports.go) — SpendModels totals spend by model. Platform administrators only.
- [`func SpendProvider(s Host, w http.ResponseWriter, r *http.Request)`](reports.go) — SpendProvider totals spend by provider. The provider is derived from the model name, which is the only provider evidence a usage row carries.
- [`func SpendTeams(s Host, w http.ResponseWriter, r *http.Request)`](reports.go) — SpendTeams totals spend by team. Platform administrators only.
- [`func SpendTags(s Host, w http.ResponseWriter, r *http.Request)`](reports.go) — SpendTags totals spend by tag. Tags are not a stored dimension of the usage row, so the answer is empty rather than invented.
- [`func SpendTagNames(s Host, w http.ResponseWriter, r *http.Request)`](reports.go) — SpendTagNames returns tag names that have appeared.
- [`func SpendEndUsers(s Host, w http.ResponseWriter, r *http.Request)`](reports.go) — SpendEndUsers totals spend by end user. An end user is a LiteLLM concept the new ownership model does not carry, so the answer is empty.
- [`func Keys(s Host, w http.ResponseWriter, r *http.Request)`](reports.go) — Keys returns the same totals as SpendKeys, for the console's older route.
- [`func Users(s Host, w http.ResponseWriter, r *http.Request)`](reports.go) — Users totals spend by account. Platform administrators only.
- [`func TagList(s Host, w http.ResponseWriter, r *http.Request)`](reports.go) — TagList returns the tag catalog read by the usage filter and the key form. Tags are not part of the new model, so the catalog is empty rather than a key-value namespace that nothing writes.
- [`func Tags(s Host, w http.ResponseWriter, r *http.Request)`](reports.go) — Tags totals spend by tag.
- [`func Calculate(s Host, w http.ResponseWriter, r *http.Request)`](reports.go) — Calculate estimates spend from a model and token counts and does not write the database. An unknown model returns 0 rather than an error, because the caller is showing an estimate and a missing priceis not a failed request.
- [`func HealthTestConnection(s Host, w http.ResponseWriter, r *http.Request)`](reports.go) — HealthTestConnection checks whether an upstream or dependency is reachable. A failure writes the reason in JSON and is not always a 500.
- [`func HealthServices(s Host, w http.ResponseWriter, r *http.Request)`](reports.go) — HealthServices returns the health list for dependent services.
- [`func HealthTest(s Host, w http.ResponseWriter, r *http.Request)`](reports.go) — HealthTest runs one health test against the named target. It is public: it proves the process is serving and says nothing about any tenant.

## 对外 HTTP 边界

入口由下表所列注册文件安装。别名共享处理器；路径存在不替代权限和业务断言，授权说明见模块契约及接口参考。

| Method / path | 注册文件 |
| --- | --- |
| `GET /global/spend/teams` | [mount.go](mount.go) |
| `GET /spend/logs/v2` | [mount.go](mount.go) |
| `GET /spend/logs/ui` | [mount.go](mount.go) |
| `GET /spend/logs/ui/{request_id}` | [mount.go](mount.go) |
| `GET /spend/logs/session/ui` | [mount.go](mount.go) |
| `GET /global/spend/logs` | [mount.go](mount.go) |
| `GET /global/spend/keys` | [mount.go](mount.go) |
| `GET /global/spend/models` | [mount.go](mount.go) |
| `GET /global/spend/provider` | [mount.go](mount.go) |
| `POST /global/spend/end_users` | [mount.go](mount.go) |
| `GET /user/daily/activity` | [mount.go](mount.go) |
| `GET /user/daily/activity/aggregated` | [mount.go](mount.go) |
| `GET /team/daily/activity` | [mount.go](mount.go) |
| `GET /team/daily/activity/aggregated` | [mount.go](mount.go) |
| `GET /team/spend/by_user` | [mount.go](mount.go) |
| `GET /organization/daily/activity` | [mount.go](mount.go) |
| `GET /gateway/daily/activity` | [mount.go](mount.go) |
| `GET /global/activity` | [mount.go](mount.go) |
| `GET /global/activity/model` | [mount.go](mount.go) |
| `GET /global/activity/cache_hits` | [mount.go](mount.go) |
| `POST /spend/calculate` | [mount.go](mount.go) |
| `GET /spend/keys` | [mount.go](mount.go) |
| `GET /spend/users` | [mount.go](mount.go) |
| `GET /tag/list` | [mount.go](mount.go) |
| `GET /spend/tags` | [mount.go](mount.go) |
| `GET /global/spend/tags` | [mount.go](mount.go) |
| `GET /global/spend/all_tag_names` | [mount.go](mount.go) |
| `POST /health/test_connection` | [mount.go](mount.go) |
| `GET /health/services` | [mount.go](mount.go) |
| `GET /test` | [mount.go](mount.go) |

## 依赖关系

[internal/auth](../../auth/readme_cn.md), [internal/authz](../../authz/readme_cn.md), [internal/catalog](../../catalog/readme_cn.md), [internal/config](../../config/readme_cn.md), [internal/dataplane](../../dataplane/readme_cn.md), [internal/gateway/identity](../identity/readme_cn.md), [internal/httpx](../../httpx/readme_cn.md), [internal/iam](../../iam/readme_cn.md), [internal/logx](../../logx/readme_cn.md).

## 验证与维护入口

| 测试文件 | 场景入口 |
| --- | --- |
| [activity_test.go](activity_test.go) | `TestDailyActivityResponseFoldsOneDay`, `TestActivityCountsZeroCostCallsAndPreservesRecordedProvider`, `TestDailyActivityReportsKnownCacheReadsAndOmitsUnknownCacheFields`, `TestGatewayActivityBodySplitsByOutcomeAndRoute`, `TestActivityRowWithNoKeyStillLandsInABucket`, `TestActivityBodyGroupsByTheRequestedEntity`, `TestKeepVisibleRefusesAnEmptyIntersection`, `TestActivityDayShiftsByTimezoneOffset`, `TestParseDayRejectsGarbageWithoutWideningTheWindow` |
| [cost_breakdown_test.go](cost_breakdown_test.go) | `TestCostBreakdownMultipliesTokensByRate`, `TestCostBreakdownReadsTheStoredSnapshot`, `TestCostBreakdownIsNotRewrittenByAPriceChange`, `TestCostBreakdownFallsBackForRowsWithoutASnapshot`, `TestCostBreakdownTreatsAnUnreadableSnapshotAsAbsent`, `TestCostBreakdownReportsThePromptSideWhole`, `TestCostBreakdownDoesNotInventAPerTokenRateForPictures`, `TestCostBreakdownAlwaysReportsBothSides`, `TestCostBreakdownKeepsAnUnpricedRowUnpriced`, `TestEventRowCarriesTheBill`, `TestEventRowExposesTheConsoleColumns` |
| [log_guardrail_test.go](log_guardrail_test.go) | `TestEventRowCarriesGuardrailMonitoring` |
| [reports_test.go](reports_test.go) | `TestCollapseSessionsSeparatesCallersWithTheSameSessionID`, `TestCollapseSessionsUsesUserForKeylessCalls` |

```bash
go test ./internal/gateway/usage -count=1
```

数据库验收设置 XHUB_REGRESSION_STRICT=1 并检查跳过项；Redis 和真实供应商需单独配置。接口、字段或行为改变后同步本说明及相关功能文档。
