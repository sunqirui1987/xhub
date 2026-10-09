# Request guardrails

[简体中文](readme_cn.md) · [Feature implementation reference](../../../docs/development/implementation.md)

## Responsibilities and behavior

guard.go selects default and explicitly requested pre-call rules, orders them by priority, and stops on a block. rules.go implements local keyword/RE2 checks; custom.go compiles XGo scripts and executes them with iXGo. Module exposes two application routes; PreCall runs before supported chat execution. The Host supplies identity and settings access.
Blocking occurs before upstream dialing. Redaction changes the actual outgoing body. Configuration read failure denies execution rather than silently disabling protection. Direct guardrail endpoints still require authenticated, authorized access.
Supported checks include keyword/RE2 rules, XGo scripts, OpenAI/Lakera/Azure/Bedrock/Presidio, remote LiteLLM and local secret detection. The virtual xhub/guardrail package provides HTTP, JSON, LLMChat and Allow/Block/Modify/Flag; test requests and live requests share the executor. See the [configuration and XGo guide](../../../docs/development/guardrails.md) for CRUD, YAML, API contracts and execution limits. This is not a universal classifier or media moderation service. Official bypass does not use ordinary chat guardrails. Tests must observe upstream call counts, transformed content, failure accounting, and log redaction.

## Source responsibilities and entry points

### codec.go

Internal implementation and protocol boundaries:  [codec.go](codec.go)。

### guard.go

- [`func Apply(s Host, w http.ResponseWriter, r *http.Request)`](guard.go) — Apply is the management trial for a guardrail. It does not change storage.
- [`func PreCall(s Host, body map[string]any) (bool, string)`](guard.go) — PreCall runs guardrails before chat is sent. A block returns true and a reason, and the data plane does not call the upstream.
- [`func Evaluate(s Host, body map[string]any) (blocked bool, message string, findings []map[string]any)`](guard.go) — Evaluate runs the union of default-on and explicitly selected pre-call rules in priority order and reports each one. The logs drawer reads these rows as guardrail monitoring. A block stops the scan: later rules did not run, so they are not reported as if they had.

### host.go

Exported types: `Host`.

Internal implementation and protocol boundaries:  [host.go](host.go)。

### mount.go

- [`func Module(h Host) httpx.Module`](mount.go) — Module is the guardrail trial API. The data plane calls PreCall before the upstream and does not go through these two paths.

## External HTTP boundary

The registration files below mount these routes. Aliases share handlers. Registration does not replace authorization or business assertions; see module contracts and the API reference.

| Method / path | Registration |
| --- | --- |
| `POST /apply_guardrail` | [mount.go](mount.go) |
| `POST /guardrails/apply_guardrail` | [mount.go](mount.go) |

## Dependencies

[internal/auth](../../auth/readme.md), [internal/httpx](../../httpx/readme.md), [internal/logx](../../logx/readme.md), [internal/store](../../store/readme.md).

## Verification and maintenance

| Test file | Scenario entry points |
| --- | --- |
| [guard_regression_test.go](guard_regression_test.go) | `TestGuardrailLookupFailureBlocks`, `TestGuardrailRedactsEveryTextLeafWithoutChangingImageURL` |
| [guard_test.go](guard_test.go) | `TestGuardrailTextScansAllProtocolFields`, `TestGuardrailTextJoinsMessageStringsAndSkipsOtherShapes`, `TestExtraWordsAcceptsTheShapesASavedRuleUses`, `TestBoolOfReadsTheFormsJSONProduces`, `TestMatchGuardrailBlocksRedactsAndAllows`, `TestEvaluateReportsEachDefaultRule`, `TestPreCallOnlyStopsOnADefaultPreCallBlock`, `TestPreCallNamesAnUnnamedRule`, `TestEvalNamedRunsTheAskedRuleAndOtherwiseTheDefault`, `TestApplyTrialsOneRuleWithoutWritingIt` |

```bash
go test ./internal/gateway/guard -count=1
```

Use XHUB_REGRESSION_STRICT=1 for database acceptance and inspect skips. Redis and live providers require separate configuration. Update this reference and feature documentation after contract changes.

## Adapters and primitives

external.go implements service protocols; primitives.go provides bounded HTTP/JSON and invocation-specific bindings; secrets.go detects local secret patterns; capabilities.go masks management credentials. Local scripts have 100ms; network scripts and external rules have a 10-second budget. See the [guide](../../../docs/development/guardrails.md) for contracts and limits.
