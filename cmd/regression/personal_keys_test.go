package regression

import (
	"net/http"
	"testing"
)

// TestPersonalKeysIsolation 验证个人密钥 HTTP 契约及生命周期；前置独立数据库、管理员、两个成员和服务密钥。
// 参数 t 为测试上下文，返回无；验证本人列表、管理员代建的他人密钥不可见、越界详情 404、分页、非法参数 400、推理和删除失效。
// 清理：harness 自动停止网关并删除独立 schema，测试不调用外部供应商。
func TestPersonalKeysIsolation(t *testing.T) {
	h := newHarness(t, chatDeployment("personal-chat"))
	admin := h.adminSession()
	a := h.provision(t, admin, "personal-a")
	b := h.provision(t, admin, "personal-b")
	own := h.ok(http.MethodPost, "/key/generate", admin, map[string]any{"team_id": a.teamID, "key_alias": "personal-admin"}).json()
	delegated := h.ok(http.MethodPost, "/key/generate", admin, map[string]any{"team_id": a.teamID, "user_id": a.userID, "key_alias": "personal-delegated"}).json()
	service := h.ok(http.MethodPost, "/key/service-account/generate", admin, map[string]any{"team_id": a.teamID, "key_alias": "personal-service"}).json()
	// 管理接口继续允许管理员管理全局密钥，个人入口必须额外收窄，避免误伤团队管理用途。
	if got := len(rowsOf(h.ok(http.MethodGet, "/key/list", admin, nil), "keys")); got != 5 {
		t.Fatalf("管理列表权限被个人页面改动误收窄: got=%d want=5", got)
	}
	for _, tc := range []struct {
		token string
		want  int
	}{{admin, 1}, {a.session, 2}, {b.session, 1}} {
		body := h.ok(http.MethodGet, "/key/list?scope=personal&include_team_keys=true&include_created_by_keys=true", tc.token, nil)
		if got := len(rowsOf(body, "keys")); got != tc.want {
			t.Fatalf("本人列表混入他人密钥: got=%d want=%d body=%s", got, tc.want, body.describe())
		}
	}
	for _, token := range []string{a.key, firstString(delegated, "key"), firstString(service, "key")} {
		r := h.do(http.MethodGet, "/key/info?scope=personal&key="+token, admin, nil)
		if r.status != http.StatusNotFound {
			t.Fatalf("管理员个人详情泄露他人/服务密钥: %s", r.describe())
		}
	}
	// 团队管理员拥有服务密钥管理权，但个人入口仍只包含本人个人密钥。
	h.ok(http.MethodPost, "/team/member_update", admin, map[string]any{"team_id": a.teamID, "user_id": a.userID, "role": "admin"})
	if got := len(rowsOf(h.ok(http.MethodGet, "/key/list?scope=personal", a.session, nil), "keys")); got != 2 {
		t.Fatalf("团队管理员个人列表归属错误: got=%d want=2", got)
	}
	if r := h.do(http.MethodGet, "/key/info?scope=personal&key="+firstString(service, "token_id"), a.session, nil); r.status != 404 {
		t.Fatalf("团队管理员个人详情泄露服务密钥: %s", r.describe())
	}
	h.ok(http.MethodGet, "/key/info?scope=personal&key="+firstString(own, "key"), admin, nil)
	for _, suffix := range []string{"&user_id=" + a.userID, "&search=delegated", "&key_hash=" + firstString(delegated, "token_id")} {
		r := h.ok(http.MethodGet, "/key/list?scope=personal"+suffix, admin, nil)
		if len(rowsOf(r, "keys")) != 0 {
			t.Fatalf("筛选扩大本人范围: %s", r.describe())
		}
	}
	page := h.ok(http.MethodGet, "/key/list?scope=personal&size=1&page=2", a.session, nil)
	if len(rowsOf(page, "keys")) != 1 || page.json()["total_count"] != float64(2) || page.json()["current_page"] != float64(2) {
		t.Fatalf("本人分页契约错误: %s", page.describe())
	}
	if r := h.do(http.MethodGet, "/key/list?scope=personal&size=0", admin, nil); r.status != 400 {
		t.Fatalf("非法页大小未拒绝: %s", r.describe())
	}
	if r := h.do(http.MethodGet, "/key/list?scope=personal", "", nil); r.status != 401 {
		t.Fatalf("未登录查询未拒绝: %s", r.describe())
	}
	h.ok(http.MethodPost, "/v1/chat/completions", firstString(own, "key"), map[string]any{"model": "personal-chat", "messages": []any{map[string]any{"role": "user", "content": "personal key"}}})
	h.ok(http.MethodPost, "/key/delete", admin, map[string]any{"keys": []string{firstString(own, "key")}})
	if got := len(rowsOf(h.ok(http.MethodGet, "/key/list?scope=personal", admin, nil), "keys")); got != 0 {
		t.Fatalf("删除后本人列表仍有 %d 条", got)
	}
	if r := h.do(http.MethodPost, "/v1/chat/completions", firstString(own, "key"), map[string]any{"model": "personal-chat", "messages": []any{}}); r.status != 401 {
		t.Fatalf("删除后的密钥仍可推理: %s", r.describe())
	}
}
