package auth

import (
	"net/http/httptest"
	"testing"
)

// TestGoogleNativeKey 验证 Google 原厂凭据只在原生入口读取；参数 t 为上下文，验证优先级和空值，无持久数据。
func TestGoogleNativeKey(t *testing.T) {
	for _, tc := range []struct{ path, google, bearer, want string }{
		{"/v1beta/models/demo:generateContent?key=query", "", "", "query"},
		{"/vertex/v1/models/demo:countTokens?key=query", "header", "", "header"},
		{"/v1beta/models/demo:generateContent?key=query", "header", "session", "session"},
		{"/v1/chat/completions?key=query", "header", "", ""},
		{"/v1beta/models/demo:generateContent?key=%20", "", "", ""},
	} {
		r := httptest.NewRequest("POST", tc.path, nil)
		r.Header.Set("x-goog-api-key", tc.google)
		if tc.bearer != "" {
			r.Header.Set("Authorization", "Bearer "+tc.bearer)
		}
		if got := APIKeyFrom(r); got != tc.want {
			t.Errorf("%s: 密钥=%q 预期=%q", tc.path, got, tc.want)
		}
	}
}
