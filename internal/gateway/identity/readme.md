# Identity, membership, and routing templates

[简体中文](readme_cn.md) · [Feature implementation reference](../../../docs/development/implementation.md)

## Responsibilities and behavior

Handlers implement users, organizations, teams, projects, and audit; members.go manages membership; route_template.go manages complete template documents, bindings, references, and deletion rules. Gate supplies authentication, authorization, IAM, and pagination capabilities.
Management checks actions and trusted ownership before mutation. Queries are scoped before loading rows. Cross-organization identifiers do not grant visibility. Role, password, and status changes must invalidate sessions through version semantics.
Templates are seeded from platform settings once and subsequently replaced as whole documents. Selection validates existence and visibility before writes; an empty selection inherits. Referenced templates cannot be deleted. Users and projects are not separate template-priority levels. The route inventory follows mount.go, not stale comments about retired features.

## Source responsibilities and entry points

### codec.go

Internal implementation and protocol boundaries:  [codec.go](codec.go)。

### gate.go

Exported types: `Gate`.

Internal implementation and protocol boundaries:  [gate.go](gate.go)。

### handlers.go

- [`func UserNew(g Gate, w http.ResponseWriter, r *http.Request)`](handlers.go) — UserNew creates an account. Only a platform administrator may, because the account's existence changes what every other decision can see. The account, its team membership, its organization administration and its budget are all set here rather than by follow-up calls: a create that half succeeded used to leave a person who existed but could reach nothing, and the console had to guess whether the second call was needed.
- [`func UserList(g Gate, w http.ResponseWriter, r *http.Request)`](handlers.go) — UserList lists accounts the caller may see. A platform administrator sees every account. An organization administrator sees the people in that organization's teams. A team administrator sees the people in the teams they administer. Everyone else sees only themselves.
- [`func UserFilterUI(g Gate, w http.ResponseWriter, r *http.Request)`](handlers.go) — UserFilterUI is the account picker the console builds filters from. It returns names and ids only and is a platform administrator route.
- [`func AvailableUsers(g Gate, w http.ResponseWriter, r *http.Request)`](handlers.go) — AvailableUsers is the account picker. It is the same listing as the filter picker: only a platform administrator may enumerate accounts.
- [`func UserInfo(g Gate, w http.ResponseWriter, r *http.Request)`](handlers.go) — UserInfo reads one account. A caller reads themselves; a platform administrator reads anybody.
- [`func UserUpdate(g Gate, w http.ResponseWriter, r *http.Request)`](handlers.go) — UserUpdate changes a role, a status, a budget or an email. Only a platform administrator may, and only their own name and password are reachable through the profile routes.
- [`func UserSetPassword(g Gate, w http.ResponseWriter, r *http.Request)`](handlers.go) — UserSetPassword sets somebody else's password. It is what the console calls when a person has forgotten theirs, and it replaces the reset link the product used to generate: that link pointed at an onboarding page which has no backend behind it, so the flow could not work and a password typed into it was discarded. A platform administrator may reset any account. A team administrator may reset a member of a team they administer, and an organization administrator a member of any team in their organization. That is narrower than the power to change a role, and the decision in authz refuses an account that runs the platform or administers another organization. The new password is checked by the store, not here: the eight-character minimum lives with the hash so every writer of a password obeysi
- [`func UserDelete(g Gate, w http.ResponseWriter, r *http.Request)`](handlers.go) — UserDelete removes an account. Its memberships go with it and its personal keys are revoked; service keys belong to teams and are untouched.
- [`func OrgNew(g Gate, w http.ResponseWriter, r *http.Request)`](handlers.go) — OrgNew creates an organization. Only a platform administrator may: an organization is a grouping of teams, not something a tenant owns.
- [`func OrgList(g Gate, w http.ResponseWriter, r *http.Request)`](handlers.go) — OrgList lists the organizations the caller may see. A platform administrator sees every one; everybody else sees only the organizations that own one of their teams, which is what the console shows asthe team's grouping.
- [`func OrgInfo(g Gate, w http.ResponseWriter, r *http.Request)`](handlers.go) — OrgInfo reads one organization.
- [`func OrgUpdate(g Gate, w http.ResponseWriter, r *http.Request)`](handlers.go) — OrgUpdate changes an organization's name, status or budget.
- [`func OrgDelete(g Gate, w http.ResponseWriter, r *http.Request)`](handlers.go) — OrgDelete removes an organization. It refuses while teams remain, because deleting the teams underneath as a side effect of a rename-shaped request is not what the caller asked for.
- [`func TeamNew(g Gate, w http.ResponseWriter, r *http.Request)`](handlers.go) — TeamNew creates a team inside an organization, together with its first administrator. Only a platform administrator may create or move a team.
- [`func TeamList(g Gate, w http.ResponseWriter, r *http.Request)`](handlers.go) — TeamList lists the teams the caller belongs to, or every team for a platform administrator. The rows carry the caller's own role. The body is an array, which is what GET /team/list has always returned
- [`func TeamListV2(g Gate, w http.ResponseWriter, r *http.Request)`](handlers.go) — TeamListV2 is GET /v2/team/list. The console's paged query reads teams, total, page, page_size and total_pages. A bare array makes teams undefined, and React Query rejects a query function that returns undefined.
- [`func TeamAvailable(g Gate, w http.ResponseWriter, r *http.Request)`](handlers.go) — TeamAvailable lists the teams the caller could put a resource in, which is the same set as their own teams.
- [`func TeamInfo(g Gate, w http.ResponseWriter, r *http.Request)`](handlers.go) — TeamInfo reads one team.
- [`func TeamUpdate(g Gate, w http.ResponseWriter, r *http.Request)`](handlers.go) — TeamUpdate changes a team. A team administrator may change the name and the description of their own team; the status and the budget ceiling are a platform administrator's decision, and the two are separate actions because the matrix separates them.
- [`func TeamDelete(g Gate, w http.ResponseWriter, r *http.Request)`](handlers.go) — TeamDelete removes a team. Its projects, memberships and key assignments go with it, and its keys stop working on their next request.
- [`func TeamMove(g Gate, w http.ResponseWriter, r *http.Request)`](handlers.go) — TeamMove changes the organization a team belongs to. It is a platform administrator route because it re-scopes every access group the team holds.
- [`func ProjectNew(g Gate, w http.ResponseWriter, r *http.Request)`](handlers.go) — ProjectNew creates a project inside a team. A team administrator may.
- [`func ProjectList(g Gate, w http.ResponseWriter, r *http.Request)`](handlers.go) — ProjectList lists projects. A platform administrator sees every project; a member sees the projects of the teams they belong to; nobody sees a project of a team they are not in. The narrowing happensin SQL.
- [`func ProjectInfo(g Gate, w http.ResponseWriter, r *http.Request)`](handlers.go) — ProjectInfo reads one project.
- [`func ProjectUpdate(g Gate, w http.ResponseWriter, r *http.Request)`](handlers.go) — ProjectUpdate changes a project's name, status, narrowing and budget. The store refuses a narrowing that is not inside the team's grants.
- [`func ProjectDelete(g Gate, w http.ResponseWriter, r *http.Request)`](handlers.go) — ProjectDelete removes a project and its keys.
- [`func TeamModels(g Gate, w http.ResponseWriter, r *http.Request)`](handlers.go) — TeamModels reports the models a team may reach. A member may read it for their own team.
- [`func AuditLog(g Gate, w http.ResponseWriter, r *http.Request)`](handlers.go) — AuditLog lists the audit trail. Only a platform administrator may read it.

