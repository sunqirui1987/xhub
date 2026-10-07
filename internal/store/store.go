// Package store persists the gateway's framework records: key-value documents,
// dashboard-saved proxy models, and namespaced configuration. It accepts
// PostgreSQL only and rejects sqlite and an empty database_url.
//
// Identity, membership, keys, usage and the audit log are not here; they belong
// to internal/iam, which owns their schema and their constraints.
package store

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/sunqirui1987/xhub/internal/logx"
	"sync"
	"xorm.io/xorm"
)

// Store is the PostgreSQL storage. Reads and writes go through the cached xorm engine. DB is only for connectivity checks and test fixtures.
type Store struct {
	Engine *xorm.Engine
	DB     *sql.DB
	stmts  *int64
}

var logTraceOnceStore sync.Once

// Open opens storage. sqlite, file:, and an empty URL are rejected. Only postgres:// and postgresql:// are accepted.
// 参数 databaseURL（string）：打开使用的database地址。空串表示调用方没有提供这项。
// 返回 *Store（*Store）：打开的配置库；error（error）：失败原因，nil 表示成功。
// 调用：gateway/server.go、iam/db.go、live/redis.go、store/engine.go
// 测试：authz_test.go、builtin_providers_test.go、chains_test.go
func Open(databaseURL string) (*Store, error) {
	logTraceOnceStore.Do(func() { logx.Trace("enter store.Open") })

	u := strings.ToLower(strings.TrimSpace(databaseURL))
	switch {
	case u == "", strings.HasPrefix(u, "sqlite:"), strings.HasPrefix(u, "file:"):
		return nil, fmt.Errorf("sqlite is not supported; set general_settings.database_url to a postgres:// URL")
	case strings.HasPrefix(u, "postgres://"), strings.HasPrefix(u, "postgresql://"):
		engine, db, stmts, err := openEngine(databaseURL)
		if err != nil {
			return nil, err
		}
		return &Store{Engine: engine, DB: db, stmts: stmts}, nil
	default:
		return nil, fmt.Errorf("database_url must be postgres:// or postgresql://")
	}
}
