# In-flight request accounting

[简体中文](readme_cn.md) · [Feature implementation reference](../../docs/development/implementation.md)

## Responsibilities and behavior

Engine tracks process-local in-flight calls for routing hints such as least-busy. New creates the engine; Begin increments a key's count and returns a release function protected by sync.Once. Empty identifiers are no-ops.
The gateway must defer release across normal completion, timeout, send errors, and stream interruption. These counters are routing hints, not authorization, rate enforcement, or accounting records.
There are no HTTP or storage interfaces here. Counts are not automatically aggregated across instances. Tests cover concurrent starts, repeated release, empty identifiers, and return to zero.

## Source responsibilities and entry points

### hooks.go

Exported types: `Engine`.

- [`func New() *Engine`](hooks.go) — New returns an empty gate. In-flight calls are counted by key id.
- [`func (e *Engine) Begin(keyID string) func()`](hooks.go) — Begin reserves one in-flight slot and returns the release function. An empty key id is not counted, so a caller without a key still gets a release.

## External HTTP boundary

This directory registers no direct HTTP route. Higher layers call its Go API; trace catalog dispatch or host calls through the dependency chain.

## Dependencies

[internal/logx](../logx/readme.md).

## Verification and maintenance

| Test file | Scenario entry points |
| --- | --- |
| [hooks_test.go](hooks_test.go) | `TestReleaseIsIdempotentWithOtherCallsInFlight` |

```bash
go test ./internal/hooks -count=1
```

Use XHUB_REGRESSION_STRICT=1 for database acceptance and inspect skips. Redis and live providers require separate configuration. Update this reference and feature documentation after contract changes.
