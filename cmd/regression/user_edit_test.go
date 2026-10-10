package regression

import (
	"net/http"
	"testing"
)

// TestUserAccountEditing 验证真实用户更新接口和团队关系；前置隔离网关、管理员与两个成员，参数 t 为测试上下文，无返回值。
// 覆盖更新契约、空名称/预算、非法字段回滚、普通成员越权、团队角色、移除后推理拒绝及删除；harness 清理私有 schema 和本地上游。
func TestUserAccountEditing(t *testing.T) {
	h := newHarness(t, chatDeployment("edit-chat"))
	admin := h.adminSession()
	a := h.provision(t, admin, "edit-a")
	b := h.provision(t, admin, "edit-b")
	result := h.ok(http.MethodPost, "/user/update", admin, map[string]any{"user_id": a.userID, "user_email": "Edited@xhub.local", "user_alias": "编辑后的名称", "max_budget": 12.5, "user_role": "user"}).json()["user_info"].(map[string]any)
	if result["user_alias"] != "编辑后的名称" || result["user_email"] != "edited@xhub.local" || result["max_budget"] != 12.5 {
		t.Fatalf("用户更新契约错误: %+v", result)
	}
	for _, fields := range []map[string]any{{"user_role": "invalid"}, {"max_budget": -1}, {"user_email": ""}} {
		fields["user_id"], fields["user_alias"] = a.userID, "不能部分保存"
		if r := h.do(http.MethodPost, "/user/update", admin, fields); r.status != 400 {
			t.Fatalf("非法更新未返回 400: %s", r.describe())
		}
		current := h.ok(http.MethodGet, "/user/info?user_id="+a.userID, admin, nil).json()["user_info"].(map[string]any)
		if current["user_alias"] != "编辑后的名称" {
			t.Fatalf("非法更新未整体回滚: %+v", current)
		}
	}
	if r := h.do(http.MethodPost, "/user/update", b.session, map[string]any{"user_id": a.userID, "user_alias": "越权"}); r.status != 403 && r.status != 404 {
		t.Fatalf("普通成员可编辑他人: %s", r.describe())
	}
	h.ok(http.MethodPost, "/user/update", admin, map[string]any{"user_id": a.userID, "user_alias": "", "max_budget": nil})
	empty := h.ok(http.MethodGet, "/v2/user/info?user_id="+a.userID, admin, nil).json()["user_info"].(map[string]any)
	if empty["user_alias"] != "" || empty["max_budget"] != nil {
		t.Fatalf("空名称/无限预算未保存: %+v", empty)
	}
	h.ok(http.MethodPost, "/team/member_update", admin, map[string]any{"team_id": a.teamID, "user_id": a.userID, "role": "admin"})
	h.ok(http.MethodPost, "/team/member_update", admin, map[string]any{"team_id": a.teamID, "user_id": a.userID, "role": "user"})
	h.ok(http.MethodPost, "/v1/chat/completions", a.key, map[string]any{"model": "edit-chat", "messages": []any{map[string]any{"role": "user", "content": "before removal"}}})
	h.ok(http.MethodPost, "/team/member_delete", admin, map[string]any{"team_id": a.teamID, "user_id": a.userID})
	if r := h.do(http.MethodPost, "/v1/chat/completions", a.key, map[string]any{"model": "edit-chat", "messages": []any{}}); r.status != 401 && r.status != 403 {
		t.Fatalf("移除团队后密钥仍可访问: %s", r.describe())
	}
	h.ok(http.MethodPost, "/user/delete", admin, map[string]any{"user_id": a.userID})
	if r := h.do(http.MethodGet, "/user/info?user_id="+a.userID, admin, nil); r.status != 404 {
		t.Fatalf("删除后账户仍存在: %s", r.describe())
	}
}
