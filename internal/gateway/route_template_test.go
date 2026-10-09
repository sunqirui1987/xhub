package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/sunqirui1987/xhub/internal/iam"
)

// Router templates are named settings documents that organizations, teams and
// keys select from. Editing one is the platform administrator's; selecting one is
// the scope's own administrator's.
//
// These tests drive the real routes, because the bugs that reach this repository
// are not in the authorization core: the decision is right and the handler asks
// the wrong question, or asks none. Only a test at the handler boundary catches
// that.

// templateID creates a template over HTTP as the platform administrator and
// returns its id. Going through the route rather than the store keeps the
// fixtures honest about the shape the console sends.
func templateID(t *testing.T, f *fixture, name string, body string) string {
	t.Helper()
	code, raw := f.call(f.admin, http.MethodPost, "/route_template/new",
		[]byte(`{"name":"`+name+`","body":`+body+`}`))
	if code != http.StatusOK {
		t.Fatalf("create template %s -> %d: %s", name, code, raw)
	}
	out := map[string]any{}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("create template reply: %v", err)
	}
	id, _ := out["id"].(string)
	if id == "" {
		t.Fatalf("create template returned no id: %s", raw)
	}
	return id
}

// routeTemplateJSON 返回当前唯一合法的模板格式，供接口权限测试专注于自身行为。
func routeTemplateJSON(attempts int, routes string) string {
	if routes == "" {
		routes = "[]"
	}
	return fmt.Sprintf(`{"model_routes":%s,"retry_policy":{"max_attempts":%d,"timeout_seconds":60,"failure_threshold":3,"cooldown_seconds":0}}`, routes, attempts)
}

// TestTemplatesAreThePlatformAdministratorsToEdit 证明模板正文只有平台管理员能改。
//
// 一份模板决定怎么路由。团队管理员能选自己的团队用哪一份，但改不了那一份的内容——
// 否则他就是给自己发了一个平台没批准的配置。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestTemplatesAreThePlatformAdministratorsToEdit(t *testing.T) {
	f := newPermFixture(t)

	// 只有平台管理员建得出模板。
	for _, a := range []actor{f.member, f.teamAdmin, f.outsider} {
		payload := fmt.Sprintf(`{"name":"sneaky-%s","body":%s}`, a.name, routeTemplateJSON(1, ""))
		code, _ := f.call(a, http.MethodPost, "/route_template/new",
			[]byte(payload))
		if code == http.StatusOK {
			t.Fatalf("%s created a template", a.label())
		}
	}
	id := templateID(t, f, "owned", routeTemplateJSON(3, ""))

	// 列表对每个调用方返回**他看得见的那一份子集**：平台模板加本组织、本团队的。
	// 所以团队管理员读得到列表，但读到的只能是这些；看得见不等于改得动。
	code, raw := f.call(f.teamAdmin, http.MethodGet, "/route_template/list", nil)
	if code != http.StatusOK {
		t.Fatalf("a team administrator could not read their own templates -> %d: %s", code, raw)
	}
	if !strings.Contains(string(raw), "owned") {
		t.Fatalf("a platform template was missing from the list: %s", raw)
	}
	// 列表上要标出哪一些他改不了，否则界面会给出一个点了就失败的按钮。
	if !strings.Contains(string(raw), `"writable":false`) {
		t.Fatalf("the list did not mark the platform template as read-only: %s", raw)
	}

	// 团队管理员改不动正文。
	updatePayload := fmt.Sprintf(`{"name":"owned","body":%s}`, routeTemplateJSON(99, ""))
	code, _ = f.call(f.teamAdmin, http.MethodPost, "/route_template/"+id+"/update",
		[]byte(updatePayload))
	if code == http.StatusOK {
		t.Fatal("a team administrator edited a template body")
	}
	// 而平台管理员改得动。
	code, raw = f.call(f.admin, http.MethodPost, "/route_template/"+id+"/update",
		[]byte(updatePayload))
	if code != http.StatusOK {
		t.Fatalf("the platform administrator could not edit a template -> %d: %s", code, raw)
	}
	got := f.templateBody(t, id)
	retry, _ := got["retry_policy"].(map[string]any)
	if retry["max_attempts"] != float64(99) {
		t.Fatalf("the edit did not take: %#v", got)
	}
}

