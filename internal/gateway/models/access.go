package models

import (
	"strings"

	"github.com/sunqirui1987/xhub/internal/auth"
	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/logx"
	"github.com/sunqirui1987/xhub/internal/store"
)

// AllowsModel is the model authorization decision shared by the model list and
// the inference path. Empty allow-lists mean "no additional restriction";
// multiple non-empty scopes are intersected.
func AllowsModel(s Host, p *auth.Principal, alias string) bool {
	logx.Trace("evaluate model access")
	if p == nil || strings.TrimSpace(alias) == "" {
		return false
	}
	if p.Kind == "master" {
		return true
	}

	scope := effectiveScope(s, p)
	if !scope.restricted {
		return true
	}
	return scope.contains(alias)
}

type modelScope struct {
	names      map[string]struct{}
	restricted bool
}

func unrestrictedScope() modelScope { return modelScope{} }

func restrictedScope(names []string) modelScope {
	set := map[string]struct{}{}
	for _, name := range names {
		if name = strings.TrimSpace(name); name != "" {
			set[name] = struct{}{}
		}
	}
	return modelScope{names: set, restricted: true}
}

func (s modelScope) contains(name string) bool {
	if !s.restricted {
		return true
	}
	_, ok := s.names[name]
	return ok
}

func linkedScopeValid(s Host, k *store.Key) bool {
	if k == nil || s == nil || s.DB() == nil {
		return true
	}
	var team *store.Entity
	if k.UserID != "" {
		user, err := s.DB().GetUser(k.UserID)
		if err != nil || user.BlockedState() {
			return false
		}
	}
	if k.TeamID != "" {
		var err error
		team, err = s.DB().GetTeam(k.TeamID)
		if err != nil || team.BlockedState() {
			return false
		}
	}
	if k.OrganizationID != "" {
		org, err := s.DB().GetOrg(k.OrganizationID)
		if err != nil || org.BlockedState() {
			return false
		}
		if team != nil && team.TeamID != k.OrganizationID {
			return false
		}
	}
	if k.ProjectID != "" {
		project, err := s.DB().GetProject(k.ProjectID)
		if err != nil || project.BlockedState() {
			return false
		}
		projectOrg, _ := project.Extra()["organization_id"].(string)
		if k.OrganizationID != "" && projectOrg != "" && projectOrg != k.OrganizationID {
			return false
		}
		if team != nil && project.TeamID != "" && project.TeamID != team.ID {
			return false
		}
	}
	return true
}

func intersectScopes(scopes ...modelScope) modelScope {
	var current map[string]struct{}
	restricted := false
	for _, scope := range scopes {
		if !scope.restricted {
			continue
		}
		if !restricted {
			current = cloneSet(scope.names)
			restricted = true
			continue
		}
		for name := range current {
			if _, ok := scope.names[name]; !ok {
				delete(current, name)
			}
		}
	}
	if !restricted {
		return unrestrictedScope()
	}
	return modelScope{names: current, restricted: true}
}

func unionScopes(scopes ...modelScope) modelScope {
	set := map[string]struct{}{}
	seenRestricted := false
	for _, scope := range scopes {
		if !scope.restricted {
			return unrestrictedScope()
		}
		seenRestricted = true
		for name := range scope.names {
			set[name] = struct{}{}
		}
	}
	if !seenRestricted {
		return unrestrictedScope()
	}
	return modelScope{names: set, restricted: true}
}

func cloneSet(in map[string]struct{}) map[string]struct{} {
	out := make(map[string]struct{}, len(in))
	for name := range in {
		out[name] = struct{}{}
	}
	return out
}

func effectiveScope(s Host, p *auth.Principal) modelScope {
	if s == nil || s.DB() == nil {
		return unrestrictedScope()
	}
	if p.Key != nil {
		if !linkedScopeValid(s, p.Key) {
			return restrictedScope(nil)
		}
		teamModels := []string(nil)
		scopes := []modelScope{}
		if p.Key.TeamID != "" {
			if team, err := s.DB().GetTeam(p.Key.TeamID); err == nil {
				teamModels = team.Models()
				scopes = append(scopes, grantScope(s, teamModels, nil, false))
			}
		}
		scopes = append(scopes, grantScope(s, p.Key.Models(), teamModels, true))
		if p.Key.UserID != "" {
			if user, err := s.DB().GetUser(p.Key.UserID); err == nil {
				scopes = append(scopes, grantScope(s, user.Models(), nil, false))
				scopes = append(scopes, membershipScopes(s, *user))
			}
		}
		if p.Key.OrganizationID != "" {
			if org, err := s.DB().GetOrg(p.Key.OrganizationID); err == nil {
				scopes = append(scopes, grantScope(s, org.Models(), nil, false))
			}
		}
		if p.Key.ProjectID != "" {
			if project, err := s.DB().GetProject(p.Key.ProjectID); err == nil {
				scopes = append(scopes, grantScope(s, project.Models(), nil, false))
			} else {
				return restrictedScope(nil)
			}
		}
		return intersectScopes(scopes...)
	}

	if p.UserID == "" {
		return unrestrictedScope()
	}
	user, err := s.DB().GetUser(p.UserID)
	if err != nil {
		return restrictedScope(nil)
	}
	return intersectScopes(grantScope(s, user.Models(), nil, false), membershipScopes(s, *user))
}

