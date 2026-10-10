package httpx

import (
	"math"
	"testing"
)

// TestValidateBudgetFields 验证额度输入的正常、空值、边界和失败输入；前置纯 JSON 字段，无外部数据需清理。
// 参数 t 为测试上下文，无返回；确认错误内容不被解析成不限额，调用方可以在写入前返回 400。
func TestValidateBudgetFields(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value any
		valid bool
	}{
		{"null", nil, true}, {"empty", "", true}, {"zero", float64(0), true}, {"positive", 100.5, true}, {"numeric string", "100.5", true},
		{"negative", -1.0, false}, {"malformed", "wrong", false}, {"NaN string", "NaN", false}, {"infinite string", "Infinity", false},
		{"NaN", math.NaN(), false}, {"infinite", math.Inf(1), false}, {"boolean", true, false}, {"array", []any{}, false}, {"object", map[string]any{}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := ValidateBudgetFields(map[string]any{"max_budget": tc.value}, "max_budget"); (err == nil) != tc.valid {
				t.Fatalf("额度校验错误 valid=%v err=%v", tc.valid, err)
			}
		})
	}
	if err := ValidateBudgetFields(map[string]any{}, "max_budget"); err != nil {
		t.Fatal(err)
	}
}
