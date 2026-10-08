// Package templateauth shares the visibility check across template binding and
// ordinary organization, team and key saves.
package templateauth

import (
	"net/http"
	"strings"

	"github.com/sunqirui1987/xhub/internal/auth"
	"github.com/sunqirui1987/xhub/internal/authz"
	"github.com/sunqirui1987/xhub/internal/iam"
)

type Host interface {
	Identity() *iam.DB
	Authorize(*http.Request, *auth.Principal, authz.Action, authz.Object) error
}

// Selection checks the selected template before any scope is mutated. Empty
// means inherit and needs no template read. Ownership must be loaded explicitly:
// route templates are not resolved by Authorize from their ID alone.
func Selection(h Host, r *http.Request, p *auth.Principal, id string) error {
	id = strings.TrimSpace(id)
	if id == "" { return nil }
	row, err := h.Identity().GetRouteTemplate(r.Context(), id)
	if err != nil { return &authz.InternalError{Err: err} }
	if row == nil { return authz.ErrNotFound }
	obj := authz.Object{Type: authz.ObjectRouteTemplate, ID: row.ID}
	if row.OrganizationID != nil { obj.OrgID = *row.OrganizationID }
	if row.TeamID != nil { obj.TeamID = *row.TeamID }
	return h.Authorize(r, p, authz.ActionRouteTemplateRead, obj)
}
