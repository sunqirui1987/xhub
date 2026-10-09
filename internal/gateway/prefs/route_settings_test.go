package prefs

import (
	"encoding/json"
	"testing"

	"github.com/sunqirui1987/xhub/internal/iam"
)

// fakeScopes 用内存映射模拟范围绑定和模板存储，使解析测试不依赖数据库。
type fakeScopes struct {
	bound     map[string]string
	templates map[string]*iam.RouteTemplate
}

// newFakeScopes 返回空的范围存储；调用方可继续绑定模板，无外部副作用。
func newFakeScopes() *fakeScopes {
	return &fakeScopes{bound: map[string]string{}, templates: map[string]*iam.RouteTemplate{}}
}

// bind 将一个身份范围绑定到模板并返回自身，供测试链式构造。
func (f *fakeScopes) bind(kind, id, templateID string) *fakeScopes {
	f.bound[kind+"\x00"+id] = templateID
	return f
}

// template 添加一份经过 JSON 编码的模板，模拟真实存储格式。
func (f *fakeScopes) template(id, name string, body map[string]any) *fakeScopes {
	raw, err := json.Marshal(body)
	if err != nil {
		panic(err)
	}
	f.templates[id] = &iam.RouteTemplate{ID: id, Name: name, Body: string(raw)}
	return f
}

// delete 删除模板但保留绑定，用来模拟并发删除后的悬空引用。
func (f *fakeScopes) delete(id string) *fakeScopes { delete(f.templates, id); return f }

// templateDocument 创建一份完整的新模板文档；模型规则为空时只配置执行参数。
func templateDocument(attempts int) map[string]any {
	return map[string]any{
		"model_routes": []any{},
		"retry_policy": map[string]any{"max_attempts": attempts, "timeout_seconds": 60, "failure_threshold": 3, "cooldown_seconds": 0},
	}
}

// TemplateFor 返回范围当前选择的模板 ID；未绑定返回空串。
func (f *fakeScopes) TemplateFor(kind, id string) string { return f.bound[kind+"\x00"+id] }

// Load 返回模板实体；模板不存在时返回 nil。
func (f *fakeScopes) Load(id string) *iam.RouteTemplate { return f.templates[id] }

// TestNoTemplateUsesBuiltinModelDefault 验证没有任何绑定时直接使用模型管理默认分配。
// 测试不读取平台 router_settings，也不访问数据库，无需清理。
func TestNoTemplateUsesBuiltinModelDefault(t *testing.T) {
	got := Resolve(newFakeScopes(), ScopeRef{"key", "k1"}, ScopeRef{"team", "t1"}, ScopeRef{"organization", "o1"})
	if got.Source != BuiltinSource || got.TemplateID != "" || got.Strategy() != "traffic-split" || got.Retries() != 1 || got.TimeoutSeconds() != 60 {
		t.Fatalf("未绑定模板没有使用内置模型默认: %+v", got)
	}
}

// TestNarrowestTemplateSelectionWins 验证密钥、团队、组织均绑定时选择最窄范围的完整模板。
// 模板之间不合并；测试使用内存数据，无需清理。
func TestNarrowestTemplateSelectionWins(t *testing.T) {
	f := newFakeScopes().
		template("key-template", "key", templateDocument(9)).
		template("team-template", "team", templateDocument(5)).
		template("org-template", "org", templateDocument(3)).
		bind("key", "k1", "key-template").bind("team", "t1", "team-template").bind("organization", "o1", "org-template")
	got := Resolve(f, ScopeRef{"key", "k1"}, ScopeRef{"team", "t1"}, ScopeRef{"organization", "o1"})
	if got.TemplateID != "key-template" || got.Source != "key" || got.Retries() != 9 {
		t.Fatalf("最窄范围没有生效: %+v", got)
	}
}

