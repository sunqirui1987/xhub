package models

import (
	"context"
	"strings"

	"github.com/sunqirui1987/xhub/internal/auth"
	"github.com/sunqirui1987/xhub/internal/logx"
)

// AllowsModel is the model authorization decision shared by the model catalog
// and the inference path, so a listed model is always a usable model.
//
// The permitted set comes from iam alone, which resolves it down one chain:
//
//	team    = union of the team's active access groups
//	project = team ∩ project narrowing (when the key names a project)
//	key     = (project or team) ∩ key narrowing (when the key narrows)
//
// teamID narrows a session that is browsing one team's catalog; a key ignores it
// and answers for the single team it is bound to, never merging capabilities
// across teams. A scope with no grants at all is unrestricted: nothing has been
// assigned yet, and an empty assignment is not a denial.
func AllowsModel(s Host, ctx context.Context, p *auth.Principal, teamID, alias string) bool {
	logx.Trace("evaluate model access")
	alias = strings.TrimSpace(alias)
	if p == nil || alias == "" {
		return false
	}
	if p.IsMaster() {
		return true
	}
	names, err := effectiveModels(s, ctx, p, teamID)
	if err != nil {
		logx.Error("model access failed kind=%s user=%s err=%v", p.Kind, p.UserID, err)
		return false
	}
	if names == nil {
		return true
	}
	for _, name := range names {
		if name == alias {
			return true
		}
	}
	return false
}

// effectiveModels is the caller's permitted model names. A nil slice means the
// caller is unrestricted; an error means the check could not be made and the
// caller must deny rather than fall open.
//
// Only a key carries a model binding. A session has none of its own, so it is
// narrowed only when it names a team, and otherwise sees every deployment.
func effectiveModels(s Host, ctx context.Context, p *auth.Principal, teamID string) ([]string, error) {
	if s == nil || s.Identity() == nil {
		return nil, nil
	}
	if p.Key != nil {
		return s.Identity().AllowedModelsForKey(ctx, p.Key)
	}
	if teamID != "" {
		return s.Identity().AllowedModelsForTeam(ctx, teamID)
	}
	return nil, nil
}
