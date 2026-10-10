package regression

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/sunqirui1987/xhub/internal/iam"
)

// TestLogsPaginationContract 前置真实网关、私有 schema 和本地上游；参数 t 管理生命周期。
// 验证会话聚合页数、原始请求翻页、完整统计、排序、筛选、非法参数和未鉴权错误；服务及数据自动清理。
func TestLogsPaginationContract(t *testing.T) {
	h := openHarness(t, false, chatDeployment("pagination"))
	admin := h.adminSession()
	tn := h.provision(t, admin, "pagination")
	now := time.Now().UTC()
	records := []iam.UsageRecord{}
	for i := 0; i < 60; i++ {
		session := fmt.Sprintf("session-%02d", i/2)
		records = append(records, iam.UsageRecord{RequestID: fmt.Sprintf("pagination-%02d", i), TS: now, UserID: tn.userID, SessionID: session, Status: "success", HTTPStatus: 200, PromptTokens: 3, CompletionTokens: 2, Cost: 0.1})
	}
	if err := h.db.RecordUsage(t.Context(), records); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for page := 1; page <= 2; page++ {
		body := h.ok(http.MethodGet, fmt.Sprintf("/spend/logs/ui?group_by_session=true&status_filter=non_error&page_size=25&page=%d", page), admin, nil)
		rows := rowsOf(body, "data")
		want := 25
		if page == 2 {
			want = 5
		}
		if firstFloat(body.json(), "total") != 30 || firstFloat(body.json(), "total_pages") != 2 || len(rows) != want {
			t.Fatalf("聚合分页契约: page=%d body=%v", page, body)
		}
		for _, row := range rows {
			id := firstString(row, "session_id")
			if seen[id] {
				t.Fatalf("会话跨页重复: %s", id)
			}
			seen[id] = true
			if firstFloat(row, "session_total_count") != 2 || firstFloat(row, "session_total_tokens") != 10 || firstFloat(row, "session_total_spend") != 0.2 {
				t.Fatalf("会话完整汇总: %v", row)
			}
		}
	}
	raw := h.ok(http.MethodGet, "/spend/logs/ui?group_by_session=false&page_size=25&page=3&sort_by=startTime&sort_order=asc", admin, nil)
	rows := rowsOf(raw, "data")
	if len(rows) != 10 || firstFloat(raw.json(), "total") != 60 || firstString(rows[0], "request_id") != "pagination-50" {
		t.Fatalf("原始日志稳定排序与末页: %v", raw)
	}
	session := h.ok(http.MethodGet, "/spend/logs/session/ui?session_id=session-00&page_size=1&page=2&group_by_session=true", admin, nil)
	if len(rowsOf(session, "data")) != 1 || firstFloat(session.json(), "total") != 2 {
		t.Fatalf("会话详情仍应逐条分页: %v", session)
	}
	for _, query := range []string{"page=3&page_size=25", "page=9223372036854775807&page_size=25", "page=1&page_size=25&search=does-not-exist", "page=1&page_size=25&status_filter=error"} {
		result := h.ok(http.MethodGet, "/spend/logs/ui?group_by_session=true&"+query, admin, nil)
		if len(rowsOf(result, "data")) != 0 {
			t.Fatalf("空页或空过滤错误: %s %v", query, result)
		}
	}
	invalid := h.ok(http.MethodGet, "/spend/logs/ui?group_by_session=true&page=-1&page_size=0&sort_by=invalid", admin, nil)
	if firstFloat(invalid.json(), "page") != 1 || firstFloat(invalid.json(), "page_size") != 50 || len(rowsOf(invalid, "data")) != 30 {
		t.Fatalf("非法参数默认值错误: %v", invalid)
	}
	unauth := h.do(http.MethodGet, "/spend/logs/ui?page=2", "", nil)
	if unauth.status != 401 {
		t.Fatalf("未鉴权必须拒绝: %s", unauth.describe())
	}
}

// TestKeyListPaginationContract 前置真实网关、私有 schema、一个团队项目及九把专属密钥；参数 t 管理生命周期。
// 验证项目筛选发生在分页前、页码/总数/总页数准确、别名子串筛选有效；最后精确删除密钥和项目，schema 兜底清理。
func TestKeyListPaginationContract(t *testing.T) {
	h := openHarness(t, false, chatDeployment("key-pagination"))
	admin := h.adminSession()
	tn := h.provision(t, admin, "key-pagination")
	project := h.ok(http.MethodPost, "/project/new", admin, map[string]any{"team_id": tn.teamID, "project_alias": "key-pagination-project"}).json()
	projectID := firstString(project, "project_id")
	ids := []string{}
	for i := 0; i < 6; i++ {
		key := h.ok(http.MethodPost, "/key/generate", admin, map[string]any{"team_id": tn.teamID, "project_id": projectID, "key_alias": fmt.Sprintf("project-page-%02d", i)}).json()
		ids = append(ids, firstString(key, "token_id"))
	}
	for i := 0; i < 3; i++ {
		key := h.ok(http.MethodPost, "/key/generate", admin, map[string]any{"key_alias": fmt.Sprintf("personal-page-%02d", i)}).json()
		ids = append(ids, firstString(key, "token_id"))
	}
	first := h.ok(http.MethodGet, "/key/list?project_id="+projectID+"&page=1&size=5&sort_by=key_alias&sort_order=asc", admin, nil)
	second := h.ok(http.MethodGet, "/key/list?project_id="+projectID+"&page=2&size=5&sort_by=key_alias&sort_order=asc", admin, nil)
	if len(rowsOf(first, "keys")) != 5 || firstFloat(first.json(), "total_count") != 6 || firstFloat(first.json(), "total_pages") != 2 || len(rowsOf(second, "keys")) != 1 || firstFloat(second.json(), "current_page") != 2 {
		t.Fatalf("项目密钥筛选分页错误: first=%s second=%s", first.describe(), second.describe())
	}
	aliases := h.ok(http.MethodGet, "/key/list?key_alias=project-page-0&substring_matching=true&page=1&size=10", admin, nil)
	if len(rowsOf(aliases, "keys")) != 6 || firstFloat(aliases.json(), "total_count") != 6 {
		t.Fatalf("密钥别名子串筛选错误: %s", aliases.describe())
	}
	h.ok(http.MethodPost, "/key/delete", admin, map[string]any{"keys": ids})
	h.ok(http.MethodPost, "/project/delete", admin, map[string]any{"project_id": projectID})
}

