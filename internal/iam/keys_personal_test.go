package iam

import (
	"context"
	"testing"
)

// TestTeamlessPersonalKeyStorage 验证独立个人密钥持久化和数据库约束；前置隔离 PostgreSQL 与无团队账号。
// 参数 t 为测试上下文，返回无；确认团队列为 NULL、迁移幂等、更新轮换正常、非法归属回滚；testDB 清理专属 schema。
func TestTeamlessPersonalKeyStorage(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	u, err := db.CreateUser(ctx, Actor{}, UserInput{Email: "solo@example.com", Password: "password123", Role: RoleUser})
	if err != nil {
		t.Fatal(err)
	}
	by := Actor{ID: u.ID, Kind: "session"}
	k, secret, err := db.CreateKey(ctx, by, KeyInput{OwnerType: OwnerPersonal, UserID: u.ID, Name: "solo"})
	if err != nil {
		t.Fatalf("无团队创建失败: %v", err)
	}
	if k.TeamID != "" || k.UserID == nil || *k.UserID != u.ID || k.TokenHash != HashKey(secret) {
		t.Fatalf("个人归属或哈希错误: %+v", k)
	}
	rows, err := db.Engine.QueryString("SELECT team_id IS NULL AS unbound FROM api_keys WHERE id = ?", k.ID)
	if err != nil || len(rows) != 1 || rows[0]["unbound"] != "true" {
		t.Fatalf("团队列必须存储 NULL: %v %v", rows, err)
	}
	for i := 0; i < 2; i++ {
		if err := db.Migrate(ctx); err != nil {
			t.Fatalf("迁移重复运行失败: %v", err)
		}
	}
	budget := 10.0
	updated, err := db.UpdateKey(ctx, by, k.ID, KeyInput{Name: "updated", Models: []string{"chat"}, MaxBudget: &budget})
	if err != nil || updated.TeamID != "" || updated.Name != "updated" {
		t.Fatalf("独立个人密钥更新失败: %+v %v", updated, err)
	}
	if updated.MaxBudget == nil || *updated.MaxBudget != budget {
		t.Fatal("无团队个人密钥无法设置额度")
	}
	if _, _, err := db.CreateKey(ctx, by, KeyInput{OwnerType: OwnerPersonal, UserID: u.ID, MaxBudget: &budget}); err != nil {
		t.Fatalf("带额度的个人密钥创建失败: %v", err)
	}
	rotated, next, err := db.RotateKey(ctx, by, k.ID)
	if err != nil || next == secret || rotated.TeamID != "" {
		t.Fatalf("轮换改变团队或未生成新密钥: %v", err)
	}
	for _, in := range []KeyInput{
		{OwnerType: OwnerService}, {OwnerType: OwnerPersonal},
		{OwnerType: OwnerPersonal, UserID: u.ID, ProjectID: "project"},
		{OwnerType: "unknown", UserID: u.ID},
		{OwnerType: OwnerPersonal, UserID: "missing"},
	} {
		if _, _, err := db.CreateKey(ctx, by, in); err == nil {
			t.Fatalf("非法归属被持久化: %+v", in)
		}
	}
	if _, err := db.Engine.Exec("UPDATE api_keys SET owner_type = 'service', user_id = NULL WHERE id = ?", k.ID); err == nil {
		t.Fatal("数据库允许无团队服务密钥")
	}
	status := StatusDisabled
	if _, err := db.AdminUpdateUser(ctx, by, u.ID, UserUpdate{Status: &status}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := db.CreateKey(ctx, by, KeyInput{OwnerType: OwnerPersonal, UserID: u.ID}); err != ErrInactive {
		t.Fatalf("停用用户创建结果: %v", err)
	}
}
