package suite

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sunqirui1987/xhub/internal/gateway/module"
	"github.com/sunqirui1987/xhub/internal/httpx"
)

type labModule struct{}

func (labModule) Name() string { return "lab-extra" }

func (labModule) Mount(reg module.Registrar) {
	reg.Handle("GET /lab/ping", func(w http.ResponseWriter, r *http.Request) {
		httpx.WriteJSON(w, 200, map[string]any{"ext": "lab-extra"})
	})
}

func TestUseMountsModuleTheProcessDoesNotName(t *testing.T) {
	s, _ := testEnv(t)
	names := s.ModuleNames()
	for _, n := range names {
		if n == "lab-extra" {
			t.Fatal("builtin list contains the test module")
		}
	}
	if err := s.Use(labModule{}); err != nil {
		t.Fatal(err)
	}
	if err := s.Use(labModule{}); err == nil {
		t.Fatal("duplicate module registered")
	}
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/lab/ping", nil))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "lab-extra") {
		t.Fatalf("mounted %d %s", rec.Code, rec.Body.String())
	}

	plain, _ := testEnv(t)
	rec = httptest.NewRecorder()
	plain.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/lab/ping", nil))
	if rec.Code != 404 {
		t.Fatalf("unregistered module status %d %s", rec.Code, rec.Body.String())
	}
}
