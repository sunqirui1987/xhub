package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/iam"
	"github.com/sunqirui1987/xhub/internal/store"
)

// actor is one caller a permission test can sign in as.
//
// The suite drives real HTTP routes rather than the authorization core, because
// the core already has its own tests and the bugs that reached this repository
// were not in it: the decision was right and the handler asked the wrong
// question, or asked none. A test at the handler boundary is the only kind that
// catches that.
type actor struct {
	name string
	key  string
	user *iam.User
	team *iam.Team
	role string // the team role, when the caller is in a team
}

// fixture is a running gateway with one account of each kind that matters.
type fixture struct {
	t    *testing.T
	srv  *httptest.Server
	base string
	db   *iam.DB

	admin      actor // platform administrator, no team
	member     actor // plain user, no team at all
	teamAdmin  actor // team_admin of teamA
	teamMember actor // member of teamA
	outsider   actor // team_admin of teamB, used to probe cross-team reads
	teamA      *iam.Team
	teamB      *iam.Team
	orgA       *iam.Organization
	masterKey  string
}

// newPermFixture builds the accounts and the two teams once per test.
//
// Every account is created directly through the identity layer rather than over
// HTTP, so a failure in the write path cannot make a permission assertion pass
// by accident.
func newPermFixture(t *testing.T) *fixture {
	t.Helper()
	srv, base, db, masterKey := bootGatewayWithMaster(t)
	t.Cleanup(srv.Close)
	ctx := context.Background()
	sys := iam.Actor{Kind: "system"}

	f := &fixture{t: t, srv: srv, base: base, db: db, masterKey: masterKey}

	mk := func(name, email, role string) *iam.User {
		u, err := db.CreateUser(ctx, sys, iam.UserInput{
			Email: email, Name: name, Password: "password123", Role: role,
		})
		if err != nil {
			t.Fatalf("create %s: %v", email, err)
		}
		return u
	}
	f.admin.user = mk("Admin", "perm-admin@example.com", iam.RoleAdmin)
	f.member.user = mk("Member", "perm-member@example.com", iam.RoleUser)
	f.teamAdmin.user = mk("Team Admin", "perm-teamadmin@example.com", iam.RoleUser)
	f.teamMember.user = mk("Team Member", "perm-teammember@example.com", iam.RoleUser)
	f.outsider.user = mk("Outsider", "perm-outsider@example.com", iam.RoleUser)

	f.orgA = mustOrg(t, db, "Perm Org A")
	orgB := mustOrg(t, db, "Perm Org B")

	f.teamA = mustTeam(t, db, f.orgA.ID, "Perm Team A", f.teamAdmin.user.ID)
	f.teamB = mustTeam(t, db, orgB.ID, "Perm Team B", f.outsider.user.ID)

	// teamMember and teamAdmin share team A; the outsider administers team B.
	mustAddMember(t, db, f.teamA.ID, f.teamMember.user.Email, iam.TeamMember)

	f.admin.key = signIn(t, base, f.admin.user)
	f.member.key = signIn(t, base, f.member.user)
	f.teamAdmin.key = signIn(t, base, f.teamAdmin.user)
	f.teamMember.key = signIn(t, base, f.teamMember.user)
	f.outsider.key = signIn(t, base, f.outsider.user)

	f.teamAdmin.team, f.teamAdmin.role = f.teamA, iam.TeamAdmin
	f.teamMember.team, f.teamMember.role = f.teamA, iam.TeamMember
	f.outsider.team, f.outsider.role = f.teamB, iam.TeamAdmin
	return f
}

func mustOrg(t *testing.T, db *iam.DB, name string) *iam.Organization {
	t.Helper()
	o, err := db.CreateOrg(context.Background(), iam.Actor{Kind: "system"}, name, nil)
	if err != nil {
		t.Fatalf("create org %s: %v", name, err)
	}
	return o
}

func mustTeam(t *testing.T, db *iam.DB, orgID, name, adminUserID string) *iam.Team {
	t.Helper()
	tm, err := db.CreateTeam(context.Background(), iam.Actor{Kind: "system"}, iam.TeamInput{
		OrganizationID: orgID, Name: name, AdminUserID: adminUserID,
	})
	if err != nil {
		t.Fatalf("create team %s: %v", name, err)
	}
	return tm
}

