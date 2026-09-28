// Package models shares JSON readers for model handlers. An empty body becomes an empty map, and a parse failure is not treated as 400.
package models

import (
	"encoding/json"
	"github.com/sunqirui1987/xhub/internal/logx"
	"io"
	"net/http"
	"sync"
)

var logTraceOnceCodec sync.Once

// readMap reads a JSON object. An empty body or a parse failure returns an empty map.
func readMap(r *http.Request) map[string]any {
	logTraceOnceCodec.Do(func() { logx.Trace("enter models.readMap") })

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
