# Shared template-selection authorization

[简体中文](readme_cn.md) · [Feature implementation reference](../../../docs/development/implementation.md)

## Responsibilities and behavior

selection.go is shared by ordinary organization/team/key saves and dedicated template binding. It has no HTTP module. Selection trims the ID; an empty value means inherit and requires no template read.
For a nonempty ID, it loads the IAM row and authorizes ActionRouteTemplateRead using trusted organization/team ownership. Missing rows return ErrNotFound; storage failures become InternalError. An invisible template cannot be assigned.
This checks template visibility, while the caller separately checks permission to write the target scope. Both checks precede mutation. Runtime precedence and whole-document resolution belong to prefs and the gateway routing lookup.

## Source responsibilities and entry points

### selection.go

Exported types: `Host`.

- [`func Selection(h Host, r *http.Request, p *auth.Principal, id string) error`](selection.go) — Selection checks the selected template before any scope is mutated. Empty means inherit and needs no template read. Ownership must be loaded explicitly: route templates are not resolved by Authorize from their ID alone.

## External HTTP boundary

This directory registers no direct HTTP route. Higher layers call its Go API; trace catalog dispatch or host calls through the dependency chain.

## Dependencies

[internal/auth](../../auth/readme.md), [internal/authz](../../authz/readme.md), [internal/iam](../../iam/readme.md), [internal/logx](../../logx/readme.md).

## Verification and maintenance

[selection_test.go](selection_test.go) covers empty inheritance, trusted platform/organization/team ownership, authorization denial, missing templates and fail-closed storage errors. Ownership checks use real PostgreSQL in an isolated schema deleted at cleanup; they are service-boundary tests. Empty-input and canceled-context cases are deterministic unit checks. HTTP binding and browser flows are listed in [the verification matrix](../../../docs/development/product-fixes-20261010.md).

```bash
go test ./internal/gateway/templateauth -count=1
```

Use XHUB_REGRESSION_STRICT=1 for database acceptance and inspect skips. Redis and live providers require separate configuration. Update this reference and feature documentation after contract changes.
