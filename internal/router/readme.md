# Deployment filtering, ordering, and weighted split

[简体中文](readme_cn.md) · [Feature implementation reference](../../docs/development/implementation.md) · [Routing contract](../../docs/development/routing.md)

## Responsibilities and behavior

The router matches a public model or routing group, removes disabled and cooled deployments, and orders the remaining candidates. It does not grant model access. `simple-shuffle` and `random` select uniformly and ignore weights. `traffic-split` performs an independent random draw from relative positive weights; it is not smooth weighted round-robin and finite samples do not promise an exact ratio.

Explicit allocations are keyed by `deployment_id`; a deployment absent from the allocation list has weight `1`. An empty allocation list gives every matching deployment weight `1`. Weights must be finite and non-negative, and at least one candidate must remain positive. Zero, cooled, disabled, and invalid-weight candidates are excluded from `traffic-split`. Other strategies exclude cooled candidates; if none remain, the result is empty. A pinned deployment can move to the front only when it is still compatible, open, and eligible.

`cost-based-routing` compares the current input-token price, with unpriced candidates ordered last. Runtime identities use `pricing_id`, then `deployment_id`, then `model_info.id` for cooldown and metrics isolation. Public deployment identity uses `litellm_params.deployment_id`, then `model_info.id`; `pricing_id` is not a deployment ID.

## Source responsibilities and entry points

- [`router.go`](router.go) matches candidates, validates strategies, filters disabled deployments, and orders or picks them.
- [`schedule.go`](schedule.go) combines matching, health and weight filtering, and session pinning for request attempts.
- [`policy.go`](policy.go) parses and validates allocation policies.
- [`settings.go`](settings.go) reads the template contract and retry policy defaults.
- [`compile.go`](compile.go) combines model rules with platform defaults and fallback settings.
- [`groups.go`](groups.go) validates and expands routing groups.
- [`fallback.go`](fallback.go) validates fallback graphs.
- [`template.go`](template.go) validates template references and fallback configuration.
- [`weight_cleanup.go`](weight_cleanup.go) removes stale allocation rows when deployment catalogs change.

The package exports the scheduling API used by gateway and dataplane callers. It registers no HTTP route directly.

## Template contract

New documents use `model_routes` and a complete `retry_policy`. A model rule contains `model`, `strategy`, and optional `allocations`; allocations are accepted only for `traffic-split` and contain `deployment_id` plus `weight`. `max_attempts`, `timeout_seconds`, `failure_threshold`, and `cooldown_seconds` are read from `retry_policy`. Routing groups and general, context-window, and content-policy fallbacks are validated and executed by the request path.

## Verification and maintenance

| Test file | Scenario entry points |
| --- | --- |
| [`allocation_test.go`](allocation_test.go) | `TestRandomAndTrafficIntervals`, `TestStickySchedulingDoesNotDraw`, `TestAllocationPolicyValidation`, `TestRelativeDefaultWeights`, `TestSimpleShuffleIsUniformRandom` |
| [`compile_test.go`](compile_test.go) | `TestCompileOverrides`, `TestCompileFailures`, `TestCompileGroupIsolation`, `TestCompileModelWeights` |
| [`compile_fallback_test.go`](compile_fallback_test.go) | `TestCompileFallbackPrecedence`, `TestCompileFallbackFailures` |
| [`groups_test.go`](groups_test.go) | `TestGroupContract`, `TestGroupScheduleIdentity` |
| [`fallback_test.go`](fallback_test.go) | `TestFallbackPolicy`, `TestFallbackGraph` |
| [`fallback_boundary_test.go`](fallback_boundary_test.go) | `TestFallbackPolicyCategoryBoundaries`, `TestFallbackGraphSharedAndDisconnected` |
| [`cost_regression_test.go`](cost_regression_test.go) | `TestCostRoutingUsesSettlementRatePrecedenceAndWindow`, `TestCostRoutingAcceptsValidNumericRates` |
| [`disabled_test.go`](disabled_test.go) | `TestAllExcludesDisabledExactAndWildcardDeployments` |
| [`weight_cleanup_test.go`](weight_cleanup_test.go) | `TestCleanAllocations`, `TestCleanTemplateWeights` |

Run the focused package tests when implementation changes require them. Database, Redis, and live-provider coverage needs the corresponding configured environment; it is not implied by this package-level suite.

## Dependencies

[internal/catalog](../catalog/readme.md), [internal/config](../config/readme.md), [internal/llm](../llm/readme.md), [internal/logx](../logx/readme.md).
