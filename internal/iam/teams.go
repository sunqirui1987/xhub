package iam

import (
	"context"
	"sort"

	"xorm.io/xorm"

	"github.com/sunqirui1987/xhub/internal/logx"
)

// teamRow reads one team together with the caller's role, if any.
type teamRow struct {
	ID             string   `xorm:"'id'"`
	OrganizationID string   `xorm:"'organization_id'"`
	Name           string   `xorm:"'name'"`
	Description    string   `xorm:"'description'"`
	Status         string   `xorm:"'status'"`
	MaxBudget      *float64 `xorm:"'max_budget'"`
	Spend          float64  `xorm:"'spend'"`
	CreatedAt      Time     `xorm:"'created_at'"`
	UpdatedAt      Time     `xorm:"'updated_at'"`
	Role           string   `xorm:"'role'"`
}

// ---------- organizations ----------

// CreateOrg inserts an organization.
func (db *DB) CreateOrg(ctx context.Context, by Actor, name string, maxBudget *float64) (*Organization, error) {
	var out *Organization
	err := db.tx(ctx, func(s *xorm.Session) error {
		o := Organization{ID: newID(), Name: name, Status: StatusActive, MaxBudget: maxBudget}
		if _, err := s.Insert(&o); err != nil {
			return err
		}
		out = &o
		return writeAudit(s, by, Audit{Action: "org.create", ObjectType: "organization", ObjectID: o.ID})
	})
	return out, err
}

// OrgsByIDs loads the organizations named by ids. An empty list is an empty
// map, not every organization.
func (db *DB) OrgsByIDs(ctx context.Context, ids []string) (map[string]Organization, error) {
	out := map[string]Organization{}
	if len(ids) == 0 {
		return out, nil
	}
	s := db.session(ctx)
	defer s.Close()
	var rows []Organization
	if err := s.In("id", ids).Find(&rows); err != nil {
		return nil, mapErr(err)
	}
	for _, o := range rows {
		out[o.ID] = o
	}
	return out, nil
}

// GetOrg loads one organization.
func (db *DB) GetOrg(ctx context.Context, id string) (*Organization, error) {
	s := db.session(ctx)
	defer s.Close()
	return getOrg(ctx, s, id)
}

func getOrg(_ context.Context, s *xorm.Session, id string) (*Organization, error) {
	var o Organization
	if err := get(s.Where("id = ?", id), &o); err != nil {
		return nil, err
	}
	return &o, nil
}

// ListOrgs returns every organization, or only those that own one of the
// user's teams when userID is set.
func (db *DB) ListOrgs(ctx context.Context, userID string) ([]Organization, error) {
	s := db.session(ctx)
	defer s.Close()
	if userID != "" {
		// A person sees an organization by belonging to one of its teams, or by
		// administering it. The two are independent.
		s = s.Where(
			"id IN (SELECT t.organization_id FROM teams t INNER JOIN team_members tm ON tm.team_id = t.id WHERE tm.user_id = ?) OR id IN (SELECT organization_id FROM organization_members WHERE user_id = ? AND role = ?)",
			userID, userID, OrgAdmin,
		)
	}
	var out []Organization
	err := s.Asc("name").Find(&out)
	return out, err
}

// OrgUpdate changes an organization; nil fields keep their value.
type OrgUpdate struct {
	Name      *string
	Status    *string
	MaxBudget **float64
}

// UpdateOrg applies an OrgUpdate.
func (db *DB) UpdateOrg(ctx context.Context, by Actor, id string, in OrgUpdate) (*Organization, error) {
	var out *Organization
	err := db.tx(ctx, func(s *xorm.Session) error {
		cur, err := getOrg(ctx, s, id) // Get takes the primary-key lock path below
		if err != nil {
			return err
		}
		if in.Name != nil {
			cur.Name = *in.Name
		}
		if in.Status != nil {
			cur.Status = *in.Status
		}
		if in.MaxBudget != nil {
			cur.MaxBudget = *in.MaxBudget
		}
		if _, err := s.ID(id).Cols("name", "status", "max_budget").Update(cur); err != nil {
			return err
		}
		if out, err = getOrg(ctx, s, id); err != nil {
			return err
		}
		return writeAudit(s, by, Audit{Action: "org.update", ObjectType: "organization", ObjectID: id})
	})
	return out, err
}

