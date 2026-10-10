package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/sunqirui1987/xhub/cmd/regression/testsupport"
	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/iam"
	"github.com/sunqirui1987/xhub/internal/store"
)

// testGatewayStores 为真实网关测试建立同一个私有 schema 中的身份库和配置库。
// 参数 t：测试上下文；返回配置、配置存储和身份存储，供 HTTP 测试启动网关。
// 调用：bootGateway、紧急凭据及日志测试；连接失败使测试失败或按测试数据库规则跳过。
// 副作用：覆盖开发配置中的数据库和 Redis；普通测试直接落库，避免共用热队列污染正在运行的服务。
// 清理：测试结束关闭两个连接池并删除私有 schema，不读取或修改 public 中的用户配置。
func testGatewayStores(t *testing.T) (*config.Config, *store.Store, *iam.DB) {
	t.Helper()
	cfg, err := config.Load(configPath(t))
	if err != nil {
		t.Fatal(err)
	}
	dsn := testsupport.Postgres(t, "gateway")
	cfg.GeneralSettings.DatabaseURL = dsn
	cfg.GeneralSettings.RedisURL = ""
	cfg.GeneralRaw["database_url"] = dsn
	cfg.GeneralRaw["redis_url"] = ""
	db, err := iam.Open(context.Background(), dsn)
	if err != nil {
		t.Fatalf("打开隔离身份库失败: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	st, err := store.Open(dsn)
	if err != nil {
		t.Fatalf("打开隔离配置库失败: %v", err)
	}
	t.Cleanup(func() { _ = st.Engine.Close() })
	return cfg, st, db
}

// TestGatewayStoresIsolateConfigurationAndSpend 验证 HTTP 测试不会连接开发库配置表或共享 Redis 花费队列。
// 参数 t 为测试上下文；前置可达 PostgreSQL，比较两个真实连接的 schema 并启动网关。
// 返回：无；两库必须处于同一非 public schema 且 Redis 被禁用，所有夹具在结束时自动删除。
func TestGatewayStoresIsolateConfigurationAndSpend(t *testing.T) {
	cfg, st, db := testGatewayStores(t)
	identity, err := db.Engine.QueryString("SELECT current_schema() AS schema")
	if err != nil {
		t.Fatal(err)
	}
	configuration, err := st.Engine.QueryString("SELECT current_schema() AS schema")
	if err != nil {
		t.Fatal(err)
	}
	if len(identity) != 1 || len(configuration) != 1 || identity[0]["schema"] == "public" || identity[0]["schema"] != configuration[0]["schema"] {
		t.Fatalf("身份和配置必须使用同一私有 schema: identity=%v configuration=%v", identity, configuration)
	}
	if cfg.GeneralSettings.RedisURL != "" || cfg.GeneralRaw["redis_url"] != "" || New(cfg, st, db).Live != nil {
		t.Fatal("普通网关测试不得使用开发环境 Redis")
	}
}

// testIdentityStore opens the identity store on a private schema for one test.
//
// A private schema matters more here than elsewhere: these tests write real
// accounts and real usage rows, and the gateway they start reads those same
// tables. Running them against the shared schema would let one test see
// another's rows, and would leave the developer's own accounts behind.
func testIdentityStore(t *testing.T) *iam.DB {
	t.Helper()
	db, err := iam.Open(context.Background(), testsupport.Postgres(t, "gateway"))
	if err != nil {
		t.Fatalf("open iam: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
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
