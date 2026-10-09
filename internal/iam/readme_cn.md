# 身份、授权资源与事务账务

[English](readme.md) · [全功能实现说明](../../docs/development/implementation.md)

## 职责与实现契约

schema.sql 定义表与约束；db.go 建库连接并协调应用、会话和数据库时区；users/teams/keys 管理身份资源；route_template 管理模板；usage/usage_read 写入和查询用量。此包不导入网关或定义 HTTP 角色策略。
认证读取新鲜身份和成员状态。RecordUsage 以 request_id 幂等，在事务内更新事件、spend log、日汇总以及用户/密钥/团队/项目/组织五个视图。重复或并发重复结算不应新增费用；事务失败不留下部分累加。
事件保存调用时的归属、provider、模型及价格快照，之后删除密钥、迁移成员和更新价格不能改写历史。读取必须接收 authz 生成的范围，不能先全量读取再在内存过滤。测试使用独立 schema，生产身份和框架 KV 不应混用。

## 源码职责与入口

### db.go

公开类型：`DB`, `Actor`, `Audit`, `AuditFilter`.

- [`func Open(ctx context.Context, dsn string) (*DB, error)`](db.go) — Open connects with the pgx driver and applies the schema. The engine keeps xorm's default time zones — DatabaseTZ and TZLocation both Local — and they must agree with the session zone. xorm formats atime.Time into the database zone on the way in and re-labels the value it reads back into the application zone, while pgx returns the instant in the session zone. Pinning any one of the three to UTC while the others stayed Local made stored timestamps land eight hours off on a UTC+8 machine: created _at and updated_at disagreed with columns PostgreSQL filled in itself, because DEFAULT now() is computed by the server and never passes through this conversion. Setting all three together is what makes the round trip lossless. It matters that the session zone is named explicitly rather than left tot
- [`func (db *DB) Migrate(ctx context.Context) error`](db.go) — Migrate creates every table and constraint; idempotent on the new schema.
- [`func (db *DB) Close() error`](db.go) — Close releases the engine.
- [`func (db *DB) RecordAudit(ctx context.Context, a Actor, e Audit) error`](db.go) — RecordAudit writes an audit row outside a larger transaction, e.g. when a platform administrator reads someone else's request log.
- [`func (db *DB) ListAudit(ctx context.Context, limit, offset int) ([]AuditEntry, error)`](db.go) — ListAudit returns the newest rows first.
- [`func (db *DB) QueryAudit(ctx context.Context, filter AuditFilter, limit, offset int) ([]AuditEntry, int64, error)`](db.go) — QueryAudit returns matching rows and their total, newest first.

### audit_display.go

`enrichAudit` 在已授权、已分页的审计结果上补充操作者和对象的显示名。事件详情中保存的 `actor_name`、`actor_email`、`object_name` 快照优先于当前名称，保证改名后历史记录仍显示当时的事实；缺少快照的记录才按当前页批量查询用户、组织、团队、项目、密钥和路由模板。已删除对象没有可查名称时保持空值，不扩大查询范围。`applyAuditSearch` 在计数与分页之前，对事件编号、动作、身份、对象及名称应用相同的搜索条件，并用 `strpos` 将 `%` 和 `_` 当成普通字符，避免用户输入改变匹配语义。这两个函数是 `QueryAudit` 的内部实现，不新增 HTTP 入口。

### keys.go

公开类型：`Key`, `APIKey`, `KeyInput`, `KeyFilter`.

