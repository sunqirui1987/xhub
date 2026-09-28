// Package llm turns a call into an upstream URL, headers, and body, and maps provider responses back to the public shape. This file only maps HTTP status codes and does not send a request.
package llm

// ExceptionForStatus maps an upstream HTTP status to a LiteLLM exception class name.
// Status 400 and 422 are BadRequestError. Status 408 and 504 are Timeout.
// A status without its own branch returns ok false so the caller treats it as an unknown upstream error.
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
