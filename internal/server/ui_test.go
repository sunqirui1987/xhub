package server

import (
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"testing"
)

func TestDashboardRootRedirectsWithoutKey(t *testing.T) {
	s, _ := testEnv(t)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept", "text/html")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("code %d body %s", rec.Code, rec.Body.String())
	}
	if loc := rec.Header().Get("Location"); loc != "/ui/" {
		t.Fatalf("location %s", loc)
	}
}

func TestDashboardRootWithKeyStaysAPI(t *testing.T) {
	s, master := testEnv(t)
	rec := doJSON(t, s.Handler(), http.MethodGet, "/", master, nil)
	if rec.Code == http.StatusFound {
		t.Fatalf("authed GET / redirected to %s", rec.Header().Get("Location"))
	}
	if rec.Code != 200 {
		t.Fatalf("code %d %s", rec.Code, rec.Body.String())
	}
	ct := rec.Result().Header.Get("Content-Type")
	if !strings.Contains(ct, "json") {
		t.Fatalf("content-type %s", ct)
	}
}

func TestDashboardJSONAcceptWithoutKeyStaysAPI(t *testing.T) {
	s, _ := testEnv(t)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept", "application/json")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != 401 {
		t.Fatalf("code %d body %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "invalid_api_key") {
		t.Fatalf("body %s", rec.Body.String())
	}
}

func TestDashboardUIProxiesToConsole(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ui/login/" {
			t.Errorf("proxied path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte("<html><h1>Login</h1></html>"))
	}))
	t.Cleanup(upstream.Close)

	s, _ := testEnv(t)
	u, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	s.uiProxy = httputil.NewSingleHostReverseProxy(u)

	req := httptest.NewRequest(http.MethodGet, "/ui/login/?redirect_to=http://localhost:3000/", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("code %d body %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Login") {
		t.Fatalf("body %s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "invalid_api_key") {
		t.Fatalf("auth error leaked: %s", rec.Body.String())
	}
}

func TestDashboardUIUnavailableIsHTML(t *testing.T) {
	s, _ := testEnv(t)
	s.uiProxy = nil
	req := httptest.NewRequest(http.MethodGet, "/ui/login/", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("code %d body %s", rec.Code, rec.Body.String())
	}
	ct := rec.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/html") {
		t.Fatalf("content-type %s", ct)
	}
	if strings.Contains(rec.Body.String(), "invalid_api_key") {
		t.Fatalf("still json auth error: %s", rec.Body.String())
	}
}

func TestDashboardLoginAliasRedirects(t *testing.T) {
	s, _ := testEnv(t)
	req := httptest.NewRequest(http.MethodGet, "/login?redirect_to=%2F", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("code %d body %s", rec.Code, rec.Body.String())
	}
	if loc := rec.Header().Get("Location"); loc != "/ui/login/?redirect_to=%2F" {
		t.Fatalf("location %s", loc)
	}
}