- [`func (APIKey) TableName() string`](keys.go) — 告诉 xorm 这个结构体对应数据库表 api_keys。
- [`func (k *Key) ActiveKey() bool`](keys.go) — ActiveKey reports a key that may be used: not blocked, not revoked, not expired.
- [`func (k *Key) OwnedBy(userID string) bool`](keys.go) — OwnedBy reports whether the key belongs to this person.
- [`func HashKey(plain string) string`](keys.go) — HashKey hashes a plaintext key for storage and lookup.
- [`func NewPlainKey() string`](keys.go) — NewPlainKey returns a new sk-... credential.
- [`func (db *DB) CreateKey(ctx context.Context, by Actor, in KeyInput) (*Key, string, error)`](keys.go) — CreateKey stores a key and returns it with the plaintext, which is shown once.
- [`func (db *DB) RotateKey(ctx context.Context, by Actor, id string) (*Key, string, error)`](keys.go) — RotateKey replaces the secret and returns the new plaintext once.
- [`func (db *DB) DeleteKey(ctx context.Context, by Actor, id string) error`](keys.go) — DeleteKey removes a key.
- [`func (db *DB) SetKeyStatus(ctx context.Context, by Actor, id, status string) (*Key, error)`](keys.go) — SetKeyStatus blocks or unblocks a key.
- [`func (db *DB) UpdateKey(ctx context.Context, by Actor, id string, in KeyInput) (*Key, error)`](keys.go) — UpdateKey changes the narrowing and limits of an existing key. Narrowing may never exceed what the team (and project) grants.
- [`func (db *DB) GetKey(ctx context.Context, id string) (*Key, error)`](keys.go) — GetKey loads one key with its narrowing.
- [`func (db *DB) KeyByHash(ctx context.Context, hash string) (*Key, error)`](keys.go) — KeyByHash loads a key by credential hash for authentication.
- [`func (db *DB) ListKeys(ctx context.Context, f KeyFilter) ([]Key, error)`](keys.go) — ListKeys returns keys inside the filter, newest first.
- [`func (db *DB) TouchKey(ctx context.Context, id string) error`](keys.go) — TouchKey records the last use of a key.
- [`func (db *DB) RevokeTeamKeys(ctx context.Context, by Actor, teamID string) error`](keys.go) — RevokeTeamKeys ends every key of a team; used when a team is blocked.
- [`func (db *DB) AllowedModelsForTeam(ctx context.Context, teamID string) ([]string, error)`](keys.go) — AllowedModelsForTeam is the team's model set, used before a key exists. A nil result means the team has no restriction of its own.
- [`func (db *DB) AllowedModelsForKey(ctx context.Context, k *Key) ([]string, error)`](keys.go) — AllowedModelsForKey is the key's effective model set; the catalog and the inference path both call it so a listed model is always a usable model.
- [`func KeyNameFromPlain(plain string) string`](keys.go) — KeyNameFromPlain derives a display name from a credential.
- [`func ParseExpiry(d string) (*time.Time, error)`](keys.go) — ParseExpiry turns a relative duration such as 30s, 30m, 30h or 30d into the absolute instant it names, counted from now. An empty value is "no expiry" and returns nil. Anything else is an error, so atypo never becomes a key that never expires.
- [`func (db *DB) ResetKeySpend(ctx context.Context, by Actor, id string, to float64) (*Key, error)`](keys.go) — ResetKeySpend sets a key's spend back to zero, or to the given value. The usage events behind the old figure stay, so history is never rewritten.

### models.go

公开类型：`Time`, `User`, `Organization`, `Team`, `TeamMembership`, `OrganizationMembership`, `Project`, `AuditEntry`.

- [`func (User) TableName() string`](models.go) — 告诉 xorm 这个结构体对应数据库表 users。
- [`func (u *User) Active() bool`](models.go) — Active reports an account that may sign in and use keys.
- [`func (u *User) Admin() bool`](models.go) — Admin reports an active platform administrator.
- [`func (Organization) TableName() string`](models.go) — 告诉 xorm 这个结构体对应数据库表 organizations。
- [`func (Team) TableName() string`](models.go) — 告诉 xorm 这个结构体对应数据库表 teams。
- [`func (TeamMembership) TableName() string`](models.go) — 告诉 xorm 这个结构体对应数据库表 team_members。
- [`func (OrganizationMembership) TableName() string`](models.go) — 告诉 xorm 这个结构体对应数据库表 organization_members。
- [`func (Project) TableName() string`](models.go) — 告诉 xorm 这个结构体对应数据库表 projects。
- [`func (AuditEntry) TableName() string`](models.go) — 告诉 xorm 这个结构体对应数据库表 audit_logs。
- [`func (bootstrapState) TableName() string`](models.go) — 告诉 xorm 这个结构体对应数据库表 bootstrap_state。

### route_template.go

