// Package guard shares JSON readers. An empty body becomes an empty map.
package guard

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

// boolOf accepts a bool, the strings true and 1, or a non-zero number. Every other value, including a missing one, is false.
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
