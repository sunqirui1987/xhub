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

func (e *InternalError) Error() string { return "authz: dependency read failed: " + e.Err.Error() }
func (e *InternalError) Unwrap() error { return e.Err }

// IsInternal reports a dependency failure, which callers answer with 500.
func IsInternal(err error) bool {
	var target *InternalError
	return errors.As(err, &target)
}

// IsNotFound reports an object that does not exist or is not visible to the
// actor. The two cases are deliberately indistinguishable.
func IsNotFound(err error) bool { return errors.Is(err, ErrNotFound) }

// IsForbidden reports a visible object the actor may not act on.
func IsForbidden(err error) bool { return errors.Is(err, ErrForbidden) }

// IsUnauthenticated reports a missing or unusable identity.
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
	ObjectUsage   ObjectType = "usage"
	ObjectLog     ObjectType = "log"
	ObjectAudit   ObjectType = "audit"
	ObjectBudget  ObjectType = "budget"
	ObjectModel   ObjectType = "model"
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

// blocked reports an object that is itself blocked or sits under a blocked team
// or organization. Nothing but a platform administrator may act on it, which is
// what makes a block take effect on the very next request.
func (o Object) blocked() bool {
	return o.Status == iam.StatusBlocked ||
		o.TeamStatus == iam.StatusBlocked ||
		o.OrgStatus == iam.StatusBlocked
}

// inactive reports a revoked key or a disabled account.
func (o Object) inactive() bool {
	return o.KeyStatus == iam.StatusRevoked || o.Status == iam.StatusDisabled
}

// resolvable reports whether the object names a database row whose ownership
// can be read. A create carries only hints — the team it will land in, the
// owner it is meant for — and must not be looked up, because the row it
// describes does not exist yet.
func (o Object) resolvable() bool {
	return o.ID != "" && o.Type != ObjectUsage && o.Type != ObjectLog &&
		o.Type != ObjectAudit && o.Type != ObjectModel
}

// Authorizer holds the database the decisions read from.
type Authorizer struct{ db *iam.DB }

// New returns an Authorizer over the given store.
func New(db *iam.DB) *Authorizer { return &Authorizer{db: db} }

// DB exposes the store so handlers can run the query the Guard authorized.
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
func (z *Authorizer) Guard(ctx context.Context, a Actor) (*Guard, error) {
	g := &Guard{z: z, actor: a, roles: map[string]string{}, orgs: map[string]bool{},
		adminOrgs: map[string]bool{}, resolved: map[string]Object{}}
	if err := g.load(ctx); err != nil {
		return nil, err
	}
	return g, nil
}

// load reads the membership list exactly once. A failed read is fatal to the
// request: without memberships every scoped answer would be wrong.
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
func (g *Guard) Actor() Actor { return g.actor }

// PlatformAdmin reports an active platform administrator session.
func (g *Guard) PlatformAdmin() bool {
	return g.actor.Kind == KindSession && g.actor.Role == iam.RoleAdmin
}

// TeamRole returns the actor's role in a team, or "" when not a member.
func (g *Guard) TeamRole(teamID string) string { return g.roles[teamID] }

// InTeam reports membership of a team.
func (g *Guard) InTeam(teamID string) bool { return teamID != "" && g.roles[teamID] != "" }

// TeamRoles lists the actor's teams with the role held in each, sorted by team
// id. It is what /auth/me reports so the console knows which teams it may
// administer without re-deriving the rule itself.
func (g *Guard) TeamRoles() []map[string]string {
	out := make([]map[string]string, 0, len(g.roles))
	for id, role := range g.roles {
		out = append(out, map[string]string{"team_id": id, "role": role})
	}
	sort.Slice(out, func(i, j int) bool { return out[i]["team_id"] < out[j]["team_id"] })
	return out
}

// TeamAdminOf reports team administration inside one team.
func (g *Guard) TeamAdminOf(teamID string) bool {
	return teamID != "" && g.roles[teamID] == iam.TeamAdmin
}

// InOrg reports membership of at least one team of an organization.
func (g *Guard) InOrg(orgID string) bool { return orgID != "" && g.orgs[orgID] }

// OrgAdminOf reports whether this actor administers one organization.
//
// It is distinct from PlatformAdmin, which reaches every organization: this one
// is true for exactly the organizations the actor was granted.
func (g *Guard) OrgAdminOf(orgID string) bool { return orgID != "" && g.adminOrgs[orgID] }

// AdminOrgIDs lists the organizations this actor administers, sorted. It is
// what the console reads to offer organization management, and it is the same
// set the decision matrix consults.
func (g *Guard) AdminOrgIDs() []string {
	out := make([]string, 0, len(g.adminOrgs))
	for id := range g.adminOrgs {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// VisibleTeamIDs lists the teams the actor belongs to, sorted. Only a platform
// administrator sees every team, and that is expressed by returning nil rather
// than a list, matching the store's "nil means all" convention.
//
// The master credential is deliberately not in that group, even though it is the
// most privileged string in the config. It reaches the bootstrap and emergency
// routes only, so it belongs to no team: an empty, non-nil slice means "no
// team", and every caller turns that into an empty result. Treating it as "every
// team" is what let GET /organization/list and POST /project/list answer with
// the whole tenant inventory, because those listings computed their scope from a
// principal that carries no user ID, and the store reads an empty ID as "no
// filter". KeysScope, UsageScope and LogsScope already refuse the master
// explicitly; this is the same rule, at the one helper the listings share.
//
// A caller that feeds the result to a query must treat nil as "every team" and
// an empty, non-nil slice as "no team": the two are different answers and the
// store enforces that distinction. ApplyTeam does this, and it is the reason a
// member with no teams gets an empty result rather than the whole table.
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

// Capabilities lists what this actor may do, sorted. It is derived from the same
// roles the decision matrix reads, so the UI cannot drift from the server.
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
		for _, c := range []string{CapTeamRead, CapUsageTeam, CapTeamManage, CapMembers,
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
