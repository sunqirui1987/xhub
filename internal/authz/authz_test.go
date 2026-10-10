package authz

import (
	"context"
	"strings"
	"testing"

	"github.com/sunqirui1987/xhub/cmd/regression/testsupport"
	"github.com/sunqirui1987/xhub/internal/iam"
	"xorm.io/builder"
)

// fixture exercises the role boundaries documented in docs/development/permissions.md:
// two organizations, two teams each, two projects each, and five accounts.
//
//	orgA: teamA1 (alice admin, plain member)    teamA2 (dave admin)
//	orgB: teamB1 (carol admin)                  teamB2
//
// 每个人仅属于一个团队；carol 管理 B 而不属于 A，用于验证跨组织权限隔离。
type fixture struct {
	db   *iam.DB
	auth *Authorizer

	admin *iam.User // platform administrator
	alice *iam.User // team_admin of teamA1
	carol *iam.User // team_admin of teamB1 only
	dave  *iam.User // team_admin of teamA2, member of nothing else
	plain *iam.User // member of teamA1 only
	gone  *iam.User // disabled account
	solo  *iam.User // active account in no team at all

	orgA, orgB                          *iam.Organization
	teamA1, teamA2, teamB1              string
	projA1, projB1                      *iam.Project
	aliceKey, aliceServiceKey, carolKey *iam.Key
	plainKey                            *iam.Key
}

// sys is the fixture's acting identity for writes made by the test itself. It is
// a real account because api_keys.created_by is a foreign key to users.
func (f *fixture) sys() iam.Actor { return iam.Actor{ID: f.admin.ID, Kind: "session"} }

