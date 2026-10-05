// Package identity shares the JSON readers these handlers assemble requests
// from. A parse failure stays unset rather than being written as zero, so a bad
// patch never silently clears a budget or a narrowing list.
package identity

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"sync"

	"github.com/sunqirui1987/xhub/internal/logx"
)

var logTraceOnceCodec sync.Once

// readMap reads a JSON object from a request. An empty body or a parse failure
// returns an empty map, and later writes on the returned map are local.
func readMap(r *http.Request) map[string]any {
	logTraceOnceCodec.Do(func() { logx.Trace("enter identity.readMap") })

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

// has reports whether the body carried the key at all. A partial update needs
// this to tell "leave it alone" from "clear it".
func has(body map[string]any, key string) bool {
	_, ok := body[key]
	return ok
}

// floatPtr reads an optional float. An empty or unparseable value stays unset,
// so a bad patch never writes 0.
func floatPtr(v any) *float64 {
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

// intPtr reads an optional integer. A float64 is truncated; any other type
// stays unset.
func intPtr(v any) *int {
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

// optionalFloat is a two-level optional for a budget patch: nil means the field
// was absent, a non-nil pointer to nil means the body asked to clear it.
func optionalFloat(body map[string]any, key string) **float64 {
	if !has(body, key) {
		return nil
	}
	v := floatPtr(body[key])
	return &v
}

// stringPtr reads an optional string. A field that was absent returns nil.
func stringPtr(body map[string]any, key string) *string {
	if !has(body, key) {
		return nil
	}
	s := str(body[key])
	return &s
}

// stringList reads a model or access-group list. A missing or unrecognized
// value is nil, which means "inherit" rather than "deny".
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

// queryInt reads a query parameter as an integer. A missing or bad value
// returns the fallback.
func queryInt(r *http.Request, key string, fallback int) int {
	raw := r.URL.Query().Get(key)
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return n
}

// pageOffset turns a 1-based page into a row offset for a page size. A missing
// or non-positive page reads as the first one, so a bad value never skips rows.
func pageOffset(r *http.Request, limit int) int {
	if limit < 1 {
		return 0
	}
	off := (queryInt(r, "page", 1) - 1) * limit
	if off < 0 {
		return 0
	}
	return off
}

// idsFrom reads an id list from the body. The plural key wins, and a non-empty
// singular field is appended.
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

// nonNilStrings keeps the JSON list shape stable: a record with no narrowing
// emits an empty list rather than null.
func nonNilStrings(in []string) []string {
	if in == nil {
		return []string{}
	}
	return in
}

// emptyNil turns an empty string into null, so public JSON can tell an unset
// field from an empty one.
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
