package regression

import (
	"net/http"
	"testing"
)

// TestRemovedSurfacesStayGone 确认已经从产品拿掉的设置不再提供接口。
// 折扣、加成、价格计算器、SSO、IP 白名单、SCIM 和密钥库都只是把配置存下来，
// 请求路径不读它们。界面开关还在，一次聊天仍然按价目表计费。
func TestRemovedSurfacesStayGone(t *testing.T) {
	h := newHarness(t, chatDeployment("kept-model"))
	admin := h.adminSession()

	gone := []struct{ method, path string }{
		{http.MethodGet, "/config/cost_discount_config"},
		{http.MethodPatch, "/config/cost_margin_config"},
		{http.MethodPost, "/cost/estimate"},
		{http.MethodGet, "/config_overrides/hashicorp_vault"},
		{http.MethodGet, "/config_overrides/cyberark"},
		{http.MethodGet, "/get/sso_settings"},
		{http.MethodGet, "/sso/key/generate"},
		{http.MethodGet, "/get/allowed_ips"},
		{http.MethodGet, "/Users"},
		{http.MethodGet, "/Groups"},
		{http.MethodGet, "/cloudzero/settings"},
		{http.MethodGet, "/email/event_settings"},
		{http.MethodGet, "/alerting/settings"},
		{http.MethodPost, "/config/callback/delete"},
	}
	for _, item := range gone {
		got := h.do(item.method, item.path, admin, map[string]any{})
		if got.status != http.StatusNotFound {
			t.Fatalf("%s %s status %d %s", item.method, item.path, got.status, got.text())
		}
	}

	h.ok(http.MethodGet, "/sso/get/ui_settings", admin, nil)

	for _, dest := range []string{"slack", "email", "ms_teams"} {
		got := h.do(http.MethodPost, "/config/update", admin, map[string]any{
			"general_settings": map[string]any{"alerting": []any{dest}},
		})
		if got.status != http.StatusBadRequest {
			t.Fatalf("alerting %s status %d %s", dest, got.status, got.text())
		}
	}
	callbacks := h.ok(http.MethodGet, "/get/config/callbacks", admin, nil).json()
	if alerts, _ := callbacks["alerts"].([]any); len(alerts) != 0 {
		t.Fatalf("callbacks still advertise alerts: %s", mustJSON(callbacks["alerts"]))
	}
	if _, ok := callbacks["router_settings"].(map[string]any); !ok {
		t.Fatalf("router settings disappeared with the alerts: %s", mustJSON(callbacks))
	}

	c := h.openScope(t, admin, "kept")
	h.assertBilled(t, c, admin, "kept-model", "ping", nil)
}
