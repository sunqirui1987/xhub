# Built-in provider assembly

[简体中文](readme_cn.md) · [Feature implementation reference](../../../docs/development/implementation.md)

## Responsibilities and behavior

Add new provider packages here and confirm startup imports when extending built-ins. Tests live in provider registration and supplier packages. Import success does not verify real provider authentication or inference.

## Source responsibilities and entry points

### all.go

Internal implementation and protocol boundaries:  [all.go](all.go)。

## External HTTP boundary

This directory registers no direct HTTP route. Higher layers call its Go API; trace catalog dispatch or host calls through the dependency chain.

## Dependencies


## Verification and maintenance

There are no direct test files here. Integration tests prove executed paths rather than every internal failure branch.

```bash
go test ./internal/provider/all -count=1
```

Use XHUB_REGRESSION_STRICT=1 for database acceptance and inspect skips. Redis and live providers require separate configuration. Update this reference and feature documentation after contract changes.
