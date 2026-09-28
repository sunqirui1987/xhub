// Package keys shares JSON readers used while assembling a virtual key. An empty body becomes an empty map, and a bad number stays invalid instead of becoming 0.
package keys

import (
	"database/sql"
	"encoding/json"
	"github.com/sunqirui1987/xhub/internal/logx"
	"io"
	"net/http"
	"strconv"
	"sync"
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

// nullFloatMap turns a nullable float into a JSON number or null.
func nullFloatMap(v sql.NullFloat64) any {
	if !v.Valid {
		return nil
	}
	return v.Float64
}

// nullIntMap turns a nullable integer into a JSON number or null.
func nullIntMap(v sql.NullInt64) any {
	if !v.Valid {
		return nil
	}
	return v.Int64
}

// emptyNil turns an empty string into null so public JSON can tell an unset field from an empty one.
func emptyNil(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// encodeModels stores a model list as a JSON string. An unrecognized type becomes an empty array.
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

// parseNullFloat converts a JSON number or numeric string into a nullable float. An empty or unparseable value stays invalid and is not written as 0.
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

// parseNullInt converts a JSON number into a nullable integer. A float64 is truncated. Any other type stays invalid.
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
