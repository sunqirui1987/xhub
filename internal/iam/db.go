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
)

// DB wraps the xorm engine. No query cache is installed: authorization must
// read live rows.
type DB struct {
	Engine *xorm.Engine
}

// Open connects with the pgx driver and applies the schema.
//
// The engine keeps xorm's default time zones — DatabaseTZ and TZLocation both
// Local — and they must agree with the session zone. xorm formats a time.Time
// into the database zone on the way in and re-labels the value it reads back
// into the application zone, while pgx returns the instant in the session zone.
// Pinning any one of the three to UTC while the others stayed Local made stored
// timestamps land eight hours off on a UTC+8 machine: created_at and updated_at
// disagreed with columns PostgreSQL filled in itself, because DEFAULT now() is
// computed by the server and never passes through this conversion.
//
// Setting all three together is what makes the round trip lossless. It matters
// that the session zone is named explicitly rather than left to the server,
// since a server whose zone differs from the machine's would reintroduce the
// same skew.
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
func (db *DB) Migrate(ctx context.Context) error {
	if _, err := db.Engine.Context(ctx).Exec(schemaSQL); err != nil {
		logx.Error("iam schema apply failed: %v", err)
		return err
	}
	return nil
}

// Close releases the engine.
func (db *DB) Close() error { return db.Engine.Close() }

// tx runs fn in one xorm transaction and rolls back on any error.
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
func (db *DB) session(ctx context.Context) *xorm.Session {
	return db.Engine.NewSession().Context(ctx)
}

// dsnWithSessionZone returns the DSN with the connection's TimeZone set to the
// machine's own zone.
//
// The session zone is what pgx uses to render a TIMESTAMPTZ, and xorm reads that
// rendering back as though it were already in the application zone. Leaving the
// session to the server means a server configured for UTC and a machine at
// UTC+8 disagree by the offset, so the zone is named here rather than assumed.
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

// serverZone renders a Go location as a zone name PostgreSQL understands.
//
// A location that knows its name is used directly. Otherwise the offset is sent
// in the POSIX form, and it is easy to get backwards: PostgreSQL reads the sign
// of a numeric zone as the offset *west* of UTC, so a machine at UTC+8 is named
// "-08" there, not "+08:00". Sending the Go spelling would put the session in
// UTC-8 and move every stored timestamp by twice the offset.
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

// affected turns a zero-row write into ErrNotFound.
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

// RecordAudit writes an audit row outside a larger transaction, e.g. when a
// platform administrator reads someone else's request log.
func (db *DB) RecordAudit(ctx context.Context, a Actor, e Audit) error {
	s := db.session(ctx)
	defer s.Close()
	return writeAudit(s, a, e)
}

// ListAudit returns the newest rows first.
func (db *DB) ListAudit(ctx context.Context, limit, offset int) ([]AuditEntry, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	var out []AuditEntry
	err := db.Engine.Context(ctx).Desc("id").Limit(limit, offset).Find(&out)
	return out, err
}
