package gateway

import (
	"encoding/json"
	"net/http"
	"testing"
)

func compliantPayload() map[string]any {
	return map[string]any{
		"request_id": "req-1",
		"user_id":    "user-1",
		"model":      "gpt-4o",
		"timestamp":  "2026-10-08T00:00:00Z",
		"guardrail_information": []any{
			map[string]any{"guardrail_name": "pii", "guardrail_mode": "pre_call", "guardrail_status": "success"},
			map[string]any{"guardrail_name": "skipped", "guardrail_status": "not_run"},
		},
	}
}

func TestEUAIActWithoutGuardrailsFailsClosed(t *testing.T) {
	checks := euAIActChecks(map[string]any{"request_id": "req-1"})
	if len(checks) != 3 {
		t.Fatalf("checks = %d", len(checks))
	}
	for _, check := range checks {
		if check.Passed {
			t.Fatalf("expected failure, got %+v", check)
		}
	}
	if checks[2].Detail != "Missing: user_id, model, timestamp, guardrail_results" {
		t.Fatalf("audit detail = %q", checks[2].Detail)
	}
}

func TestCompliancePassesWhenPreCallSucceeds(t *testing.T) {
	body := compliantPayload()
	for _, check := range euAIActChecks(body) {
		if !check.Passed {
			t.Fatalf("eu check failed: %+v", check)
		}
	}
	for _, check := range gdprChecks(body) {
		if !check.Passed {
			t.Fatalf("gdpr check failed: %+v", check)
		}
	}
	if gdprChecks(body)[1].Detail != "No sensitive data detected" {
		t.Fatalf("sensitive detail = %q", gdprChecks(body)[1].Detail)
	}
}

func TestMixedGuardrailModeDoesNotCountAsPreCall(t *testing.T) {
	body := compliantPayload()
	body["guardrail_information"] = []any{
		map[string]any{"guardrail_mode": []any{"pre_call", "post_call"}, "guardrail_status": "success"},
	}
	screened := euAIActChecks(body)[1]
	if screened.Passed {
		t.Fatalf("mixed mode counted as pre-call: %+v", screened)
	}

	body["guardrail_information"] = []any{
		map[string]any{"guardrail_mode": []any{"pre_call"}, "guardrail_status": "blocked"},
	}
	if !euAIActChecks(body)[1].Passed {
		t.Fatal("single-mode list was not counted as pre-call")
	}
	sensitive := gdprChecks(body)[1]
	if !sensitive.Passed || sensitive.Detail != "Guardrail intervened to protect sensitive data" {
		t.Fatalf("intervention = %+v", sensitive)
	}
}

func TestSingleGuardrailObjectIsAccepted(t *testing.T) {
	body := compliantPayload()
	body["guardrail_information"] = map[string]any{
		"guardrail_mode":   "pre_call",
		"guardrail_status": "success",
	}
	if got := len(activeGuardrails(body)); got != 1 {
		t.Fatalf("active = %d", got)
	}
}

func TestComplianceEndpointsReturnChecks(t *testing.T) {
	srv, base, db := bootGateway(t)
	defer srv.Close()
	key := loginAdmin(t, base, db)

	payload, _ := json.Marshal(compliantPayload())
	for _, path := range []string{"/compliance/eu-ai-act", "/compliance/gdpr"} {
		status, resp := authed(t, base, key, http.MethodPost, path, payload)
		if status != http.StatusOK {
			t.Fatalf("POST %s status %d %s", path, status, trim(resp))
		}
		var parsed complianceResponse
		if err := json.Unmarshal(resp, &parsed); err != nil {
			t.Fatal(err)
		}
		if !parsed.Compliant || len(parsed.Checks) != 3 {
			t.Fatalf("POST %s body %s", path, trim(resp))
		}
		if parsed.Checks[0].CheckName == "" {
			t.Fatalf("POST %s missing check name %s", path, trim(resp))
		}
	}

	status, resp := authed(t, base, key, http.MethodPost, "/compliance/eu-ai-act", []byte(`{}`))
	if status != http.StatusBadRequest {
		t.Fatalf("missing request_id status %d %s", status, trim(resp))
	}
}
