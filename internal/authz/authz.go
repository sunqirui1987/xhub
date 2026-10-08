// Package authz is the single authorization layer for the gateway. Every
// management, listing, aggregate and inference decision is made here from the
// live database relationships; no other package decides who may see what.
//
// The model is deliberately small: one platform role pair (admin/user) plus a
// team role pair (team_admin/member), and a single membership table. What the
// caller may do follows from the actor kind, the actor's platform role, and the
// actor's role inside the team that owns the object.
package authz

import (
	"context"
	"errors"
	"sort"

	"github.com/sunqirui1987/xhub/internal/iam"
	"github.com/sunqirui1987/xhub/internal/logx"
)

// Sentinel outcomes. Callers map them onto HTTP: unauthenticated is 401,
// forbidden is 403, not found is 404. An internal error is a 500 and the
// request must be refused, never guessed at.
var (
	ErrUnauthenticated = errors.New("authz: unauthenticated")
	ErrForbidden       = errors.New("authz: forbidden")
	ErrNotFound        = errors.New("authz: not found")
)

// InternalError reports a failed dependency read. Authorization fails closed:
// when the ownership of an object cannot be read, nothing is permitted.
type InternalError struct{ Err error }

// 实现 error 接口，返回写进日志或 HTTP 错误体的文本。
// 参数：无。
// 返回 string（string）：error 接口的文本，给日志和 HTTP 错误体使用。
// 调用：auth/auth.go、authz/decide.go、authz/scope.go、catalog/embed.go
// 测试：无直接单测
func (e *InternalError) Error() string { return "authz: dependency read failed: " + e.Err.Error() }

// 交出被包装的底层错误，供 errors.Is 和 errors.As 识别具体类型。
// 参数：无。
// 返回 error（error）：失败原因，nil 表示这一步成功。
// 调用：auth/auth.go、gateway/engine.go
// 测试：无直接单测
func (e *InternalError) Unwrap() error { return e.Err }

// IsInternal reports a dependency failure, which callers answer with 500.
// 参数 err（error）：失败原因，nil 表示这一步成功。
// 返回 bool（bool）：错误是鉴权依赖读取失败时为真，用来和未找到区分开。
// 调用：gateway/session.go
// 测试：authz_test.go
func IsInternal(err error) bool {
	var target *InternalError
	return errors.As(err, &target)
}

// IsNotFound reports an object that does not exist or is not visible to the actor. The two cases are deliberately indistinguishable.
// 参数 err（error）：失败原因，nil 表示这一步成功。
// 返回 bool（bool）：对象不存在，或存在但当前调用方看不见时返回真。这两种情况故意无法区分。
// 调用：gateway/session.go、gateway/usage/entity_activity.go
// 测试：activity_test.go、authz_test.go
func IsNotFound(err error) bool { return errors.Is(err, ErrNotFound) }

// IsForbidden reports a visible object the actor may not act on.
// 参数 err（error）：失败原因，nil 表示这一步成功。
// 返回 bool（bool）：对象可见，但当前调用方不能做这个动作时返回真。
// 调用：gateway/session.go
// 测试：authz_test.go
func IsForbidden(err error) bool { return errors.Is(err, ErrForbidden) }

// IsUnauthenticated reports a missing or unusable identity.
// 参数 err（error）：失败原因，nil 表示这一步成功。
// 返回 bool（bool）：没有身份，或身份不能使用时返回真。
// 调用：gateway/session.go
// 测试：authz_test.go
func IsUnauthenticated(err error) bool { return errors.Is(err, ErrUnauthenticated) }

// Kind is how the caller proved who they are.
type Kind string

const (
	// KindSession is a signed-in UI session.
	KindSession Kind = "session"
	// KindKey is a virtual API key. It may infer and read itself and nothing else.
	KindKey Kind = "key"
	// KindMaster is the emergency master credential. It may only reach the
	// bootstrap and emergency routes, which the router keeps narrow.
	KindMaster Kind = "master"
)

// Actor is the authenticated caller.
type Actor struct {
	Kind   Kind
	UserID string
	Role   string // iam.RoleAdmin or iam.RoleUser, for a session
	KeyID  string
	// KeyTeamID is the single team a key is bound to. Capabilities are never
	// merged across teams.
	KeyTeamID    string
	KeyProjectID string
	OwnerType    string // iam.OwnerPersonal or iam.OwnerService
}

