# 跨模块业务回归

[English](readme.md) · [全功能实现说明](../../docs/development/implementation.md)

## 职责与实现契约

本目录通过真实 HTTP 网关、真实 PostgreSQL、独立 schema 与随机监听端口验收业务链。默认上游为可编排的本地 fake，真实模型通过 live 环境显式启用；Redis 可选但冷却和热状态验收必须连接真实 Redis。
harness_test 构造服务、租户、模型与请求辅助，setup/chain_support 提供资源流程，mode_test 为业务链切换模拟/真实上游。用例不能只判断 HTTP 200，要观察上游次数、调用 ID、模型/权限/模板来源、费用响应头、日志、费率快照及五个费用作用域。
route_template_config_test 覆盖完整配置往返、平台播种一次、整文档优先级、作用域隔离、重试只计一笔、权重、超时热更新、冷却、表单保持/清空、引用删除和跨组织拒绝。route_template_live_test 验证真实模型在模板选择变化后推理、记日志并推动五层费用。pricing_live_test 对真实 usage 做数量×费率与快照核对。公开函数主要为 Test 入口，不是生产 HTTP API；完整文件矩阵与执行流程见[回归方案](../../docs/development/regression.md)。
多供应商确定性用例只通过共享解析器读取 `testdata/config_provider.yaml`，把其中的 base 替换成两个独立环回上游，写入具名合成凭据且不读取 `key_env`；调用记录只保留解析后的供应商身份，不保存原始凭据值。配置部署使用 `litellm_params.deployment_id`，通过 `/model/new` 创建的数据库部署使用 `model_info.id`。覆盖最终完整周期聚合、物理连接完全相同的数据库重复行、模板继承、作用域/模型隔离、重试、零权重、流式、并发、响应亲和、禁用、账务，以及 supplier A/B 冷却 key 的 Redis 隔离；详细契约见[多供应商同模型回归](../../docs/development/multi-supplier-regression.md)。

## 源码职责与入口

### doc.go

内部实现和协议边界见 [doc.go](doc.go)。

## 对外 HTTP 边界

无本目录直接登记的 HTTP 路由。导出的 Go API 由上层调用；运行时目录调度或调用宿主的入口应沿依赖链追踪。

## 依赖关系

[internal/logx](../logx/readme_cn.md).

## 验证与维护入口

