package httpx

import (
	"fmt"
	"math"
	"strconv"
)

// ValidateBudgetFields 检查请求中的额度字段，防止非法输入被可选数值解析器当作共享或不限额。
// 参数 body 为已解析 JSON，fields 为待检查字段；返回错误或 nil；供管理接口在持久化前调用，无副作用。
// 缺省、null 和空字符串表示共享；数值及数字字符串必须有限且非负，错误字段名用于定位请求。
func ValidateBudgetFields(body map[string]any, fields ...string) error {
	for _, field := range fields {
		raw, present := body[field]
		if !present || raw == nil || raw == "" {
			continue
		}
		var value float64
		switch v := raw.(type) {
		case float64:
			value = v
		case string:
			parsed, err := strconv.ParseFloat(v, 64)
			if err != nil {
				return fmt.Errorf("%s must be a finite non-negative number or null", field)
			}
			value = parsed
		default:
			return fmt.Errorf("%s must be a finite non-negative number or null", field)
		}
		if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
			return fmt.Errorf("%s must be a finite non-negative number or null", field)
		}
	}
	return nil
}

// ValidateRateFields 拒绝分数、负数、溢出及错误类型；参数 body 为请求，返回可定位字段的错误。
// 调用：四层管理接口；null/空白清空本层限制，缺字段保留，不写数据库。
func ValidateRateFields(body map[string]any) error {
	for _, field := range []string{"rpm_limit", "tpm_limit"} {
		if err := ValidateBudgetFields(body, field); err != nil {
			return err
		}
		raw := body[field]
		if raw == nil || raw == "" {
			continue
		}
		value, _ := RateValue(raw)
		if math.Trunc(value) != value || value > 2147483647 {
			return fmt.Errorf("%s must be a non-negative integer up to 2147483647 or null", field)
		}
	}
	return nil
}

// RateValue 解析已经校验的数字或数字字符串；参数 raw 为单值，返回数值和解析错误，无副作用。
func RateValue(raw any) (float64, error) {
	if v, ok := raw.(float64); ok {
		return v, nil
	}
	if v, ok := raw.(string); ok {
		return strconv.ParseFloat(v, 64)
	}
	return 0, fmt.Errorf("invalid rate")
}

// OptionalRate 区分缺字段与清空；参数 body/field 为已校验请求，返回可空增量指针，供 IAM 更新调用。
func OptionalRate(body map[string]any, field string) **int {
	raw, ok := body[field]
	if !ok {
		return nil
	}
	var value *int
	if raw != nil && raw != "" {
		number, _ := RateValue(raw)
		v := int(number)
		value = &v
	}
	return &value
}

// Rate 解析已校验的创建字段；参数 body/field 为请求，返回本层上限或 nil，不改变请求。
func Rate(body map[string]any, field string) *int {
	if p := OptionalRate(body, field); p != nil {
		return *p
	}
	return nil
}
