package iam

import (
	"context"
	"errors"
	"strings"

	"golang.org/x/crypto/bcrypt"
	"xorm.io/xorm"

	"github.com/sunqirui1987/xhub/internal/logx"
)

func normEmail(e string) string { return strings.ToLower(strings.TrimSpace(e)) }

func hashPassword(plain string) (string, error) {
	if len(plain) < 8 {
		return "", errors.New("password must be at least 8 characters")
	}
	b, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	return string(b), err
}

// Bootstrapped reports whether the first platform administrator exists.
func (db *DB) Bootstrapped(ctx context.Context) (bool, error) {
	return db.Engine.Context(ctx).Exist(&bootstrapState{ID: 1})
}

// Bootstrap creates the first platform administrator exactly once.
func (db *DB) Bootstrap(ctx context.Context, email, name, password string) (*User, error) {
	hash, err := hashPassword(password)
	if err != nil {
		return nil, err
	}
	var out *User
	err = db.tx(ctx, func(s *xorm.Session) error {
		if _, err := s.Insert(&bootstrapState{ID: 1}); err != nil {
			return err
		}
		u := User{ID: newID(), Email: normEmail(email), Name: name, PasswordHash: hash, Role: RoleAdmin, Status: StatusActive, SessionVersion: 1}
		if _, err := s.Insert(&u); err != nil {
			return err
		}
		out = &u
		return writeAudit(s, Actor{Kind: "master"}, Audit{Action: "bootstrap", ObjectType: "user", ObjectID: u.ID})
	})
	return out, err
}

// UserByEmail loads one account by its address, case-insensitively.
func (db *DB) UserByEmail(ctx context.Context, email string) (*User, error) {
	s := db.session(ctx)
	defer s.Close()
	var u User
	ok, err := s.Where("lower(email) = ?", normEmail(email)).Get(&u)
	if err != nil {
		return nil, mapErr(err)
	}
	if !ok {
		return nil, ErrNotFound
	}
	return &u, nil
}

// EnsureAdmin creates a platform administrator from configuration if no account
// with that address exists yet, and reports whether it created one.
//
// It deliberately never updates an existing row. The configured password is an
// initial password: an operator who changes the YAML after the first start
// expects the running account to keep the password it has, not to have it
// silently reset by a config file that may sit in a repository. Changing a live
// password is a write against the account, not a deployment side effect.
//
// An empty email or password is not an error here; it means the deployment has
// not configured seeding, and the caller falls back to POST /bootstrap.
func (db *DB) EnsureAdmin(ctx context.Context, email, name, password string) (bool, error) {
	if normEmail(email) == "" || password == "" {
		return false, nil
	}
	existing, err := db.UserByEmail(ctx, email)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return false, err
	}
	if existing != nil {
		logx.Info("configured administrator already exists user=%s role=%s", existing.ID, existing.Role)
		return false, nil
	}
	hash, err := hashPassword(password)
	if err != nil {
		return false, err
	}
	if name == "" {
		name = defaultAdminName(email)
	}
	u := User{ID: newID(), Email: normEmail(email), Name: name, PasswordHash: hash,
		Role: RoleAdmin, Status: StatusActive, SessionVersion: 1}
	err = db.tx(ctx, func(s *xorm.Session) error {
		if _, err := s.Insert(&u); err != nil {
			return err
		}
		// Mark the platform initialized, the same marker POST /bootstrap sets.
		// Without it the console would keep offering the setup form, and
		// /bootstrap would let a second administrator be created beside this
		// one. An already-marked platform is not an error: seeding runs on
		// every start, and the marker is written once.
		if _, err := s.Exec(`INSERT INTO bootstrap_state (id) VALUES (1) ON CONFLICT (id) DO NOTHING`); err != nil {
			return err
		}
		return writeAudit(s, Actor{Kind: "system"}, Audit{Action: "admin.seed", ObjectType: "user",
			ObjectID: u.ID, Detail: map[string]any{"role": RoleAdmin, "source": "config"}})
	})
	if err != nil {
		return false, err
	}
	logx.Info("configured administrator created user=%s email=%s", u.ID, u.Email)
	return true, nil
}

// defaultAdminName derives a display name from the address when config left the
// name unset: "ops@example.com" becomes "ops".
func defaultAdminName(email string) string {
	e := normEmail(email)
	if i := strings.IndexByte(e, '@'); i > 0 {
		return e[:i]
	}
	return e
}