| 测试文件 | 场景入口 |
| --- | --- |
| [budget_chain_test.go](budget_chain_test.go) | `TestPersonalBudgetChain`, `TestTeamBudgetChain`, `TestProjectBudgetChain`, `TestOrganizationBudgetChain`, `TestKeyBudgetChain`, `TestBudgetNamesTheNarrowestExhaustedScope` |
| [budget_test.go](budget_test.go) | `TestBudgetAccumulatesAcrossCalls`, `TestRateLimitIsSeparateFromBudget`, `TestUnlimitedScopeIsUnlimited` |
| [chain_support_test.go](chain_support_test.go) | fixture/辅助函数，无顶层 Test |
| [chain_test.go](chain_test.go) | `TestRequestChain` |
| [consistency_test.go](consistency_test.go) | `TestResponseUsageAndLogsAgree`, `TestUsageCountersMatchTheBill`, `TestCacheHitIsFreeAndLoggedAsOne`, `TestFailedCallIsLoggedButNotCharged`, `TestLogDetailMatchesTheRowInTheList` |
| [endpoints_test.go](endpoints_test.go) | `TestChatEndpointRewritesAndAnswers`, `TestEmbeddingEndpointUsesItsOwnPath`, `TestSeedanceBypassCreatesAndPollsATask`, `TestABypassCannotBeFilledInFromTheConsole` |
| [fallback_test.go](fallback_test.go) | `TestFallbackChain`, `TestCooldownSkipsADeployment` |
| [fal_billing_test.go](fal_billing_test.go) | `TestFalSettlementUsesOutputSecondsAndDeduplicates`：模拟 fal 视频任务创建、排队、处理中、失败和重复完成轮询；只按实际输出时长结算一次，核对费用响应头、五层累计及账单明细。 |
| [guardrail_test.go](guardrail_test.go) | `TestGuardrailBlocksBeforeTheUpstreamIsDialed`, `TestGuardrailBlockIsLoggedButNotCharged`, `TestGuardrailOnlyAppliesToTheModelsItCovers`, `TestGuardrailTrialDoesNotChangeStorage`, `TestGuardrailRedactRewritesInsteadOfBlocking` |
| [harness_test.go](harness_test.go) | `TestMain` |
| [interaction_test.go](interaction_test.go) | `TestGuardrailBlockDoesNotConsumeBudget`, `TestCacheHitDoesNotConsumeBudget`, `TestManagerActionsAreAudited`, `TestChangingABudgetTakesEffectImmediately`, `TestBlockingAKeyTakesEffectImmediately`, `TestDeletingATeamStopsItsKeys`, `TestModelRestrictionAndBudgetAreIndependent` |
| [live_test.go](live_test.go) | `TestLiveConfiguredVendorsAnswer`, `TestLiveBypassCreatesAndPollsARealTask` |
| [logs_test.go](logs_test.go) | `TestEveryLogViewAgreesOnTheSameTraffic`, `TestCallIdIsTheSameEverywhereItAppears`, `TestTenantSeesOnlyItsOwnLogs`, `TestDailyActivityCountsTheCalls`, `TestPromptStorageFollowsTheSwitch`, `TestSpendCalculateUsesThePriceCatalog` |
| [mode_test.go](mode_test.go) | fixture/辅助函数，无顶层 Test |
| [model_limit_test.go](model_limit_test.go) | `TestModelLimitChain` |
| [models_test.go](models_test.go) | `TestModelListShowsWhatTheCallerMayUse`, `TestModelsOfEveryEndpointTypeAreListed`, `TestModelAvailableCarriesThePriceFromTheCatalog`, `TestModelAvailableLeavesUnknownPricesEmpty`, `TestEveryDeclaredTransportExistsInTheCatalog`, `TestTransportsAreRegisteredBypasses`, `TestPriceCatalogRatesArePerToken`, `TestAddingAModelMakesItCallable`, `TestDeletingAModelStopsIt`, `TestConfigModelCannotBeDeletedFromTheConsole`, `TestDisabledModelIsRefused`, `TestUnimplementedProviderNamesItsOwnProblem`, `TestMissingCredentialStillBlamesTheCredential`, `TestUnknownModelIsRefused` |
| [multi_supplier_test.go](multi_supplier_test.go) | `TestMultiSupplierConfiguredWeights`, `TestMultiSupplierTemplateWeights`, `TestMultiSupplierModelRulesAndScopeIsolation`, `TestMultiSupplierRetryAndBilling`, `TestMultiSupplierZeroWeightExcludesFailover`, `TestMultiSupplierAllZeroRejects`, `TestMultiSupplierStreamingAndConcurrentWeights`, `TestMultiSupplierResponseAffinity`, `TestMultiSupplierDatabaseDuplicateDeployments`, `TestMultiSupplierCooldownIsolation` |
| [more_cases_test.go](more_cases_test.go) | `TestIdempotencyReplaysWithoutASecondCharge`, `TestStreamSkipsIdempotency`, `TestMalformedRequestsNeverDial`, `TestTPMLimitIsNotABudgetError`, `TestUpstream429FailsOverAndAMissingKeyIsSkipped`, `TestGuardrailDoesNotCoverEmbeddings`, `TestEachInferenceFamilyReachesItsOwnUpstreamPath` |
| [permission_test.go](permission_test.go) | `TestTenantIsolationAcrossOrganizations`, `TestMemberCannotReadAnotherTenant`, `TestMemberCannotAdminister`, `TestKeyLifecycleAndScope`, `TestKeyRegenerateInvalidatesTheOldSecret`, `TestUnauthenticatedIsRefused`, `TestInvalidKeyIsRefused`, `TestOrganizationAndTeamHierarchy` |
| [pricing_live_test.go](pricing_live_test.go) | `TestLiveRealModelsAreBilledFromTheCatalog`, `TestLiveVendorUsageFieldsAreUnderstood`, `TestLiveStreamingIsBilledFromTheRealUsage`, `TestLiveSameVendorViaEitherProtocolIsBilledAlike`, `TestLiveUsageOracleHonorsExplicitZeroAndNullAliases`, `TestLiveUsageOracleDistinguishesNestedSubsetFromAnthropicCache`, `TestParseStreamUsageMergesResponseUsageEvents`, `TestSameRatePricesHandlesUnequalRateListLengths`, `TestSameRatePricesRequiresComparableTokenRates` |
| [pricing_test.go](pricing_test.go) | `TestAWindowPricedModelIsChargedTheWindowItLandedIn`, `TestAPerSecondModelIsNotRecordedAsFree`, `TestLogDetailExplainsTheChargeWithoutRecomputing`, `TestTheBreakdownSurvivesAPriceChange`, `TestAnUnpricedCallIsNotRecordedAsFree`, `TestACachedCallIsNotBilledAtTheInputRate` |
| [pricing_units_test.go](pricing_units_test.go) | `TestConsolePriceFormIsBilledForEveryDimension`, `TestTheFlatPeakFieldIsActuallyRead`, `TestAnthropicShapedUsageIsBilledWhole`, `TestTheCatalogPricesEveryMeasureItQuotes`, `TestEveryCatalogModelIsReachableByTheNamesDeploymentsUse` |
| [protocol_test.go](protocol_test.go) | `TestSameModelAnswersThreeChatProtocols`, `TestEveryProtocolIsBilledTheSameWay`, `TestStreamingAnswersArriveAndAreBilled`, `TestStreamingAndNonStreamingAgreeOnTokens`, `TestEmbeddingIsBilledFromItsOwnUsage`, `TestUnknownPathIsNotFound` |
| [removed_surfaces_test.go](removed_surfaces_test.go) | `TestRemovedSurfacesStayGone` |
| [reset_test.go](reset_test.go) | `TestResetSpendChain`, `TestPasswordResetChain` |
| [route_template_config_test.go](route_template_config_test.go) | `TestRouteTemplateConfigurationRoundTrip`, `TestRouteTemplateSeedsOnceFromPlatformDefaults`, `TestRouteTemplatePrecedenceUsesOneWholeDocument`, `TestRouteTemplateEditsApplyOnlyToSelectedScopes`, `TestRouteTemplateRetriesAndFailoverBillOnce`, `TestRouteTemplateWeightsDriveTraffic`, `TestRouteTemplateModelRoutingUsesOverridesAndRootDefault`, `TestRouteTemplateTimeoutChangesWithoutRestart`, `TestRouteTemplateCooldownUsesSelectedThresholds`, `TestRouteTemplateScopeFormsKeepAndClearSelections`, `TestRouteTemplateUsageAndDeletionCoverEveryScope`, `TestRouteTemplatePermissionsRejectCrossOrganizationChanges` |
| [route_template_live_test.go](route_template_live_test.go) | `TestLiveRouteTemplateSelectionAndBilling` |
| [weighted_live_test.go](weighted_live_test.go) | `TestLiveConfiguredWeightedRouting` |
| [weighted_live_metadata_test.go](weighted_live_metadata_test.go) | `TestDecodeProviderMetadataRejectsInvalidInput`, `TestExpectedWeightedCountsAvoidsIntegerOverflow` |
| [seedance_billing_test.go](seedance_billing_test.go) | `TestSeedanceSettlementUsesMeasuredBandAndDeduplicates` |
| [route_template_test.go](route_template_test.go) | `TestNoTemplateSelectedBehavesLikeThePlatformDefault`, `TestATeamsOwnTemplateBeatsItsOrganizations`, `TestASessionPicksUpItsTeamsTemplate`, `TestTheConsoleSaysWhichTemplateIsInEffect`, `TestDeletingATemplateInUseIsRefusedWithTheList`, `TestAKeyCreatedWithATemplateKeepsIt` |
| [router_settings_test.go](router_settings_test.go) | `TestRouterSettingsChain` |
| [routing_test.go](routing_test.go) | `TestRoutingStrategyChain`, `TestLeastBusyChain`, `TestUnknownStrategyChain`, `TestExactNameBeatsWildcard`, `TestSessionPinOverridesStrategy` |
| [split_test.go](split_test.go) | `TestWeightedSplitSendsTrafficToBothDeployments`, `TestWeightedSplitDoesNotChangeSimpleShuffle`, `TestSplitStillBillsAndLogsEveryCall`, `TestSplitIsEvenWhenNoWeightsAreSet` |

```bash
go test ./internal/regression -count=1
```

数据库验收设置 XHUB_REGRESSION_STRICT=1 并检查跳过项；Redis 和真实供应商需单独配置。每个同时运行的回归进程必须使用专用 Redis，花费队列和冷却 key 属于共享外部状态，不能跨 runner 共用。harness 清理先等待 HTTP handler 结束，在私有 schema 仍存在时排空最终花费记录，再关闭 Redis 客户端。接口、字段或行为改变后同步本说明及相关功能文档。