// DeleteOrg refuses while teams remain (foreign key RESTRICT).
func (db *DB) DeleteOrg(ctx context.Context, by Actor, id string) error {
	return db.tx(ctx, func(s *xorm.Session) error {
		if err := affected(s.ID(id).Delete(&Organization{})); err != nil {
			return err
		}
		return writeAudit(s, by, Audit{Action: "org.delete", ObjectType: "organization", ObjectID: id})
	})
}

// ---------- teams ----------

// TeamInput creates a team together with its first team administrator.
//
// Models is the team's model set. An empty list means no restriction rather
// than no models, so a team created without a choice reaches what the
// deployment offers; narrowing it later is what limits it.
type TeamInput struct {
	OrganizationID string
	Name           string
	Description    string
	Models         []string
	MaxBudget      *float64
	AdminUserID    string
}

// CreateTeam inserts the team and its first team_admin in one transaction.
func (db *DB) CreateTeam(ctx context.Context, by Actor, in TeamInput) (*Team, error) {
	if in.AdminUserID == "" {
		return nil, ErrInvalid
	}
	var out *Team
	err := db.tx(ctx, func(s *xorm.Session) error {
		if err := requireActiveUser(s, in.AdminUserID); err != nil {
			return err
		}
		t := Team{ID: newID(), OrganizationID: in.OrganizationID, Name: in.Name, Description: in.Description,
			Status: StatusActive, Models: nonNil(in.Models), MaxBudget: in.MaxBudget}
		if _, err := s.Insert(&t); err != nil {
			return err
		}
		if _, err := s.Insert(&TeamMembership{TeamID: t.ID, UserID: in.AdminUserID, Role: TeamAdmin}); err != nil {
			return err
		}
		out = &t
		return writeAudit(s, by, Audit{Action: "team.create", ObjectType: "team", ObjectID: t.ID, TeamID: t.ID,
			Detail: map[string]any{"organization_id": in.OrganizationID, "admin": in.AdminUserID, "models": len(in.Models)}})
	})
	return out, err
}

// TeamsByIDs loads the teams named by ids. An empty list is an empty map, not
// every team.
func (db *DB) TeamsByIDs(ctx context.Context, ids []string) (map[string]Team, error) {
	out := map[string]Team{}
	if len(ids) == 0 {
		return out, nil
	}
	s := db.session(ctx)
	defer s.Close()
	var rows []Team
	if err := s.In("id", ids).Find(&rows); err != nil {
		return nil, mapErr(err)
	}
	for _, team := range rows {
		out[team.ID] = team
	}
	return out, nil
}

// GetTeam loads one team.
func (db *DB) GetTeam(ctx context.Context, id string) (*Team, error) {
	s := db.session(ctx)
	defer s.Close()
	return getTeam(ctx, s, id)
}

func getTeam(_ context.Context, s *xorm.Session, id string) (*Team, error) {
	var t Team
	if err := get(s.Where("id = ?", id), &t); err != nil {
		return nil, err
	}
	return &t, nil
}

// TeamWithRole is a team as seen by one user.
type TeamWithRole struct {
	Team
	Role string `json:"role"`
}

// ListTeams returns every team for a platform administrator (userID empty),
// or just the user's teams with their role.
func (db *DB) ListTeams(ctx context.Context, userID, organizationID string) ([]TeamWithRole, error) {
	s := db.session(ctx)
	defer s.Close()
	var rows []teamRow
	if userID == "" {
		err := s.Table("teams").
			Select("id, organization_id, name, description, status, max_budget, spend, created_at, updated_at, '' AS role").
			Where("? = '' OR organization_id = ?", organizationID, organizationID).
			Asc("name", "id").Find(&rows)
		if err != nil {
			return nil, err
		}
	} else {
		err := s.Table("teams").Alias("t").
			Join("INNER", "team_members m", "m.team_id = t.id").
			Select("t.id AS id, t.organization_id AS organization_id, t.name AS name, t.description AS description, t.status AS status, "+
				"t.max_budget AS max_budget, t.spend AS spend, t.created_at AS created_at, t.updated_at AS updated_at, m.role AS role").
			Where("m.user_id = ? AND (? = '' OR t.organization_id = ?)", userID, organizationID, organizationID).
			Asc("t.name", "t.id").Find(&rows)
		if err != nil {
			return nil, err
		}
	}
	out := make([]TeamWithRole, 0, len(rows))
	for _, r := range rows {
		out = append(out, TeamWithRole{
			Team: Team{ID: r.ID, OrganizationID: r.OrganizationID, Name: r.Name, Description: r.Description,
				Status: r.Status, MaxBudget: r.MaxBudget, Spend: r.Spend,
				CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt},
			Role: r.Role,
		})
	}
	return out, nil
}

