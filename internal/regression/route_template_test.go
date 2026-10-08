package regression

import (
	"net/http"
	"strings"
	"testing"
)

// 路由模板这一段回答的问题是：一次调用到底按哪一份路由设置走，以及运维能不能
// 在界面上看出那一份是从哪来的。
//
// 本文件覆盖解析和选择；route_template_config_test.go 把完整配置的读写与
// 请求效果连起来，防止“平台设置有效，换成模板却没读到”。

// routeTemplate 建一份模板并把它的 id 返回。
//
// body 是一整份 router_settings。服务器会用平台默认补齐缺的键，所以这里可以只写
// 关心的那一项。
// 参数 t（*testing.T）：当前测试；h（*harness）：当前用例的网关；admin（string）：管理员会话；name（string）：模板名；body（map[string]any）：设置内容。
// 返回 string（string）：模板 id。
func routeTemplate(t *testing.T, h *harness, admin, name string, body map[string]any) string {
	t.Helper()
	r := h.ok(http.MethodPost, "/route_template/new", admin, map[string]any{"name": name, "body": body})
	id := firstString(r.json(), "id")
	if id == "" {
		t.Fatalf("route_template/new returned no id: %s", r.describe())
	}
	return id
}

// bindTemplate 把一份模板挂到一个范围上。
// 参数 t（*testing.T）：当前测试；h（*harness）：当前用例的网关；admin（string）：管理员会话；
// scope/scopeID（string）：范围名和范围 id；templateID（string）：模板 id，空串表示清除选择。
// 返回：无。
func bindTemplate(t *testing.T, h *harness, admin, scope, scopeID, templateID string) {
	t.Helper()
	h.ok(http.MethodPost, "/route_template/binding", admin, map[string]any{
		"scope": scope, "scope_id": scopeID, "route_template_id": templateID,
	})
}

// effectiveTemplate 读一个范围当前生效的模板和它的来源。
// 参数 t（*testing.T）：当前测试；h（*harness）：当前用例的网关；admin（string）：管理员会话；scope/scopeID（string）：范围。
// 返回 map[string]any（map[string]any）：effective 字段。
func effectiveTemplate(t *testing.T, h *harness, admin, scope, scopeID string) map[string]any {
	t.Helper()
	r := h.ok(http.MethodGet,
		"/route_template/binding?scope="+scope+"&scope_id="+scopeID, admin, nil)
	effective, _ := r.json()["effective"].(map[string]any)
	if effective == nil {
		t.Fatalf("the binding reply carried no effective resolution: %s", r.describe())
	}
	return effective
}

// TestNoTemplateSelectedBehavesLikeThePlatformDefault 钉住兼容线。
//
// 这是整次改动最重要的一条：不选任何模板时，行为和这个功能存在之前一模一样。
// 选一份只改重试次数的模板，断言重试次数真的变了；不绑定时断言它回到平台值。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestNoTemplateSelectedBehavesLikeThePlatformDefault(t *testing.T) {
	h := newHarness(t)
	admin := h.adminSession()
	tn := h.provision(t, admin, "tmpl-default")
	h.flushSpend()

	addFlatPricedModel(t, h, admin, "regression-tmpl", map[string]any{
		"input_cost_per_token":  testInputRate,
		"output_cost_per_token": testOutputRate,
	}, "chat")

	// 平台默认改成一次重试，并把上游打成 500，于是"试几次"能被数出来。
	h.setRouter(admin, map[string]any{"num_retries": 1})
	h.scriptStatus("regression-tmpl", http.StatusInternalServerError)

	h.resetUpstream()
	h.do(http.MethodPost, "/v1/chat/completions", tn.key, chatRequest("regression-tmpl", "once"))
	if got := h.upstreamAttempts("regression-tmpl"); got != 1 {
		t.Fatalf("the platform default did not apply: %d attempts, want 1", got)
	}

	// 组织选一份三次重试的模板，同一个团队再试。
	tmpl := routeTemplate(t, h, admin, "three tries", map[string]any{"num_retries": 3})
	bindTemplate(t, h, admin, "organization", tn.orgID, tmpl)

	h.resetUpstream()
	h.do(http.MethodPost, "/v1/chat/completions", tn.key, chatRequest("regression-tmpl", "thrice"))
	if got := h.upstreamAttempts("regression-tmpl"); got != 3 {
		t.Fatalf("the organization's template did not apply: %d attempts, want 3", got)
	}

	// 清除选择之后回到平台默认。
	bindTemplate(t, h, admin, "organization", tn.orgID, "")
	h.resetUpstream()
	h.do(http.MethodPost, "/v1/chat/completions", tn.key, chatRequest("regression-tmpl", "once again"))
	if got := h.upstreamAttempts("regression-tmpl"); got != 1 {
		t.Fatalf("clearing the selection did not restore the platform default: %d attempts, want 1", got)
	}
}

