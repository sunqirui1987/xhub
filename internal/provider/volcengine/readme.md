# Volcengine Ark video-task transport

[简体中文](readme_cn.md) · [Feature implementation reference](../../../docs/development/implementation.md)

## Responsibilities and behavior

seedance.go registers ark_contents_generation with ark.cn-beijing.volces.com. The upstream /api/v3/contents/generations/tasks path supports create, list, and get-by-ID; transport mappings define public gateway paths.
model and id are the request/model and task ID fields, and the volcengine prefix is stripped. One contributed model has input/output token rates of USD 7 per million; the fast entry lacks complete pricing, so zero fallback is not a free-service promise.
Dataplane owns identity, budgets, retry rules, and settlement deduplication. Changes need registration and official-task integration tests, with terminal live usage required for real billing evidence.

## Source responsibilities and entry points

### seedance.go

Internal implementation and protocol boundaries:  [seedance.go](seedance.go)。

## External HTTP boundary

This directory registers no direct HTTP route. Higher layers call its Go API; trace catalog dispatch or host calls through the dependency chain.

## Dependencies

[internal/logx](../../logx/readme.md), [internal/provider](../readme.md).

## Verification and maintenance

| Test file | Scenario entry points |
| --- | --- |
| [seedance_test.go](seedance_test.go) | `TestSeedanceModelsAndPaths` |

```bash
go test ./internal/provider/volcengine -count=1
```

Use XHUB_REGRESSION_STRICT=1 for database acceptance and inspect skips. Redis and live providers require separate configuration. Update this reference and feature documentation after contract changes.