func testDB(t *testing.T) *iam.DB {
	t.Helper()
	db, err := iam.Open(context.Background(), testsupport.Postgres(t, "authz"))
	if err != nil {
		t.Fatalf("open iam: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// newFixture 构造单团队授权矩阵；参数 t 为测试上下文，返回夹具，隔离 schema 自动清理。
// Carol 只属于 B1；A1 成员隐私由 plainKey 验证。
func newFixture(t *testing.T) *fixture {
	t.Helper()
	db := testDB(t)
	ctx := context.Background()

	// Bootstrap needs an actor, and api_keys.created_by is a foreign key to
	// users, so the fixture's first account is created by the empty identity and
	// every later write is attributed to that administrator.
	mk := func(email, name, role string) *iam.User {
		u, err := db.CreateUser(ctx, bootActor, iam.UserInput{Email: email, Name: name, Password: "password123", Role: role})
		if err != nil {
			t.Fatalf("create user %s: %v", email, err)
		}
		return u
	}

	f := &fixture{db: db, auth: New(db)}
	f.admin = mk("admin@example.com", "Admin", iam.RoleAdmin)
	f.alice = mk("alice@example.com", "Alice", iam.RoleUser)
	f.carol = mk("carol@example.com", "Carol", iam.RoleUser)
	f.dave = mk("dave@example.com", "Dave", iam.RoleUser)
	f.plain = mk("plain@example.com", "Plain", iam.RoleUser)
	f.gone = mk("gone@example.com", "Gone", iam.RoleUser)
	f.solo = mk("solo@example.com", "Solo", iam.RoleUser)
	sys := iam.Actor{ID: f.admin.ID, Kind: "session"}

	org := func(name string) *iam.Organization {
		o, err := db.CreateOrg(ctx, sys, name, nil)
		if err != nil {
			t.Fatalf("create org %s: %v", name, err)
		}
		return o
	}
	f.orgA, f.orgB = org("Org A"), org("Org B")

	team := func(orgID, name, adminID string) string {
		tm, err := db.CreateTeam(ctx, sys, iam.TeamInput{OrganizationID: orgID, Name: name, AdminUserID: adminID})
		if err != nil {
			t.Fatalf("create team %s: %v", name, err)
		}
		return tm.ID
	}
	f.teamA1 = team(f.orgA.ID, "Team A1", f.alice.ID)
	f.teamA2 = team(f.orgA.ID, "Team A2", f.dave.ID)
	// B1 belongs to carol outright. Leaving alice in it would have made her a
	// member of orgB, which is exactly the cross-organization reach the matrix
	// says she must not have.
	f.teamB1 = team(f.orgB.ID, "Team B1", f.carol.ID)
	_ = team(f.orgB.ID, "Team B2", "")

	join := func(teamID, userID, role string) {
		if _, err := db.AddMember(ctx, sys, teamID, emailOf(db, t, userID), role); err != nil {
			t.Fatalf("add member %s: %v", userID, err)
		}
	}
	join(f.teamA1, f.plain.ID, iam.TeamMember)
	// f.solo is deliberately left in no team at all.

	proj := func(teamID, name string) *iam.Project {
		p, err := db.CreateProject(ctx, sys, iam.ProjectInput{TeamID: teamID, Name: name})
		if err != nil {
			t.Fatalf("create project %s: %v", name, err)
		}
		return p
	}
	f.projA1 = proj(f.teamA1, "Proj A1")
	f.projB1 = proj(f.teamB1, "Proj B1")

	key := func(in iam.KeyInput) *iam.Key {
		k, _, err := db.CreateKey(ctx, sys, in)
		if err != nil {
			t.Fatalf("create key %s: %v", in.Name, err)
		}
		return k
	}
	f.aliceKey = key(iam.KeyInput{OwnerType: iam.OwnerPersonal, UserID: f.alice.ID, TeamID: f.teamA1, Name: "alice-personal"})
	f.aliceServiceKey = key(iam.KeyInput{OwnerType: iam.OwnerService, TeamID: f.teamA1, Name: "a1-service"})
	f.carolKey = key(iam.KeyInput{OwnerType: iam.OwnerPersonal, UserID: f.carol.ID, TeamID: f.teamB1, Name: "carol-personal"})
	f.plainKey = key(iam.KeyInput{OwnerType: iam.OwnerPersonal, UserID: f.plain.ID, TeamID: f.teamA1, Name: "plain-personal"})

	if _, err := db.AdminUpdateUser(ctx, sys, f.gone.ID, iam.UserUpdate{Status: ptr(iam.StatusDisabled)}); err != nil {
		t.Fatalf("disable gone: %v", err)
	}
	return f
}

func ptr[T any](v T) *T { return &v }

// bootActor is the empty identity the fixture uses to create the first account,
// before any account exists to attribute the write to.
var bootActor = iam.Actor{}

// TestTeamQuotaAndModelAdministrationScope 验证团队额度与模型/状态管理使用不同权限。
// 参数 t 为测试上下文，无返回；前置隔离数据库与四角色，覆盖本组织/跨组织/不存在团队的边界，fixture 自动清理数据。
func TestTeamQuotaAndModelAdministrationScope(t *testing.T) {
	f := newFixture(t)
	if err := f.db.SetOrgAdmin(t.Context(), f.sys(), f.orgA.ID, f.solo.ID, true); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name           string
		actor          *iam.User
		team           string
		budget, models string
	}{
		{"平台管理本组织", f.admin, f.teamA1, "allow", "allow"},
		{"平台管理其他组织", f.admin, f.teamB1, "allow", "allow"},
		{"组织分配本组织额度", f.solo, f.teamA1, "allow", "forbidden"},
		{"组织分配未加入的本组织团队", f.solo, f.teamA2, "allow", "forbidden"},
		{"组织不能跨组织修改", f.solo, f.teamB1, "forbidden", "forbidden"},
		{"团队不能自行提额", f.alice, f.teamA1, "forbidden", "forbidden"},
		{"普通成员不能自行提额", f.plain, f.teamA1, "forbidden", "forbidden"},
		{"不存在的团队", f.admin, "missing-team", "notfound", "notfound"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for action, want := range map[Action]string{ActionTeamBudget: tc.budget, ActionTeamModels: tc.models} {
				if got := f.decide(t, session(tc.actor), action, Object{Type: ObjectTeam, ID: tc.team}); got != want {
					t.Fatalf("团队权限 %s: got=%s want=%s", action, got, want)
				}
			}
		})
	}
}

func emailOf(db *iam.DB, t *testing.T, id string) string {
	t.Helper()
	u, err := db.GetUser(context.Background(), id)
	if err != nil {
		t.Fatalf("get user %s: %v", id, err)
	}
	return u.Email
}

// session builds the actor a signed-in person produces.
func session(u *iam.User) Actor {
	return Actor{Kind: KindSession, UserID: u.ID, Role: u.Role}
}

// guard builds the per-request guard for an actor.
func (f *fixture) guard(t *testing.T, a Actor) *Guard {
	t.Helper()
	g, err := f.auth.Guard(context.Background(), a)
	if err != nil {
		t.Fatalf("guard: %v", err)
	}
	return g
}

// decide is the whole assertion surface: it authorizes an action and reports the
// outcome as allow / notfound / forbidden / internal.
func (f *fixture) decide(t *testing.T, a Actor, action Action, obj Object) string {
	t.Helper()
	g := f.guard(t, a)
	err := f.auth.Authorize(context.Background(), g, action, obj)
	switch {
	case err == nil:
		return "allow"
	case IsNotFound(err):
		return "notfound"
	case IsForbidden(err):
		return "forbidden"
	case IsUnauthenticated(err):
		return "unauthenticated"
	case IsInternal(err):
		return "internal"
	default:
		t.Fatalf("unexpected error: %v", err)
		return ""
	}
}

// TestSessionMatrix walks the plan's role x resource x verb table. Every row is
// a decision the server must make the same way no matter which handler asks.
func TestSessionMatrix(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	for _, tc := range []struct {
		name   string
		actor  Actor
		action Action
		obj    Object
		want   string
	}{
		// ---- own account ----
		{"plain reads self", session(f.plain), ActionUserRead, Object{Type: ObjectUser, ID: f.plain.ID}, "allow"},
		{"plain writes self", session(f.plain), ActionUserWrite, Object{Type: ObjectUser, ID: f.plain.ID}, "allow"},
		{"plain reads a colleague", session(f.plain), ActionUserRead, Object{Type: ObjectUser, ID: f.alice.ID}, "notfound"},
		// A team administrator sees their colleagues through the member list,
		// which returns public fields; the account object itself is not theirs.
		{"team admin cannot read a colleague's account", session(f.alice), ActionUserRead, Object{Type: ObjectUser, ID: f.plain.ID}, "notfound"},
		{"team admin reads a foreign user", session(f.alice), ActionUserRead, Object{Type: ObjectUser, ID: f.dave.ID}, "notfound"},
		{"member cannot create users", session(f.alice), ActionUserCreate, Object{Type: ObjectUser}, "forbidden"},
		{"team admin cannot delete a user", session(f.alice), ActionUserDelete, Object{Type: ObjectUser, ID: f.plain.ID}, "forbidden"},
		{"admin creates users", session(f.admin), ActionUserCreate, Object{Type: ObjectUser}, "allow"},
		{"admin deletes users", session(f.admin), ActionUserDelete, Object{Type: ObjectUser, ID: f.plain.ID}, "allow"},

		// ---- organizations ----
		{"member reads own org", session(f.alice), ActionOrgRead, Object{Type: ObjectOrg, ID: f.orgA.ID}, "allow"},
		{"member cannot read the other org", session(f.alice), ActionOrgRead, Object{Type: ObjectOrg, ID: f.orgB.ID}, "notfound"},
		{"carol reaches her own org", session(f.carol), ActionOrgRead, Object{Type: ObjectOrg, ID: f.orgB.ID}, "allow"},
		{"member cannot write orgs", session(f.alice), ActionOrgWrite, Object{Type: ObjectOrg, ID: f.orgA.ID}, "forbidden"},
		{"team admin cannot write orgs", session(f.alice), ActionOrgWrite, Object{Type: ObjectOrg, ID: f.orgA.ID}, "forbidden"},
		{"admin writes orgs", session(f.admin), ActionOrgWrite, Object{Type: ObjectOrg, ID: f.orgA.ID}, "allow"},

		// ---- teams ----
		{"member reads own team", session(f.plain), ActionTeamRead, Object{Type: ObjectTeam, ID: f.teamA1}, "allow"},
		{"member cannot read a foreign team", session(f.plain), ActionTeamRead, Object{Type: ObjectTeam, ID: f.teamB1}, "notfound"},
		{"guessed team id is not found", session(f.plain), ActionTeamRead, Object{Type: ObjectTeam, ID: "does-not-exist"}, "notfound"},
		{"member cannot rename the team", session(f.plain), ActionTeamWrite, Object{Type: ObjectTeam, ID: f.teamA1}, "forbidden"},
		{"team admin renames own team", session(f.alice), ActionTeamWrite, Object{Type: ObjectTeam, ID: f.teamA1}, "allow"},
		{"carol administers B1 but not A1", session(f.carol), ActionTeamWrite, Object{Type: ObjectTeam, ID: f.teamB1}, "allow"},
		{"carol cannot rename A1", session(f.carol), ActionTeamWrite, Object{Type: ObjectTeam, ID: f.teamA1}, "forbidden"},
		{"team admin cannot delete a team", session(f.alice), ActionTeamDelete, Object{Type: ObjectTeam, ID: f.teamA1}, "forbidden"},
		{"team admin cannot set the team budget", session(f.alice), ActionTeamBudget, Object{Type: ObjectTeam, ID: f.teamA1}, "forbidden"},
		{"admin deletes a team", session(f.admin), ActionTeamDelete, Object{Type: ObjectTeam, ID: f.teamA1}, "allow"},
		{"admin moves a team", session(f.admin), ActionTeamMove, Object{Type: ObjectTeam, ID: f.teamA1}, "allow"},

		// ---- members ----
		{"member lists members", session(f.plain), ActionMemberRead, Object{Type: ObjectMember, TeamID: f.teamA1}, "allow"},
		{"member cannot see another team's members", session(f.plain), ActionMemberRead, Object{Type: ObjectMember, TeamID: f.teamB1}, "notfound"},
		{"member cannot add members", session(f.plain), ActionMemberWrite, Object{Type: ObjectMember, TeamID: f.teamA1}, "forbidden"},
		{"team admin adds a member", session(f.alice), ActionMemberWrite, Object{Type: ObjectMember, TeamID: f.teamA1}, "allow"},
		{"team admin cannot add to a foreign team", session(f.alice), ActionMemberWrite, Object{Type: ObjectMember, TeamID: f.teamB1}, "forbidden"},
		{"carol adds to B1", session(f.carol), ActionMemberWrite, Object{Type: ObjectMember, TeamID: f.teamB1}, "allow"},
		{"carol cannot add to A1", session(f.carol), ActionMemberWrite, Object{Type: ObjectMember, TeamID: f.teamA1}, "forbidden"},
		{"admin adds anywhere", session(f.admin), ActionMemberWrite, Object{Type: ObjectMember, TeamID: f.teamB1}, "allow"},

		// ---- projects ----
		{"member reads own project", session(f.plain), ActionProjectRead, Object{Type: ObjectProject, ID: f.projA1.ID}, "allow"},
		{"member cannot read a foreign project", session(f.plain), ActionProjectRead, Object{Type: ObjectProject, ID: f.projB1.ID}, "notfound"},
		{"member cannot edit a project", session(f.plain), ActionProjectWrite, Object{Type: ObjectProject, ID: f.projA1.ID}, "forbidden"},
		{"team admin edits own project", session(f.alice), ActionProjectWrite, Object{Type: ObjectProject, ID: f.projA1.ID}, "allow"},
		{"team admin cannot edit a foreign project", session(f.alice), ActionProjectWrite, Object{Type: ObjectProject, ID: f.projB1.ID}, "forbidden"},
		{"admin edits any project", session(f.admin), ActionProjectWrite, Object{Type: ObjectProject, ID: f.projB1.ID}, "allow"},

		// ---- personal keys: the privacy rule ----
		{"owner reads own personal key", session(f.alice), ActionKeyRead, Object{Type: ObjectKey, ID: f.aliceKey.ID}, "allow"},
		{"team admin cannot read a member's personal key", session(f.alice), ActionKeyRead, Object{Type: ObjectKey, ID: f.plainKey.ID}, "notfound"},
		{"admin reads any personal key", session(f.admin), ActionKeyRead, Object{Type: ObjectKey, ID: f.carolKey.ID}, "allow"},
		{"owner deletes own personal key", session(f.alice), ActionKeyDelete, Object{Type: ObjectKey, ID: f.aliceKey.ID}, "allow"},
		{"team admin cannot delete a member's personal key", session(f.alice), ActionKeyDelete, Object{Type: ObjectKey, ID: f.plainKey.ID}, "notfound"},
		{"a member cannot create a personal key for someone else", session(f.alice), ActionKeyCreate,
			Object{Type: ObjectKey, TeamID: f.teamA1, OwnerType: iam.OwnerPersonal, OwnerUserID: f.plain.ID}, "forbidden"},
		{"a member creates a personal key for themselves", session(f.plain), ActionKeyCreate,
			Object{Type: ObjectKey, TeamID: f.teamA1, OwnerType: iam.OwnerPersonal, OwnerUserID: f.plain.ID}, "allow"},
		{"a member cannot create a key in a foreign team", session(f.plain), ActionKeyCreate,
			Object{Type: ObjectKey, TeamID: f.teamB1, OwnerType: iam.OwnerPersonal, OwnerUserID: f.plain.ID}, "forbidden"},

		// ---- service keys ----
		{"member cannot see a service key", session(f.plain), ActionKeyRead, Object{Type: ObjectKey, ID: f.aliceServiceKey.ID}, "notfound"},
		{"team admin sees the team's service key", session(f.alice), ActionKeyRead, Object{Type: ObjectKey, ID: f.aliceServiceKey.ID}, "allow"},
		{"carol cannot see A1's service key", session(f.carol), ActionKeyRead, Object{Type: ObjectKey, ID: f.aliceServiceKey.ID}, "notfound"},
		{"member cannot create a service key", session(f.plain), ActionKeyCreate,
			Object{Type: ObjectKey, TeamID: f.teamA1, OwnerType: iam.OwnerService}, "forbidden"},
		{"team admin creates a service key", session(f.alice), ActionKeyCreate,
			Object{Type: ObjectKey, TeamID: f.teamA1, OwnerType: iam.OwnerService}, "allow"},
		{"team admin rotates the team's service key", session(f.alice), ActionKeyWrite, Object{Type: ObjectKey, ID: f.aliceServiceKey.ID}, "allow"},

		// ---- router templates ----
		//
		// Visibility comes from ownership. A platform template has no owner and
		// everyone reads it; an organization's template is read inside that
		// organization; a team's template is read by that team.
		{"member reads a platform template", session(f.plain), ActionRouteTemplateRead, Object{Type: ObjectRouteTemplate}, "allow"},
		{"member reads their organization's template", session(f.plain), ActionRouteTemplateRead,
			Object{Type: ObjectRouteTemplate, OrgID: f.orgA.ID}, "allow"},
		{"member reads their team's template", session(f.plain), ActionRouteTemplateRead,
			Object{Type: ObjectRouteTemplate, OrgID: f.orgA.ID, TeamID: f.teamA1}, "allow"},
		{"member cannot read another tenant's organization template", session(f.plain), ActionRouteTemplateRead,
			Object{Type: ObjectRouteTemplate, OrgID: f.orgB.ID}, "notfound"},
		{"member cannot read another team's template", session(f.plain), ActionRouteTemplateRead,
			Object{Type: ObjectRouteTemplate, OrgID: f.orgB.ID, TeamID: f.teamB1}, "notfound"},
		{"carol reads her own organization's templates", session(f.carol), ActionRouteTemplateRead,
			Object{Type: ObjectRouteTemplate, OrgID: f.orgB.ID}, "allow"},

		// Writing is the owner's alone: reading a sibling team's configuration is
		// allowed, editing it is not.
		{"member cannot write a template", session(f.plain), ActionRouteTemplateWrite,
			Object{Type: ObjectRouteTemplate, OrgID: f.orgA.ID, TeamID: f.teamA1}, "forbidden"},
		{"team admin writes their team's template", session(f.alice), ActionRouteTemplateWrite,
			Object{Type: ObjectRouteTemplate, OrgID: f.orgA.ID, TeamID: f.teamA1}, "allow"},
		{"team admin cannot write a sibling team's template", session(f.alice), ActionRouteTemplateWrite,
			Object{Type: ObjectRouteTemplate, OrgID: f.orgA.ID, TeamID: f.teamA2}, "forbidden"},
		{"team admin cannot write another organization's template", session(f.alice), ActionRouteTemplateWrite,
			Object{Type: ObjectRouteTemplate, OrgID: f.orgB.ID, TeamID: f.teamB1}, "forbidden"},
		{"org admin writes a template of a team beneath it", session(f.carol), ActionRouteTemplateWrite,
			Object{Type: ObjectRouteTemplate, OrgID: f.orgB.ID, TeamID: f.teamB1}, "allow"},
		{"member cannot write an organization template", session(f.plain), ActionRouteTemplateWrite,
			Object{Type: ObjectRouteTemplate, OrgID: f.orgA.ID}, "forbidden"},
		{"admin writes any template", session(f.admin), ActionRouteTemplateWrite,
			Object{Type: ObjectRouteTemplate, OrgID: f.orgB.ID, TeamID: f.teamB1}, "allow"},

		// Selecting is narrower than writing: it picks among configurations that
		// already exist, on scopes the caller already belongs to.
		{"member selects for their own team", session(f.plain), ActionRouteTemplateSelect,
			Object{Type: ObjectTeam, ID: f.teamA1, TeamID: f.teamA1, OrgID: f.orgA.ID}, "allow"},
		{"member selects for their own organization", session(f.plain), ActionRouteTemplateSelect,
			Object{Type: ObjectOrg, ID: f.orgA.ID, OrgID: f.orgA.ID}, "allow"},
		{"member cannot select for a foreign team", session(f.plain), ActionRouteTemplateSelect,
			Object{Type: ObjectTeam, ID: f.teamB1, TeamID: f.teamB1, OrgID: f.orgB.ID}, "notfound"},
		{"member selects for their own personal key", session(f.plain), ActionRouteTemplateSelect,
			Object{Type: ObjectKey, ID: f.plainKey.ID}, "allow"},
		{"member cannot select for a colleague's key", session(f.plain), ActionRouteTemplateSelect,
			Object{Type: ObjectKey, ID: f.aliceKey.ID}, "notfound"},
		{"member cannot select for a service key", session(f.plain), ActionRouteTemplateSelect,
			Object{Type: ObjectKey, ID: f.aliceServiceKey.ID}, "forbidden"},
		{"team admin selects for the team's service key", session(f.alice), ActionRouteTemplateSelect,
			Object{Type: ObjectKey, ID: f.aliceServiceKey.ID}, "allow"},

		// ---- usage ----
		{"member reads own usage", session(f.plain), ActionUsageRead, Object{Type: ObjectUsage, OwnerUserID: f.plain.ID}, "allow"},
		{"member cannot read someone else's usage", session(f.plain), ActionUsageRead,
			Object{Type: ObjectUsage, TeamID: f.teamA1, OwnerUserID: f.alice.ID}, "forbidden"},
		{"team admin reads another member's usage breakdown", session(f.alice), ActionUsageRead,
			Object{Type: ObjectUsage, TeamID: f.teamA1, OwnerUserID: f.plain.ID}, "allow"},
		{"team admin cannot read a foreign team's usage", session(f.alice), ActionUsageRead,
			Object{Type: ObjectUsage, TeamID: f.teamB1, OwnerUserID: f.plain.ID}, "notfound"},
		{"member cannot read team totals", session(f.plain), ActionUsageRead, Object{Type: ObjectUsage, TeamID: f.teamA1}, "forbidden"},
		{"admin reads global usage", session(f.admin), ActionUsageRead, Object{Type: ObjectUsage}, "allow"},

		// ---- logs: personal content is the owner's alone ----
		{"owner reads own personal log", session(f.alice), ActionLogRead,
			Object{Type: ObjectLog, ID: "r1", TeamID: f.teamA1, OwnerType: iam.OwnerPersonal, OwnerUserID: f.alice.ID}, "allow"},
		{"team admin reads a member's log inside the team", session(f.alice), ActionLogRead,
			Object{Type: ObjectLog, ID: "r2", TeamID: f.teamA1, OwnerType: iam.OwnerPersonal, OwnerUserID: f.carol.ID}, "allow"},
		{"team admin reads the team's service log", session(f.alice), ActionLogRead,
			Object{Type: ObjectLog, ID: "r3", TeamID: f.teamA1, OwnerType: iam.OwnerService}, "allow"},
		{"member cannot read the team's service log", session(f.plain), ActionLogRead,
			Object{Type: ObjectLog, ID: "r4", TeamID: f.teamA1, OwnerType: iam.OwnerService}, "notfound"},
		{"carol cannot read A1's service log", session(f.carol), ActionLogRead,
			Object{Type: ObjectLog, ID: "r5", TeamID: f.teamA1, OwnerType: iam.OwnerService}, "notfound"},
		{"admin reads any log", session(f.admin), ActionLogRead,
			Object{Type: ObjectLog, ID: "r6", TeamID: f.teamA1, OwnerType: iam.OwnerPersonal, OwnerUserID: f.carol.ID}, "allow"},

		// ---- audit ----
		{"member cannot read audit", session(f.alice), ActionAuditRead, Object{Type: ObjectAudit}, "forbidden"},
		{"admin reads audit", session(f.admin), ActionAuditRead, Object{Type: ObjectAudit}, "allow"},

		// ---- inference ----
		{"member infers in own team", session(f.plain), ActionInfer, Object{Type: ObjectModel, TeamID: f.teamA1}, "allow"},
		{"member cannot infer in a foreign team", session(f.plain), ActionInfer, Object{Type: ObjectModel, TeamID: f.teamB1}, "forbidden"},

		// ---- unknown actions deny ----
		{"unknown action denies a member", session(f.plain), Action("nonsense"), Object{Type: ObjectTeam, ID: f.teamA1}, "forbidden"},
		{"unknown action denies an admin", session(f.admin), Action("nonsense"), Object{Type: ObjectTeam, ID: f.teamA1}, "forbidden"},
		{"unknown object type is not an object", session(f.admin), ActionTeamRead, Object{Type: "mystery", ID: f.teamA1}, "notfound"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := f.decide(t, tc.actor, tc.action, tc.obj); got != tc.want {
				t.Fatalf("got %s, want %s", got, tc.want)
			}
		})
	}

	// The audit trail for a platform administrator reading somebody else's log
	// content must exist; the plan requires the read to be recorded.
	logs, err := f.db.ListAudit(ctx, 50, 0)
	if err != nil {
		t.Fatalf("list audit: %v", err)
	}
	found := false
	for _, e := range logs {
		if e.Action == "log.read_content" {
			found = true
		}
	}
	if !found {
		t.Fatal("platform administrator read of log content was not audited")
	}
}

