package httpx

import (
	"fmt"
	"testing"
)

// TestRateFields 验证创建和更新的整数契约与缺省/null 区分；前置已解码 JSON，无存储副作用。
func TestRateFields(t *testing.T) {
	for _, field := range []string{"rpm_limit", "tpm_limit"} {
		for i, raw := range []any{nil, "", float64(0), float64(2147483647), "12", "0", float64(-1), float64(1.5), "NaN", "Infinity", "wrong", true, []any{}, map[string]any{}, float64(2147483648)} {
			t.Run(fmt.Sprintf("%s/%d", field, i), func(t *testing.T) {
				body := map[string]any{field: raw}
				err := ValidateRateFields(body)
				if (err != nil) != (i >= 6) {
					t.Fatalf("非法限额 %v: %v", raw, err)
				}
				if err == nil {
					p := OptionalRate(body, field)
					if p == nil {
						t.Fatal("提交被误判为缺字段")
					}
					if (raw == nil || raw == "") && *p != nil {
						t.Fatal("清空未生成 nil")
					}
				}
			})
		}
	}
	if OptionalRate(map[string]any{}, "rpm_limit") != nil {
		t.Fatal("缺字段覆盖旧值")
	}
}
