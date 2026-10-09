// Package guard 提供历史调试接口的动态 JSON 读取辅助方法。
package guard

import (
	"encoding/json"
	"github.com/sunqirui1987/xhub/internal/logx"
	"io"
	"net/http"
	"sync"
)

var logTraceOnceCodec sync.Once

// readMap 读取历史接口 JSON 正文；空正文或解析失败按空对象处理。
// 参数：r：带 JSON 正文的入站 HTTP 请求；本方法只读取正文。
// 返回：动态字段表；空正文或解析失败返回非 nil 空对象，便于历史调用方按缺键处理。
// 调用：历史调试接口；新管理接口另行解码并调用 Validate 严格校验。
// 测试：无直接单测；历史接口行为由 guard_test.go 间接覆盖。
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

// str 从动态值读取字符串，其他类型返回空串。
// 参数：v：JSON 字段或配置中的动态值。
// 返回：原字符串；nil 及其他类型返回空串，不隐式转换数字、布尔值或对象。
// 调用：本包的规则解析、调试输入和结果读取。
// 测试：无直接单测；管理、执行和外部协议测试间接覆盖。
func str(v any) string {
	s, _ := v.(string)
	return s
}

// boolOf 兼容布尔值、true/1 字符串及非零 JSON 数字；缺失或其他类型为 false。
// 参数：v：历史配置动态值；兼容 JSON 解码后的 float64 数值。
// 返回：布尔值本身、字符串 true/1 或非零 float64 对应真；缺失及其他类型为假。
// 调用：默认启用判定及旧配置兼容；严格写入校验不通过本函数放宽字段类型。
// 测试：guard_test.go 的 TestBoolOfReadsTheFormsJSONProduces。
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
