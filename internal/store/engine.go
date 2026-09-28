// Package store opens a cached xorm engine and can isolate tables in a test schema.
package store

import (
	"database/sql"
	"fmt"
	"net/url"
	"strings"
	"sync/atomic"

	_ "github.com/jackc/pgx/v5/stdlib"
	"xorm.io/xorm"
	"xorm.io/xorm/caches"
	"xorm.io/xorm/log"
)

type stmtLog struct {
	n    *int64
	show bool
	lvl  log.LogLevel
}

// BeforeSQL does not count. The statement has not been sent yet.
func (l *stmtLog) BeforeSQL(log.LogContext) {}

// AfterSQL increments once for each statement actually sent to the database. A cache hit does not reach here.
func (l *stmtLog) AfterSQL(log.LogContext) {
	atomic.AddInt64(l.n, 1)
}

// Debugf discards debug logs so SQL text is not printed again.
func (l *stmtLog) Debugf(string, ...interface{}) {}

// Errorf discards error logs. The statement count looks only at AfterSQL.
func (l *stmtLog) Errorf(string, ...interface{}) {}

// Infof discards info logs.
func (l *stmtLog) Infof(string, ...interface{}) {}

// Warnf discards warning logs.
func (l *stmtLog) Warnf(string, ...interface{}) {}

// Debug discards debug logs.
func (l *stmtLog) Debug(...interface{}) {}

// Info discards info logs.
func (l *stmtLog) Info(...interface{}) {}

// Warn discards warning logs.
func (l *stmtLog) Warn(...interface{}) {}

// Error discards error logs.
func (l *stmtLog) Error(...interface{}) {}

// Level returns the current log level.
func (l *stmtLog) Level() log.LogLevel { return l.lvl }

// SetLevel sets the log level.
func (l *stmtLog) SetLevel(v log.LogLevel) { l.lvl = v }

// ShowSQL turns SQL recording on. With no argument it is treated as on.
func (l *stmtLog) ShowSQL(show ...bool) {
	if len(show) == 0 {
		l.show = true
		return
	}
	l.show = show[0]
}

// IsShowSQL reports whether SQL is recorded. When it is off, AfterSQL is not called.
func (l *stmtLog) IsShowSQL() bool { return l.show }

// openEngine connects to PostgreSQL, turns the cache on, and syncs structs into tables.
func openEngine(databaseURL string) (*xorm.Engine, *sql.DB, *int64, error) {
	schema, err := ensureSchema(databaseURL)
	if err != nil {
		return nil, nil, nil, err
	}
	dsn := databaseURL
	if schema != "" {
		dsn, err = dsnWithSearchPath(databaseURL, schema)
		if err != nil {
			return nil, nil, nil, err
		}
	}
	engine, err := xorm.NewEngine("pgx", dsn)
	if err != nil {
		return nil, nil, nil, err
	}
	counter := new(int64)
	logger := &stmtLog{n: counter, show: true, lvl: log.LOG_INFO}
	engine.SetLogger(logger)
	engine.ShowSQL(true)
	if schema != "" {
		engine.SetSchema(schema)
	}
	engine.SetDefaultCacher(caches.NewLRUCacher(caches.NewMemoryStore(), 10000))
	if err := engine.Ping(); err != nil {
		engine.Close()
		return nil, nil, nil, err
	}
	// Sync must not drop the UNIQUE constraint on dashboard projects, or the process cannot start. The structs declare no unique index, so this sync does not add or drop constraints.
	if _, err := engine.SyncWithOptions(xorm.SyncOptions{IgnoreConstrains: true},
		new(userRow), new(teamRow), new(orgRow), new(projectRow), new(budgetRow),
		new(kvRow), new(tokenRow), new(spendRow), new(proxyModelRow), new(configRow),
	); err != nil {
		engine.Close()
		return nil, nil, nil, err
	}
	return engine, engine.DB().DB, counter, nil
}

// ensureSchema creates the schema named by search_path when the connection string has one. Otherwise it uses public.
func ensureSchema(databaseURL string) (string, error) {
	u, err := url.Parse(databaseURL)
	if err != nil {
		return "", err
	}
	schema := u.Query().Get("search_path")
	if schema == "" {
		return "", nil
	}
	ident, err := quoteIdent(schema)
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Del("search_path")
	u.RawQuery = q.Encode()
	admin, err := sql.Open("pgx", u.String())
	if err != nil {
		return "", err
	}
	defer admin.Close()
	_, err = admin.Exec(fmt.Sprintf("CREATE SCHEMA IF NOT EXISTS %s", ident))
	return schema, err
}

// dsnWithSearchPath writes search_path into the connection parameters. A space is encoded as %20 so it is not treated as a plus.
func dsnWithSearchPath(databaseURL, schema string) (string, error) {
	u, err := url.Parse(databaseURL)
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Del("search_path")
	q.Del("options")
	encoded := q.Encode()
	opt := "options=" + strings.ReplaceAll(url.QueryEscape("-c search_path="+schema), "+", "%20")
	if encoded != "" {
		u.RawQuery = encoded + "&" + opt
	} else {
		u.RawQuery = opt
	}
	return u.String(), nil
}
