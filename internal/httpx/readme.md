# HTTP errors and module contracts

[简体中文](readme_cn.md) · [Feature implementation reference](../../docs/development/implementation.md)

## Responsibilities and behavior

httpx.go provides JSON/error helpers and call IDs; module.go defines Registrar, Module, and Bind. Registrar.Handle is the registration seam used by independently mounted modules.
CallID creates a 32-character hexadecimal identifier and SetCallID exposes it for response/log/accounting correlation. WriteTypedError chooses a protocol-specific error envelope; 429 responses carry Retry-After: 1.
This package expresses decisions as HTTP and does not authenticate or authorize callers. Once headers or stream bytes are sent, handlers cannot replace the status; later failures need explicit log evidence.

## Source responsibilities and entry points

### httpx.go

- [`func CallID() string`](httpx.go) — CallID returns a new 32-character hexadecimal call identifier.
- [`func SetCallID(w http.ResponseWriter, id string)`](httpx.go) — SetCallID writes x-litellm-call-id. If a value is already set, the caller decides whether to overwrite it.
- [`func WriteError(w http.ResponseWriter, status int, typ, msg string)`](httpx.go) — WriteError writes the gateway JSON error. Inference paths should call WriteTypedError so the provider envelope is used instead.
- [`func WriteTypedError(w http.ResponseWriter, path string, status int, typ, msg string)`](httpx.go) — WriteTypedError picks the error envelope from the request path. Status 429 also sets Retry-After to 1. A missing call ID is filled in.
- [`func WriteJSON(w http.ResponseWriter, status int, v any)`](httpx.go) — WriteJSON sets the status code and writes JSON. A later encoding failure does not change the status that was already sent.

### module.go

Exported types: `Registrar`, `Module`.

- [`func Bind(name string, mount func(Registrar)) Module`](module.go) — Bind builds a module from a name and a mount function. An empty name stays empty, and the process refuses to register it.

## External HTTP boundary

This directory registers no direct HTTP route. Higher layers call its Go API; trace catalog dispatch or host calls through the dependency chain.

## Dependencies

[internal/logx](../logx/readme.md).

## Verification and maintenance

| Test file | Scenario entry points |
| --- | --- |
| [httpx_test.go](httpx_test.go) | `TestWriteTypedErrorUsesProviderEnvelope`, `TestWriteJSONPreservesExistingHeaders` |

```bash
go test ./internal/httpx -count=1
```

Use XHUB_REGRESSION_STRICT=1 for database acceptance and inspect skips. Redis and live providers require separate configuration. Update this reference and feature documentation after contract changes.
