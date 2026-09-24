package server

import (
	"encoding/json"
	"testing"
)

func TestEmailEventSettingsDefaultsAndUpdate(t *testing.T) {
	s, master := testEnv(t)
	h := s.Handler()
	got := doJSON(t, h, "GET", "/email/event_settings", master, nil)
	if got.Code != 200 {
		t.Fatalf("get %d %s", got.Code, got.Body.String())
	}
	var body struct {
		Settings []emailEventSetting `json:"settings"`
	}
	if err := json.Unmarshal(got.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Settings) != 5 || !body.Settings[1].Enabled || body.Settings[1].Event != "New User Invitation" {
		t.Fatalf("defaults %#v", body.Settings)
	}
	upd := doJSON(t, h, "PATCH", "/email/event_settings", master, map[string]any{
		"settings": []map[string]any{{"event": "New User Invitation", "enabled": false}},
	})
	if upd.Code != 200 {
		t.Fatalf("patch %d %s", upd.Code, upd.Body.String())
	}
	again := doJSON(t, h, "GET", "/email/event_settings", master, nil)
	if err := json.Unmarshal(again.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Settings) != 1 || body.Settings[0].Enabled {
		t.Fatalf("updated %#v", body.Settings)
	}
	reset := doJSON(t, h, "POST", "/email/event_settings/reset", master, map[string]any{})
	if reset.Code != 200 {
		t.Fatalf("reset %d %s", reset.Code, reset.Body.String())
	}
}
