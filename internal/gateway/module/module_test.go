package module

import (
	"net/http"
	"testing"
)

type rec struct {
	patterns []string
}

func (r *rec) Handle(pattern string, h http.HandlerFunc) {
	r.patterns = append(r.patterns, pattern)
	if h == nil {
		panic("nil handler")
	}
}

func TestBindMountsInCallOrder(t *testing.T) {
	m := Bind("lab", func(reg Registrar) {
		reg.Handle("GET /lab", func(http.ResponseWriter, *http.Request) {})
		reg.Handle("POST /lab", func(http.ResponseWriter, *http.Request) {})
	})
	if m.Name() != "lab" {
		t.Fatalf("name %q", m.Name())
	}
	var got rec
	m.Mount(&got)
	if len(got.patterns) != 2 || got.patterns[0] != "GET /lab" || got.patterns[1] != "POST /lab" {
		t.Fatalf("patterns %v", got.patterns)
	}
	Bind("empty", nil).Mount(&got)
	if len(got.patterns) != 2 {
		t.Fatalf("nil mount wrote %v", got.patterns)
	}
}
