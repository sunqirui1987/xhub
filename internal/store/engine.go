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
// 参数 format：xorm 的格式串。args：填进格式串的值。这里都丢掉，避免 SQL 里的密钥进日志。
// 返回：无。
// 调用：xorm 在打开引擎之后。测试：无直接单测。
func (l *quietLog) Debugf(string, ...interface{}) {}

// Errorf discards error logs. Callers log their own failures with context.
// 参数 format：xorm 的格式串。args：填进格式串的值。这里都丢掉。
// 返回：无。
// 调用：xorm。测试：无直接单测。
func (l *quietLog) Errorf(string, ...interface{}) {}

// Infof discards info logs.
// 参数 format：xorm 的格式串。args：填进格式串的值。这里都丢掉。
// 返回：无。
// 调用：xorm。测试：无直接单测。
func (l *quietLog) Infof(string, ...interface{}) {}

// Warnf discards warning logs.
// 参数 format：xorm 的格式串。args：填进格式串的值。这里都丢掉。
// 返回：无。
// 调用：xorm。测试：无直接单测。
func (l *quietLog) Warnf(string, ...interface{}) {}

// Debug discards debug logs.
// 参数 args：xorm 拼好的调试片段。这里丢掉。
// 返回：无。
// 调用：xorm。测试：无直接单测。
func (l *quietLog) Debug(...interface{}) {}

// Info discards info logs.
// 参数 args：xorm 拼好的信息片段。这里丢掉。
// 返回：无。
// 调用：xorm。测试：无直接单测。
func (l *quietLog) Info(...interface{}) {}

// Warn discards warning logs.
// 参数 args：xorm 拼好的警告片段。这里丢掉。
// 返回：无。
// 调用：xorm。测试：无直接单测。
func (l *quietLog) Warn(...interface{}) {}

// Error discards error logs.
// 参数 args：xorm 拼好的错误片段。这里丢掉，避免把 SQL 打进进程日志。
// 返回：无。
// 调用：xorm。测试：无直接单测。
func (l *quietLog) Error(...interface{}) {}

// Level returns the current log level.
// 参数：无。
// 调用：仅在 engine.go 内使用
// 测试：无直接单测
// 返回：当前级别。quietLog 不按这个级别打印。
func (l *quietLog) Level() log.LogLevel { return l.lvl }

// SetLevel 满足 xorm 的日志接口。级别被记住，SQL 文本仍然不打印。
// 参数 v（log.LogLevel）：xorm 要求的日志级别，存下来供 Level 读回。
// 返回：无。不写 HTTP 响应，也不打印 SQL。
// 调用：xorm 在打开引擎之后。
// 测试：无直接单测
func (l *quietLog) SetLevel(v log.LogLevel) { l.lvl = v }

// ShowSQL 满足 xorm 的日志接口。无论传入什么，SQL 都不写入日志。
// 参数 show：xorm 是否想打印 SQL。这里忽略，SQL 始终不记录。
// 返回：无。
// 调用：xorm。这里忽略该开关，SQL 始终不记录。
// 测试：无直接单测
func (l *quietLog) ShowSQL(...bool) {}

// IsShowSQL reports that SQL is not recorded.
// 参数：无。
// 返回：恒为 false，表示不打印 SQL。
// 调用：仅在 engine.go 内使用
// 测试：无直接单测
func (l *quietLog) IsShowSQL() bool { return false }

var logTraceOnceEngine sync.Once

// openEngine connects to PostgreSQL, turns the cache on, and syncs structs into tables.
// 参数 databaseURL（string）：打开Engine使用的database地址。空串表示调用方没有提供这项。
// 返回 *xorm.Engine（*xorm.Engine）：已连上并打开缓存的引擎；*sql.DB（*sql.DB）：底层数据库连接；*int64（*int64）：恒为 nil，不再返回连接计数；error（error）：建模式、连接或同步表失败。nil 表示引擎可以用。失败时前三个都是 nil。
// 调用：store/store.go
// 测试：无直接单测
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
// 参数 databaseURL（string）：确保模式使用的database地址。空串表示调用方没有提供这项。
// 返回 string（string）：search_path 里的模式名，并且已经 CREATE SCHEMA IF NOT EXISTS。没有这个参数时返回空串，连接就用数据库默认的 public；error（error）：URL 解析失败或建模式失败。nil 表示可以继续连接。
// 调用：仅在 engine.go 内使用
// 测试：无直接单测
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
// 参数 databaseURL（string）：连接串带Search路径使用的database地址。空串表示调用方没有提供这项；schema（string）：PostgreSQL schema 名。非法名称会被拒绝。
// 返回 string（string）：写好 search_path 或 TimeZone 的连接串；error（error）：失败原因。nil 表示这一步成功。
// 调用：仅在 engine.go 内使用
// 测试：无直接单测
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
