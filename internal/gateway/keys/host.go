// Package keys defines the process capabilities virtual-key management needs. *gateway.Server implements them. This package does not import gateway.
package keys

import (
	"net/http"

	"github.com/sunqirui1987/xhub/internal/auth"
	"github.com/sunqirui1987/xhub/internal/authz"
	"github.com/sunqirui1987/xhub/internal/iam"
	"github.com/sunqirui1987/xhub/internal/logx"
)

// Host is what create, list, and update ask the process for. It offers identity
// and authorization and nothing else: a handler resolves who is calling, asks
// for one decision per object, and only then touches identity data.
type Host interface {
	// RequireUser accepts any signed-in session or virtual key. Key routes use
	// it rather than RequireManage, because a member mints and reads their own
	// keys; the authorization decision narrows the reach, not the gate.
	RequireUser(w http.ResponseWriter, r *http.Request) *auth.Principal
	// RequireManage requires a platform administrator session.
	RequireManage(w http.ResponseWriter, r *http.Request) *auth.Principal
	// RequireMixed accepts a management identity or an inference identity. The
	// liveness check uses it.
	RequireMixed(w http.ResponseWriter, r *http.Request) *auth.Principal
	// Authorize decides one action for an already resolved caller. Ownership is
	// read from the database inside the decision, so a handler cannot widen its
	// reach by naming a team the key does not belong to.
	Authorize(r *http.Request, p *auth.Principal, action authz.Action, obj authz.Object) error
	// KeysScope returns the key-listing scope, so a listing is narrowed in SQL
	// instead of being filtered after the rows are read.
	KeysScope(r *http.Request, p *auth.Principal) (*authz.Scope, error)
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

// traceModule records that virtual-key routes are being mounted.
func traceModule(name string) {
	logx.Trace("mount %s", name)
}
