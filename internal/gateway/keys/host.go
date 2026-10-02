// Package keys defines the process capabilities virtual-key management needs. *gateway.Server implements them. This package does not import gateway.
package keys

import (
	"net/http"

	"github.com/sunqirui1987/xhub/internal/auth"
	"github.com/sunqirui1987/xhub/internal/logx"
	"github.com/sunqirui1987/xhub/internal/store"
)

// Host is what create, list, and update ask the process for.
type Host interface {
	RequireManage(w http.ResponseWriter, r *http.Request) *auth.Principal
	// RequireMixed accepts either a management identity or an inference identity. Key health checks use it. Pure management routes use RequireManage.
	RequireMixed(w http.ResponseWriter, r *http.Request) *auth.Principal
	DB() *store.Store
	ValidateKeyRelations(k store.Key) error
}

// traceModule records that virtual-key routes are being mounted.
func traceModule(name string) {
	logx.Trace("mount %s", name)
}