// ObjectType is the kind of thing an action is requested against.
type ObjectType string

const (
	ObjectUser    ObjectType = "user"
	ObjectOrg     ObjectType = "organization"
	ObjectTeam    ObjectType = "team"
	ObjectMember  ObjectType = "team_member"
	ObjectProject ObjectType = "project"
	ObjectKey     ObjectType = "key"
	// ObjectRouteTemplate is one named router settings document. Its OrgID and
	// TeamID are the template's ownership, not a requester's scope: both empty
	// means a platform template, which everyone may read and only a platform
	// administrator may write.
	ObjectRouteTemplate ObjectType = "route_template"
	ObjectUsage         ObjectType = "usage"
	ObjectLog           ObjectType = "log"
	ObjectAudit         ObjectType = "audit"
	ObjectBudget        ObjectType = "budget"
	ObjectModel         ObjectType = "model"
)

// Object names the target of an action. Callers fill in what they know and the
// Guard reads the rest from the database, so a handler can never pass a
// self-declared team or owner that disagrees with the stored row.
type Object struct {
	Type ObjectType
	ID   string
	// OrgID, TeamID and ProjectID are the ownership path. They are resolved
	// from the database whenever ID is set.
	OrgID     string
	TeamID    string
	ProjectID string
	// OwnerUserID and OwnerType describe a person-owned row, such as a personal
	// key or one request log.
	OwnerUserID string
	OwnerType   string
	// Status, TeamStatus and OrgStatus are the lifecycle statuses of the object
	// and of its owning team and organization, resolved from the database.
	Status     string
	TeamStatus string
	OrgStatus  string
	// KeyStatus is the lifecycle status of a key, for the key actions.
	KeyStatus string
}

// blocked reports an object that is itself blocked or sits under a blocked team or organization. Nothing but a platform administrator may act on it, which is what makes a block take effect on the very next request.
// 参数：无。
// 返回 bool（bool）：对象本身被封禁，或它所属的团队、组织被封禁时返回真。除平台管理员外都不能操作。
// 调用：authz/decide.go
// 测试：无直接单测
func (o Object) blocked() bool {
	return o.Status == iam.StatusBlocked ||
		o.TeamStatus == iam.StatusBlocked ||
		o.OrgStatus == iam.StatusBlocked
}

// inactive reports a revoked key or a disabled account.
// 参数：无。
// 返回 bool（bool）：密钥已吊销或账号已停用时返回真。
// 调用：仅在 authz.go 内使用
// 测试：无直接单测
func (o Object) inactive() bool {
	return o.KeyStatus == iam.StatusRevoked || o.Status == iam.StatusDisabled
}

// resolvable reports whether the object names a database row whose ownership can be read. A create carries only hints — the team it will land in, the owner it is meant for — and must not be looked up, because the row it describes does not exist yet.
// 参数：无。
// 返回 bool（bool）：这个对象已经有数据库行、可以读取归属时返回真。创建请求只有意向，不能拿去查库。
// 调用：authz/decide.go
// 测试：无直接单测
func (o Object) resolvable() bool {
	return o.ID != "" && o.Type != ObjectUsage && o.Type != ObjectLog &&
		o.Type != ObjectAudit && o.Type != ObjectModel &&
		// A route template carries its ownership on the object already: the
		// handler reads the row to get it, because the caller named a template id
		// and the ownership is the thing being decided. Re-resolving it here
		// would mean a second read of the same row and a second place the
		// mapping from columns to object fields is written.
		o.Type != ObjectRouteTemplate
}

// Authorizer holds the database the decisions read from.
type Authorizer struct{ db *iam.DB }

// New returns an Authorizer over the given store.
// 参数 db（*iam.DB）：新使用的身份库。
// 返回 *Authorizer（*Authorizer）：包住这个库的鉴权器。传入 nil 时库也是 nil，鉴权器本身不会是 nil。
// 调用：authz/decide.go、cache/cache.go、dataplane/serve.go、gateway/engine.go
// 测试：activity_http_test.go、authz_test.go、builtin_providers_test.go
func New(db *iam.DB) *Authorizer { return &Authorizer{db: db} }

