package templateauth

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/sunqirui1987/xhub/cmd/regression/testsupport"
	"github.com/sunqirui1987/xhub/internal/auth"
	"github.com/sunqirui1987/xhub/internal/authz"
	"github.com/sunqirui1987/xhub/internal/iam"
	"xorm.io/xorm"
)

type selectionHost struct {
	db     *iam.DB
	calls  int
	action authz.Action
	object authz.Object
	denied error
}

// Identity 返回测试的真实身份库；无参数，无读写副作用，供 Selection 查询模板使用。
func (h *selectionHost) Identity() *iam.DB { return h.db }

// Authorize 记录传入授权边界的动作与完整归属对象，返回夹具指定的授权结果。
// 参数为请求、身份、动作与对象；只记录内存调用，供服务边界测试证明查库后没有遗漏租户范围。
func (h *selectionHost) Authorize(_ *http.Request, _ *auth.Principal, action authz.Action, object authz.Object) error {
	h.calls++
	h.action = action
	h.object = object
	return h.denied
}

// TestEmptySelectionDoesNotReadStorage 验证清空模板表示继承，空白 ID 不访问存储或触发模板读授权。
// 前置为 nil Host，任何依赖调用都会失败；正常空值与空白均成功，无数据清理。
func TestEmptySelectionDoesNotReadStorage(t *testing.T) {
	for _, id := range []string{"", "  ", "\t\n"} {
		if err := Selection(nil, nil, nil, id); err != nil {
			t.Fatalf("空绑定 %q 不应查询模板: %v", id, err)
		}
	}
}

// TestSelectionLoadsTrustedOwnership 验证从真实 PostgreSQL 读取模板归属再进行 read 授权，拒绝结果原样传递。
// 前置为专用 schema 中平台、组织、团队模板，覆盖 ID 去空白、缺失和授权失败；结束关闭连接并删除整个 schema。
func TestSelectionLoadsTrustedOwnership(t *testing.T) {
	db, err := iam.Open(t.Context(), testsupport.Postgres(t, "template_selection"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	// 真实外键要求模板归属先存在；夹具在专用 schema 中创建父对象，不能用不存在的租户模拟可信归属。
	if _, err := db.Engine.Insert(&iam.Organization{ID: "org-test", Name: "template-test-org", Status: iam.StatusActive}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Engine.Insert(&iam.Team{ID: "team-test", OrganizationID: "org-test", Name: "template-test-team", Status: iam.StatusActive}); err != nil {
		t.Fatal(err)
	}
	request := httptestRequest(t.Context())
	host := &selectionHost{db: db}
	principal := &auth.Principal{}
	for _, tc := range []struct {
		name  string
		owner iam.TemplateOwner
	}{
		{"平台", iam.TemplateOwner{}}, {"组织", iam.TemplateOwner{OrgID: "org-test"}}, {"团队", iam.TemplateOwner{OrgID: "org-test", TeamID: "team-test"}},
	} {
		row, err := db.CreateRouteTemplate(t.Context(), iam.Actor{Kind: "system"}, tc.owner, tc.name, "{}")
		if err != nil {
			t.Fatal(err)
		}
		host.calls = 0
		host.denied = nil
		if err := Selection(host, request, principal, "  "+row.ID+"  "); err != nil {
			t.Fatalf("%s 合法模板被拒绝: %v", tc.name, err)
		}
		want := authz.Object{Type: authz.ObjectRouteTemplate, ID: row.ID, OrgID: tc.owner.OrgID, TeamID: tc.owner.TeamID}
		if host.calls != 1 || host.action != authz.ActionRouteTemplateRead || host.object != want {
			t.Fatalf("%s 模板授权未使用存储归属: calls=%d action=%v object=%+v", tc.name, host.calls, host.action, host.object)
		}
		host.denied = authz.ErrForbidden
		if err := Selection(host, request, principal, row.ID); !errors.Is(err, authz.ErrForbidden) {
			t.Fatalf("%s 未保留授权拒绝: %v", tc.name, err)
		}
	}
	host.calls = 0
	if err := Selection(host, request, principal, "missing-template"); !errors.Is(err, authz.ErrNotFound) || host.calls != 0 {
		t.Fatalf("缺失模板没有提前拒绝: err=%v calls=%d", err, host.calls)
	}
}

// httptestRequest 构造只携带上下文的模板查询请求；参数为测试上下文，返回无网络请求，无副作用。
func httptestRequest(ctx context.Context) *http.Request {
	request, _ := http.NewRequestWithContext(ctx, http.MethodPost, "http://test/route_template/binding", nil)
	return request
}

// TestSelectionFailsClosedOnStorageError 验证存储读取失败保留底层错误并阻止授权，不可将故障转换为继承。
// 前置为取消的请求上下文和未连接数据库 engine；断言 InternalError 与零授权次数，结束关闭 engine，无 schema 写入。
func TestSelectionFailsClosedOnStorageError(t *testing.T) {
	engine, err := xorm.NewEngine("pgx", "postgres://127.0.0.1:1/unavailable")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = engine.Close() })
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	host := &selectionHost{db: &iam.DB{Engine: engine}}
	err = Selection(host, httptestRequest(ctx), &auth.Principal{}, "template")
	var internal *authz.InternalError
	if !errors.As(err, &internal) || !errors.Is(err, context.Canceled) || host.calls != 0 {
		t.Fatalf("存储失败没有关闭授权: err=%v calls=%d", err, host.calls)
	}
}
