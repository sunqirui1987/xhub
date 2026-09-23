package httpx

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

func CallID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

func SetCallID(w http.ResponseWriter, id string) {
	w.Header().Set("x-litellm-call-id", id)
}

func WriteError(w http.ResponseWriter, status int, typ, msg string) {
	WriteTypedError(w, "", status, typ, msg)
}

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

func isAnthropicMessagesPath(p string) bool {
	if strings.Contains(p, "chat") || strings.Contains(p, "/threads") {
		return false
	}
	return strings.Contains(p, "/messages")
}

func isGeminiNativePath(p string) bool {
	return strings.Contains(p, "generatecontent") ||
		strings.Contains(p, "streamgeneratecontent") ||
		(strings.Contains(p, "counttokens") && !strings.Contains(p, "/messages"))
}

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