### members.go

- [`func TeamMembers(g Gate, w http.ResponseWriter, r *http.Request)`](members.go) — TeamMembers lists a team's roster. Any member of the team may read it, which is what the member selector is built from; the rows carry no key, no budget and no per-member spend.
- [`func OrgMemberAdd(g Gate, w http.ResponseWriter, r *http.Request)`](members.go) — OrgMemberAdd grants organization administration to an existing account. The only organization role is org_admin. People reach an organization by joining one of its teams; this route does not create ageneral membership.
- [`func OrgMemberRemove(g Gate, w http.ResponseWriter, r *http.Request)`](members.go) — OrgMemberRemove revokes organization administration.
- [`func TeamMemberAdd(g Gate, w http.ResponseWriter, r *http.Request)`](members.go) — TeamMemberAdd adds an existing account to a team by exact email. An unknown address and a disabled account give the same answer, so the route cannot be used to enumerate accounts.
- [`func TeamMemberUpdate(g Gate, w http.ResponseWriter, r *http.Request)`](members.go) — TeamMemberUpdate changes a team role. The store refuses to leave a team without an administrator, and the refusal is a 400 rather than a silent demotion.
- [`func TeamMemberRemove(g Gate, w http.ResponseWriter, r *http.Request)`](members.go) — TeamMemberRemove removes a membership. The store revokes the personal keys bound to that team in the same transaction, so a removed member's keys stop working on their next request.

### mount.go

- [`func Module(h Gate) httpx.Module`](mount.go) — Module is users, organizations, teams, members, projects, access groups and the audit trail. The process implements Gate. This package does not import gateway. The paths are the LiteLLM-compatible ones the console already calls, mounted over the new authorization. The routes whose subject does not exist in the new model are gone rather than stubbed: there is no budget object to create, no organization member, no account role beyond the two the matrix has, and no bulk role assignment.

### route_template.go

