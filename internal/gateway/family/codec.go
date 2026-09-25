// 目录资源共用的 JSON 读取和分页。空正文得到空表，页码从 1 开始。
package family

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

// sliceMaps 按页切片。页码从 1 开始，size 小于 1 时按 50 处理，越界得到空切片。
func sliceMaps(list []map[string]any, page, size int) []map[string]any {
	if size < 1 {
		size = 50
	}
	if page < 1 {
		page = 1
	}
	start := (page - 1) * size
	if start >= len(list) {
		return []map[string]any{}
	}
	end := start + size
	if end > len(list) {
		end = len(list)
	}
	return list[start:end]
}
