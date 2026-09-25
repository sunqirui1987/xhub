// 虚拟密钥组装用的 JSON 读取。空正文得到空表，坏数字保持无效，不写成 0。
package keys

import (
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
)

// readMap 读取 JSON 对象。空正文或解析失败时返回空表。
func readMap(r *http.Request) map[string]any {
	var body map[string]any
	b, _ := io.ReadAll(r.Body)
	_ = json.Unmarshal(b, &body)
	if body == nil {
		body = map[string]any{}
	}
	return body
}

// str 把值当成字符串。不是字符串时返回空串，不 panic。
func str(v any) string {
	s, _ := v.(string)
	return s
}

// boolOf 把布尔、字符串 true/1 或非零数字收成布尔。其余类型，包括缺省，都是 false。
func boolOf(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		return t == "true" || t == "1"
	case float64:
		return t != 0
	default:
		return false
	}
}

// idsFrom 从正文读取 id 列表。复数键优先，单数字段有值时再追加。
func idsFrom(body map[string]any, plural, singular string) []string {
	var out []string
	switch v := body[plural].(type) {
	case []any:
		for _, x := range v {
			if s, ok := x.(string); ok && s != "" {
				out = append(out, s)
			}
		}
	case []string:
		out = append(out, v...)
	}
	if s := str(body[singular]); s != "" {
		out = append(out, s)
	}
	return out
}

// nullFloatMap 把可空浮点变成 JSON 数字或 null。
func nullFloatMap(v sql.NullFloat64) any {
	if !v.Valid {
		return nil
	}
	return v.Float64
}

// nullIntMap 把可空整数变成 JSON 数字或 null。
func nullIntMap(v sql.NullInt64) any {
	if !v.Valid {
		return nil
	}
	return v.Int64
}

// emptyNil 把空字符串变成 null，便于对外 JSON 区分未设置。
func emptyNil(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// encodeModels 把模型列表收成库存的 JSON 字符串。无法识别的类型写成空数组。
func encodeModels(v any) string {
	switch t := v.(type) {
	case []any:
		b, _ := json.Marshal(t)
		return string(b)
	case []string:
		b, _ := json.Marshal(t)
		return string(b)
	default:
		return "[]"
	}
}

// parseNullFloat 把 JSON 数字或数字字符串收成可空浮点。空串和无法解析的值保持无效，不写成 0。
func parseNullFloat(v any) sql.NullFloat64 {
	switch t := v.(type) {
	case float64:
		return sql.NullFloat64{Float64: t, Valid: true}
	case string:
		if t == "" {
			return sql.NullFloat64{}
		}
		f, err := strconv.ParseFloat(t, 64)
		if err != nil {
			return sql.NullFloat64{}
		}
		return sql.NullFloat64{Float64: f, Valid: true}
	default:
		return sql.NullFloat64{}
	}
}

// parseNullInt 把 JSON 数字收成可空整数。float64 会截断小数。其它类型保持无效。
func parseNullInt(v any) sql.NullInt64 {
	switch t := v.(type) {
	case float64:
		return sql.NullInt64{Int64: int64(t), Valid: true}
	case int:
		return sql.NullInt64{Int64: int64(t), Valid: true}
	default:
		return sql.NullInt64{}
	}
}
