// Package identity serves HTTP for users, teams, organizations, projects, and budgets. Module mounts the routes. This package does not import gateway.
package identity

import (
	"net/http"

	"github.com/sunqirui1987/xhub/internal/auth"
	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/store"
)

// Gate is the process capability these management handlers need. *gateway.Server implements it.
type Gate interface {
	RequireManage(w http.ResponseWriter, r *http.Request) *auth.Principal
	DB() *store.Store
	MakeKey(plain string, body map[string]any) (store.Key, error)
	KeyJSON(k store.Key, plain string, includePlain bool) map[string]any
	ModelList() []config.ModelEntry
	ModelPublic(m config.ModelEntry) map[string]any
}
