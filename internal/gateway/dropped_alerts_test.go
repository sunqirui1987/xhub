package gateway

import (
	"encoding/json"
	"net/http"
	"testing"
)

// TestDroppedAlertsAreGone checks the products that left the logging page.
// CloudZero, email events, and alerting settings answer 404. Saving an alert
// destination or probing one answers 400. Vantage stays mounted。
func TestDroppedAlertsAreGone(t *testing.T) {
	srv, base, db := bootGateway(t)
	defer srv.Close()
	key := loginAdmin(t, base, db)

	for _, path := range []string{
		"/cloudzero/settings",
		"/cloudzero/export",
		"/email/event_settings",
		"/email/event_settings/reset",
	} {
		status, body := authed(t, base, key, http.MethodGet, path, nil)
		if status != http.StatusNotFound {
			t.Fatalf("GET %s status %d %s", path, status, trim(body))
		}
	}
	status, body := authed(t, base, key, http.MethodPost, "/cloudzero/init", []byte(`{"api_key":"cz","connection_id":"c"}`))
	if status != http.StatusNotFound {
		t.Fatalf("POST /cloudzero/init status %d %s", status, trim(body))
	}
	status, body = authed(t, base, key, http.MethodPatch, "/email/event_settings", []byte(`{"settings":[]}`))
	if status != http.StatusNotFound {
		t.Fatalf("PATCH /email/event_settings status %d %s", status, trim(body))
	}

	for _, dest := range []string{"email", "ms_teams", "slack"} {
		payload := []byte(`{"general_settings":{"alerting":["` + dest + `"]}}`)
		status, body = authed(t, base, key, http.MethodPost, "/config/update", payload)
		if status != http.StatusBadRequest {
			t.Fatalf("alerting %s status %d %s", dest, status, trim(body))
		}
	}
	status, body = authed(t, base, key, http.MethodGet, "/get/config/callbacks", nil)
	if status != http.StatusOK {
		t.Fatalf("callbacks status %d %s", status, trim(body))
	}
	var callbacks struct {
		Alerts []any `json:"alerts"`
	}
	if err := json.Unmarshal(body, &callbacks); err != nil {
		t.Fatal(err)
	}
	if len(callbacks.Alerts) != 0 {
		t.Fatalf("alerts = %#v", callbacks.Alerts)
	}
	for _, service := range []string{"email", "ms_teams", "slack"} {
		status, body = authed(t, base, key, http.MethodGet, "/health/services?service="+service, nil)
		if status != http.StatusBadRequest {
			t.Fatalf("health %s status %d %s", service, status, trim(body))
		}
	}

	status, body = authed(t, base, key, http.MethodGet, "/vantage/settings", nil)
	if status != http.StatusOK {
		t.Fatalf("vantage status %d %s", status, trim(body))
	}
	status, body = authed(t, base, key, http.MethodGet, "/alerting/settings", nil)
	if status != http.StatusNotFound {
		t.Fatalf("alerting settings status %d %s", status, trim(body))
	}
}

// TestUnusedAdminSurfacesStayGone checks the settings that never affected a
// request: cost discounts and margins, the price calculator, SSO login, the IP
// allow-list, SCIM, and the vault backends. Prompt storage and the UI settings
// alias stay, because spend logs and the sidebar still read them.
func TestUnusedAdminSurfacesStayGone(t *testing.T) {
	srv, base, db := bootGateway(t)
	defer srv.Close()
	key := loginAdmin(t, base, db)

	for _, item := range []struct {
		method, path string
	}{
		{http.MethodGet, "/config/cost_discount_config"},
		{http.MethodPatch, "/config/cost_margin_config"},
		{http.MethodGet, "/config/block_requests_for_models_without_pricing"},
		{http.MethodPost, "/cost/estimate"},
		{http.MethodGet, "/config_overrides/hashicorp_vault"},
		{http.MethodGet, "/config_overrides/cyberark"},
		{http.MethodGet, "/get/sso_settings"},
		{http.MethodGet, "/sso/key/generate"},
		{http.MethodGet, "/get/allowed_ips"},
		{http.MethodGet, "/Users"},
		{http.MethodGet, "/Groups"},
	} {
		status, body := authed(t, base, key, item.method, item.path, []byte(`{}`))
		if status != http.StatusNotFound {
			t.Fatalf("%s %s status %d %s", item.method, item.path, status, trim(body))
		}
	}

	status, body := authed(t, base, key, http.MethodGet, "/sso/get/ui_settings", nil)
	if status != http.StatusOK {
		t.Fatalf("ui settings alias status %d %s", status, trim(body))
	}
	status, body = authed(t, base, key, http.MethodPost, "/config/update", []byte(`{"general_settings":{"store_prompts_in_spend_logs":true}}`))
	if status != http.StatusOK {
		t.Fatalf("prompt storage status %d %s", status, trim(body))
	}
}
