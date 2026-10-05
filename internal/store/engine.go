// Package store opens a cached xorm engine and can isolate tables in a test schema.
package store

import (
	"database/sql"
	"fmt"
	"net/url"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/sunqirui1987/xhub/internal/logx"
	"sync"
	"xorm.io/xorm"
	"xorm.io/xorm/caches"
	"xorm.io/xorm/log"
)

// quietLog discards xorm's own logging. SQL text carries key hashes and
// credential values, so it is never printed.
type quietLog struct {
	lvl log.LogLevel
}

// Debugf discards debug logs.
func (l *quietLog) Debugf(string, ...interface{}) {}

// Errorf discards error logs. Callers log their own failures with context.
func (l *quietLog) Errorf(string, ...interface{}) {}

// Infof discards info logs.
func (l *quietLog) Infof(string, ...interface{}) {}

// Warnf discards warning logs.
func (l *quietLog) Warnf(string, ...interface{}) {}

// Debug discards debug logs.
func (l *quietLog) Debug(...interface{}) {}

// Info discards info logs.
func (l *quietLog) Info(...interface{}) {}

// Warn discards warning logs.
func (l *quietLog) Warn(...interface{}) {}

// Error discards error logs.
func (l *quietLog) Error(...interface{}) {}

// Level returns the current log level.
func (l *quietLog) Level() log.LogLevel { return l.lvl }

// SetLevel sets the log level.
func (l *quietLog) SetLevel(v log.LogLevel) { l.lvl = v }

// ShowSQL is accepted and ignored. SQL recording stays off.
func (l *quietLog) ShowSQL(...bool) {}

// IsShowSQL reports that SQL is not recorded.
func (l *quietLog) IsShowSQL() bool { return false }

var logTraceOnceEngine sync.Once

// openEngine connects to PostgreSQL, turns the cache on, and syncs structs into tables.
func openEngine(databaseURL string) (*xorm.Engine, *sql.DB, *int64, error) {
	logTraceOnceEngine.Do(func() { logx.Trace("enter store.openEngine") })

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
	engine.SetLogger(&quietLog{lvl: log.LOG_INFO})
	engine.ShowSQL(false)
	if schema != "" {
		engine.SetSchema(schema)
	}
	// The query cache covers framework reads only. Identity and session reads go
	// through internal/iam, which deliberately installs no cache.
	engine.SetDefaultCacher(caches.NewLRUCacher(caches.NewMemoryStore(), 10000))
	if err := engine.Ping(); err != nil {
		engine.Close()
		return nil, nil, nil, err
	}
	if _, err := engine.SyncWithOptions(xorm.SyncOptions{IgnoreConstrains: true},
		new(kvRow), new(proxyModelRow), new(configRow),
	); err != nil {
		engine.Close()
		return nil, nil, nil, err
	}
	return engine, engine.DB().DB, nil, nil
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
