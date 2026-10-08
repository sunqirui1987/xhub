package identity

import (
	"context"
	"testing"

	"github.com/sunqirui1987/xhub/internal/iam"
	"xorm.io/xorm"
)

// A failed console lookup must not be converted into the platform fallback.
// That would show an operator an apparently valid effective route while the
// database was unavailable.
func TestRequestLookupRetainsDatabaseErrors(t *testing.T) {
	engine, err := xorm.NewEngine("pgx", "postgres://127.0.0.1:1/unavailable")
	if err != nil {
		t.Fatalf("create test engine: %v", err)
	}
	defer engine.Close()

	lookup := &requestLookup{
		db:    &iam.DB{Engine: engine},
		ctx:   context.Background(),
		byID:  map[string]*iam.RouteTemplate{},
		bound: map[string]string{},
	}
	if got := lookup.TemplateFor("organization", "org"); got != "" {
		t.Fatalf("failed scope lookup returned %q", got)
	}
	if lookup.err == nil {
		t.Fatal("failed scope lookup was swallowed")
	}

	lookup.err = nil
	if got := lookup.Load("template"); got != nil {
		t.Fatalf("failed template lookup returned %#v", got)
	}
	if lookup.err == nil {
		t.Fatal("failed template lookup was swallowed")
	}
}
