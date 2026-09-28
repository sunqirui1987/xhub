// Package httpx writes JSON bodies and error envelopes, and it stamps a call ID on the response. Inference paths use the provider's own error shape.
package httpx

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"github.com/sunqirui1987/xhub/internal/logx"
	"net/http"
	"strconv"
	"strings"
	"sync"
)

var logTraceOnceHttpx sync.Once

// CallID returns a new 32-character hexadecimal call identifier.
func CallID() string {
	logTraceOnceHttpx.Do(func() { logx.Trace("enter httpx.CallID") })

	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// SetCallID writes x-litellm-call-id. If a value is already set, the caller decides whether to overwrite it.
func SetCallID(w http.ResponseWriter, id string) {
	w.Header().Set("x-litellm-call-id", id)
}

// WriteError writes the gateway JSON error. Inference paths should call WriteTypedError so the provider envelope is used instead.
func WriteError(w http.ResponseWriter, status int, typ, msg string) {
	WriteTypedError(w, "", status, typ, msg)
}

// WriteTypedError picks the error envelope from the request path. Status 429 also sets Retry-After to 1. A missing call ID is filled in.
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

// isAnthropicMessagesPath reports whether the path uses the Anthropic error envelope instead of the OpenAI error object.
func isAnthropicMessagesPath(p string) bool {
	if strings.Contains(p, "chat") || strings.Contains(p, "/threads") {
		return false
	}
	return strings.Contains(p, "/messages")
}

// isGeminiNativePath reports whether the path uses the native Gemini error shape.
func isGeminiNativePath(p string) bool {
	return strings.Contains(p, "generatecontent") ||
		strings.Contains(p, "streamgeneratecontent") ||
		(strings.Contains(p, "counttokens") && !strings.Contains(p, "/messages"))
}

// googleRPCStatus maps an HTTP status code to a Google RPC status name.
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

// WriteJSON sets the status code and writes JSON. A later encoding failure does not change the status that was already sent.
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
