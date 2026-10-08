# OpenAI provider placeholder

[简体中文](readme_cn.md) · [Feature implementation reference](../../../docs/development/implementation.md)

## Responsibilities and behavior

chat.go retains the OpenAI supplier package boundary without registering a set of endpoint types through init. Adapted requests, usage, and streams live in llm and dataplane; capabilities come from provider.
There are no independent management APIs here. Historical claims about seven registered types are not the current contract. chat_test checks the present boundary. Future registration must distinguish capabilities from transports and be assembled through provider/all.
Trace chat behavior through gateway ingress, dataplane Serve, and llm Build/Endpoint rather than treating this placeholder as a full HTTP client.

## Source responsibilities and entry points

### chat.go

Internal implementation and protocol boundaries:  [chat.go](chat.go)。

## External HTTP boundary

This directory registers no direct HTTP route. Higher layers call its Go API; trace catalog dispatch or host calls through the dependency chain.

## Dependencies

[internal/logx](../../logx/readme.md).

## Verification and maintenance

| Test file | Scenario entry points |
| --- | --- |
| [chat_test.go](chat_test.go) | `TestAdaptedTypesAreNotRegistered`, `TestCapabilityTableDescribesTheAdaptedEntrypoints`, `TestRegisterTransportRejectsAnEntryWithNoActions` |

```bash
go test ./internal/provider/openai -count=1
```

Use XHUB_REGRESSION_STRICT=1 for database acceptance and inspect skips. Redis and live providers require separate configuration. Update this reference and feature documentation after contract changes.
