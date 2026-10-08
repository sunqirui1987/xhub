// Package iam stores users, organizations, teams, memberships, projects,
// access groups, keys and the audit log. schema.sql owns the constraints
// (composite foreign keys, CHECKs, case-insensitive email); xorm owns every
// read and write. Membership is only ever read from team_members.
package iam

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib"
	"xorm.io/xorm"

	"github.com/sunqirui1987/xhub/internal/logx"
)

//go:embed schema.sql
var schemaSQL string

var (
	ErrNotFound          = errors.New("not found")
	ErrConflict          = errors.New("already exists")
	ErrInvalid           = errors.New("invalid relation")
	ErrLastAdmin         = errors.New("team must keep at least one team_admin")
	ErrLastPlatformAdmin = errors.New("platform must keep at least one active admin")
	ErrInactive          = errors.New("inactive")
	// ErrRouteTemplateMissing is a template id that names no row. It is separate
	// from ErrNotFound because the handler has to tell "the template is gone"
	// from "the scope is gone" and answer differently.
	ErrRouteTemplateMissing = errors.New("route template not found")
	// ErrUnknownScope is a scope name outside the three the inheritance chain
	// knows. The name reaches SQL, so it is rejected rather than interpolated.
	ErrUnknownScope = errors.New("unknown scope")
)

// DB wraps the xorm engine. No query cache is installed: authorization must
// read live rows.
type DB struct {
	Engine *xorm.Engine
}

// Open connects with the pgx driver and applies the schema. The engine keeps xorm's default time zones — DatabaseTZ and TZLocation both Local — and they must agree with the session zone. xorm formats atime.Time into the database zone on the way in and re-labels the value it reads back into the application zone, while pgx returns the instant in the session zone. Pinning any one of the three to UTC while the others stayed Local made stored timestamps land eight hours off on a UTC+8 machine: created _at and updated_at disagreed with columns PostgreSQL filled in itself, because DEFAULT now() is computed by the server and never passes through this conversion. Setting all three together is what makes the round trip lossless. It matters that the session zone is named explicitly rather than left tothe server, since a server whose zone differs from the machine's would reintroduce the same skew.
// 参数 ctx（context.Context）：上下文，取消时停止；dsn（string）：数据库连接串。
// 返回 *DB（*DB）：用 pgx 连上并套好模式的库。连接或建表失败时为 nil；error（error）：连接、ping 或建表失败。nil 表示库可以用。
// 调用：gateway/server.go、live/redis.go、store/engine.go、store/store.go
// 测试：authz_test.go、builtin_providers_test.go、chains_test.go
func Open(ctx context.Context, dsn string) (*DB, error) {
	engine, err := xorm.NewEngine("pgx", dsnWithSessionZone(dsn))
	if err != nil {
		return nil, err
	}
	if err := engine.PingContext(ctx); err != nil {
		engine.Close()
		return nil, err
	}
	db := &DB{Engine: engine}
	if err := db.Migrate(ctx); err != nil {
		engine.Close()
		return nil, err
	}
	return db, nil
}

// Migrate creates every table and constraint; idempotent on the new schema.
// 参数 ctx（context.Context）：上下文，取消或超时时停止后续工作。
// 返回 error（error）：失败原因，nil 表示这一步成功。
// 调用：仅在 db.go 内使用
// 测试：无直接单测
func (db *DB) Migrate(ctx context.Context) error {
	if _, err := db.Engine.Context(ctx).Exec(schemaSQL); err != nil {
		logx.Error("iam schema apply failed: %v", err)
		return err
	}
	// Names and hashes that already exist on the referenced rows. TTFT and
	// headers were never stored, so they stay empty.
	_, _ = db.Engine.Context(ctx).Exec(`
		UPDATE usage_events e SET team_alias = t.name
		FROM teams t WHERE e.team_id = t.id AND e.team_alias = '' AND e.team_id <> ''`)
	_, _ = db.Engine.Context(ctx).Exec(`
		UPDATE usage_events e SET key_alias = k.name, key_hash = k.token_hash
		FROM api_keys k WHERE e.key_id = k.id AND e.key_alias = '' AND e.key_id <> ''`)
	return nil
}

