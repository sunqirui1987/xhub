package regression

import (
	"net/http"
	"strings"
	"testing"
)

// TestFallbackAcceptanceSharedModel 验证同名多部署验收模板的真实接口契约与数据面。
// 前置隔离 PostgreSQL、本地429上游与两条同名健康部署，验证非法重复规则返回400、
// 合法模板保存回读、重复回退不缓存、目标名单复核及禁用回退不外发；无测试返回值，
// defer先删除密钥再模板与部署，harness最终删除schema，不使用外部凭据。
func TestFallbackAcceptanceSharedModel(t *testing.T) {
	h := newHarness(t)
	admin := h.adminSession()
	owner := h.openScope(t, admin, "acceptance-fallback")
	const primary, backup = "acceptance-shared", "acceptance-glm"
	ids := []string{"acceptance-fault", "acceptance-fenno", "acceptance-qiniu", "acceptance-backup"}
	upstreams := []string{"acceptance-rate-limit", "acceptance-good-fenno", "acceptance-good-qiniu", "acceptance-good-glm"}
	for i, id := range ids {
		name := primary
		if i == 3 {
			name = backup
		}
		h.addDBModel(t, admin, name, upstreams[i], id, nil)
	}
	h.scriptStatus(upstreams[0], http.StatusTooManyRequests)
	body := routeTemplateBody([]any{
		map[string]any{"model": primary, "strategy": "traffic-split", "allocations": []any{
			map[string]any{"deployment_id": ids[0], "weight": 100},
			map[string]any{"deployment_id": ids[1], "weight": 0},
			map[string]any{"deployment_id": ids[2], "weight": 0},
		}},
		map[string]any{"model": backup, "strategy": "traffic-split", "allocations": []any{
			map[string]any{"deployment_id": ids[3], "weight": 100},
		}},
	}, 1, 60, 0, 60)
	body["fallbacks"] = []any{map[string]any{primary: []any{backup}}}
	invalid := routeTemplateBody([]any{
		map[string]any{"model": primary, "strategy": "traffic-split"},
		map[string]any{"model": primary, "strategy": "traffic-split"},
	}, 1, 60, 0, 60)
	result := h.do(http.MethodPost, "/route_template/new", admin, map[string]any{"name": "invalid acceptance", "body": invalid})
	if result.status != 400 || errorMessage(result) == "" {
		t.Fatalf("重复模型规则应返回明确400: %s", result.describe())
	}
	template := routeTemplate(t, h, admin, "shared acceptance fallback", body)
	bindTemplate(t, h, admin, "key", owner.keyID, template)
	h.ok(http.MethodPost, "/key/update", admin, map[string]any{"key": owner.key, "models": []string{primary, backup}})
	defer func() {
		h.ok(http.MethodPost, "/key/delete", admin, map[string]any{"keys": []string{owner.key}})
		h.ok(http.MethodPost, "/route_template/"+template+"/delete", admin, map[string]any{})
		for _, id := range ids {
			h.ok(http.MethodPost, "/model/delete", admin, map[string]any{"id": id})
		}
	}()
	persisted := templateBody(t, h, admin, template)
	if len(persisted["model_routes"].([]any)) != 2 {
		t.Fatal("跨模型模板未保存两条独立规则")
	}
	request := chatRequest(primary, "repeat acceptance fallback")
	for repeat := 0; repeat < 2; repeat++ {
		mark := len(h.upstreamCalls())
		response := h.ok(http.MethodPost, "/v1/chat/completions", owner.key, request)
		if isCacheHit(response) || strings.Join(h.upstreamSince(mark), ",") != upstreams[0]+","+upstreams[3] {
			t.Fatalf("第%d次必须429回退且排除两条健康部署、禁用缓存: %s calls=%v", repeat+1, response.describe(), h.upstreamSince(mark))
		}
	}
	h.ok(http.MethodPost, "/key/update", admin, map[string]any{"key": owner.key, "models": []string{primary}})
	mark := len(h.upstreamCalls())
	denied := h.do(http.MethodPost, "/v1/chat/completions", owner.key, request)
	if denied.status != 401 || !strings.Contains(errorMessage(denied), "model not in allowed model list") || strings.Join(h.upstreamSince(mark), ",") != upstreams[0] {
		t.Fatalf("备用模型白名单应拒绝且无外发: %s calls=%v", denied.describe(), h.upstreamSince(mark))
	}
	h.ok(http.MethodPost, "/key/update", admin, map[string]any{"key": owner.key, "models": []string{primary, backup}})
	request["disable_fallbacks"] = true
	mark = len(h.upstreamCalls())
	disabled := h.do(http.MethodPost, "/v1/chat/completions", owner.key, request)
	if disabled.status != 502 || strings.Join(h.upstreamSince(mark), ",") != upstreams[0] {
		t.Fatalf("禁用回退只能调用故障部署: %s calls=%v", disabled.describe(), h.upstreamSince(mark))
	}
}
