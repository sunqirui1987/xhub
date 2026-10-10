package regression

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// TestRateHierarchyHTTP 验证真实四层创建/更新、汇总、零/null、非法输入和拒绝不调上游；隔离 schema 与供应商由 harness 清理。
func TestRateHierarchyHTTP(t *testing.T) {
	for _, scope := range []string{"key", "user", "team", "organization"} {
		for _, dimension := range []string{"rpm_limit", "tpm_limit"} {
			t.Run(scope+"/"+dimension, func(t *testing.T) {
				h := newHarness(t, chatDeployment("rates-model"))
				admin := h.adminSession()
				tn := h.provision(t, admin, "rates")
				method := http.MethodPost
				if scope == "organization" {
					method = http.MethodPatch
				}
				path, idField, id := "/"+scope+"/update", scope+"_id", tn.userID
				switch scope {
				case "key":
					idField = "key"
					id = tn.key
				case "team":
					id = tn.teamID
				case "organization":
					idField = "organization_id"
					id = tn.orgID
				}
				for _, invalid := range []any{-1, 1.5, 2147483648, "NaN", "wrong", true} {
					response := h.do(method, path, admin, map[string]any{idField: id, dimension: invalid})
					if response.status != 400 {
						t.Fatalf("%s 非法值 %v: %s", scope, invalid, response.describe())
					}
				}
				// RPM 放行两次；TPM 先用零验证任何请求不可调用，再放开用于检验 null。
				limit := 2
				if dimension == "tpm_limit" {
					limit = 0
				}
				h.ok(method, path, admin, map[string]any{idField: id, dimension: limit})
				if dimension == "rpm_limit" {
					h.ok(http.MethodPost, "/v1/chat/completions", tn.key, chatRequest("rates-model", "first"))
					if scope == "key" {
						h.ok(http.MethodPost, "/v1/chat/completions", tn.key, chatRequest("rates-model", "second"))
					} else {
						h.ok(http.MethodPost, "/v1/chat/completions", tn.session, chatRequest("rates-model", "session shares personal"))
					}
				}
				mark := len(h.upstreamCalls())
				response := h.do(http.MethodPost, "/v1/chat/completions", tn.key, chatRequest("rates-model", "blocked"))
				if response.status != 429 || !strings.Contains(response.describe(), dimension) || len(h.upstreamSince(mark)) != 0 {
					t.Fatalf("%s %s 未拦截数据面: %s", scope, dimension, response.describe())
				}
				h.ok(method, path, admin, map[string]any{idField: id, dimension: nil})
				h.ok(http.MethodPost, "/v1/chat/completions", tn.key, chatRequest("rates-model", "cleared"))
			})
		}
	}
}

// TestRateAllocationHTTP 验证与预算一致的逐级分配、超配回滚与共享保留；真实接口和数据面，harness 清理隔离 schema。
func TestRateAllocationHTTP(t *testing.T) {
	for _, dimension := range []string{"rpm_limit", "tpm_limit"} {
		t.Run(dimension, func(t *testing.T) {
			h := newHarness(t, chatDeployment("allocation-model"))
			admin := h.adminSession()
			tn := h.provision(t, admin, "allocate")
			h.ok(http.MethodPatch, "/organization/update", admin, map[string]any{"organization_id": tn.orgID, dimension: 10})
			h.ok(http.MethodPost, "/team/update", admin, map[string]any{"team_id": tn.teamID, dimension: 6})
			other := h.ok(http.MethodPost, "/team/new", admin, map[string]any{"organization_id": tn.orgID, "team_alias": "other", dimension: 4}).json()
			response := h.do(http.MethodPost, "/team/new", admin, map[string]any{"organization_id": tn.orgID, "team_alias": "overflow", dimension: 1})
			if response.status != 400 || !strings.Contains(response.describe(), "rate_allocation_exceeded") {
				t.Fatalf("团队超配: %s", response.describe())
			}
			h.ok(http.MethodPost, "/user/update", admin, map[string]any{"user_id": tn.userID, dimension: 6})
			response = h.do(http.MethodPost, "/user/new", admin, map[string]any{"user_email": "overflow@local.invalid", "password": "password123", "team_id": tn.teamID, dimension: 1})
			if response.status != 400 {
				t.Fatalf("个人超配: %s", response.describe())
			}
			fixed := h.ok(http.MethodPost, "/key/generate", tn.session, map[string]any{"key_alias": "fixed", dimension: 6}).json()
			response = h.do(http.MethodPost, "/key/generate", tn.session, map[string]any{"key_alias": "overflow", dimension: 1})
			if response.status != 400 {
				t.Fatalf("Key 超配: %s", response.describe())
			}
			response = h.do(http.MethodPatch, "/organization/update", admin, map[string]any{"organization_id": tn.orgID, dimension: 9})
			if response.status != 400 {
				t.Fatalf("组织缩小未校验分配: %s", response.describe())
			}
			mark := len(h.upstreamCalls())
			response = h.do(http.MethodPost, "/v1/chat/completions", tn.key, chatRequest("allocation-model", "reserved for fixed"))
			if response.status != 429 || len(h.upstreamSince(mark)) != 0 {
				t.Fatalf("共享占用固定 Key: %s", response.describe())
			}
			if dimension == "rpm_limit" {
				h.ok(http.MethodPost, "/v1/chat/completions", firstString(fixed, "key"), chatRequest("allocation-model", "fixed succeeds"))
			}
			info := h.ok(http.MethodGet, "/team/info?team_id="+firstString(other, "team_id"), admin, nil).json()["team_info"].(map[string]any)
			if fmt.Sprint(info[dimension]) != "4" {
				t.Fatalf("列表丢失分钟上限: %v", info)
			}
		})
	}
}

