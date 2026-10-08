package identity

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/sunqirui1987/xhub/internal/authz"
	"github.com/sunqirui1987/xhub/internal/gateway/prefs"
	"github.com/sunqirui1987/xhub/internal/gateway/templateauth"
	"github.com/sunqirui1987/xhub/internal/httpx"
	"github.com/sunqirui1987/xhub/internal/iam"
	"github.com/sunqirui1987/xhub/internal/logx"
)

// Router templates are named bundles of router settings that an organization, a
// team or a key selects from. The handlers live here rather than with the rest of
// the router settings because everything they need is here: who is calling, the
// one authorization decision per object, the identity store, and the error
// writers that turn a store failure into a response.
//
// Reading and writing the settings themselves is still prefs. This file only
// manages which named document a scope points at.

// templateView is one template as the console reads it. Body is handed over as
// parsed JSON rather than as the stored string so the console does not have to
// parse it a second time in a different language.
type templateView struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Body      map[string]any `json:"body"`
	UpdatedAt string         `json:"updated_at"`
	// UsedBy counts the scopes selecting this template. The list screen shows it,
	// and a non-zero value is what makes the delete button refuse.
	UsedBy int `json:"used_by"`
	// Writable says whether the caller may edit this one. A template a sibling
	// team built is visible and selectable but not editable, and the console
	// needs to know which is which rather than showing an edit button that fails.
	Writable bool `json:"writable"`
	// Ownership, so the console can group the list without inferring it.
	OrgID  string `json:"organization_id,omitempty"`
	TeamID string `json:"team_id,omitempty"`
}