// TestATeamsOwnTemplateBeatsItsOrganizations 证明最窄的那一层赢。
//
// 组织选了 A、团队选了 B，团队用 B —— 而且**只用 B**，不是两份拼起来。
// 拼接没有可解释的语义，而且组织改一次就会连带改掉团队的行为。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestATeamsOwnTemplateBeatsItsOrganizations(t *testing.T) {
	h := newHarness(t)
	admin := h.adminSession()
	tn := h.provision(t, admin, "tmpl-narrow")
	h.flushSpend()

	addFlatPricedModel(t, h, admin, "regression-narrow", map[string]any{
		"input_cost_per_token":  testInputRate,
		"output_cost_per_token": testOutputRate,
	}, "chat")
	h.scriptStatus("regression-narrow", http.StatusInternalServerError)

	orgTmpl := routeTemplate(t, h, admin, "org tries 4", map[string]any{"num_retries": 4})
	teamTmpl := routeTemplate(t, h, admin, "team tries 2", map[string]any{"num_retries": 2})
	bindTemplate(t, h, admin, "organization", tn.orgID, orgTmpl)

	// 团队没选时继承组织：4 次。
	h.resetUpstream()
	h.do(http.MethodPost, "/v1/chat/completions", tn.key, chatRequest("regression-narrow", "inherit"))
	if got := h.upstreamAttempts("regression-narrow"); got != 4 {
		t.Fatalf("the team did not inherit its organization's template: %d attempts, want 4", got)
	}

	// 团队自己选了：2 次，且不是 4 和 2 的某种组合。
	bindTemplate(t, h, admin, "team", tn.teamID, teamTmpl)
	h.resetUpstream()
	h.do(http.MethodPost, "/v1/chat/completions", tn.key, chatRequest("regression-narrow", "own"))
	if got := h.upstreamAttempts("regression-narrow"); got != 2 {
		t.Fatalf("the team's own template did not take over: %d attempts, want 2", got)
	}

	// 组织改成别的，团队那一份不受影响。
	other := routeTemplate(t, h, admin, "org tries 7", map[string]any{"num_retries": 7})
	bindTemplate(t, h, admin, "organization", tn.orgID, other)
	h.resetUpstream()
	h.do(http.MethodPost, "/v1/chat/completions", tn.key, chatRequest("regression-narrow", "own again"))
	if got := h.upstreamAttempts("regression-narrow"); got != 2 {
		t.Fatalf("editing the organization's template changed the team's behaviour: %d attempts, want 2", got)
	}
}

// TestASessionPicksUpItsTeamsTemplate 覆盖一个只影响一个入口的缺口。
//
// 预算链只对有密钥行的调用方走。控制台的演练场是**会话**调用，如果只在那条链上
// 解析模板，团队选了模板、演练场里的请求却仍然走平台默认——配上没生效，而且只有
// 那一个入口不对。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestASessionPicksUpItsTeamsTemplate(t *testing.T) {
	h := newHarness(t)
	admin := h.adminSession()
	tn := h.provision(t, admin, "tmpl-session")
	h.flushSpend()

	addFlatPricedModel(t, h, admin, "regression-session", map[string]any{
		"input_cost_per_token":  testInputRate,
		"output_cost_per_token": testOutputRate,
	}, "chat")
	h.scriptStatus("regression-session", http.StatusInternalServerError)
	h.setRouter(admin, map[string]any{"num_retries": 1})

	tmpl := routeTemplate(t, h, admin, "session tries 3", map[string]any{"num_retries": 3})
	bindTemplate(t, h, admin, "team", tn.teamID, tmpl)

	// 用会话（不是密钥）发一次推理请求。会话的主人只属于这一个团队，所以能唯一
	// 确定团队，模板必须生效。
	h.resetUpstream()
	h.do(http.MethodPost, "/v1/chat/completions", tn.session, chatRequest("regression-session", "from the console"))
	if got := h.upstreamAttempts("regression-session"); got != 3 {
		t.Fatalf("a console session did not use its team's template: %d attempts, want 3", got)
	}
}

// TestTheConsoleSaysWhichTemplateIsInEffect 证明接口说清了生效的是哪一份、来自哪一层。
//
// 一个没选模板的团队和选了模板的组织，路由行为一样但配置不一样。界面不显示来源
// 的话，运维看到的就是"我没选，但它行为变了"。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestTheConsoleSaysWhichTemplateIsInEffect(t *testing.T) {
	h := newHarness(t)
	admin := h.adminSession()
	tn := h.provision(t, admin, "tmpl-reports")

	// 都没选：生效的是平台默认，来源是 platform。
	got := effectiveTemplate(t, h, admin, "team", tn.teamID)
	if got["scope_type"] != "platform" {
		t.Fatalf("with nothing selected the source is %v, want platform", got["scope_type"])
	}

	tmpl := routeTemplate(t, h, admin, "reported", map[string]any{"num_retries": 6})
	bindTemplate(t, h, admin, "organization", tn.orgID, tmpl)

	// 团队自己没选，生效的应该是组织那一份，并且**说得出**来自组织。
	got = effectiveTemplate(t, h, admin, "team", tn.teamID)
	if got["template_id"] != tmpl {
		t.Fatalf("the team's effective template is %v, want the organization's %s", got["template_id"], tmpl)
	}
	if got["scope_type"] != "organization" {
		t.Fatalf("the source is %v, want organization", got["scope_type"])
	}
}

