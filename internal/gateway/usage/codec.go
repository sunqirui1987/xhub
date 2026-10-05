// Package usage shares JSON readers. An empty body becomes an empty map, and a bad number becomes 0 instead of an error.
package usage

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
	logTraceOnceCodec.Do(func() { logx.Trace("enter usage.readMap") })

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

// queryInt reads an integer query parameter. A missing or unparseable value
// returns the fallback rather than an error, because every caller here has a
// sensible default.
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
// or nonsensical page returns the first row.
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

// asFloat converts a JSON number to float64. An int is accepted. Any other type returns 0.
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
