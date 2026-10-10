# Runtime logging and secret redaction

[简体中文](readme_cn.md) · [Feature implementation reference](../../docs/development/implementation.md)

## Responsibilities and behavior

log.go wraps the standard logger with fmt-style Trace, Debug, Info, and Error calls. Redaction filters Bearer values, sk-style tokens, and URL credentials before output.
Runtime diagnostics differ from durable IAM audit and spend logs. Do not pass complete environment values, DSNs, prompts, or provider secrets into routine logs; pattern redaction cannot recognize every possible secret format.
There are no HTTP routes. files_test checks logging conventions. New diagnostics should identify call and failure stage while retaining secrecy; operations requiring audit use the business audit path.

## Source responsibilities and entry points

### log.go

- [`func Trace(format string, args ...any)`](log.go) — Trace records a step inside a call, such as entering the data plane.
- [`func Debug(format string, args ...any)`](log.go) — Debug records a decision that is useful when a call did not do what the operator expected, such as skipping a deployment.
- [`func Info(format string, args ...any)`](log.go) — Info records a normal outcome, such as the process listening or one HTTP request finishing.
- [`func Error(format string, args ...any)`](log.go) — Error records a failure. The process keeps running unless the caller exits after this returns.

## External HTTP boundary

This directory registers no direct HTTP route. Higher layers call its Go API; trace catalog dispatch or host calls through the dependency chain.

## Verification and maintenance

| Test file | Scenario entry points |
| --- | --- |
| [files_test.go](files_test.go) | `TestLoggerDefinesLevels`, `TestRedactHidesBearerAndKey`, `TestRedactHidesCaseInsensitiveKeysAndURLCredentials` |

```bash
go test ./internal/logx -count=1
```

Use XHUB_REGRESSION_STRICT=1 for database acceptance and inspect skips. Redis and live providers require separate configuration. Update this reference and feature documentation after contract changes.