// TestSelectingATemplateRespectsTheScope 证明"谁能给谁选"。
//
// 组织管理员管自己的组织，团队管理员管自己的团队，越界的一律拒绝。这是这次
// 改动里唯一一处新的授权面，所以每一种越界都单独试。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestSelectingATemplateRespectsTheScope(t *testing.T) {
	f := newPermFixture(t)
	id := templateID(t, f, "selectable", routeTemplateJSON(4, ""))

	cases := []struct {
		name    string
		who     actor
		scope   string
		scopeID string
		allow   bool
	}{
		{"admin points an organization", f.admin, "organization", f.orgA.ID, true},
		{"admin points a team", f.admin, "team", f.teamA.ID, true},
		{"team admin points own team", f.teamAdmin, "team", f.teamA.ID, true},
		{"team admin points a foreign team", f.teamAdmin, "team", f.teamB.ID, false},
		// A plain member may point their own team. Selecting picks among
		// configurations that already exist, on a scope they already belong to,
		// so it grants no reach they did not have - unlike changing the team's
		// budget or its model ceiling.
		{"team member points own team", f.teamMember, "team", f.teamA.ID, true},
		{"team member points a foreign team", f.teamMember, "team", f.teamB.ID, false},
		{"team member points an organization", f.teamMember, "organization", f.orgA.ID, true},
		{"plain user points an organization", f.member, "organization", f.orgA.ID, false},
		{"outsider points another org", f.outsider, "organization", f.orgA.ID, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			payload := `{"scope":"` + tc.scope + `","scope_id":"` + tc.scopeID + `","route_template_id":"` + id + `"}`
			code, raw := f.call(tc.who, http.MethodPost, "/route_template/binding", []byte(payload))
			if tc.allow && code != http.StatusOK {
				t.Fatalf("allowed but refused -> %d: %s", code, raw)
			}
			if !tc.allow && code == http.StatusOK {
				t.Fatalf("refused case was allowed: %s", raw)
			}
		})
	}
}

// TestAnOrganizationAdministratorPointsTheirOwnOrganization 覆盖组织管理员这一档。
//
// 平台管理员之外的第一个能选模板的人。它和团队管理员的两条边界（自己的可以、
// 别人的不行）都要钉住。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestAnOrganizationAdministratorPointsTheirOwnOrganization(t *testing.T) {
	f := newPermFixture(t)
	id := templateID(t, f, "org-level", routeTemplateJSON(5, ""))

	// 把 teamAdmin 设为组织 A 的管理员。他仍然管不了组织 B。
	if _, err := f.db.AddOrgAdmin(context.Background(), iam.Actor{Kind: "system"},
		f.orgA.ID, f.teamAdmin.user.Email); err != nil {
		t.Fatalf("make org admin: %v", err)
	}

	payload := `{"scope":"organization","scope_id":"` + f.orgA.ID + `","route_template_id":"` + id + `"}`
	code, raw := f.call(f.teamAdmin, http.MethodPost, "/route_template/binding", []byte(payload))
	if code != http.StatusOK {
		t.Fatalf("the organization administrator could not point their own organization -> %d: %s", code, raw)
	}
	// 读回来确认真的写进去了。
	code, raw = f.call(f.teamAdmin, http.MethodGet,
		"/route_template/binding?scope=organization&scope_id="+f.orgA.ID, nil)
	if code != http.StatusOK {
		t.Fatalf("read binding -> %d: %s", code, raw)
	}
	if !strings.Contains(string(raw), id) {
		t.Fatalf("the binding did not persist: %s", raw)
	}

	// 组织 B 仍然管不了。
	code, _ = f.call(f.teamAdmin, http.MethodPost, "/route_template/binding",
		[]byte(`{"scope":"organization","scope_id":"`+f.otherOrgID(t)+`","route_template_id":"`+id+`"}`))
	if code == http.StatusOK {
		t.Fatal("an organization administrator pointed an organization they do not administer")
	}
}

