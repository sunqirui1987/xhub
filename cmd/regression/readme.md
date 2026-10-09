# Cross-module business regression

[简体中文](readme_cn.md) · [Feature implementation reference](../../docs/development/implementation.md)

Business regression belongs here; internal contains production modules and their unit tests. providerconfig holds test metadata parsing, testsupport provides shared isolated PostgreSQL fixtures.

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

[internal/logx](../../internal/logx/readme.md).

## Verification and maintenance

| Test file | Scenario entries |
| --- | --- |
| [acceptance_limits_test.go](acceptance_limits_test.go) | `TestAcceptanceLimitsAndAccountingContract` |
| [acceptance_surfaces_test.go](acceptance_surfaces_test.go) | `TestAcceptanceGuardrailMonitor`, `TestAcceptanceModelLifecycle` |
| [budget_chain_test.go](budget_chain_test.go) | `TestPersonalBudgetChain`, `TestTeamBudgetChain`, `TestProjectBudgetChain`, `TestOrganizationBudgetChain`, `TestKeyBudgetChain`, `TestBudgetNamesTheNarrowestExhaustedScope` |
| [budget_test.go](budget_test.go) | `TestBudgetAccumulatesAcrossCalls`, `TestRateLimitIsSeparateFromBudget`, `TestUnlimitedScopeIsUnlimited` |
| [chain_support_test.go](chain_support_test.go) | Fixture/helpers; no top-level Test |
| [chain_test.go](chain_test.go) | `TestRequestChain` |
| [consistency_test.go](consistency_test.go) | `TestResponseUsageAndLogsAgree`, `TestUsageCountersMatchTheBill`, `TestCacheHitIsFreeAndLoggedAsOne`, `TestFailedCallIsLoggedButNotCharged`, `TestLogDetailMatchesTheRowInTheList` |
| [endpoint_redesign_test.go](endpoint_redesign_test.go) | `TestEndpointAvailableContract`, `TestEndpointRedesignNativeLifecycle`, `TestEndpointDialogueMatrix`, `TestEndpointRetiredGlobals`, `TestEndpointGoogleGuardrails` |
| [endpoints_test.go](endpoints_test.go) | `TestChatEndpointRewritesAndAnswers`, `TestEmbeddingEndpointUsesItsOwnPath`, `TestSeedanceBypassCreatesAndPollsATask`, `TestABypassCannotBeFilledInFromTheConsole` |
| [fal_billing_test.go](fal_billing_test.go) | `TestFalSettlementUsesOutputSecondsAndDeduplicates` |
| [fallback_test.go](fallback_test.go) | `TestFallbackChain`, `TestCooldownSkipsADeployment` |
| [guardrail_test.go](guardrail_test.go) | `TestGuardrailBlocksBeforeTheUpstreamIsDialed`, `TestGuardrailBlockIsLoggedButNotCharged`, `TestGuardrailOnlyAppliesToTheModelsItCovers`, `TestGuardrailTrialDoesNotChangeStorage`, `TestGuardrailRedactRewritesInsteadOfBlocking` |
| [harness_test.go](harness_test.go) | `TestMain` |
| [interaction_test.go](interaction_test.go) | `TestGuardrailBlockDoesNotConsumeBudget`, `TestCacheHitDoesNotConsumeBudget`, `TestManagerActionsAreAudited`, `TestChangingABudgetTakesEffectImmediately`, `TestBlockingAKeyTakesEffectImmediately`, `TestDeletingATeamStopsItsKeys`, `TestModelRestrictionAndBudgetAreIndependent` |
| [internal_modules_test.go](internal_modules_test.go) | `TestInternalLLMRouteBoundary`, `TestInternalLLMProtocolHelpersBoundary`, `TestInternalEstimateBoundary`, `TestInternalCatalogClassificationBoundary`, `TestInternalCacheAndPluginBoundary`, `TestInternalLiveUnavailableBoundary`, `TestInternalProviderProtocolBoundary`, `TestInternalProxyParameterBoundary`, `TestInternalConfigLoadBoundary`, `TestInternalGeminiNativeStreamBoundary` |
| [layout_test.go](layout_test.go) | `TestProductionModulesExcludeRegressionFixtures` |
| [live_mode_test.go](live_mode_test.go) | Fixture/helpers; no top-level Test |
| [live_test.go](live_test.go) | `TestLiveConfiguredVendorsAnswer`, `TestLiveBypassCreatesAndPollsARealTask` |
| [logs_test.go](logs_test.go) | `TestEveryLogViewAgreesOnTheSameTraffic`, `TestCallIdIsTheSameEverywhereItAppears`, `TestTenantSeesOnlyItsOwnLogs`, `TestDailyActivityCountsTheCalls`, `TestPromptStorageFollowsTheSwitch`, `TestSpendCalculateUsesThePriceCatalog` |
| [mode_test.go](mode_test.go) | Fixture/helpers; no top-level Test |
| [model_alias_test.go](model_alias_test.go) | `TestDialogueAliasLifecycle` |
| [model_credential_test.go](model_credential_test.go) | `TestOfficialCredentialSupportsNativeModel` |
| [model_discovery_test.go](model_discovery_test.go) | `TestModelDiscoveryPathFallback`, `TestDiscoveryTreatsProviderNamesUniformly` |
| [model_fallback_test.go](model_fallback_test.go) | `TestModelFallbackLifecycle`, `TestModelFallbackPersistenceBoundaries`, `TestModelFallbackBudgetRecheck`, `TestModelFallbackTemplatePrecedence` |
| [model_limit_test.go](model_limit_test.go) | `TestModelLimitChain` |
| [model_weights_test.go](model_weights_test.go) | `TestModelWeightsLifecycle` |
| [models_test.go](models_test.go) | `TestModelListShowsWhatTheCallerMayUse`, `TestModelsOfEveryEndpointTypeAreListed`, `TestModelAvailableCarriesThePriceFromTheCatalog`, `TestModelAvailableLeavesUnknownPricesEmpty`, `TestEveryDeclaredTransportExistsInTheCatalog`, `TestTransportsAreRegisteredBypasses`, `TestPriceCatalogRatesArePerToken`, `TestAddingAModelMakesItCallable`, `TestDeletingAModelStopsIt`, `TestConfigModelCannotBeDeletedFromTheConsole`, `TestDisabledModelIsRefused`, `TestMissingCredentialStillBlamesTheCredential`, `TestUnknownModelIsRefused` |
| [more_cases_test.go](more_cases_test.go) | `TestIdempotencyReplaysWithoutASecondCharge`, `TestStreamSkipsIdempotency`, `TestMalformedRequestsNeverDial`, `TestTPMLimitIsNotABudgetError`, `TestUpstream429FailsOverAndAMissingKeyIsSkipped`, `TestGuardrailDoesNotCoverEmbeddings`, `TestEachInferenceFamilyReachesItsOwnUpstreamPath` |
| [multi_supplier_test.go](multi_supplier_test.go) | `TestMultiSupplierConfiguredWeights`, `TestMultiSupplierTemplateWeights`, `TestMultiSupplierModelRulesAndScopeIsolation`, `TestMultiSupplierRetryAndBilling`, `TestMultiSupplierZeroWeightExcludesFailover`, `TestMultiSupplierAllZeroRejects`, `TestMultiSupplierResponseAffinity`, `TestMultiSupplierStreamingAndConcurrentWeights`, `TestMultiSupplierDatabaseDuplicateDeployments`, `TestMultiSupplierCooldownIsolation` |
| [permission_test.go](permission_test.go) | `TestTenantIsolationAcrossOrganizations`, `TestMemberCannotReadAnotherTenant`, `TestMemberCannotAdminister`, `TestKeyLifecycleAndScope`, `TestKeyRegenerateInvalidatesTheOldSecret`, `TestUnauthenticatedIsRefused`, `TestInvalidKeyIsRefused`, `TestOrganizationAndTeamHierarchy` |
| [price_selection_test.go](price_selection_test.go) | `TestPriceSelectionContract` |
| [pricing_live_test.go](pricing_live_test.go) | `TestLiveRealModelsAreBilledFromTheCatalog`, `TestLiveVendorUsageFieldsAreUnderstood`, `TestLiveStreamingIsBilledFromTheRealUsage`, `TestLiveSameVendorViaEitherProtocolIsBilledAlike`, `TestLiveUsageOracleHonorsExplicitZeroAndNullAliases`, `TestLiveUsageOracleDistinguishesNestedSubsetFromAnthropicCache`, `TestParseStreamUsageMergesResponseUsageEvents`, `TestSameRatePricesHandlesUnequalRateListLengths`, `TestSameRatePricesRequiresComparableTokenRates` |
| [pricing_test.go](pricing_test.go) | `TestAWindowPricedModelIsChargedTheWindowItLandedIn`, `TestAPerSecondModelIsNotRecordedAsFree`, `TestLogDetailExplainsTheChargeWithoutRecomputing`, `TestTheBreakdownSurvivesAPriceChange`, `TestAnUnpricedCallIsNotRecordedAsFree`, `TestACachedCallIsNotBilledAtTheInputRate` |
| [pricing_units_test.go](pricing_units_test.go) | `TestConsolePriceFormIsBilledForEveryDimension`, `TestTheFlatPeakFieldIsActuallyRead`, `TestAnthropicShapedUsageIsBilledWhole`, `TestTheCatalogPricesEveryMeasureItQuotes`, `TestEveryCatalogModelIsReachableByTheNamesDeploymentsUse` |
| [protocol_test.go](protocol_test.go) | `TestSameModelAnswersThreeChatProtocols`, `TestEveryProtocolIsBilledTheSameWay`, `TestStreamingAnswersArriveAndAreBilled`, `TestStreamingAndNonStreamingAgreeOnTokens`, `TestEmbeddingIsBilledFromItsOwnUsage`, `TestUnknownPathIsNotFound` |
| [provider_forms_test.go](provider_forms_test.go) | `TestProviderFormsContract`, `TestPriceReloadRequiresConfiguredSource`, `TestPriceLocalListingRoutes` |
| [provider_transport_test.go](provider_transport_test.go) | `TestProviderTransportContract` |
| [removed_surfaces_test.go](removed_surfaces_test.go) | `TestRemovedSurfacesStayGone` |
| [reset_test.go](reset_test.go) | `TestResetSpendChain`, `TestPasswordResetChain` |
| [responses_continuation_test.go](responses_continuation_test.go) | `TestResponsesIncrementalContinuation` |
| [responses_native_continuation_test.go](responses_native_continuation_test.go) | `TestResponsesNativeToolContinuation`, `TestResponsesFailedStreamCannotContinue` |
| [route_template_config_test.go](route_template_config_test.go) | `TestRouteTemplateConfigurationRoundTrip`, `TestRouteTemplateModelRulesContainNoDeploymentData`, `TestRouteTemplateDuplicateModelsAreRejected`, `TestRouteTemplateDefaultStrategyContract` |
| [route_template_live_test.go](route_template_live_test.go) | `TestLiveRouteTemplateSelectionAndBilling` |
| [route_template_test.go](route_template_test.go) | `TestRouteTemplateScopePrecedence`, `TestRouteTemplateRetryPolicyApplies`, `TestRouteTemplateRejectsLegacyDocuments` |
| [router_settings_test.go](router_settings_test.go) | `TestRouterSettingsChain` |
| [routing_groups_test.go](routing_groups_test.go) | `TestRoutingGroupLifecycle` |
| [routing_test.go](routing_test.go) | `TestRoutingStrategyChain`, `TestLeastBusyChain`, `TestUnknownStrategyChain`, `TestExactNameBeatsWildcard`, `TestSessionPinOverridesStrategy` |
| [seedance_billing_test.go](seedance_billing_test.go) | `TestSeedanceSettlementUsesMeasuredBandAndDeduplicates` |
| [setup_test.go](setup_test.go) | `TestStartupDoesNotInstallRelayCapabilities`, `TestScopedKeyCallsInference`, `TestModelAllowListIsEnforced` |
| [split_test.go](split_test.go) | `TestWeightedSplitSendsTrafficToBothDeployments`, `TestWeightedSplitDoesNotChangeSimpleShuffle`, `TestSplitStillBillsAndLogsEveryCall`, `TestSplitIsEvenWhenNoWeightsAreSet` |
| [supplier_catalog_test.go](supplier_catalog_test.go) | `TestSupplierCatalogLifecycle` |
| [template_routing_test.go](template_routing_test.go) | `TestTemplateRoutingUnified` |
| [weighted_live_metadata_test.go](weighted_live_metadata_test.go) | `TestDecodeProviderMetadataRejectsInvalidInput`, `TestExpectedWeightedCountsAvoidsIntegerOverflow`, `TestWeightedObserverRejectsSuccessfulFallback`, `TestHealthyWeightedCycleRequiresEveryRequest` |
| [weighted_live_test.go](weighted_live_test.go) | `TestLiveConfiguredWeightedRouting` |
| [xgo_guardrail_test.go](xgo_guardrail_test.go) | `TestXGoGuardrailEnforcesPersistedPolicy` |

```bash
go test ./cmd/regression/... -count=1
```

Use XHUB_REGRESSION_STRICT=1 for database acceptance and inspect skips. Redis and live providers require separate configuration. Give every simultaneously running regression process a dedicated Redis instance: spend queues and cooldown keys are shared external state and are not safe to share across runners. Harness cleanup waits for HTTP handlers, drains final spend records while its private schema still exists, and then closes the Redis client. Update this reference and feature documentation after contract changes.
