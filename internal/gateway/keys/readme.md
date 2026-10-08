# Virtual and service-key lifecycle

[简体中文](readme_cn.md) · [Feature implementation reference](../../../docs/development/implementation.md)

## Responsibilities and behavior

generate.go creates personal and service-account credentials; admin.go implements listing, detail, edits, bulk changes, blocking, deletion, rotation, and spend reset. Host dependencies cover identity, authorization, storage, and template visibility.
Plaintext is returned at creation and rotation boundaries. Response's includePlain controls disclosure; ordinary list/detail reads must not leak credentials. Service-key ownership, organization, team, and project must be validated rather than confused with personal ownership.
Management permissions and model permissions remain separate. Template selections are read and authorized before mutation; empty selections inherit. Authentication rechecks blocked, expired, rotated, and deleted keys. Bulk operations validate each target object rather than assuming one request-level authorization covers all rows.

## Source responsibilities and entry points

### admin.go

- [`func ServiceAccount(s Host, w http.ResponseWriter, r *http.Request)`](admin.go) — ServiceAccount generates a service key for a team or one of its projects. It has no owner, so it requires the team's administration rather than mere membership; Authorize makes that decision.
- [`func Regenerate(s Host, w http.ResponseWriter, r *http.Request)`](admin.go) — Regenerate rotates the key plaintext. The old plaintext stops working immediately, and the key's own limits and narrowing are untouched.
- [`func ResetSpend(s Host, w http.ResponseWriter, r *http.Request)`](admin.go) — ResetSpend sets the key spend back to zero, or to reset_to when the body carries one. Historical usage rows are not deleted, so the figures that produced the old total remain.
- [`func Aliases(s Host, w http.ResponseWriter, r *http.Request)`](admin.go) — Aliases lists the key names the caller may see, for the dashboard pickers.
- [`func Health(s Host, w http.ResponseWriter, r *http.Request)`](admin.go) — Health checks that a credential still passes identification. It is the liveness probe the LiteLLM clients call before their first request.
- [`func BulkUpdate(s Host, w http.ResponseWriter, r *http.Request)`](admin.go) — BulkUpdate applies one patch to many keys. A key the caller may not write is skipped rather than failing the batch, and updated is the count that landed.

### codec.go

Internal implementation and protocol boundaries:  [codec.go](codec.go)。

### generate.go

- [`func Generate(s Host, w http.ResponseWriter, r *http.Request)`](generate.go) — Generate creates a virtual key and returns the plaintext only in this response. A member may mint a personal key for themselves inside a team they belong to; a service key needs the team's administration, and the decision is made by Authorize rather than here.
- [`func List(s Host, w http.ResponseWriter, r *http.Request)`](generate.go) — List lists the keys the caller may see and never returns plaintext. The rows are narrowed in SQL by the scope, so a handler bug cannot widen the listing.
- [`func Info(s Host, w http.ResponseWriter, r *http.Request)`](generate.go) — Info reads one virtual key.
- [`func Delete(s Host, w http.ResponseWriter, r *http.Request)`](generate.go) — Delete removes virtual keys. The plaintext can no longer call inference.
- [`func Block(s Host, w http.ResponseWriter, r *http.Request)`](generate.go) — Block marks a virtual key blocked.
- [`func Unblock(s Host, w http.ResponseWriter, r *http.Request)`](generate.go) — Unblock clears the blocked flag.
- [`func Update(s Host, w http.ResponseWriter, r *http.Request)`](generate.go) — Update changes a virtual key's name, narrowing, and limits. The plaintext stays the same, and a field the caller left out keeps its value.
- [`func Response(k iam.Key, plain string, includePlain bool) map[string]any`](generate.go) — Response is the public JSON for a virtual key. The plaintext is omitted when includePlain is false; the stored hash never appears, because the hash is what authentication compares and publishing it would leak the credential.

### host.go

Exported types: `Host`.

Internal implementation and protocol boundaries:  [host.go](host.go)。

### mount.go

- [`func Module(h Host) httpx.Module`](mount.go) — Module creates, lists, updates, and rotates virtual keys.

## External HTTP boundary

The registration files below mount these routes. Aliases share handlers. Registration does not replace authorization or business assertions; see module contracts and the API reference.

| Method / path | Registration |
| --- | --- |
| `POST /key/generate` | [mount.go](mount.go) |
| `POST /key/service-account/generate` | [mount.go](mount.go) |
| `GET /key/list` | [mount.go](mount.go) |
| `GET /key/info` | [mount.go](mount.go) |
| `POST /v2/key/info` | [mount.go](mount.go) |
| `POST /key/delete` | [mount.go](mount.go) |
| `POST /key/block` | [mount.go](mount.go) |
| `POST /key/unblock` | [mount.go](mount.go) |
| `POST /key/update` | [mount.go](mount.go) |
| `POST /key/bulk_update` | [mount.go](mount.go) |
| `POST /key/regenerate` | [mount.go](mount.go) |
| `POST /key/{key}/regenerate` | [mount.go](mount.go) |
| `POST /key/{key}/reset_spend` | [mount.go](mount.go) |
| `GET /key/aliases` | [mount.go](mount.go) |
| `POST /key/health` | [mount.go](mount.go) |

## Dependencies

[internal/auth](../../auth/readme.md), [internal/authz](../../authz/readme.md), [internal/gateway/templateauth](../templateauth/readme.md), [internal/httpx](../../httpx/readme.md), [internal/iam](../../iam/readme.md), [internal/logx](../../logx/readme.md).

## Verification and maintenance

There are no direct test files here. Integration tests prove executed paths rather than every internal failure branch.

```bash
go test ./internal/gateway/keys -count=1
```

Use XHUB_REGRESSION_STRICT=1 for database acceptance and inspect skips. Redis and live providers require separate configuration. Update this reference and feature documentation after contract changes.