// UpdateTeamProfile is what a team administrator may change.
func (db *DB) UpdateTeamProfile(ctx context.Context, by Actor, id string, name, description *string) (*Team, error) {
	var out *Team
	err := db.tx(ctx, func(s *xorm.Session) error {
		if err := lockTeam(ctx, s, id); err != nil {
			return err
		}
		var cols []string
		var patch Team
		if name != nil {
			patch.Name, cols = *name, append(cols, "name")
		}
		if description != nil {
			patch.Description, cols = *description, append(cols, "description")
		}
		if _, err := s.ID(id).Cols(cols...).Update(&patch); err != nil {
			return err
		}
		var err error
		if out, err = getTeam(ctx, s, id); err != nil {
			return err
		}
		return writeAudit(s, by, Audit{Action: "team.profile", ObjectType: "team", ObjectID: id, TeamID: id})
	})
	return out, err
}

// AdminUpdateTeam changes status, budget and model set; platform administrators
// only. The model list sits at this level rather than at the team
// administrator's because it is the team's ceiling: a team administrator who
// could widen it would be granting their own team reach the platform did not
// give them.
func (db *DB) AdminUpdateTeam(ctx context.Context, by Actor, id string, status *string, maxBudget **float64, models *[]string) (*Team, error) {
	var out *Team
	err := db.tx(ctx, func(s *xorm.Session) error {
		if err := lockTeam(ctx, s, id); err != nil {
			return err
		}
		var cols []string
		var patch Team
		if status != nil {
			patch.Status, cols = *status, append(cols, "status")
		}
		if maxBudget != nil {
			patch.MaxBudget, cols = *maxBudget, append(cols, "max_budget")
		}
		if models != nil {
			// The names are stored as given. This package has no view of which
			// deployments exist — that is the gateway's model table — so it
			// cannot tell a typo from a model added a moment later, and a name
			// that matches nothing simply grants nothing.
			patch.Models, cols = nonNil(*models), append(cols, "models")
		}
		if _, err := s.ID(id).Cols(cols...).Update(&patch); err != nil {
			return err
		}
		var err error
		if out, err = getTeam(ctx, s, id); err != nil {
			return err
		}
		return writeAudit(s, by, Audit{Action: "team.admin_update", ObjectType: "team", ObjectID: id, TeamID: id,
			Detail: map[string]any{"status": out.Status}})
	})
	return out, err
}

// MoveTeam re-parents a team. Access-group assignments from the previous
// organization are removed first, which cascades to project and key
// narrowing; a failure leaves everything untouched.
func (db *DB) MoveTeam(ctx context.Context, by Actor, id, organizationID string) (*Team, error) {
	var out *Team
	err := db.tx(ctx, func(s *xorm.Session) error {
		if err := lockTeam(ctx, s, id); err != nil {
			return err
		}
		if _, err := s.ID(id).Cols("organization_id").Update(&Team{OrganizationID: organizationID}); err != nil {
			return err
		}
		var err error
		if out, err = getTeam(ctx, s, id); err != nil {
			return err
		}
		return writeAudit(s, by, Audit{Action: "team.move", ObjectType: "team", ObjectID: id, TeamID: id,
			Detail: map[string]any{"organization_id": organizationID}})
	})
	return out, err
}

// DeleteTeam cascades to members, projects, keys and access-group assignments.
func (db *DB) DeleteTeam(ctx context.Context, by Actor, id string) error {
	return db.tx(ctx, func(s *xorm.Session) error {
		if err := affected(s.ID(id).Delete(&Team{})); err != nil {
			return err
		}
		return writeAudit(s, by, Audit{Action: "team.delete", ObjectType: "team", ObjectID: id, TeamID: id})
	})
}

// lockOrg takes the organization row lock that serializes its membership
// changes, so two administrators cannot each read the roster and write back a
// result that loses the other's change.
func lockOrg(_ context.Context, s *xorm.Session, id string) error {
	var o Organization
	return get(s.SQL("SELECT * FROM organizations WHERE id = ? FOR UPDATE", id), &o)
}

// lockTeam takes the team row lock that serializes membership changes.
func lockTeam(_ context.Context, s *xorm.Session, id string) error {
	var t Team
	return get(s.SQL("SELECT * FROM teams WHERE id = ? FOR UPDATE", id), &t)
}

