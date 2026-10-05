package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/sunqirui1987/xhub/internal/iam"
)

// testIdentityStore opens the identity store on a private schema for one test.
//
// A private schema matters more here than elsewhere: these tests write real
// accounts and real usage rows, and the gateway they start reads those same
// tables. Running them against the shared schema would let one test see
// another's rows, and would leave the developer's own accounts behind.
func testIdentityStore(t *testing.T) *iam.DB {
	t.Helper()
	dsn := os.Getenv("XHUB_TEST_DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://xhub:xhub_dev_password@127.0.0.1:5433/xhub?sslmode=disable"
	}
	u, err := url.Parse(dsn)
	if err != nil {
		t.Skipf("test database url unusable: %v", err)
	}
	schema := fmt.Sprintf("gateway_test_%d", time.Now().UnixNano())
	ctx := context.Background()

	root, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Skipf("no test database at %s: %v", dsn, err)
	}
	if _, err := root.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		root.Close(ctx)
		t.Fatalf("create schema: %v", err)
	}
	root.Close(ctx)

	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	db, err := iam.Open(ctx, u.String())
	if err != nil {
		t.Fatalf("open iam: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
		c, err := pgx.Connect(context.Background(), dsn)
		if err != nil {
			return
		}
		defer c.Close(context.Background())
		_, _ = c.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE")
	})
	return db
}

// adminSession creates a platform administrator and signs it in through the
// console's own login route, returning the session credential.
//
// Management routes cannot be reached with the master key: that credential is
// limited to bootstrap and emergency administration, so a test that wants to
// manage something has to be an authenticated administrator.
func adminSession(t *testing.T, base string, db *iam.DB) string {
	t.Helper()
	const (
		email    = "test-admin@example.com"
		password = "password123"
	)
	ctx := context.Background()
	if _, err := db.CreateUser(ctx, iam.Actor{Kind: "system"}, iam.UserInput{
		Email: email, Name: "Test Admin", Password: password, Role: iam.RoleAdmin,
	}); err != nil {
		t.Fatalf("create admin: %v", err)
	}
	payload, _ := json.Marshal(map[string]string{"username": email, "password": password})
	status, body := authed(t, base, "", http.MethodPost, "/v2/login", payload)
	if status != http.StatusOK {
		t.Fatalf("admin login %d %s", status, trim(body))
	}
	var parsed struct {
		Key string `json:"key"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil || parsed.Key == "" {
		t.Fatalf("admin login key: %s", trim(body))
	}
	return parsed.Key
}
