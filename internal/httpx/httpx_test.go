package httpx

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWriteTypedErrorUsesProviderEnvelope(t *testing.T) {
	tests := []struct {
		name  string
		path  string
		check func(*testing.T, map[string]any)
	}{
		{name: "openai", path: "/v1/chat/completions", check: func(t *testing.T, body map[string]any) {
			err := nestedMap(t, body, "error")
			if err["type"] != "rate_limit" || err["code"] != "429" {
				t.Fatalf("OpenAI error = %#v", err)
			}
		}},
		{name: "anthropic", path: "/v1/messages", check: func(t *testing.T, body map[string]any) {
			if body["type"] != "error" || nestedMap(t, body, "error")["type"] != "rate_limit" {
				t.Fatalf("Anthropic error = %#v", body)
			}
		}},
		{name: "gemini", path: "/v1beta/models/gemini:generateContent", check: func(t *testing.T, body map[string]any) {
			err := nestedMap(t, body, "error")
			if err["status"] != "RESOURCE_EXHAUSTED" || err["code"] != float64(429) {
				t.Fatalf("Gemini error = %#v", err)
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			WriteTypedError(rr, tt.path, http.StatusTooManyRequests, "rate_limit", "slow down")
			if rr.Code != http.StatusTooManyRequests || rr.Header().Get("Retry-After") != "1" {
				t.Fatalf("status/header = %d/%q", rr.Code, rr.Header().Get("Retry-After"))
			}
			if len(rr.Header().Get("x-litellm-call-id")) != 32 || rr.Header().Get("x-litellm-version") == "" {
				t.Fatalf("missing gateway headers: %#v", rr.Header())
			}
			var body map[string]any
			if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			tt.check(t, body)
		})
	}
}

func TestWriteJSONPreservesExistingHeaders(t *testing.T) {
	rr := httptest.NewRecorder()
	rr.Header().Set("Content-Type", "application/problem+json")
	rr.Header().Set("x-litellm-call-id", "existing")
	rr.Header().Set("x-litellm-version", "existing-version")
	WriteJSON(rr, http.StatusAccepted, map[string]bool{"ok": true})
	if rr.Code != http.StatusAccepted || rr.Header().Get("Content-Type") != "application/problem+json" ||
		rr.Header().Get("x-litellm-call-id") != "existing" || rr.Header().Get("x-litellm-version") != "existing-version" {
		t.Fatalf("response = status %d headers %#v", rr.Code, rr.Header())
	}
}

func nestedMap(t *testing.T, body map[string]any, key string) map[string]any {
	t.Helper()
	got, ok := body[key].(map[string]any)
	if !ok {
		t.Fatalf("%q = %#v, want object", key, body[key])
	}
	return got
}