// TestKeyActorCannotManage is the "a leaked key is not an admin" rule. A key is
// checked even when its owner is a platform administrator.
func TestKeyActorCannotManage(t *testing.T) {
	f := newFixture(t)

	for _, action := range []Action{
		ActionUserRead, ActionUserWrite, ActionUserCreate, ActionUserDelete,
		ActionOrgRead, ActionOrgWrite, ActionTeamRead, ActionTeamWrite,
		ActionMemberRead, ActionMemberWrite, ActionProjectWrite,
		ActionAuditRead,
	} {
		t.Run(string(action), func(t *testing.T) {
			keyActor := Actor{Kind: KindKey, KeyID: f.aliceKey.ID, UserID: f.alice.ID,
				Role: iam.RoleAdmin, KeyTeamID: f.teamA1, OwnerType: iam.OwnerPersonal}
			if got := f.decide(t, keyActor, action, Object{Type: ObjectKey, ID: f.aliceServiceKey.ID}); got != "forbidden" {
				t.Fatalf("key actor got %s for %s, want forbidden", got, action)
			}
		})
	}

	// A key may infer and read itself.
	keyActor := Actor{Kind: KindKey, KeyID: f.aliceKey.ID, UserID: f.alice.ID,
		KeyTeamID: f.teamA1, OwnerType: iam.OwnerPersonal}
	if got := f.decide(t, keyActor, ActionInfer, Object{Type: ObjectModel}); got != "allow" {
		t.Fatalf("key infer: got %s, want allow", got)
	}
	if got := f.decide(t, keyActor, ActionKeyRead, Object{Type: ObjectKey, ID: f.aliceKey.ID}); got != "allow" {
		t.Fatalf("key reads itself: got %s, want allow", got)
	}
	if got := f.decide(t, keyActor, ActionKeyRead, Object{Type: ObjectKey, ID: f.aliceServiceKey.ID}); got != "notfound" {
		t.Fatalf("key reads another key: got %s, want notfound", got)
	}

	// A revoked key loses even inference.
	if _, err := f.db.SetKeyStatus(context.Background(), f.sys(), f.aliceKey.ID, iam.StatusRevoked); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if got := f.decide(t, keyActor, ActionInfer, Object{Type: ObjectModel}); got != "forbidden" {
		t.Fatalf("revoked key infer: got %s, want forbidden", got)
	}
}

