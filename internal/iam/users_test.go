package iam

import (
	"context"
	"errors"
	"testing"

	"github.com/sunqirui1987/xhub/cmd/regression/testsupport"
)

// TestAdminUpdateUserName 验证管理员名称增量更新；前置隔离数据库和普通账户，覆盖正常、空值、未指定及约束失败回滚。
// 参数 t 为测试上下文，无返回值；断言会话版本不变及不存在账户返回 ErrNotFound，testDB 自动清理整个 schema。
func TestAdminUpdateUserName(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	actor := Actor{Kind: "master"}
	u, err := db.CreateUser(ctx, actor, UserInput{Email: "edit@example.com", Name: "Before", Password: "edit-password"})
	if err != nil {
		t.Fatal(err)
	}
	name := "After"
	updated, err := db.AdminUpdateUser(ctx, actor, u.ID, UserUpdate{Name: &name})
	if err != nil || updated.Name != name || updated.SessionVersion != u.SessionVersion {
		t.Fatalf("名称更新或会话保留失败: %+v %v", updated, err)
	}
	name = ""
	updated, err = db.AdminUpdateUser(ctx, actor, u.ID, UserUpdate{Name: &name})
	if err != nil || updated.Name != "" {
		t.Fatalf("清空名称失败: %+v %v", updated, err)
	}
	updated, err = db.AdminUpdateUser(ctx, actor, u.ID, UserUpdate{})
	if err != nil || updated.Name != "" {
		t.Fatalf("未指定字段覆盖名称: %+v %v", updated, err)
	}
	name, invalidRole := "must rollback", "invalid"
	if _, err = db.AdminUpdateUser(ctx, actor, u.ID, UserUpdate{Name: &name, Role: &invalidRole}); err == nil {
		t.Fatal("非法角色未拒绝")
	}
	updated, err = db.GetUser(ctx, u.ID)
	if err != nil || updated.Name != "" {
		t.Fatalf("失败更新未回滚名称: %+v %v", updated, err)
	}
	if _, err = db.AdminUpdateUser(ctx, actor, "missing", UserUpdate{Name: &name}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("不存在账户错误: %v", err)
	}
}

