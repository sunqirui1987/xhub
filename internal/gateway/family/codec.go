// Package family shares JSON readers and paging. An empty body becomes an empty map, and page numbers start at 1.
package family

import (
	"encoding/json"
	"github.com/sunqirui1987/xhub/internal/logx"
	"github.com/sunqirui1987/xhub/internal/pagination"
	"io"
	"net/http"
	"sync"
)

var logTraceOnceCodec sync.Once

// readMap reads a JSON object. An empty body or a parse failure returns an empty map.
// 参数 r（*http.Request）：入站 HTTP 请求，用来读路径、头和正文。
// 返回 map[string]any（map[string]any）：读取表的字段表。缺键表示上游或库里没有这个字段。
// 调用：gateway/family/handlers.go。
// 测试：无直接单测
func readMap(r *http.Request) map[string]any {
	logTraceOnceCodec.Do(func() { logx.Trace("enter family.readMap") })

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
// 调用：gateway/family/handlers.go。
// 测试：无直接单测
func str(v any) string {
	s, _ := v.(string)
	return s
}

// asInt converts a JSON number to int. A float64 is truncated. Any other type returns 0.
// 参数 v（any）：JSON 里读出的动态值。数字、字符串和对象都要接住，类型不符时按零值而不是 panic。
// 返回 int（int）：从 JSON 或查询参数转成的整数。类型不符或缺失时为 0，不 panic。
// 调用：gateway/family/handlers.go。
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

// idsFrom reads an id list from the body. The plural key wins, and a non-empty singular field is appended.
// 参数 body（map[string]any）：已解析或原始的 JSON；plural（string）：标识列表来源使用的plural。空串表示调用方没有提供这项；singular（string）：标识列表来源使用的singular。空串表示调用方没有提供这项。
// 返回 []string（[]string）：标识列表来源。没有匹配时为空切片。
// 调用：gateway/family/handlers.go。
// 测试：无直接单测
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
// 参数 list（[]map[string]any）：slice表使用的map[string]any；page（int）：页码，从 1 开始；size（int）：最多返回的条数。零或负数表示用默认页大小。
// 返回 []map[string]any（[]map[string]any）：一组map[string]any。没有匹配时为空切片，不是 nil 分页。
// 调用：gateway/family/handlers.go。
// 测试：无直接单测
func sliceMaps(list []map[string]any, page, size int) []map[string]any {
	if size < 1 {
		size = 50
	}
	if page < 1 {
		page = 1
	}
	// 统一边界先比较页码，避免超大页码回绕为负数导致切片崩溃。
	start, end := pagination.Bounds(len(list), page, size)
	return list[start:end]
}