公开类型：`TemplateOwner`, `RouteTemplate`, `TemplateUsage`.

- [`func (RouteTemplate) TableName() string`](route_template.go) — TableName tells xorm which table these rows live in.
- [`func (t RouteTemplate) Settings() map[string]any`](route_template.go) — Settings parses the body into a router settings document. A body that does not parse is reported as an empty document rather than failing the caller: the alternative is a single malformed row taking down every request that inherits it, and an empty document is exactly the platform behaviour.
- [`func (db *DB) ListRouteTemplates(ctx context.Context) ([]RouteTemplate, error)`](route_template.go) — ListRouteTemplates returns every template by name.
- [`func (db *DB) GetRouteTemplate(ctx context.Context, id string) (*RouteTemplate, error)`](route_template.go) — GetRouteTemplate loads one template by id.
- [`func (db *DB) CreateRouteTemplate(ctx context.Context, by Actor, owner TemplateOwner, name, body string) (*RouteTemplate, error)`](route_template.go) — CreateRouteTemplate inserts a template. The name must be free.
- [`func (db *DB) UpdateRouteTemplate(ctx context.Context, by Actor, id, name, body string) (*RouteTemplate, error)`](route_template.go) — UpdateRouteTemplate replaces a template's name and body.
- [`func (db *DB) DeleteRouteTemplate(ctx context.Context, by Actor, id string) error`](route_template.go) — DeleteRouteTemplate removes a template. Refusing while a scope still selects it is enforced by the caller, which reads the usage list first and reports where. Doing it here as well would put a user-facing explanation inside the store.
- [`func (db *DB) RouteTemplateUsage(ctx context.Context, id string) ([]TemplateUsage, error)`](route_template.go) — RouteTemplateUsage lists every scope currently selecting a template. It is three queries rather than one join because the three scopes live in three tables with no common parent beyond the id. Each is indexed on the column, so this is cheap even though it looks like a fan-out.
- [`func (db *DB) RouteTemplateUsageCounts(ctx context.Context) (map[string]int, error)`](route_template.go) — RouteTemplateUsageCounts returns how many scopes select each template, keyed by template id. The list screen shows the count, and computing it here keeps that screen from running one usage query per row.
- [`func (db *DB) SetScopeRouteTemplate(ctx context.Context, by Actor, scope, scopeID, templateID string) error`](route_template.go) — SetScopeRouteTemplate points one scope at a template, or clears it when id is empty. Clearing is what puts the scope back on inheritance. The scope is addressed as a table plus a column-safe id rather than through three separate functions, because the only difference between them is the table name and an id column that is spelled the same in all three.
- [`func (db *DB) ScopeRouteTemplate(ctx context.Context, scope, scopeID string) (string, error)`](route_template.go) — ScopeRouteTemplate reads one scope's selected template id. An empty string means the scope selects nothing and inherits.

### teams.go

公开类型：`OrgUpdate`, `TeamInput`, `TeamWithRole`, `Membership`, `Member`, `ProjectInput`.

