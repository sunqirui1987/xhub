# Cross-module business regression

[简体中文](readme_cn.md) · [Feature implementation reference](../../docs/development/implementation.md)

## Responsibilities and behavior

This package exercises a real HTTP gateway and PostgreSQL using private schemas and random ports. A programmable local upstream makes default tests deterministic; live configuration enables actual models. Redis is optional for basic cases but required to accept cooldown and shared hot-state behavior.
Harness/setup helpers build resources; mode helpers run selected business chains against fake and live vendors. Assertions observe upstream attempts, independent call IDs, policy/template source, cost headers, logs, rate snapshots, and five scope totals rather than only HTTP success.
Template configuration tests cover whole-document precedence, round trips, one-time seeding, isolated edits, retries charged once, weights, dynamic timeout, cooldown, selection clearing, deletion references, and cross-organization denial. A separate live template case checks real model inference and five-scope billing through selection changes. Live pricing verifies provider quantities against recorded rates. These are test entry points rather than production APIs; the full plan lives in [docs/development/regression.md](../../docs/development/regression.md).
Multi-supplier deterministic cases load only `testdata/config_provider.yaml` through the shared provider parser, replace its bases with two independent loopback upstreams, and seed synthetic named credentials without reading `key_env`. Attempt records retain resolved supplier identity rather than raw credential values. Config deployments use `litellm_params.deployment_id`; database deployments created through `/model/new` use `model_info.id`. The suite covers final complete-cycle aggregates, physically duplicate database rows, template inheritance, scope/model isolation, retries, zero weights, streaming, concurrency, response affinity, disable behavior, billing, and Redis-backed isolation between supplier A and B cooldown keys. See [the focused regression contract](../../docs/development/multi-supplier-regression.md).

## Source responsibilities and entry points

### doc.go

Internal implementation and protocol boundaries:  [doc.go](doc.go)。

## External HTTP boundary

This directory registers no direct HTTP route. Higher layers call its Go API; trace catalog dispatch or host calls through the dependency chain.

## Dependencies

[internal/logx](../logx/readme.md).

## Verification and maintenance

| Test file | Scenario entry points |
| --- | --- |
| [budget_chain_test.go](budget_chain_test.go) | `TestPersonalBudgetChain`, `TestTeamBudgetChain`, `TestProjectBudgetChain`, `TestOrganizationBudgetChain`, `TestKeyBudgetChain`, `TestBudgetNamesTheNarrowestExhaustedScope` |
| [budget_test.go](budget_test.go) | `TestBudgetAccumulatesAcrossCalls`, `TestRateLimitIsSeparateFromBudget`, `TestUnlimitedScopeIsUnlimited` |
| [chain_support_test.go](chain_support_test.go) | Fixtures/helpers without a top-level Test |
| [chain_test.go](chain_test.go) | `TestRequestChain` |
| [consistency_test.go](consistency_test.go) | `TestResponseUsageAndLogsAgree`, `TestUsageCountersMatchTheBill`, `TestCacheHitIsFreeAndLoggedAsOne`, `TestFailedCallIsLoggedButNotCharged`, `TestLogDetailMatchesTheRowInTheList` |
| [endpoints_test.go](endpoints_test.go) | `TestChatEndpointRewritesAndAnswers`, `TestEmbeddingEndpointUsesItsOwnPath`, `TestSeedanceBypassCreatesAndPollsATask`, `TestABypassCannotBeFilledInFromTheConsole` |
| [fallback_test.go](fallback_test.go) | `TestFallbackChain`, `TestCooldownSkipsADeployment` |
| [fal_billing_test.go](fal_billing_test.go) | `TestFalSettlementUsesOutputSecondsAndDeduplicates`: simulate task creation, pending, failure, and repeated completion polls; verify measured output-second pricing, one settlement, spend headers, and bill breakdown. |
| [guardrail_test.go](guardrail_test.go) | `TestGuardrailBlocksBeforeTheUpstreamIsDialed`, `TestGuardrailBlockIsLoggedButNotCharged`, `TestGuardrailOnlyAppliesToTheModelsItCovers`, `TestGuardrailTrialDoesNotChangeStorage`, `TestGuardrailRedactRewritesInsteadOfBlocking` |
| [harness_test.go](harness_test.go) | `TestMain` |
| [interaction_test.go](interaction_test.go) | `TestGuardrailBlockDoesNotConsumeBudget`, `TestCacheHitDoesNotConsumeBudget`, `TestManagerActionsAreAudited`, `TestChangingABudgetTakesEffectImmediately`, `TestBlockingAKeyTakesEffectImmediately`, `TestDeletingATeamStopsItsKeys`, `TestModelRestrictionAndBudgetAreIndependent` |
| [live_test.go](live_test.go) | `TestLiveConfiguredVendorsAnswer`, `TestLiveBypassCreatesAndPollsARealTask` |
| [logs_test.go](logs_test.go) | `TestEveryLogViewAgreesOnTheSameTraffic`, `TestCallIdIsTheSameEverywhereItAppears`, `TestTenantSeesOnlyItsOwnLogs`, `TestDailyActivityCountsTheCalls`, `TestPromptStorageFollowsTheSwitch`, `TestSpendCalculateUsesThePriceCatalog` |
| [mode_test.go](mode_test.go) | Fixtures/helpers without a top-level Test |
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
| [setup_test.go](setup_test.go) | `TestProviderSetupAddsFennoaiAndQiniu`, `TestScopedKeyCallsInference`, `TestModelAllowListIsEnforced` |
| [split_test.go](split_test.go) | `TestWeightedSplitSendsTrafficToBothDeployments`, `TestWeightedSplitDoesNotChangeSimpleShuffle`, `TestSplitStillBillsAndLogsEveryCall`, `TestSplitIsEvenWhenNoWeightsAreSet` |

```bash
go test ./internal/regression -count=1
```

Use XHUB_REGRESSION_STRICT=1 for database acceptance and inspect skips. Redis and live providers require separate configuration. Give every simultaneously running regression process a dedicated Redis instance: spend queues and cooldown keys are shared external state and are not safe to share across runners. Harness cleanup waits for HTTP handlers, drains final spend records while its private schema still exists, and then closes the Redis client. Update this reference and feature documentation after contract changes.