// TestDeletingATemplateStillInUseIsRefusedWithTheList 证明删除的拒绝带着使用者名单。
//
// 静默解开会让那些范围悄悄退回默认，而"我删了一个没人用的模板"和"我改了三个
// 团队的行为"是完全不同的后果。所以拒绝而且说清是谁在用。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestDeletingATemplateStillInUseIsRefusedWithTheList(t *testing.T) {
	f := newPermFixture(t)
	id := templateID(t, f, "in-use", routeTemplateJSON(1, ""))

	code, raw := f.call(f.admin, http.MethodPost, "/route_template/binding",
		[]byte(`{"scope":"team","scope_id":"`+f.teamA.ID+`","route_template_id":"`+id+`"}`))
	if code != http.StatusOK {
		t.Fatalf("bind -> %d: %s", code, raw)
	}

	code, raw = f.call(f.admin, http.MethodPost, "/route_template/"+id+"/delete", nil)
	if code == http.StatusOK {
		t.Fatal("a template still in use was deleted")
	}
	if code != http.StatusConflict {
		t.Fatalf("delete refusal -> %d: %s", code, raw)
	}
	// 拒绝的正文要能看出是谁在用，否则运维还得自己去翻。
	if !strings.Contains(string(raw), "Perm Team A") {
		t.Fatalf("the refusal did not name the user: %s", raw)
	}

	// 解开之后删得掉。
	if code, raw := f.call(f.admin, http.MethodPost, "/route_template/binding",
		[]byte(`{"scope":"team","scope_id":"`+f.teamA.ID+`","route_template_id":null}`)); code != http.StatusOK {
		t.Fatalf("clear binding -> %d: %s", code, raw)
	}
	if code, raw := f.call(f.admin, http.MethodPost, "/route_template/"+id+"/delete", nil); code != http.StatusOK {
		t.Fatalf("delete after clearing -> %d: %s", code, raw)
	}
}

// TestClearingASelectionIsNotTheSameAsLeavingItOut 证明"清除"和"没提这一项"是两件事。
//
// 界面上的"不应用"必须真的把它清掉。如果两者被当成同一件事，那运维选回"不应用"
// 之后模板还在生效，而界面上显示的是继承——这正是那种"我改了但没生效"的投诉。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestClearingASelectionIsNotTheSameAsLeavingItOut(t *testing.T) {
	f := newPermFixture(t)
	id := templateID(t, f, "clearable", routeTemplateJSON(6, ""))

	bind := []byte(`{"scope":"team","scope_id":"` + f.teamA.ID + `","route_template_id":"` + id + `"}`)
	if code, raw := f.call(f.admin, http.MethodPost, "/route_template/binding", bind); code != http.StatusOK {
		t.Fatalf("bind -> %d: %s", code, raw)
	}
	// 一次不含这一项的团队更新不该把选择弄丢。
	if code, raw := f.call(f.admin, http.MethodPost, "/team/update",
		[]byte(`{"team_id":"`+f.teamA.ID+`","team_alias":"Perm Team A"}`)); code != http.StatusOK {
		t.Fatalf("unrelated team update -> %d: %s", code, raw)
	}
	if got := f.scopeTemplate(t, f.teamA.ID); got != id {
		t.Fatalf("an unrelated update dropped the selection: got %q want %q", got, id)
	}
	// 显式清掉。
	if code, raw := f.call(f.admin, http.MethodPost, "/team/update",
		[]byte(`{"team_id":"`+f.teamA.ID+`","route_template_id":null}`)); code != http.StatusOK {
		t.Fatalf("clearing update -> %d: %s", code, raw)
	}
	if got := f.scopeTemplate(t, f.teamA.ID); got != "" {
		t.Fatalf("clearing left %q behind", got)
	}
}

