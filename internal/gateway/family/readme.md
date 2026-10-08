# API family handlers and resource isolation

[简体中文](readme_cn.md) · [Feature implementation reference](../../../docs/development/implementation.md)

## Responsibilities and behavior

mount.go explicitly registers the two Responses routes; handlers.go maps family paths to inference operations, with other registered paths dispatched through the gateway catalog. Host interfaces expose only required execution and storage capabilities.
Supported inference operations use the shared data plane and retain policy, budgets, and templates. Local Assistants/Threads metadata is tenant-isolated; possessing an ID does not grant access to another caller's object.
Unsupported asynchronous families, files, batches, fine-tuning, and realtime return explicit unsupported responses rather than fabricated success. previous_response_id contributes to affinity. Route registration is separate from full protocol compatibility, which needs fixtures and live-provider evidence.

## Source responsibilities and entry points

### codec.go

Internal implementation and protocol boundaries:  [codec.go](codec.go)。

### handlers.go

- [`func ServeDataPlane(s Host, w http.ResponseWriter, r *http.Request)`](handlers.go) — ServeDataPlane handles inference paths in the catalog. A recognized operation enters the data plane. The rest are resource reads and writes.
- [`func Responses(s Host, w http.ResponseWriter, r *http.Request)`](handlers.go) — Responses is the Responses API entry and always uses the data-plane responses operation.
- [`func ServeMixed(s Host, w http.ResponseWriter, r *http.Request)`](handlers.go) — ServeMixed answers the catalog's "mixed" paths, such as /v1/agents, /v1/skills and /v1/workflows. These were removed from the product and only remain in the catalog. They are refused rather than served: the generic store behind them is one shared key-value namespace keyed by resource kind, with no owner and no team column, so serving them let any signed-in member — and any inference key — create records that every other principal could then list. Refusing is what the classification has always described; this makes the code match. The identity is still required before the refusal, so an anonymous caller learns nothing beyond the fact that the path exists.
- [`func ServeMgmt(s Host, w http.ResponseWriter, r *http.Request)`](handlers.go) — ServeMgmt handles management catalog paths. It requires a management identity, then reads or writes by resource kind.
- [`func Freeze(kind string, obj map[string]any)`](handlers.go) — Freeze fills fields the public contract for this resource family requires. An existing value is not overwritten.

### host.go

Exported types: `Host`.

Internal implementation and protocol boundaries:  [host.go](host.go)。

### mount.go

- [`func Module(h Host) httpx.Module`](mount.go) — Module sends the Responses API to the data plane. Other catalog resources are not part of this module.

## External HTTP boundary

The registration files below mount these routes. Aliases share handlers. Registration does not replace authorization or business assertions; see module contracts and the API reference.

| Method / path | Registration |
| --- | --- |
| `POST /v1/responses` | [mount.go](mount.go) |
| `POST /responses` | [mount.go](mount.go) |

## Dependencies

[internal/auth](../../auth/readme.md), [internal/catalog](../../catalog/readme.md), [internal/dataplane](../../dataplane/readme.md), [internal/httpx](../../httpx/readme.md), [internal/logx](../../logx/readme.md), [internal/store](../../store/readme.md).

## Verification and maintenance

| Test file | Scenario entry points |
| --- | --- |
| [resource_isolation_test.go](resource_isolation_test.go) | `TestInferenceResourcesAreIsolatedByCaller`, `TestUnsupportedInferenceDoesNotFabricateUsage`, `TestInjectModelPreservesInvalidPayloadForValidation` |

```bash
go test ./internal/gateway/family -count=1
```

Use XHUB_REGRESSION_STRICT=1 for database acceptance and inspect skips. Redis and live providers require separate configuration. Update this reference and feature documentation after contract changes.
