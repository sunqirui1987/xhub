// Package guard shares JSON readers. An empty body becomes an empty map.
package guard

import (
	"encoding/json"
	"github.com/sunqirui1987/xhub/internal/logx"
	"io"
	"net/http"
	"sync"
)

var logTraceOnceCodec sync.Once

// readMap reads a JSON object. An empty body or a parse failure returns an empty map.
// 参数 r（*http.Request）：入站 HTTP 请求，用来读路径、头和正文。
// 返回 map[string]any（map[string]any）：读取表的字段表。缺键表示上游或库里没有这个字段。
// 调用：gateway/guard/guard.go。
// 测试：无直接单测
func readMap(r *http.Request) map[string]any {
	logTraceOnceCodec.Do(func() { logx.Trace("enter guard.readMap") })

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
// 调用：gateway/guard/guard.go。
// 测试：无直接单测
func str(v any) string {
	s, _ := v.(string)
	return s
}

// boolOf accepts a bool, the strings true and 1, or a non-zero number. Every other value, including a missing one, is false.
// 参数 v（any）：JSON 里读出的动态值。数字、字符串和对象都要接住，类型不符时按零值而不是 panic。
// 返回 bool（bool）：值是布尔真、字符串 true 或 1、或非零数字时返回真。缺失和其他值都是假。
// 调用：gateway/guard/guard.go
// 测试：guard_test.go
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
