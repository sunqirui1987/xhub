# 推理执行与结算数据面

[English](readme.md) · [全功能实现说明](../../docs/development/implementation.md)

## 职责与实现契约

serve.go 提供 Serve，执行 Adapted 宿主的推理链；official.go 提供 ServeBypass，执行已登记官方传输；host.go 把依赖拆为 Adapted、Bypass、Runtime、SpendLog 等接口，避免导入 gateway 形成循环。
请求依次经过身份、预算/模型/限流检查、正文和护栏、hooks/plugins、模板解析、能力过滤、排序、缓存和上游发送。每次重试重新验证身份及限制。缺失凭据跳过候选；429、5xx 和可重试发送失败进入下一次尝试，其它 4xx 终止。收到流输出后不再重试；中断不提交成功缓存或粘性。
stream.go 与 usage.go 处理 SSE usage，log.go 保留成功与失败事件。live.go 的 Flush 从 Redis PeekLogs 读取最多 500 条，通过 IAM 事务落库，再 Ack；SpendAck 是可注入的确认边界。官方任务仅在 terminal success 且有可用正数 usage 时持久结算一次；未知创建结果不能盲目重发。

## 源码职责与入口

### doc.go

内部实现和协议边界见 [doc.go](doc.go)。

### host.go

公开类型：`SpendLog`, `Adapted`, `Bypass`, `Runtime`, `Host`, `RoutePlan`, `CallNote`.

内部实现和协议边界见 [host.go](host.go)。

### live.go

- [`func State(h Runtime) router.State`](live.go) — State 组装这一刻的在途请求、冷却、延迟和用量，交给路由器排序。没有 Redis 时只有在途请求。
- [`func RecordFailure(h Runtime, id string, settings prefs.RouteSettings)`](live.go) — RecordFailure 按这一次请求生效的路由设置记一次失败。allowed_fails 小于 1 时不写 Redis。 cooldown_time 为 0 或缺失时冷却一分钟。 阈值来自 RouteSettingsFor 解析出来的那一份，而不是全局文档：一个团队把 allowed_fails 调低之后，它的失败要按它自己的阈值计数。
- [`func RecordLatency(h Runtime, id string, ms float64)`](live.go) — RecordLatency 把这次延迟累进 Redis，供下次排序使用。没有 Redis 时直接返回。
- [`func RecordUsage(h Runtime, id string, tokens int)`](live.go) — RecordUsage 把这次 token 数累进 Redis。没有 Redis 时直接返回。
- [`func FlushLoop(h Runtime)`](live.go) — FlushLoop 每分钟调用一次 Flush，直到进程退出。只在配置了 Redis 时启动。
- [`func Flush(h Runtime)`](live.go) — Flush 把 Redis 队列里的花费日志写入 PostgreSQL，成功后再从队列确认删除。

### log.go

内部实现和协议边界见 [log.go](log.go)。

### official.go

- [`func ServeBypass(h Bypass, w http.ResponseWriter, r *http.Request, hit provider.Hit)`](official.go) — ServeBypass 转发一次已经匹配到的官方调用。正文只改模型字段，状态码和响应字节原样返回。 usage 在后续查询第一次出现时扣一次。

### serve.go

- [`func Serve(h Adapted, w http.ResponseWriter, r *http.Request, op string)`](serve.go) — Serve runs one inference. It picks deployments with the routing strategy, encodes the upstream request, and tries the next deployment after a failure. Serve 跑一次适配推理。预算或护栏拒绝时响应已经写好，函数直接返回。 扩展按注册顺序在缓存和上游之前运行。重试次数小于 1 时按 1 次。没有 api_base 时用供应商默认地址。没有密钥或地址的部署跳过。

### stream.go

内部实现和协议边界见 [stream.go](stream.go)。

### usage.go

- [`func EstimateTokens(body map[string]any) int`](usage.go) — EstimateTokens 用正文长度估一个 token 上界。它不是分词器，只给预算和 TPM 一个扣留数。