// testDB opens the identity store on a private schema, so a test never touches
// the tables a running gateway is using and never has to clean up after itself.
func testDB(t *testing.T) *DB {
	t.Helper()
	db, err := Open(context.Background(), testsupport.Postgres(t, "iam"))
	if err != nil {
		t.Fatalf("open iam: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// TestEnsureAdminCreatesTheConfiguredAccount covers the first start of a
// deployment that names an administrator in configuration.
func TestEnsureAdminCreatesTheConfiguredAccount(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	created, err := db.EnsureAdmin(ctx, "Ops@Example.com", "", "initial-password")
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if !created {
		t.Fatal("the first call must create the account")
	}
	u, err := db.UserByEmail(ctx, "ops@example.com")
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if u.Role != RoleAdmin {
		t.Fatalf("role: got %q want %q", u.Role, RoleAdmin)
	}
	if u.Status != StatusActive {
		t.Fatalf("status: got %q", u.Status)
	}
	// The address is normalised and the missing name is derived from it, so the
	// console has something to show without a second config key.
	if u.Email != "ops@example.com" || u.Name != "ops" {
		t.Fatalf("email/name: %q %q", u.Email, u.Name)
	}
	if _, err := db.Login(ctx, "ops@example.com", "initial-password"); err != nil {
		t.Fatalf("the configured password must sign in: %v", err)
	}
	// The console username box often gets the display name or the mailbox
	// name, not the full address. Both have to reach the same account.
	if _, err := db.Login(ctx, "ops", "initial-password"); err != nil {
		t.Fatalf("display name must sign in: %v", err)
	}
	if _, err := db.Login(ctx, "OPS", "initial-password"); err != nil {
		t.Fatalf("email local part must sign in: %v", err)
	}
}

func TestConsoleRoleIsWhatTheAdminUIReads(t *testing.T) {
	if ConsoleRole(RoleAdmin) != "proxy_admin" {
		t.Fatalf("admin console role: %q", ConsoleRole(RoleAdmin))
	}
	if StoreRole("proxy_admin") != RoleAdmin || StoreRole("internal_user") != RoleUser {
		t.Fatal("console roles must map back onto the stored spelling")
	}
}

// TestEnsureAdminNeverRewritesAnExistingAccount is the property that makes the
// configured password an initial password rather than a managed one: changing
// the value in a config file must not reset a live account.
func TestEnsureAdminNeverRewritesAnExistingAccount(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	if _, err := db.EnsureAdmin(ctx, "admin@example.com", "Admin", "first-password"); err != nil {
		t.Fatalf("first seed: %v", err)
	}
	before, err := db.UserByEmail(ctx, "admin@example.com")
	if err != nil {
		t.Fatalf("read back: %v", err)
	}

	created, err := db.EnsureAdmin(ctx, "admin@example.com", "Renamed", "second-password")
	if err != nil {
		t.Fatalf("second seed: %v", err)
	}
	if created {
		t.Fatal("an existing account must not be recreated")
	}
	after, err := db.UserByEmail(ctx, "admin@example.com")
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if after.ID != before.ID || after.PasswordHash != before.PasswordHash || after.Name != before.Name {
		t.Fatal("an existing account was rewritten by the seeding pass")
	}
	if _, err := db.Login(ctx, "admin@example.com", "first-password"); err != nil {
		t.Fatalf("the original password must still work: %v", err)
	}
	if _, err := db.Login(ctx, "admin@example.com", "second-password"); err == nil {
		t.Fatal("the newly configured password must not have been applied")
	}
}

// TestEnsureAdminIsANoOpWhenUnconfigured pins the three states that leave
// seeding off. None of them is an error: an operator who configures nothing
// still gets POST /bootstrap.
func TestEnsureAdminIsANoOpWhenUnconfigured(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	for _, tc := range []struct{ name, email, password string }{
		{"no email", "", "password-1234"},
		{"no password", "admin@example.com", ""},
		{"neither", "", ""},
	} {
		created, err := db.EnsureAdmin(ctx, tc.email, "", tc.password)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if created {
			t.Fatalf("%s: created an account from an incomplete configuration", tc.name)
		}
	}
	if done, err := db.Bootstrapped(ctx); err != nil || done {
		t.Fatalf("an unconfigured start must leave the platform uninitialised: done=%v err=%v", done, err)
	}
}

// TestEnsureAdminMarksThePlatformInitialised checks the marker the console reads:
// once a configured administrator exists, the setup form must not be offered,
// and POST /bootstrap must not be able to add a second first administrator.
func TestEnsureAdminMarksThePlatformInitialised(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	if done, err := db.Bootstrapped(ctx); err != nil || done {
		t.Fatalf("fresh schema must be uninitialised: done=%v err=%v", done, err)
	}
	if _, err := db.EnsureAdmin(ctx, "admin@example.com", "Admin", "initial-password"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if done, err := db.Bootstrapped(ctx); err != nil || !done {
		t.Fatalf("seeding must mark the platform initialised: done=%v err=%v", done, err)
	}
	if _, err := db.Bootstrap(ctx, "other@example.com", "Other", "another-password"); err == nil {
		t.Fatal("bootstrap must refuse to run after a configured administrator exists")
	}
}

// TestEnsureAdminRejectsAShortPassword checks that a mistyped configuration is
// reported rather than turned into an account nobody can sign in to.
func TestEnsureAdminRejectsAShortPassword(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	created, err := db.EnsureAdmin(ctx, "admin@example.com", "Admin", "short")
	if err == nil {
		t.Fatal("a password below the minimum length must be refused")
	}
	if created {
		t.Fatal("a refused password must not create an account")
	}
	if _, err := db.UserByEmail(ctx, "admin@example.com"); err == nil {
		t.Fatal("no account may exist after a refused password")
	}
}
