// Package identity defines the process capabilities account, organization,
// team, member and project management need. *gateway.Server implements them.
// This package does not import gateway.
package identity

import (
	"net/http"

	"github.com/sunqirui1987/xhub/internal/auth"
	"github.com/sunqirui1987/xhub/internal/authz"
	"github.com/sunqirui1987/xhub/internal/iam"
	"github.com/sunqirui1987/xhub/internal/logx"
)

// Gate is what these handlers ask the process for. It offers identity and
// authorization and nothing else: a handler resolves who is calling, asks for
// one decision per object, and only then touches identity data.
type Gate interface {
	// RequireUser accepts any signed-in session or virtual key. Most routes
	// here use it rather than RequireManage, because a member reads their own
	// team, its projects and its members; the authorization decision narrows
	// the reach, not the gate.
	RequireUser(w http.ResponseWriter, r *http.Request) *auth.Principal
	// RequireManage requires a platform administrator session.
	RequireManage(w http.ResponseWriter, r *http.Request) *auth.Principal
	// Authorize decides one action for an already resolved caller. Ownership is
	// read from the database inside the decision, so a handler cannot widen its
	// reach by naming a team the caller does not belong to.
	Authorize(r *http.Request, p *auth.Principal, action authz.Action, obj authz.Object) error
	// TeamFilter returns the teams whose rows a listing may include. A nil
	// slice means every team, which only a platform administrator receives; an
	// empty slice means no team.
	TeamFilter(r *http.Request, p *auth.Principal) ([]string, error)
	// Identity is the store. A handler reads its rows from here, but only after
	// Authorize has permitted the action that reads them.
	Identity() *iam.DB
	// WriteAuthz turns a refused authorization into the response and reports
	// whether one was written.
	WriteAuthz(w http.ResponseWriter, r *http.Request, err error) bool
	// WriteIAMError turns a store failure into the response and reports whether
	// one was written.
	WriteIAMError(w http.ResponseWriter, r *http.Request, err error) bool
}

// traceModule records that identity routes are being mounted.
func traceModule(name string) {
	logx.Trace("mount %s", name)
}
