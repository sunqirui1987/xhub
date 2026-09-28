// Package family shares JSON readers and paging. An empty body becomes an empty map, and page numbers start at 1.
package family

import (
	"encoding/json"
	"io"
	"net/http"
)

// readMap reads a JSON object. An empty body or a parse failure returns an empty map.
func readMap(r *http.Request) map[string]any {
	var body map[string]any
	b, _ := io.ReadAll(r.Body)
	_ = json.Unmarshal(b, &body)
	if body == nil {
		body = map[string]any{}
	}
	return body
}

// str reads v as a string. A non-string returns an empty string and does not panic.
func str(v any) string {
	s, _ := v.(string)
	return s
}

// asInt converts a JSON number to int. A float64 is truncated. Any other type returns 0.
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

// idsFrom reads an id list from the body. The plural key wins, and a non-empty singular field is appended.
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

// sliceMaps returns one page of maps. Page numbers start at 1. A size below 1 is treated as 50. A page past the end returns an empty slice.
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
