# Regression fixtures

This directory contains input data for tests in the parent `internal/regression` package. It has no production API or HTTP routes.

| File | Purpose |
| --- | --- |
| [config_provider.yaml](config_provider.yaml) | Shared provider and deployment structure for deterministic multi-supplier routing tests. The test harness replaces upstream bases with independent loopback servers and injects synthetic named credentials. |

The fixture is parsed by the same configuration loader used by the gateway. It must contain no real API keys or production endpoints. Routing assertions cover the resulting deployment identities, template weights, retries, and accounting rather than merely checking that YAML parses. Run the parent package with `go test ./internal/regression -count=1 -timeout=600s`; the complete mode and prerequisite matrix is in [the regression plan](../../../docs/development/regression.md).
