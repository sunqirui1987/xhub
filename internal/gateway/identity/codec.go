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

// readMap reads a JSON object from a request. An empty body or a parse failure returns an empty map, and later writes on the returned map are local.
// 参数 r（*http.Request）：入站 HTTP 请求，用来读路径、头和正文。
// 返回 map[string]any（map[string]any）：读取表的字段表。缺键表示上游或库里没有这个字段。
// 调用：gateway/identity/handlers.go、gateway/identity/members.go。
// 测试：无直接单测
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
// 参数 v（any）：JSON 里读出的动态值。数字、字符串和对象都要接住，类型不符时按零值而不是 panic。
// 返回 string（string）：按字符串读出的值。不是字符串或没有该键时为空串，不 panic。
// 调用：gateway/identity/handlers.go、gateway/identity/members.go。
// 测试：无直接单测
func str(v any) string {
	s, _ := v.(string)
	return s
}

// has reports whether the body carried the key at all. A partial update needs this to tell "leave it alone" from "clear it".
// 参数 body（map[string]any）：已解析或原始的 JSON；key（string）：上游或调用方的密钥。空串表示还不能转发或还没有密钥。
// 返回 bool（bool）：请求正文里带了这个键时返回真。部分更新靠它区分「不改」和「清空」。
// 调用：gateway/identity/handlers.go
// 测试：无直接单测
func has(body map[string]any, key string) bool {
	_, ok := body[key]
	return ok
}

// floatPtr reads an optional float. An empty or unparseable value stays unset, so a bad patch never writes 0.
// 参数 v（any）：JSON 里读出的动态值。数字、字符串和对象都要接住，类型不符时按零值而不是 panic。
// 返回 *float64（*float64）：从 JSON 数字或数字字符串解析出的小数。空串或其他类型时为 nil，坏补丁不会写成 0。
// 调用：gateway/identity/handlers.go
// 测试：无直接单测
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

// intPtr reads an optional integer. A float64 is truncated; any other type stays unset.
// 参数 v（any）：JSON 里读出的动态值。数字、字符串和对象都要接住，类型不符时按零值而不是 panic。
// 返回 *int（*int）：从 JSON 数字截出的整数。float64 会丢掉小数。其他类型为 nil。
// 调用：仅在 codec.go 内使用
// 测试：无直接单测
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

// optionalFloat is a two-level optional for a budget patch: nil means the field was absent, a non-nil pointer to nil means the body asked to clear it.
// 参数 body（map[string]any）：已经解析的 JSON 对象；key（string）：要读取的字段名，例如 max_budget。
// 返回 **float64（**float64）：预算字段的两层可选。字段不在正文里时为 nil。在正文里时指向 *float64，值是 null 则内层为 nil，表示要清空。
// 调用：gateway/identity/handlers.go
// 测试：无直接单测
func optionalFloat(body map[string]any, key string) **float64 {
	if !has(body, key) {
		return nil
	}
	v := floatPtr(body[key])
	return &v
}

// stringPtr reads an optional string. A field that was absent returns nil.
// 参数 body（map[string]any）：已经解析的 JSON 对象；key（string）：要读取的字段名。
// 返回 *string（*string）：正文字段里的字符串。字段不在正文里时为 nil，更新不会改这一项。
// 调用：gateway/identity/handlers.go
// 测试：无直接单测
func stringPtr(body map[string]any, key string) *string {
	if !has(body, key) {
		return nil
	}
	s := str(body[key])
	return &s
}

// stringList reads a model or access-group list. A missing or unrecognized value is nil, which means "inherit" rather than "deny".
// 参数 v（any）：JSON 里读出的动态值。数字、字符串和对象都要接住，类型不符时按零值而不是 panic。
// 返回 []string（[]string）：字符串列表。没有匹配时为 nil 或空切片，调用方按长度判断。
// 调用：gateway/identity/handlers.go。
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

// queryInt reads a query parameter as an integer. A missing or bad valuereturns the fallback.
// 参数 r：入站请求。key：查询参数名。fallback：缺失或无法解析时的默认值。
// 返回：解析出的整数，或 fallback。
// 调用：gateway/identity/handlers.go。测试：无直接单测。
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

// pageOffset turns a 1-based page into a row offset for a page size. A missing or non-positive page reads as the first one, so a bad value never skips rows.
// 参数 r（*http.Request）：入站 HTTP 请求；limit（int）：最多返回的条数。
// 返回 int（int）：(page-1)*limit 的行偏移。页码缺失、不是数字或小于 1 时为 0。limit 小于 1 时也是 0。
// 调用：gateway/identity/handlers.go。
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

// idsFrom reads an id list from the body. The plural key wins, and a non-empty singular field is appended.
// 参数 body（map[string]any）：已解析或原始的 JSON；plural（string）：标识列表来源使用的plural。空串表示调用方没有提供这项；singular（string）：标识列表来源使用的singular。空串表示调用方没有提供这项。
// 返回 []string（[]string）：标识列表来源。没有匹配时为空切片。
// 调用：仅在 codec.go 内使用。
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

// nonNilStrings keeps the JSON list shape stable: a record with no narrowing emits an empty list rather than null.
// 参数 in（[]string）：内列表。空切片表示没有可处理的项。
// 返回 []string（[]string）：非空字符串。没有匹配时为 nil 或空切片，调用方按长度判断。
// 调用：gateway/identity/handlers.go。
// 测试：无直接单测
func nonNilStrings(in []string) []string {
	if in == nil {
		return []string{}
	}
	return in
}

// emptyNil turns an empty string into null, so public JSON can tell an unset field from an empty one.
// 参数 s（string）：空空要处理的文本。空串表示这段没有内容。
// 返回 any（any）：空空。没有合格值时为 nil。
// 调用：gateway/identity/handlers.go。
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
// 调用：仅在 codec.go 内使用。
// 测试：无直接单测
func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// floatJSON turns an optional float into a JSON number or null.
// 参数 v（*float64）：小数JSON使用的float64。
// 返回 any（any）：小数JSON。没有合格值时为 nil。
// 调用：gateway/identity/handlers.go。
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
// 调用：仅在 codec.go 内使用。
// 测试：无直接单测
func intJSON(v *int) any {
	if v == nil {
		return nil
	}
	return *v
}
