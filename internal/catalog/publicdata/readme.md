# Embedded public catalog resources

[简体中文](readme_cn.md) · [Feature implementation reference](../../../docs/development/implementation.md)

## Responsibilities and behavior

pricedata.json supplies the embedded baseline, cn_holidays.json supplies the pricing calendar, and autorouter_presets.json is a retained resource rather than evidence that retired automatic routing is active. The parent catalog package embeds these files.
There are no Go or HTTP entry points in this directory. Parsing, validation, overrides, and presentation are implemented by catalog and gateway/models. Changes require valid JSON, stable model identifiers, correct units, and explicit-zero handling.
Embedded files are part of the build. Editing them does not update an existing binary; runtime feed reloads and overrides are separate mechanisms. Test calendar boundaries, rate selection, and historical snapshot isolation when changing resources.

## Source responsibilities and entry points

This directory contains resources, subpackages, or tests without independent production Go implementation.

Resources and persistence definitions: [autorouter_presets.json](autorouter_presets.json), [cn_holidays.json](cn_holidays.json), [pricedata.json](pricedata.json).

## External HTTP boundary

This directory registers no direct HTTP route. Higher layers call its Go API; trace catalog dispatch or host calls through the dependency chain.

## Verification and maintenance

There are no direct test files here. Integration tests prove executed paths rather than every internal failure branch.

```bash
go test ./internal/catalog -count=1
```

Use XHUB_REGRESSION_STRICT=1 for database acceptance and inspect skips. Redis and live providers require separate configuration. Update this reference and feature documentation after contract changes.