- [`func (db *DB) CreateOrg(ctx context.Context, by Actor, name string, maxBudget *float64) (*Organization, error)`](teams.go) — CreateOrg inserts an organization.
- [`func (db *DB) OrgsByIDs(ctx context.Context, ids []string) (map[string]Organization, error)`](teams.go) — OrgsByIDs loads the organizations named by ids. An empty list is an empty map, not every organization.
- [`func (db *DB) GetOrg(ctx context.Context, id string) (*Organization, error)`](teams.go) — GetOrg loads one organization.
- [`func (db *DB) ListOrgs(ctx context.Context, userID string) ([]Organization, error)`](teams.go) — ListOrgs returns every organization, or only those that own one of the user's teams when userID is set.
- [`func (db *DB) UpdateOrg(ctx context.Context, by Actor, id string, in OrgUpdate) (*Organization, error)`](teams.go) — UpdateOrg applies an OrgUpdate.
- [`func (db *DB) DeleteOrg(ctx context.Context, by Actor, id string) error`](teams.go) — DeleteOrg refuses while teams remain (foreign key RESTRICT).
- [`func (db *DB) CreateTeam(ctx context.Context, by Actor, in TeamInput) (*Team, error)`](teams.go) — CreateTeam inserts the team and its first team_admin in one transaction.
- [`func (db *DB) TeamsByIDs(ctx context.Context, ids []string) (map[string]Team, error)`](teams.go) — TeamsByIDs loads the teams named by ids. An empty list is an empty map, not every team.
- [`func (db *DB) GetTeam(ctx context.Context, id string) (*Team, error)`](teams.go) — GetTeam loads one team.
- [`func (db *DB) ListTeams(ctx context.Context, userID, organizationID string) ([]TeamWithRole, error)`](teams.go) — ListTeams returns every team for a platform administrator (userID empty), or just the user's teams with their role.
- [`func (db *DB) UpdateTeamProfile(ctx context.Context, by Actor, id string, name, description *string) (*Team, error)`](teams.go) — UpdateTeamProfile is what a team administrator may change.
- [`func (db *DB) AdminUpdateTeam(ctx context.Context, by Actor, id string, status *string, maxBudget **float64, models *[]string) (*Team, error)`](teams.go) — AdminUpdateTeam changes status, budget and model set; platform administrators only. The model list sits at this level rather than at the team administrator's because it is the team's ceiling: a team administrator who could widen it would be granting their own team reach the platform did not give them.
- [`func (db *DB) MoveTeam(ctx context.Context, by Actor, id, organizationID string) (*Team, error)`](teams.go) — MoveTeam re-parents a team. Access-group assignments from the previous organization are removed first, which cascades to project and key narrowing; a failure leaves everything untouched.
- [`func (db *DB) DeleteTeam(ctx context.Context, by Actor, id string) error`](teams.go) — DeleteTeam cascades to members, projects, keys and access-group assignments.
- [`func (db *DB) MemberTeams(ctx context.Context, userID string) ([]Membership, error)`](teams.go) — MemberTeams returns the user's memberships with the team role and the team's organization. This is the only membership source the authorization layer reads: there is no mirror of members anywhere else
- [`func (db *DB) ListOrgAdmins(ctx context.Context, orgID string) ([]Member, error)`](teams.go) — ListOrgAdmins returns the organization administrators with public account fields.
- [`func (db *DB) AddOrgAdmin(ctx context.Context, by Actor, orgID, email string) (*Member, error)`](teams.go) — AddOrgAdmin grants organization administration to an existing active account, found by exact email. An unknown and a disabled account both return ErrNotFound. Granting it again is a no-op success.
- [`func (db *DB) RemoveOrgAdmin(ctx context.Context, by Actor, orgID, userID string) error`](teams.go) — RemoveOrgAdmin revokes organization administration. An organization may have no administrator: a platform administrator still reaches it.
- [`func (db *DB) ListVisibleTeams(ctx context.Context, userID, organizationID string) ([]TeamWithRole, error)`](teams.go) — ListVisibleTeams returns the teams a person may see: the ones they belong to, plus every team in an organization they administer. The role is their team role, empty when they administer the organization without joining the team.
- [`func (db *DB) OversightTeamIDs(ctx context.Context, userID string) ([]string, error)`](teams.go) — OversightTeamIDs lists the teams whose contents this person may see in full: teams they administer, and every team in an organization they administer. Membership alone does not put a team here.
- [`func (db *DB) TeamIDsByOrg(ctx context.Context, orgIDs []string) ([]string, error)`](teams.go) — TeamIDsByOrg lists every team that belongs to one of the organizations.
- [`func (db *DB) AdminOrgs(ctx context.Context, userID string) ([]string, error)`](teams.go) — AdminOrgs 列出这个用户担任管理员的组织 id。没有这种组织时为空切片。
- [`func (db *DB) SetOrgAdmin(ctx context.Context, by Actor, orgID, userID string, admin bool) error`](teams.go) — SetOrgAdmin grants or revokes organization administration for one person.
- [`func (db *DB) OrgAdmins(ctx context.Context, orgID string) ([]string, error)`](teams.go) — OrgAdmins lists the people who administer one organization.
- [`func (db *DB) Memberships(ctx context.Context, userID string) (map[string]string, error)`](teams.go) — Memberships returns team ID → team role, for callers that only need the role.
- [`func (db *DB) ListMembers(ctx context.Context, teamID string) ([]Member, error)`](teams.go) — ListMembers returns the team's members with public account fields only.
- [`func (db *DB) AddMember(ctx context.Context, by Actor, teamID, email, role string) (*Member, error)`](teams.go) — AddMember adds an existing active account by exact email. An unknown and a disabled account both return ErrNotFound, so callers cannot enumerate users.
- [`func (db *DB) SetMemberRole(ctx context.Context, by Actor, teamID, userID, role string) error`](teams.go) — SetMemberRole changes a team role, protecting the last team_admin.
- [`func (db *DB) RemoveMember(ctx context.Context, by Actor, teamID, userID string) error`](teams.go) — RemoveMember deletes a membership and revokes the user's personal keys bound to this team, protecting the last team_admin.
- [`func (db *DB) GetProject(ctx context.Context, id string) (*Project, error)`](teams.go) — GetProject loads one project with its organization derived from the team.
- [`func (db *DB) ListProjects(ctx context.Context, teamIDs []string) ([]Project, error)`](teams.go) — ListProjects returns the projects of the given teams, or of every team when teamIDs is nil (platform administrators). An empty, non-nil list returns nothing rather than everything.
- [`func (db *DB) CreateProject(ctx context.Context, by Actor, in ProjectInput) (*Project, error)`](teams.go) — CreateProject inserts a project narrowed within its team's grants.
- [`func (db *DB) UpdateProject(ctx context.Context, by Actor, id string, in ProjectInput) (*Project, error)`](teams.go) — UpdateProject replaces name, status, narrowing and budget.
- [`func (db *DB) DeleteProject(ctx context.Context, by Actor, id string) error`](teams.go) — DeleteProject cascades to the project's keys.