// UserInput creates an account. Only platform administrators call this.
type UserInput struct {
	Email     string
	Name      string
	Password  string
	Role      string
	MaxBudget *float64
	// TeamID and TeamRole put the new account on a team in the same
	// transaction that creates it. Creating an account and adding it to a team
	// were two calls, and the second could fail on its own, which left a person
	// who existed but could reach nothing.
	TeamID   string
	TeamRole string
	// AdminOrgIDs makes the new account an organization administrator. It is
	// the same relationship /organization/member_add grants, so an account that
	// runs an organization can be set up without a second round trip.
	AdminOrgIDs []string
}

// CreateUser inserts a platform account together with the memberships the
// caller asked for, in one transaction.
//
// The memberships are written only after the account row exists, and a failure
// to write any of them rolls the whole thing back: an account that came out of
// a failed create would be one nobody intended to make. The budget is stored as
// given; nil means no ceiling rather than zero, which is the difference between
// "unlimited" and "cannot spend anything".
func (db *DB) CreateUser(ctx context.Context, by Actor, in UserInput) (*User, error) {
	hash, err := hashPassword(in.Password)
	if err != nil {
		return nil, err
	}
	if in.Role == "" {
		in.Role = RoleUser
	}
	if in.TeamRole != "" && in.TeamRole != TeamAdmin && in.TeamRole != TeamMember {
		return nil, ErrInvalid
	}
	if in.TeamRole != "" && in.TeamID == "" {
		return nil, ErrInvalid
	}
	var out *User
	err = db.tx(ctx, func(s *xorm.Session) error {
		u := User{ID: newID(), Email: normEmail(in.Email), Name: in.Name, PasswordHash: hash,
			Role: in.Role, Status: StatusActive, SessionVersion: 1, MaxBudget: in.MaxBudget}
		if _, err := s.Insert(&u); err != nil {
			return err
		}
		if in.TeamID != "" {
			if err := lockTeam(ctx, s, in.TeamID); err != nil {
				return err
			}
			role := in.TeamRole
			if role == "" {
				role = TeamMember
			}
			if _, err := s.Insert(&TeamMembership{TeamID: in.TeamID, UserID: u.ID, Role: role}); err != nil {
				return err
			}
		}
		for _, orgID := range in.AdminOrgIDs {
			if orgID == "" {
				continue
			}
			if err := lockOrg(ctx, s, orgID); err != nil {
				return err
			}
			if _, err := s.Insert(&OrganizationMembership{OrganizationID: orgID, UserID: u.ID, Role: OrgAdmin}); err != nil {
				return err
			}
		}
		out = &u
		return writeAudit(s, by, Audit{Action: "user.create", ObjectType: "user", ObjectID: u.ID,
			Detail: map[string]any{"role": u.Role, "team_id": in.TeamID, "team_role": in.TeamRole,
				"admin_organizations": len(in.AdminOrgIDs)}})
	})
	return out, err
}

// GetUser loads one account.
func (db *DB) GetUser(ctx context.Context, id string) (*User, error) {
	s := db.session(ctx)
	defer s.Close()
	return getUser(ctx, s, id)
}

// getUser loads one user by id. The filter is applied here and not by the
// caller: xorm resets the statement after every call, so a WHERE set before
// another statement has already been discarded, and an unfiltered Get would
// return an arbitrary row of the table.
func getUser(_ context.Context, s *xorm.Session, id string) (*User, error) {
	if id == "" {
		return nil, ErrNotFound
	}
	var u User
	if err := get(s.Where("id = ?", id), &u); err != nil {
		return nil, err
	}
	return &u, nil
}

// userQuery builds a session that reads exactly one user.
func (db *DB) userQuery(ctx context.Context, id string) *xorm.Session {
	return db.Engine.NewSession().Context(ctx).Where("id = ?", id)
}

// ListScopedUsers lists the caller and the accounts that belong to the teams
// they oversee. An empty team list returns only the caller.
func (db *DB) ListScopedUsers(ctx context.Context, self string, teamIDs []string, query string, limit, offset int) ([]User, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	s := db.session(ctx)
	defer s.Close()
	args := []any{self}
	where := "id = ?"
	if len(teamIDs) > 0 {
		ph := make([]string, len(teamIDs))
		for i, id := range teamIDs {
			ph[i] = "?"
			args = append(args, id)
		}
		where += " OR id IN (SELECT user_id FROM team_members WHERE team_id IN (" + strings.Join(ph, ",") + "))"
	}
	if query != "" {
		like := "%" + query + "%"
		where = "(" + where + ") AND (email ILIKE ? OR name ILIKE ?)"
		args = append(args, like, like)
	}
	var out []User
	err := s.Where(where, args...).Asc("created_at", "id").Limit(limit, offset).Find(&out)
	return out, err
}

