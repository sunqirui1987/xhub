package iam

import (
	"context"
	"errors"
	"strings"

	"golang.org/x/crypto/bcrypt"
	"xorm.io/xorm"

	"github.com/sunqirui1987/xhub/internal/logx"
)

// 去掉邮箱首尾空白并转成小写，作为查找和存储的统一形式。
// 参数 e（string）：规范化邮箱使用的事件。空串表示调用方没有提供这项。
// 返回 string（string）：去掉空白并转成小写的邮箱。
// 调用：iam/teams.go
// 测试：无直接单测
func normEmail(e string) string { return strings.ToLower(strings.TrimSpace(e)) }

// 用 bcrypt 保存口令。短于 8 个字符时返回错误，明文不会入库。
// 参数 plain（string）：明文口令或明文密钥。只在创建和轮换时出现，不会写入日志。
// 返回 string（string）：bcrypt 口令哈希。明文不会出现在返回值里；error（error）：失败原因。不足 8 位或 bcrypt 失败时非 nil。
// 调用：仅在 users.go 内使用
// 测试：无直接单测
func hashPassword(plain string) (string, error) {
	if len(plain) < 8 {
		return "", errors.New("password must be at least 8 characters")
	}
	b, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	return string(b), err
}

// Bootstrapped reports whether the first platform administrator exists.
// 参数 ctx（context.Context）：上下文，取消或超时时停止后续工作。
// 返回 bool（bool）：第一个平台管理员已经存在时返回真；error（error）：失败原因。nil 表示这一步成功。
// 调用：gateway/session.go
// 测试：users_test.go
func (db *DB) Bootstrapped(ctx context.Context) (bool, error) {
	return db.Engine.Context(ctx).Exist(&bootstrapState{ID: 1})
}

// Bootstrap creates the first platform administrator exactly once.
// 参数 ctx（context.Context）：上下文，取消时停止；email（string）：用户邮箱。比较前会去掉空白并转成小写；name（string）：Bootstrap要查找或展示的名称。空串表示还没有命名；password（string）：明文口令或明文密钥。只在创建和轮换时出现，不会写入日志。
// 返回 *User（*User）：第一次创建出的平台管理员。已经引导过时不再创建。失败时为 nil；error（error）：密码哈希或写入失败。nil 表示这一次引导结束。
// 调用：gateway/session.go
// 测试：users_test.go
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
// 参数 ctx（context.Context）：上下文，取消时停止；email（string）：用户邮箱。比较前会去掉空白并转成小写。
// 返回 *User（*User）：按邮箱不区分大小写查出的账户。没有这个邮箱时为 nil；error（error）：查询失败。nil 表示查询完成。
// 调用：仅在 users.go 内使用
// 测试：chains_test.go、users_test.go
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

// EnsureAdmin creates a platform administrator from configuration if no account with that address exists yet, and reports whether it created one. It deliberately never updates an existing row. The configured password is an initial password: an operator who changes the YAML after the first start expects the running account to keep the password it has, not to have it silently reset by a config file that may sit in a repository. Changing a live password is a write against the account, not a deployment side effect. An empty email or password is not an error here; it means the deployment has not configured seeding, and the caller falls back to POST /bootstrap.
// 参数 ctx（context.Context）：上下文，取消时停止；email（string）：用户邮箱。比较前会去掉空白并转成小写；name（string）：确保管理员要查找或展示的名称。空串表示还没有命名；password（string）：明文口令或明文密钥。只在创建和轮换时出现，不会写入日志。
// 返回 bool（bool）：这次根据配置新建了平台管理员时返回真。已有同邮箱账号时不会更新，并返回假；error（error）：失败原因。nil 表示这一步成功。
// 调用：gateway/session.go
// 测试：chains_test.go、users_test.go
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

