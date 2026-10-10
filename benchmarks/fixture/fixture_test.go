package fixture

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/sunqirui1987/xhub/benchmarks/load"
)

// TestOpenInvalid 验证配置错误与不可达数据库不会返回半初始化资源；本地拒绝地址，错误脱敏，无需清理。
func TestOpenInvalid(t *testing.T) {
	for _, dsn := range []string{"file:///tmp/db", "postgres://", "postgres://u:secret@127.0.0.1:1/db?connect_timeout=1"} {
		if e, err := Open(context.Background(), dsn, 0); err == nil || e != nil || strings.Contains(err.Error(), "secret") {
			t.Fatalf("失败路径未脱敏或泄漏资源: %v", err)
		}
	}
	if _, err := Open(context.Background(), DefaultDatabaseURL, -time.Second); err == nil {
		t.Fatal("负延迟未拒绝")
	}
}

// TestJSONErrors 验证夹具 HTTP 正常、错误状态、错误正文和编码失败；本地服务器，结束自动关闭，不回显敏感响应。
func TestJSONErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ok":
			_, _ = w.Write([]byte("{\"ok\":true}"))
		case "/bad":
			_, _ = w.Write([]byte("broken"))
		default:
			http.Error(w, "secret", 401)
		}
	}))
	defer server.Close()
	if doc, err := JSON(context.Background(), server.URL, "/ok", "", nil); err != nil || doc["ok"] != true {
		t.Fatalf("正常 HTTP 失败: %v", err)
	}
	for _, path := range []string{"/bad", "/denied"} {
		if _, err := JSON(context.Background(), server.URL, path, "", nil); err == nil || strings.Contains(err.Error(), "secret") {
			t.Fatalf("错误未检出/脱敏: %v", err)
		}
	}
	if _, err := JSON(context.Background(), server.URL, "/ok", "", make(chan int)); err == nil {
		t.Fatal("无效 JSON 输入未拒绝")
	}
}

// TestEnvironmentRegression 验证真实网关四场景、用量费用、错误响应及 schema 删除；前置本地 PostgreSQL，无供应商凭据。
// 参数 t 为测试上下文；返回无；结束关闭环境并查询 schema 不存在，失败路径也由 Cleanup 回收。
func TestEnvironmentRegression(t *testing.T) {
	dsn := os.Getenv("XHUB_TEST_DATABASE_URL")
	if dsn == "" {
		dsn = DefaultDatabaseURL
	}
	ctx := context.Background()
	e, err := Open(ctx, dsn, time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := e.Close(); err != nil {
			t.Error(err)
		}
	})
	for _, scenario := range []string{"chat", "stream", "models", "auth-reject"} {
		c := load.Config{BaseURL: e.URL, Key: e.Key, Model: Model, Scenario: scenario, Concurrency: 4, Requests: 8, Duration: 5 * time.Second, Timeout: 5 * time.Second, PromptBytes: 16}
		result, err := load.Run(ctx, c)
		if err != nil || result.Attempted != 8 || result.Succeeded != 8 {
			t.Fatalf("真实场景 %s 回归: %+v err=%v", scenario, result, err)
		}
		if scenario == "stream" && result.TTFT.Samples != 8 {
			t.Fatalf("流式未采集首字: %+v", result)
		}
	}
	audit, err := e.Audit()
	if err != nil || audit["consistent"] != true || audit["upstream_calls"] != int64(16) || audit["total_tokens"] != int64(256) {
		t.Fatalf("真实数据面审计错误: %v err=%v", audit, err)
	}
	// 人为修改本次私有用量，验证费用审计能发现持久化损坏；不触及任何已有数据。
	if _, err = e.DB.Engine.Exec("UPDATE usage_events SET cost = cost + 1 WHERE model = ?", Model); err != nil {
		t.Fatal(err)
	}
	audit, err = e.Audit()
	if err != nil || audit["consistent"] != false {
		t.Fatalf("费用损坏未发现: %v err=%v", audit, err)
	}
	schema := e.Schema
	if err = e.Close(); err != nil {
		t.Fatal(err)
	}
	root, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close(ctx)
	var exists bool
	if err = root.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM pg_namespace WHERE nspname=$1)", schema).Scan(&exists); err != nil || exists {
		t.Fatalf("本次 schema 未清理: exists=%t err=%v", exists, err)
	}
}

// TestAuditUnavailableRegression 验证数据库查询失败仍保留审计失败证据；前置独立数据库夹具，手动关闭引擎模拟不可达。
// 参数 t 为测试上下文，断言非空审计且非一致；Cleanup 关闭 HTTP 并删除本次 schema。
func TestAuditUnavailableRegression(t *testing.T) {
	dsn := os.Getenv("XHUB_TEST_DATABASE_URL")
	if dsn == "" {
		dsn = DefaultDatabaseURL
	}
	e, err := Open(context.Background(), dsn, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := e.Close(); err != nil {
			t.Error(err)
		}
	})
	if err = e.DB.Engine.Close(); err != nil {
		t.Fatal(err)
	}
	audit, err := e.Audit()
	if err == nil || audit == nil || audit["consistent"] != false || audit["audit_complete"] != false {
		t.Fatalf("不可达审计丢失证据: %v %v", audit, err)
	}
}