### usage.go

公开类型：`UsageEvent`, `RequestLog`, `UsageDaily`, `UsageRecord`.

- [`func (UsageEvent) TableName() string`](usage.go) — 告诉 xorm 这个结构体对应数据库表 usage_events。
- [`func (RequestLog) TableName() string`](usage.go) — 告诉 xorm 这个结构体对应数据库表 request_logs。
- [`func (UsageDaily) TableName() string`](usage.go) — 告诉 xorm 这个结构体对应数据库表 usage_daily。
- [`func (db *DB) RecordUsage(ctx context.Context, records []UsageRecord) error`](usage.go) — RecordUsage writes the event, its stored request/response, the daily roll-up and the live spend counters in one transaction. The batch is idempotent on request_id. A retried flush, or a second gateway process replaying the same Redis entry, inserts the event once and therefore increments everything else once. The existence check and the increments share the transaction so a partial replay cannot double-count.

### usage_read.go

公开类型：`UsageQuery`, `DailyRow`, `DailyByModelRow`, `KeySpendRow`, `TeamSpendRow`.

- [`func (db *DB) ListUsage(ctx context.Context, q UsageQuery) ([]UsageEvent, error)`](usage_read.go) — ListUsage returns usage events inside the scope, newest first. A missing limit is a page of logs. A caller that names a limit gets that many rows, capped here. Clamping an over-large limit down to a small page would make the usage screen report a sample as the whole window.
- [`func (db *DB) CountUsage(ctx context.Context, q UsageQuery) (int64, error)`](usage_read.go) — CountUsage counts the events inside the scope, for paging.
- [`func (db *DB) GetUsageEvent(ctx context.Context, q UsageQuery, requestID string) (*UsageEvent, error)`](usage_read.go) — GetUsageEvent loads one event inside the scope. A row outside the scope is reported as not found rather than forbidden, so a caller cannot probe which request ids exist.
- [`func (db *DB) GetRequestLog(ctx context.Context, requestID string) (*RequestLog, error)`](usage_read.go) — GetRequestLog loads the stored bodies of one event that is already inside the scope. The caller checks the scope with GetUsageEvent first.
- [`func (db *DB) DailyUsage(ctx context.Context, q UsageQuery, tzMinutes int) ([]DailyRow, error)`](usage_read.go) — DailyUsage totals the roll-up per day inside the scope.
- [`func (db *DB) DailyUsageByModel(ctx context.Context, q UsageQuery, tzMinutes int) ([]DailyByModelRow, error)`](usage_read.go) — DailyUsageByModel totals the roll-up per day and model inside the scope.
- [`func (db *DB) RollupByModel(ctx context.Context, q UsageQuery, tzMinutes int) ([]DailyByModelRow, error)`](usage_read.go) — RollupByModel totals the roll-up per model inside the scope, for the per-model table the usage page shows.
- [`func (db *DB) RollupByKey(ctx context.Context, q UsageQuery, tzMinutes int) ([]KeySpendRow, error)`](usage_read.go) — RollupByKey totals the roll-up per key inside the scope. The display name is a correlated subquery rather than a join, so the grouped table stays the only source of rows and the scope cannot widen orduplicate them. A key that was deleted since keeps its spend: the roll-up carries the id, and the name falls back to it.
- [`func (db *DB) RollupByTeam(ctx context.Context, q UsageQuery, tzMinutes int) ([]TeamSpendRow, error)`](usage_read.go) — RollupByTeam totals the roll-up per team inside the scope, with the same correlated-subquery shape as RollupByKey.
- [`func (db *DB) AuditLogRead(ctx context.Context, by Actor, requestID string, e UsageEvent) error`](usage_read.go) — AuditLogRead records that an actor read a log they do not own. The caller writes this before returning the row: the access is the event, not the row.

