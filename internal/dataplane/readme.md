# Inference execution and settlement

[简体中文](readme_cn.md) · [Feature implementation reference](../../docs/development/implementation.md)

## Responsibilities and behavior

Serve executes adapted inference; ServeBypass executes registered official transports. Host interfaces split authentication, policy, runtime state, and spend recording so this package does not import gateway.
Execution checks identity, budgets, models, and limits, processes body/guardrails/extensions, resolves a routing document, filters capabilities, orders candidates, and sends upstream. Each retry revalidates policy. Missing credentials skip candidates; retryable 429/5xx and transport failures may retry; other 4xx terminate. Once stream bytes are emitted, retries stop and interrupted streams do not create successful cache or affinity state.
Stream and usage helpers collect billing evidence. Flush peeks up to 500 Redis records, commits an IAM transaction, then acknowledges. Official tasks settle once after terminal success with usable positive usage. Ambiguous creation results must not be retried blindly.

## Source responsibilities and entry points

### doc.go

Internal implementation and protocol boundaries:  [doc.go](doc.go)。

### host.go

Exported types: `SpendLog`, `Adapted`, `Bypass`, `Runtime`, `Host`, `RoutePlan`, `CallNote`.

Internal implementation and protocol boundaries:  [host.go](host.go)。

### live.go

- [`func State(h Runtime) router.State`](live.go) — State 组装这一刻的在途请求、冷却、延迟和用量，交给路由器排序。没有 Redis 时只有在途请求。
- [`func RecordFailure(h Runtime, id string, settings prefs.RouteSettings)`](live.go) — RecordFailure 按这一次请求生效的路由设置记一次失败。allowed_fails 小于 1 时不写 Redis。 cooldown_time 为 0 或缺失时冷却一分钟。 阈值来自 RouteSettingsFor 解析出来的那一份，而不是全局文档：一个团队把 allowed_fails 调低之后，它的失败要按它自己的阈值计数。
- [`func RecordLatency(h Runtime, id string, ms float64)`](live.go) — RecordLatency 把这次延迟累进 Redis，供下次排序使用。没有 Redis 时直接返回。
- [`func RecordUsage(h Runtime, id string, tokens int)`](live.go) — RecordUsage 把这次 token 数累进 Redis。没有 Redis 时直接返回。
- [`func FlushLoop(h Runtime)`](live.go) — FlushLoop 每分钟调用一次 Flush，直到进程退出。只在配置了 Redis 时启动。
- [`func Flush(h Runtime)`](live.go) — Flush 把 Redis 队列里的花费日志写入 PostgreSQL，成功后再从队列确认删除。

### log.go

Internal implementation and protocol boundaries:  [log.go](log.go)。

### official.go

- [`func ServeBypass(h Bypass, w http.ResponseWriter, r *http.Request, hit provider.Hit)`](official.go) — ServeBypass 转发一次已经匹配到的官方调用。正文只改模型字段，状态码和响应字节原样返回。 usage 在后续查询第一次出现时扣一次。

### serve.go

- [`func Serve(h Adapted, w http.ResponseWriter, r *http.Request, op string)`](serve.go) — Serve runs one inference. It picks deployments with the routing strategy, encodes the upstream request, and tries the next deployment after a failure. Serve 跑一次适配推理。预算或护栏拒绝时响应已经写好，函数直接返回。 扩展按注册顺序在缓存和上游之前运行。重试次数小于 1 时按 1 次。没有 api_base 时用供应商默认地址。没有密钥或地址的部署跳过。

### stream.go

Internal implementation and protocol boundaries:  [stream.go](stream.go)。

### usage.go

- [`func EstimateTokens(body map[string]any) int`](usage.go) — EstimateTokens 用正文长度估一个 token 上界。它不是分词器，只给预算和 TPM 一个扣留数。

## External HTTP boundary

This directory registers no direct HTTP route. Higher layers call its Go API; trace catalog dispatch or host calls through the dependency chain.

## Dependencies

[internal/auth](../auth/readme.md), [internal/cache](../cache/readme.md), [internal/catalog](../catalog/readme.md), [internal/config](../config/readme.md), [internal/gateway/prefs](../gateway/prefs/readme.md), [internal/hooks](../hooks/readme.md), [internal/httpx](../httpx/readme.md), [internal/iam](../iam/readme.md), [internal/live](../live/readme.md), [internal/llm](../llm/readme.md), [internal/logx](../logx/readme.md), [internal/plugin](../plugin/readme.md), [internal/provider](../provider/readme.md), [internal/router](../router/readme.md).

## Verification and maintenance


| Test file | Scenario entry points |
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

Use XHUB_REGRESSION_STRICT=1 for database acceptance and inspect skips. Redis and live providers require separate configuration. Update this reference and feature documentation after contract changes.