// TestUnselectedNarrowScopesInheritWiderTemplate 验证密钥和团队未绑定时会使用组织绑定模板。
// 这是范围选择继承，不是模型规则合并；测试仅使用内存数据。
func TestUnselectedNarrowScopesInheritWiderTemplate(t *testing.T) {
	f := newFakeScopes().template("org-template", "org", templateDocument(4)).bind("organization", "o1", "org-template")
	got := Resolve(f, ScopeRef{"key", "k1"}, ScopeRef{"team", "t1"}, ScopeRef{"organization", "o1"})
	if got.TemplateID != "org-template" || got.Source != "organization" || got.Retries() != 4 {
		t.Fatalf("组织模板没有被继承: %+v", got)
	}
}

// TestDanglingSelectionFallsBackToModelDefault 验证已选择模板被删除后不静默改用更宽范围模板。
// 解析应回到模型管理默认分配；测试构造悬空引用后无需额外清理。
func TestDanglingSelectionFallsBackToModelDefault(t *testing.T) {
	f := newFakeScopes().template("team-template", "team", templateDocument(2)).template("org-template", "org", templateDocument(8)).bind("team", "t1", "team-template").bind("organization", "o1", "org-template").delete("team-template")
	got := Resolve(f, ScopeRef{"team", "t1"}, ScopeRef{"organization", "o1"})
	if got.Source != BuiltinSource || got.TemplateID != "" || got.Strategy() != "traffic-split" {
		t.Fatalf("悬空绑定没有回到模型默认: %+v", got)
	}
}

// TestRequestChainIsNarrowestFirst 验证请求范围顺序固定为密钥、团队、组织，并跳过空 ID。
// 测试为纯函数断言，无需清理。
func TestRequestChainIsNarrowestFirst(t *testing.T) {
	chain := RequestChain("k1", "t1", "o1")
	want := []string{"key", "team", "organization"}
	if len(chain) != len(want) {
		t.Fatalf("范围数量错误: %+v", chain)
	}
	for i, kind := range want {
		if chain[i].Kind != kind {
			t.Fatalf("范围顺序错误: %+v", chain)
		}
	}
	if short := RequestChain("k1", "", ""); len(short) != 1 || short[0].Kind != "key" {
		t.Fatalf("空范围未被跳过: %+v", short)
	}
}

// TestTypedExecutionDefaultsAndBoundaries 验证公共执行参数的默认值、零值和错误类型处理。
// 这些读取器不得让请求变成零次尝试或零秒超时；纯内存测试无需清理。
func TestTypedExecutionDefaultsAndBoundaries(t *testing.T) {
	empty := RouteSettings{Settings: map[string]any{}}
	if empty.Retries() != 1 || empty.TimeoutSeconds() != 60 || empty.AllowedFails() != 3 || empty.Strategy() != "traffic-split" {
		t.Fatalf("缺省执行参数错误: %+v", empty)
	}
	zero := RouteSettings{Settings: map[string]any{"retry_policy": map[string]any{"max_attempts": 0.0, "timeout_seconds": 60, "failure_threshold": 0.0, "cooldown_seconds": 0}}}
	if zero.Retries() != 1 || zero.AllowedFails() != 0 {
		t.Fatalf("零值边界错误: %+v", zero)
	}
	quoted := RouteSettings{Settings: map[string]any{"retry_policy": map[string]any{"max_attempts": 1, "timeout_seconds": "30", "failure_threshold": 3, "cooldown_seconds": 0}}}
	if quoted.TimeoutSeconds() != 60 {
		t.Fatalf("字符串数字被错误接受: %v", quoted.TimeoutSeconds())
	}
	fromYAML := RouteSettings{Settings: templateDocument(5)}
	fromYAML.Settings["retry_policy"].(map[string]any)["timeout_seconds"] = 90
	if fromYAML.Retries() != 5 || fromYAML.TimeoutSeconds() != 90 {
		t.Fatalf("整数读取错误: %+v", fromYAML)
	}
}

// TestResolveWithoutLookupUsesModelDefault 验证身份库不可用时仍可使用模型管理默认分配。
// nil lookup 不应导致请求失败；测试无外部状态。
func TestResolveWithoutLookupUsesModelDefault(t *testing.T) {
	got := Resolve(nil, ScopeRef{"team", "t1"})
	if got.Source != BuiltinSource || got.Strategy() != "traffic-split" {
		t.Fatalf("无 lookup 时解析错误: %+v", got)
	}
}
