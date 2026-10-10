package regression

import (
	"fmt"
	"net/http"
	"testing"
)

// TestLiveRouteTemplateSelectionAndBilling 用真实供应商核对模板选择和五层记账。
// 参数 t 为当前测试。返回无。未开启 live 时跳过；开启后缺少供应商配置则失败。
// 调用方是回归入口。精确故障次数和权重周期由确定性上游覆盖，这里只验证选择、推理和账单。
// 模板正文使用当前契约 model_routes 与 retry_policy；旧的顶层 timeout 会被接口拒绝，导致还没拨上游就失败。
func TestLiveRouteTemplateSelectionAndBilling(t *testing.T) {
	if !liveEnabled() {
		t.Skip("set XHUB_REGRESSION_LIVE=1 to call a real model")
	}
	h, admin, c, model := openLiveChat(t, "live-template")
	// 整份文档替换平台默认值。每层都写明 90 秒超时，避免 live 调用落到未声明的默认时限。
	liveBody := routeTemplateBody([]any{}, 1, 90, 0, 0)
	org := routeTemplate(t, h, admin, "live organization", liveBody)
	team := routeTemplate(t, h, admin, "live team", liveBody)
	key := routeTemplate(t, h, admin, "live key", liveBody)
	check := func(label, id, scope, scopeID string) {
		t.Helper()
		effective := effectiveTemplate(t, h, admin, "key", c.keyID)
		if got := stringField(effective, "template_id"); got != id || effective["scope_type"] != scope || stringField(effective, "scope_id") != scopeID {
			t.Fatalf("%s: effective template = %v; want %s at %s/%s", label, effective, id, scope, scopeID)
		}
		t.Logf("live template stage=%s scope=%s/%s template=%s model=%s", label, scope, scopeID, id, model)
		h.assertBilled(t, c, admin, model, fmt.Sprintf("Reply ok. Template stage %s.", label), nil)
	}
	check("platform", "", "builtin", "")
	bindTemplate(t, h, admin, "organization", c.orgID, org)
	check("organization", org, "organization", c.orgID)
	bindTemplate(t, h, admin, "team", c.teamID, team)
	check("team", team, "team", c.teamID)
	bindTemplate(t, h, admin, "key", c.keyID, key)
	check("key", key, "key", c.keyID)
	edited := routeTemplateBody([]any{}, 1, 75, 0, 0)
	h.ok(http.MethodPost, "/route_template/"+key+"/update", admin, map[string]any{"body": edited})
	retry, _ := templateBody(t, h, admin, key)["retry_policy"].(map[string]any)
	if retry["timeout_seconds"] != float64(75) {
		t.Fatalf("edited template timeout_seconds = %v", retry["timeout_seconds"])
	}
	check("edited key", key, "key", c.keyID)
	bindTemplate(t, h, admin, "key", c.keyID, "")
	check("clear key", team, "team", c.teamID)
	bindTemplate(t, h, admin, "team", c.teamID, "")
	check("clear team", org, "organization", c.orgID)
	bindTemplate(t, h, admin, "organization", c.orgID, "")
	check("clear organization", "", "builtin", "")
}