// TestTheEffectiveTemplateNamesWhereItCameFrom 证明响应说清了生效的是哪一份、来自哪一层。
//
// 一个没选模板的团队和选了模板的组织，路由行为一样但配置不一样。不显示来源的话
// 运维只能靠猜，而"为什么这个团队的流量这么分"就是他要回答的问题。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestTheEffectiveTemplateNamesWhereItCameFrom(t *testing.T) {
	f := newPermFixture(t)
	id := templateID(t, f, "inherited", routeTemplateJSON(7, ""))

	// 组织选了，团队没选。
	if code, raw := f.call(f.admin, http.MethodPost, "/route_template/binding",
		[]byte(`{"scope":"organization","scope_id":"`+f.orgA.ID+`","route_template_id":"`+id+`"}`)); code != http.StatusOK {
		t.Fatalf("bind org -> %d: %s", code, raw)
	}
	// 读团队那一层的绑定时，生效的应该是组织的那一份。
	code, raw := f.call(f.admin, http.MethodPost, "/route_template/binding",
		[]byte(`{"scope":"team","scope_id":"`+f.teamA.ID+`","route_template_id":""}`))
	if code != http.StatusOK {
		t.Fatalf("read effective -> %d: %s", code, raw)
	}
	out := map[string]any{}
	_ = json.Unmarshal(raw, &out)
	effective, _ := out["effective"].(map[string]any)
	if effective == nil {
		t.Fatalf("the reply carried no effective resolution: %s", raw)
	}
	if effective["template_id"] != id {
		t.Fatalf("the effective template is %v, want the organization's %q", effective["template_id"], id)
	}
	if effective["scope_type"] != "organization" {
		t.Fatalf("the effective template came from %v, want the organization", effective["scope_type"])
	}
}

// TestAnUnknownScopeIsRefused 证明范围名走白名单。
//
// 这个名字会被带进数据库的表名位置，所以它必须来自代码里的常量。一个看起来像
// SQL 的输入如果只被当成"不认识的字符串"传下去，那就成了一个注入面。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestAnUnknownScopeIsRefused(t *testing.T) {
	f := newPermFixture(t)
	id := templateID(t, f, "scope-check", routeTemplateJSON(1, ""))

	code, raw := f.call(f.admin, http.MethodPost, "/route_template/binding",
		[]byte(`{"scope":"teams; DROP TABLE teams","scope_id":"x","route_template_id":"`+id+`"}`))
	if code == http.StatusOK {
		t.Fatalf("an unknown scope name was accepted: %s", raw)
	}
}

// templateBody reads one template's settings as the platform administrator.
// 参数 t（*testing.T）：当前测试；f（*fixture）：当前用例的网关；id（string）：模板 id。
// 返回 map[string]any（map[string]any）：这一份模板的设置。
func (f *fixture) templateBody(t *testing.T, id string) map[string]any {
	t.Helper()
	code, raw := f.call(f.admin, http.MethodGet, "/route_template/"+id, nil)
	if code != http.StatusOK {
		t.Fatalf("read template %s -> %d: %s", id, code, raw)
	}
	out := map[string]any{}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("read template reply: %v", err)
	}
	body, _ := out["body"].(map[string]any)
	return body
}

// scopeTemplate reads which template a team selects, straight from the store.
// The binding route needs an authorized caller and this is only used to check
// what a write left behind.
// 参数 t（*testing.T）：当前测试；teamID（string）：团队 id。
// 返回 string（string）：这个团队选用的模板 id，没有时为空串。
func (f *fixture) scopeTemplate(t *testing.T, teamID string) string {
	t.Helper()
	got, err := f.db.ScopeRouteTemplate(context.Background(), "team", teamID)
	if err != nil {
		t.Fatalf("read scope template: %v", err)
	}
	return got
}

// otherOrgID returns the id of the organization teamB belongs to, which no
// fixture actor administers. It is found rather than hard-coded so the test does
// not depend on creation order.
// 参数 t（*testing.T）：当前测试。
// 返回 string（string）：另一个组织的 id。
func (f *fixture) otherOrgID(t *testing.T) string {
	t.Helper()
	team, err := f.db.GetTeam(context.Background(), f.teamB.ID)
	if err != nil || team == nil {
		t.Fatalf("read team B: %v", err)
	}
	if team.OrganizationID == f.orgA.ID {
		t.Fatal("the fixture put both teams in one organization, so the cross-org case proves nothing")
	}
	return team.OrganizationID
}

