# 动作授权与查询范围

[English](readme.md) · [全功能实现说明](../../docs/development/implementation.md)

## 职责与实现契约

authz.go 定义 Actor、Action、Object、Guard 和 Authorizer，decide.go 实现角色与对象决策，scope.go 把授权结果应用到查询。Authorize(ctx, guard, action, object) 在 guard 为空时拒绝；资源 ID 并不自动等于资源归属，需按对象类型加载可信数据库信息。
平台管理员、组织管理员、团队管理员、普通成员与服务密钥拥有不同动作集合。密钥只能推理和读取允许范围的自身信息、用量及日志；主密钥的动作集合很窄。被禁用对象通常拒绝，只允许平台管理员执行相应解禁操作。
列表授权在 SQL 查询之前生成 Scope。无合法范围拒绝查询，All 才明确表示全局，不把空团队列表解释为全平台。用户提供的筛选只能收窄已授权范围。特权读取个人日志正文要写入持久审计；审计写失败拒绝读取。

## 源码职责与入口

### authz.go

公开类型：`InternalError`, `Kind`, `Actor`, `ObjectType`, `Object`, `Authorizer`, `Guard`.

- [`func (e *InternalError) Error() string`](authz.go) — 实现 error 接口，返回写进日志或 HTTP 错误体的文本。
- [`func (e *InternalError) Unwrap() error`](authz.go) — 交出被包装的底层错误，供 errors.Is 和 errors.As 识别具体类型。
- [`func IsInternal(err error) bool`](authz.go) — IsInternal reports a dependency failure, which callers answer with 500.
- [`func IsNotFound(err error) bool`](authz.go) — IsNotFound reports an object that does not exist or is not visible to the actor. The two cases are deliberately indistinguishable.
- [`func IsForbidden(err error) bool`](authz.go) — IsForbidden reports a visible object the actor may not act on.
- [`func IsUnauthenticated(err error) bool`](authz.go) — IsUnauthenticated reports a missing or unusable identity.
- [`func New(db *iam.DB) *Authorizer`](authz.go) — New returns an Authorizer over the given store.
- [`func (z *Authorizer) DB() *iam.DB`](authz.go) — DB exposes the store so handlers can run the query the Guard authorized.
- [`func (z *Authorizer) Guard(ctx context.Context, a Actor) (*Guard, error)`](authz.go) — Guard loads the actor's memberships and returns the request's Guard.
- [`func (g *Guard) Actor() Actor`](authz.go) — Actor returns the actor this Guard was built for.
- [`func (g *Guard) PlatformAdmin() bool`](authz.go) — PlatformAdmin reports an active platform administrator session.
- [`func (g *Guard) TeamRole(teamID string) string`](authz.go) — TeamRole returns the actor's role in a team, or "" when not a member.
- [`func (g *Guard) InTeam(teamID string) bool`](authz.go) — InTeam reports membership of a team.
- [`func (g *Guard) TeamRoles() []map[string]string`](authz.go) — TeamRoles lists the actor's teams with the role held in each, sorted by team id. It is what /auth/me reports so the console knows which teams it may administer without re-deriving the rule itself.
- [`func (g *Guard) TeamAdminOf(teamID string) bool`](authz.go) — TeamAdminOf reports team administration inside one team.
- [`func (g *Guard) InOrg(orgID string) bool`](authz.go) — InOrg reports membership of at least one team of an organization.
- [`func (g *Guard) OrgAdminOf(orgID string) bool`](authz.go) — OrgAdminOf reports whether this actor administers one organization. It is distinct from PlatformAdmin, which reaches every organization: this one is true for exactly the organizations the actor was granted.
- [`func (g *Guard) AdminOrgIDs() []string`](authz.go) — AdminOrgIDs lists the organizations this actor administers, sorted. It is what the console reads to offer organization management, and it is the same set the decision matrix consults.
- [`func (g *Guard) VisibleTeamIDs() []string`](authz.go) — VisibleTeamIDs lists the teams the actor belongs to, sorted. Only a platform administrator sees every team, and that is expressed by returning nil rather than a list, matching the store's "nil means all" convention. The master credential is deliberately not in that group, even though it is the most privileged string in the config. It reaches the bootstrap and emergency routes only, so it belongs to no team: an empty, non-nil slice means "no team", and every caller turns that into an empty result. Treating it as "every team" is what let GET /organization/list and POST /project/list answer withthe whole tenant inventory, because those listings computed their scope from a principal that carries no user ID, and the store reads an empty ID as "no filter". KeysScope, UsageScope and LogsScope alr
- [`func (g *Guard) ManagedTeamIDs() []string`](authz.go) — ManagedTeamIDs lists the teams the actor administers, sorted.
- [`func (g *Guard) Capabilities() []string`](authz.go) — Capabilities lists what this actor may do, sorted. It is derived from the same roles the decision matrix reads, so the UI cannot drift from the server.

