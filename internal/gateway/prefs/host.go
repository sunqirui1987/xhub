// Package prefs serves router settings and general settings. A key present in the database overrides YAML. A key that is absent stays.
package prefs

import (
	"net/http"

	"github.com/sunqirui1987/xhub/internal/auth"
	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/store"
)

// Host is what settings reads and writes ask the process for. *gateway.Server implements it. This package does not import gateway.
// Config returns the in-process config pointer. ApplyTyped changes its routing strategy, retries, and timeout, the same fields as before this package was split, and it does not add a lock.
type Host interface {
	RequireManage(w http.ResponseWriter, r *http.Request) *auth.Principal
	DB() *store.Store
	Config() *config.Config
}