// TestRateSiblingAggregationHTTP 验证多个 Key、个人和团队的真实共享汇总、退出释放与权限；隔离数据库和本地上游由 harness 清理。
func TestRateSiblingAggregationHTTP(t *testing.T) {
	for _, dimension := range []string{"rpm_limit", "tpm_limit"} {
		t.Run(dimension, func(t *testing.T) {
			h := newHarness(t, chatDeployment("siblings-model"))
			admin := h.adminSession()
			tn := h.provision(t, admin, "siblings")
			limit := 2
			if dimension == "tpm_limit" {
				limit = 64
			}
			h.ok(http.MethodPatch, "/organization/update", admin, map[string]any{"organization_id": tn.orgID, dimension: limit})
			other := h.ok(http.MethodPost, "/team/new", admin, map[string]any{"organization_id": tn.orgID, "team_alias": "siblings-other"}).json()
			member := h.ok(http.MethodPost, "/user/new", admin, map[string]any{"team_id": firstString(other, "team_id"), "user_email": "siblings-other@local.invalid", "password": "password123"}).json()
			key := h.ok(http.MethodPost, "/key/generate", admin, map[string]any{"user_id": firstString(member, "user_id"), "key_alias": "other"}).json()
			// 短正文输入估算 32 tokens；两团队合计正好消耗 2 RPM / 64 TPM。
			body := chatRequest("siblings-model", "ok")
			h.ok(http.MethodPost, "/v1/chat/completions", tn.key, body)
			h.ok(http.MethodPost, "/v1/chat/completions", firstString(key, "key"), body)
			mark := len(h.upstreamCalls())
			response := h.do(http.MethodPost, "/v1/chat/completions", tn.session, body)
			if response.status != 429 || !strings.Contains(response.describe(), "org "+dimension) || len(h.upstreamSince(mark)) != 0 {
				t.Fatalf("跨团队组织汇总未拦截: %s", response.describe())
			}
			denied := h.do(http.MethodPatch, "/organization/update", tn.session, map[string]any{"organization_id": tn.orgID, dimension: nil})
			if denied.status < 400 {
				t.Fatalf("普通个人自行解除组织上限: %s", denied.describe())
			}
			h.ok(http.MethodPost, "/team/member_delete", admin, map[string]any{"team_id": firstString(other, "team_id"), "user_id": firstString(member, "user_id")})
			h.ok(http.MethodPost, "/v1/chat/completions", firstString(key, "key"), body)
			response = h.do(http.MethodPost, "/v1/chat/completions", tn.key, body)
			if response.status != 429 {
				t.Fatalf("退出重置了旧组织分钟用量: %s", response.describe())
			}
			h.ok(http.MethodPatch, "/organization/update", admin, map[string]any{"organization_id": tn.orgID, dimension: nil})
			h.ok(http.MethodPost, "/user/update", admin, map[string]any{"user_id": tn.userID, dimension: limit})
			second := h.ok(http.MethodPost, "/key/generate", tn.session, map[string]any{"key_alias": "second"}).json()
			response = h.do(http.MethodPost, "/v1/chat/completions", firstString(second, "key"), body)
			// 个人仅调用过一次；第二个 Key 共享最后一次，再次则个人拒绝。
			if response.status != 200 {
				t.Fatalf("个人第二 Key 共享容量: %s", response.describe())
			}
			response = h.do(http.MethodPost, "/v1/chat/completions", tn.key, body)
			if response.status != 429 || !strings.Contains(response.describe(), "user "+dimension) {
				t.Fatalf("跨 Key 个人汇总: %s", response.describe())
			}
		})
	}
}