// TestKeyRevalidatedEveryRequest covers the plan's immediate-effect rule: a
// removed member's personal key, a disabled owner's key, a blocked team and a
// blocked project all stop working on the next call.
func TestKeyRevalidatedEveryRequest(t *testing.T) {
	ctx := context.Background()

	t.Run("removing the member kills their personal key", func(t *testing.T) {
		f := newFixture(t)
		k, _, err := f.db.CreateKey(ctx, f.sys(), iam.KeyInput{OwnerType: iam.OwnerPersonal, UserID: f.plain.ID, TeamID: f.teamA1, Name: "plain-key"})
		if err != nil {
			t.Fatalf("create key: %v", err)
		}
		actor := Actor{Kind: KindKey, KeyID: k.ID, UserID: f.plain.ID, KeyTeamID: f.teamA1, OwnerType: iam.OwnerPersonal}
		if got := f.decide(t, actor, ActionInfer, Object{Type: ObjectModel}); got != "allow" {
			t.Fatalf("before removal: got %s, want allow", got)
		}
		if err := f.db.RemoveMember(ctx, f.sys(), f.teamA1, f.plain.ID); err != nil {
			t.Fatalf("remove member: %v", err)
		}
		if got := f.decide(t, actor, ActionInfer, Object{Type: ObjectModel}); got != "forbidden" {
			t.Fatalf("after removal: got %s, want forbidden", got)
		}
	})

	t.Run("disabling the owner kills their personal key", func(t *testing.T) {
		f := newFixture(t)
		k, _, err := f.db.CreateKey(ctx, f.sys(), iam.KeyInput{OwnerType: iam.OwnerPersonal, UserID: f.plain.ID, TeamID: f.teamA1, Name: "plain-key"})
		if err != nil {
			t.Fatalf("create key: %v", err)
		}
		actor := Actor{Kind: KindKey, KeyID: k.ID, UserID: f.plain.ID, KeyTeamID: f.teamA1, OwnerType: iam.OwnerPersonal}
		if _, err := f.db.AdminUpdateUser(ctx, f.sys(), f.plain.ID, iam.UserUpdate{Status: ptr(iam.StatusDisabled)}); err != nil {
			t.Fatalf("disable: %v", err)
		}
		if got := f.decide(t, actor, ActionInfer, Object{Type: ObjectModel}); got != "forbidden" {
			t.Fatalf("after disabling the owner: got %s, want forbidden", got)
		}
	})

	t.Run("blocking the team kills the key", func(t *testing.T) {
		f := newFixture(t)
		k, _, err := f.db.CreateKey(ctx, f.sys(), iam.KeyInput{OwnerType: iam.OwnerService, TeamID: f.teamA1, Name: "a1-service"})
		if err != nil {
			t.Fatalf("create key: %v", err)
		}
		actor := Actor{Kind: KindKey, KeyID: k.ID, KeyTeamID: f.teamA1, OwnerType: iam.OwnerService}
		if got := f.decide(t, actor, ActionInfer, Object{Type: ObjectModel}); got != "allow" {
			t.Fatalf("before block: got %s, want allow", got)
		}
		if _, err := f.db.AdminUpdateTeam(ctx, f.sys(), f.teamA1, ptr(iam.StatusBlocked), nil, nil); err != nil {
			t.Fatalf("block team: %v", err)
		}
		if got := f.decide(t, actor, ActionInfer, Object{Type: ObjectModel}); got != "forbidden" {
			t.Fatalf("after blocking the team: got %s, want forbidden", got)
		}
	})

	t.Run("blocking a project kills its key", func(t *testing.T) {
		f := newFixture(t)
		k, _, err := f.db.CreateKey(ctx, f.sys(), iam.KeyInput{OwnerType: iam.OwnerService, TeamID: f.teamA1, ProjectID: f.projA1.ID, Name: "proj-service"})
		if err != nil {
			t.Fatalf("create key: %v", err)
		}
		actor := Actor{Kind: KindKey, KeyID: k.ID, KeyTeamID: f.teamA1, KeyProjectID: f.projA1.ID, OwnerType: iam.OwnerService}
		if got := f.decide(t, actor, ActionInfer, Object{Type: ObjectModel}); got != "allow" {
			t.Fatalf("before block: got %s, want allow", got)
		}
		if _, err := f.db.UpdateProject(ctx, f.sys(), f.projA1.ID, iam.ProjectInput{Status: iam.StatusBlocked}); err != nil {
			t.Fatalf("block project: %v", err)
		}
		if got := f.decide(t, actor, ActionInfer, Object{Type: ObjectModel}); got != "forbidden" {
			t.Fatalf("after blocking the project: got %s, want forbidden", got)
		}
	})
}