// DB exposes the store so handlers can run the query the Guard authorized.
// 参数：无。
// 返回 *iam.DB（*iam.DB）：鉴权通过之后，处理函数用来查询的身份库。这不是用量刷写；构造时传入 nil，这里也是 nil。
// 调用：store/engine.go
// 测试：无直接单测
func (z *Authorizer) DB() *iam.DB { return z.db }

// Guard is the per-request authorization context. It loads the actor's team
// memberships once and answers every check in the request from that snapshot,
// so a handler that makes several checks does not re-read the same rows. It is
// never reused across requests and never cached.
type Guard struct {
	z     *Authorizer
	actor Actor
	roles map[string]string // team ID -> team role
	orgs  map[string]bool   // organization ID -> member of one of its teams
	// adminOrgs is the organizations this actor administers. Kept apart from
	// orgs because the two answer different questions: reaching an organization
	// through a team is not administering it.
	adminOrgs map[string]bool
	// resolved memoizes ownership lookups made during this request.
	resolved map[string]Object
}

// Guard loads the actor's memberships and returns the request's Guard.
// 参数 ctx（context.Context）：上下文，取消时停止；a（Actor）：执行这次修改的操作者。审计日志记这个人的 id 和角色。
// 返回 *Guard（*Guard）：装好成员关系之后的判定器。载入成员失败时为 nil；error（error）：成员关系读失败。nil 表示可以开始判定。
// 调用：gateway/session.go
// 测试：authz_test.go
func (z *Authorizer) Guard(ctx context.Context, a Actor) (*Guard, error) {
	g := &Guard{z: z, actor: a, roles: map[string]string{}, orgs: map[string]bool{},
		adminOrgs: map[string]bool{}, resolved: map[string]Object{}}
	if err := g.load(ctx); err != nil {
		return nil, err
	}
	return g, nil
}

// load reads the membership list exactly once. A failed read is fatal to the request: without memberships every scoped answer would be wrong.
// 参数 ctx（context.Context）：上下文，取消或超时时停止后续工作。
// 返回 error（error）：失败原因，nil 表示这一步成功。
// 调用：仅在 authz.go 内使用
// 测试：无直接单测
func (g *Guard) load(ctx context.Context) error {
	if g.z == nil || g.z.db == nil {
		return nil
	}
	if g.actor.Kind != KindSession || g.actor.UserID == "" {
		return nil
	}
	rows, err := g.z.db.MemberTeams(ctx, g.actor.UserID)
	if err != nil {
		logx.Error("authz membership read failed user=%s err=%v", g.actor.UserID, err)
		return &InternalError{Err: err}
	}
	for _, m := range rows {
		g.roles[m.TeamID] = m.Role
		if m.OrganizationID != "" {
			g.orgs[m.OrganizationID] = true
		}
	}
	// A failed read here is fatal for the same reason the team read is: without
	// it an organization administrator would silently lose their reach, and the
	// answer would be "denied" rather than "unknown".
	adminOrgs, err := g.z.db.AdminOrgs(ctx, g.actor.UserID)
	if err != nil {
		logx.Error("authz org-admin read failed user=%s err=%v", g.actor.UserID, err)
		return &InternalError{Err: err}
	}
	for _, id := range adminOrgs {
		g.adminOrgs[id] = true
	}
	logx.Trace("authz memberships user=%s teams=%d admin_orgs=%d",
		g.actor.UserID, len(g.roles), len(g.adminOrgs))
	return nil
}

// Actor returns the actor this Guard was built for.
// 参数：无。
// 返回 Actor（Actor）：构造这个 Guard 时的操作者。
// 调用：auth/auth.go、gateway/session.go
// 测试：无直接单测
func (g *Guard) Actor() Actor { return g.actor }

// PlatformAdmin reports an active platform administrator session.
// 参数：无。
// 返回 bool（bool）：当前会话是平台管理员时返回真。密钥不会因为主人是管理员而变成管理凭证。
// 调用：auth/auth.go、authz/decide.go、gateway/identity/handlers.go、gateway/models/list.go
// 测试：无直接单测
func (g *Guard) PlatformAdmin() bool {
	return g.actor.Kind == KindSession && g.actor.Role == iam.RoleAdmin
}

