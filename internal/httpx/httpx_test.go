package httpx

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
)

func TestWriteTypedErrorEnvelopes(t *testing.T) {
	t.Parallel()
	cases := []struct {
		path   string
		status int
		kind   string
	}{
		{"/v1/chat/completions", 401, "openai"},
		{"/v1/embeddings", 429, "openai"},
		{"/v1/messages", 401, "anthropic"},
		{"/v1/messages/count_tokens", 400, "anthropic"},
		{"/v1/threads/th_1/messages", 401, "openai"},
		{"/v1beta/models/gemini-pro:generateContent", 401, "google"},
		{"/models/gemini-pro:streamGenerateContent", 429, "google"},
		{"/v1beta/models/gemini-pro:countTokens", 400, "google"},
		{"/budget/new", 401, "openai"},
	}
	for _, c := range cases {
		rec := httptest.NewRecorder()
		WriteTypedError(rec, c.path, c.status, "invalid_api_key", "no key")
		if rec.Code != c.status {
			t.Fatalf("%s status %d", c.path, rec.Code)
		}
		if rec.Header().Get("x-litellm-call-id") == "" {
			t.Fatalf("%s missing call-id", c.path)
		}
		if rec.Header().Get("Content-Type") == "" {
			t.Fatalf("%s missing content-type", c.path)
		}
		if c.status == 429 && rec.Header().Get("Retry-After") == "" {
			t.Fatalf("%s missing Retry-After", c.path)
		}
		var m map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
			t.Fatalf("%s json %s", c.path, rec.Body.String())
		}
		switch c.kind {
		case "anthropic":
			if m["type"] != "error" {
				t.Fatalf("%s want type=error got %s", c.path, rec.Body.String())
			}
			errObj, _ := m["error"].(map[string]any)
			if errObj["type"] == nil || errObj["message"] == nil {
				t.Fatalf("%s anthropic envelope %s", c.path, rec.Body.String())
			}
		case "google":
			errObj, _ := m["error"].(map[string]any)
			if errObj["code"] == nil || errObj["message"] == nil || errObj["status"] == nil {
				t.Fatalf("%s google envelope %s", c.path, rec.Body.String())
			}
			if _, hasType := m["type"]; hasType {
				t.Fatalf("%s google should not have top-level type %s", c.path, rec.Body.String())
			}
		default:
			errObj, _ := m["error"].(map[string]any)
			if errObj["message"] == nil || errObj["type"] == nil {
				t.Fatalf("%s openai envelope %s", c.path, rec.Body.String())
			}
			if _, ok := errObj["param"]; !ok {
				t.Fatalf("%s openai missing param %s", c.path, rec.Body.String())
			}
			if _, ok := errObj["code"]; !ok {
				t.Fatalf("%s openai missing code %s", c.path, rec.Body.String())
			}
		}
	}
}