func TestTeamAdminLogScopeExcludesOtherPersonalKeys(t *testing.T) {
	f := newFixture(t)
	g := f.guard(t, session(f.alice))
	scope, err := g.LogsScope(context.Background())
	if err != nil {
		t.Fatalf("logs scope: %v", err)
	}
	query, args, err := builder.ToSQL(scope.Cond)
	if err != nil {
		t.Fatalf("render scope: %v", err)
	}
	if !strings.Contains(query, "owner_type") || !strings.Contains(query, "team_id") || !scope.ServiceOnly {
		t.Fatalf("team log scope does not restrict team rows to service keys: query=%q args=%v scope=%#v", query, args, scope)
	}
	if strings.Count(query, "owner_type") < 2 {
		t.Fatalf("scope must independently constrain own personal and team service logs: %q", query)
	}
}

// TestMasterIsNarrow pins the master credential's deliberate lack of reach. It
// exists for /bootstrap and emergencies; it is not a platform administrator.
func TestMasterIsNarrow(t *testing.T) {
	f := newFixture(t)
	master := Actor{Kind: KindMaster}

	if got := f.decide(t, master, ActionUserRead, Object{Type: ObjectUser, ID: f.alice.ID}); got != "allow" {
		t.Fatalf("master user read: got %s, want allow", got)
	}
	for _, action := range []Action{ActionInfer, ActionKeyRead, ActionLogRead, ActionUsageRead, ActionTeamWrite, ActionMemberWrite, ActionUserWrite} {
		if got := f.decide(t, master, action, Object{Type: ObjectKey, ID: f.aliceKey.ID}); got != "forbidden" {
			t.Fatalf("master %s: got %s, want forbidden", action, got)
		}
	}
	// A master listing scope must deny rather than hand over every tenant.
	ctx := context.Background()
	g := f.guard(t, master)
	keys, err := g.KeysScope(ctx)
	if err != nil {
		t.Fatalf("master keys scope: %v", err)
	}
	logs, err := g.LogsScope(ctx)
	if err != nil {
		t.Fatalf("master logs scope: %v", err)
	}
	usage, err := g.UsageScope(ctx, "")
	if err != nil {
		t.Fatalf("master usage scope: %v", err)
	}
	for _, sc := range []*Scope{keys, logs, usage} {
		if sc.All {
			t.Fatalf("master scope %q must not be unlimited", sc.Kind)
		}
		if sc.Cond == nil || !sc.Cond.IsValid() {
			t.Fatalf("master scope %q must carry a valid, always-false condition", sc.Kind)
		}
	}

	// TeamFilter is the fourth listing scope, and the one the team-scoped
	// listings use. It must answer "no team" rather than nil, because nil is the
	// store's "every team" and this principal carries no user ID — a listing that
	// derived its scope from that empty ID answered with the whole inventory.
	teams := g.TeamFilter()
	if teams == nil {
		t.Fatal("master team filter must not be nil (nil means every team)")
	}
	if len(teams) != 0 {
		t.Fatalf("master team filter must be empty, got %v", teams)
	}

	// And the same answer through the exported helper, which is what a listing
	// calls directly.
	if v := g.VisibleTeamIDs(); v == nil || len(v) != 0 {
		t.Fatalf("master visible teams must be an empty slice, got %#v", v)
	}

	// A platform administrator is the one actor that legitimately gets nil.
	if v := f.guard(t, session(f.admin)).VisibleTeamIDs(); v != nil {
		t.Fatalf("a platform administrator must see every team as nil, got %#v", v)
	}
}

