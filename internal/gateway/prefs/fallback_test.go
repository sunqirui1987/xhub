package prefs

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/router"
)

// TestTemplateFallbackParsing 验证模板三类策略的映射契约、类别合并和跨类别循环。
// 前置内存模板，正常顺序保留，重复/空白/类型错误拒绝且不修改输入，无数据库清理。
func TestTemplateFallbackParsing(t *testing.T) {
	for _, field := range []string{"fallbacks", "context_window_fallbacks", "content_policy_fallbacks"} {
		for _, tc := range []struct {
			name  string
			raw   any
			valid bool
		}{
			{"empty", []any{}, true},
			{"ordered", []any{map[string]any{"a": []any{"c", "b"}}}, true},
			{"null", nil, false}, {"scalar", "a", false}, {"row-scalar", []any{"a"}, false},
			{"two-sources-in-row", []any{map[string]any{"a": []any{"b"}, "c": []any{"b"}}}, false},
			{"duplicate-source", []any{map[string]any{"a": []any{"b"}}, map[string]any{"a": []any{"c"}}}, false},
			{"space-source", []any{map[string]any{" a": []any{"b"}}}, false},
			{"empty-source", []any{map[string]any{"": []any{"b"}}}, false},
			{"null-targets", []any{map[string]any{"a": nil}}, false},
			{"duplicate-target", []any{map[string]any{"a": []any{"b", "b"}}}, false},
			{"self-cycle", []any{map[string]any{"a": []any{"a"}}}, false},
		} {
			t.Run(field+"/"+tc.name, func(t *testing.T) {
				doc := map[string]any{field: tc.raw}
				before, _ := json.Marshal(doc)
				_, policies, err := TemplateRouting(doc)
				if (err == nil) != tc.valid {
					t.Fatalf("模板%s解析契约错误: %v", field, err)
				}
				if tc.name == "ordered" {
					kind := map[string]string{"fallbacks": "general", "context_window_fallbacks": "context", "content_policy_fallbacks": "content"}[field]
					if !reflect.DeepEqual(policies["a"].Targets(kind), []string{"c", "b"}) {
						t.Fatal("模板目标顺序损坏")
					}
				}
				after, _ := json.Marshal(doc)
				if string(before) != string(after) {
					t.Fatal("模板解析修改输入")
				}
			})
		}
	}
	doc := map[string]any{}
	for _, field := range []string{"fallbacks", "context_window_fallbacks", "content_policy_fallbacks"} {
		doc[field] = []any{map[string]any{"a": []any{"b"}}}
	}
	_, policies, err := TemplateRouting(doc)
	if err != nil || !reflect.DeepEqual(policies["a"], router.FallbackPolicy{Fallbacks: []string{"b"}, ContextWindow: []string{"b"}, ContentPolicy: []string{"b"}}) {
		t.Fatalf("跨类别合并失败: %v %v", policies, err)
	}
	doc["content_policy_fallbacks"] = []any{map[string]any{"b": []any{"a"}}}
	if _, _, err := TemplateRouting(doc); err == nil {
		t.Fatal("跨类别环未拒绝")
	}
}

// TestTemplateFallbackCatalog 验证真实目录边界的源和目标存在性，支持组名并拒绝悬空引用。
// 前置纯内存模型和单成员组；每个表项独立验证，输入不变，无持久化数据或服务。
func TestTemplateFallbackCatalog(t *testing.T) {
	models := []config.ModelEntry{{ModelName: "a"}, {ModelName: "b"}}
	for _, tc := range []struct {
		name, source, target string
		valid                bool
	}{
		{"model", "a", "b", true}, {"group-source", "group", "b", true}, {"group-target", "b", "group", true},
		{"missing-source", "missing", "b", false}, {"missing-target", "a", "missing", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc := map[string]any{"routing_groups": []any{map[string]any{"group_name": "group", "models": []any{"a"}, "routing_strategy": "simple-shuffle"}}, "fallbacks": []any{map[string]any{tc.source: []any{tc.target}}}}
			if err := ValidateTemplateCatalog(doc, models); (err == nil) != tc.valid {
				t.Fatalf("模板目录%s验证结果: %v", tc.name, err)
			}
		})
	}
}
