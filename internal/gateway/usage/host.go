// Package usage serves spend reports, usage summaries and health probes. Every
// number comes from the usage_events and usage_daily tables in PostgreSQL;
// nothing is recomputed from a log file at request time.
//
// This package never decides who may read what. It asks the process for a scope
// (authz.UsageScope for usage, authz.LogsScope for request logs) and hands that
// scope to iam, which applies it as a WHERE fragment. A handler that forgot to
// ask for a scope would have nothing to pass, because iam takes the scope as a
// required argument rather than a default.
package usage

import (
	"net/http"

	"github.com/sunqirui1987/xhub/internal/auth"
	"github.com/sunqirui1987/xhub/internal/authz"
	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/iam"
	"github.com/sunqirui1987/xhub/internal/logx"
)

// Host is what the usage handlers ask the process for. *gateway.Server implements
// it. This package does not import gateway.
type Host interface {
	// RequireManage requires a platform administrator session. The global
	// spend reports use it: they are a platform-wide view by definition.
	RequireManage(w http.ResponseWriter, r *http.Request) *auth.Principal
	// RequireUser accepts any signed-in caller. The per-user and per-team
	// summaries use it, then narrow the rows with a scope.
	RequireUser(w http.ResponseWriter, r *http.Request) *auth.Principal
	// Identity is the usage and key store. A handler reads from it only with a
	// scope it obtained from one of the two methods below.
	Identity() *iam.DB
	// UsageScope narrows a usage listing to what the caller may see, optionally
	// limited to one team. It is fail-closed: a caller who may see nothing
	// receives a scope that matches no row.
	UsageScope(r *http.Request, p *auth.Principal, teamID string) (*authz.Scope, error)
	// LogsScope narrows a request-log listing: the caller's own personal logs,
	// plus the service-key logs of the teams they administer.
	LogsScope(r *http.Request, p *auth.Principal) (*authz.Scope, error)
	// WriteAuthz turns a refused scope into the response and reports whether one
	// was written. A scope failure is answered rather than ignored, because a
	// failed scope is not an empty listing.
	WriteAuthz(w http.ResponseWriter, r *http.Request, err error) bool
	// WriteIAMError turns a store failure into the response and reports whether
	// one was written.
	WriteIAMError(w http.ResponseWriter, r *http.Request, err error) bool
	// AuditLogRead records that a caller read a request log they do not own.
	// Only a platform administrator reading someone else's content produces a
	// row; reading your own log is not an audited event.
	AuditLogRead(r *http.Request, p *auth.Principal, requestID string, e iam.UsageEvent) error
	// ModelList returns a copy of the current model table. Benchmarks use it
	// only to find strategy-router names. They do not invent scores when there
	// is no sample.
	ModelList() []config.ModelEntry
}

// traceModule records that usage routes are being mounted.
func traceModule(name string) {
	logx.Trace("mount %s", name)
}