func mustScope(t *testing.T, s *Scope, err error) *Scope {
	t.Helper()
	if err != nil {
		t.Fatalf("scope: %v", err)
	}
	return s
}

// TestScopeFailsClosed is the regression test for the dangerous xorm behaviour
// Apply exists to neutralise: builder.And drops conditions whose IsValid is
// false, and builder.In over an empty list is exactly such a condition, so a
// member who belongs to no team would otherwise list every tenant's rows.
//
// It asserts on row counts from real queries rather than on the rendered SQL,
// because the failure mode is a filter that is silently absent.
func TestScopeFailsClosed(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	count := func(sc *Scope, err error) int64 {
		t.Helper()
		if err != nil {
			t.Fatalf("scope: %v", err)
		}
		if sc.All {
			t.Fatal("this actor must not receive an unlimited scope")
		}
		s := sc.Apply(f.db.Engine.NewSession().Context(ctx).Table("api_keys"))
		defer s.Close()
		n, err := s.Count()
		if err != nil {
			t.Fatalf("count: %v", err)
		}
		return n
	}

	// alice administers teamA1: her own personal key and the team's service key,
	// and never carol's personal key.
	if n := count(f.guard(t, session(f.alice)).KeysScope(ctx)); n != 2 {
		t.Fatalf("team admin sees %d keys, want 2 (own personal + team service)", n)
	}
	// An ordinary member sees only their own, so carol sees just her own key.
	if n := count(f.guard(t, session(f.carol)).KeysScope(ctx)); n != 1 {
		t.Fatalf("ordinary member sees %d keys, want 1 (own only)", n)
	}
	// The whole table, for reference.
	total, err := f.db.Engine.NewSession().Context(ctx).Table("api_keys").Count()
	if err != nil {
		t.Fatalf("count all: %v", err)
	}
	if total != 4 {
		t.Fatalf("fixture holds %d keys, want 4", total)
	}
	// A member of no team at all: zero, and emphatically not the whole table.
	if n := count(f.guard(t, session(f.solo)).KeysScope(ctx)); n != 0 {
		t.Fatalf("a member of no team sees %d keys, want 0", n)
	}
	// The master credential gets no listing at all.
	if n := count(f.guard(t, Actor{Kind: KindMaster}).KeysScope(ctx)); n != 0 {
		t.Fatalf("master sees %d keys, want 0", n)
	}
}