// Close releases the engine.
// 参数：无。
// 返回 error（error）：失败原因，nil 表示这一步成功。
// 调用：dataplane/official.go、dataplane/serve.go、dataplane/stream.go、gateway/ingress.go
// 测试：authz_test.go、builtin_providers_test.go、bypass_logic_test.go
func (db *DB) Close() error { return db.Engine.Close() }

// tx runs fn in one xorm transaction and rolls back on any error.
// 参数 ctx（context.Context）：上下文，取消时停止；fn（值）：tx使用的值。
// 返回 error（error）：失败原因，nil 表示这一步成功。
// 调用：iam/keys.go、iam/teams.go、iam/usage.go、iam/users.go
// 测试：无直接单测
func (db *DB) tx(ctx context.Context, fn func(*xorm.Session) error) error {
	s := db.Engine.NewSession().Context(ctx)
	defer s.Close()
	if err := s.Begin(); err != nil {
		return err
	}
	if err := fn(s); err != nil {
		_ = s.Rollback()
		return mapErr(err)
	}
	return mapErr(s.Commit())
}

// session returns a non-transactional session bound to ctx.
// 参数 ctx（context.Context）：上下文，取消或超时时停止后续工作。
// 调用：iam/keys.go、iam/teams.go、iam/usage_read.go、iam/users.go
// 测试：authz_test.go、usage_idempotency_test.go
// 返回 *xorm.Session（*xorm.Session）：带当前过滤条件的数据库会话，调用方负责关闭。
func (db *DB) session(ctx context.Context) *xorm.Session {
	return db.Engine.NewSession().Context(ctx)
}

// dsnWithSessionZone returns the DSN with the connection's TimeZone set to the machine's own zone. The session zone is what pgx uses to render a TIMESTAMPTZ, and xorm reads that rendering back as though
//
//	it were already in the application zone. Leaving the session to the server means a server configured for UTC and a machine at UTC+8 disagree by the offset, so the zone is named here rather than assumed.
//
// 参数 dsn（string）：数据库连接串。
// 返回 string（string）：写好 search_path 或 TimeZone 的连接串。
// 调用：仅在 db.go 内使用
// 测试：db_zone_test.go
func dsnWithSessionZone(dsn string) string {
	u, err := url.Parse(dsn)
	if err != nil {
		return dsn
	}
	q := u.Query()
	q.Set("timezone", serverZone(time.Local))
	u.RawQuery = q.Encode()
	return u.String()
}

// serverZone renders a Go location as a zone name PostgreSQL understands. A location that knows its name is used directly. Otherwise the offset is sent in the POSIX form, and it is easy to get backwards
// : PostgreSQL reads the sign of a numeric zone as the offset *west* of UTC, so a machine at UTC+8 is named "-08" there, not "+08:00". Sending the Go spelling would put the session in UTC-8 and move every stored timestamp by twice the offset.
// 参数 loc（*time.Location）：服务时区使用的Location。
// 返回 string（string）：PostgreSQL 能识别的时区名。地区有名字时直接用名字，否则用相对 UTC 的偏移，例如 +08:00。
// 调用：仅在 db.go 内使用
// 测试：db_zone_test.go
func serverZone(loc *time.Location) string {
	if name := loc.String(); name != "Local" && name != "" {
		return name
	}
	_, offset := time.Now().In(loc).Zone()
	sign := "-"
	if offset < 0 {
		sign, offset = "+", -offset
	}
	return fmt.Sprintf("%s%02d:%02d", sign, offset/3600, offset%3600/60)
}

// mapErr turns constraint violations into package errors.
// 参数 err（error）：失败原因，nil 表示这一步成功。
// 返回 error（error）：失败原因，nil 表示这一步成功。
// 调用：iam/keys.go、iam/teams.go、iam/usage.go、iam/usage_read.go
// 测试：无直接单测
func mapErr(err error) error {
	if err == nil {
		return nil
	}
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		switch pg.Code {
		case "23505":
			return fmt.Errorf("%w: %s", ErrConflict, pg.ConstraintName)
		case "23503", "23514", "23502":
			return fmt.Errorf("%w: %s", ErrInvalid, pg.ConstraintName)
		}
	}
	return err
}

