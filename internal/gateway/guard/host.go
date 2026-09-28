// Package guard runs content rules before a request is sent upstream. A blocking match stops the data plane from calling the provider.
package guard

import (
	"net/http"

	"github.com/sunqirui1987/xhub/internal/auth"
	"github.com/sunqirui1987/xhub/internal/logx"
	"github.com/sunqirui1987/xhub/internal/store"
)

// Host is what a guardrail trial and a config read ask the process for. *gateway.Server implements it. This package does not import gateway.
type Host interface {
	RequireManage(w http.ResponseWriter, r *http.Request) *auth.Principal
	DB() *store.Store
}

// traceModule records that guardrail routes are being mounted.
func traceModule(name string) {
	logx.Trace("mount %s", name)
}