// defaultAdminName derives a display name from the address when config left the name unset: "ops@example.com" becomes "ops".
// 参数 email（string）：用户邮箱。比较前会去掉空白并转成小写。
// 返回 string（string）：default管理员名称对应的名字。空输入时使用约定的默认。
// 调用：仅在 users.go 内使用
// 测试：无直接单测
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

// CreateUser inserts a platform account together with the memberships the caller asked for, in one transaction. The memberships are written only after the account row exists, and a failure to write anyof them rolls the whole thing back: an account that came out of a failed create would be one nobody intended to make. The budget is stored as given; nil means no ceiling rather than zero, which is the
//
//	difference between "unlimited" and "cannot spend anything".
//
// 参数 ctx（context.Context）：上下文，取消时停止；by（Actor）：执行这次修改的操作者。审计日志记这个人的 id 和角色；in（UserInput）：调用方提交的UserInput。字段为空表示这项不改。
// 返回 *User（*User）：新建的平台账户。成员关系在同一事务里写入。失败时为 nil；error（error）：密码、唯一约束或事务失败。nil 表示账户和成员都已写入。
// 调用：gateway/identity/handlers.go
// 测试：activity_http_test.go、authz_test.go、dial_log_test.go
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

// UsersByIDs loads the accounts named by ids. An empty list is an empty map, not every account. Callers use it to label rows they have already scoped.
// 参数 ctx（context.Context）：上下文，取消时停止；ids（[]string）：要保留或查询的一组 id。空切片表示没有可处理的记录。
// 返回 map[string]User（map[string]User）：Users按标识列表的字段表。缺键表示上游或库里没有这个字段；error（error）：失败原因。nil 表示这一步成功。
// 调用：gateway/usage/entity_activity.go
// 测试：无直接单测
func (db *DB) UsersByIDs(ctx context.Context, ids []string) (map[string]User, error) {
	out := map[string]User{}
	if len(ids) == 0 {
		return out, nil
	}
	s := db.session(ctx)
	defer s.Close()
	var rows []User
	if err := s.In("id", ids).Find(&rows); err != nil {
		return nil, mapErr(err)
	}
	for _, u := range rows {
		out[u.ID] = u
	}
	return out, nil
}

// GetUser loads one account.
// 参数 ctx（context.Context）：上下文，取消或超时时停止后续工作；id（string）：用户 id。空串表示没有指定用户。
// 返回 *User（*User）：按 id 查出的账户。没有这个 id 时为 nil；error（error）：没有这条记录或查询失败。nil 表示查到了。
// 调用：authz/decide.go、gateway/identity/handlers.go、gateway/limits.go、gateway/session.go
// 测试：authz_test.go
func (db *DB) GetUser(ctx context.Context, id string) (*User, error) {
	s := db.session(ctx)
	defer s.Close()
	return getUser(ctx, s, id)
}

// getUser loads one user by id. The filter is applied here and not by the caller: xorm resets the statement after every call, so a WHERE set before another statement has already been discarded, and an unfiltered Get would return an arbitrary row of the table.
// 参数 _（context.Context）：请求上下文。取消或超时后停止数据库和上游调用；s（*xorm.Session）：当前事务里的数据库会话。调用方负责提交，这里不关闭它；id（string）：用户 id。空串表示没有指定用户。
// 返回 *User（*User）：按 id 查出的用户。id 为空或没有这条记录时为 nil；error（error）：ErrNotFound 或查询失败。nil 表示查到了。
// 调用：仅在 users.go 内使用
// 测试：无直接单测
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
// 参数 ctx（context.Context）：上下文，取消或超时时停止后续工作；id（string）：用户查询使用的主键。空串表示调用方没有指定记录。
// 返回 *xorm.Session（*xorm.Session）：带当前过滤条件的数据库会话，调用方负责关闭。
// 调用：仅在 users.go 内使用
// 测试：无直接单测
func (db *DB) userQuery(ctx context.Context, id string) *xorm.Session {
	return db.Engine.NewSession().Context(ctx).Where("id = ?", id)
}