func grantScope(s Host, grant, teamModels []string, keyPass bool) modelScope {
	list := deploymentList(s)
	proxy := proxyModelNames(list)
	groups := modelAccessGroups(list)
	if len(grant) == 0 {
		return unrestrictedScope()
	}
	if containsStr(grant, "*") {
		return restrictedScope(proxy)
	}
	expanded := expandGrantedModels(grant, teamModels, proxy, groups, false, keyPass)
	expanded = expandWildcardNames(expanded, list, false)
	deployed := map[string]struct{}{}
	for _, name := range proxy {
		deployed[name] = struct{}{}
	}
	kept := make([]string, 0, len(expanded))
	for _, name := range dedupeModels(expanded) {
		if _, ok := deployed[name]; ok {
			kept = append(kept, name)
		}
	}
	return restrictedScope(kept)
}

func membershipScopes(s Host, user store.Entity) modelScope {
	teams := entitiesForIDs(s, extraIDs(user, "teams"), false)
	if all, err := s.DB().ListTeams(); err == nil {
		seen := map[string]struct{}{}
		for _, entity := range teams {
			seen[entity.ID] = struct{}{}
		}
		for _, entity := range all {
			if _, ok := seen[entity.ID]; !ok && memberMatches(entity, user.ID) {
				teams = append(teams, entity)
				seen[entity.ID] = struct{}{}
			}
		}
	}
	orgs := entitiesForIDs(s, extraIDs(user, "organizations"), true)
	if all, err := s.DB().ListOrgs(); err == nil {
		seen := map[string]struct{}{}
		for _, entity := range orgs {
			seen[entity.ID] = struct{}{}
		}
		for _, entity := range all {
			if _, ok := seen[entity.ID]; !ok && memberMatches(entity, user.ID) {
				orgs = append(orgs, entity)
				seen[entity.ID] = struct{}{}
			}
		}
	}

	scopes := []modelScope{}
	if len(teams) > 0 {
		teamScopes := make([]modelScope, 0, len(teams))
		for _, entity := range teams {
			teamScopes = append(teamScopes, grantScope(s, entity.Models(), nil, false))
		}
		scopes = append(scopes, unionScopes(teamScopes...))
	}
	if len(orgs) > 0 {
		orgScopes := make([]modelScope, 0, len(orgs))
		for _, entity := range orgs {
			orgScopes = append(orgScopes, grantScope(s, entity.Models(), nil, false))
		}
		scopes = append(scopes, unionScopes(orgScopes...))
	}
	if len(scopes) == 0 {
		return unrestrictedScope()
	}
	return intersectScopes(scopes...)
}

func memberMatches(entity store.Entity, userID string) bool {
	keys := []string{"members_with_roles"}
	if entity.TeamID == "" {
		keys = append(keys, "members")
	}
	for _, key := range keys {
		for _, raw := range entity.ExtraList(key) {
			member, ok := raw.(map[string]any)
			if ok && strValue(member["user_id"]) == userID {
				return true
			}
		}
	}
	return false
}

func entitiesForIDs(s Host, ids []string, organizations bool) []store.Entity {
	if len(ids) == 0 || s.DB() == nil {
		return nil
	}
	var out []store.Entity
	seen := map[string]struct{}{}
	if organizations {
		for _, id := range ids {
			if _, ok := seen[id]; ok {
				continue
			}
			if entity, err := s.DB().GetOrg(id); err == nil {
				out = append(out, *entity)
				seen[id] = struct{}{}
			}
		}
		return out
	}
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}
		if entity, err := s.DB().GetTeam(id); err == nil {
			out = append(out, *entity)
			seen[id] = struct{}{}
		}
	}
	return out
}

func deploymentList(s Host) []config.ModelEntry {
	s.LockModels()
	defer s.UnlockModels()
	return append([]config.ModelEntry(nil), (*s.ModelTable())...)
}

func extraIDs(entity store.Entity, key string) []string {
	var out []string
	seen := map[string]struct{}{}
	for _, raw := range entity.ExtraList(key) {
		id := strValue(raw)
		if m, ok := raw.(map[string]any); ok {
			for _, field := range []string{"team_id", "organization_id", "id"} {
				if id = strValue(m[field]); id != "" {
					break
				}
			}
		}
		if id != "" {
			if _, ok := seen[id]; !ok {
				seen[id] = struct{}{}
				out = append(out, id)
			}
		}
	}
	return out
}

func strValue(v any) string {
	s, _ := v.(string)
	return strings.TrimSpace(s)
}
