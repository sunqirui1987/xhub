// Package identity shares JSON and time helpers. A parse failure stays empty instead of writing a bad input as 0.
package identity

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/sunqirui1987/xhub/internal/logx"
	"net/http"
	"sort"
	"strconv"
	"sync"
	"time"
)

var logTraceOnceCodec sync.Once

// str reads v as a string. A non-string returns an empty string and does not panic.
func str(v any) string {
	logTraceOnceCodec.Do(func() { logx.Trace("enter identity.str") })

	s, _ := v.(string)
	return s
}

// boolOf accepts a bool, the strings true and 1, or a non-zero number. Every other type is false.
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

// cloneMap shallow-copies a map. Later edits to the copy do not change the original.
func cloneMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// pagedListBody slices one page and adds meta and links. A page below 1 is page 1. A size below 1 is 50.
func pagedListBody(path string, list []map[string]any, page, size int) map[string]any {
	if list == nil {
		list = []map[string]any{}
	}
	if size < 1 {
		size = 50
	}
	if page < 1 {
		page = 1
	}
	total := len(list)
	pages := total / size
	if total%size != 0 {
		pages++
	}
	if pages < 1 {
		pages = 1
	}
	data := sliceMaps(list, page, size)
	self := fmt.Sprintf("%s?page=%d&page_size=%d", path, page, size)
	links := map[string]any{
		"self":  self,
		"first": fmt.Sprintf("%s?page=1&page_size=%d", path, size),
		"last":  fmt.Sprintf("%s?page=%d&page_size=%d", path, pages, size),
		"next":  nil,
		"prev":  nil,
	}
	if page < pages {
		links["next"] = fmt.Sprintf("%s?page=%d&page_size=%d", path, page+1, size)
	}
	if page > 1 {
		links["prev"] = fmt.Sprintf("%s?page=%d&page_size=%d", path, page-1, size)
	}
	return map[string]any{
		"data":  data,
		"meta":  map[string]any{"page": page, "page_size": size, "total_count": total, "total_pages": pages},
		"links": links,
	}
}

// spendTimeInRange reports whether a log time falls between start_date and end_date. Both empty, or an unparseable time, keeps the row.
func spendTimeInRange(ts string, r *http.Request) bool {
	start := r.URL.Query().Get("start_date")
	end := r.URL.Query().Get("end_date")
	if start == "" && end == "" {
		return true
	}
	at, ok := parseSpendTime(ts)
	if !ok {
		return true
	}
	if bound, ok := parseSpendTime(start); ok && at.Before(bound) {
		return false
	}
	if bound, ok := parseSpendTime(end); ok && at.After(bound) {
		return false
	}
	return true
}

// parseSpendTime parses RFC3339 or 2006-01-02 15:04:05. An empty string or an unrecognized format returns false.
func parseSpendTime(ts string) (time.Time, bool) {
	if ts == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05", "2006-01-02T15:04"} {
		if t, err := time.Parse(layout, ts); err == nil {
			return t.UTC(), true
		}
	}
	return time.Time{}, false
}

// sortSpendLogs orders logs by startTime, newest first. A time that cannot be compared stays later.
func sortSpendLogs(list []map[string]any) {
	sort.Slice(list, func(i, j int) bool {
		return str(list[i]["startTime"]) > str(list[j]["startTime"])
	})
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
