# Deployment filtering, ordering, and weighted split

[简体中文](readme_cn.md) · [Feature implementation reference](../../docs/development/implementation.md)

## Responsibilities and behavior

router.go filters and orders deployments for a public name; adapter.go reads runtime state; split.go performs smooth weighted scheduling. Disabled entries are excluded. Runtime lookup prefers extended deployment identity with legacy fallback.
DeploymentID is api_base|model for weight overrides. CooldownID also includes pricing/deployment IDs and named credentials without raw secrets. ApplyWeights copies parameters. simple-shuffle favors maximum weight; weighted-split performs proportional scheduling with equal defaults and explicit-zero exclusion.
Routing never grants model access. Ordinary strategies may try an entirely cooled pool; split returns no pool without eligible positive-weight entries. Cost ordering compares current input-token prices, not total predicted request cost. Affinity only reorders eligible candidates.

## Source responsibilities and entry points

### adapter.go

- [`func EncodeRequest(op, provider string, body map[string]any, realModel string) ([]byte, error)`](adapter.go) — EncodeRequest turns the public JSON body into the upstream request body. The protocol details live in internal/llm. This wrapper keeps the old name so callers do not each import that package.
- [`func DecodeResponse(op, provider, alias string, raw []byte) []byte`](adapter.go) — DecodeResponse turns an upstream response into the public shape and puts the caller's model alias back into the model field.

### router.go

Exported types: `State`.

- [`func All(list []config.ModelEntry, alias string) []config.ModelEntry`](router.go) — All returns every deployment under one public model name, before a strategy orders them.
- [`func Order(list []config.ModelEntry, alias, strategy string, st State) []config.ModelEntry`](router.go) — Order sorts usable deployments into attempt order for a strategy. A cooling deployment is not placed first when another deployment exists.
- [`func Pick(list []config.ModelEntry, alias, strategy string, st State) *config.ModelEntry`](router.go) — Pick returns the first deployment from Order. It returns nil when no deployment is usable.
- [`func DeploymentID(e config.ModelEntry) string`](router.go) — DeploymentID is the physical deployment identity, shaped as api_base|model. Weight overrides use this stable frontend-facing id.
- [`func CooldownID(e config.ModelEntry) string`](router.go) — CooldownID is the runtime deployment identity. It isolates cooldown, busy, latency, usage, session pinning, and billing state for named credentials that share one physical endpoint. A configured pricing_id or deployment_id is included when present so rows with the same endpoint, model, and credential name remain distinct. No API key is included.
- [`func IsSplitStrategy(strategy string) bool`](router.go) — IsSplitStrategy reports whether strategy divides traffic by weight. Hyphens and underscores are the same name. Anything else, including simple-shuffle, is not a split: simple-shuffle still picks the heaviest deployment, and treating it as a split would change that.
- [`func ApplyWeights(list []config.ModelEntry, overrides map[string]float64) []config.ModelEntry`](router.go) — ApplyWeights copies list and sets weight on the deployments named in overrides. The key is DeploymentID (api_base|model). A deployment that is not in the map keeps the weight already on it, which defaults to 1 inside the split. An empty map returns the same slice, so a document that does not configure shares does not allocate or change the pool. The copy matters: ModelList is the process config, and writing weight onto it would leak one request's template into the next request.
- [`func AdapterURL(provider, apiBase, realModel string) string`](router.go) — AdapterURL is the upstream address for chat completions. Other operations use AdapterURLOp.
- [`func AdapterURLOp(op, provider, apiBase, realModel string) string`](router.go) — AdapterURLOp returns the full URL for an operation and a provider. The rules live in internal/llm.Endpoint.
- [`func ValidateStrategy(strategy string) error`](router.go) — ValidateStrategy accepts the strategy names from the catalog and the hyphenated spellings the gateway config already uses. An unknown name returns an error and is not treated as simple-shuffle.

### split.go

Exported types: `SplitState`.

- [`func NewSplitState() *SplitState`](split.go) — NewSplitState returns an empty split state. One instance is shared by a process. The counters are per-process, so several replicas each converge on the configured ratio independently rather than coordinating one global schedule; the aggregate ratio is right either way.
- [`func SharedSplit() *SplitState`](split.go) — SharedSplit returns the process-wide split cursors. The router State is rebuilt for every request, so the cursors cannot live on it - a split that forgot where it was would send every request to the same deployment. One shared instance is what makes the ratio hold across requests.
- [`func (s *SplitState) PickWeighted(ids []string, weights []float64, available []bool) int`](split.go) — PickWeighted chooses the deployment that is furthest behind its share. This is smooth weighted round-robin: each turn every candidate's score grows by its weight, the highest score wins, and the winner's score drops by the total. Over any ten draws a 3:7 split lands exactly 3 and 7 rather than merely averaging that over a long run, which matters because a caller that watches ten consecutive requests should see the ratio they configured. A candidate that is not available has its score forgotten, so it re-enters at zero rather than immediately claiming the share it accrued while it was cooling down. That would otherwise send the first requests after a recovery all to the deployment that just came back. available（[]bool）：每条候选此刻是否可以接流量。
- [`func (s *SplitState) Forget(id string)`](split.go) — Forget drops a deployment's cursor. A deployment that is removed or renamed would otherwise leave its score behind forever, which is a slow leak in a process that runs for weeks.

## External HTTP boundary

This directory registers no direct HTTP route. Higher layers call its Go API; trace catalog dispatch or host calls through the dependency chain.

## Dependencies

[internal/catalog](../catalog/readme.md), [internal/config](../config/readme.md), [internal/llm](../llm/readme.md), [internal/logx](../logx/readme.md).

## Verification and maintenance

| Test file | Scenario entry points |
| --- | --- |
| [cost_regression_test.go](cost_regression_test.go) | `TestCostRoutingUsesSettlementRatePrecedenceAndWindow`, `TestCostRoutingAcceptsValidNumericRates` |
| [disabled_test.go](disabled_test.go) | `TestAllExcludesDisabledExactAndWildcardDeployments` |
| [split_test.go](split_test.go) | `TestWeightedSplitFollowsTheConfiguredRatio`, `TestWeightedSplitDoesNotRequireHundred`, `TestSplitWithoutWeightsIsEven`, `TestSplitSkipsACoolingDeployment`, `TestSplitIsEvenAfterACoolingDeploymentReturns`, `TestSplitWithOneDeploymentDoesNotDisturbIt`, `TestSplitWithoutStateFallsBackToHighestWeight`, `TestApplyWeightsUsesTheDocumentWithoutTouchingThePool`, `TestWeightedSplitIsItsOwnStrategy`, `TestSplitKeepsRatioAcrossManyDraws`, `TestCostStrategyDoesNotLetAnUnpricedDeploymentWin` |
| [template_regression_test.go](template_regression_test.go) | `TestSplitExclusionsApplyToEveryAttempt`, `TestNamedCredentialCooldownAndRetryIsolation`, `TestCooldownIDUsesConfiguredStablePricingIdentity`, `TestRuntimeMetricsUseCredentialAwareIDs`, `TestRuntimeMetricsAcceptLegacyPhysicalID` |

```bash
go test ./internal/router -count=1
```

Use XHUB_REGRESSION_STRICT=1 for database acceptance and inspect skips. Redis and live providers require separate configuration. Update this reference and feature documentation after contract changes.
