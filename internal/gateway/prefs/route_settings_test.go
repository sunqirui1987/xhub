package prefs

import (
	"encoding/json"
	"testing"

	"github.com/sunqirui1987/xhub/internal/iam"
)

// fakeScopes is a lookup backed by three maps, one per scope, plus the templates
// themselves. It stands in for the identity store so these tests are about the
// resolution rules rather than about SQL.
type fakeScopes struct {
	bound     map[string]string
	templates map[string]*iam.RouteTemplate
}

func newFakeScopes() *fakeScopes {
	return &fakeScopes{bound: map[string]string{}, templates: map[string]*iam.RouteTemplate{}}
}

// bind points one scope at a template.
func (f *fakeScopes) bind(kind, id, templateID string) *fakeScopes {
	f.bound[kind+"\x00"+id] = templateID
	return f
}

// template adds a template with a document.
func (f *fakeScopes) template(id, name string, body map[string]any) *fakeScopes {
	f.templates[id] = &iam.RouteTemplate{ID: id, Name: name, Body: encodeBody(body)}
	return f
}

// delete removes a template, leaving any scope that points at it dangling.
func (f *fakeScopes) delete(id string) *fakeScopes {
	delete(f.templates, id)
	return f
}

func (f *fakeScopes) TemplateFor(kind, id string) string {
	return f.bound[kind+"\x00"+id]
}

func (f *fakeScopes) Load(id string) *iam.RouteTemplate {
	return f.templates[id]
}

// encodeBody renders a settings document the way the store holds it. It goes
// through the same JSON encoder the store does, so a test cannot pass on a body
// the real write path would never produce.
func encodeBody(body map[string]any) string {
	if body == nil {
		return "{}"
	}
	raw, err := json.Marshal(body)
	if err != nil {
		panic(err)
	}
	return string(raw)
}

// platform is the baseline every test resolves against.
func platform() map[string]any {
	return map[string]any{
		"routing_strategy": "simple-shuffle",
		"num_retries":      2,
		"timeout":          float64(60),
		"allowed_fails":    3,
		"cooldown_time":    float64(0),
	}
}

// TestNoTemplateSelectedReadsThePlatformDocument 钉住兼容线。
//
// 三处都不选时，取出来的必须就是平台那一份，来源标成 platform。这条断了
// 意味着这次改动改变了所有现有部署的行为。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestNoTemplateSelectedReadsThePlatformDocument(t *testing.T) {
	got := Resolve(platform(), newFakeScopes(),
		ScopeRef{"key", "k1"}, ScopeRef{"team", "t1"}, ScopeRef{"organization", "o1"})

	if got.Source != PlatformSource {
		t.Fatalf("source = %q, want %q", got.Source, PlatformSource)
	}
	if got.TemplateID != "" {
		t.Fatalf("a template was reported with nothing selected: %q", got.TemplateID)
	}
	// 取到的就是平台文档本身，不是空表。
	if got.Strategy() != "simple-shuffle" || got.Retries() != 2 || got.TimeoutSeconds() != 60 {
		t.Fatalf("the platform baseline did not come through: %+v", got.Settings)
	}
}

// TestTheNarrowestSelectionWins 覆盖整条链。
//
// 密钥、团队、组织都选了，生效的是密钥那一份。这不是"合并"，所以另外两层的
// 字段一个都不该出现在结果里。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestTheNarrowestSelectionWins(t *testing.T) {
	f := newFakeScopes().
		template("T-key", "key choice", map[string]any{"routing_strategy": "lowest-cost", "num_retries": 9}).
		template("T-team", "team choice", map[string]any{"routing_strategy": "least-busy", "num_retries": 5}).
		template("T-org", "org choice", map[string]any{"routing_strategy": "latency-based-routing", "num_retries": 3}).
		bind("key", "k1", "T-key").
		bind("team", "t1", "T-team").
		bind("organization", "o1", "T-org")

	got := Resolve(platform(), f,
		ScopeRef{"key", "k1"}, ScopeRef{"team", "t1"}, ScopeRef{"organization", "o1"})

	if got.TemplateID != "T-key" || got.Source != "key" {
		t.Fatalf("the key's selection did not win: id=%q source=%q", got.TemplateID, got.Source)
	}
	if got.Strategy() != "lowest-cost" || got.Retries() != 9 {
		t.Fatalf("the winning document is not the one in effect: %+v", got.Settings)
	}
	// 整份替换：团队那一份的 least-busy 不该出现在结果里。
	if got.Strategy() == "least-busy" {
		t.Fatal("a wider scope's strategy leaked into the result")
	}
}

