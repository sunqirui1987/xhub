package llm

// ExceptionForStatus 把上游 HTTP 状态收成 LiteLLM 的异常类名。
//
// 对应 exception_mapping_utils.py 里 OpenAI 这条状态分支：
// 400 和 422 都是 BadRequestError，408 和 504 都是 Timeout。
// 没有单独分支的状态返回 ok=false，调用方按未知上游错误处理，不要猜一个类名。
func ExceptionForStatus(status int) (name string, ok bool) {
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
