// 身份接口共用的 JSON 和时间辅助。解析失败保持空值，不把坏输入写成 0。
package identity

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"time"
)

// str 把值当成字符串。不是字符串时返回空串，不 panic。
func str(v any) string {
	s, _ := v.(string)
	return s
}

// boolOf 把布尔、字符串 true/1 或非零数字收成布尔。其余类型都是 false。
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

// parseNullFloat 把 JSON 数字或数字字符串收成可空浮点。空串和无法解析的值保持无效，不写成 0。
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

// parseNullInt 把 JSON 数字收成可空整数。float64 会截断小数。其它类型保持无效。
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

// cloneMap 浅拷贝 map。随后修改副本不会改到原表。
func cloneMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// pagedListBody 按页切片并带上 meta 和 links。page 小于 1 时按第 1 页，size 小于 1 时按 50。
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

// spendTimeInRange 判断日志时间是否在查询的 start_date 与 end_date 之间。两个参数都空，或时间无法解析时，保留该行。
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

// parseSpendTime 解析 RFC3339 或 2006-01-02 15:04:05。空串和认不出的格式返回 false。
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

// sortSpendLogs 按 startTime 从新到旧排序。无法比较的时间保持相对靠后。
func sortSpendLogs(list []map[string]any) {
	sort.Slice(list, func(i, j int) bool {
		return str(list[i]["startTime"]) > str(list[j]["startTime"])
	})
}

// encodeModels 把模型列表收成库存的 JSON 字符串。无法识别的类型写成空数组。
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
