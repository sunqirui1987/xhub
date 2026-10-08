# 身份资源、成员与模板管理

[English](readme.md) · [全功能实现说明](../../../docs/development/implementation.md)

## 职责与实现契约

handlers.go 处理用户、组织、团队、项目及审计，members.go 管理成员关系，route_template.go 管理模板创建、完整更新、选择、usage 引用查询和删除限制。Gate 集中提供调用身份、动作授权、IAM 与分页能力。
管理 API 先检查操作者动作权限，再按可信归属读取和修改资源；列表在查询层收窄。组织间 ID 猜测和跨团队成员写入必须被拒绝。角色、密码和账号状态变化要配合会话版本失效。
模板创建从平台默认播种一次，之后更新替换整份文档。绑定组织、团队或密钥前由 templateauth.Selection 检查模板存在及可见性；空选择恢复继承。仍被引用的模板拒绝删除。项目和用户没有独立模板优先级层。确切 HTTP 列表以 mount.go 为准，旧注释中的已退休功能不能当作现有接口。

## 源码职责与入口

### codec.go

内部实现和协议边界见 [codec.go](codec.go)。

### gate.go

公开类型：`Gate`.

内部实现和协议边界见 [gate.go](gate.go)。

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

## 对外 HTTP 边界

入口由下表所列注册文件安装。别名共享处理器；路径存在不替代权限和业务断言，授权说明见模块契约及接口参考。

| Method / path | 注册文件 |
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

## 依赖关系

[internal/auth](../../auth/readme_cn.md), [internal/authz](../../authz/readme_cn.md), [internal/gateway/prefs](../prefs/readme_cn.md), [internal/gateway/templateauth](../templateauth/readme_cn.md), [internal/httpx](../../httpx/readme_cn.md), [internal/iam](../../iam/readme_cn.md), [internal/logx](../../logx/readme_cn.md).

## 验证与维护入口

| 测试文件 | 场景入口 |
| --- | --- |
| [route_template_error_test.go](route_template_error_test.go) | `TestRequestLookupRetainsDatabaseErrors` |

```bash
go test ./internal/gateway/identity -count=1
```

数据库验收设置 XHUB_REGRESSION_STRICT=1 并检查跳过项；Redis 和真实供应商需单独配置。接口、字段或行为改变后同步本说明及相关功能文档。