// ListUsers is the platform administrator's directory.
func (db *DB) ListUsers(ctx context.Context, query string, limit, offset int) ([]User, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	s := db.session(ctx)
	defer s.Close()
	if query != "" {
		like := "%" + query + "%"
		s = s.Where("email ILIKE ? OR name ILIKE ?", like, like)
	}
	var out []User
	err := s.Asc("created_at", "id").Limit(limit, offset).Find(&out)
	return out, err
}

// Login checks a password. A disabled account and a wrong password both return
// ErrNotFound so callers cannot tell the cases apart.
//
// The identifier is an email, or, when it contains no @, the display name or
// the part of the address before @. The console's username field is free text,
// and the configured administrator is often typed as "admin" rather than
// "admin@xhub.local". A name that matches more than one account is refused,
// the same as a missing account, so the response still cannot enumerate users.
func (db *DB) Login(ctx context.Context, email, password string) (*User, error) {
	s := db.session(ctx)
	defer s.Close()
	u, err := findLoginUser(s, email)
	if err != nil {
		return nil, err
	}
	if u.PasswordHash == "" {
		return nil, ErrNotFound
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)) != nil {
		logx.Debug("login rejected for user %s", u.ID)
		return nil, ErrNotFound
	}
	if !u.Active() {
		logx.Debug("login rejected for disabled user %s", u.ID)
		return nil, ErrNotFound
	}
	return u, nil
}

// findLoginUser resolves the username the login form sent. An address matches
// the email column. A bare word matches one display name or one email local
// part, and only when that word identifies a single account.
func findLoginUser(s *xorm.Session, ident string) (*User, error) {
	key := normEmail(ident)
	if key == "" {
		return nil, ErrNotFound
	}
	var u User
	ok, err := s.Where("lower(email) = ?", key).Get(&u)
	if err != nil {
		return nil, mapErr(err)
	}
	if ok {
		return &u, nil
	}
	if strings.Contains(key, "@") {
		return nil, ErrNotFound
	}
	var matches []User
	err = s.Where("lower(name) = ? OR lower(split_part(email, '@', 1)) = ?", key, key).Find(&matches)
	if err != nil {
		return nil, mapErr(err)
	}
	seen := map[string]User{}
	for _, m := range matches {
		seen[m.ID] = m
	}
	if len(seen) != 1 {
		return nil, ErrNotFound
	}
	for _, m := range seen {
		copy := m
		return &copy, nil
	}
	return nil, ErrNotFound
}

// ConsoleRole is the role name the admin UI understands. The row stores admin
// or user. The console only treats proxy_admin as a platform administrator, so
// a session that carries the stored spelling is shown as an unknown role and
// every admin-only query is narrowed to that one user.
func ConsoleRole(role string) string {
	switch role {
	case RoleAdmin:
		return "proxy_admin"
	case RoleUser:
		return "internal_user"
	default:
		return role
	}
}

// StoreRole maps a role the console sent back onto the stored spelling. An
// empty value is a normal user, which is what the account form sends when the
// picker is left on its default.
func StoreRole(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "", "user", "internal_user", "internal user":
		return RoleUser
	case "admin", "proxy_admin", "proxy admin":
		return RoleAdmin
	default:
		return role
	}
}

// UpdateProfile changes display fields only.
func (db *DB) UpdateProfile(ctx context.Context, by Actor, id, name string) (*User, error) {
	var out *User
	err := db.tx(ctx, func(s *xorm.Session) error {
		if err := lockUser(ctx, s, id); err != nil {
			return err
		}
		if _, err := s.ID(id).Cols("name").Update(&User{Name: name}); err != nil {
			return err
		}
		var err error
		if out, err = getUser(ctx, s, id); err != nil {
			return err
		}
		return writeAudit(s, by, Audit{Action: "user.profile", ObjectType: "user", ObjectID: id})
	})
	return out, err
}

