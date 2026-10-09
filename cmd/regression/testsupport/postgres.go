// Package testsupport holds the fixture plumbing shared by the Go test suites.
//
// It exists because the same block was copied into three packages: resolve a
// DSN, create a throwaway schema, point the connection's search_path at it, and
// drop it on cleanup. Three copies meant three places to fix when the default
// port moved, and it quietly made the suites skip for different reasons.
//
// The package deliberately imports neither iam nor gateway. Every consumer is a
// test inside one of those packages, so importing them back would be an import
// cycle, and the fixture does not need them: it hands back a DSN and lets the
// caller open whatever store it wants.
package testsupport

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// DefaultDatabaseURL is the local verification database from docker-compose.yml.
//
// Tests skip, rather than fail, when no database answers here. That keeps
// `go test ./...` usable on a machine without Docker, at the cost of a green
// run that proves less than it appears to; XHUB_TEST_DATABASE_URL overrides it
// when a real run is wanted.
const DefaultDatabaseURL = "postgres://xhub:xhub_dev_password@127.0.0.1:5433/xhub?sslmode=disable"

// DatabaseURL returns the DSN the suites should connect to.
// 参数：无。
// 返回 string（string）：测试套件连接 Postgres 的 DSN。环境没有配置时为空串，不能拿去建连。
// 调用：仅在 postgres.go 内使用
// 测试：无直接单测
func DatabaseURL() string {
	if dsn := os.Getenv("XHUB_TEST_DATABASE_URL"); dsn != "" {
		return dsn
	}
	return DefaultDatabaseURL
}

// Postgres returns a DSN whose search_path points at a schema created for this one test, and registers the cleanup that drops it. A private schema is what makes these suites safe to run against a database a gateway is also using: the test writes real accounts, real keys and real usage rows, and without the schema it would read the developer's own rows and leave its own behind. The prefix names the caller in the schema, so a schema leaked by a killed test run can be traced back to the package that made it. The test skips when the database is unreachable. It fails only when the database answers and the fixture cannot be built, because that is a real error rather than a missing dependency.
// 参数 t（*testing.T）：当前测试。库不可达时跳过，建夹具失败时让测试失败；prefix（string）：写进 schema 名的前缀，用来认出是哪个包留下的库。不是用来匹配或去掉的路径前缀。
// 返回 string（string）：search_path 指向本次测试 schema 的 DSN。测试结束时注册的清理会删掉这个 schema。
// 调用：仅在 postgres.go 内使用
// 测试：authz_test.go、guard_test.go、guardrail_block_test.go
func Postgres(t *testing.T, prefix string) string {
	t.Helper()
	dsn := DatabaseURL()
	u, err := url.Parse(dsn)
	if err != nil {
		t.Skipf("test database url unusable: %v", err)
	}
	schema := fmt.Sprintf("%s_test_%d", prefix, time.Now().UnixNano())
	ctx := context.Background()

	root, err := pgx.Connect(ctx, dsn)
	if err != nil {
		if os.Getenv("XHUB_REGRESSION_STRICT") == "1" {
			t.Fatalf("regression database is unreachable; check XHUB_TEST_DATABASE_URL or start PostgreSQL on 127.0.0.1:5433")
		}
		t.Skipf("test database is unreachable (%T)", err)
	}
	if _, err := root.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		root.Close(ctx)
		t.Fatalf("create schema: %v", err)
	}
	root.Close(ctx)

	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()

	t.Cleanup(func() {
		c, err := pgx.Connect(context.Background(), dsn)
		if err != nil {
			// The database went away mid-test. Nothing to drop it from.
			return
		}
		defer c.Close(context.Background())
		_, _ = c.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE")
	})
	return u.String()
}
