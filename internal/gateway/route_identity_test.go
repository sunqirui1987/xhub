package gateway

import (
	"context"
	"testing"

	"github.com/sunqirui1987/xhub/internal/auth"
	"github.com/sunqirui1987/xhub/internal/authz"
	"github.com/sunqirui1987/xhub/internal/iam"
)

// TestSelectRouteTemplate 验证最窄范围优先、空值继承、空白修剪及已有选择不可覆盖。
// 前置内存主体；覆盖正常和边界输入，无数据库或计费副作用，无需清理。
func TestSelectRouteTemplate(t *testing.T) {
	p := &auth.Principal{}
	blank, key, team := "  ", " key-id ", "team-id"
	selectRouteTemplate(p, nil, "key")
	selectRouteTemplate(p, &blank, "key")
	if p.RouteTemplateID != "" || p.RouteTemplateSource != "" {
		t.Fatal("空选择未继续继承")
	}
	selectRouteTemplate(p, &key, "key")
	selectRouteTemplate(p, &team, "team")
	if p.RouteTemplateID != "key-id" || p.RouteTemplateSource != "key" {
		t.Fatalf("最窄选择被覆盖: %+v", p)
	}
}

// TestPreviewIdentityUnavailable 验证身份存储不可用时返回错误，不能错误宣称平台默认。
// 前置无身份存储服务；失败输入不修改主体，不访问外部资源，无需清理。
func TestPreviewIdentityUnavailable(t *testing.T) {
	if err := (&Server{}).resolvePreviewIdentity(t.Context(), &auth.Principal{}); err == nil {
		t.Fatal("身份存储不可用未报错")
	}
}

// TestPreviewIdentityInheritance 验证会话与密钥只读归属链及模板优先级。
// 前置私有 PostgreSQL、零团队和单团队账号；覆盖组织继承、团队覆盖、密钥覆盖、个人密钥和多团队边界。
// 返回无；不调用上游、不创建速率桶，fixture 自动关闭连接并删除整个私有 schema。
func TestPreviewIdentityInheritance(t *testing.T) {
	f := newPermFixture(t)
	ctx := t.Context()
	by := iam.Actor{ID: f.admin.user.ID, Kind: "session"}
	s := &Server{IAM: f.db}
	org := templateID(t, f, "identity-org", routeTemplateJSON(1, ""))
	team := templateID(t, f, "identity-team", routeTemplateJSON(1, ""))
	keyTemplate := templateID(t, f, "identity-key", routeTemplateJSON(1, ""))
	if err := f.db.SetScopeRouteTemplate(ctx, by, "organization", f.orgA.ID, org); err != nil {
		t.Fatal(err)
	}
	// check 解析指定主体并核对归属和模板，不触发预算或速率检查；失败定位到场景名称，无独立资源。
	check := func(name string, p *auth.Principal, wantTeam, wantOrg, wantTemplate, wantSource string) {
		t.Helper()
		if err := s.resolvePreviewIdentity(ctx, p); err != nil {
			t.Fatalf("%s 归属读取失败: %v", name, err)
		}
		if p.TeamID != wantTeam || p.OrgID != wantOrg || p.RouteTemplateID != wantTemplate || p.RouteTemplateSource != wantSource {
			t.Fatalf("%s 归属或模板错误: %+v", name, p)
		}
	}
	check("零团队清除旧归属", &auth.Principal{Kind: authz.KindSession, UserID: f.member.user.ID, TeamID: "stale", OrgID: "stale", RouteTemplateID: "stale"}, "", "", "", "")
	check("单团队组织继承", &auth.Principal{Kind: authz.KindSession, UserID: f.teamMember.user.ID}, f.teamA.ID, f.orgA.ID, org, "organization")
	if err := f.db.SetScopeRouteTemplate(ctx, by, "team", f.teamA.ID, team); err != nil {
		t.Fatal(err)
	}
	check("单团队覆盖组织", &auth.Principal{Kind: authz.KindSession, UserID: f.teamMember.user.ID}, f.teamA.ID, f.orgA.ID, team, "team")
	k, _, err := f.db.CreateKey(ctx, by, iam.KeyInput{OwnerType: iam.OwnerPersonal, UserID: f.teamMember.user.ID, TeamID: f.teamA.ID})
	if err != nil {
		t.Fatal(err)
	}
	check("密钥继承团队", &auth.Principal{Kind: authz.KindKey, KeyID: k.ID}, f.teamA.ID, f.orgA.ID, team, "team")
	if err := f.db.SetScopeRouteTemplate(ctx, by, "key", k.ID, keyTemplate); err != nil {
		t.Fatal(err)
	}
	check("密钥优先", &auth.Principal{Kind: authz.KindKey, KeyID: k.ID}, f.teamA.ID, f.orgA.ID, keyTemplate, "key")
	personal, _, err := f.db.CreateKey(ctx, by, iam.KeyInput{OwnerType: iam.OwnerPersonal, UserID: f.teamMember.user.ID})
	if err != nil {
		t.Fatal(err)
	}
	check("个人密钥不继承成员团队", &auth.Principal{Kind: authz.KindKey, KeyID: personal.ID, TeamID: "stale"}, "", "", "", "")
	mustAddMember(t, f.db, f.teamB.ID, f.teamMember.user.Email, iam.TeamMember)
	check("多团队不猜测", &auth.Principal{Kind: authz.KindSession, UserID: f.teamMember.user.ID}, "", "", "", "")
	if err := s.resolvePreviewIdentity(ctx, &auth.Principal{Kind: authz.KindKey, KeyID: "missing"}); err == nil {
		t.Fatal("不存在的密钥未返回读取错误")
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if err := s.resolveSessionRouteTemplate(cancelled, &auth.Principal{Kind: authz.KindSession, UserID: f.teamAdmin.user.ID}); err == nil {
		t.Fatal("会话身份读取取消后未停止推理")
	}
}
