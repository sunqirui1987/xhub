// Package llm turns a call into an upstream URL, headers, and body, and maps provider responses back to the public shape. This file only maps HTTP status codes and does not send a request.
package llm

import (
	"github.com/sunqirui1987/xhub/internal/logx"
	"sync"
)

var logTraceOnceError sync.Once

// ExceptionForStatus maps an upstream HTTP status to a LiteLLM exception class name. Status 400 and 422 are BadRequestError. Status 408 and 504 are Timeout. A status without its own branch returns ok false so the caller treats it as an unknown upstream error.
// 参数 status（int）：上游 HTTP 状态码。
// 返回 name（string）：对应的 LiteLLM 异常类名，例如 BadRequestError 或 Timeout。没有单独分支时为空串；ok（bool）：这个状态码有对应异常类时返回真。没有分支时返回假，调用方应把它当成未知上游错误。
// 调用：仅在 error.go 内使用
// 测试：无直接单测
func ExceptionForStatus(status int) (name string, ok bool) {
	logTraceOnceError.Do(func() { logx.Trace("enter llm.ExceptionForStatus") })

	switch status {
	case 400, 422:
		return "BadRequestError", true
	case 401:
		return "AuthenticationError", true
	case 404:
		return "NotFoundError", true
	case 408, 504:
		return "Timeout", true
	case 429:
		return "RateLimitError", true
	case 500:
		return "InternalServerError", true
	case 502:
		return "BadGatewayError", true
	case 503:
		return "ServiceUnavailableError", true
	default:
		return "", false
	}
}
