# iam

## Purpose

`iam` is the identity store: accounts, organizations, teams, memberships, projects, access groups, virtual keys, usage, request logs and the audit trail. `schema.sql` owns the constraints (composite foreign keys, CHECKs, a case-insensitive email) and xorm owns every read and write. There is no migration from the previous JSON-membership schema and no compatibility layer: the database is rebuilt, and this schema is the whole of it.

Team membership is read from `team_members`; organization-administrator assignments are read from `organization_members`. Authorization uses current rows, so removing membership takes effect on the next request rather than at the next login.

## The model

Account, organization, and team roles are independent:

| Scope | Values |
| --- | --- |
| Account (`users.role`) | `admin`, `user` |
| Organization (`organization_members.role`) | `org_admin` |
| Team (`team_members.role`) | `team_admin`, `member` |

Resources descend **organization → team → project**. Platform administrators manage global resources; organization administrators manage members and projects within their organization, with the limits in the [permission reference](../../docs/development/permissions.md). Personal inference access requires team membership. There is no project-administrator or access-group role.

Ownership of an inference call is **snapshotted** onto `usage_events` at write time. `usage_events` therefore has no foreign keys at all: a member who leaves a team does not move the spend they already produced, and deleting a team does not erase history.

## Files

| File | Contents |
| --- | --- |
| `schema.sql` | Every table, index and constraint. Applied by `Migrate`. |
| `db.go` | `Open`, `Migrate`, the transaction helper, error mapping, and the audit writer. |
| `models.go` | The table beans and the role/status constants. |
| `users.go` | Accounts: create, read, profile, password, admin update, delete, `EnsureAdmin`. |
| `teams.go` | Organizations, teams, memberships, projects, and access-group assignment. |
| `keys.go` | Virtual keys, their narrowing, access groups, and the model-resolution functions. |
| `usage.go` | The usage write path: events, request logs, the daily roll-up, live spend. |
| `usage_read.go` | The scoped read API over usage and request logs. |

## The model set

`allowedModels` is the only function the catalog and the inference path use. Every scope resolves to a single team and capabilities are never merged across teams:

```
team    = the team's model list (empty means unrestricted at this level)
project = team ∩ project narrowing   (when the scope names a project)
key     = (project or team) ∩ key narrowing   (when the scope names a key)
```

Two callers wrap it:

- `AllowedModelsForTeam(ctx, teamID)` — a team's set, used before a key exists
- `AllowedModelsForKey(ctx, k)` — a key's effective set

An empty model list on a project or key means **inherit**, not **deny**. The resolver uses nil for unrestricted scope; authorization still checks identity, current membership, blocked resources, and deployment availability. Access-group records are not the source of this resolver's team model set.

## Reading usage and logs

`usage_events` and `usage_daily` are read through `UsageQuery`, which takes a `builder.Cond` produced by `authz.UsageScope` or `authz.LogsScope`. The scope is applied **first** and every other filter only narrows inside it, so a `user_id` or `api_key` taken from a query string can never widen a read. This package does not import `authz`; the caller passes the condition in, which keeps the policy in one place and the query layer free of it.

| Function | Returns |
| --- | --- |
| `ListUsage` | Events in scope, newest first |
| `CountUsage` | How many, for paging |
| `GetUsageEvent` | One event in scope, or `ErrNotFound` outside it |
| `GetRequestLog` | The stored bodies of one event |
| `DailyUsage` | The roll-up per day |
| `DailyUsageByModel` | The roll-up per day and model |
| `RollupByModel` | Totals per model |
| `RollupByKey` | Totals per key, with a display name |
| `RollupByTeam` | Totals per team |
| `AuditLogRead` | Writes the audit row for reading someone else's log |

## How another package uses it

```go
db, err := iam.Open(ctx, cfg.GeneralSettings.DatabaseURL)
if err != nil {
    log.Fatal(err)
}
defer db.Close()

p, err := auth.Resolve(ctx, cfg, db, r)          // who is calling
g, err := authz.New(db).Guard(ctx, p.Actor())    // what they may do
sc, err := g.UsageScope(ctx, teamID)             // what rows they may read
rows, err := db.ListUsage(ctx, iam.UsageQuery{Cond: sc.Cond, Limit: 50})
```

Two details matter when adding a query:

- A **read projection** must carry xorm tags. xorm maps result columns by its own `xorm:` tag and never by `json:`, so a struct with only json tags scans every column into the zero value without reporting an error.
- A `models` column holds a JSON array in a `TEXT` column. Project it into a struct with `xorm:"json 'models'"`; projecting it straight into a `[]string` returns the raw document as a single element.

## Seeding the first administrator

`EnsureAdmin` creates a platform administrator from configuration if no account with that address exists, and reports whether it created one. It **never** updates an existing row: the configured password is an initial password, so changing the value in a config file does not reset a live account. `Bootstrapped` is the marker the console reads; both the seeding pass and `POST /bootstrap` set it.

## What a request touches

`usage_events` is written by `RecordUsage` in one transaction with `request_logs`, the `usage_daily` roll-up, and the live spend counters on the user, key, team, project, and organization. The insert is `ON CONFLICT (request_id) DO NOTHING`. A replayed flush, including a second gateway draining the same Redis entry, inserts the event once and therefore increments the roll-up once. `usage_events` has no foreign keys: the owner, team, and organization are the snapshot taken when the call was authorized.

`request_logs` holds the stored request body, response body, and proxy request. Reading another account's row is what the audit log records. Platform-admin reads of someone else's personal log go through `authz`, not through a shortcut here.

`CountUsage` counts events inside the caller's `UsageQuery` for paging. A failed count returns `0` and the error. `DailyUsage` groups by the calendar day in the caller's timezone offset. `RollupByKey` and `RollupByTeam` take the display name with a correlated subquery so the join does not multiply the grouped rows.

`api_keys.token_hash` is what `Principal.Hash` points at. RPM and TPM Redis keys use that hash (`xhub:rpm:`, `xhub:tpm:`). They do not use `api_base|model`.

## What this package does not do

It does not decide authorization. Every predicate — who may read whose usage, who may manage which team — lives in `authz`, and this package only applies the resulting filter. It does not hash or verify session tokens; `auth` does. It does not count in-flight calls (`internal/hooks`) and it does not talk to Redis (`internal/live`).

中文使用说明见同目录的 readme_cn.md。
