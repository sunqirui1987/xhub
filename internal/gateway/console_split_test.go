package gateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGatewayDoesNotHostTheConsole(t *testing.T) {
	t.Setenv("XHUB_PUBLIC_ORIGIN", "")
	s := &Server{
		engine:     newEngine(),
		registered: map[string]struct{}{},
		idem:       map[string]*idemRec{},
	}
	s.Handle("GET /health/liveliness", s.healthLive)
	s.Handle("GET /litellm/.well-known/litellm-ui-config", s.uiConfig)

	pages := []string{"/ui", "/ui/", "/ui/login/", "/login", "/login/", "/", "/_next/static/app.js", "/assets/logo.svg", "/favicon.ico"}
	for _, path := range pages {
		for _, method := range []string{http.MethodGet, http.MethodHead} {
			req := httptest.NewRequest(method, path, nil)
			req.Header.Set("Accept", "text/html,application/xhtml+xml")
			rec := httptest.NewRecorder()
			s.Handler().ServeHTTP(rec, req)
			if rec.Code >= 300 && rec.Code < 400 {
				loc := rec.Header().Get("Location")
				if strings.HasPrefix(loc, "/ui") || strings.Contains(loc, "/ui/") || strings.Contains(loc, "/ui?") {
					t.Fatalf("%s %s redirected to console path %q", method, path, loc)
				}
			}
			body := rec.Body.String()
			if strings.Contains(strings.ToLower(body), "<html") || strings.Contains(body, "/ui/login") {
				t.Fatalf("%s %s returned console HTML: %s", method, path, body)
			}
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/health/liveliness", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"status"`) {
		t.Fatalf("health status %d body %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/litellm/.well-known/litellm-ui-config", nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("ui-config status %d body %s", rec.Code, rec.Body.String())
	}
	var doc map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	base, _ := doc["proxy_base_url"].(string)
	if base != LocalGatewayOrigin || strings.Contains(base, "/ui") {
		t.Fatalf("proxy_base_url = %q", base)
	}
}

func TestPublicOriginOverride(t *testing.T) {
	const configured = "https://gateway.example.com"
	t.Setenv("XHUB_PUBLIC_ORIGIN", configured)
	s := &Server{
		engine:     newEngine(),
		registered: map[string]struct{}{},
		idem:       map[string]*idemRec{},
	}
	s.Handle("GET /litellm/.well-known/litellm-ui-config", s.uiConfig)
	req := httptest.NewRequest(http.MethodGet, "/litellm/.well-known/litellm-ui-config", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	var doc map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	base, _ := doc["proxy_base_url"].(string)
	if base != configured || strings.Contains(base, "/ui") {
		t.Fatalf("proxy_base_url = %q", base)
	}
}
