// Package store persists gateway records in PostgreSQL. Schema names used by tests are quoted only when they are safe identifiers.
package store

import (
	"fmt"
	"github.com/sunqirui1987/xhub/internal/logx"
	"sync"
)

var logTraceOncePg sync.Once

// quoteIdent quotes a schema name. An empty, too-long, or symbolic name is rejected.
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
