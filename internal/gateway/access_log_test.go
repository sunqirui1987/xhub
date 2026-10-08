package gateway

import (
	"bytes"
	"log"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

func TestHandlerLogsMethodPathStatusAndDuration(t *testing.T) {
	s := &Server{
		engine:     newEngine(),
		registered: map[string]struct{}{},
		idem:       map[string]*idemRec{},
	}
	s.Handle("GET /health/liveliness", s.healthLive)

	var buf bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(prev) })

	req := httptest.NewRequest(http.MethodGet, "/health/liveliness", nil)
	req.Header.Set("Authorization", "Bearer sk-local-master")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	line := buf.String()
	t.Log(line)
	want := regexp.MustCompile(`info GET /health/liveliness 200 \d+(\.\d+)?(ns|µs|μs|ms|s)`)
	if !want.MatchString(line) {
		t.Fatalf("log %q does not contain level, method, path, status, and duration", line)
	}
	enter := strings.Index(line, "trace process GET /health/liveliness step=enter")
	dispatch := strings.Index(line, "debug process GET /health/liveliness step=dispatch mode=buffered")
	route := strings.Index(line, "debug process GET /health/liveliness step=route pattern=GET /health/liveliness")
	health := strings.Index(line, "debug process GET /health/liveliness step=health check=liveliness")
	if enter < 0 || dispatch < enter || route < dispatch || health < route {
		t.Fatalf("gateway process log missing or out of order: %s", line)
	}
	if strings.Contains(line, "sk-local-master") || strings.Contains(strings.ToLower(line), "bearer") || strings.Contains(line, "sk-") {
		t.Fatalf("log leaked credentials: %s", line)
	}
}

func TestHandlerLogsErrorLineForFailure(t *testing.T) {
	s := &Server{
		engine:     newEngine(),
		registered: map[string]struct{}{},
		idem:       map[string]*idemRec{},
	}
	var buf bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(prev) })

	req := httptest.NewRequest(http.MethodGet, "/missing", nil)
	req.Header.Set("Authorization", "Bearer sk-local-master")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status %d", rec.Code)
	}
	line := buf.String()
	t.Log(line)
	if !regexp.MustCompile(`info GET /missing 404 \d+(\.\d+)?(ns|µs|μs|ms|s)`).MatchString(line) {
		t.Fatalf("access log missing: %s", line)
	}
	if !strings.Contains(line, "error GET /missing 404") || !strings.Contains(line, "Not Found") {
		t.Fatalf("error log missing: %s", line)
	}
	if strings.Contains(line, "sk-local-master") || strings.Contains(strings.ToLower(line), "bearer") || strings.Contains(line, "sk-") {
		t.Fatalf("log leaked credentials: %s", line)
	}
}