// TeamRole returns the actor's role in a team, or "" when not a member.
// 参数 teamID（string）：团队 id。空串表示没有指定团队。
// 返回 string（string）：操作者在这个团队里的角色。不是成员时为空串。
// 调用：仅在 authz.go 内使用
// 测试：无直接单测
func (g *Guard) TeamRole(teamID string) string { return g.roles[teamID] }

// InTeam reports membership of a team.
// 参数 teamID（string）：团队 id。空串表示没有指定团队。
// 返回 bool（bool）：操作者是这个团队的成员时返回真。
// 调用：authz/decide.go
// 测试：无直接单测
func (g *Guard) InTeam(teamID string) bool { return teamID != "" && g.roles[teamID] != "" }

// TeamRoles lists the actor's teams with the role held in each, sorted by team id. It is what /auth/me reports so the console knows which teams it may administer without re-deriving the rule itself.
// 参数：无。
// 返回 []map[string]string（[]map[string]string）：一组map[string]string。没有匹配时为空切片，不是 nil 分页。
// 调用：gateway/session.go
// 测试：无直接单测
func (g *Guard) TeamRoles() []map[string]string {
	out := make([]map[string]string, 0, len(g.roles))
	for id, role := range g.roles {
		out = append(out, map[string]string{"team_id": id, "role": role})
	}
	sort.Slice(out, func(i, j int) bool { return out[i]["team_id"] < out[j]["team_id"] })
	return out
}

// TeamAdminOf reports team administration inside one team.
// 参数 teamID（string）：团队 id。空串表示没有指定团队。
// 返回 bool（bool）：操作者是这一个团队的管理员时返回真。
// 调用：authz/decide.go
// 测试：无直接单测
func (g *Guard) TeamAdminOf(teamID string) bool {
	return teamID != "" && g.roles[teamID] == iam.TeamAdmin
}

// InOrg reports membership of at least one team of an organization.
// 参数 orgID（string）：组织 id。空串表示不按组织过滤。
// 返回 bool（bool）：操作者至少加入了这个组织里的一个团队时返回真。
// 调用：authz/decide.go
// 测试：无直接单测
func (g *Guard) InOrg(orgID string) bool { return orgID != "" && g.orgs[orgID] }

// OrgAdminOf reports whether this actor administers one organization. It is distinct from PlatformAdmin, which reaches every organization: this one is true for exactly the organizations the actor was granted.
// 参数 orgID（string）：组织 id。空串表示不按组织过滤。
// 返回 bool（bool）：操作者管理这一个组织时返回真。这和能看所有组织的平台管理员不是一回事。
// 调用：authz/decide.go
// 测试：无直接单测
func (g *Guard) OrgAdminOf(orgID string) bool { return orgID != "" && g.adminOrgs[orgID] }