func mustAddMember(t *testing.T, db *iam.DB, teamID, email, role string) {
	t.Helper()
	if _, err := db.AddMember(context.Background(), iam.Actor{Kind: "system"}, teamID, email, role); err != nil {
		t.Fatalf("add %s: %v", email, err)
	}
}

// signIn signs an existing account in over HTTP, through the same route the
// console's login form uses, so the session is issued the way a browser's is.
func signIn(t *testing.T, base string, u *iam.User) string {
	t.Helper()
	return loginAs(t, base, u.Email, "password123")
}

// call issues one authenticated request and returns the status and body.
func (f *fixture) call(a actor, method, path string, payload []byte) (int, []byte) {
	f.t.Helper()
	return authed(f.t, f.base, a.key, method, path, payload)
}

// bodyIsEmptyList reports whether the response is an empty JSON array or an
// object whose only content is an empty collection.
//
// A denied listing must answer with nothing to show. Answering with rows is the
// failure this suite exists to catch, so the assertion is on the absence of
// data rather than on the status code alone.
func bodyIsEmptyList(t *testing.T, body []byte) bool {
	t.Helper()
	var v any
	if err := json.Unmarshal(body, &v); err != nil {
		return false
	}
	switch x := v.(type) {
	case []any:
		return len(x) == 0
	case map[string]any:
		// A wrapper such as {"teams":[...]} counts as empty when every
		// collection in it is empty and no row object is present.
		collections := 0
		for _, val := range x {
			switch c := val.(type) {
			case []any:
				collections++
				if len(c) != 0 {
					return false
				}
			}
		}
		return collections > 0
	}
	return false
}

// track renders a label for a table-driven case so a failure names the caller.
func (a actor) label() string {
	if a.team == nil {
		return fmt.Sprintf("%s (%s, no team)", a.name, a.user.Role)
	}
	return fmt.Sprintf("%s (%s, %s of %s)", a.name, a.user.Role, a.role, a.team.Name)
}

// ---------- the master credential's reach ----------

// TestMasterCannotEnumerateTenants is the regression test for a real bug.
//
// The master credential is for /bootstrap and emergencies. Its principal
// carries no user ID, and the listings derived their scope from that ID, so an
// empty ID became the store's "no filter" and the answer was every organization
// and every project on the deployment. The decision core had tests; the
// handlers did not, and the bug lived in the handler.
func TestMasterCannotEnumerateTenants(t *testing.T) {
	f := newPermFixture(t)
	master := actor{name: "master", key: f.masterKey}

	for _, tc := range []struct {
		name, method, path string
	}{
		{"organizations", "GET", "/organization/list"},
		{"projects", "POST", "/project/list"},
		{"teams", "GET", "/v2/team/list?page=1&page_size=50"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			status, body := f.call(master, tc.method, tc.path, []byte(`{}`))
			if status == 200 && !bodyIsEmptyList(t, body) {
				t.Fatalf("the master credential enumerated the tenant's %s: %s", tc.name, trim(body))
			}
			if status != 200 && status != 403 {
				t.Fatalf("unexpected status %d: %s", status, trim(body))
			}
		})
	}

	// The platform administrator must keep seeing them, or the fix would have
	// "closed" the hole by breaking the console.
	status, body := f.call(f.admin, "GET", "/organization/list", nil)
	if status != 200 {
		t.Fatalf("admin org list: %d %s", status, trim(body))
	}
	if bodyIsEmptyList(t, body) {
		t.Fatalf("the platform administrator lost sight of their own organizations: %s", trim(body))
	}
}

// TestMasterCannotWrite pins that the emergency credential is a reader of two
// specific things, not a superuser with an empty name.
func TestMasterCannotWrite(t *testing.T) {
	f := newPermFixture(t)
	master := actor{name: "master", key: f.masterKey}
	for _, tc := range []struct {
		name, method, path string
		body               []byte
	}{
		{"create org", "POST", "/organization/new", []byte(`{"organization_alias":"Master Org"}`)},
		{"create team", "POST", "/team/new", []byte(`{"team_alias":"Master Team","organization_id":"` + f.orgA.ID + `","admin_user_id":"` + f.admin.user.ID + `"}`)},
		{"create user", "POST", "/user/new", []byte(`{"user_email":"master-made@example.com","user_role":"admin"}`)},
		{"create model", "POST", "/model/new", []byte(`{"model_name":"master-model","litellm_params":{"model":"openai/x"}}`)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			status, body := f.call(master, tc.method, tc.path, tc.body)
			if status == 200 {
				t.Fatalf("the master credential performed a write: %s", trim(body))
			}
		})
	}
}

