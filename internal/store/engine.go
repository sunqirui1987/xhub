// 打开带缓存的 xorm 引擎，并按测试 schema 隔离表。
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

// SQL 开始前不记数。
func (l *stmtLog) BeforeSQL(log.LogContext) {}

// 每条真正发到数据库的语句加一。缓存命中不会走到这里。
func (l *stmtLog) AfterSQL(log.LogContext) {
	atomic.AddInt64(l.n, 1)
}

// 调试日志丢掉，避免把 SQL 文本再打一遍。
func (l *stmtLog) Debugf(string, ...interface{}) {}

// 错误日志丢掉，计数只看 AfterSQL。
func (l *stmtLog) Errorf(string, ...interface{}) {}

// 信息日志丢掉。
func (l *stmtLog) Infof(string, ...interface{}) {}

// 警告日志丢掉。
func (l *stmtLog) Warnf(string, ...interface{}) {}

// 调试日志丢掉。
func (l *stmtLog) Debug(...interface{}) {}

// 信息日志丢掉。
func (l *stmtLog) Info(...interface{}) {}

// 警告日志丢掉。
func (l *stmtLog) Warn(...interface{}) {}

// 错误日志丢掉。
func (l *stmtLog) Error(...interface{}) {}

// 返回当前日志级别。
func (l *stmtLog) Level() log.LogLevel { return l.lvl }

// 设置日志级别。
func (l *stmtLog) SetLevel(v log.LogLevel) { l.lvl = v }

// 打开 SQL 记录。不传参数时视为打开。
func (l *stmtLog) ShowSQL(show ...bool) {
	if len(show) == 0 {
		l.show = true
		return
	}
	l.show = show[0]
}

// 是否记录 SQL。关闭后 AfterSQL 不会被调用。
func (l *stmtLog) IsShowSQL() bool { return l.show }

// 连接 PostgreSQL，打开缓存，并把结构体同步成表。
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
	// 控制台 projects 上的 UNIQUE 约束不能被 Sync 删掉，否则进程起不来。结构体里没有声明唯一索引，所以这里不增删约束。
	if _, err := engine.SyncWithOptions(xorm.SyncOptions{IgnoreConstrains: true},
		new(userRow), new(teamRow), new(orgRow), new(projectRow), new(budgetRow),
		new(kvRow), new(tokenRow), new(spendRow), new(proxyModelRow), new(configRow),
	); err != nil {
		engine.Close()
		return nil, nil, nil, err
	}
	return engine, engine.DB().DB, counter, nil
}

// 连接串里有 search_path 时先建好这个 schema。没有则用 public。
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

// 把 search_path 写进连接参数。空格用 %20，避免被当成加号。
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