// ListScopedUsers lists the caller and the accounts that belong to the teams they oversee. An empty team list returns only the caller.
// 参数 ctx（context.Context）：上下文，取消时停止；self（string）：列出ScopedUsers使用的自己。空串表示调用方没有提供这项；teamIDs（[]string）：团队标识列表列表。空切片表示没有可处理的项；query（string）：URL 查询串或查询文本；limit（int）：最多返回的条数；offset（int）：跳过的条数。
// 返回 []User（[]User）：符合条件的用户；error（error）：失败原因，nil 表示成功。
// 调用：gateway/identity/handlers.go
// 测试：无直接单测
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
// 参数 ctx（context.Context）：上下文，取消时停止；query（string）：URL 查询串或查询文本；limit（int）：最多返回的条数；offset（int）：跳过的条数。
// 返回 []User（[]User）：符合条件的用户；error（error）：失败原因，nil 表示成功。
// 调用：gateway/access.go、gateway/identity/handlers.go、gateway/usage/reports.go
// 测试：无直接单测
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

// Login checks a password. A disabled account and a wrong password both return ErrNotFound so callers cannot tell the cases apart. The identifier is an email, or, when it contains no @, the display name
//
//	or the part of the address before @. The console's username field is free text, and the configured administrator is often typed as "admin" rather than "admin@xhub.local". A name that matches more than one account is refused, the same as a missing account, so the response still cannot enumerate users.
//
// 参数 ctx（context.Context）：上下文，取消时停止；email（string）：用户邮箱。比较前会去掉空白并转成小写；password（string）：明文口令或明文密钥。只在创建和轮换时出现，不会写入日志。
// 返回 *User（*User）：a password。找不到或这一步失败时为 nil；error（error）：失败原因，nil 表示成功。
// 调用：gateway/session.go
// 测试：users_test.go
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

// findLoginUser resolves the username the login form sent. An address matches the email column. A bare word matches one display name or one email local part, and only when that word identifies a singleaccount.
// 参数 s（*xorm.Session）：当前事务里的数据库会话。调用方负责提交，这里不关闭它；ident（string）：查找Login用户使用的ident。空串表示调用方没有提供这项。
// 返回 *User（*User）：登录框里的用户名对应的账户。带 @ 时按邮箱，否则按唯一的显示名。对不上或重名时为 nil；error（error）：没有这个账户或查询失败。nil 表示对上了唯一账户。
// 调用：仅在 users.go 内使用
// 测试：无直接单测
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

// ConsoleRole is the role name the admin UI understands. The row stores admin or user. The console only treats proxy_admin as a platform administrator, so a session that carries the stored spelling is shown as an unknown role and every admin-only query is narrowed to that one user.
// 参数 role（string）：角色名。控制台的 admin 或 user 会映射成库存的写法。
// 返回 string（string）：Console角色对应的名字。空输入时使用约定的默认。
// 调用：gateway/identity/handlers.go、gateway/session.go
// 测试：users_test.go
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

// StoreRole maps a role the console sent back onto the stored spelling. An empty value is a normal user, which is what the account form sends when the picker is left on its default.
// 参数 role（string）：角色名。控制台的 admin 或 user 会映射成库存的写法。
// 返回 string（string）：库角色对应的名字。空输入时使用约定的默认。
// 调用：gateway/identity/handlers.go
// 测试：users_test.go
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
// 参数 ctx（context.Context）：上下文，取消时停止；by（Actor）：执行这次修改的操作者。审计日志记这个人的 id 和角色；id（string）：更新Profile使用的主键。空串表示调用方没有指定记录；name（string）：更新Profile要查找或展示的名称。空串表示还没有命名。
// 返回 *User（*User）：改完显示名后的账户。只改显示字段；error（error）：账户不存在或写入失败。nil 表示已经改完。
// 调用：gateway/identity/handlers.go
// 测试：无直接单测
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
// 参数 ctx（context.Context）：上下文，取消时停止；by（Actor）：执行这次修改的操作者。审计日志记这个人的 id 和角色；id（string）：写入口令使用的主键。空串表示调用方没有指定记录；password（string）：明文口令或明文密钥。只在创建和轮换时出现，不会写入日志。
// 返回 error（error）：失败原因，nil 表示这一步成功。
// 调用：gateway/identity/handlers.go
// 测试：无直接单测
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