// TestNotSelectingInheritsRatherThanFallingBack 是这次改动最容易写错的一条。
//
// 组织选了模板、团队没选 —— 团队必须用组织那一份，而不是跳回平台默认。
// 写成"没选就用默认"会让组织级的配置对整个组织都不起作用，而界面上看起来
// 明明配好了。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestNotSelectingInheritsRatherThanFallingBack(t *testing.T) {
	f := newFakeScopes().
		template("T-org", "org choice", map[string]any{"routing_strategy": "lowest-cost"}).
		bind("organization", "o1", "T-org")

	got := Resolve(platform(), f,
		ScopeRef{"key", "k1"}, ScopeRef{"team", "t1"}, ScopeRef{"organization", "o1"})

	if got.TemplateID != "T-org" {
		t.Fatalf("the team did not inherit from its organization: %q", got.TemplateID)
	}
	if got.Source != "organization" {
		t.Fatalf("source = %q, want organization", got.Source)
	}
	if got.Strategy() != "lowest-cost" {
		t.Fatalf("the inherited document is not in effect: %+v", got.Settings)
	}
}

// TestANarrowerSelectionOverridesTheWiderOne 证明团队自己选了就用团队的。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestANarrowerSelectionOverridesTheWiderOne(t *testing.T) {
	f := newFakeScopes().
		template("T-org", "org choice", map[string]any{"num_retries": 3}).
		template("T-team", "team choice", map[string]any{"num_retries": 7}).
		bind("organization", "o1", "T-org").
		bind("team", "t1", "T-team")

	got := Resolve(platform(), f,
		ScopeRef{"key", "k1"}, ScopeRef{"team", "t1"}, ScopeRef{"organization", "o1"})

	if got.TemplateID != "T-team" || got.Retries() != 7 {
		t.Fatalf("the team's own selection did not win: %+v", got)
	}
}

// TestADeletedTemplateFallsBackWithoutUsingAWiderOne 覆盖一个具体的竞态。
//
// 绑定还在、模板已经被删。这时不能顺着链去用更宽那一层的模板 —— 那等于把这次
// 请求换成另一个运维没选过的配置。退回平台默认，也就是"这一层没选"的状态。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestADeletedTemplateFallsBackWithoutUsingAWiderOne(t *testing.T) {
	f := newFakeScopes().
		template("T-org", "org choice", map[string]any{"routing_strategy": "lowest-cost"}).
		template("T-team", "team choice", map[string]any{"routing_strategy": "least-busy"}).
		bind("organization", "o1", "T-org").
		bind("team", "t1", "T-team").
		delete("T-team")

	got := Resolve(platform(), f,
		ScopeRef{"key", "k1"}, ScopeRef{"team", "t1"}, ScopeRef{"organization", "o1"})

	if got.Source != PlatformSource {
		t.Fatalf("a deleted template let a wider one take over: source=%q id=%q", got.Source, got.TemplateID)
	}
	if got.Strategy() != "simple-shuffle" {
		t.Fatalf("the platform default is not in effect: %+v", got.Settings)
	}
}

// TestAMissingScopeIsSkipped 证明链里缺的那一层不参与。
//
// 没有密钥的调用（会话）只有团队和组织两级；keyID 为空时不能去查"空的密钥"，
// 那会是一个永远查不到的 id，白白多一次查询。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestAMissingScopeIsSkipped(t *testing.T) {
	f := newFakeScopes().
		template("T-team", "team choice", map[string]any{"num_retries": 4}).
		bind("team", "t1", "T-team")

	got := Resolve(platform(), f, RequestChain("", "t1", "o1")...)
	if got.TemplateID != "T-team" {
		t.Fatalf("a session with no key did not reach its team's template: %+v", got)
	}
}

// TestRequestChainIsNarrowestFirst 钉住链的顺序。
//
// 顺序写反了就是"组织覆盖团队"，而结果是"能跑但行为不对"，最难查的一类。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestRequestChainIsNarrowestFirst(t *testing.T) {
	chain := RequestChain("k1", "t1", "o1")
	want := []string{"key", "team", "organization"}
	if len(chain) != len(want) {
		t.Fatalf("chain = %+v", chain)
	}
	for i, kind := range want {
		if chain[i].Kind != kind {
			t.Fatalf("chain order: got %+v want %v", chain, want)
		}
	}
	// 缺的那些不在链里。
	if short := RequestChain("k1", "", ""); len(short) != 1 || short[0].Kind != "key" {
		t.Fatalf("a chain with only a key came out as %+v", short)
	}
}

