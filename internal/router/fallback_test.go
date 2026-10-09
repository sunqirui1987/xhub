package router

import (
	"encoding/json"
	"github.com/sunqirui1987/xhub/internal/config"
	"testing"
)

// TestFallbackPolicy 验证三类策略解析及无效边界；前置内存 JSON，断言严格格式且无需清理。
func TestFallbackPolicy(t *testing.T) {
	for _, raw := range []string{
		"null", "[]", "{}", "{\"fallbacks\":[\"b\",\"c\"]}",
		"{\"unknown\":[]}", "{\"fallbacks\":[\"\"]}", "{\"fallbacks\":[\" b\"]}", "{\"fallbacks\":[\"b\",\"b\"]}", "{\"fallbacks\":[1]}",
	} {
		t.Run(raw, func(t *testing.T) {
			p, err := ParseFallbackPolicy(json.RawMessage(raw))
			valid := raw == "{}" || raw == "{\"fallbacks\":[\"b\",\"c\"]}"
			if (err == nil) != valid {
				t.Fatalf("解析 %s: %v", raw, err)
			}
			if valid && len(p.Targets("unknown")) != 0 {
				t.Fatal("未知类别返回了目标")
			}
		})
	}
	targets := make([]string, 33)
	for i := range targets {
		targets[i] = string(rune('a' + i))
	}
	if _, err := ParseFallbackPolicy(FallbackPolicy{Fallbacks: targets}); err == nil {
		t.Fatal("超过32个目标未拒绝")
	}
}

// TestFallbackGraph 验证存在性和跨错误类别环；前置三个公开部署，正常有向图通过，错误图拒绝，无存储副作用。
func TestFallbackGraph(t *testing.T) {
	models := []config.ModelEntry{{ModelName: "a"}, {ModelName: "b"}, {ModelName: "c"}}
	for _, tc := range []struct {
		name  string
		p     map[string]FallbackPolicy
		valid bool
	}{
		{"ordered", map[string]FallbackPolicy{"a": {Fallbacks: []string{"b", "c"}}}, true},
		{"missing", map[string]FallbackPolicy{"a": {Fallbacks: []string{"missing"}}}, false},
		{"self", map[string]FallbackPolicy{"a": {ContextWindow: []string{"a"}}}, false},
		{"cross-category", map[string]FallbackPolicy{"a": {Fallbacks: []string{"b"}}, "b": {ContentPolicy: []string{"a"}}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := ValidateFallbackGraph(tc.p, models); (err == nil) != tc.valid {
				t.Fatalf("回退图校验: %v", err)
			}
		})
	}
}
