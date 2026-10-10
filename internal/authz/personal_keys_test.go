package authz

import (
	"context"
	"testing"

	"github.com/sunqirui1987/xhub/internal/iam"
)

// TestTeamlessKeyCreatePolicy 验证创建权限矩阵；前置内存会话身份和不同归属输入。
// 参数 t 为测试上下文，返回无；仅本人或平台管理员可代建个人密钥，服务和项目必须绑定团队；无需清理。
func TestTeamlessKeyCreatePolicy(t *testing.T) {
	g := &Guard{actor: Actor{Kind: KindSession, UserID: "me"}}
	for _, tc := range []struct {
		owner, user, project string
		admin, allowed       bool
	}{
		{iam.OwnerPersonal, "me", "", false, true},
		{iam.OwnerPersonal, "other", "", false, false},
		{iam.OwnerPersonal, "other", "", true, true},
		{iam.OwnerPersonal, "", "", true, false},
		{iam.OwnerService, "", "", true, false},
		{iam.OwnerPersonal, "me", "project", true, false},
		{"unknown", "me", "", true, false},
	} {
		err := g.decideKeyCreate(context.Background(), Object{OwnerType: tc.owner, OwnerUserID: tc.user, ProjectID: tc.project}, tc.admin)
		if (err == nil) != tc.allowed {
			t.Fatalf("无团队创建权限错误: %+v err=%v", tc, err)
		}
	}
}

// TestTeamlessKeyLiveAuthorization 验证独立密钥实时授权；前置隔离数据库和无团队账号。
// 参数 t 为测试上下文，返回无；确认推理和本人读取可用、密钥不能管理、停用或删除立即拒绝；fixture 清理 schema。
func TestTeamlessKeyLiveAuthorization(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	k, _, err := f.db.CreateKey(ctx, f.sys(), iam.KeyInput{OwnerType: iam.OwnerPersonal, UserID: f.solo.ID})
	if err != nil {
		t.Fatal(err)
	}
	a := Actor{Kind: KindKey, KeyID: k.ID, UserID: f.solo.ID, OwnerType: iam.OwnerPersonal}
	g := f.guard(t, a)
	if err := g.CheckKey(ctx); err != nil {
		t.Fatalf("独立密钥应可用: %v", err)
	}
	if err := g.keyOnly(ctx, ActionInfer, Object{}); err != nil {
		t.Fatalf("个人密钥推理被拒绝: %v", err)
	}
	if err := g.keyOnly(ctx, ActionKeyCreate, Object{}); err == nil {
		t.Fatal("虚拟密钥获得管理权限")
	}
	for _, bad := range []Actor{{Kind: KindKey, OwnerType: iam.OwnerService}, {Kind: KindKey, OwnerType: iam.OwnerPersonal}} {
		if err := (&Guard{actor: bad}).keyOnly(ctx, ActionInfer, Object{}); err == nil {
			t.Fatalf("不完整独立密钥可推理: %+v", bad)
		}
	}
	if got := f.decide(t, session(f.solo), ActionKeyRead, Object{Type: ObjectKey, ID: k.ID}); got != "allow" {
		t.Fatalf("本人不能读独立密钥: %s", got)
	}
	if _, err := f.db.AdminUpdateUser(ctx, f.sys(), f.solo.ID, iam.UserUpdate{Status: ptr(iam.StatusDisabled)}); err != nil {
		t.Fatal(err)
	}
	if err := g.CheckKey(ctx); err == nil {
		t.Fatal("停用用户的个人密钥仍有效")
	}
	if err := f.db.DeleteKey(ctx, f.sys(), k.ID); err != nil {
		t.Fatal(err)
	}
	if err := g.CheckKey(ctx); err == nil {
		t.Fatal("删除后密钥仍有效")
	}
}