## 对外 HTTP 边界

无本目录直接登记的 HTTP 路由。导出的 Go API 由上层调用；运行时目录调度或调用宿主的入口应沿依赖链追踪。

## 依赖关系

[internal/auth](../auth/readme_cn.md), [internal/cache](../cache/readme_cn.md), [internal/catalog](../catalog/readme_cn.md), [internal/config](../config/readme_cn.md), [internal/gateway/prefs](../gateway/prefs/readme_cn.md), [internal/hooks](../hooks/readme_cn.md), [internal/httpx](../httpx/readme_cn.md), [internal/iam](../iam/readme_cn.md), [internal/live](../live/readme_cn.md), [internal/llm](../llm/readme_cn.md), [internal/logx](../logx/readme_cn.md), [internal/plugin](../plugin/readme_cn.md), [internal/provider](../provider/readme_cn.md), [internal/router](../router/readme_cn.md).

## 验证与维护入口

可选的 [qiniu_live_test.go](qiniu_live_test.go) 会调用真实七牛 Seedance 任务，核验创建、轮询和按实测用量计费；仅在配置对应旁路模型、地址及端点时执行。

| 测试文件 | 场景入口 |
| --- | --- |
| [bypass_logic_test.go](bypass_logic_test.go) | `TestEndpointAndModelLogic` |
| [cache_scope_test.go](cache_scope_test.go) | `TestCacheScopeChangesWithConfigurationSessionAndQuery` |
| [capability_test.go](capability_test.go) | `TestCapabilitySetsStaySeparate` |
| [disabled_test.go](disabled_test.go) | `TestDropDisabledExcludesDisabled` |
| [failure_log_test.go](failure_log_test.go) | `TestServeLogsBuildSkipAndTerminalAuth`, `TestServeLogsMissingCredential`, `TestServeLogsUnimplementedProvider`, `TestServeLogsEmptyStreamAndUpstreamStatus`, `TestServeLogsCacheHitAndStreamMetrics` |
| [live_test.go](live_test.go) | `TestHotDeltasIncludesProject` |
| [official_settlement_test.go](official_settlement_test.go) | `TestOfficialConcurrentCompletedPollsIsolateMetadata`, `TestOfficialSettlementRetriesAfterPersistenceFailure`, `TestOfficialZeroAndPendingPollsKeepUniqueIDs` |
| [official_template_test.go](official_template_test.go) | `TestOfficialTemplateWeightsAndCredentialPin`, `TestOfficialTemplateRetriesHTTPFailure`, `TestOfficialTimeoutDoesNotReplayAmbiguousCreate`, `TestOfficialTaskScopesAndPendingUsage`, `TestOfficialForwardDoesNotLeakGatewayCredentials` |
| [prefer_test.go](prefer_test.go) | `TestPreferDeploymentMovesThePinnedOneFirst`, `TestPreferDeploymentLeavesAnUnknownPinAlone`, `TestOutputTokensCountsStreamedTextWhenUsageIsMissing`, `TestTTFTMillisOmitsAnUnmeasuredDelay` |
| [stream_failure_regression_test.go](stream_failure_regression_test.go) | `TestStreamFailureAfterOutputDoesNotRetryOrPin`, `TestTemplateLookupFailureDoesNotContactUpstream` |
| [usage_stream_test.go](usage_stream_test.go) | `TestCompleteUsagePreservesReportedZeroAndCacheInput`, `TestEstimateTokensDoesNotCountOutputLimitAsInput`, `TestStreamUsageMergesAnthropicFramesAndNestedProviders`, `TestPipeStreamParsesFragmentedSSEAndReturnsBodyError` |

```bash
go test ./internal/dataplane -count=1
```

数据库验收设置 XHUB_REGRESSION_STRICT=1 并检查跳过项；Redis 和真实供应商需单独配置。接口、字段或行为改变后同步本说明及相关功能文档。