// TestApplyTeamFailsClosed pins the same guarantee for the team-column form,
// where the filter is built from a slice rather than a stored condition.
func TestApplyTeamFailsClosed(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	if s := (&Scope{Kind: ScopeTeamKeys}).ApplyTeam(f.db.Engine.NewSession().Context(ctx), "team_id"); true {
		defer s.Close()
		n, err := s.Table("api_keys").Count()
		if err != nil {
			t.Fatalf("count: %v", err)
		}
		if n != 0 {
			t.Fatalf("an empty team scope returned %d keys, want 0", n)
		}
	}
	if s := (&Scope{Kind: ScopeTeamKeys}).ApplyUser(f.db.Engine.NewSession().Context(ctx), "user_id"); true {
		defer s.Close()
		n, err := s.Table("api_keys").Count()
		if err != nil {
			t.Fatalf("count: %v", err)
		}
		if n != 0 {
			t.Fatalf("an empty owner scope returned %d keys, want 0", n)
		}
	}
}

// TestTeamFilterNilMeansAllEmptyMeansNone documents the two different answers
// TeamFilter may give, because the store turns them into different SQL.
func TestTeamFilterNilMeansAllEmptyMeansNone(t *testing.T) {
	f := newFixture(t)

	if got := f.guard(t, session(f.admin)).TeamFilter(); got != nil {
		t.Fatalf("platform admin: got %v, want nil (all teams)", got)
	}
	if got := f.guard(t, session(f.plain)).TeamFilter(); len(got) != 1 || got[0] != f.teamA1 {
		t.Fatalf("plain member: got %v, want exactly teamA1", got)
	}
	stranger := session(f.solo)
	got := f.guard(t, stranger).TeamFilter()
	if got == nil {
		t.Fatal("a member with no teams must not get nil (that means all teams)")
	}
	if len(got) != 0 {
		t.Fatalf("a member with no teams: got %v, want empty", got)
	}
}

