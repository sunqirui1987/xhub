package regression

import (
	"fmt"
	"github.com/sunqirui1987/xhub/internal/iam"
	"net/http"
	"testing"
)

// TestQuotaHierarchyHTTP 验证真实管理接口和数据面额度链；前置隔离 schema 与本地供应商，无外部凭据。
// 参数 t 为测试上下文，无返回；断言超配回滚、单团队、密钥共享、消费归属、额度拦截、退出团队后的独立个人消费，harness 清理全部数据。
func TestQuotaHierarchyHTTP(t *testing.T) {
	h := newHarness(t, chatDeployment("quota-model"))
	admin := h.adminSession()
	tn := h.provision(t, admin, "quota")
	h.setOrgBudget(t, admin, tn.orgID, 1000)
	h.setTeamBudget(t, admin, tn.teamID, 600)
	h.setUserBudget(t, admin, tn.userID, 400)
	// 非法额度不得被转换为 null，接口拒绝后个人额度仍为 400，harness 清理所有记录。
	for _, invalid := range []any{"wrong", "NaN", "Infinity", true, -1} {
		response := h.do(http.MethodPost, "/user/update", admin, map[string]any{"user_id": tn.userID, "max_budget": invalid})
		if response.status != 400 {
			t.Fatalf("非法个人额度未拒绝: %s", response.describe())
		}
		response = h.do(http.MethodPost, "/key/generate", tn.session, map[string]any{"max_budget": invalid})
		if response.status != 400 {
			t.Fatalf("非法密钥额度未拒绝: %s", response.describe())
		}
		response = h.do(http.MethodPost, "/organization/new", admin, map[string]any{"organization_alias": "invalid", "max_budget": invalid})
		if response.status != 400 {
			t.Fatalf("非法组织额度未拒绝: %s", response.describe())
		}
	}
	unchanged, err := h.gw.Identity().GetUser(t.Context(), tn.userID)
	if err != nil || unchanged.MaxBudget == nil || *unchanged.MaxBudget != 400 {
		t.Fatalf("非法更新改变了个人额度: %+v %v", unchanged, err)
	}
	other := h.ok(http.MethodPost, "/team/new", admin, map[string]any{"organization_id": tn.orgID, "team_alias": "quota-other", "max_budget": 400}).json()
	otherID := firstString(other, "team_id")
	rejected := h.do(http.MethodPost, "/team/new", admin, map[string]any{"organization_id": tn.orgID, "team_alias": "overflow", "max_budget": 1})
	if rejected.status != 400 || firstString(rejected.json()["error"].(map[string]any), "type") != "quota_allocation_exceeded" {
		t.Fatalf("组织超配契约: %s", rejected.describe())
	}
	second := h.do(http.MethodPost, "/team/member_add", admin, map[string]any{"team_id": otherID, "member": map[string]any{"user_email": tn.email, "role": "user"}})
	if second.status != 400 {
		t.Fatalf("第二团队未拒绝: %s", second.describe())
	}
	reserved := h.ok(http.MethodPost, "/key/generate", tn.session, map[string]any{"key_alias": "reserved", "max_budget": 400}).json()
	overflow := h.do(http.MethodPost, "/key/generate", tn.session, map[string]any{"key_alias": "overflow", "max_budget": 1})
	if overflow.status != 400 {
		t.Fatalf("个人密钥超配未拒绝: %s", overflow.describe())
	}
	mark := len(h.upstreamCalls())
	blocked := h.do(http.MethodPost, "/v1/chat/completions", tn.key, chatRequest("quota-model", "shared key blocked"))
	if blocked.status != 429 || len(h.upstreamSince(mark)) != 0 {
		t.Fatalf("共享密钥花掉固定保留额: %s", blocked.describe())
	}
	// 更新接口同样拒绝坏值，并保留已分配的固定 Key 金额。
	badPatch := h.do(http.MethodPost, "/key/update", tn.session, map[string]any{"key": firstString(reserved, "token_id"), "max_budget": "wrong"})
	if badPatch.status != 400 {
		t.Fatalf("非法密钥更新未拒绝: %s", badPatch.describe())
	}
	persisted, err := h.gw.Identity().GetKey(t.Context(), firstString(reserved, "token_id"))
	if err != nil || persisted.MaxBudget == nil || *persisted.MaxBudget != 400 {
		t.Fatalf("非法更新清空密钥额度: %+v %v", persisted, err)
	}
	secret := firstString(reserved, "key")
	h.ok(http.MethodPost, "/v1/chat/completions", secret, chatRequest("quota-model", "fixed key succeeds"))
	h.flushSpend()
	info := h.ok(http.MethodGet, "/team/info?team_id="+tn.teamID, admin, nil).json()["team_info"].(map[string]any)
	if parseFloatOrZero(fmt.Sprint(info["spend"])) <= 0 {
		t.Fatalf("未绑定团队密钥消费未归属团队: %v", info)
	}
	h.ok(http.MethodPost, "/key/delete", tn.session, map[string]any{"keys": []string{firstString(reserved, "token_id")}})
	h.ok(http.MethodPost, "/v1/chat/completions", tn.key, chatRequest("quota-model", "released reservation"))
	h.ok(http.MethodPost, "/team/member_delete", admin, map[string]any{"team_id": tn.teamID, "user_id": tn.userID})
	h.flushSpend()
	before := h.ok(http.MethodGet, "/team/info?team_id="+tn.teamID, admin, nil).json()["team_info"].(map[string]any)["spend"]
	h.ok(http.MethodPost, "/v1/chat/completions", tn.session, chatRequest("quota-model", "independent personal session"))
	h.flushSpend()
	after := h.ok(http.MethodGet, "/team/info?team_id="+tn.teamID, admin, nil).json()["team_info"].(map[string]any)["spend"]
	if before != after {
		t.Fatalf("退出后的个人消费增加了原团队账单: before=%v after=%v", before, after)
	}
	stale := h.do(http.MethodPost, "/v1/chat/completions", tn.key, chatRequest("quota-model", "revoked key"))
	if stale.status != 401 {
		t.Fatalf("退出旧密钥未失效: %s", stale.describe())
	}
}