// TestDirectoryPaginationContract 前置真实网关、私有数据库、六名用户及可见团队；参数 t 管理生命周期。
// 验证用户多页真实计数、筛选、排序、未鉴权、审计/团队/模型/公开广场的超大页安全空页；schema 自动清理。
func TestDirectoryPaginationContract(t *testing.T) {
	h := openHarness(t, false, chatDeployment("directory"))
	admin := h.adminSession()
	ids := []string{}
	for i := 0; i < 6; i++ {
		body := h.ok("POST", "/user/new", admin, map[string]any{"user_email": fmt.Sprintf("directory-%02d@example.com", i), "user_alias": "directory-user", "password": "directory-password", "user_role": "user"}).json()
		ids = append(ids, firstString(body, "user_id"))
	}
	for page := 1; page <= 3; page++ {
		result := h.ok("GET", fmt.Sprintf("/user/list?search=directory-user&page=%d&page_size=2&sort_by=user_email&sort_order=asc", page), admin, nil)
		rows := rowsOf(result, "users")
		if len(rows) != 2 || firstFloat(result.json(), "total") != 6 || firstFloat(result.json(), "page") != float64(page) || firstFloat(result.json(), "total_pages") != 3 || firstString(rows[0], "user_id") != ids[(page-1)*2] {
			t.Fatalf("用户目录第%d页契约: %v", page, result)
		}
	}
	for _, query := range []string{"search=directory-user&page=4&page_size=2", "user_ids=missing", "search=directory-missing", "page=9223372036854775807&page_size=25", "role=invalid", "sso_user_ids=not-stored"} {
		result := h.ok("GET", "/user/list?"+query, admin, nil)
		if len(rowsOf(result, "users")) != 0 {
			t.Fatalf("用户目录空页条件%s: %v", query, result)
		}
	}
	lookup := h.ok("GET", "/user/list?user_ids="+ids[5]+"&page_size=1", admin, nil)
	if len(rowsOf(lookup, "users")) != 1 || firstFloat(lookup.json(), "total") != 1 {
		t.Fatalf("用户ID查询契约: %v", lookup)
	}
	email := h.ok("GET", "/user/list?user_email=directory-05", admin, nil)
	if len(rowsOf(email, "users")) != 1 {
		t.Fatalf("邮箱选择器条件被忽略: %v", email)
	}
	invalid := h.ok("GET", "/user/list?search=directory-user&page=-1&page_size=0", admin, nil)
	if firstFloat(invalid.json(), "page") != 1 || firstFloat(invalid.json(), "page_size") != 100 || firstFloat(invalid.json(), "total") != 6 {
		t.Fatalf("用户分页默认参数错误: %v", invalid)
	}
	// 普通无团队用户只应看到自己；provision 会同时授予组织管理关系，不适合作此夹具。
	memberSession := h.login("directory-00@example.com", "directory-password")
	self := h.ok("GET", "/user/list?page_size=1", memberSession, nil)
	if firstFloat(self.json(), "total") != 1 || firstString(rowsOf(self, "users")[0], "user_id") != ids[0] {
		t.Fatalf("用户目录范围扩大: %v", self)
	}
	if got := h.do("GET", "/user/list?page=2", "", nil); got.status != 401 {
		t.Fatalf("用户目录未鉴权错误: %s", got.describe())
	}
	// 这些路由使用同一边界计算；真实路由必须保留空数组及计数契约，不能因切片越界 panic。
	for _, c := range []struct{ path, field string }{
		{"/audit/logs?page=9223372036854775807&page_size=25", "audit_logs"},
		{"/v2/team/list?page=9223372036854775807&page_size=25", "teams"},
		{"/v2/model/info?page=9223372036854775807&size=25", "data"},
		{"/model/groups?page=9223372036854775807&size=25", "data"},
		{"/public/v1/model_hub?page=9223372036854775807&page_size=25", "data"},
		{"/public/v1/model_hub/providers?page=9223372036854775807&page_size=25", "data"},
		{"/guardrails/usage/logs?page=9223372036854775807&page_size=25", "logs"},
	} {
		result := h.ok("GET", c.path, admin, nil)
		if len(rowsOf(result, c.field)) != 0 {
			t.Fatalf("路由极大页应为空 %s: %v", c.path, result)
		}
	}
	for _, id := range ids {
		h.ok("POST", "/user/delete", admin, map[string]any{"user_id": id})
	}
}