// AdminOrgIDs lists the organizations this actor administers, sorted. It is what the console reads to offer organization management, and it is the same set the decision matrix consults.
// 参数：无。
// 返回 []string（[]string）：管理员组织标识列表。没有匹配时为空切片。
// 调用：gateway/session.go
// 测试：无直接单测
func (g *Guard) AdminOrgIDs() []string {
	out := make([]string, 0, len(g.adminOrgs))
	for id := range g.adminOrgs {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// VisibleTeamIDs lists the teams the actor belongs to, sorted. Only a platform administrator sees every team, and that is expressed by returning nil rather than a list, matching the store's "nil means all" convention. The master credential is deliberately not in that group, even though it is the most privileged string in the config. It reaches the bootstrap and emergency routes only, so it belongs to no team: an empty, non-nil slice means "no team", and every caller turns that into an empty result. Treating it as "every team" is what let GET /organization/list and POST /project/list answer withthe whole tenant inventory, because those listings computed their scope from a principal that carries no user ID, and the store reads an empty ID as "no filter". KeysScope, UsageScope and LogsScope already refuse the master explicitly; this is the same rule, at the one helper the listings share. A caller that feeds the result to a query must treat nil as "every team" and an empty, non-nil slice as
//
//	"no team": the two are different answers and the store enforces that distinction. ApplyTeam does this, and it is the reason a member with no teams gets an empty result rather than the whole table.
//
// 参数：无。
// 返回 []string（[]string）：可见团队标识列表。没有匹配时为 nil 或空切片，调用方按长度判断。
// 调用：authz/decide.go
// 测试：authz_test.go
func (g *Guard) VisibleTeamIDs() []string {
	if g.actor.Kind == KindMaster {
		return []string{}
	}
	if g.PlatformAdmin() {
		return nil
	}
	out := make([]string, 0, len(g.roles))
	for id := range g.roles {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// ManagedTeamIDs lists the teams the actor administers, sorted.
// 参数：无。
// 返回 []string（[]string）：Managed团队标识列表。没有匹配时为空切片。
// 调用：authz/decide.go
// 测试：无直接单测
func (g *Guard) ManagedTeamIDs() []string {
	out := make([]string, 0, len(g.roles))
	for id, role := range g.roles {
		if role == iam.TeamAdmin {
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out
}

// Capability names what the caller may do, as reported by /auth/me and used by
// the UI to decide which pages and buttons exist. The UI presents; the server
// still decides.
const (
	CapProfile         = "profile.self"
	CapPersonalKeys    = "keys.personal"
	CapSelfUsage       = "usage.self"
	CapSelfLogs        = "logs.self"
	CapInfer           = "infer"
	CapTeamRead        = "teams.read"
	CapTeamManage      = "teams.manage"
	CapMembers         = "members.manage"
	CapMemberPassword  = "members.password"
	CapProjects        = "projects.manage"
	CapServiceKeys     = "keys.service"
	CapUsageTeam       = "usage.team"
	CapUsageTeamDetail = "usage.team.detail"
	CapPlatformAdmin   = "platform.admin"
	CapUsers           = "users.manage"
	CapOrgs            = "orgs.manage"
	CapTeamsPlatform   = "teams.platform"
	CapUsageGlobal     = "usage.global"
	CapAudit           = "audit.read"
)

// Capabilities lists what this actor may do, sorted. It is derived from the same roles the decision matrix reads, so the UI cannot drift from the server.
// 参数：无。
// 返回 []string（[]string）：Capabilities。没有匹配时为空切片。
// 调用：gateway/session.go
// 测试：无直接单测
func (g *Guard) Capabilities() []string {
	set := map[string]bool{
		CapProfile:      true,
		CapPersonalKeys: true,
		CapSelfUsage:    true,
		CapSelfLogs:     true,
		CapInfer:        true,
	}
	if len(g.roles) > 0 {
		set[CapTeamRead] = true
		set[CapUsageTeam] = true
	}
	if len(g.ManagedTeamIDs()) > 0 {
		set[CapTeamManage] = true
		set[CapMembers] = true
		set[CapMemberPassword] = true
		set[CapProjects] = true
		set[CapServiceKeys] = true
		set[CapUsageTeamDetail] = true
	}
	if len(g.AdminOrgIDs()) > 0 {
		// Organization administration reaches the org's teams, members and
		// projects. It does not reach service keys or another member's usage
		// breakdown; those stay with the team administrator.
		set[CapTeamRead] = true
		set[CapTeamManage] = true
		set[CapMembers] = true
		set[CapMemberPassword] = true
		set[CapProjects] = true
	}
	if g.PlatformAdmin() {
		for _, c := range []string{CapPlatformAdmin, CapUsers, CapOrgs, CapTeamsPlatform,
			CapUsageGlobal, CapAudit} {
			set[c] = true
		}
		// A platform administrator reaches every team, and does so without
		// being a member of any: the decision matrix already lets them read and
		// write any team, so the reported capabilities have to say the same or
		// the console hides pages the server would serve. Omitting these left a
		// deployment's own administrator unable to open the teams page.
		for _, c := range []string{CapTeamRead, CapUsageTeam, CapTeamManage, CapMembers, CapMemberPassword,
			CapProjects, CapServiceKeys, CapUsageTeamDetail} {
			set[c] = true
		}
	}
	out := make([]string, 0, len(set))
	for c := range set {
		out = append(out, c)
	}
	sort.Strings(out)
	return out
}