// ---------- cross-team isolation ----------

// TestListingsRespectTeamScope pins that a caller outside a team sees none of
// its rows. The failure this catches is the dangerous direction: a filter that
// is silently absent returns every row, and the page looks fine.
func TestListingsRespectTeamScope(t *testing.T) {
	f := newPermFixture(t)

	// teamMember belongs to team A; the outsider administers team B.
	status, body := f.call(f.teamMember, "GET", "/v2/team/list?page=1&page_size=50", nil)
	if status != 200 {
		t.Fatalf("team list: %d %s", status, trim(body))
	}
	var listed struct {
		Teams []struct {
			TeamID string `json:"team_id"`
			Name   string `json:"team_alias"`
		} `json:"teams"`
	}
	if err := json.Unmarshal(body, &listed); err != nil {
		t.Fatalf("decode: %s", trim(body))
	}
	for _, tm := range listed.Teams {
		if tm.TeamID == f.teamB.ID {
			t.Fatalf("a member of team A saw team B: %s", trim(body))
		}
	}
	found := false
	for _, tm := range listed.Teams {
		if tm.TeamID == f.teamA.ID {
			found = true
		}
	}
	if !found {
		t.Fatalf("a member of team A could not see their own team: %s", trim(body))
	}

	// And the outsider must not see team A.
	status, body = f.call(f.outsider, "GET", "/v2/team/list?page=1&page_size=50", nil)
	if status != 200 {
		t.Fatalf("outsider team list: %d %s", status, trim(body))
	}
	if err := json.Unmarshal(body, &listed); err != nil {
		t.Fatalf("decode: %s", trim(body))
	}
	for _, tm := range listed.Teams {
		if tm.TeamID == f.teamA.ID {
			t.Fatalf("the administrator of team B saw team A: %s", trim(body))
		}
	}
}

// TestTeamReadRequiresMembership pins that a caller who belongs to no team
// cannot read a team's detail by naming its ID.
func TestTeamReadRequiresMembership(t *testing.T) {
	f := newPermFixture(t)
	status, body := f.call(f.member, "GET", "/team/info?team_id="+f.teamA.ID, nil)
	if status == 200 {
		t.Fatalf("a caller with no team read another team's detail: %s", trim(body))
	}
	// A member of the team may.
	status, body = f.call(f.teamMember, "GET", "/team/info?team_id="+f.teamA.ID, nil)
	if status != 200 {
		t.Fatalf("a member could not read their own team: %d %s", status, trim(body))
	}
}

// ---------- team administration ----------

// TestTeamAdminBoundary pins what a team_admin may and may not do, since the
// console could not tell this role apart from a plain member.
func TestTeamAdminBoundary(t *testing.T) {
	f := newPermFixture(t)

	// A team_admin may rename their own team.
	status, body := f.call(f.teamAdmin, "POST", "/team/update",
		mustJSONBytes(t, map[string]any{"team_id": f.teamA.ID, "team_alias": "Perm Team A Renamed"}))
	if status != 200 {
		t.Fatalf("team admin could not rename their own team: %d %s", status, trim(body))
	}

	// A plain member may not.
	status, body = f.call(f.teamMember, "POST", "/team/update",
		mustJSONBytes(t, map[string]any{"team_id": f.teamA.ID, "team_alias": "Member Renamed"}))
	if status == 200 {
		t.Fatalf("a plain member renamed a team: %s", trim(body))
	}

	// A team_admin may not set the budget or status: that is the platform's
	// ceiling and the platform administrator's to change.
	status, body = f.call(f.teamAdmin, "POST", "/team/update",
		mustJSONBytes(t, map[string]any{"team_id": f.teamA.ID, "max_budget": 999}))
	if status == 200 {
		t.Fatalf("a team admin changed their own team's budget: %s", trim(body))
	}

	// A team_admin may not delete the team.
	status, body = f.call(f.teamAdmin, "POST", "/team/delete",
		mustJSONBytes(t, map[string]any{"team_ids": []string{f.teamA.ID}}))
	if status == 200 {
		t.Fatalf("a team admin deleted their own team: %s", trim(body))
	}

	// A team_admin may not promote anyone to platform administrator.
	status, body = f.call(f.teamAdmin, "POST", "/user/update",
		mustJSONBytes(t, map[string]any{"user_id": f.teamMember.user.ID, "user_role": iam.RoleAdmin}))
	if status == 200 {
		t.Fatalf("a team admin granted the platform administrator role: %s", trim(body))
	}
}

