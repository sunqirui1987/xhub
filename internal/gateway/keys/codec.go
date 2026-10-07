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
// 参数 r（*http.Request）：入站 HTTP 请求，用来读路径、头和正文。
// 返回 map[string]any（map[string]any）：读取表的字段表。缺键表示上游或库里没有这个字段。
// 调用：gateway/keys/admin.go。
// 测试：无直接单测
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
// 参数 v（any）：JSON 里读出的动态值。数字、字符串和对象都要接住，类型不符时按零值而不是 panic。
// 返回 string（string）：按字符串读出的值。不是字符串或没有该键时为空串，不 panic。
// 调用：gateway/keys/admin.go、gateway/keys/generate.go。
// 测试：无直接单测
func str(v any) string {
	s, _ := v.(string)
	return s
}

// idsFrom reads an id list from the body. The plural key wins, and a non-empty singular field is appended.
// 参数 body（map[string]any）：已解析或原始的 JSON；plural（string）：标识列表来源使用的plural。空串表示调用方没有提供这项；singular（string）：标识列表来源使用的singular。空串表示调用方没有提供这项。
// 返回 []string（[]string）：标识列表来源。没有匹配时为空切片。
// 调用：gateway/keys/admin.go。
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

// stringList reads a model or access-group list. A missing or unrecognized value is an empty list, which means "inherit" rather than "deny".
// 参数 v（any）：JSON 里读出的动态值。数字、字符串和对象都要接住，类型不符时按零值而不是 panic。
// 返回 []string（[]string）：字符串列表。没有匹配时为 nil 或空切片，调用方按长度判断。
// 调用：gateway/keys/generate.go。
// 测试：无直接单测
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
// 参数 v（*float64）：小数JSON使用的float64。
// 返回 any（any）：小数JSON。没有合格值时为 nil。
// 调用：gateway/keys/generate.go。
// 测试：无直接单测
func floatJSON(v *float64) any {
	if v == nil {
		return nil
	}
	return *v
}

// intJSON turns an optional integer into a JSON number or null.
// 参数 v（*int）：整数JSON使用的int。
// 返回 any（any）：整数JSON。没有合格值时为 nil。
// 调用：gateway/keys/generate.go。
// 测试：无直接单测
func intJSON(v *int) any {
	if v == nil {
		return nil
	}
	return *v
}

// emptyNil turns an empty string into null so public JSON can tell an unset field from an empty one.
// 参数 s（string）：空空要处理的文本。空串表示这段没有内容。
// 返回 any（any）：空空。没有合格值时为 nil。
// 调用：gateway/keys/generate.go。
// 测试：无直接单测
func emptyNil(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// deref reads an optional string. A nil pointer is the empty string.
// 参数 s（*string）：解引用使用的string。
// 返回 string（string）：指针指向的字符串。指针为 nil 时为空串。
// 调用：gateway/keys/generate.go。
// 测试：无直接单测
func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// parseFloat converts a JSON number or numeric string into an optional float. An empty or unparseable value stays unset, so a bad patch never silently writes 0.
// 参数 v（any）：JSON 里读出的动态值。数字、字符串和对象都要接住，类型不符时按零值而不是 panic。
// 返回 *float64（*float64）：从 JSON 数字或数字字符串解析出的小数。空串或无法解析时为 nil，坏补丁不会写成 0。
// 调用：gateway/keys/admin.go、gateway/keys/generate.go
// 测试：无直接单测
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

// parseInt converts a JSON number into an optional integer. A float64 is truncated. Any other type stays unset.
// 参数 v（any）：JSON 里读出的动态值。数字、字符串和对象都要接住，类型不符时按零值而不是 panic。
// 返回 *int（*int）：从 JSON 数字截出的整数。float64 会丢掉小数。其他类型为 nil。
// 调用：gateway/keys/generate.go
// 测试：无直接单测
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
