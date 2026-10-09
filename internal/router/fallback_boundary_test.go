package router

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/sunqirui1987/xhub/internal/config"
)

// TestFallbackPolicyCategoryBoundaries 验证三类策略的类型、顺序、32个目标边界和跨类别复用。
// 前置独立内存JSON；正常输入完整保留，错误输入拒绝，解析不修改输入，无外部数据需清理。
func TestFallbackPolicyCategoryBoundaries(t *testing.T) {
	for _, category := range []struct{ field, kind string }{{"fallbacks", "general"}, {"context_window_fallbacks", "context"}, {"content_policy_fallbacks", "content"}} {
		t.Run(category.kind, func(t *testing.T) {
			for _, tc := range []struct {
				name  string
				value any
				valid bool
			}{
				{"null-list", nil, true}, {"empty-list", []string{}, true}, {"ordered", []string{"z", "a"}, true},
				{"scalar", "a", false}, {"object", map[string]any{}, false}, {"boolean", true, false},
				{"null-target", []any{nil}, false}, {"number-target", []any{1}, false},
				{"trailing-space", []string{"a "}, false}, {"unicode-space", []string{"\u3000"}, false},
				{"duplicate", []string{"a", "a"}, false},
			} {
				t.Run(tc.name, func(t *testing.T) {
					raw := map[string]any{category.field: tc.value}
					before, _ := json.Marshal(raw)
					p, err := ParseFallbackPolicy(raw)
					if (err == nil) != tc.valid {
						t.Fatalf("%s 解析结果与契约不符: %v", category.field, err)
					}
					if targets, ok := tc.value.([]string); tc.valid && ok && !reflect.DeepEqual(p.Targets(category.kind), targets) {
						t.Fatalf("目标顺序损坏: %v", p)
					}
					after, _ := json.Marshal(raw)
					if string(before) != string(after) {
						t.Fatal("解析修改了调用方输入")
					}
				})
			}
			for _, count := range []int{32, 33} {
				t.Run(fmt.Sprint(count), func(t *testing.T) {
					names := make([]string, count)
					for i := range names {
						names[i] = fmt.Sprintf("target-%02d", i)
					}
					p, err := ParseFallbackPolicy(map[string]any{category.field: names})
					if (err == nil) != (count == 32) {
						t.Fatalf("%s 的%d个目标边界: %v", category.field, count, err)
					}
					if err == nil && !reflect.DeepEqual(p.Targets(category.kind), names) {
						t.Fatal("边界列表未完整保留")
					}
				})
			}
		})
	}
	p, err := ParseFallbackPolicy(FallbackPolicy{Fallbacks: []string{"b"}, ContextWindow: []string{"b"}, ContentPolicy: []string{"b"}})
	if err != nil || len(p.Targets("content")) != 1 {
		t.Fatalf("不同错误类别应允许相同目标: %v", err)
	}
	for _, raw := range []any{nil, true, 1, "policy", func() {}, json.RawMessage(`{`)} {
		if _, err := ParseFallbackPolicy(raw); err == nil {
			t.Fatalf("非法对象被接受: %T", raw)
		}
	}
}

// TestFallbackGraphSharedAndDisconnected 验证共享后继、独立策略、三类悬空引用及多级循环。
// 前置含重复部署的公开目录；合法有向无环图通过，非法图拒绝，图和目录始终不变，无持久化资源。
func TestFallbackGraphSharedAndDisconnected(t *testing.T) {
	models := []config.ModelEntry{{ModelName: "a"}, {ModelName: "b"}, {ModelName: "c"}, {ModelName: "d"}, {ModelName: "a"}}
	for _, tc := range []struct {
		name     string
		policies map[string]FallbackPolicy
		valid    bool
	}{
		{"diamond", map[string]FallbackPolicy{"a": {Fallbacks: []string{"b", "c"}}, "b": {ContextWindow: []string{"d"}}, "c": {ContentPolicy: []string{"d"}}}, true},
		{"independent", map[string]FallbackPolicy{"a": {Fallbacks: []string{"b"}}, "c": {Fallbacks: []string{"d"}}}, true},
		{"deep-cross-category-cycle", map[string]FallbackPolicy{"a": {Fallbacks: []string{"b"}}, "b": {ContextWindow: []string{"c"}}, "c": {ContentPolicy: []string{"a"}}}, false},
		{"disconnected-cycle", map[string]FallbackPolicy{"a": {Fallbacks: []string{"b"}}, "c": {Fallbacks: []string{"d"}}, "d": {ContextWindow: []string{"c"}}}, false},
		{"missing-context", map[string]FallbackPolicy{"a": {ContextWindow: []string{"missing"}}}, false},
		{"missing-content", map[string]FallbackPolicy{"a": {ContentPolicy: []string{"missing"}}}, false},
		{"empty", nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before, _ := json.Marshal(tc.policies)
			if err := ValidateFallbackGraph(tc.policies, models); (err == nil) != tc.valid {
				t.Fatalf("完整图验证错误: %v", err)
			}
			after, _ := json.Marshal(tc.policies)
			if string(before) != string(after) || len(models) != 5 {
				t.Fatal("验证修改了输入图或目录")
			}
		})
	}
}