// get loads one bean or returns ErrNotFound.
// 参数 s（*xorm.Session）：当前事务里的数据库会话。调用方负责提交，这里不关闭它；bean（any）：读取接到的动态值。类型在函数体内收窄。
// 返回 error（error）：失败原因，nil 表示这一步成功。
// 调用：iam/keys.go、iam/teams.go、iam/users.go
// 测试：activity_http_test.go
func get(s *xorm.Session, bean any) error {
	ok, err := s.Get(bean)
	if err != nil {
		return mapErr(err)
	}
	if !ok {
		return ErrNotFound
	}
	return nil
}

// affected 把 xorm 的影响行数收成业务错误。
// 参数 n（int64）：这条语句影响的行数。0 表示没有命中记录；err（error）：驱动返回的错误。nil 表示语句本身成功。
// 返回 error（error）：err 非 nil 时是映射后的错误。影响 0 行时是 ErrNotFound。否则为 nil。
// 调用：iam 的更新和删除。
// 测试：无直接单测
func affected(n int64, err error) error {
	if err != nil {
		return mapErr(err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// Actor identifies who performed an audited write.
type Actor struct {
	ID   string
	Kind string // session | master | system
}

// Audit is the content of one audit-log row.
type Audit struct {
	Action     string
	ObjectType string
	ObjectID   string
	TeamID     string
	Detail     map[string]any
}

// 在当前事务里写一条审计。操作者类型为空时记成 system。
// 参数 s（*xorm.Session）：当前事务里的数据库会话。调用方负责提交，这里不关闭它；a（Actor）：执行这次修改的操作者。审计日志记这个人的 id 和角色；e（Audit）：写入审计使用的Audit。
// 返回 error（error）：失败原因，nil 表示这一步成功。
// 调用：iam/keys.go、iam/teams.go、iam/users.go
// 测试：无直接单测
func writeAudit(s *xorm.Session, a Actor, e Audit) error {
	kind := a.Kind
	if kind == "" {
		kind = "system"
	}
	detail := e.Detail
	if detail == nil {
		detail = map[string]any{}
	}
	row := AuditEntry{ActorID: a.ID, ActorKind: kind, Action: e.Action, ObjectType: e.ObjectType,
		ObjectID: e.ObjectID, TeamID: e.TeamID, Detail: detail}
	if _, err := s.Insert(&row); err != nil {
		return err
	}
	logx.Info("audit %s %s %s by %s:%s", e.Action, e.ObjectType, e.ObjectID, kind, a.ID)
	return nil
}

// RecordAudit writes an audit row outside a larger transaction, e.g. when a platform administrator reads someone else's request log.
// 参数 ctx（context.Context）：上下文，取消时停止；a（Actor）：执行这次修改的操作者。审计日志记这个人的 id 和角色；e（Audit）：记录审计使用的Audit。
// 返回 error（error）：失败原因，nil 表示这一步成功。
// 调用：authz/decide.go、iam/usage_read.go
// 测试：无直接单测
func (db *DB) RecordAudit(ctx context.Context, a Actor, e Audit) error {
	s := db.session(ctx)
	defer s.Close()
	return writeAudit(s, a, e)
}

// ListAudit returns the newest rows first.
// 参数 ctx（context.Context）：上下文，取消时停止；limit（int）：最多返回的条数；offset（int）：跳过的条数。
// 返回 []AuditEntry（[]AuditEntry）：按 id 倒序的审计行。limit 小于等于 0 或大于 500 时按 100。没有行时为空切片；error（error）：查询失败。nil 表示成功。
// 调用：gateway/identity/handlers.go
// 测试：authz_test.go
func (db *DB) ListAudit(ctx context.Context, limit, offset int) ([]AuditEntry, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	var out []AuditEntry
	err := db.Engine.Context(ctx).Desc("id").Limit(limit, offset).Find(&out)
	return out, err
}
