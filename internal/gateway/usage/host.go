// Package usage defines what spend reports and auto-router benchmarks ask the process for. Numbers come from PostgreSQL. This package does not write spend logs.
package usage

import (
	"net/http"

	"github.com/sunqirui1987/xhub/internal/auth"
	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/store"
)

// Host is what the usage handlers ask the process for. *gateway.Server implements it. This package does not import gateway.
type Host interface {
	RequireManage(w http.ResponseWriter, r *http.Request) *auth.Principal
	DB() *store.Store
	// ModelList returns a copy of the current model table. Benchmarks use it only to find strategy-router names. They do not invent scores when there is no sample.
	ModelList() []config.ModelEntry
}