- [`func RouteTemplateList(g Gate, w http.ResponseWriter, r *http.Request)`](route_template.go) — RouteTemplateList returns every template with its usage count.
- [`func RouteTemplateGet(g Gate, w http.ResponseWriter, r *http.Request)`](route_template.go) — RouteTemplateGet returns one template.
- [`func RouteTemplateCreate(g Gate, w http.ResponseWriter, r *http.Request)`](route_template.go) — RouteTemplateCreate inserts a template.
- [`func RouteTemplateUpdate(g Gate, w http.ResponseWriter, r *http.Request)`](route_template.go) — RouteTemplateUpdate replaces a template's name and body.
- [`func RouteTemplateDelete(g Gate, w http.ResponseWriter, r *http.Request)`](route_template.go) — RouteTemplateDelete removes a template, refusing while any scope still selects it. The refusal carries the scopes that hold it because "in use" alone leaves the operator to go find them, and the difference between "I removed something nobody used" and "I changed three teams' behaviour" is the whole reason to refuse in the first place.
- [`func RouteTemplateUsage(g Gate, w http.ResponseWriter, r *http.Request)`](route_template.go) — RouteTemplateUsage lists the scopes selecting one template. It is the same list the delete refusal carries, for an operator who wants to look before deleting.
- [`func RouteTemplateBinding(g Gate, w http.ResponseWriter, r *http.Request)`](route_template.go) — RouteTemplateBinding reads or writes which template one scope selects. Selecting is narrower than editing: an organization administrator may point their own organization, a team administrator their own team, and a key's owner their own key. That is why each branch asks for the write action on the scope's own object rather than for the platform administrator gate. An empty template id clears the selection, which is how a scope goes back to inheriting.

## External HTTP boundary

The registration files below mount these routes. Aliases share handlers. Registration does not replace authorization or business assertions; see module contracts and the API reference.

| Method / path | Registration |
| --- | --- |
| `POST /user/new` | [mount.go](mount.go) |
| `GET /user/list` | [mount.go](mount.go) |
| `GET /user/filter/ui` | [mount.go](mount.go) |
| `GET /user/available_users` | [mount.go](mount.go) |
| `GET /user/info` | [mount.go](mount.go) |
| `GET /v2/user/info` | [mount.go](mount.go) |
| `POST /user/update` | [mount.go](mount.go) |
| `POST /user/delete` | [mount.go](mount.go) |
| `POST /user/set_password` | [mount.go](mount.go) |
| `POST /user/{user_id}/password` | [mount.go](mount.go) |
| `POST /organization/new` | [mount.go](mount.go) |
| `GET /organization/list` | [mount.go](mount.go) |
| `GET /organization/info` | [mount.go](mount.go) |
| `PATCH /organization/update` | [mount.go](mount.go) |
| `DELETE /organization/delete` | [mount.go](mount.go) |
| `POST /organization/member_add` | [mount.go](mount.go) |
| `POST /organization/member_delete` | [mount.go](mount.go) |
| `DELETE /organization/member_delete` | [mount.go](mount.go) |
| `POST /team/new` | [mount.go](mount.go) |
| `GET /team/list` | [mount.go](mount.go) |
| `GET /v2/team/list` | [mount.go](mount.go) |
| `GET /team/available` | [mount.go](mount.go) |
| `GET /team/info` | [mount.go](mount.go) |
| `POST /team/update` | [mount.go](mount.go) |
| `POST /team/delete` | [mount.go](mount.go) |
| `POST /team/move` | [mount.go](mount.go) |
| `GET /team/models` | [mount.go](mount.go) |
| `GET /team/member_list` | [mount.go](mount.go) |
| `POST /team/member_add` | [mount.go](mount.go) |
| `POST /team/member_update` | [mount.go](mount.go) |
| `POST /team/member_delete` | [mount.go](mount.go) |
| `POST /project/new` | [mount.go](mount.go) |
| `GET /project/list` | [mount.go](mount.go) |
| `POST /project/list` | [mount.go](mount.go) |
| `GET /project/info` | [mount.go](mount.go) |
| `POST /project/info` | [mount.go](mount.go) |
| `POST /project/update` | [mount.go](mount.go) |
| `POST /project/delete` | [mount.go](mount.go) |
| `GET /route_template/list` | [mount.go](mount.go) |
| `POST /route_template/new` | [mount.go](mount.go) |
| `GET /route_template/{template_id}` | [mount.go](mount.go) |
| `POST /route_template/{template_id}/update` | [mount.go](mount.go) |
| `POST /route_template/{template_id}/delete` | [mount.go](mount.go) |
| `GET /route_template/{template_id}/usage` | [mount.go](mount.go) |
| `GET /route_template/binding` | [mount.go](mount.go) |
| `POST /route_template/binding` | [mount.go](mount.go) |
| `GET /audit/logs` | [mount.go](mount.go) |

## Dependencies

[internal/auth](../../auth/readme.md), [internal/authz](../../authz/readme.md), [internal/gateway/prefs](../prefs/readme.md), [internal/gateway/templateauth](../templateauth/readme.md), [internal/httpx](../../httpx/readme.md), [internal/iam](../../iam/readme.md), [internal/logx](../../logx/readme.md).

## Verification and maintenance

| Test file | Scenario entry points |
| --- | --- |
| [route_template_error_test.go](route_template_error_test.go) | `TestRequestLookupRetainsDatabaseErrors` |

```bash
go test ./internal/gateway/identity -count=1
```

Use XHUB_REGRESSION_STRICT=1 for database acceptance and inspect skips. Redis and live providers require separate configuration. Update this reference and feature documentation after contract changes.