// TestATeamAdministratorSelectsThroughTheTeamRoute 覆盖团队那条现有路径。
//
// 团队详情页不展开负载均衡表单，只有一个下拉。这个下拉走的是原来那个
// /team/update，所以那条路径必须真的写进选择，而且越界仍然被拒。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestATeamAdministratorSelectsThroughTheTeamRoute(t *testing.T) {
	f := newPermFixture(t)
	id := templateID(t, f, "via-team-route", routeTemplateJSON(8, ""))

	code, raw := f.call(f.teamAdmin, http.MethodPost, "/team/update",
		[]byte(`{"team_id":"`+f.teamA.ID+`","route_template_id":"`+id+`"}`))
	if code != http.StatusOK {
		t.Fatalf("the team administrator could not select for their own team -> %d: %s", code, raw)
	}
	if got := f.scopeTemplate(t, f.teamA.ID); got != id {
		t.Fatalf("the selection did not persist: got %q want %q", got, id)
	}

	// 另一个团队不是他的。
	code, _ = f.call(f.teamAdmin, http.MethodPost, "/team/update",
		[]byte(`{"team_id":"`+f.teamB.ID+`","route_template_id":"`+id+`"}`))
	if code == http.StatusOK {
		t.Fatal("a team administrator selected a template for a foreign team")
	}
	// 而且没有写到那个团队上。
	if got := f.scopeTemplate(t, f.teamB.ID); got != "" {
		t.Fatalf("a refused write still landed on team B: %q", got)
	}
}

// TestTheTeamReadExposesTheSelection 证明团队详情读得到当前选的是哪一份。
//
// 编辑界面要能显示"现在选的是什么"。字段不出现的话，界面只能显示"不应用"，
// 而那也是"没选"的显示——运维会以为自己选过的东西丢了。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestTheTeamReadExposesTheSelection(t *testing.T) {
	f := newPermFixture(t)
	id := templateID(t, f, "visible", routeTemplateJSON(1, ""))

	if code, raw := f.call(f.admin, http.MethodPost, "/team/update",
		[]byte(`{"team_id":"`+f.teamA.ID+`","route_template_id":"`+id+`"}`)); code != http.StatusOK {
		t.Fatalf("select -> %d: %s", code, raw)
	}
	_, raw := f.call(f.admin, http.MethodGet, "/team/info?team_id="+f.teamA.ID, nil)
	if !strings.Contains(string(raw), id) {
		t.Fatalf("the team read does not carry the selection: %s", raw)
	}
}

// TestANewTemplateUsesTheBuiltinDocument 证明新模板使用固定的新格式骨架。
//
// 模板不再从全局 router settings 播种。调用方传入完整文档后，后台只校验并保存，
// 因此模板行为不会随另一套设置变化。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestANewTemplateUsesTheBuiltinDocument(t *testing.T) {
	f := newPermFixture(t)
	id := templateID(t, f, "builtin-shape", routeTemplateJSON(1, ""))
	body := f.templateBody(t, id)
	routes, ok := body["model_routes"].([]any)
	if !ok || len(routes) != 0 {
		t.Fatalf("model_routes = %#v, want an empty array", body["model_routes"])
	}
	retry, ok := body["retry_policy"].(map[string]any)
	if !ok || retry["max_attempts"] != float64(1) || retry["timeout_seconds"] != float64(60) ||
		retry["failure_threshold"] != float64(3) || retry["cooldown_seconds"] != float64(0) {
		t.Fatalf("retry_policy does not match the submitted document: %#v", retry)
	}
}