// TestUnknownActorKindDenies makes sure a zero Actor is not a bypass, and that
// a role string the matrix does not recognise never reads as platform
// administration. The matrix knows exactly admin and user.
func TestUnknownActorKindDenies(t *testing.T) {
	f := newFixture(t)
	for _, a := range []Actor{{}, {Kind: "bogus"}, {Kind: KindSession}, {Kind: KindSession, UserID: f.solo.ID, Role: "root"}} {
		t.Run(a.Role+string(a.Kind), func(t *testing.T) {
			if got := f.decide(t, a, ActionTeamRead, Object{Type: ObjectTeam, ID: f.teamA1}); got == "allow" {
				t.Fatalf("actor %+v was allowed", a)
			}
		})
	}

	// A session whose role is not admin is an ordinary user even if the string
	// reads like a promotion, and a platform administrator is only ever the
	// literal admin role.
	for _, role := range []string{"root", "proxy_admin", "org_admin", "team_admin", ""} {
		a := Actor{Kind: KindSession, UserID: f.plain.ID, Role: role}
		if got := f.decide(t, a, ActionOrgWrite, Object{Type: ObjectOrg, ID: f.orgA.ID}); got != "forbidden" {
			t.Fatalf("role %q got %s writing an organization, want forbidden", role, got)
		}
	}
}

// TestQuotaAdministrationScope 验证个人额度管理只授予唯一归属团队/组织管理员；前置真实隔离 schema。
// 参数 t 为测试上下文，无返回；覆盖本团队、跨团队、独立个人与普通成员，testDB 自动清理记录。
func TestQuotaAdministrationScope(t *testing.T) {
	f := newFixture(t)
	for _, tc := range []struct {
		name   string
		actor  *iam.User
		target string
		want   string
	}{
		{"平台管理独立个人", f.admin, f.solo.ID, "allow"},
		{"团队管理本团队个人", f.alice, f.plain.ID, "allow"},
		{"团队不能管理另一团队", f.alice, f.dave.ID, "forbidden"},
		{"跨组织管理员不能管理个人", f.carol, f.plain.ID, "forbidden"},
		{"普通成员不能自行提额", f.plain, f.plain.ID, "forbidden"},
		{"团队不能管理独立个人", f.alice, f.solo.ID, "forbidden"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := f.decide(t, session(tc.actor), ActionUserBudget, Object{Type: ObjectUser, ID: tc.target}); got != tc.want {
				t.Fatalf("个人额度权限 got=%s want=%s", got, tc.want)
			}
		})
	}
	if err := f.db.SetOrgAdmin(t.Context(), f.sys(), f.orgA.ID, f.solo.ID, true); err != nil {
		t.Fatal(err)
	}
	if got := f.decide(t, session(f.solo), ActionUserBudget, Object{Type: ObjectUser, ID: f.plain.ID}); got != "allow" {
		t.Fatalf("组织管理员本组织额度: %s", got)
	}
	if got := f.decide(t, session(f.solo), ActionUserBudget, Object{Type: ObjectUser, ID: f.carol.ID}); got != "forbidden" {
		t.Fatalf("组织管理员跨组织额度: %s", got)
	}
}
