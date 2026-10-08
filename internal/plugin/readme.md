# Upstream extension registry

[简体中文](readme_cn.md) · [Feature implementation reference](../../docs/development/implementation.md)

## Responsibilities and behavior

registry.go defines Extension, Call, and Decision. Extensions name themselves and implement BeforeUpstream; Call carries operation/model/path, and Decision can refuse with status, code, message, and headers. Register rejects nil, empty, and duplicate names; Names returns a copy.
Run invokes extensions in order and stops on the first refusal while preserving applicable accumulated headers. Invoke looks up an exact name. Extensions execute before upstream sending without replacing identity or budget checks.
There is no dynamic script loader or HTTP installation endpoint. Extensions should avoid irreversible side effects and credential disclosure. Tests cover registration, ordering, header behavior, lookup failures, and first-refusal semantics.

## Source responsibilities and entry points

### registry.go

Exported types: `Call`, `Decision`, `Extension`, `Registry`.

- [`func New() *Registry`](registry.go) — New returns an empty registry. Run allows the call when nothing is registered.
- [`func (r *Registry) Register(ext Extension) error`](registry.go) — Register appends an extension. An empty or duplicate name returns an error and leaves the existing order unchanged.
- [`func (r *Registry) Names() []string`](registry.go) — Names returns a copy of the registered names in order. Changing the slice does not change the registry.
- [`func (r *Registry) Invoke(name string, call Call) (Decision, error)`](registry.go) — Invoke runs the extension registered under name. A missing name returns an error and does not call any other extension.
- [`func (r *Registry) Run(call Call) Decision`](registry.go) — Run calls every extension in registration order. The first refusal stops the rest and keeps headers already set. An empty registry returns a zero Decision so the data plane continues.

## External HTTP boundary

This directory registers no direct HTTP route. Higher layers call its Go API; trace catalog dispatch or host calls through the dependency chain.

## Dependencies

[internal/logx](../logx/readme.md).

## Verification and maintenance

| Test file | Scenario entry points |
| --- | --- |
| [registry_test.go](registry_test.go) | `TestRegistryZeroValueRegistersWithStableName`, `TestRegistryRunMergesHeadersAndStopsAtRefusal` |

```bash
go test ./internal/plugin -count=1
```

Use XHUB_REGRESSION_STRICT=1 for database acceptance and inspect skips. Redis and live providers require separate configuration. Update this reference and feature documentation after contract changes.