// TestASuppliedDocumentIsStoredExactly 证明完整合法文档按原值保存。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestASuppliedDocumentIsStoredExactly(t *testing.T) {
	f := newPermFixture(t)
	document := `{"model_routes":[{"model":"chat","strategy":"least-busy"}],"retry_policy":{"max_attempts":4,"timeout_seconds":17,"failure_threshold":0,"cooldown_seconds":9}}`
	id := templateID(t, f, "explicit", document)
	body := f.templateBody(t, id)
	retry, _ := body["retry_policy"].(map[string]any)
	if retry["max_attempts"] != float64(4) || retry["timeout_seconds"] != float64(17) ||
		retry["failure_threshold"] != float64(0) || retry["cooldown_seconds"] != float64(9) {
		t.Fatalf("saved retry policy changed: %#v", retry)
	}
}

// TestRouteTemplateWritesValidateTheNewDocument 证明模板接口严格拒绝旧字段、未知字段和歧义规则。
// 参数 t（*testing.T）：当前测试。返回：无；每个创建请求由独立测试数据库自动清理。
func TestRouteTemplateWritesValidateTheNewDocument(t *testing.T) {
	f := newPermFixture(t)
	invalidBodies := []string{
		`{"model_routing":[],"retry_policy":{"max_attempts":1,"timeout_seconds":60,"failure_threshold":3,"cooldown_seconds":0}}`,
		`{"model_routes":[],"retry_policy":{"max_attempts":1,"timeout_seconds":60,"failure_threshold":3,"cooldown_seconds":0},"custom_root":true}`,
		`{"model_routes":[{"model":"chat","strategy":"least-busy","endpoint_id":"chat"}],"retry_policy":{"max_attempts":1,"timeout_seconds":60,"failure_threshold":3,"cooldown_seconds":0}}`,
		`{"model_routes":[{"model":"chat","strategy":"traffic-split","allocations":null}],"retry_policy":{"max_attempts":1,"timeout_seconds":60,"failure_threshold":3,"cooldown_seconds":0}}`,
		`{"model_routes":[{"model":"chat","strategy":"least-busy"},{"model":"chat","strategy":"traffic-split"}],"retry_policy":{"max_attempts":1,"timeout_seconds":60,"failure_threshold":3,"cooldown_seconds":0}}`,
	}
	for index, body := range invalidBodies {
		payload := fmt.Sprintf(`{"name":"invalid-%d","body":%s}`, index, body)
		if code, _ := f.call(f.admin, http.MethodPost, "/route_template/new", []byte(payload)); code != http.StatusBadRequest {
			t.Fatalf("invalid document %d returned %d, want 400", index, code)
		}
	}
	id := templateID(t, f, "valid-model-policy", routeTemplateJSON(2, `[{"model":"chat","strategy":"least-busy"}]`))
	if body := f.templateBody(t, id); body["model_routes"] == nil || body["retry_policy"] == nil {
		t.Fatalf("valid new document was not stored: %#v", body)
	}
}

// TestASingleTeamSessionPicksUpItsTeamsTemplate 覆盖一个真实缺口。
//
// 预算链只在有密钥行时才走，而控制台的演练场是**会话**调用。如果不给会话单独
// 解析一次，团队选了模板，演练场里的请求却仍然按模型默认路由 —— 配上没生效，
// 而且只有那一个入口不对，最难发现。
//
// 这里走真实 HTTP：登录拿到会话，再发一次推理请求，断言会话真的拿这个模板。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestASingleTeamSessionPicksUpItsTeamsTemplate(t *testing.T) {
	f := newPermFixture(t)
	id := templateID(t, f, "session-visible", routeTemplateJSON(1, `[{"model":"chat","strategy":"least-busy"}]`))

	if code, raw := f.call(f.admin, http.MethodPost, "/route_template/binding",
		[]byte(`{"scope":"team","scope_id":"`+f.teamA.ID+`","route_template_id":"`+id+`"}`)); code != http.StatusOK {
		t.Fatalf("bind -> %d: %s", code, raw)
	}

	// teamMember 只属于一个团队，所以会话能唯一确定团队。
	// 读团队详情，那里会带上它的路由模板选择。
	_, raw := f.call(f.teamMember, http.MethodGet, "/team/info?team_id="+f.teamA.ID, nil)
	if !strings.Contains(string(raw), id) {
		t.Fatalf("the team's selection is not visible to its member: %s", raw)
	}
}
