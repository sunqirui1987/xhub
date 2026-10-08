package iam

import (
	"context"
	"errors"
	"testing"
)

// actor is the operator the store writes into the audit row. These tests are
// about the template rows, so one actor is enough.
func actor() Actor { return Actor{ID: "admin-1", Kind: "session"} }

// TestTemplateBodyRoundTripsAsRouterSettings 覆盖模板正文的读写。
//
// 正文是一整份 router_settings 文档，不拆成一堆列。所以最重要的是它原样往返：
// 存进去什么，读出来还是什么——中间任何一处丢字段，运维在模板里配的东西就静默
// 不见了。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestTemplateBodyRoundTripsAsRouterSettings(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	body := `{"routing_strategy":"lowest-cost","num_retries":5,"timeout":90,
	          "fallbacks":[{"gpt-4o":["claude-sonnet-4"]}],"max_fallbacks":2}`
	created, err := db.CreateRouteTemplate(ctx, actor(), TemplateOwner{}, "cheap first", body)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	read, err := db.GetRouteTemplate(ctx, created.ID)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	settings := read.Settings()
	if settings["routing_strategy"] != "lowest-cost" {
		t.Fatalf("strategy: %#v", settings["routing_strategy"])
	}
	if settings["num_retries"] != float64(5) {
		t.Fatalf("retries: %#v", settings["num_retries"])
	}
	// 回退链是这一版要真的执行的东西，字段不能在往返里被吃掉。
	chain, ok := settings["fallbacks"].([]any)
	if !ok || len(chain) != 1 {
		t.Fatalf("fallbacks did not survive the round trip: %#v", settings["fallbacks"])
	}
}

// TestTemplateWithAnUnreadableBodyFallsBackToEmpty 证明坏正文不会让请求失败。
//
// 一份解不出来的模板如果让继承它的每个请求都报错，一次误写就是一次全站故障。
// 退回空文档等于"这份模板什么都没说"，那是能继续跑的那一侧。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestTemplateWithAnUnreadableBodyFallsBackToEmpty(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	created, err := db.CreateRouteTemplate(ctx, actor(), TemplateOwner{}, "broken", `{"routing_strategy":`)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	read, err := db.GetRouteTemplate(ctx, created.ID)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got := read.Settings(); len(got) != 0 {
		t.Fatalf("a truncated body produced settings: %#v", got)
	}
}

// TestTemplateNamesAreUnique 证明同名模板建不出来。
//
// 三个范围的界面上是按名字选模板的。允许重名就是允许两个不同的东西叫同一个名字，
// 而运维没法从中选对。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestTemplateNamesAreUnique(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	if _, err := db.CreateRouteTemplate(ctx, actor(), TemplateOwner{}, "same", `{}`); err != nil {
		t.Fatalf("first create: %v", err)
	}
	_, err := db.CreateRouteTemplate(ctx, actor(), TemplateOwner{}, "same", `{}`)
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("a duplicate name gave %v, want a conflict", err)
	}
}

// TestRenameKeepsTheTemplateUsable 证明改名不新建一份、也不丢正文。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestRenameKeepsTheTemplateUsable(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	created, err := db.CreateRouteTemplate(ctx, actor(), TemplateOwner{}, "before", `{"num_retries":7}`)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	updated, err := db.UpdateRouteTemplate(ctx, actor(), created.ID, "after", `{"num_retries":7}`)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Name != "after" {
		t.Fatalf("name: %q", updated.Name)
	}
	read, err := db.GetRouteTemplate(ctx, created.ID)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if read.Settings()["num_retries"] != float64(7) {
		t.Fatalf("the body changed during a rename: %#v", read.Settings())
	}
}

// TestScopeSelectionRoundTrips 覆盖三个范围的选择。
//
// 空值表示"不应用"，这是继承的开关。它和"选了平台默认"必须是同一件事，
// 所以清除选择读回来就是空串。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestScopeSelectionRoundTrips(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	tmpl, err := db.CreateRouteTemplate(ctx, actor(), TemplateOwner{}, "picked", `{"num_retries":4}`)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	org, err := db.CreateOrg(ctx, actor(), "acme", nil)
	if err != nil {
		t.Fatalf("create org: %v", err)
	}

	if err := db.SetScopeRouteTemplate(ctx, actor(), "organization", org.ID, tmpl.ID); err != nil {
		t.Fatalf("bind: %v", err)
	}
	got, err := db.ScopeRouteTemplate(ctx, "organization", org.ID)
	if err != nil {
		t.Fatalf("read binding: %v", err)
	}
	if got != tmpl.ID {
		t.Fatalf("binding: got %q want %q", got, tmpl.ID)
	}

	// 清除之后读回来是空的，也就是回到继承。
	if err := db.SetScopeRouteTemplate(ctx, actor(), "organization", org.ID, ""); err != nil {
		t.Fatalf("clear: %v", err)
	}
	got, err = db.ScopeRouteTemplate(ctx, "organization", org.ID)
	if err != nil {
		t.Fatalf("read after clear: %v", err)
	}
	if got != "" {
		t.Fatalf("a cleared binding still reads %q", got)
	}
}

// TestBindingRefusesAnUnknownTemplate 证明不能把范围指向一个不存在的模板。
//
// 指向空处会让那次调用取不到设置，而"取不到"和"用平台默认"看起来一样，
// 于是错误会安静地降级。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestBindingRefusesAnUnknownTemplate(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	org, err := db.CreateOrg(ctx, actor(), "acme", nil)
	if err != nil {
		t.Fatalf("create org: %v", err)
	}
	err = db.SetScopeRouteTemplate(ctx, actor(), "organization", org.ID, "no-such-template")
	if !errors.Is(err, ErrRouteTemplateMissing) {
		t.Fatalf("binding to a missing template gave %v", err)
	}
}

// TestBindingRefusesAnUnknownScopeName 证明范围名是从白名单里取的。
//
// 范围名会被拼进 SQL 的表名位置，所以它必须来自代码里的常量，不能来自请求体。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestBindingRefusesAnUnknownScopeName(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	// 一个看起来像表名、但不是我们承认的范围的输入。
	err := db.SetScopeRouteTemplate(ctx, actor(), "organizations; DROP TABLE teams", "x", "")
	if !errors.Is(err, ErrUnknownScope) {
		t.Fatalf("an unknown scope name gave %v, want a refusal", err)
	}
}

// TestUsageListsEveryScopeThatSelectsTheTemplate 是删除前那道检查的依据。
//
// 删一份还有人用的模板会让那些范围悄悄退回默认——那是"我改了一个没人用的东西"
// 和"我改了三个团队的行为"的区别，所以拒绝时必须能说清是谁在用。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestUsageListsEveryScopeThatSelectsTheTemplate(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	tmpl, err := db.CreateRouteTemplate(ctx, actor(), TemplateOwner{}, "in use", `{}`)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	org, err := db.CreateOrg(ctx, actor(), "acme", nil)
	if err != nil {
		t.Fatalf("create org: %v", err)
	}
	// 建团队要一个在册的管理员，所以先建一个人。
	admin, err := db.CreateUser(ctx, actor(), UserInput{
		Email: "route-owner@example.com", Name: "owner", Password: "initial-password",
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	team, err := db.CreateTeam(ctx, actor(), TeamInput{
		OrganizationID: org.ID, Name: "core", AdminUserID: admin.ID,
	})
	if err != nil {
		t.Fatalf("create team: %v", err)
	}
	for _, scope := range []struct{ name, id string }{
		{"organization", org.ID},
		{"team", team.ID},
	} {
		if err := db.SetScopeRouteTemplate(ctx, actor(), scope.name, scope.id, tmpl.ID); err != nil {
			t.Fatalf("bind %s: %v", scope.name, err)
		}
	}

	usage, err := db.RouteTemplateUsage(ctx, tmpl.ID)
	if err != nil {
		t.Fatalf("usage: %v", err)
	}
	seen := map[string]string{}
	for _, row := range usage {
		seen[row.ScopeType] = row.Name
	}
	if seen["organization"] != "acme" {
		t.Fatalf("the organization using it was not listed: %#v", usage)
	}
	if seen["team"] != "core" {
		t.Fatalf("the team using it was not listed: %#v", usage)
	}

	counts, err := db.RouteTemplateUsageCounts(ctx)
	if err != nil {
		t.Fatalf("counts: %v", err)
	}
	if counts[tmpl.ID] != 2 {
		t.Fatalf("usage count: got %d want 2", counts[tmpl.ID])
	}
}

// TestDeletingATemplateInUseIsRefusedByTheCaller 证明删除的检查点在哪。
//
// 存储层不认识"用户能看到的话"，所以它照删不误；gateway 先读使用清单再决定。
// 这条用例钉住这个分工：用完的模板能删，还在用的读得到使用者。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestDeletingATemplateInUseIsRefusedByTheCaller(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	tmpl, err := db.CreateRouteTemplate(ctx, actor(), TemplateOwner{}, "doomed", `{}`)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	org, err := db.CreateOrg(ctx, actor(), "acme", nil)
	if err != nil {
		t.Fatalf("create org: %v", err)
	}
	if err := db.SetScopeRouteTemplate(ctx, actor(), "organization", org.ID, tmpl.ID); err != nil {
		t.Fatalf("bind: %v", err)
	}

	// 还在用的时候，gateway 看到的使用清单非空，于是拒绝。清理掉之后为空。
	if usage, _ := db.RouteTemplateUsage(ctx, tmpl.ID); len(usage) == 0 {
		t.Fatal("a template in use reported no users, so the delete check would not fire")
	}
	if err := db.SetScopeRouteTemplate(ctx, actor(), "organization", org.ID, ""); err != nil {
		t.Fatalf("unbind: %v", err)
	}
	if usage, _ := db.RouteTemplateUsage(ctx, tmpl.ID); len(usage) != 0 {
		t.Fatalf("an unused template still reported users: %#v", usage)
	}
	if err := db.DeleteRouteTemplate(ctx, actor(), tmpl.ID); err != nil {
		t.Fatalf("delete an unused template: %v", err)
	}
	read, err := db.GetRouteTemplate(ctx, tmpl.ID)
	if err != nil {
		t.Fatalf("read after delete: %v", err)
	}
	if read != nil {
		t.Fatal("the template survived its own delete")
	}
}

// TestDeletingAMissingTemplateReportsIt 证明删一个不存在的东西有明确答复。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestDeletingAMissingTemplateReportsIt(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	err := db.DeleteRouteTemplate(ctx, actor(), "no-such-template")
	if !errors.Is(err, ErrRouteTemplateMissing) {
		t.Fatalf("deleting a missing template gave %v", err)
	}
}

// TestListRouteTemplatesIsOrderedByName 证明列表顺序稳定。
//
// 三个范围的下拉框直接用这个列表。顺序随查询变会让同一个选项每次刷新都在不同
// 位置，选起来只能靠读字。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestListRouteTemplatesIsOrderedByName(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	for _, name := range []string{"zeta", "alpha", "mid"} {
		if _, err := db.CreateRouteTemplate(ctx, actor(), TemplateOwner{}, name, `{}`); err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
	}
	rows, err := db.ListRouteTemplates(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	var names []string
	for _, row := range rows {
		names = append(names, row.Name)
	}
	want := []string{"alpha", "mid", "zeta"}
	if len(names) != len(want) {
		t.Fatalf("listed %v", names)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("order: got %v want %v", names, want)
		}
	}
}

// TestADanglingTemplateIdIsRejectedByTheDatabase 证明外键是真的。
//
// 应用层的绑定会先查模板存不存在，所以正常路径上写不进坏值。但外键是兜底：
// 一个悬空的 id 会让那个范围看起来选了模板、而查找什么也找不到，于是它安静地
// 退回平台默认——行为看起来正常，配置看起来也正常，只有账单不一样。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestADanglingTemplateIdIsRejectedByTheDatabase(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	org, err := db.CreateOrg(ctx, actor(), "acme", nil)
	if err != nil {
		t.Fatalf("create org: %v", err)
	}
	// 绕过绑定接口直接写坏值，这是唯一能试出外键的方式。
	_, err = db.Engine.Context(ctx).Exec(
		"UPDATE organizations SET route_template_id = 'no-such-row' WHERE id = ?", org.ID)
	if err == nil {
		t.Fatal("a dangling template id was accepted; the foreign key is missing")
	}
}
