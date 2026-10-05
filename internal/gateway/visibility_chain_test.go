package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/sunqirui1987/xhub/internal/iam"
)

// This file is the visibility chain: one realistic tenant built from the top
// down, then every account in it asked the same questions.
//
// The point is not that one route is guarded. It is that the answer is the same
// shape at every tier, and that the shape is what the product promises:
//
//	platform administrator   sees everything
//	organization administrator sees their organization, and only theirs
//	team administrator       sees the teams they administer, and only those
//	member                   sees themselves, and nothing about anybody else
//
// Two accounts in different organizations are built to sit beside each other,
// so a bug that widens a scope shows up as the wrong organization's rows
// appearing rather than as a missing guard. Every tier is probed with both
// positive and negative reads, because a listing that returns nothing at all is
// as broken as one that returns everything.

// chain is one tenant: two organizations, three teams, and an account at every
// tier. It is built once and read by every tier's subtest.
type chain struct {
	t   *testing.T
	f   *fixture
	ctx context.Context

	orgA, orgB         *iam.Organization
	teamA1, teamA2     *iam.Team // both in orgA
	teamB1             *iam.Team // in orgB
	projectA1          *iam.Project
	orgAdminA          actor // administers orgA, is a plain member of teamA1
	teamAdminA1        actor // administers teamA1
	memberA1           actor // plain member of teamA1
	teamAdminA2        actor // administers teamA2, same organization as teamA1
	orgAdminB          actor // administers orgB
	memberB1           actor // plain member of teamB1, a different organization
	platformAdminFirst string
}

