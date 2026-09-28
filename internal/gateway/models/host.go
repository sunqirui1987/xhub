// Package models defines the process capabilities model management needs. *gateway.Server implements them. This package does not import gateway, which avoids an import cycle.
package models

import (
	"net/http"

	"github.com/sunqirui1987/xhub/internal/auth"
	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/logx"
	"github.com/sunqirui1987/xhub/internal/store"
)

// Host is what create, update, and list ask the process for.
// Update, delete, and block write the database while holding the model lock. The lock covers the same critical section as before this package was split.
type Host interface {
	RequireManage(w http.ResponseWriter, r *http.Request) *auth.Principal
	DB() *store.Store
	// LockModels and UnlockModels are a pair. Lock before changing the model table on a request path.
	LockModels()
	UnlockModels()
	// ModelTable returns a pointer to the in-process model slice. Change it only while holding the lock. LoadStored runs before the process serves traffic, when there are no concurrent requests.
	ModelTable() *[]config.ModelEntry
	// Resolve parses a session or a virtual key. On failure it does not write a response. The caller chooses the 401 body.
	Resolve(r *http.Request) (*auth.Principal, error)
	// AllowLLM reports whether this identity may call inference. The master key may not unless allow_master_key_llm is on.
	AllowLLM(p *auth.Principal) bool
}

// traceModule records that model routes are being mounted.
func traceModule(name string) {
	logx.Trace("mount %s", name)
}
