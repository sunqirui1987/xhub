// Package catalog prices a token count from the built-in price map. An unknown model returns ok false and must not be recorded as zero.
package catalog

import (
	"github.com/sunqirui1987/xhub/internal/logx"
	"strconv"
	"sync"
)

var logTraceOnceCost sync.Once

// Cost returns the dollar total, input cost, and output cost for a token count. An unknown model returns ok false so the caller does not record a free call.
// 调用：gateway/spend.go、gateway/usage/reports.go
// 测试：无直接单测
// 参数 model（string）：发给上游或对外展示的模型名；prompt（int）：提示 token 数，用来估价；completion（int）：完成 token 数，用来估价。
// 返回 total（float64）：这一次的总费用；input（float64）：输入侧费用；output（float64）：输出侧费用；ok（bool）：真表示找到了可用结果。
func Cost(model string, prompt, completion int) (total, input, output float64, ok bool) {
	logTraceOnceCost.Do(func() { logx.Trace("enter catalog.Cost") })

	in, out, ok := rates(model)
	if !ok {
		return 0, 0, 0, false
	}
	input = float64(prompt) * in
	output = float64(completion) * out
	return input + output, input, output, true
}

// rates returns the per-token dollar price for input and output from the built-in price map. An unknown model returns ok false.
// 参数 model（string）：对外模型名，用来选部署和记用量。
// 调用：仅在 cost.go 内使用
// 测试：无直接单测
// 返回 input（float64）：输入侧费用；output（float64）：输出侧费用；ok（bool）：真表示找到了可用结果。
func rates(model string) (input, output float64, ok bool) {
	return TokenRates(model)
}

// Format renders a dollar amount as a decimal string without trailing zeros.
// 参数 v（float64）：格式化使用的小数。0 表示没有费用或尚未计价。
// 返回 string（string）：不带多余尾随零的金额十进制文本。
// 调用：catalog/model_cost.go、gateway/family/handlers.go、gateway/guard/guard.go、gateway/identity/handlers.go
// 测试：activity_http_test.go、activity_test.go、log_completeness_test.go
func Format(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}