func requireActiveUser(s *xorm.Session, id string) error {
	var u User
	if err := get(s.Where("id = ?", id), &u); err != nil {
		return err
	}
	if u.Status != StatusActive {
		return ErrInactive
	}
	return nil
}

// keepTeamAdmin fails when no other active team_admin remains.
// The caller must already hold the team row lock.
func keepTeamAdmin(_ context.Context, s *xorm.Session, teamID, excluding string) error {
	n, err := s.Table("team_members").Alias("m").
		Join("INNER", "users u", "u.id = m.user_id").
		Where("m.team_id = ? AND m.role = ? AND u.status = ? AND m.user_id <> ?", teamID, TeamAdmin, StatusActive, excluding).
		Count()
	if err != nil {
		return mapErr(err)
	}
	if n == 0 {
		logx.Error("iam refusing to leave team %s without an active team_admin", teamID)
		return ErrLastAdmin
	}
	return nil
}

// ---------- membership ----------

// Membership is one of a user's team memberships, carrying the team's
// organization so the authorization layer can answer "does this user belong to
// this organization" without a second query.
//
// The xorm tags are load-bearing, not decoration: xorm maps result columns by
// its own tag, so a struct carrying only json tags scans every column into the
// zero value without reporting an error.
type Membership struct {
	TeamID         string `xorm:"'team_id'" json:"team_id"`
	OrganizationID string `xorm:"'organization_id'" json:"organization_id"`
	Role           string `xorm:"'role'" json:"role"`
}

// MemberTeams returns the user's memberships with the team role and the team's
// organization. This is the only membership source the authorization layer
// reads: there is no mirror of members anywhere else.
func (db *DB) MemberTeams(ctx context.Context, userID string) ([]Membership, error) {
	s := db.session(ctx)
	defer s.Close()
	var out []Membership
	err := s.Table("team_members").Alias("m").
		Join("INNER", "teams t", "t.id = m.team_id").
		Select("m.team_id AS team_id, t.organization_id AS organization_id, m.role AS role").
		Where("m.user_id = ?", userID).Asc("m.team_id").Find(&out)
	if err != nil {
		return nil, mapErr(err)
	}
	return out, nil
}

// ListOrgAdmins returns the organization administrators with public account fields.
func (db *DB) ListOrgAdmins(ctx context.Context, orgID string) ([]Member, error) {
	s := db.session(ctx)
	defer s.Close()
	var out []Member
	err := s.Table("organization_members").Alias("m").
		Join("INNER", "users u", "u.id = m.user_id").
		Select("u.id AS user_id, u.email AS email, u.name AS name, m.role AS role, u.status AS status").
		Where("m.organization_id = ? AND m.role = ?", orgID, OrgAdmin).
		Asc("u.email").Find(&out)
	return out, err
}

