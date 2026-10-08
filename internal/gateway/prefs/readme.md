# Platform settings and request routing documents

[简体中文](readme_cn.md) · [Feature implementation reference](../../../docs/development/implementation.md)

## Responsibilities and behavior

Settings and page handlers persist and expose configuration; merge.go supplies OverlayDB and MergePatch; route_settings.go supplies Resolve and typed readers. Database top-level values override YAML, including null, lists, zero, and false. MergePatch recursively merges maps and replaces lists.
Resolve selects one complete document in key, team, organization, platform order. A selected missing template falls back to platform rather than selecting another ancestor. Gateway lookup failures are propagated rather than silently changing policy.
Retries means total attempts per deployment, at least one. Timeout defaults to 60 seconds; allowed failures defaults to three and nonpositive disables cooldown; nonpositive cooldown duration becomes 60 seconds in failure recording. Zero split weight excludes a deployment. Advanced stored fallback and retry fields are not promises of runtime execution.

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

Exported types: `RouteSettings`, `ScopeRef`, `ScopeLookup`.

- [`func Resolve(platform map[string]any, lookup ScopeLookup, chain ...ScopeRef) RouteSettings`](route_settings.go) — Resolve picks the router settings for one request from an ordered chain and reports where they came from. chain runs from the narrowest scope outward, and the FIRST scope that selects a template wins outright: key > team > organization > platform default The winning template's document IS the result - it is not stitched field by field with anything. A merge would have to answer "whose num_retries wins when the organization's template and the team's both set one", "whose weights do these percentages normalise against", and "what is strategy A composed with strategy B" - questions with no good answers, whose answers also change a team's behaviour whenever the level above it is edited. Replacing has a rule that fits in one sentence: the settings that apply are the ones the winning scope chose
- [`func RequestChain(keyID, teamID, orgID string) []ScopeRef`](route_settings.go) — RequestChain is the inheritance chain for one request, narrowest first. One function for both callers of Resolve, so the request path and the console cannot disagree about the order. A missing id is simply not part of the chain.
- [`func PlatformSettings(settings map[string]any) RouteSettings`](route_settings.go) — PlatformSettings is the platform-default document as a RouteSettings value. It exists so a caller with no lookup at all - a path with no database, or a test - still expresses "the platform default applies" the same way.
- [`func (r RouteSettings) Strategy() string`](route_settings.go) — Strategy is the routing strategy to order deployments with.
- [`func (r RouteSettings) Retries() int`](route_settings.go) — Retries is how many times one deployment is tried before the next is used. A value below one becomes one: a request that is never attempted cannot succeed, and the caller would see a 502 for what is really a settings mistake.
- [`func (r RouteSettings) TimeoutSeconds() float64`](route_settings.go) — TimeoutSeconds is how long one upstream call may take.
- [`func (r RouteSettings) AllowedFails() int`](route_settings.go) — AllowedFails is how many failures a deployment may collect before it is put into cooldown. Zero or less disables cooldown entirely, which is the contract RecordFailure already reads.
- [`func (r RouteSettings) CooldownSeconds() float64`](route_settings.go) — CooldownSeconds is how long a deployment stays out after it trips the failure count. Zero does not mean "no cooldown": the existing contract reads a non-positive value as one minute, and that contract is kept here.
- [`func (r RouteSettings) WeightOverrides() map[string]float64`](route_settings.go) — WeightOverrides reads the traffic shares stored on this document. They live under routing_strategy_args.weights, which is part of the same router settings JSON the console edits. Two shapes are accepted, because the form writes a list and a hand-edited file may write a map: {"weights": [{"api_base": "https://a", "model": "gpt-4o", "weight": 70}]} {"weights": {"https://a|gpt-4o": 70}} The key is api_base|model, the same string DeploymentID uses. A missing or empty list means "use each deployment's own weight". Zero disables a deployment. Negative or non-finite weights are ignored.

### settings.go

- [`func Base(s Host) map[string]any`](settings.go) — Base is the router-settings baseline from YAML and code defaults. allowed_fails is fixed at 3 here. A merged allowed_fails of at least 1 records the failure in Redis and starts cooldown. A cooldown_time of 0 means one minute. Only an explicit value below 1 skips recording the failure.
- [`func MergedRouter(s Host) map[string]any`](settings.go) — MergedRouter overlays database router settings on the baseline. Keys absent from the database keep the YAML value.
- [`func MergedGeneral(s Host) map[string]any`](settings.go) — MergedGeneral overlays database general settings on YAML. master_key, database_url, and redis_url are not exposed from the database.
- [`func ApplyTyped(s Host, m map[string]any)`](settings.go) — ApplyTyped copies strategy, retries, and timeout from the merged document into the typed config. Fields that are absent are left unchanged.
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
| [route_settings_test.go](route_settings_test.go) | `TestNoTemplateSelectedReadsThePlatformDocument`, `TestTheNarrowestSelectionWins`, `TestNotSelectingInheritsRatherThanFallingBack`, `TestANarrowerSelectionOverridesTheWiderOne`, `TestADeletedTemplateFallsBackWithoutUsingAWiderOne`, `TestAMissingScopeIsSkipped`, `TestRequestChainIsNarrowestFirst`, `TestATemplateThatOmitsAKeyMeansTheDefaultNotZero`, `TestAnExplicitZeroRetryIsClampedToOne`, `TestAllowedFailsKeepsZeroMeaningOff`, `TestAQuotedNumberIsNotReadAsANumber`, `TestIntegersFromYAMLAreRead`, `TestAPlatformWithNoLookupStillResolves`, `TestWeightOverridesReadsTheDocument` |
| [weight_regression_test.go](weight_regression_test.go) | `TestWeightOverridesPreserveZero` |

```bash
go test ./internal/gateway/prefs -count=1
```

Use XHUB_REGRESSION_STRICT=1 for database acceptance and inspect skips. Redis and live providers require separate configuration. Update this reference and feature documentation after contract changes.
