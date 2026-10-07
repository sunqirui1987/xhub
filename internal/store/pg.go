// Package store persists gateway records in PostgreSQL. Schema names used by tests are quoted only when they are safe identifiers.
package store

import (
	"fmt"
	"github.com/sunqirui1987/xhub/internal/logx"
	"sync"
)

var logTraceOncePg sync.Once

// quoteIdent quotes a schema name. An empty, too-long, or symbolic name is rejected.
// 参数 name（string）：要引用的 PostgreSQL 模式名。空串、超过 63 字符或含符号时拒绝。
// 返回 string（string）：加上双引号的模式名，可以放进 CREATE SCHEMA。空串、过长或含符号时为空串；error（error）：名字不合法。nil 表示可以引用。
// 调用：store/engine.go
// 测试：无直接单测
func quoteIdent(name string) (string, error) {
	logTraceOncePg.Do(func() { logx.Trace("enter store.quoteIdent") })

	if name == "" || len(name) > 63 {
		return "", fmt.Errorf("invalid schema name")
	}
	for _, c := range name {
		if c != '_' && (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') {
			return "", fmt.Errorf("invalid schema name")
		}
	}
	return `"` + name + `"`, nil
}