### users.go

公开类型：`UserInput`, `UserUpdate`.

- [`func (db *DB) Bootstrapped(ctx context.Context) (bool, error)`](users.go) — Bootstrapped reports whether the first platform administrator exists.
- [`func (db *DB) Bootstrap(ctx context.Context, email, name, password string) (*User, error)`](users.go) — Bootstrap creates the first platform administrator exactly once.
- [`func (db *DB) UserByEmail(ctx context.Context, email string) (*User, error)`](users.go) — UserByEmail loads one account by its address, case-insensitively.
- [`func (db *DB) EnsureAdmin(ctx context.Context, email, name, password string) (bool, error)`](users.go) — EnsureAdmin creates a platform administrator from configuration if no account with that address exists yet, and reports whether it created one. It deliberately never updates an existing row. The configured password is an initial password: an operator who changes the YAML after the first start expects the running account to keep the password it has, not to have it silently reset by a config file that may sit in a repository. Changing a live password is a write against the account, not a deployment side effect. An empty email or password is not an error here; it means the deployment has not configured seeding, and the caller falls back to POST /bootstrap.
- [`func (db *DB) CreateUser(ctx context.Context, by Actor, in UserInput) (*User, error)`](users.go) — CreateUser inserts a platform account together with the memberships the caller asked for, in one transaction. The memberships are written only after the account row exists, and a failure to write anyof them rolls the whole thing back: an account that came out of a failed create would be one nobody intended to make. The budget is stored as given; nil means no ceiling rather than zero, which is the difference between "unlimited" and "cannot spend anything".
- [`func (db *DB) UsersByIDs(ctx context.Context, ids []string) (map[string]User, error)`](users.go) — UsersByIDs loads the accounts named by ids. An empty list is an empty map, not every account. Callers use it to label rows they have already scoped.
- [`func (db *DB) GetUser(ctx context.Context, id string) (*User, error)`](users.go) — GetUser loads one account.
- [`func (db *DB) ListScopedUsers(ctx context.Context, self string, teamIDs []string, query string, limit, offset int) ([]User, error)`](users.go) — ListScopedUsers lists the caller and the accounts that belong to the teams they oversee. An empty team list returns only the caller.
- [`func (db *DB) ListUsers(ctx context.Context, query string, limit, offset int) ([]User, error)`](users.go) — ListUsers is the platform administrator's directory.
- [`func (db *DB) Login(ctx context.Context, email, password string) (*User, error)`](users.go) — Login checks a password. A disabled account and a wrong password both return ErrNotFound so callers cannot tell the cases apart. The identifier is an email, or, when it contains no @, the display name or the part of the address before @. The console's username field is free text, and the configured administrator is often typed as "admin" rather than "admin@xhub.local". A name that matches more than one account is refused, the same as a missing account, so the response still cannot enumerate users.
- [`func ConsoleRole(role string) string`](users.go) — ConsoleRole is the role name the admin UI understands. The row stores admin or user. The console only treats proxy_admin as a platform administrator, so a session that carries the stored spelling is shown as an unknown role and every admin-only query is narrowed to that one user.
- [`func StoreRole(role string) string`](users.go) — StoreRole maps a role the console sent back onto the stored spelling. An empty value is a normal user, which is what the account form sends when the picker is left on its default.
- [`func (db *DB) UpdateProfile(ctx context.Context, by Actor, id, name string) (*User, error)`](users.go) — UpdateProfile changes display fields only.
- [`func (db *DB) SetPassword(ctx context.Context, by Actor, id, password string) error`](users.go) — SetPassword replaces the password and ends existing sessions.
- [`func (db *DB) AdminUpdateUser(ctx context.Context, by Actor, id string, in UserUpdate) (*User, error)`](users.go) — AdminUpdateUser changes role, status, email or budget in one transaction. A role or status change ends the user's sessions; disabling also revokes every personal key.
- [`func (db *DB) DeleteUser(ctx context.Context, by Actor, id string) error`](users.go) — DeleteUser removes an account; memberships and personal keys cascade, but the delete is refused while the user is a team's last team_admin.