// newChain builds the tenant and signs every account in over HTTP, through the
// same login route the console's form uses.
func newChain(t *testing.T) *chain {
	t.Helper()
	f := newPermFixture(t)
	c := &chain{t: t, f: f, ctx: context.Background()}

	c.orgA = mustOrg(t, f.db, "Chain Org A")
	c.orgB = mustOrg(t, f.db, "Chain Org B")

	// An account per tier. Each is created through the identity layer rather
	// than over HTTP, so a fault in the write path cannot make a visibility
	// assertion pass by accident.
	c.orgAdminA = chainUser(t, f, "chain-orgadmin-a@example.com", iam.RoleUser)
	c.teamAdminA1 = chainUser(t, f, "chain-teamadmin-a1@example.com", iam.RoleUser)
	c.memberA1 = chainUser(t, f, "chain-member-a1@example.com", iam.RoleUser)
	c.teamAdminA2 = chainUser(t, f, "chain-teamadmin-a2@example.com", iam.RoleUser)
	c.orgAdminB = chainUser(t, f, "chain-orgadmin-b@example.com", iam.RoleUser)
	c.memberB1 = chainUser(t, f, "chain-member-b1@example.com", iam.RoleUser)

	c.teamA1 = mustTeam(t, f.db, c.orgA.ID, "Chain Team A1", c.teamAdminA1.user.ID)
	c.teamA2 = mustTeam(t, f.db, c.orgA.ID, "Chain Team A2", c.teamAdminA2.user.ID)
	c.teamB1 = mustTeam(t, f.db, c.orgB.ID, "Chain Team B1", c.orgAdminB.user.ID)

	// The organization administrator is deliberately a plain member of a team
	// in their own organization. Running an organization is a relationship on
	// the organization, not a team role, and the two must not be conflated.
	mustAddMember(t, f.db, c.teamA1.ID, c.orgAdminA.user.Email, iam.TeamMember)
	mustAddMember(t, f.db, c.teamA1.ID, c.memberA1.user.Email, iam.TeamMember)
	mustAddMember(t, f.db, c.teamB1.ID, c.memberB1.user.Email, iam.TeamMember)

	if _, err := f.db.AddOrgAdmin(c.ctx, iam.Actor{Kind: "system"}, c.orgA.ID, c.orgAdminA.user.Email); err != nil {
		t.Fatalf("make %s an org admin of A: %v", c.orgAdminA.user.Email, err)
	}

	// A project inside team A1, so the project listing has something to leak.
	proj, err := f.db.CreateProject(c.ctx, iam.Actor{Kind: "system"}, iam.ProjectInput{
		TeamID: c.teamA1.ID, Name: "Chain Project A1",
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	c.projectA1 = proj

	for _, a := range []*actor{&c.orgAdminA, &c.teamAdminA1, &c.memberA1, &c.teamAdminA2, &c.orgAdminB, &c.memberB1} {
		a.key = signIn(t, f.base, a.user)
	}

	// The fixture's own accounts are outsiders to this tenant, except the
	// platform administrator, who is the top of the chain.
	c.platformAdminFirst = f.admin.user.Email
	return c
}

// chainUser creates one account with the fixture's standard password.
func chainUser(t *testing.T, f *fixture, email, role string) actor {
	t.Helper()
	u, err := f.db.CreateUser(context.Background(), iam.Actor{Kind: "system"}, iam.UserInput{
		Email: email, Name: email, Password: "password123", Role: role,
	})
	if err != nil {
		t.Fatalf("create %s: %v", email, err)
	}
	return actor{name: email, user: u}
}

// ---------- reading the listings ----------

// listIDs reads a wrapper listing such as {"teams":[...]} and returns the
// values of one id field from every row.
//
// The field is named by the caller rather than guessed. A row carries several
// ids — a project names both itself and its team — and picking whichever
// appeared first would silently assert on the wrong one.
func (c *chain) listIDs(a actor, path, collection, idField string) []string {
	c.t.Helper()
	status, body := c.f.call(a, http.MethodGet, path, nil)
	if status != http.StatusOK {
		c.t.Fatalf("%s GET %s: status %d body %s", a.label(), path, status, trim(body))
	}
	var probe map[string]any
	if err := json.Unmarshal(body, &probe); err != nil {
		c.t.Fatalf("%s GET %s: not an object: %s", a.label(), path, trim(body))
	}
	rows, ok := probe[collection].([]any)
	if !ok {
		// A listing with no rows may serialize the key away entirely.
		if _, present := probe[collection]; !present {
			return nil
		}
		c.t.Fatalf("%s GET %s: %q is not a list: %s", a.label(), path, collection, trim(body))
	}
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		obj, ok := row.(map[string]any)
		if !ok {
			c.t.Fatalf("%s GET %s: a row is not an object: %s", a.label(), path, trim(body))
		}
		if v, ok := obj[idField].(string); ok && v != "" {
			out = append(out, v)
		}
	}
	return out
}

// idSet renders ids for a failure message, so a leak names what leaked.
func idSet(ids []string) string {
	if len(ids) == 0 {
		return "(none)"
	}
	return strings.Join(ids, ",")
}

// ---------- the chain ----------

// TestVisibilityChainPlatformAdminSeesEverything is the top of the chain: the
// platform administrator is the only account that sees both organizations.
func TestVisibilityChainPlatformAdminSeesEverything(t *testing.T) {
	c := newChain(t)
	admin := c.f.admin

	teams := c.listIDs(admin, "/v2/team/list?page=1&page_size=100", "teams", "team_id")
	for _, want := range []string{c.teamA1.ID, c.teamA2.ID, c.teamB1.ID} {
		if !contains(teams, want) {
			t.Fatalf("platform administrator does not see team %s: %s", want, idSet(teams))
		}
	}

	// The organization listing is a bare array rather than a wrapped
	// collection, so it is read directly.
	status, body := c.f.call(admin, http.MethodGet, "/organization/list", nil)
	if status != http.StatusOK {
		t.Fatalf("platform administrator GET /organization/list: %d %s", status, trim(body))
	}
	var orgList []map[string]any
	if err := json.Unmarshal(body, &orgList); err != nil {
		t.Fatalf("organization listing is not an array: %s", trim(body))
	}
	seen := map[string]bool{}
	for _, o := range orgList {
		if id, ok := o["organization_id"].(string); ok {
			seen[id] = true
		}
	}
	if !seen[c.orgA.ID] || !seen[c.orgB.ID] {
		t.Fatalf("platform administrator sees %d organizations, missing A or B: %s", len(orgList), trim(body))
	}

	users := c.listIDs(admin, "/user/list", "users", "user_id")
	if !contains(users, c.memberB1.user.ID) || !contains(users, c.orgAdminA.user.ID) {
		t.Fatalf("platform administrator does not see every account: %s", idSet(users))
	}

	projects := c.listIDs(admin, "/project/list", "projects", "project_id")
	if !contains(projects, c.projectA1.ID) {
		t.Fatalf("platform administrator does not see project %s: %s", c.projectA1.ID, idSet(projects))
	}
}

// TestVisibilityChainOrgAdminSeesOwnOrganization is the middle tier. Running an
// organization reaches every team and project in it, and nothing in the other.
func TestVisibilityChainOrgAdminSeesOwnOrganization(t *testing.T) {
	c := newChain(t)
	orgAdmin := c.orgAdminA

	teams := c.listIDs(orgAdmin, "/v2/team/list?page=1&page_size=100", "teams", "team_id")
	if !contains(teams, c.teamA1.ID) {
		t.Fatalf("organization administrator does not see their own team A1: %s", idSet(teams))
	}
	if !contains(teams, c.teamA2.ID) {
		t.Fatalf("organization administrator does not see team A2 of their own organization: %s", idSet(teams))
	}
	if contains(teams, c.teamB1.ID) {
		t.Fatalf("organization administrator sees a team of another organization: %s", idSet(teams))
	}

	projects := c.listIDs(orgAdmin, "/project/list", "projects", "project_id")
	if !contains(projects, c.projectA1.ID) {
		t.Fatalf("organization administrator does not see a project of their own organization: %s", idSet(projects))
	}

	// An organization administrator reaches the people of their organization's
	// teams, which is what the members tab reads.
	status, body := c.f.call(orgAdmin, http.MethodGet, "/team/member_list?team_id="+c.teamA1.ID, nil)
	if status != http.StatusOK {
		t.Fatalf("organization administrator reads own team roster: %d %s", status, trim(body))
	}
	if !strings.Contains(string(body), c.memberA1.user.Email) {
		t.Fatalf("organization administrator cannot see the roster of their own team: %s", trim(body))
	}

	// And not the roster of a team in the other organization.
	status, body = c.f.call(orgAdmin, http.MethodGet, "/team/member_list?team_id="+c.teamB1.ID, nil)
	if status == http.StatusOK && strings.Contains(string(body), c.memberB1.user.Email) {
		t.Fatalf("organization administrator read the roster of another organization's team: %s", trim(body))
	}
}

// TestVisibilityChainTeamAdminSeesOwnTeam is the tier below. A team
// administrator runs the teams they administer and nothing else, including
// nothing else in their own organization.
func TestVisibilityChainTeamAdminSeesOwnTeam(t *testing.T) {
	c := newChain(t)
	teamAdmin := c.teamAdminA1

	teams := c.listIDs(teamAdmin, "/v2/team/list?page=1&page_size=100", "teams", "team_id")
	if !contains(teams, c.teamA1.ID) {
		t.Fatalf("team administrator does not see their own team: %s", idSet(teams))
	}
	if contains(teams, c.teamA2.ID) {
		t.Fatalf("team administrator sees a sibling team they do not administer: %s", idSet(teams))
	}
	if contains(teams, c.teamB1.ID) {
		t.Fatalf("team administrator sees a team of another organization: %s", idSet(teams))
	}

	// The roster of their own team is theirs to read.
	status, body := c.f.call(teamAdmin, http.MethodGet, "/team/member_list?team_id="+c.teamA1.ID, nil)
	if status != http.StatusOK || !strings.Contains(string(body), c.memberA1.user.Email) {
		t.Fatalf("team administrator cannot read their own roster: %d %s", status, trim(body))
	}

	// A sibling team's roster is not.
	status, body = c.f.call(teamAdmin, http.MethodGet, "/team/member_list?team_id="+c.teamA2.ID, nil)
	if status == http.StatusOK && !bodyIsEmptyList(t, body) {
		t.Fatalf("team administrator read a sibling team's roster: %s", trim(body))
	}

	// Nor is another organization's.
	status, body = c.f.call(teamAdmin, http.MethodGet, "/team/member_list?team_id="+c.teamB1.ID, nil)
	if status == http.StatusOK && !bodyIsEmptyList(t, body) {
		t.Fatalf("team administrator read another organization's roster: %s", trim(body))
	}
}

// TestVisibilityChainMemberSeesOnlySelf is the bottom tier, and the one with
// the least room for error: a plain member must not learn about anybody else.
func TestVisibilityChainMemberSeesOnlySelf(t *testing.T) {
	c := newChain(t)
	member := c.memberA1

	// Their own account is theirs to read.
	status, body := c.f.call(member, http.MethodGet, "/user/info?user_id="+member.user.ID, nil)
	if status != http.StatusOK {
		t.Fatalf("member cannot read their own account: %d %s", status, trim(body))
	}

	// Anybody else's is not, and the refusal does not say whether the account
	// exists.
	for _, other := range []actor{c.orgAdminA, c.teamAdminA1, c.orgAdminB, c.memberB1} {
		status, body := c.f.call(member, http.MethodGet, "/user/info?user_id="+other.user.ID, nil)
		if status == http.StatusOK {
			t.Fatalf("member read another account (%s): %s", other.name, trim(body))
		}
		if status != 403 && status != 404 {
			t.Fatalf("reading another account answered %d rather than a refusal: %s", status, trim(body))
		}
	}

	// The account listing narrows to themselves. It does not fail, it shows
	// one row.
	users := c.listIDs(member, "/user/list", "users", "user_id")
	for _, other := range []string{c.teamAdminA1.user.ID, c.orgAdminA.user.ID, c.memberB1.user.ID} {
		if contains(users, other) {
			t.Fatalf("member's account listing leaked %s: %s", other, idSet(users))
		}
	}

	// They see the teams they belong to and nothing more.
	teams := c.listIDs(member, "/v2/team/list?page=1&page_size=100", "teams", "team_id")
	if !contains(teams, c.teamA1.ID) {
		t.Fatalf("member cannot see their own team: %s", idSet(teams))
	}
	if contains(teams, c.teamA2.ID) || contains(teams, c.teamB1.ID) {
		t.Fatalf("member sees a team they do not belong to: %s", idSet(teams))
	}

	// They may read their own team's roster: it is their team.
	status, body = c.f.call(member, http.MethodGet, "/team/member_list?team_id="+c.teamA1.ID, nil)
	if status != http.StatusOK {
		t.Fatalf("member cannot read their own team's roster: %d %s", status, trim(body))
	}

	// They may not read the roster of a team they are not in.
	status, body = c.f.call(member, http.MethodGet, "/team/member_list?team_id="+c.teamB1.ID, nil)
	if status == http.StatusOK && !bodyIsEmptyList(t, body) {
		t.Fatalf("member read another team's roster: %s", trim(body))
	}

	// A plain member does not administer their team, so the administrative
	// routes refuse rather than silently succeeding.
	status, body = c.f.call(member, http.MethodPost, "/team/member_update",
		mustJSONBytes(t, map[string]any{"team_id": c.teamA1.ID, "user_id": c.teamAdminA1.user.ID, "role": "user"}))
	if status == http.StatusOK {
		t.Fatalf("a plain member demoted their team's administrator: %s", trim(body))
	}

	status, body = c.f.call(member, http.MethodPost, "/user/new",
		mustJSONBytes(t, map[string]any{"user_email": "chain-smuggled@example.com", "password": "password123"}))
	if status == http.StatusOK {
		t.Fatalf("a plain member created an account: %s", trim(body))
	}

	// And the platform-wide listings stay closed.
	status, body = c.f.call(member, http.MethodGet, "/audit/logs", nil)
	if status == http.StatusOK {
		t.Fatalf("a plain member read the audit trail: %s", trim(body))
	}
}

// TestVisibilityChainOrgAdminCannotCrossOrganizations is the specific mistake
// this design is most likely to make: treating organization administration as
// platform administration. The account that runs org B must be exactly as
// blind to org A as an ordinary outsider.
func TestVisibilityChainOrgAdminCannotCrossOrganizations(t *testing.T) {
	c := newChain(t)
	orgAdminB := c.orgAdminB

	teams := c.listIDs(orgAdminB, "/v2/team/list?page=1&page_size=100", "teams", "team_id")
	if contains(teams, c.teamA1.ID) || contains(teams, c.teamA2.ID) {
		t.Fatalf("organization administrator of B sees a team of A: %s", idSet(teams))
	}
	if !contains(teams, c.teamB1.ID) {
		t.Fatalf("organization administrator of B does not see their own team: %s", idSet(teams))
	}

	// Reading A's team directly is refused, not narrowed.
	for _, teamID := range []string{c.teamA1.ID, c.teamA2.ID} {
		status, body := c.f.call(orgAdminB, http.MethodGet, "/team/info?team_id="+teamID, nil)
		if status == http.StatusOK {
			t.Fatalf("organization administrator of B read team %s of A: %s", teamID, trim(body))
		}
	}

	// The projects of A are not in their listing.
	projects := c.listIDs(orgAdminB, "/project/list", "projects", "project_id")
	if contains(projects, c.projectA1.ID) {
		t.Fatalf("organization administrator of B sees a project of A: %s", idSet(projects))
	}

	// The accounts of A's teams are not in their account listing.
	users := c.listIDs(orgAdminB, "/user/list", "users", "user_id")
	for _, other := range []string{c.memberA1.user.ID, c.teamAdminA1.user.ID, c.teamAdminA2.user.ID} {
		if contains(users, other) {
			t.Fatalf("organization administrator of B sees an account of A: %s", idSet(users))
		}
	}

	// They may not rename A, and may not change A's budget.
	status, body := c.f.call(orgAdminB, http.MethodPatch, "/organization/update",
		mustJSONBytes(t, map[string]any{"organization_id": c.orgA.ID, "organization_alias": "taken"}))
	if status == http.StatusOK {
		t.Fatalf("organization administrator of B renamed A: %s", trim(body))
	}
	status, body = c.f.call(orgAdminB, http.MethodPatch, "/organization/update",
		mustJSONBytes(t, map[string]any{"organization_id": c.orgA.ID, "max_budget": 1}))
	if status == http.StatusOK {
		t.Fatalf("organization administrator of B changed A's budget: %s", trim(body))
	}
}

// TestVisibilityChainTeamAdminCannotCrossTeams is the same mistake one tier
// down: administering a team must not reach a sibling team of the same
// organization, which an organization administrator legitimately does reach.
func TestVisibilityChainTeamAdminCannotCrossTeams(t *testing.T) {
	c := newChain(t)
	teamAdmin := c.teamAdminA1

	// A sibling team in the same organization is out of reach, both to read and
	// to change.
	status, body := c.f.call(teamAdmin, http.MethodGet, "/team/info?team_id="+c.teamA2.ID, nil)
	if status == http.StatusOK {
		t.Fatalf("team administrator read a sibling team: %s", trim(body))
	}

	status, body = c.f.call(teamAdmin, http.MethodPost, "/team/update",
		mustJSONBytes(t, map[string]any{"team_id": c.teamA2.ID, "team_alias": "hijacked"}))
	if status == http.StatusOK {
		t.Fatalf("team administrator renamed a sibling team: %s", trim(body))
	}

	status, body = c.f.call(teamAdmin, http.MethodPost, "/team/member_add",
		mustJSONBytes(t, map[string]any{"team_id": c.teamA2.ID, "user_email": c.memberB1.user.Email, "role": "member"}))
	if status == http.StatusOK {
		t.Fatalf("team administrator added a member to a sibling team: %s", trim(body))
	}

	// The budget and the model list are the platform's to set, not a team
	// administrator's, even on a team they do administer.
	status, body = c.f.call(teamAdmin, http.MethodPost, "/team/update",
		mustJSONBytes(t, map[string]any{"team_id": c.teamA1.ID, "max_budget": 1}))
	if status == http.StatusOK {
		t.Fatalf("team administrator raised their own team's budget: %s", trim(body))
	}

	status, body = c.f.call(teamAdmin, http.MethodPost, "/team/update",
		mustJSONBytes(t, map[string]any{"team_id": c.teamA1.ID, "models": []string{"gpt-4o-mini"}}))
	if status == http.StatusOK {
		t.Fatalf("team administrator widened their own team's model list: %s", trim(body))
	}
}

// TestVisibilityChainPasswordResetFollowsTheChain checks the one write that
// deliberately crosses tiers: handing somebody a new password.
//
// A team administrator may reset a member of a team they run, because that is
// the ordinary support case. They may not reset their peers, their
// organization's administrator, or anybody in another organization, because
// doing so would take over an account that reaches more than they do.
func TestVisibilityChainPasswordResetFollowsTheChain(t *testing.T) {
	c := newChain(t)
	newPassword := "superseded-password-1"

	set := func(a actor, target *iam.User, password string) (int, []byte) {
		return c.f.call(a, http.MethodPost, "/user/set_password",
			mustJSONBytes(t, map[string]any{"user_id": target.ID, "password": password}))
	}

	t.Run("platform administrator resets anybody", func(t *testing.T) {
		status, body := set(c.f.admin, c.orgAdminB.user, newPassword)
		if status != http.StatusOK {
			t.Fatalf("platform administrator could not reset an org admin: %d %s", status, trim(body))
		}
		// The new password works, which is the whole point: the old onboarding
		// flow answered 200 while discarding the password.
		if got := loginAs(t, c.f.base, c.orgAdminB.user.Email, newPassword); got == "" {
			t.Fatal("the account could not sign in with the password that was just set")
		}
	})

	t.Run("team administrator resets their member", func(t *testing.T) {
		status, body := set(c.teamAdminA1, c.memberA1.user, newPassword)
		if status != http.StatusOK {
			t.Fatalf("team administrator could not reset their own member: %d %s", status, trim(body))
		}
		if got := loginAs(t, c.f.base, c.memberA1.user.Email, newPassword); got == "" {
			t.Fatal("the member could not sign in with the password their team administrator set")
		}
	})

	t.Run("a reset ends the sessions the account had open", func(t *testing.T) {
		// c.memberA1 still holds the session it signed in with before the
		// reset above. A password change must invalidate it: otherwise a
		// stolen session survives the very act meant to end it.
		status, body := c.f.call(c.memberA1, http.MethodGet, "/user/info", nil)
		if status == http.StatusOK {
			t.Fatalf("the session issued before the password reset still works: %s", trim(body))
		}
		// Re-sign so the later subtests read a live session.
		c.memberA1.key = loginAs(t, c.f.base, c.memberA1.user.Email, newPassword)
	})

	t.Run("organization administrator resets a member of their organization", func(t *testing.T) {
		fresh := chainUser(t, c.f, "chain-orgadmin-resets@example.com", iam.RoleUser)
		mustAddMember(t, c.f.db, c.teamA2.ID, fresh.user.Email, iam.TeamMember)
		status, body := set(c.orgAdminA, fresh.user, newPassword)
		if status != http.StatusOK {
			t.Fatalf("organization administrator could not reset a member of their organization: %d %s", status, trim(body))
		}
		if got := loginAs(t, c.f.base, fresh.user.Email, newPassword); got == "" {
			t.Fatal("the member could not sign in with the password their organization administrator set")
		}
	})

	t.Run("team administrator cannot reset their organization administrator", func(t *testing.T) {
		status, body := set(c.teamAdminA1, c.orgAdminA.user, newPassword)
		if status == http.StatusOK {
			t.Fatalf("a team administrator reset their organization's administrator: %s", trim(body))
		}
		if status != 403 && status != 404 {
			t.Fatalf("expected a refusal, got %d: %s", status, trim(body))
		}
	})

	t.Run("team administrator cannot reset a platform administrator", func(t *testing.T) {
		status, body := set(c.teamAdminA1, c.f.admin.user, newPassword)
		if status == http.StatusOK {
			t.Fatalf("a team administrator reset a platform administrator: %s", trim(body))
		}
	})

	t.Run("neither administrator crosses organizations", func(t *testing.T) {
		// c.teamAdminA2 administers team A2, which c.memberB1 is not in.
		status, body := set(c.teamAdminA2, c.memberB1.user, newPassword)
		if status == http.StatusOK {
			t.Fatalf("a team administrator reset an account they do not administer: %s", trim(body))
		}
		// An organization administrator of A has no claim on B's people.
		status, body = set(c.orgAdminA, c.memberB1.user, newPassword)
		if status == http.StatusOK {
			t.Fatalf("an organization administrator reset an account of another organization: %s", trim(body))
		}
	})

	t.Run("a plain member cannot reset anybody", func(t *testing.T) {
		status, body := set(c.memberA1, c.teamAdminA1.user, newPassword)
		if status == http.StatusOK {
			t.Fatalf("a plain member reset another account: %s", trim(body))
		}
	})
}

// TestVisibilityChainCreateAccountCarriesItsScope checks that an account
// created with a team and an organization administrator relationship actually
// has both, and that the second half cannot fail on its own.
//
// The previous shape created the account and then added the membership in a
// separate request. When that request failed, the person existed but could
// reach nothing, and the caller had no way to tell.
func TestVisibilityChainCreateAccountCarriesItsScope(t *testing.T) {
	c := newChain(t)
	admin := c.f.admin

	status, body := c.f.call(admin, http.MethodPost, "/user/new", mustJSONBytes(t, map[string]any{
		"user_email":      "chain-created@example.com",
		"user_alias":      "Created By The Chain",
		"password":        "password123",
		"user_role":       "user",
		"team_id":         c.teamA1.ID,
		"team_role":       "admin",
		"max_budget":      25.5,
		"organization_id": c.orgA.ID,
	}))
	if status != http.StatusOK {
		t.Fatalf("create with a team and an organization: %d %s", status, trim(body))
	}
	var created struct {
		UserID    string   `json:"user_id"`
		MaxBudget *float64 `json:"max_budget"`
	}
	if err := json.Unmarshal(body, &created); err != nil || created.UserID == "" {
		t.Fatalf("create did not return the new account: %s", trim(body))
	}

	// The budget was stored, rather than accepted and dropped.
	if created.MaxBudget == nil || *created.MaxBudget != 25.5 {
		t.Fatalf("the budget did not survive the create: %s", trim(body))
	}

	// The team membership exists, which is what the account needs to call
	// anything at all.
	status, body = c.f.call(admin, http.MethodGet, "/team/member_list?team_id="+c.teamA1.ID, nil)
	if status != http.StatusOK || !strings.Contains(string(body), "chain-created@example.com") {
		t.Fatalf("the new account is not on the team it was created in: %d %s", status, trim(body))
	}
	// As a team administrator, not a plain member.
	if !strings.Contains(string(body), iam.TeamAdmin) {
		t.Fatalf("the team role did not survive the create: %s", trim(body))
	}

	// The organization administration exists.
	status, body = c.f.call(admin, http.MethodGet, "/organization/info?organization_id="+c.orgA.ID, nil)
	if status != http.StatusOK || !strings.Contains(string(body), "chain-created@example.com") {
		t.Fatalf("the new account is not an administrator of the organization it named: %d %s", status, trim(body))
	}

	// And the account can sign in and reach its team, which is the only
	// end-to-end proof that the whole create landed.
	key := loginAs(t, c.f.base, "chain-created@example.com", "password123")
	if key == "" {
		t.Fatal("the created account cannot sign in")
	}
	teams := c.listIDs(actor{name: "created", user: &iam.User{ID: created.UserID}, key: key}, "/v2/team/list?page=1&page_size=100", "teams", "team_id")
	if !contains(teams, c.teamA1.ID) {
		t.Fatalf("the created account does not see the team it was created in: %s", idSet(teams))
	}
}

// TestVisibilityChainRejectsScopeTheCallerMayNotGrant checks that naming a team
// in a create does not become a way to place an account outside the caller's
// own reach, and that a create with an unusable scope leaves nothing behind.
func TestVisibilityChainRejectsScopeTheCallerMayNotGrant(t *testing.T) {
	c := newChain(t)

	// A team that does not exist is a rejected request, not a silent success.
	status, body := c.f.call(c.f.admin, http.MethodPost, "/user/new", mustJSONBytes(t, map[string]any{
		"user_email": "chain-bad-team@example.com",
		"password":   "password123",
		"team_id":    "team_does_not_exist",
	}))
	if status == http.StatusOK {
		t.Fatalf("a create naming a nonexistent team succeeded: %s", trim(body))
	}

	// The account was not left behind by the failed membership write.
	status, body = c.f.call(c.f.admin, http.MethodGet, "/user/filter/ui?user_email=chain-bad-team@example.com", nil)
	if status == http.StatusOK && strings.Contains(string(body), "chain-bad-team@example.com") {
		t.Fatalf("the rejected create left the account behind: %s", trim(body))
	}

	// A team role with no team is a malformed request rather than a member.
	status, body = c.f.call(c.f.admin, http.MethodPost, "/user/new", mustJSONBytes(t, map[string]any{
		"user_email": "chain-orphan-role@example.com",
		"password":   "password123",
		"team_role":  "admin",
	}))
	if status == http.StatusOK {
		t.Fatalf("a create with a team role and no team succeeded: %s", trim(body))
	}
}

// TestVisibilityChainUsageAndLogsFollowTheChain checks the reads that carry the
// most detail: usage and request logs.
//
// Two families exist and they answer different questions. /global/* is the
// platform-wide report and stays with the platform administrator. The scoped
// equivalent is /user/daily/activity, which narrows every row to what the caller
// may see. A member reads their own; a team administrator additionally reads the
// teams they administer; an organization administrator every team in their
// organization.
func TestVisibilityChainUsageAndLogsFollowTheChain(t *testing.T) {
	c := newChain(t)

	// The platform-wide report is the platform administrator's alone.
	for _, a := range []actor{c.orgAdminA, c.teamAdminA1, c.memberA1} {
		status, body := c.f.call(a, http.MethodGet, "/global/activity", nil)
		if status == http.StatusOK {
			t.Fatalf("%s read the platform-wide usage report: %s", a.label(), trim(body))
		}
	}
	status, body := c.f.call(c.f.admin, http.MethodGet, "/global/activity", nil)
	if status != http.StatusOK {
		t.Fatalf("the platform administrator cannot read the platform-wide report: %d %s", status, trim(body))
	}

	// A member may read their own usage and their own request logs. Refusing
	// these would be as wrong as leaking somebody else's.
	for _, path := range []string{"/user/daily/activity", "/spend/logs/v2?page=1&page_size=10"} {
		status, body := c.f.call(c.memberA1, http.MethodGet, path, nil)
		if status != http.StatusOK {
			t.Fatalf("a member cannot read their own %s: %d %s", path, status, trim(body))
		}
	}

	// A team administrator may read the usage of the team they run.
	status, body = c.f.call(c.teamAdminA1, http.MethodGet, "/user/daily/activity?team_id="+c.teamA1.ID, nil)
	if status != http.StatusOK {
		t.Fatalf("a team administrator cannot read their own team's usage: %d %s", status, trim(body))
	}

	// A team administrator may not read a sibling team's usage. The refusal and
	// an empty answer are both acceptable; rows are not.
	status, body = c.f.call(c.teamAdminA1, http.MethodGet, "/user/daily/activity?team_id="+c.teamA2.ID, nil)
	if status == http.StatusOK && !bodyIsEmptyList(t, body) {
		t.Fatalf("a team administrator read a sibling team's usage: %s", trim(body))
	}

	// An organization administrator reads every team of their organization.
	status, body = c.f.call(c.orgAdminA, http.MethodGet, "/user/daily/activity?team_id="+c.teamA2.ID, nil)
	if status != http.StatusOK {
		t.Fatalf("an organization administrator cannot read usage in their own organization: %d %s", status, trim(body))
	}

	// But not another organization's.
	status, body = c.f.call(c.orgAdminA, http.MethodGet, "/user/daily/activity?team_id="+c.teamB1.ID, nil)
	if status == http.StatusOK && !bodyIsEmptyList(t, body) {
		t.Fatalf("an organization administrator read another organization's usage: %s", trim(body))
	}

	// The audit trail is the platform's alone.
	for _, a := range []actor{c.orgAdminA, c.teamAdminA1, c.memberA1} {
		status, body := c.f.call(a, http.MethodGet, "/audit/logs", nil)
		if status == http.StatusOK {
			t.Fatalf("%s read the audit trail: %s", a.label(), trim(body))
		}
	}
	status, body = c.f.call(c.f.admin, http.MethodGet, "/audit/logs", nil)
	if status != http.StatusOK {
		t.Fatalf("the platform administrator cannot read the audit trail: %d %s", status, trim(body))
	}

	// And it records what happened. A password reset is the act this chain
	// cares most about being attributable, so it is performed here and looked
	// for rather than assumed.
	status, body = c.f.call(c.f.admin, http.MethodPost, "/user/set_password",
		mustJSONBytes(t, map[string]any{"user_id": c.memberA1.user.ID, "password": "audited-password-1"}))
	if status != http.StatusOK {
		t.Fatalf("the platform administrator could not reset a password: %d %s", status, trim(body))
	}
	status, body = c.f.call(c.f.admin, http.MethodGet, "/audit/logs?page_size=200", nil)
	if status != http.StatusOK {
		t.Fatalf("the platform administrator cannot read the audit trail: %d %s", status, trim(body))
	}
	if !strings.Contains(string(body), "user.password") {
		t.Fatalf("the audit trail does not record a password reset: %s", trim(body))
	}
	if !strings.Contains(string(body), c.f.admin.user.ID) {
		t.Fatalf("the audit record does not name the administrator who reset the password: %s", trim(body))
	}
}

// TestVisibilityChainNothingLeaksThroughTheAccountPicker checks the account
// picker, which is the one listing that returns names and addresses rather than
// ids and is therefore the most valuable thing to leak.
func TestVisibilityChainNothingLeaksThroughTheAccountPicker(t *testing.T) {
	c := newChain(t)

	// Only a platform administrator may enumerate accounts, so the picker is
	// closed to every other tier.
	for _, a := range []actor{c.orgAdminA, c.teamAdminA1, c.memberA1, c.orgAdminB} {
		status, body := c.f.call(a, http.MethodGet, "/user/filter/ui", nil)
		if status == http.StatusOK {
			t.Fatalf("%s enumerated accounts through the picker: %s", a.label(), trim(body))
		}
	}

	status, body := c.f.call(c.f.admin, http.MethodGet, "/user/filter/ui?user_email=chain-", nil)
	if status != http.StatusOK {
		t.Fatalf("the platform administrator cannot use the picker: %d %s", status, trim(body))
	}
	if !strings.Contains(string(body), c.memberB1.user.Email) {
		t.Fatalf("the platform administrator's picker does not find an account: %s", trim(body))
	}
}

// TestVisibilityChainBlockTakesEffectImmediately checks that a block anywhere on
// the path takes effect on the very next request, for every account under it,
// without waiting for anything to expire.
func TestVisibilityChainBlockTakesEffectImmediately(t *testing.T) {
	c := newChain(t)

	// Baseline: the member can see their team.
	teams := c.listIDs(c.memberA1, "/v2/team/list?page=1&page_size=100", "teams", "team_id")
	if !contains(teams, c.teamA1.ID) {
		t.Fatalf("baseline: member cannot see their team: %s", idSet(teams))
	}

	// Block the team through the platform administrator, who is the only tier
	// that may set a status.
	status, body := c.f.call(c.f.admin, http.MethodPost, "/team/update",
		mustJSONBytes(t, map[string]any{"team_id": c.teamA1.ID, "blocked": true}))
	if status != http.StatusOK {
		t.Fatalf("the platform administrator could not block a team: %d %s", status, trim(body))
	}

	// The member's next request reflects it. Nothing was cached.
	status, body = c.f.call(c.memberA1, http.MethodGet, "/team/info?team_id="+c.teamA1.ID, nil)
	if status == http.StatusOK {
		var probe map[string]any
		if json.Unmarshal(body, &probe) == nil {
			if info, ok := probe["team_info"].(map[string]any); ok {
				if blocked, _ := info["blocked"].(bool); !blocked {
					t.Fatalf("the block did not take effect on the next read: %s", trim(body))
				}
			}
		}
	}

	// Unblocking restores it, so the block is not a one-way door.
	status, body = c.f.call(c.f.admin, http.MethodPost, "/team/update",
		mustJSONBytes(t, map[string]any{"team_id": c.teamA1.ID, "blocked": false}))
	if status != http.StatusOK {
		t.Fatalf("the platform administrator could not lift a block: %d %s", status, trim(body))
	}
	teams = c.listIDs(c.memberA1, "/v2/team/list?page=1&page_size=100", "teams", "team_id")
	if !contains(teams, c.teamA1.ID) {
		t.Fatalf("lifting the block did not restore the team: %s", idSet(teams))
	}
}
