package regression

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

// TestTeamlessPersonalKeys 验证真实 HTTP 个人密钥契约与数据面；前置独立数据库、无团队用户和本地上游。
// 参数 t 为测试上下文，返回无；覆盖 null/缺省/空团队、归属越权、模型权限、记账、额度、禁用、轮换和删除。
// harness 清理全部测试账号、密钥与专属 schema；不依赖外部供应商凭据。
func TestTeamlessPersonalKeys(t *testing.T) {
	h := newHarness(t, chatDeployment("solo-chat"))
	admin := h.adminSession()
	member := h.provision(t, admin, "already-member")
	memberKey := h.ok(http.MethodPost, "/key/generate", member.session, map[string]any{"team_id": nil, "models": []string{"all-proxy-models"}}).json()
	if memberKey["team_id"] != nil || memberKey["user_id"] != member.userID {
		t.Fatalf("已有团队用户被强制绑定团队: %v", memberKey)
	}
	h.ok(http.MethodPost, "/v1/chat/completions", firstString(memberKey, "key"), chatRequest("solo-chat", "already member"))
	h.ok(http.MethodPost, "/key/delete", member.session, map[string]any{"keys": []string{firstString(memberKey, "token_id")}})
	// 显式绑定团队的旧接口仍继承团队模型权限，全选标识不能绕过团队限制。
	h.ok(http.MethodPost, "/team/update", admin, map[string]any{"team_id": member.teamID, "models": []string{"different-model"}})
	bound := h.ok(http.MethodPost, "/key/generate", member.session, map[string]any{"team_id": member.teamID, "models": []string{"all-proxy-models"}}).json()
	boundSecret, boundID := firstString(bound, "key"), firstString(bound, "token_id")
	markBound := len(h.upstreamCalls())
	if r := h.do(http.MethodPost, "/v1/chat/completions", boundSecret, chatRequest("solo-chat", "team restriction")); r.status != 401 || !strings.Contains(errorMessage(r), "model not in allowed model list") {
		t.Fatalf("全部模型选项绕过团队授权: %s", r.describe())
	}
	if len(h.upstreamSince(markBound)) != 0 {
		t.Fatal("团队模型拒绝仍调用供应商")
	}
	h.ok(http.MethodPost, "/team/update", admin, map[string]any{"team_id": member.teamID, "models": []string{"solo-chat"}})
	h.ok(http.MethodPost, "/key/update", member.session, map[string]any{"key": boundID, "models": []string{"all-team-models"}})
	h.ok(http.MethodPost, "/v1/chat/completions", boundSecret, chatRequest("solo-chat", "team allowed"))
	h.ok(http.MethodPost, "/key/delete", member.session, map[string]any{"keys": []string{boundID}})
	u := h.ok(http.MethodPost, "/user/new", admin, map[string]any{"user_email": "solo@example.com", "password": "solo-password", "user_role": "user"}).json()
	uid := firstString(u, "user_id")
	session := h.login("solo@example.com", "solo-password")
	var secret, id string
	for i, body := range []map[string]any{
		{"user_id": uid, "key_alias": "test", "models": []string{"all-proxy-models"}, "key_type": "llm_api", "duration": nil, "team_id": nil, "metadata": map[string]any{}},
		{"key_alias": "missing-team"}, {"team_id": "", "key_alias": "empty-team"},
	} {
		issued := h.ok(http.MethodPost, "/key/generate", session, body).json()
		if issued["user_id"] != uid || issued["team_id"] != nil {
			t.Fatalf("个人密钥被绑定虚构团队: %v", issued)
		}
		if i == 0 {
			secret, id = firstString(issued, "key"), firstString(issued, "token_id")
		} else {
			h.ok(http.MethodPost, "/key/delete", session, map[string]any{"keys": []string{firstString(issued, "token_id")}})
		}
	}
	for _, tc := range []struct {
		path   string
		body   map[string]any
		status int
	}{
		{"/key/generate", map[string]any{"user_id": "someone-else"}, 403},
		{"/key/service-account/generate", map[string]any{}, 400},
		{"/key/generate", map[string]any{"project_id": "project"}, 400},
		{"/key/generate", map[string]any{"owner_type": "unknown"}, 400},
	} {
		if r := h.do(http.MethodPost, tc.path, session, tc.body); r.status != tc.status {
			t.Fatalf("非法个人密钥请求状态错误: path=%s %s", tc.path, r.describe())
		}
	}
	if r := h.do(http.MethodPost, "/key/generate", "", map[string]any{}); r.status != 401 {
		t.Fatalf("未登录创建未拒绝: %s", r.describe())
	}
	if got := len(rowsOf(h.ok(http.MethodGet, "/key/list?scope=personal", session, nil), "keys")); got != 1 {
		t.Fatalf("本人列表缺少独立密钥: %d", got)
	}
	h.ok(http.MethodGet, "/key/info?scope=personal&key="+id, session, nil)
	h.ok(http.MethodPost, "/v1/chat/completions", secret, chatRequest("solo-chat", "teamless first"))
	h.flushSpend()
	k, err := h.db.GetKey(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := h.db.GetUser(context.Background(), uid)
	if err != nil || k.Spend <= 0 || !nearlyEqual(owner.Spend, k.Spend) {
		t.Fatalf("独立个人密钥未正确记账: key=%+v user=%+v err=%v", k, owner, err)
	}
	events, err := h.db.Engine.QueryString("SELECT user_id, team_id, organization_id FROM usage_events WHERE key_id = ?", id)
	if err != nil || len(events) != 1 || events[0]["user_id"] != uid || events[0]["team_id"] != "" || events[0]["organization_id"] != "" {
		t.Fatalf("用量归属错误: %v %v", events, err)
	}
	// 拒绝必须在供应商调用前生效，且恢复配置后下一次调用即可使用。
	h.ok(http.MethodPost, "/key/update", session, map[string]any{"key": id, "models": []string{"other-model"}})
	mark := len(h.upstreamCalls())
	if r := h.do(http.MethodPost, "/v1/chat/completions", secret, chatRequest("solo-chat", "restricted")); r.status != 401 || !strings.Contains(errorMessage(r), "model not in allowed model list") {
		t.Fatalf("独立密钥模型限制失效: %s", r.describe())
	}
	h.ok(http.MethodPost, "/key/update", session, map[string]any{"key": id, "models": []string{"all-proxy-models"}})
	h.setUserBudget(t, admin, uid, owner.Spend)
	if r := h.do(http.MethodPost, "/v1/chat/completions", secret, chatRequest("solo-chat", "user budget")); r.status != 429 || !strings.Contains(errorMessage(r), "User") {
		t.Fatalf("个人额度未生效: %s", r.describe())
	}
	h.setUserBudget(t, admin, uid, 100)
	h.setKeyBudget(t, admin, secret, k.Spend)
	if r := h.do(http.MethodPost, "/v1/chat/completions", secret, chatRequest("solo-chat", "key budget")); r.status != 429 || !strings.Contains(errorMessage(r), "Key") {
		t.Fatalf("密钥额度未生效: %s", r.describe())
	}
	if got := h.upstreamSince(mark); len(got) != 0 {
		t.Fatalf("权限或额度拒绝仍调用供应商: %v", got)
	}
	h.setKeyBudget(t, admin, secret, 100)
	h.ok(http.MethodPost, "/key/block", session, map[string]any{"key": id})
	if r := h.do(http.MethodPost, "/v1/chat/completions", secret, chatRequest("solo-chat", "blocked")); r.status != 401 {
		t.Fatalf("禁用个人密钥仍可推理: %s", r.describe())
	}
	h.ok(http.MethodPost, "/key/unblock", session, map[string]any{"key": id})
	rotated := h.ok(http.MethodPost, "/key/"+id+"/regenerate", session, map[string]any{}).json()
	if r := h.do(http.MethodPost, "/v1/chat/completions", secret, chatRequest("solo-chat", "old key")); r.status != 401 {
		t.Fatalf("轮换后旧密钥仍有效: %s", r.describe())
	}
	secret = firstString(rotated, "key")
	h.ok(http.MethodPost, "/v1/chat/completions", secret, chatRequest("solo-chat", "rotated"))
	h.ok(http.MethodPost, "/key/delete", session, map[string]any{"keys": []string{id}})
	if r := h.do(http.MethodPost, "/v1/chat/completions", secret, chatRequest("solo-chat", "deleted")); r.status != 401 {
		t.Fatalf("删除后个人密钥仍有效: %s", r.describe())
	}
	if got := len(rowsOf(h.ok(http.MethodGet, "/key/list?scope=personal", session, nil), "keys")); got != 0 {
		t.Fatalf("删除后列表仍含密钥: %d", got)
	}
	newKey := h.ok(http.MethodPost, "/key/generate", session, map[string]any{}).json()
	h.ok(http.MethodPost, "/user/update", admin, map[string]any{"user_id": uid, "blocked": true})
	if r := h.do(http.MethodPost, "/v1/chat/completions", firstString(newKey, "key"), chatRequest("solo-chat", "disabled owner")); r.status != 401 {
		t.Fatalf("停用属主未撤销独立密钥: %s", r.describe())
	}
}
