package gateway

import (
	"context"
	"errors"
	"testing"

	"github.com/sunqirui1987/xhub/internal/auth"
	"github.com/sunqirui1987/xhub/internal/iam"
)

// TestTeamlessKeyBudget 验证独立密钥额度边界；前置隔离数据库、无团队账号和个人密钥。
// 参数 t 为测试上下文，返回无；验证不限额、零额度、恢复、停用、删除和密钥路由模板；fixture 清理 schema。
func TestTeamlessKeyBudget(t *testing.T) {
	f := newPermFixture(t)
	ctx := context.Background()
	by := iam.Actor{ID: f.admin.user.ID, Kind: "session"}
	route, err := f.db.CreateRouteTemplate(ctx, by, iam.TemplateOwner{}, "personal-template", "{}")
	if err != nil {
		t.Fatal(err)
	}
	template := route.ID
	selection := &template
	k, _, err := f.db.CreateKey(ctx, by, iam.KeyInput{OwnerType: iam.OwnerPersonal, UserID: f.member.user.ID, RouteTemplateID: &selection})
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{IAM: f.db}
	p := &auth.Principal{KeyID: k.ID, Hash: k.TokenHash}
	if err := s.keyBudgetOK(ctx, p); err != nil {
		t.Fatalf("不限额个人密钥被拒绝: %v", err)
	}
	if p.RouteTemplateID != template || p.RouteTemplateSource != "key" || p.OrgID != "" {
		t.Fatalf("独立密钥路由归属错误: %+v", p)
	}
	zero := 0.0
	ceiling := &zero
	if _, err := f.db.AdminUpdateUser(ctx, by, f.member.user.ID, iam.UserUpdate{MaxBudget: &ceiling}); err != nil {
		t.Fatal(err)
	}
	if err := s.keyBudgetOK(ctx, p); err != (errBudget{scope: "User"}) {
		t.Fatalf("用户零额度未拦截: %v", err)
	}
	ceiling = nil
	if _, err := f.db.AdminUpdateUser(ctx, by, f.member.user.ID, iam.UserUpdate{MaxBudget: &ceiling}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.UpdateKey(ctx, by, k.ID, iam.KeyInput{MaxBudget: &zero}); err != nil {
		t.Fatal(err)
	}
	if err := s.keyBudgetOK(ctx, p); err != (errBudget{scope: "Key"}) {
		t.Fatalf("密钥零额度未拦截: %v", err)
	}
	if _, err := f.db.UpdateKey(ctx, by, k.ID, iam.KeyInput{}); err != nil {
		t.Fatal(err)
	}
	if err := s.keyBudgetOK(ctx, p); err != nil {
		t.Fatalf("额度恢复后仍拒绝: %v", err)
	}
	status := iam.StatusDisabled
	if _, err := f.db.AdminUpdateUser(ctx, by, f.member.user.ID, iam.UserUpdate{Status: &status}); err != nil {
		t.Fatal(err)
	}
	if err := s.keyBudgetOK(ctx, p); !errors.Is(err, errKeyUnusable) {
		t.Fatalf("停用用户密钥未拒绝: %v", err)
	}
	if err := f.db.DeleteKey(ctx, by, k.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.keyBudgetOK(ctx, p); !errors.Is(err, errKeyGone) {
		t.Fatalf("已删除密钥未拒绝: %v", err)
	}
}
