// 写 JSON 和错误信封，并给响应带上调用 ID。推理路径用厂商各自的错误形状。
package httpx

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

// 生成 32 位十六进制调用 ID。
func CallID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// 设置 x-litellm-call-id。已有值时由调用方决定是否覆盖。
func SetCallID(w http.ResponseWriter, id string) {
	w.Header().Set("x-litellm-call-id", id)
}

// 写通用 JSON 错误。推理路径应改用 WriteTypedError，以便换成厂商信封。
func WriteError(w http.ResponseWriter, status int, typ, msg string) {
	WriteTypedError(w, "", status, typ, msg)
}

// 按路径选择错误信封。429 会带 Retry-After: 1。没有调用 ID 时补一个。
func WriteTypedError(w http.ResponseWriter, path string, status int, typ, msg string) {
	if w.Header().Get("x-litellm-call-id") == "" {
		SetCallID(w, CallID())
	}
	if w.Header().Get("x-litellm-version") == "" {
		w.Header().Set("x-litellm-version", "xhub-dev")
	}
	w.Header().Set("Content-Type", "application/json")
	if status == http.StatusTooManyRequests {
		w.Header().Set("Retry-After", "1")
	}
	w.WriteHeader(status)
	p := strings.ToLower(path)
	switch {
	case isAnthropicMessagesPath(p):
		_ = json.NewEncoder(w).Encode(map[string]any{
			"type": "error",
			"error": map[string]any{
				"type":    typ,
				"message": msg,
			},
		})
	case isGeminiNativePath(p):
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{
				"code":    status,
				"message": msg,
				"status":  googleRPCStatus(status),
			},
		})
	default:
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{
				"message": msg,
				"type":    typ,
				"param":   nil,
				"code":    strconv.Itoa(status),
			},
		})
	}
}

// 路径是否走 Anthropic 错误信封，而不是 OpenAI 的 error 对象。
func isAnthropicMessagesPath(p string) bool {
	if strings.Contains(p, "chat") || strings.Contains(p, "/threads") {
		return false
	}
	return strings.Contains(p, "/messages")
}

// 路径是否走 Gemini 原生错误形状。
func isGeminiNativePath(p string) bool {
	return strings.Contains(p, "generatecontent") ||
		strings.Contains(p, "streamgeneratecontent") ||
		(strings.Contains(p, "counttokens") && !strings.Contains(p, "/messages"))
}

// 把 HTTP 状态码映射成 Google RPC 状态名。
func googleRPCStatus(code int) string {
	switch code {
	case 401:
		return "UNAUTHENTICATED"
	case 403:
		return "PERMISSION_DENIED"
	case 404:
		return "NOT_FOUND"
	case 429:
		return "RESOURCE_EXHAUSTED"
	case 400:
		return "INVALID_ARGUMENT"
	case 502, 503:
		return "UNAVAILABLE"
	default:
		if code >= 500 {
			return "INTERNAL"
		}
		return "INVALID_ARGUMENT"
	}
}

// 写 JSON 并设置状态码。编码失败时不再改状态码。
func WriteJSON(w http.ResponseWriter, status int, v any) {
	if w.Header().Get("x-litellm-call-id") == "" {
		SetCallID(w, CallID())
	}
	if w.Header().Get("x-litellm-version") == "" {
		w.Header().Set("x-litellm-version", "xhub-dev")
	}
	if w.Header().Get("Content-Type") == "" {
		w.Header().Set("Content-Type", "application/json")
	}
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
