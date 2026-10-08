package auth

import (
	"encoding/base64"
	"net/http/httptest"
	"testing"
)

func TestBasicCredentialTrimsHeaderBeforeSlicing(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Authorization", "  Basic "+base64.StdEncoding.EncodeToString([]byte("user:secret"))+"  ")
	if got := APIKeyFrom(r); got != "secret" {
		t.Fatalf("credential=%q", got)
	}
	r.Header.Set("x-litellm-api-key", "Bearer preferred")
	if got := APIKeyFrom(r); got != "preferred" {
		t.Fatalf("precedence=%q", got)
	}
}