// TestATemplateThatOmitsAKeyMeansTheDefaultNotZero 钉住类型化读取的兜底。
//
// 模板里没写 num_retries 时不能变成 0（请求永不尝试），没写 timeout 时不能变成
// 0 秒（每个请求立刻超时）。这两个都是"能跑但全错"。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestATemplateThatOmitsAKeyMeansTheDefaultNotZero(t *testing.T) {
	empty := RouteSettings{Settings: map[string]any{}, Source: "team"}

	if empty.Retries() < 1 {
		t.Fatalf("retries = %d; a request that is never attempted cannot succeed", empty.Retries())
	}
	if empty.TimeoutSeconds() != 60 {
		t.Fatalf("timeout = %v, want the default 60", empty.TimeoutSeconds())
	}
	if empty.Strategy() != "simple-shuffle" {
		t.Fatalf("strategy = %q, want the default", empty.Strategy())
	}
	if empty.AllowedFails() != 3 {
		t.Fatalf("allowed_fails = %d, want the default 3", empty.AllowedFails())
	}
}

// TestAnExplicitZeroRetryIsClampedToOne 证明显式写 0 也不会让请求不发出。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestAnExplicitZeroRetryIsClampedToOne(t *testing.T) {
	zero := RouteSettings{Settings: map[string]any{"num_retries": float64(0)}}
	if zero.Retries() != 1 {
		t.Fatalf("retries = %d, want 1", zero.Retries())
	}
}

// TestAllowedFailsKeepsZeroMeaningOff 钉住一个反例。
//
// 和重试次数不同，allowed_fails 为 0 是**有意义的**：它表示不做冷却。
// 把它也兜底成默认值会让"关掉冷却"这个设置失效。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestAllowedFailsKeepsZeroMeaningOff(t *testing.T) {
	off := RouteSettings{Settings: map[string]any{"allowed_fails": float64(0)}}
	if off.AllowedFails() != 0 {
		t.Fatalf("allowed_fails = %d, want 0 to stay 0 so cooldown can be turned off", off.AllowedFails())
	}
}

// TestAQuotedNumberIsNotReadAsANumber 证明字符串数字不被当成数字。
//
// 设置页写的是数字，写进字符串是一次误操作。把它读成数字会让一个"0 秒超时"
// 静默生效；当成没写则是默认值，行为可预期。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestAQuotedNumberIsNotReadAsANumber(t *testing.T) {
	quoted := RouteSettings{Settings: map[string]any{"timeout": "30"}}
	if quoted.TimeoutSeconds() != 60 {
		t.Fatalf("a quoted number became %v; a string is a mistake, not a value", quoted.TimeoutSeconds())
	}
}

// TestIntegersFromYAMLAreRead 覆盖 YAML 基线里数字是 int 的情况。
//
// 内嵌目录从文件读时会给出 int，从 JSON 读时给出 float64。两条都要认。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestIntegersFromYAMLAreRead(t *testing.T) {
	fromYAML := RouteSettings{Settings: map[string]any{"num_retries": 5, "timeout": 90}}
	if fromYAML.Retries() != 5 {
		t.Fatalf("an int retry count read as %d", fromYAML.Retries())
	}
	if fromYAML.TimeoutSeconds() != 90 {
		t.Fatalf("an int timeout read as %v", fromYAML.TimeoutSeconds())
	}
}

// TestAPlatformWithNoLookupStillResolves 证明没有身份库时取默认。
//
// 一个拿不到库的进程不该因为解析不了模板就拒绝请求。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestAPlatformWithNoLookupStillResolves(t *testing.T) {
	got := Resolve(platform(), nil, ScopeRef{"team", "t1"})
	if got.Source != PlatformSource || got.Strategy() != "simple-shuffle" {
		t.Fatalf("resolution without a lookup gave %+v", got)
	}
}

// TestWeightOverridesReadsTheDocument 证明份额是这份 JSON 的一部分。
//
// 表单写成列表，手改的文件可以写成 map。两种都要落到同一个部署 id 上。
// 没写份额时必须是 nil，调用方才会不去改部署自己的 weight。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestWeightOverridesReadsTheDocument(t *testing.T) {
	listed := RouteSettings{Settings: map[string]any{
		"routing_strategy_args": map[string]any{
			"weights": []any{
				map[string]any{"api_base": "https://a", "model": "gpt-4o", "weight": 70.0},
				map[string]any{"model_name": "other", "weight": 0.0},
			},
		},
	}}
	got := listed.WeightOverrides()
	if got["https://a|gpt-4o"] != 70 || len(got) != 1 {
		t.Fatalf("list weights: %#v", got)
	}
	mapped := RouteSettings{Settings: map[string]any{
		"routing_strategy_args": map[string]any{
			"weights": map[string]any{"https://b|gpt-4o": 30.0},
		},
	}}
	if mapped.WeightOverrides()["https://b|gpt-4o"] != 30 {
		t.Fatalf("map weights: %#v", mapped.WeightOverrides())
	}
	if (RouteSettings{Settings: map[string]any{}}).WeightOverrides() != nil {
		t.Fatalf("a document without shares must not invent any")
	}
}