// UserUpdate 是管理员修改账户时的增量字段；nil 保留原值，Name 的空字符串用于清空显示名称。
type UserUpdate struct {
	Name      *string
	Role      *string
	Status    *string
	MaxBudget **float64
	Email     *string
}

// AdminUpdateUser 在同一事务中修改名称、角色、状态、邮箱和预算；供用户更新接口调用。
// 参数 ctx（context.Context）：上下文，取消时停止；by（Actor）：执行这次修改的操作者。审计日志记这个人的 id 和角色；id（string）：用户 id。空串表示没有指定用户；in（UserUpdate）：调用方提交的UserUpdate。字段为空表示这项不改。
// 返回 *User（*User）：修改后的账户；error 表示账户不存在、字段非法或事务失败，失败不会留下部分更新。角色或状态变化使会话失效，停用也撤销个人密钥。
// 调用：gateway/identity/handlers.go
// 测试：authz_test.go
func (db *DB) AdminUpdateUser(ctx context.Context, by Actor, id string, in UserUpdate) (*User, error) {
	var out *User
	err := db.tx(ctx, func(s *xorm.Session) error {
		cur, err := getUser(ctx, s, id)
		if err != nil {
			return err
		}
		next := *cur
		if in.Name != nil {
			next.Name = *in.Name
		}
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
		// 提前校验管理员可编辑字段，数据库约束不应变成页面上的内部错误；所有字段一起拒绝，避免部分保存。
		if (next.Role != RoleAdmin && next.Role != RoleUser) ||
			(next.Status != StatusActive && next.Status != StatusDisabled) || next.Email == "" ||
			(next.MaxBudget != nil && *next.MaxBudget < 0) {
			return ErrInvalid
		}
		if cur.Admin() && !(next.Role == RoleAdmin && next.Status == StatusActive) {
			if err := keepPlatformAdmin(ctx, s, id); err != nil {
				return err
			}
		}
		if next.Role != cur.Role || next.Status != cur.Status {
			next.SessionVersion = cur.SessionVersion + 1
		}
		if _, err := s.ID(id).Cols("name", "role", "status", "max_budget", "email", "session_version").Update(&next); err != nil {
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

// DeleteUser removes an account; memberships and personal keys cascade, but the delete is refused while the user is a team's last team_admin.
// 参数 ctx（context.Context）：上下文，取消时停止；by（Actor）：执行这次修改的操作者。审计日志记这个人的 id 和角色；id（string）：用户 id。空串表示没有指定用户。
// 返回 error（error）：失败原因，nil 表示这一步成功。
// 调用：gateway/identity/handlers.go
// 测试：无直接单测
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

// 在当前事务里按 id 锁住用户行。
// 参数 ctx（context.Context）：上下文，取消或超时时停止后续工作；s（*xorm.Session）：当前事务里的数据库会话。调用方负责提交，这里不关闭它；id（string）：用户 id。空串表示没有指定用户。
// 返回 error（error）：失败原因，nil 表示这一步成功。
// 调用：仅在 users.go 内使用
// 测试：无直接单测
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

// keepPlatformAdmin fails when no other active platform administrator remains. The rows are locked so two concurrent demotions cannot both succeed.
// 参数 ctx（context.Context）：上下文，取消时停止；s（*xorm.Session）：当前事务里的数据库会话。调用方负责提交，这里不关闭它；excluding（string）：保留Platform管理员使用的excluding。空串表示调用方没有提供这项。
// 返回 error（error）：失败原因，nil 表示这一步成功。
// 调用：仅在 users.go 内使用
// 测试：无直接单测
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