资源与持久化定义：[schema.sql](schema.sql).

## 对外 HTTP 边界

无本目录直接登记的 HTTP 路由。导出的 Go API 由上层调用；运行时目录调度或调用宿主的入口应沿依赖链追踪。

## 依赖关系

[internal/logx](../logx/readme_cn.md).

## 验证与维护入口


| 测试文件 | 场景入口 |
| --- | --- |
| [audit_test.go](audit_test.go) | TestQueryAuditFiltersBeforePagination、TestAuditNamesSurviveRenameAndDeletion：审计搜索先过滤再分页，历史名称在改名及删除后保留。 |
| [db_zone_test.go](db_zone_test.go) | `TestSessionZoneIsNamedOnTheConnection`, `TestServerZoneUsesThePosixSign`, `TestSessionZoneIsAppliedByTheServer`, `TestSessionZoneSurvivesAnUnparseableDSN` |
| [route_template_test.go](route_template_test.go) | `TestTemplateBodyRoundTripsAsRouterSettings`, `TestTemplateWithAnUnreadableBodyFallsBackToEmpty`, `TestTemplateNamesAreUnique`, `TestRenameKeepsTheTemplateUsable`, `TestScopeSelectionRoundTrips`, `TestBindingRefusesAnUnknownTemplate`, `TestBindingRefusesAnUnknownScopeName`, `TestUsageListsEveryScopeThatSelectsTheTemplate`, `TestDeletingATemplateInUseIsRefusedByTheCaller`, `TestDeletingAMissingTemplateReportsIt`, `TestListRouteTemplatesIsOrderedByName`, `TestADanglingTemplateIdIsRejectedByTheDatabase` |
| [usage_idempotency_test.go](usage_idempotency_test.go) | `TestRecordUsageRejectsInvalidBatchBeforeWriting`, `TestRecordUsageConcurrentSettlement`, `TestRecordUsageIsIdempotentOnRequestID`, `TestRecordUsageDropsDuplicatesWithinOneBatch`, `TestRecordUsageCountsDistinctRequests`, `TestRecordUsageRollsBackTheWholeBatch` |
| [users_test.go](users_test.go) | `TestEnsureAdminCreatesTheConfiguredAccount`, `TestConsoleRoleIsWhatTheAdminUIReads`, `TestEnsureAdminNeverRewritesAnExistingAccount`, `TestEnsureAdminIsANoOpWhenUnconfigured`, `TestEnsureAdminMarksThePlatformInitialised`, `TestEnsureAdminRejectsAShortPassword` |

```bash
go test ./internal/iam -count=1
```

数据库验收设置 XHUB_REGRESSION_STRICT=1 并检查跳过项；Redis 和真实供应商需单独配置。接口、字段或行为改变后同步本说明及相关功能文档。
