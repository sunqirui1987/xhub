# Platform settings and request routing documents

[简体中文](readme_cn.md) · [Feature implementation reference](../../../docs/development/implementation.md)

## Responsibilities and behavior

Settings and page handlers persist configuration. `Overlay` overlays database top-level keys on YAML, preserving null, lists, zero and false; `MergePatch` merges nested maps and replaces lists.
`route_settings.go` re-exports the router API. `Resolve(lookup, chain...)` selects the first complete template in key → team → organization order. No binding or a missing selected template uses builtin model defaults; missing templates do not select another ancestor. Gateway storage failures fail the request.
Model rules, allocations, retry_policy and fallback execution are documented in [internal/router](../../router/readme.md). The default strategy is traffic-split; max_attempts counts total attempts per deployment. Unsupported fields are rejected at save time.

## Source responsibilities and entry points

### codec.go

Internal implementation and protocol boundaries:  [codec.go](codec.go)。

### host.go

Exported types: `Host`.

Internal implementation and protocol boundaries:  [host.go](host.go)。

### merge.go

- [`func Overlay(base, db map[string]any) map[string]any`](merge.go) — Overlay copies the YAML base and then writes every database key on top. A database value wins even when it is a list or null. Keys absent from the database stay as they were.
- [`func MergePatch(current, patch map[string]any) map[string]any`](merge.go) — MergePatch folds a partial update into the current document. Nested objects are merged, while lists and scalars replace the previous value. Keys that the patch does not mention stay.

### mount.go

- [`func Module(h Host) httpx.Module`](mount.go) — Module reads and writes router settings and general settings.

### page.go

- [`func Page(s Host, w http.ResponseWriter, r *http.Request)`](page.go) — Page writes the merged router settings and field descriptions for the dashboard to render.
- [`func PageMap(s Host) map[string]any`](page.go) — PageMap returns the merged router-settings document, including routing_groups.
- [`func Fields(s Host, rs map[string]any) []map[string]any`](page.go) — Fields splits router settings into the dashboard field list, with the current value and the default.
- [`func Callbacks(s Host, w http.ResponseWriter, r *http.Request)`](page.go) — Callbacks returns the callback view. Its router_settings match the merged document.
- [`func List(s Host, w http.ResponseWriter, r *http.Request)`](page.go) — List returns settings for the requested config_type. general_settings includes stored_in_db.

### route_settings.go

This package exports aliases for RouteSettings, ScopeRef, ScopeLookup, Resolve, RequestChain, BuiltinSettings, BuiltinDocument, ValidateRouteTemplateDocument, TemplateRouting, and ValidateTemplateCatalog. Their implementation and contracts live in [router/settings.go](../../router/settings.go) and [router/compile.go](../../router/compile.go). Tests in this directory verify the call contract; router owns parsing and execution.

### settings.go

- [`func Base(s Host) map[string]any`](settings.go) — Base returns the four global execution settings: num_retries, timeout, allowed_fails, and cooldown_time. YAML values override code defaults; the default failure threshold is 3 and cooldown is 0.
- [`func MergedRouter(s Host) map[string]any`](settings.go) — MergedRouter overlays database router settings on the baseline. Keys absent from the database keep the YAML value.
- [`func MergedGeneral(s Host) map[string]any`](settings.go) — MergedGeneral overlays database general settings on YAML. master_key, database_url, and redis_url are not exposed from the database.
- [`func ApplyTyped(s Host, m map[string]any)`](settings.go) — ApplyTyped copies num_retries and timeout from the merged document into the typed runtime config; absent fields are left unchanged. Failure threshold and cooldown are read from the merged document where routing uses them.
- [`func Update(s Host, w http.ResponseWriter, r *http.Request)`](settings.go) — Update accepts a partial update of router, general, or LiteLLM settings. It requires a management identity.
- [`func GeneralList(s Host) []map[string]any`](settings.go) — GeneralList returns the general-settings list with stored_in_db. A key only in the database is true, a key only in YAML is false, and a key in neither is null.
- [`func FieldUpdate(s Host, w http.ResponseWriter, r *http.Request)`](settings.go) — FieldUpdate updates one general-settings field. A missing field_name returns 400.
- [`func FieldDelete(s Host, w http.ResponseWriter, r *http.Request)`](settings.go) — FieldDelete deletes one general-settings field. After deletion the YAML baseline applies again.

## External HTTP boundary

The registration files below mount these routes. Aliases share handlers. Registration does not replace authorization or business assertions; see module contracts and the API reference.

| Method / path | Registration |
| --- | --- |
| `GET /router/settings` | [mount.go](mount.go) |
| `GET /router/fields` | [mount.go](mount.go) |
| `GET /get/config/callbacks` | [mount.go](mount.go) |
| `GET /config/list` | [mount.go](mount.go) |
| `POST /config/update` | [mount.go](mount.go) |
| `POST /config/field/update` | [mount.go](mount.go) |
| `POST /config/field/delete` | [mount.go](mount.go) |

## Dependencies

[internal/auth](../../auth/readme.md), [internal/config](../../config/readme.md), [internal/httpx](../../httpx/readme.md), [internal/iam](../../iam/readme.md), [internal/logx](../../logx/readme.md), [internal/store](../../store/readme.md).

## Verification and maintenance

| Test file | Scenario entry points |
| --- | --- |
| [route_settings_test.go](route_settings_test.go) | `TestNoTemplateUsesBuiltinModelDefault`, `TestNarrowestTemplateSelectionWins`, `TestUnselectedNarrowScopesInheritWiderTemplate`, `TestDanglingSelectionFallsBackToModelDefault`, `TestRequestChainIsNarrowestFirst`, `TestTypedExecutionDefaultsAndBoundaries`, `TestResolveWithoutLookupUsesModelDefault` |
| [allocation_test.go](allocation_test.go) | `TestTemplateOnlyOverridesNamedModels`, `TestBuiltinAndEmptyWeightsUseEqualTrafficSplit`, `TestStrictRouteTemplateDocument`, `TestTemplateDefaultStrategyAndOverride` |
| [routing_groups_test.go](routing_groups_test.go) | `TestRoutingGroupPrecedence` |
| [fallback_test.go](fallback_test.go) | `TestTemplateFallbackParsing`, `TestTemplateFallbackCatalog` |

```bash
go test ./internal/gateway/prefs -count=1
```

Use XHUB_REGRESSION_STRICT=1 for database acceptance and inspect skips. Redis and live providers require separate configuration. Update this reference and feature documentation after contract changes.