### decide.go

公开类型：`Action`, `Scope`, `ScopeKind`.

- [`func (z *Authorizer) Authorize(ctx context.Context, g *Guard, action Action, obj Object) error`](decide.go) — Authorize decides one action against one object. It returns nil to allow, or ErrUnauthenticated, Err Forbidden, ErrNotFound, or an *InternalError. Ownership is read from the database, never from the caller: an object whose ID is given has its org/team/project and owner resolved here, so a handler cannot widen its own reach by passing a team or owner that is not the stored one.
- [`func (g *Guard) CheckKey(ctx context.Context) error`](decide.go) — CheckKey re-reads the live state behind a virtual key: the key row itself, the team and project it is bound to, and, for a personal key, that its owner is still an active account and still a member of the team. Authentication calls this too so there is exactly one definition of "this key still works".
- [`func (g *Guard) KeysScope(ctx context.Context) (*Scope, error)`](decide.go) — KeysScope returns the key-listing scope for an actor.
- [`func (g *Guard) UsageScope(ctx context.Context, teamID string) (*Scope, error)`](decide.go) — UsageScope returns the usage-listing scope for an actor. teamID optionally narrows to one team, and the guard checks the actor may see it at all.
- [`func (g *Guard) LogsScope(ctx context.Context) (*Scope, error)`](decide.go) — LogsScope returns the request-log scope for an actor: own personal logs, plus the service-key logs of the teams the actor administers. A platform administrator sees everything.
- [`func (g *Guard) TeamFilter() []string`](decide.go) — TeamFilter returns the team-ID filter for team-scoped listings such as projects and members. A nil slice means "every team", which only a platform administrator receives; an empty slice means "no team ", which the store turns into an empty result rather than an unfiltered query.

### scope.go

- [`func (s *Scope) Apply(session *xorm.Session) *xorm.Session`](scope.go) — Apply 把鉴权范围收成 SQL 条件，接到当前查询上。范围为空时条件恒为假，避免查出全部行。
- [`func (s *Scope) ApplyTeam(session *xorm.Session, column string) *xorm.Session`](scope.go) — ApplyTeam narrows a session by a column holding team IDs, using the resolved team list rather than a stored condition. It is used by listings whose team column is not the one the scope's condition names.
- [`func (s *Scope) ApplyUser(session *xorm.Session, column string) *xorm.Session`](scope.go) — ApplyUser narrows a session by an owner column, failing closed when the scope carries no owner.

## 对外 HTTP 边界

无本目录直接登记的 HTTP 路由。导出的 Go API 由上层调用；运行时目录调度或调用宿主的入口应沿依赖链追踪。

## 依赖关系

[internal/iam](../iam/readme_cn.md), [internal/logx](../logx/readme_cn.md).

## 验证与维护入口

| 测试文件 | 场景入口 |
| --- | --- |
| [authz_test.go](authz_test.go) | `TestSessionMatrix`, `TestKeyActorCannotManage`, `TestKeyRevalidatedEveryRequest`, `TestTeamAdminLogScopeExcludesOtherPersonalKeys`, `TestMasterIsNarrow`, `TestScopeFailsClosed`, `TestApplyTeamFailsClosed`, `TestTeamFilterNilMeansAllEmptyMeansNone`, `TestUnknownActorKindDenies` |

```bash
go test ./internal/authz -count=1
```

数据库验收设置 XHUB_REGRESSION_STRICT=1 并检查跳过项；Redis 和真实供应商需单独配置。接口、字段或行为改变后同步本说明及相关功能文档。