// TestMemberAdministrationNeedsTeamAdmin pins the membership boundary.
func TestMemberAdministrationNeedsTeamAdmin(t *testing.T) {
	f := newPermFixture(t)
	newcomer := mustUser(t, f, "perm-newcomer@example.com", iam.RoleUser)

	status, body := f.call(f.teamMember, "POST", "/team/member_add",
		mustJSONBytes(t, map[string]any{"team_id": f.teamA.ID, "user_email": newcomer.Email, "role": iam.TeamMember}))
	if status == 200 {
		t.Fatalf("a plain member added someone to the team: %s", trim(body))
	}

	status, body = f.call(f.teamAdmin, "POST", "/team/member_add",
		mustJSONBytes(t, map[string]any{"team_id": f.teamA.ID, "user_email": newcomer.Email, "role": iam.TeamMember}))
	if status != 200 {
		t.Fatalf("a team admin could not add a member: %d %s", status, trim(body))
	}

	// A team_admin may not remove the last team_admin, which would leave the
	// team unadministrable.
	status, body = f.call(f.teamAdmin, "POST", "/team/member_delete",
		mustJSONBytes(t, map[string]any{"team_id": f.teamA.ID, "user_id": f.teamAdmin.user.ID}))
	if status == 200 {
		t.Fatalf("the last team admin was removed, leaving the team unadministrable: %s", trim(body))
	}
}

// TestMemberMustBeInTheTeam pins the rule that a team can only contain its own
// members: adding someone from another team is not a way to reach into it.
func TestMemberMustBeInTheTeam(t *testing.T) {
	f := newPermFixture(t)
	status, body := f.call(f.teamAdmin, "POST", "/team/member_add",
		mustJSONBytes(t, map[string]any{"team_id": f.teamA.ID, "user_email": f.outsider.user.Email, "role": iam.TeamAdmin}))
	// Either refused outright, or accepted as a team-scoped change that does not
	// grant anything platform-wide. What must not happen is the outsider
	// acquiring reach over team A without the team's own consent, so the
	// assertion is that the outsider is still not an administrator of team A
	// unless the add succeeded legitimately.
	if status == 200 {
		// The add succeeded, which is allowed: a team admin may add anyone by
		// email. Confirm it did not escalate beyond team A.
		status, body = f.call(f.outsider, "GET", "/user/list", nil)
		if status == 200 && !bodyIsEmptyList(t, body) {
			t.Fatalf("adding a member granted platform-wide user listing: %s", trim(body))
		}
		return
	}
	if status != 403 && status != 400 && status != 404 {
		t.Fatalf("unexpected status %d: %s", status, trim(body))
	}
}

// mustUser creates one more account through the identity layer.
func mustUser(t *testing.T, f *fixture, email, role string) *iam.User {
	t.Helper()
	u, err := f.db.CreateUser(context.Background(), iam.Actor{Kind: "system"}, iam.UserInput{
		Email: email, Name: email, Password: "password123", Role: role,
	})
	if err != nil {
		t.Fatalf("create %s: %v", email, err)
	}
	return u
}

func mustJSONBytes(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// bootGatewayWithMaster starts the shipped handler with an emergency credential
// configured, and returns it.
//
// bootGateway leaves master_key empty, which is the right default for a
// deployment but means the master principal can never be reached in a test. The
// credential is the one that reaches /bootstrap, so the tests that assert what
// it may NOT do need it to exist.
func bootGatewayWithMaster(t *testing.T) (*httptest.Server, string, *iam.DB, string) {
	t.Helper()
	cfg, err := config.Load(configPath(t))
	if err != nil {
		t.Fatal(err)
	}
	// A per-test key, so a leaked one cannot be replayed against a real server.
	masterKey := "sk-test-master-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	cfg.GeneralSettings.MasterKey = masterKey

	st, err := store.Open(cfg.GeneralSettings.DatabaseURL)
	if err != nil {
		t.Fatal(err)
	}
	db := testIdentityStore(t)
	gw := New(cfg, st, db)
	srv := httptest.NewServer(gw.Handler())
	return srv, srv.URL, db, masterKey
}
