package gateway

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
)

func TestRetiredRouterPreferencesReturnBadRequest(t *testing.T) {
	srv, base, db := bootGateway(t)
	defer srv.Close()
	key := loginAdmin(t, base, db)

	retired := []string{"retry_policy", "model_group_retry_policy", "model_group_alias"}
	for _, field := range retired {
		t.Run("config update "+field, func(t *testing.T) {
			payload := []byte(fmt.Sprintf(`{"router_settings":{"%s":{}}}`, field))
			status, body := authed(t, base, key, http.MethodPost, "/config/update", payload)
			assertInvalidPreference(t, status, body, field)
		})

		t.Run("field update "+field, func(t *testing.T) {
			payload := []byte(fmt.Sprintf(`{"config_type":"router_settings","field_name":"%s","field_value":{}}`, field))
			status, body := authed(t, base, key, http.MethodPost, "/config/field/update", payload)
			assertInvalidPreference(t, status, body, field)
		})
	}

	status, body := authed(t, base, key, http.MethodPost, "/config/field/update", []byte(`{"config_type":"router_settings","field_name":"routing_strategy","field_value":"simple-shuffle"}`))
	if status != http.StatusOK {
		t.Fatalf("supported router preference status %d %s", status, trim(body))
	}
}

func assertInvalidPreference(t *testing.T, status int, body []byte, field string) {
	t.Helper()
	if status != http.StatusBadRequest {
		t.Fatalf("%s status %d %s", field, status, trim(body))
	}
	var response struct {
		Error struct {
			Type string `json:"type"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatalf("decode %s response: %v: %s", field, err, trim(body))
	}
	if response.Error.Type != "invalid_request" {
		t.Fatalf("%s error type %q: %s", field, response.Error.Type, trim(body))
	}
}