// TestRateAtomicUpdateHTTP 验证一次请求中的金额、RPM、TPM 和角色整体回滚；前置真实四层账户与本地上游。
// TPM 超配时全部保持原值且原会话仍有效；清空中间层后固定 Key 仍占组织容量，harness 删除隔离 schema。
func TestRateAtomicUpdateHTTP(t *testing.T) {
	h := newHarness(t, chatDeployment("atomic-rate-model"))
	admin := h.adminSession()
	tn := h.provision(t, admin, "atomic-rate")
	h.ok(http.MethodPatch, "/organization/update", admin, map[string]any{"organization_id": tn.orgID, "max_budget": 100, "rpm_limit": 4, "tpm_limit": 128})
	h.ok(http.MethodPost, "/team/update", admin, map[string]any{"team_id": tn.teamID, "max_budget": 100, "rpm_limit": 4, "tpm_limit": 128})
	h.ok(http.MethodPost, "/user/update", admin, map[string]any{"user_id": tn.userID, "max_budget": 50, "rpm_limit": 4, "tpm_limit": 64})
	before := h.ok(http.MethodGet, "/user/info?user_id="+tn.userID, admin, nil).json()["user_info"].(map[string]any)
	response := h.do(http.MethodPost, "/user/update", admin, map[string]any{"user_id": tn.userID, "user_alias": "must-rollback", "user_role": "proxy_admin", "max_budget": 60, "rpm_limit": 3, "tpm_limit": 129})
	if response.status != 400 || !strings.Contains(response.describe(), "rate_allocation_exceeded") {
		t.Fatalf("TPM超配未返回业务错误: %s", response.describe())
	}
	after := h.ok(http.MethodGet, "/user/info?user_id="+tn.userID, admin, nil).json()["user_info"].(map[string]any)
	for _, field := range []string{"user_alias", "user_role", "max_budget", "rpm_limit", "tpm_limit"} {
		if fmt.Sprint(before[field]) != fmt.Sprint(after[field]) {
			t.Fatalf("%s未回滚: before=%v after=%v", field, before[field], after[field])
		}
	}
	// 角色升级失败不能撤销原会话，也不能授予组织额度权限。
	h.ok(http.MethodGet, "/user/info", tn.session, nil)
	if denied := h.do(http.MethodPatch, "/organization/update", tn.session, map[string]any{"organization_id": tn.orgID, "rpm_limit": nil}); denied.status < 400 {
		t.Fatalf("回滚角色仍获得管理权限: %s", denied.describe())
	}
	fixed := h.ok(http.MethodPost, "/key/generate", tn.session, map[string]any{"key_alias": "atomic-fixed", "rpm_limit": 4, "tpm_limit": 64}).json()
	h.ok(http.MethodPost, "/user/update", admin, map[string]any{"user_id": tn.userID, "rpm_limit": nil, "tpm_limit": nil})
	h.ok(http.MethodPost, "/team/update", admin, map[string]any{"team_id": tn.teamID, "rpm_limit": nil, "tpm_limit": nil})
	mark := len(h.upstreamCalls())
	blocked := h.do(http.MethodPost, "/v1/chat/completions", tn.key, chatRequest("atomic-rate-model", "ok"))
	if blocked.status != 429 || !strings.Contains(blocked.describe(), "org rpm_limit") || len(h.upstreamSince(mark)) != 0 {
		t.Fatalf("清空中间层绕过固定后代分配: %s", blocked.describe())
	}
	h.ok(http.MethodPost, "/v1/chat/completions", firstString(fixed, "key"), chatRequest("atomic-rate-model", "ok"))
}