// TestQuotaHierarchyAdministratorHTTP 验证组织/团队管理员额度权限和越界错误；前置本地真实后台与隔离数据库。
// 参数 t 为测试上下文，无返回；覆盖额度保存、角色提升拒绝、跨组织拒绝及超配回滚，harness 清理全部记录。
func TestQuotaHierarchyAdministratorHTTP(t *testing.T) {
	h := newHarness(t, chatDeployment("quota-admin-model"))
	admin := h.adminSession()
	a := h.provision(t, admin, "quota-admin-a")
	b := h.provision(t, admin, "quota-admin-b")
	h.setOrgBudget(t, admin, a.orgID, 1000)
	if err := h.gw.Identity().SetOrgAdmin(t.Context(), iam.Actor{}, a.orgID, a.userID, true); err != nil {
		t.Fatal(err)
	}
	h.ok(http.MethodPost, "/team/update", a.session, map[string]any{"team_id": a.teamID, "max_budget": 600})
	h.ok(http.MethodPost, "/team/new", a.session, map[string]any{"organization_id": a.orgID, "team_alias": "quota-owned", "max_budget": 400})
	rejected := h.do(http.MethodPost, "/team/new", a.session, map[string]any{"organization_id": a.orgID, "team_alias": "quota-overflow", "max_budget": 1})
	if rejected.status != 400 {
		t.Fatalf("组织管理员超配未拒绝: %s", rejected.describe())
	}
	rejected = h.do(http.MethodPost, "/team/update", a.session, map[string]any{"team_id": b.teamID, "max_budget": 600})
	if rejected.status < 400 {
		t.Fatalf("组织管理员跨组织修改: %s", rejected.describe())
	}
	h.ok(http.MethodPost, "/user/update", a.session, map[string]any{"user_id": a.userID, "max_budget": 400})
	rejected = h.do(http.MethodPost, "/user/update", a.session, map[string]any{"user_id": a.userID, "max_budget": 400, "user_role": "proxy_admin"})
	if rejected.status != 400 {
		t.Fatalf("额度入口允许角色提升: %s", rejected.describe())
	}
	rejected = h.do(http.MethodPost, "/user/update", a.session, map[string]any{"user_id": b.userID, "max_budget": 400})
	if rejected.status < 400 {
		t.Fatalf("额度入口跨组织修改: %s", rejected.describe())
	}
	if err := h.gw.Identity().SetOrgAdmin(t.Context(), iam.Actor{}, a.orgID, a.userID, false); err != nil {
		t.Fatal(err)
	}
	h.ok(http.MethodPost, "/team/member_update", admin, map[string]any{"team_id": a.teamID, "user_id": a.userID, "role": "admin"})
	h.ok(http.MethodPost, "/user/update", a.session, map[string]any{"user_id": a.userID, "max_budget": 500})
	rejected = h.do(http.MethodPost, "/team/update", a.session, map[string]any{"team_id": a.teamID, "max_budget": 700})
	if rejected.status < 400 {
		t.Fatalf("团队管理员自行提高团队额度: %s", rejected.describe())
	}
}
