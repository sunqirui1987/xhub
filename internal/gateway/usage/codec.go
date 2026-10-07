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
// 参数 r（*http.Request）：入站 HTTP 请求，用来读路径、头和正文。
// 返回 map[string]any（map[string]any）：读取表的字段表。缺键表示上游或库里没有这个字段。
// 调用：gateway/usage/chat.go、gateway/usage/reports.go。
// 测试：无直接单测
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
// 参数 v（any）：JSON 里读出的动态值。数字、字符串和对象都要接住，类型不符时按零值而不是 panic。
// 返回 string（string）：按字符串读出的值。不是字符串或没有该键时为空串，不 panic。
// 调用：gateway/usage/reports.go。
// 测试：无直接单测
func str(v any) string {
	s, _ := v.(string)
	return s
}

// queryInt reads an integer query parameter. A missing or unparseable valuereturns the fallback rather than an error, because every caller here has asensible default.
// 参数 r：入站请求。key：查询参数名。fallback：缺失或无法解析时的默认值。
// 返回：解析出的整数，或 fallback。
// 调用：gateway/usage/reports.go。测试：无直接单测。
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

// pageOffset turns a 1-based page into a row offset for a page size. A missing or nonsensical page returns the first row.
// 参数 r（*http.Request）：入站 HTTP 请求；limit（int）：最多返回的条数。
// 返回 int（int）：(page-1)*limit 的行偏移。页码缺失、不是数字或小于 1 时为 0。limit 小于 1 时也是 0。
// 调用：gateway/usage/activity.go。
// 测试：无直接单测
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
// 参数 v（any）：JSON 里读出的动态值。数字、字符串和对象都要接住，类型不符时按零值而不是 panic。
// 返回 float64（float64）：作为小数。缺失时为 0，不要把它理解成免费除非调用方另有约定。
// 调用：gateway/usage/reports.go。
// 测试：无直接单测
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
// 参数 v（any）：JSON 里读出的动态值。数字、字符串和对象都要接住，类型不符时按零值而不是 panic。
// 返回 int（int）：从 JSON 或查询参数转成的整数。类型不符或缺失时为 0，不 panic。
// 调用：gateway/usage/reports.go。
// 测试：无直接单测
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