// RouteTemplateList returns every template with its usage count.
// 参数 g（Gate）：带当前操作者的鉴权守卫。允许时返回 nil，拒绝时返回禁止或未找到；w（http.ResponseWriter）：调用方的 HTTP 响应，状态和正文写在这里；r（*http.Request）：入站 HTTP 请求。
// 返回：无。状态码和正文写进调用方的响应。
// 调用：gateway/identity/mount.go
// 测试：route_template_test.go
func RouteTemplateList(g Gate, w http.ResponseWriter, r *http.Request) {
	p := g.RequireUser(w, r)
	if p == nil {
		return
	}
	rows, err := g.Identity().ListRouteTemplates(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	counts, err := g.Identity().RouteTemplateUsageCounts(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// One row at a time through the same rule that governs selecting one, rather
	// than a query filter that would have to restate the visibility rule in SQL.
	// The two would drift, and the drift would be a tenant reading another
	// tenant's configuration.
	out := make([]templateView, 0, len(rows))
	for _, row := range rows {
		visible := authz.Object{
			Type:   authz.ObjectRouteTemplate,
			ID:     row.ID,
			OrgID:  deref(row.OrganizationID),
			TeamID: deref(row.TeamID),
		}
		if err := g.Authorize(r, p, authz.ActionRouteTemplateRead, visible); err != nil {
			continue
		}
		out = append(out, templateView{
			ID:        row.ID,
			Name:      row.Name,
			Body:      row.Settings(),
			UpdatedAt: row.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
			UsedBy:    counts[row.ID],
			Writable:  g.Authorize(r, p, authz.ActionRouteTemplateWrite, visible) == nil,
			OrgID:     deref(row.OrganizationID),
			TeamID:    deref(row.TeamID),
		})
	}
	httpx.WriteJSON(w, 200, map[string]any{"data": out})
}

// RouteTemplateGet returns one template.
// 参数 g（Gate）：带当前操作者的鉴权守卫。允许时返回 nil，拒绝时返回禁止或未找到；w（http.ResponseWriter）：调用方的 HTTP 响应，状态和正文写在这里；r（*http.Request）：入站 HTTP 请求。
// 返回：无。状态码和正文写进调用方的响应。
// 调用：gateway/identity/mount.go
// 测试：route_template_test.go
func RouteTemplateGet(g Gate, w http.ResponseWriter, r *http.Request) {
	if g.RequireManage(w, r) == nil {
		return
	}
	id := r.PathValue("template_id")
	row, err := g.Identity().GetRouteTemplate(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if row == nil {
		httpx.WriteError(w, 404, "not_found", "route template not found")
		return
	}
	httpx.WriteJSON(w, 200, templateView{
		ID:        row.ID,
		Name:      row.Name,
		Body:      row.Settings(),
		UpdatedAt: row.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
	})
}

// RouteTemplateCreate inserts a template.
// 参数 g（Gate）：带当前操作者的鉴权守卫。允许时返回 nil，拒绝时返回禁止或未找到；w（http.ResponseWriter）：调用方的 HTTP 响应，状态和正文写在这里；r（*http.Request）：入站 HTTP 请求。
// 返回：无。状态码和正文写进调用方的响应。
// 调用：gateway/identity/mount.go
// 测试：route_template_test.go
func RouteTemplateCreate(g Gate, w http.ResponseWriter, r *http.Request) {
	p := g.RequireManage(w, r)
	if p == nil {
		return
	}
	body := readMap(r)
	name := strings.TrimSpace(str(body["name"]))
	if name == "" {
		httpx.WriteError(w, 400, "invalid_request", "name required")
		return
	}
	// A new template starts as a complete document seeded from the platform
	// default, not as an empty one.
	//
	// Resolution replaces rather than merges, so a template that omits a key
	// means that key's zero value - one attempt, a zero timeout. Seeding makes every
	// template in the table a working configuration, and an operator who wants
	// one attempt can still delete the line.
	encoded, err := encodeTemplateBody(seedFromPlatform(g.RouterDocument(), body["body"]))
	if err != nil {
		httpx.WriteError(w, 400, "invalid_request", err.Error())
		return
	}
	if err := validateTemplateModelRouting(encoded); err != nil {
		httpx.WriteError(w, 400, "invalid_request", err.Error())
		return
	}
	// Who this template belongs to. An absent pair means a platform template,
	// which is what the current create path produces and what the existing
	// tests exercise; an organization or team makes it theirs.
	owner := iam.TemplateOwner{OrgID: strings.TrimSpace(str(body["organization_id"])),
		TeamID: strings.TrimSpace(str(body["team_id"]))}
	if owner.TeamID != "" && owner.OrgID == "" {
		// A team template with no organization is unreachable by every rule that
		// reads the organization first, and the database refuses it too. Answering
		// here gives the caller a reason instead of a 500.
		httpx.WriteError(w, 400, "invalid_request", "organization_id is required when team_id is given")
		return
	}
	if err := g.Authorize(r, p, authz.ActionRouteTemplateWrite, authz.Object{
		Type: authz.ObjectRouteTemplate, OrgID: owner.OrgID, TeamID: owner.TeamID,
	}); err != nil {
		g.WriteAuthz(w, r, err)
		return
	}
	row, err := g.Identity().CreateRouteTemplate(r.Context(), actorOf(p), owner, name, encoded)
	if err != nil {
		g.WriteIAMError(w, r, err)
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"id": row.ID, "name": row.Name, "status": "success"})
}

// RouteTemplateUpdate replaces a template's name and body.
// 参数 g（Gate）：带当前操作者的鉴权守卫。允许时返回 nil，拒绝时返回禁止或未找到；w（http.ResponseWriter）：调用方的 HTTP 响应，状态和正文写在这里；r（*http.Request）：入站 HTTP 请求。
// 返回：无。状态码和正文写进调用方的响应。
// 调用：gateway/identity/mount.go
// 测试：route_template_test.go
func RouteTemplateUpdate(g Gate, w http.ResponseWriter, r *http.Request) {
	p := g.RequireUser(w, r)
	if p == nil {
		return
	}
	id := r.PathValue("template_id")
	existing, err := g.Identity().GetRouteTemplate(ctx0(r), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if existing == nil {
		httpx.WriteError(w, 404, "not_found", "route template not found")
		return
	}
	// Editing is the owner's alone. A sibling team's template is readable and
	// selectable; it is not yours to change.
	if err := g.Authorize(r, p, authz.ActionRouteTemplateWrite, authz.Object{
		Type: authz.ObjectRouteTemplate, ID: existing.ID,
		OrgID: deref(existing.OrganizationID), TeamID: deref(existing.TeamID),
	}); err != nil {
		g.WriteAuthz(w, r, err)
		return
	}
	body := readMap(r)
	encoded, err := encodeTemplateBody(body["body"])
	if err != nil {
		httpx.WriteError(w, 400, "invalid_request", err.Error())
		return
	}
	if err := validateTemplateModelRouting(encoded); err != nil {
		httpx.WriteError(w, 400, "invalid_request", err.Error())
		return
	}
	row, err := g.Identity().UpdateRouteTemplate(r.Context(), actorOf(p), id, str(body["name"]), encoded)
	if err != nil {
		g.WriteIAMError(w, r, err)
		return
	}
	if row == nil {
		httpx.WriteError(w, 404, "not_found", "route template not found")
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"id": row.ID, "name": row.Name, "status": "success"})
}

// RouteTemplateDelete removes a template, refusing while any scope still
// selects it.
//
// The refusal carries the scopes that hold it because "in use" alone leaves the
// operator to go find them, and the difference between "I removed something
// nobody used" and "I changed three teams' behaviour" is the whole reason to
// refuse in the first place.
//
// 参数 g（Gate）：带当前操作者的鉴权守卫。允许时返回 nil，拒绝时返回禁止或未找到；w（http.ResponseWriter）：调用方的 HTTP 响应，状态和正文写在这里；r（*http.Request）：入站 HTTP 请求。
// 返回：无。状态码和正文写进调用方的响应。
// 调用：gateway/identity/mount.go
// 测试：route_template_test.go
func RouteTemplateDelete(g Gate, w http.ResponseWriter, r *http.Request) {
	p := g.RequireUser(w, r)
	if p == nil {
		return
	}
	id := r.PathValue("template_id")
	existing, err := g.Identity().GetRouteTemplate(ctx0(r), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if existing == nil {
		httpx.WriteError(w, 404, "not_found", "route template not found")
		return
	}
	if err := g.Authorize(r, p, authz.ActionRouteTemplateWrite, authz.Object{
		Type: authz.ObjectRouteTemplate, ID: existing.ID,
		OrgID: deref(existing.OrganizationID), TeamID: deref(existing.TeamID),
	}); err != nil {
		g.WriteAuthz(w, r, err)
		return
	}
	usage, err := g.Identity().RouteTemplateUsage(ctx0(r), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if len(usage) > 0 {
		httpx.WriteJSON(w, 409, map[string]any{
			"error": map[string]any{
				"code":    "template_in_use",
				"message": "this template is still selected by an organization, a team or a key",
				"used_by": usage,
			},
		})
		return
	}
	if err := g.Identity().DeleteRouteTemplate(ctx0(r), actorOf(p), id); err != nil {
		g.WriteIAMError(w, r, err)
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"status": "success"})
}

// RouteTemplateUsage lists the scopes selecting one template. It is the same list
// the delete refusal carries, for an operator who wants to look before deleting.
// 参数 g（Gate）：带当前操作者的鉴权守卫。允许时返回 nil，拒绝时返回禁止或未找到；w（http.ResponseWriter）：调用方的 HTTP 响应，状态和正文写在这里；r（*http.Request）：入站 HTTP 请求。
// 返回：无。状态码和正文写进调用方的响应。
// 调用：gateway/identity/mount.go
// 测试：route_template_test.go
func RouteTemplateUsage(g Gate, w http.ResponseWriter, r *http.Request) {
	if g.RequireManage(w, r) == nil {
		return
	}
	usage, err := g.Identity().RouteTemplateUsage(ctx0(r), r.PathValue("template_id"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"data": usage})
}

// RouteTemplateBinding reads or writes which template one scope selects.
//
// Selecting is narrower than editing: an organization administrator may point
// their own organization, a team administrator their own team, and a key's owner
// their own key. That is why each branch asks for the write action on the scope's
// own object rather than for the platform administrator gate. An empty template
// id clears the selection, which is how a scope goes back to inheriting.
//
// 参数 g（Gate）：带当前操作者的鉴权守卫。允许时返回 nil，拒绝时返回禁止或未找到；w（http.ResponseWriter）：调用方的 HTTP 响应，状态和正文写在这里；r（*http.Request）：入站 HTTP 请求。
// 返回：无。状态码和正文写进调用方的响应。
// 调用：gateway/identity/mount.go
// 测试：route_template_test.go
func RouteTemplateBinding(g Gate, w http.ResponseWriter, r *http.Request) {
	p := g.RequireUser(w, r)
	if p == nil {
		return
	}
	body := readMap(r)
	// A read addresses the scope in the query string and a write in the body,
	// because a GET carries no body. One handler serves both so the two cannot
	// drift apart in which scope names they accept.
	scope := strings.TrimSpace(str(body["scope"]))
	scopeID := strings.TrimSpace(str(body["scope_id"]))
	if scope == "" {
		scope = strings.TrimSpace(r.URL.Query().Get("scope"))
	}
	if scopeID == "" {
		scopeID = strings.TrimSpace(r.URL.Query().Get("scope_id"))
	}
	action, obj, ok := bindingTarget(scope, scopeID)
	if !ok {
		httpx.WriteError(w, 400, "invalid_request", "scope must be organization, team or key and scope_id is required")
		return
	}
	if err := g.Authorize(r, p, action, obj); err != nil {
		g.WriteAuthz(w, r, err)
		return
	}

	templateID := strings.TrimSpace(str(body["route_template_id"]))
	// Two decisions, not one. The one above says the caller may point this scope;
	// this one says the template they named is one they may use.
	//
	// It is the read rule rather than a separate visibility rule, because
	// visibility and selectability are deliberately the same thing here: a
	// template is usable by exactly the scopes whose members can see it. Keeping
	// them one rule means the console's list and the data plane's check cannot
	// disagree about what a caller is allowed to pick.
	//
	// Without this, a team administrator could aim their own team at another
	// organization's configuration and then route by it. It was harmless while
	// only a platform administrator could author a template, and becomes a
	// cross-tenant hole the moment organizations author their own.
	//
	// An invisible template answers 404, so the caller cannot tell "not yours"
	// from "does not exist".
	if err := templateauth.Selection(g, r, p, templateID); err != nil {
		g.WriteAuthz(w, r, err)
		return
	}
	if r.Method == http.MethodGet {
		current, err := g.Identity().ScopeRouteTemplate(ctx0(r), scope, scopeID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		resolved, err := resolveRouteTemplate(g, r, scope, scopeID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		inherited := routeTemplateResolution{ScopeType: prefs.PlatformSource}
		if parent, parentID, parentErr := parentScope(g.Identity(), r, scope, scopeID); parentErr != nil {
			http.Error(w, parentErr.Error(), http.StatusInternalServerError)
			return
		} else if parentID != "" {
			inherited, err = resolveRouteTemplate(g, r, parent, parentID)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
		httpx.WriteJSON(w, 200, map[string]any{
			"scope": scope, "scope_id": scopeID,
			// selected is what this scope chose; effective is what it will
			// actually route by, which differs when it chose nothing and
			// inherited from the level above.
			"route_template_id": current,
			"effective":         resolved,
			"inherited":         inherited,
		})
		return
	}

	if err := g.Identity().SetScopeRouteTemplate(ctx0(r), actorOf(p), scope, scopeID, templateID); err != nil {
		g.WriteIAMError(w, r, err)
		return
	}
	logx.Info("route template bound scope=%s id=%s template=%s", scope, scopeID, templateID)
	resolved, err := resolveRouteTemplate(g, r, scope, scopeID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{
		"status": "success", "scope": scope, "scope_id": scopeID,
		// The effective template is what the scope will actually route by, which
		// is not the same as what it selected: a scope that selects nothing
		// inherits from the level above it.
		"effective": resolved,
	})
}

// bindingTarget maps a scope name onto the object the caller is pointing, and the
// action that governs pointing it.
//
// The action is selection rather than the scope's own write action. Selecting is
// narrower: it picks among configurations somebody already approved, and it
// applies to exactly the scopes the caller already belongs to, so a member may do
// it for their own team while still being unable to change the team's budget, its
// model ceiling or its routing template's contents.
//
// An unknown scope is rejected rather than defaulted: the name
// reaches SQL in the store, and a wrong default would let one scope write
// another's selection.
// 参数 scope（string）：organization、team 或 key；scopeID（string）：那个范围内的一行。
// 返回 authz.Action（authz.Action）：要判定的写动作；authz.Object（authz.Object）：要判定的对象；bool（bool）：范围名被承认且 id 非空时为真。
// 调用：RouteTemplateBinding。
// 测试：route_template_test.go
func bindingTarget(scope, scopeID string) (authz.Action, authz.Object, bool) {
	if scopeID == "" {
		return "", authz.Object{}, false
	}
	switch scope {
	case "organization":
		return authz.ActionRouteTemplateSelect, authz.Object{Type: authz.ObjectOrg, ID: scopeID}, true
	case "team":
		return authz.ActionRouteTemplateSelect, authz.Object{Type: authz.ObjectTeam, ID: scopeID}, true
	case "key":
		return authz.ActionRouteTemplateSelect, authz.Object{Type: authz.ObjectKey, ID: scopeID}, true
	default:
		return "", authz.Object{}, false
	}
}

// routeTemplateResolution is what a scope will actually route by, and where that
// came from. The console shows both: "inherits Cheap first (from Organization)"
// is the answer to "why is this team's traffic split this way", and without the
// origin a scope that selects nothing looks the same as one that selected
// something.
type routeTemplateResolution struct {
	TemplateID string `json:"template_id,omitempty"`
	Name       string `json:"name,omitempty"`
	ScopeType  string `json:"scope_type"` // organization | team | key | platform
	ScopeID    string `json:"scope_id,omitempty"`
}

// resolveRouteTemplate reports what one scope will actually route by.
//
// It calls the same resolver the request path calls, with the chain the request
// path would have built - so the console cannot tell an operator a team routes
// by one template while the data plane uses another. An earlier version walked
// the chain a second time here, and the two would have drifted the first time
// either changed.
//
// 参数 g（Gate）：带当前操作者的鉴权守卫，用来读平台默认那一份设置；r（*http.Request）：入站 HTTP 请求；scope（string）：起点范围；scopeID（string）：起点范围内的一行。
// 返回 routeTemplateResolution（routeTemplateResolution）：生效的模板和它来自哪一层；error（error）：失败原因，nil 表示这一步成功。
// 调用：RouteTemplateBinding。
// 测试：route_template_test.go
func resolveRouteTemplate(g Gate, r *http.Request, scope, scopeID string) (routeTemplateResolution, error) {
	lookup := &requestLookup{db: g.Identity(), ctx: ctx0(r), byID: map[string]*iam.RouteTemplate{}, bound: map[string]string{}}
	// The chain above the starting scope is discovered by walking outward, which
	// is what the store does not already know: the starting row names its parent,
	// not the other way round.
	chain := []prefs.ScopeRef{{Kind: scope, ID: scopeID}}
	at, id, err := parentScope(g.Identity(), r, scope, scopeID)
	if err != nil {
		return routeTemplateResolution{}, err
	}
	for id != "" && len(chain) < 3 {
		chain = append(chain, prefs.ScopeRef{Kind: at, ID: id})
		at, id, err = parentScope(g.Identity(), r, at, id)
		if err != nil {
			return routeTemplateResolution{}, err
		}
	}
	for _, step := range chain {
		if lookup.TemplateFor(step.Kind, step.ID) != "" {
			if lookup.err != nil {
				return routeTemplateResolution{}, lookup.err
			}
			resolved := prefs.Resolve(g.RouterDocument(), lookup, chain...)
			if lookup.err != nil {
				return routeTemplateResolution{}, lookup.err
			}
			return routeTemplateResolution{
				TemplateID: resolved.TemplateID,
				Name:       resolved.TemplateName,
				ScopeType:  resolved.Source,
				ScopeID:    step.ID,
			}, nil
		}
		if lookup.err != nil {
			return routeTemplateResolution{}, lookup.err
		}
	}
	return routeTemplateResolution{ScopeType: prefs.PlatformSource}, nil
}

// requestLookup caches the template reads one console request makes.
type requestLookup struct {
	byID  map[string]*iam.RouteTemplate
	bound map[string]string
	db    *iam.DB
	ctx   context.Context
	err   error
}

// TemplateFor returns the template one scope selects.
// 参数 kind（string）：key、team 或 organization；id（string）：那一行。
// 返回 string（string）：选用的模板 id，没选时为空串。
// 调用：prefs.Resolve。
// 测试：route_template_test.go
func (l *requestLookup) TemplateFor(kind, id string) string {
	key := kind + "\x00" + id
	if got, ok := l.bound[key]; ok {
		return got
	}
	selected, err := l.db.ScopeRouteTemplate(l.ctx, kind, id)
	if err != nil {
		l.err = err
		return ""
	}
	l.bound[key] = selected
	return selected
}

// Load returns one template by id.
// 参数 id（string）：模板 id。
// 返回 *iam.RouteTemplate（*iam.RouteTemplate）：这一行，没有时为 nil。
// 调用：prefs.Resolve。
// 测试：route_template_test.go
func (l *requestLookup) Load(id string) *iam.RouteTemplate {
	if got, ok := l.byID[id]; ok {
		return got
	}
	row, err := l.db.GetRouteTemplate(l.ctx, id)
	if err != nil {
		l.err = err
		return nil
	}
	l.byID[id] = row
	return row
}

// parentScope walks one step outward: key to team, team to organization,
// organization to nothing.
// 参数 db（*iam.DB）：身份库；r（*http.Request）：入站 HTTP 请求；scope（string）：当前范围；scopeID（string）：当前范围内的那一行。
// 返回 string（string）：上一层范围；string（string）：上一层的那一行。
// 调用：resolveRouteTemplate。
// 测试：route_template_test.go
func parentScope(db *iam.DB, r *http.Request, scope, scopeID string) (string, string, error) {
	switch scope {
	case "key":
		key, err := db.GetKey(ctx0(r), scopeID)
		if err != nil {
			return "", "", err
		}
		if key == nil {
			return "", "", nil
		}
		// A key belonging to nobody but an organization still has a team; a key
		// with no team at all stops here.
		return "team", key.TeamID, nil
	case "team":
		team, err := db.GetTeam(ctx0(r), scopeID)
		if err != nil {
			return "", "", err
		}
		if team == nil {
			return "", "", nil
		}
		return "organization", team.OrganizationID, nil
	default:
		return "", "", nil
	}
}

// seedFromPlatform fills a new template's document from the platform default, so
// every template in the table is a complete configuration.
//
// Resolution replaces rather than merges. A template that omitted num_retries
// would mean one attempt; one that omitted timeout would mean a zero timeout. Those
// are legitimate things to configure and surprising things to get by accident, so
// the starting point is the platform document and the operator edits it down.
//
// A body the caller did supply wins outright, including one that sets a key to
// its zero value - the point is to fill what was left out, not to second-guess
// what was written.
// 参数 platform（map[string]any）：平台默认那一份路由设置；supplied（any）：调用方提交的正文，可能缺失。
// 返回 map[string]any（map[string]any）：要存进库的完整文档。
// 调用：RouteTemplateCreate。
// 测试：route_template_test.go
func seedFromPlatform(platform map[string]any, supplied any) map[string]any {
	seed := map[string]any{}
	for key, value := range platform {
		seed[key] = value
	}
	override, ok := supplied.(map[string]any)
	if !ok {
		// An empty or absent body keeps the seed as it stands. RouteTemplateUpdate
		// takes the body as written, so this only touches creation.
		return seed
	}
	for key, value := range override {
		seed[key] = value
	}
	return seed
}

// encodeTemplateBody turns the request's body field into the stored string. An
// absent body is an empty document rather than an error: a template whose
// settings are not filled in yet is a name the operator is about to use.
// 参数 raw（any）：请求里的 body 字段，可能是一份设置对象，也可能缺失。
// 返回 string（string）：要存进库的 JSON；error（error）：格式不对时报错。
// 调用：RouteTemplateCreate、RouteTemplateUpdate。
// 测试：route_template_test.go
func encodeTemplateBody(raw any) (string, error) {
	switch v := raw.(type) {
	case nil:
		return "{}", nil
	case map[string]any:
		out, err := json.Marshal(v)
		if err != nil {
			return "", errors.New("body is not encodable")
		}
		return string(out), nil
	case string:
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			return "{}", nil
		}
		// It has to be an object: an array or a scalar would parse and then
		// behave as an empty document at request time, which hides the mistake.
		probe := map[string]any{}
		if err := json.Unmarshal([]byte(trimmed), &probe); err != nil {
			return "", errors.New("body must be a JSON object")
		}
		return trimmed, nil
	default:
		return "", errors.New("body must be a JSON object")
	}
}

func validateTemplateModelRouting(encoded string) error {
	document := map[string]any{}
	if err := json.Unmarshal([]byte(encoded), &document); err != nil {
		return errors.New("body must be a JSON object")
	}
	return prefs.ValidateModelRoutingDocument(document)
}

// ctx0 is the request context, or the background context when the request has
// none. Handlers here read after the response is decided, and a cancelled
// request context must not turn a completed write into a reported failure.
// 参数 r（*http.Request）：入站 HTTP 请求。
// 返回 context.Context（context.Context）：可用的上下文。
// 调用：本文件所有 handler。
// 测试：无直接单测
func ctx0(r *http.Request) context.Context {
	if r == nil {
		return context.Background()
	}
	return r.Context()
}
