package router

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/sunqirui1987/xhub/internal/config"
)

// fallbackConfigReader 为编译单测提供确定性配置与指定存储错误；只读内存，不需要资源清理。
type fallbackConfigReader struct {
	rows map[string]map[string]any
	fail string
}

// ListConfig 接收配置类别，返回共享只读原始行或故障；由Compile调用，测试检查调用后输入不变。
func (r fallbackConfigReader) ListConfig(kind string) (map[string]any, error) {
	if r.fail == kind {
		return nil, errors.New("storage unavailable: " + kind)
	}
	return r.rows[kind], nil
}

// TestCompileFallbackPrecedence 验证模板逐模型逐类别覆盖、显式清空与继承。
// 前置内存默认策略；断言合并结果、请求快照隔离和原始文档不变，无外部数据清理。
func TestCompileFallbackPrecedence(t *testing.T) {
	models := []config.ModelEntry{{ModelName: "a"}, {ModelName: "b"}, {ModelName: "c"}}
	defaults := map[string]any{"a": FallbackPolicy{Fallbacks: []string{"b"}, ContextWindow: []string{"c"}, ContentPolicy: []string{"b"}}, "b": FallbackPolicy{Fallbacks: []string{"c"}}}
	store := fallbackConfigReader{rows: map[string]map[string]any{"model_fallbacks": defaults}}
	for _, tc := range []struct {
		name  string
		local map[string]any
		want  FallbackPolicy
	}{
		{"inherit", map[string]any{}, FallbackPolicy{Fallbacks: []string{"b"}, ContextWindow: []string{"c"}, ContentPolicy: []string{"b"}}},
		{"empty-table-inherits", map[string]any{"fallbacks": []any{}}, FallbackPolicy{Fallbacks: []string{"b"}, ContextWindow: []string{"c"}, ContentPolicy: []string{"b"}}},
		{"override-general", map[string]any{"fallbacks": []any{map[string]any{"a": []any{"c"}}}}, FallbackPolicy{Fallbacks: []string{"c"}, ContextWindow: []string{"c"}, ContentPolicy: []string{"b"}}},
		{"clear-context", map[string]any{"context_window_fallbacks": []any{map[string]any{"a": []any{}}}}, FallbackPolicy{Fallbacks: []string{"b"}, ContextWindow: []string{}, ContentPolicy: []string{"b"}}},
		{"override-content", map[string]any{"content_policy_fallbacks": []any{map[string]any{"a": []any{"c"}}}}, FallbackPolicy{Fallbacks: []string{"b"}, ContextWindow: []string{"c"}, ContentPolicy: []string{"c"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc := BuiltinDocument()
			for key, value := range tc.local {
				doc[key] = value
			}
			before, _ := json.Marshal([]any{doc, defaults})
			got := Compile(RouteSettings{Settings: doc}, store, models)
			if got.Err != nil || !reflect.DeepEqual(got.ModelFallbacks["a"], tc.want) {
				t.Fatalf("模板覆盖错误: %+v %v", got.ModelFallbacks, got.Err)
			}
			if !reflect.DeepEqual(got.ModelFallbacks["b"], defaults["b"]) {
				t.Fatal("覆盖a影响了b")
			}
			after, _ := json.Marshal([]any{doc, defaults})
			if string(before) != string(after) {
				t.Fatal("编译修改输入")
			}
			got.ModelFallbacks["b"].Fallbacks[0] = "mutated"
			next := Compile(RouteSettings{Settings: doc}, store, models)
			if next.Err != nil || next.ModelFallbacks["b"].Fallbacks[0] != "c" {
				t.Fatal("请求快照污染后续请求")
			}
		})
	}
}

// TestCompileFallbackFailures 验证存储错误、损坏行、悬空引用及默认/模板合并环均显式失败。
// 前置纯内存快照，每例独立，无HTTP或数据库；预先已有错误应原样保留，无资源清理。
func TestCompileFallbackFailures(t *testing.T) {
	models := []config.ModelEntry{{ModelName: "a"}, {ModelName: "b"}}
	for _, kind := range []string{"model_defaults", "routing_groups", "model_fallbacks"} {
		t.Run(kind, func(t *testing.T) {
			if result := Compile(BuiltinSettings(), fallbackConfigReader{fail: kind}, models); result.Err == nil {
				t.Fatal("存储故障被忽略")
			}
		})
	}
	for _, tc := range []struct {
		name  string
		raw   any
		local map[string]any
	}{
		{"invalid-row", map[string]any{"fallbacks": "b"}, nil},
		{"missing-target", FallbackPolicy{Fallbacks: []string{"missing"}}, nil},
		{"merged-cycle", FallbackPolicy{Fallbacks: []string{"b"}}, map[string]any{"content_policy_fallbacks": []any{map[string]any{"b": []any{"a"}}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc := BuiltinDocument()
			for k, v := range tc.local {
				doc[k] = v
			}
			store := fallbackConfigReader{rows: map[string]map[string]any{"model_fallbacks": {"a": tc.raw}}}
			if result := Compile(RouteSettings{Settings: doc}, store, models); result.Err == nil {
				t.Fatal("非法合并配置被执行")
			}
		})
	}
	original := errors.New("prior failure")
	if result := Compile(RouteSettings{Err: original}, fallbackConfigReader{fail: "model_defaults"}, nil); result.Err != original {
		t.Fatal("先前错误未保留")
	}
}
