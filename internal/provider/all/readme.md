# Built-in provider assembly

[简体中文](readme_cn.md) · [Feature implementation reference](../../../docs/development/implementation.md)

## Responsibilities and behavior

all.go blank-imports openai, qiniu, and volcengine so required init registration runs from one startup import. This avoids entry points accidentally omitting built-in transports or models.
The package exports no independent functions or HTTP routes. OpenAI is currently a placeholder without registration init; its adapted protocol lives in llm. Qiniu and Volcengine contribute task transports and model catalog entries.
Add new provider packages here and confirm startup imports when extending built-ins. Tests live in provider registration and supplier packages. Import success does not verify real provider authentication or inference.

## Source responsibilities and entry points

### all.go

Internal implementation and protocol boundaries:  [all.go](all.go)。

## External HTTP boundary

This directory registers no direct HTTP route. Higher layers call its Go API; trace catalog dispatch or host calls through the dependency chain.

## Dependencies

[internal/logx](../../logx/readme.md), [internal/provider/openai](../openai/readme.md), [internal/provider/qiniu](../qiniu/readme.md), [internal/provider/volcengine](../volcengine/readme.md).

## Verification and maintenance

There are no direct test files here. Integration tests prove executed paths rather than every internal failure branch.

```bash
go test ./internal/provider/all -count=1
```

Use XHUB_REGRESSION_STRICT=1 for database acceptance and inspect skips. Redis and live providers require separate configuration. Update this reference and feature documentation after contract changes.