// TestDeletingATemplateInUseIsRefusedWithTheList 证明删除的拒绝带着使用者。
//
// 静默解开会让那些范围退回默认，而"我删了一个没人用的模板"和"我改了三个团队的
// 行为"是完全不同的后果。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestDeletingATemplateInUseIsRefusedWithTheList(t *testing.T) {
	h := newHarness(t)
	admin := h.adminSession()
	tn := h.provision(t, admin, "tmpl-delete")

	tmpl := routeTemplate(t, h, admin, "in use", map[string]any{"num_retries": 5})
	bindTemplate(t, h, admin, "team", tn.teamID, tmpl)

	r := h.do(http.MethodPost, "/route_template/"+tmpl+"/delete", admin, map[string]any{})
	if r.status < 400 {
		t.Fatalf("a template still in use was deleted: %s", r.describe())
	}
	// 拒绝的正文要能看出是谁在用。
	if !containsAny(r.body, tn.teamID) {
		t.Fatalf("the refusal did not name the scope holding it: %s", r.describe())
	}

	// 解开之后删得掉。
	bindTemplate(t, h, admin, "team", tn.teamID, "")
	h.ok(http.MethodPost, "/route_template/"+tmpl+"/delete", admin, map[string]any{})
}

// containsAny 报告正文里有没有出现某个片段。用它而不是解析结构，是因为这条断言
// 关心的是"运维能不能从响应里读出是谁"，而不是确切的 JSON 形状。
// 参数 body（[]byte）：响应正文；needles（...string）：要找的片段。
// 返回 bool（bool）：任何一个片段出现时为真。
func containsAny(body []byte, needles ...string) bool {
	text := string(body)
	for _, needle := range needles {
		if needle != "" && strings.Contains(text, needle) {
			return true
		}
	}
	return false
}

// upstreamAttempts 数这一轮里上游被拨了几次。
//
// 重试次数没法从响应里读出来：失败的那一次最后是 502，看不出中间试了几遍。
// 所以数的是假供应商收到的请求数——那正是"每条部署尝试几次"的字面含义。
// 调用方负责在发请求之前先 resetUpstream，这样数出来的只是这一次。
// 参数 model（string）：上游模型名，不含供应商前缀。
// 返回 int（int）：这段时间里发往这个模型的请求数。
func (h *harness) upstreamAttempts(model string) int {
	n := 0
	for _, call := range h.upstreamCalls() {
		if got, _ := call.Body["model"].(string); got == model {
			n++
		}
	}
	return n
}

// TestAKeyCreatedWithATemplateKeepsIt 覆盖一个只影响创建路径的缺口。
//
// 选择曾经只在 /key/update 上被读。/key/generate 明明收到了这个字段却丢掉它，
// 于是"建一把带模板的密钥"看起来成功了，实际那把密钥走的是团队的设置。界面上
// 没有任何地方显示出这个差别，只有账单不一样。
//
// 断言的是**行为**而不是字段：创建时把密钥指向一份 3 次重试的模板，看上游被拨
// 几次。字段存对了但请求路径不读，是同一个 bug 的另一种写法。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestAKeyCreatedWithATemplateKeepsIt(t *testing.T) {
	h := newHarness(t)
	admin := h.adminSession()
	tn := h.provision(t, admin, "tmpl-key-create")
	h.flushSpend()

	addFlatPricedModel(t, h, admin, "regression-keytmpl", map[string]any{
		"input_cost_per_token":  testInputRate,
		"output_cost_per_token": testOutputRate,
	}, "chat")
	h.scriptStatus("regression-keytmpl", http.StatusInternalServerError)
	h.setRouter(admin, map[string]any{"num_retries": 1})

	tmpl := routeTemplate(t, h, admin, "key tries 4", map[string]any{"num_retries": 4})

	// 建密钥时就把模板写上。
	key := h.keyWith(t, tn.session, map[string]any{
		"key_alias": "with-template", "team_id": tn.teamID, "route_template_id": tmpl,
	})

	h.resetUpstream()
	h.do(http.MethodPost, "/v1/chat/completions", key, chatRequest("regression-keytmpl", "created with it"))
	if got := h.upstreamAttempts("regression-keytmpl"); got != 4 {
		t.Fatalf("a key created with a template retried %d times, want 4; the field was not read on create", got)
	}

	// 界面上也要看得见。读不到的话，密钥的编辑页显示成"没选"，运维会以为自己
	// 设过的东西丢了，然后顺手覆盖掉它。
	info := h.ok(http.MethodGet, "/key/info?key="+key, admin, nil)
	if !containsAny(info.body, tmpl) {
		t.Fatalf("the key read does not carry its template selection: %s", info.describe())
	}
}
