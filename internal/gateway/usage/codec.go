// 用量接口共用的 JSON 读取。空正文得到空表，坏数字收成 0 而不是报错。
package usage

import (
	"encoding/json"
	"io"
	"net/http"
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

// asFloat 把 JSON 数字收成 float64。int 也可以。其它类型返回 0。
func asFloat(v any) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case int:
		return float64(t)
	default:
		return 0
	}
}

// asInt 把 JSON 数字收成 int。float64 会截断小数。其它类型返回 0。
func asInt(v any) int {
	switch t := v.(type) {
	case float64:
		return int(t)
	case int:
		return t
	default:
		return 0
	}
}