// SetPassword replaces the password and ends existing sessions.
func (db *DB) SetPassword(ctx context.Context, by Actor, id, password string) error {
	hash, err := hashPassword(password)
	if err != nil {
		return err
	}
	return db.tx(ctx, func(s *xorm.Session) error {
		u, err := getUser(ctx, s, id)
		if err != nil {
			return err
		}
		if _, err := s.ID(id).Cols("password_hash", "session_version").
			Update(&User{PasswordHash: hash, SessionVersion: u.SessionVersion + 1}); err != nil {
			return err
		}
		return writeAudit(s, by, Audit{Action: "user.password", ObjectType: "user", ObjectID: id})
	})
}

// UserUpdate is a platform-administrator change to someone else's account.
type UserUpdate struct {
	Role      *string
	Status    *string
	MaxBudget **float64
	Email     *string
}

// AdminUpdateUser changes role, status, email or budget in one transaction.
// A role or status change ends the user's sessions; disabling also revokes
// every personal key.
func (db *DB) AdminUpdateUser(ctx context.Context, by Actor, id string, in UserUpdate) (*User, error) {
	var out *User
	err := db.tx(ctx, func(s *xorm.Session) error {
		cur, err := getUser(ctx, s, id)
		if err != nil {
			return err
		}
		next := *cur
		if in.Role != nil {
			next.Role = *in.Role
		}
		if in.Status != nil {
			next.Status = *in.Status
		}
		if in.MaxBudget != nil {
			next.MaxBudget = *in.MaxBudget
		}
		if in.Email != nil {
			next.Email = normEmail(*in.Email)
		}
		if cur.Admin() && !(next.Role == RoleAdmin && next.Status == StatusActive) {
			if err := keepPlatformAdmin(ctx, s, id); err != nil {
				return err
			}
		}
		if next.Role != cur.Role || next.Status != cur.Status {
			next.SessionVersion = cur.SessionVersion + 1
		}
		if _, err := s.ID(id).Cols("role", "status", "max_budget", "email", "session_version").Update(&next); err != nil {
			return err
		}
		if next.Status != StatusActive && cur.Status == StatusActive {
			if _, err := revokePersonalKeys(ctx, s, "user_id = ?", id); err != nil {
				return err
			}
		}
		if out, err = getUser(ctx, s, id); err != nil {
			return err
		}
		return writeAudit(s, by, Audit{Action: "user.update", ObjectType: "user", ObjectID: id,
			Detail: map[string]any{"role": next.Role, "status": next.Status}})
	})
	return out, err
}

// DeleteUser removes an account; memberships and personal keys cascade, but the
// delete is refused while the user is a team's last team_admin.
func (db *DB) DeleteUser(ctx context.Context, by Actor, id string) error {
	return db.tx(ctx, func(s *xorm.Session) error {
		cur, err := getUser(ctx, s, id)
		if err != nil {
			return err
		}
		if cur.Admin() {
			if err := keepPlatformAdmin(ctx, s, id); err != nil {
				return err
			}
		}
		var adminOf []string
		if err := s.Table("team_members").Cols("team_id").
			Where("user_id = ? AND role = ?", id, TeamAdmin).Asc("team_id").Find(&adminOf); err != nil {
			return err
		}
		for _, teamID := range adminOf {
			if err := lockTeam(ctx, s, teamID); err != nil {
				return err
			}
			if err := keepTeamAdmin(ctx, s, teamID, id); err != nil {
				return err
			}
		}
		if err := affected(s.ID(id).Delete(&User{})); err != nil {
			return err
		}
		return writeAudit(s, by, Audit{Action: "user.delete", ObjectType: "user", ObjectID: id, Detail: map[string]any{"teams": adminOf}})
	})
}

func lockUser(ctx context.Context, s *xorm.Session, id string) error {
	var u User
	if err := get(s.SQL("SELECT * FROM users WHERE id = ? FOR UPDATE", id), &u); err != nil {
		if err == ErrNotFound {
			return ErrNotFound
		}
		return err
	}
	return nil
}

// keepPlatformAdmin fails when no other active platform administrator remains.
// The rows are locked so two concurrent demotions cannot both succeed.
func keepPlatformAdmin(ctx context.Context, s *xorm.Session, excluding string) error {
	var ids []string
	err := s.SQL("SELECT id FROM users WHERE role = ? AND status = ? AND id <> ? FOR UPDATE",
		RoleAdmin, StatusActive, excluding).Find(&ids)
	if err != nil {
		return err
	}
	if len(ids) == 0 {
		return ErrLastPlatformAdmin
	}
	return nil
}
