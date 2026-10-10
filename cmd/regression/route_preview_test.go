package regression

import (
	"net/http"
	"testing"
)

// TestRoutePreviewMatchesInference 验证组织/团队/密钥继承的预览和真实推理对应，预览不外发、不计费、不消耗限流。
// 前置私有 PostgreSQL、独立 Redis、真实网关及本地上游；显式模板/草稿仍拒绝密钥管理权限。
// harness 删除整个私有 schema 并关闭服务，无外部凭据或已有用户数据。
func TestRoutePreviewMatchesInference(t *testing.T) {
	const model = "preview-correspondence"
	h := newHarness(t, deployment(model, "openai/preview-a", nil), deployment(model, "openai/preview-b", nil))
	admin := h.adminSession()
	c := h.openScope(t, admin, "preview-correspondence")
	// makeTemplate 生成确定性模板；name 为名称，chosen 为唯一正权重部署，返回 ID，私有 schema 清理。
	makeTemplate := func(name, chosen string) string {
		weights := map[string]float64{"preview-a": 0, "preview-b": 0}
		weights[chosen] = 1
		return routeTemplate(t, h, admin, name, routeTemplateBody([]any{map[string]any{"model": model, "strategy": "traffic-split", "allocations": routeAllocations(weights)}}, 1, 60, 0, 0))
	}
	org, team, key := makeTemplate("preview org", "preview-a"), makeTemplate("preview team", "preview-b"), makeTemplate("preview key", "preview-a")
	// preview 核对指定凭据的来源及可用部署，并用前后快照验证只读；返回无，无独立资源。
	preview := func(credential, id, source, chosen string) {
		t.Helper()
		before := h.moneyOf(t, c)
		mark := len(h.upstreamCalls())
		for i := 0; i < 3; i++ {
			out := h.ok(http.MethodPost, "/route_template/preview", credential, map[string]any{"model_name": model, "endpoint_id": "chat"}).json()
			if out["template_id"] != id || out["source"] != source {
				t.Fatalf("预览归属不一致 want=%s/%s got=%v", id, source, out)
			}
			if chosen != "" {
				available := []string{}
				for _, row := range listField(out, "data") {
					if reason, _ := row["excluded_reason"].(string); reason == "" {
						available = append(available, stringField(row, "deployment_id"))
					}
				}
				if len(available) != 1 || available[0] != chosen {
					t.Fatalf("预览可用部署=%v want=%s", available, chosen)
				}
			}
		}
		if len(h.upstreamCalls()) != mark || !h.moneyOf(t, c).same(before) {
			t.Fatal("预览外发或改变账单")
		}
	}
	for _, b := range []struct{ scope, id, template, chosen string }{{"organization", c.orgID, org, "preview-a"}, {"team", c.teamID, team, "preview-b"}} {
		bindTemplate(t, h, admin, b.scope, b.id, b.template)
		preview(c.key, b.template, b.scope, b.chosen)
		preview(c.session, b.template, b.scope, b.chosen)
		h.assertBilled(t, c, admin, model, "key-"+b.scope, []string{b.chosen})
		h.resetUpstream()
		r := h.ok(http.MethodPost, "/v1/chat/completions", c.session, chatRequest(model, "session-"+b.scope))
		if calls := h.upstreamCalls(); len(calls) != 1 || calls[0].Body["model"] != b.chosen || parseFloatOrZero(r.header("x-litellm-response-cost")) <= 0 {
			t.Fatalf("会话推理偏离预览或未计费: %v %s", calls, r.describe())
		}
	}
	bindTemplate(t, h, admin, "key", c.keyID, key)
	h.ok(http.MethodPost, "/key/update", admin, map[string]any{"key": c.key, "rpm_limit": 1, "tpm_limit": 100})
	preview(c.key, key, "key", "preview-a")
	h.assertBilled(t, c, admin, model, "key-priority", []string{"preview-a"})
	preview(c.session, team, "team", "preview-b")
	personal := h.keyWith(t, c.session, map[string]any{"key_alias": "preview-personal"})
	preview(personal, "", "builtin", "")
	for _, input := range []map[string]any{{"model_name": model, "endpoint_id": "chat", "template_id": key}, {"model_name": model, "endpoint_id": "chat", "body": routeTemplateBody([]any{}, 1, 60, 0, 0)}} {
		if r := h.do(http.MethodPost, "/route_template/preview", c.key, input); r.status != 403 {
			t.Fatalf("密钥获得模板管理权限: %s", r.describe())
		}
	}
	for _, input := range []map[string]any{{"model_name": model, "endpoint_id": "unknown"}, {"model_name": "", "endpoint_id": "chat"}} {
		if r := h.do(http.MethodPost, "/route_template/preview", c.session, input); r.status != 400 {
			t.Fatalf("非法预览输入未返回400: %s", r.describe())
		}
	}
	if r := h.do(http.MethodPost, "/route_template/preview", "", map[string]any{"model_name": model, "endpoint_id": "chat"}); r.status != 401 {
		t.Fatalf("匿名预览未拒绝: %s", r.describe())
	}
	h.ok(http.MethodPost, "/key/update", admin, map[string]any{"key": c.key, "models": []string{"another-model"}})
	if r := h.do(http.MethodPost, "/route_template/preview", c.key, map[string]any{"model_name": model, "endpoint_id": "chat"}); r.status != 403 {
		t.Fatalf("预览绕过模型名单: %s", r.describe())
	}
}
