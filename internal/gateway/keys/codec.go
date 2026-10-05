// Package keys shares JSON readers used while assembling a virtual key. An empty body becomes an empty map, and a bad number stays unset instead of becoming 0.
package keys

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"sync"

	"github.com/sunqirui1987/xhub/internal/logx"
)

var logTraceOnceCodec sync.Once

// readMap reads a JSON object. An empty body or a parse failure returns an empty map.
func readMap(r *http.Request) map[string]any {
	logTraceOnceCodec.Do(func() { logx.Trace("enter keys.readMap") })

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

// stringList reads a model or access-group list. A missing or unrecognized
// value is an empty list, which means "inherit" rather than "deny".
func stringList(v any) []string {
	switch t := v.(type) {
	case []any:
		out := make([]string, 0, len(t))
		for _, x := range t {
			if s, ok := x.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	case []string:
		return append([]string(nil), t...)
	default:
		return nil
	}
}

// floatJSON turns an optional float into a JSON number or null.
func floatJSON(v *float64) any {
	if v == nil {
		return nil
	}
	return *v
}

// intJSON turns an optional integer into a JSON number or null.
func intJSON(v *int) any {
	if v == nil {
		return nil
	}
	return *v
}

// emptyNil turns an empty string into null so public JSON can tell an unset field from an empty one.
func emptyNil(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// deref reads an optional string. A nil pointer is the empty string.
func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// parseFloat converts a JSON number or numeric string into an optional float. An
// empty or unparseable value stays unset, so a bad patch never silently writes 0.
func parseFloat(v any) *float64 {
	switch t := v.(type) {
	case float64:
		return &t
	case string:
		if t == "" {
			return nil
		}
		f, err := strconv.ParseFloat(t, 64)
		if err != nil {
			return nil
		}
		return &f
	default:
		return nil
	}
}

// parseInt converts a JSON number into an optional integer. A float64 is
// truncated. Any other type stays unset.
func parseInt(v any) *int {
	switch t := v.(type) {
	case float64:
		n := int(t)
		return &n
	case int:
		return &t
	default:
		return nil
	}
}