// AddOrgAdmin grants organization administration to an existing active account,
// found by exact email. An unknown and a disabled account both return
// ErrNotFound. Granting it again is a no-op success.
func (db *DB) AddOrgAdmin(ctx context.Context, by Actor, orgID, email string) (*Member, error) {
	var out Member
	err := db.tx(ctx, func(s *xorm.Session) error {
		if err := lockOrg(ctx, s, orgID); err != nil {
			return err
		}
		var u User
		err := get(s.Where("lower(email) = ? AND status = ?", normEmail(email), StatusActive), &u)
		if err != nil {
			if err == ErrNotFound {
				return ErrNotFound
			}
			return err
		}
		if _, err := s.Where("organization_id = ? AND user_id = ?", orgID, u.ID).
			Delete(&OrganizationMembership{}); err != nil {
			return err
		}
		if _, err := s.Insert(&OrganizationMembership{OrganizationID: orgID, UserID: u.ID, Role: OrgAdmin}); err != nil {
			return err
		}
		out = Member{UserID: u.ID, Email: u.Email, Name: u.Name, Role: OrgAdmin, Status: u.Status}
		return writeAudit(s, by, Audit{Action: "org.admin_add", ObjectType: "organization", ObjectID: orgID,
			Detail: map[string]any{"user_id": u.ID}})
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// RemoveOrgAdmin revokes organization administration. An organization may have
// no administrator: a platform administrator still reaches it.
func (db *DB) RemoveOrgAdmin(ctx context.Context, by Actor, orgID, userID string) error {
	return db.tx(ctx, func(s *xorm.Session) error {
		if err := lockOrg(ctx, s, orgID); err != nil {
			return err
		}
		if err := affected(s.Where("organization_id = ? AND user_id = ? AND role = ?", orgID, userID, OrgAdmin).
			Delete(&OrganizationMembership{})); err != nil {
			return err
		}
		return writeAudit(s, by, Audit{Action: "org.admin_remove", ObjectType: "organization", ObjectID: orgID,
			Detail: map[string]any{"user_id": userID}})
	})
}

// ListVisibleTeams returns the teams a person may see: the ones they belong to,
// plus every team in an organization they administer. The role is their team
// role, empty when they administer the organization without joining the team.
func (db *DB) ListVisibleTeams(ctx context.Context, userID, organizationID string) ([]TeamWithRole, error) {
	if userID == "" {
		return db.ListTeams(ctx, "", organizationID)
	}
	s := db.session(ctx)
	defer s.Close()
	var rows []teamRow
	err := s.Table("teams").Alias("t").
		Join("LEFT", "team_members m", "m.team_id = t.id AND m.user_id = ?", userID).
		Select("t.id AS id, t.organization_id AS organization_id, t.name AS name, t.description AS description, t.status AS status, "+
			"t.max_budget AS max_budget, t.spend AS spend, t.created_at AS created_at, t.updated_at AS updated_at, COALESCE(m.role, '') AS role").
		Where("(m.user_id IS NOT NULL OR t.organization_id IN (SELECT organization_id FROM organization_members WHERE user_id = ? AND role = ?)) AND (? = '' OR t.organization_id = ?)",
			userID, OrgAdmin, organizationID, organizationID).
		Asc("t.name", "t.id").Find(&rows)
	if err != nil {
		return nil, err
	}
	out := make([]TeamWithRole, 0, len(rows))
	for _, r := range rows {
		out = append(out, TeamWithRole{
			Team: Team{ID: r.ID, OrganizationID: r.OrganizationID, Name: r.Name, Description: r.Description,
				Status: r.Status, MaxBudget: r.MaxBudget, Spend: r.Spend,
				CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt},
			Role: r.Role,
		})
	}
	return out, nil
}

// OversightTeamIDs lists the teams whose contents this person may see in full:
// teams they administer, and every team in an organization they administer.
// Membership alone does not put a team here.
func (db *DB) OversightTeamIDs(ctx context.Context, userID string) ([]string, error) {
	if userID == "" {
		return nil, nil
	}
	s := db.session(ctx)
	defer s.Close()
	var rows []struct {
		ID string `xorm:"'id'"`
	}
	err := s.SQL(`SELECT team_id AS id FROM team_members WHERE user_id = ? AND role = ?
		UNION
		SELECT id FROM teams WHERE organization_id IN (
			SELECT organization_id FROM organization_members WHERE user_id = ? AND role = ?
		)`, userID, TeamAdmin, userID, OrgAdmin).Find(&rows)
	if err != nil {
		return nil, mapErr(err)
	}
	out := make([]string, 0, len(rows))
	seen := map[string]struct{}{}
	for _, r := range rows {
		if r.ID == "" {
			continue
		}
		if _, ok := seen[r.ID]; ok {
			continue
		}
		seen[r.ID] = struct{}{}
		out = append(out, r.ID)
	}
	sort.Strings(out)
	return out, nil
}

// TeamIDsByOrg lists every team that belongs to one of the organizations.
func (db *DB) TeamIDsByOrg(ctx context.Context, orgIDs []string) ([]string, error) {
	if len(orgIDs) == 0 {
		return nil, nil
	}
	s := db.session(ctx)
	defer s.Close()
	var rows []struct {
		ID string `xorm:"'id'"`
	}
	if err := s.Table("teams").In("organization_id", orgIDs).Cols("id").Asc("id").Find(&rows); err != nil {
		return nil, mapErr(err)
	}
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.ID)
	}
	return out, nil
}

// AdminOrgs lists the organizations this person administers.
//
// It is separate from the team memberships because organization administration
// is not derived from them: someone can administer an organization without
// belonging to any of its teams, and can belong to a team without administering
// its organization.
func (db *DB) AdminOrgs(ctx context.Context, userID string) ([]string, error) {
	if userID == "" {
		return nil, nil
	}
	s := db.session(ctx)
	defer s.Close()
	var out []string
	err := s.Table("organization_members").Cols("organization_id").
		Where("user_id = ?", userID).And("role = ?", OrgAdmin).
		Asc("organization_id").Find(&out)
	if err != nil {
		return nil, mapErr(err)
	}
	return out, nil
}

// SetOrgAdmin grants or revokes organization administration for one person.
func (db *DB) SetOrgAdmin(ctx context.Context, by Actor, orgID, userID string, admin bool) error {
	return db.tx(ctx, func(s *xorm.Session) error {
		if err := lockOrg(ctx, s, orgID); err != nil {
			return err
		}
		if _, err := s.Where("organization_id = ? AND user_id = ?", orgID, userID).
			Delete(&OrganizationMembership{}); err != nil {
			return err
		}
		role := OrgMember
		if admin {
			role = OrgAdmin
		}
		if _, err := s.Insert(&OrganizationMembership{OrganizationID: orgID, UserID: userID, Role: role}); err != nil {
			return err
		}
		return writeAudit(s, by, Audit{Action: "org.member_set", ObjectType: "organization", ObjectID: orgID,
			Detail: map[string]any{"user_id": userID, "role": role}})
	})
}

// OrgAdmins lists the people who administer one organization.
func (db *DB) OrgAdmins(ctx context.Context, orgID string) ([]string, error) {
	s := db.session(ctx)
	defer s.Close()
	var out []string
	err := s.Table("organization_members").Cols("user_id").
		Where("organization_id = ?", orgID).And("role = ?", OrgAdmin).
		Asc("user_id").Find(&out)
	if err != nil {
		return nil, mapErr(err)
	}
	return out, nil
}

// Memberships returns team ID → team role, for callers that only need the role.
func (db *DB) Memberships(ctx context.Context, userID string) (map[string]string, error) {
	rows, err := db.MemberTeams(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(rows))
	for _, r := range rows {
		out[r.TeamID] = r.Role
	}
	return out, nil
}

// Member is one team membership with the public fields of the account.
type Member struct {
	UserID string `xorm:"'user_id'" json:"user_id"`
	Email  string `xorm:"'email'" json:"email"`
	Name   string `xorm:"'name'" json:"name"`
	Role   string `xorm:"'role'" json:"role"`
	Status string `xorm:"'status'" json:"status"`
}

// ListMembers returns the team's members with public account fields only.
func (db *DB) ListMembers(ctx context.Context, teamID string) ([]Member, error) {
	s := db.session(ctx)
	defer s.Close()
	var out []Member
	err := s.Table("team_members").Alias("m").
		Join("INNER", "users u", "u.id = m.user_id").
		Select("u.id AS user_id, u.email AS email, u.name AS name, m.role AS role, u.status AS status").
		Where("m.team_id = ?", teamID).Asc("u.email").Find(&out)
	return out, err
}

// AddMember adds an existing active account by exact email. An unknown and a
// disabled account both return ErrNotFound, so callers cannot enumerate users.
func (db *DB) AddMember(ctx context.Context, by Actor, teamID, email, role string) (*Member, error) {
	if role == "" {
		role = TeamMember
	}
	var out Member
	err := db.tx(ctx, func(s *xorm.Session) error {
		if err := lockTeam(ctx, s, teamID); err != nil {
			return err
		}
		var u User
		err := get(s.Where("lower(email) = ? AND status = ?", normEmail(email), StatusActive), &u)
		if err != nil {
			if err == ErrNotFound {
				// Keep the answer identical for unknown, disabled and malformed
				// addresses so membership cannot be used to probe accounts.
				return ErrNotFound
			}
			return err
		}
		if _, err := s.Insert(&TeamMembership{TeamID: teamID, UserID: u.ID, Role: role}); err != nil {
			return err
		}
		out = Member{UserID: u.ID, Email: u.Email, Name: u.Name, Role: role, Status: u.Status}
		return writeAudit(s, by, Audit{Action: "member.add", ObjectType: "team_member", ObjectID: u.ID, TeamID: teamID,
			Detail: map[string]any{"role": role}})
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// SetMemberRole changes a team role, protecting the last team_admin.
func (db *DB) SetMemberRole(ctx context.Context, by Actor, teamID, userID, role string) error {
	if role != TeamAdmin && role != TeamMember {
		return ErrInvalid
	}
	return db.tx(ctx, func(s *xorm.Session) error {
		if err := lockTeam(ctx, s, teamID); err != nil {
			return err
		}
		cur, err := membership(ctx, s, teamID, userID)
		if err != nil {
			return err
		}
		if cur.Role == TeamAdmin && role != TeamAdmin {
			if err := keepTeamAdmin(ctx, s, teamID, userID); err != nil {
				return err
			}
		}
		if _, err := s.Where("team_id = ? AND user_id = ?", teamID, userID).
			Cols("role").Update(&TeamMembership{Role: role}); err != nil {
			return err
		}
		return writeAudit(s, by, Audit{Action: "member.role", ObjectType: "team_member", ObjectID: userID, TeamID: teamID,
			Detail: map[string]any{"from": cur.Role, "to": role}})
	})
}

// RemoveMember deletes a membership and revokes the user's personal keys bound
// to this team, protecting the last team_admin.
func (db *DB) RemoveMember(ctx context.Context, by Actor, teamID, userID string) error {
	return db.tx(ctx, func(s *xorm.Session) error {
		if err := lockTeam(ctx, s, teamID); err != nil {
			return err
		}
		cur, err := membership(ctx, s, teamID, userID)
		if err != nil {
			return err
		}
		if cur.Role == TeamAdmin {
			if err := keepTeamAdmin(ctx, s, teamID, userID); err != nil {
				return err
			}
		}
		if err := affected(s.Where("team_id = ? AND user_id = ?", teamID, userID).Delete(&TeamMembership{})); err != nil {
			return err
		}
		if _, err := revokePersonalKeys(ctx, s, "team_id = ? AND user_id = ?", teamID, userID); err != nil {
			return err
		}
		return writeAudit(s, by, Audit{Action: "member.remove", ObjectType: "team_member", ObjectID: userID, TeamID: teamID})
	})
}

func membership(_ context.Context, s *xorm.Session, teamID, userID string) (*TeamMembership, error) {
	var m TeamMembership
	if err := get(s.Where("team_id = ? AND user_id = ?", teamID, userID), &m); err != nil {
		return nil, err
	}
	return &m, nil
}

// revokePersonalKeys ends personal keys matching the condition. Service keys
// are never touched: they belong to the team, not to a person.
func revokePersonalKeys(_ context.Context, s *xorm.Session, cond string, args ...any) (int64, error) {
	return s.Where("owner_type = ? AND status <> ?", OwnerPersonal, StatusRevoked).
		And(cond, args...).
		Cols("status").
		Update(&APIKey{Status: StatusRevoked})
}

// ---------- projects ----------

// projectRead is one project row joined with its team's organization and its
// access-group narrowing.
type projectRead struct {
	ID             string   `xorm:"'id'"`
	TeamID         string   `xorm:"'team_id'"`
	OrganizationID string   `xorm:"'organization_id'"`
	Name           string   `xorm:"'name'"`
	Status         string   `xorm:"'status'"`
	Models         []string `xorm:"json 'models'"`
	MaxBudget      *float64 `xorm:"'max_budget'"`
	Spend          float64  `xorm:"'spend'"`
	CreatedAt      Time     `xorm:"'created_at'"`
	UpdatedAt      Time     `xorm:"'updated_at'"`
}

func (r projectRead) project() Project {
	return Project{ID: r.ID, TeamID: r.TeamID, OrganizationID: r.OrganizationID, Name: r.Name, Status: r.Status,
		Models: nonNil(r.Models), MaxBudget: r.MaxBudget, Spend: r.Spend, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt}
}

const projectSelect = `p.id AS id, p.team_id AS team_id, t.organization_id AS organization_id, p.name AS name,
	p.status AS status, p.models AS models, p.max_budget AS max_budget, p.spend AS spend, p.created_at AS created_at, p.updated_at AS updated_at`

func (db *DB) projectSession(ctx context.Context) *xorm.Session {
	return db.session(ctx).Table("projects").Alias("p").
		Join("INNER", "teams t", "t.id = p.team_id").Select(projectSelect)
}

// GetProject loads one project with its organization derived from the team.
func (db *DB) GetProject(ctx context.Context, id string) (*Project, error) {
	s := db.session(ctx)
	defer s.Close()
	return getProject(ctx, s, id)
}

func getProject(ctx context.Context, s *xorm.Session, id string) (*Project, error) {
	s = s.Table("projects").Alias("p").Join("INNER", "teams t", "t.id = p.team_id").Select(projectSelect).Where("p.id = ?", id)
	var row projectRead
	ok, err := s.Get(&row)
	if err != nil {
		return nil, mapErr(err)
	}
	if !ok {
		return nil, ErrNotFound
	}
	p := row.project()
	return &p, nil
}

// ListProjects returns the projects of the given teams, or of every team when
// teamIDs is nil (platform administrators). An empty, non-nil list returns
// nothing rather than everything.
func (db *DB) ListProjects(ctx context.Context, teamIDs []string) ([]Project, error) {
	if teamIDs != nil && len(teamIDs) == 0 {
		return nil, nil
	}
	s := db.projectSession(ctx)
	defer s.Close()
	if teamIDs != nil {
		s = s.In("p.team_id", teamIDs)
	}
	var rows []projectRead
	if err := s.Asc("p.name", "p.id").Find(&rows); err != nil {
		return nil, mapErr(err)
	}
	out := make([]Project, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.project())
	}
	return out, nil
}

// ProjectInput creates or replaces a project's settings.
type ProjectInput struct {
	TeamID    string
	Name      string
	Status    string
	Models    []string
	MaxBudget *float64
}

// CreateProject inserts a project narrowed within its team's grants.
func (db *DB) CreateProject(ctx context.Context, by Actor, in ProjectInput) (*Project, error) {
	var out *Project
	err := db.tx(ctx, func(s *xorm.Session) error {
		if err := lockTeam(ctx, s, in.TeamID); err != nil {
			return err
		}
		if err := checkWithinTeam(s, in.TeamID, "", in.Models, in.MaxBudget); err != nil {
			return err
		}
		p := Project{ID: newID(), TeamID: in.TeamID, Name: in.Name, Status: StatusActive,
			Models: nonNil(in.Models), MaxBudget: in.MaxBudget}
		if _, err := s.Insert(&p); err != nil {
			return err
		}
		var err error
		if out, err = getProject(ctx, s, p.ID); err != nil {
			return err
		}
		return writeAudit(s, by, Audit{Action: "project.create", ObjectType: "project", ObjectID: p.ID, TeamID: in.TeamID})
	})
	return out, err
}

// UpdateProject replaces name, status, narrowing and budget.
func (db *DB) UpdateProject(ctx context.Context, by Actor, id string, in ProjectInput) (*Project, error) {
	var out *Project
	err := db.tx(ctx, func(s *xorm.Session) error {
		cur, err := getProject(ctx, s, id)
		if err != nil {
			return err
		}
		if err := lockTeam(ctx, s, cur.TeamID); err != nil {
			return err
		}
		if err := checkWithinTeam(s, cur.TeamID, id, in.Models, in.MaxBudget); err != nil {
			return err
		}
		if in.Status == "" {
			in.Status = cur.Status
		}
		if in.Name == "" {
			in.Name = cur.Name
		}
		if _, err := s.ID(id).Cols("name", "status", "models", "max_budget").
			Update(&Project{Name: in.Name, Status: in.Status, Models: nonNil(in.Models), MaxBudget: in.MaxBudget}); err != nil {
			return err
		}
		if out, err = getProject(ctx, s, id); err != nil {
			return err
		}
		return writeAudit(s, by, Audit{Action: "project.update", ObjectType: "project", ObjectID: id, TeamID: cur.TeamID,
			Detail: map[string]any{"status": in.Status}})
	})
	return out, err
}

// DeleteProject cascades to the project's keys.
func (db *DB) DeleteProject(ctx context.Context, by Actor, id string) error {
	return db.tx(ctx, func(s *xorm.Session) error {
		var row struct {
			TeamID string `xorm:"'team_id'"`
		}
		ok, err := s.Table("projects").Cols("team_id").Where("id = ?", id).ForUpdate().Get(&row)
		if err != nil {
			return mapErr(err)
		}
		if !ok {
			return ErrNotFound
		}
		if err := affected(s.ID(id).Delete(&Project{})); err != nil {
			return err
		}
		return writeAudit(s, by, Audit{Action: "project.delete", ObjectType: "project", ObjectID: id, TeamID: row.TeamID})
	})
}

// checkWithinTeam enforces that narrowing models stay inside what the team —
// and, for a key or project inside it, the enclosing scope — already grants.
func checkWithinTeam(s *xorm.Session, teamID, projectID string, models []string, budget *float64) error {
	if len(models) > 0 {
		allowed, err := allowedModels(s, teamID, projectID, "")
		if err != nil {
			return err
		}
		// A nil set means the enclosing scope places no restriction, so every
		// name is within it. Only a restricted scope can reject one.
		if allowed != nil {
			for _, m := range models {
				if _, ok := allowed[m]; !ok {
					return ErrInvalid
				}
			}
		}
	}
	if budget == nil {
		return nil
	}
	ceiling, err := budgetCeiling(s, teamID, projectID, "")
	if err != nil {
		return err
	}
	if ceiling != nil && *budget > *ceiling {
		return ErrInvalid
	}
	return nil
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return dedupe(s)
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, s := range in {
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}
