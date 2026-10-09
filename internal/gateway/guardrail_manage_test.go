package gateway

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestGuardrailManagementXGoHTTP(t *testing.T) {
	srv, base, db := bootGateway(t)
	defer srv.Close()
	key := loginAdmin(t, base, db)
	code := `import . "xhub/guardrail"
func ApplyGuardrail(texts []string, requestData map[string]any, inputType string) map[string]any {
 if Contains(texts[0], "secret") { return Block("sensitive") }
 return Allow()
}`
	payload, _ := json.Marshal(map[string]any{"custom_code": code, "test_input": map[string]any{"texts": []string{"secret"}}})
	status, body := authed(t, base, key, http.MethodPost, "/guardrails/test_custom_code", payload)
	var trial struct {
		Success bool
		Result  struct {
			Action string
			Reason string
		}
	}
	if status != http.StatusOK || json.Unmarshal(body, &trial) != nil || !trial.Success || trial.Result.Action != "block" || trial.Result.Reason != "sensitive" {
		t.Fatalf("XGo test route: %d %s", status, trim(body))
	}
	status, body = authed(t, base, key, http.MethodGet, "/guardrails/submissions", nil)
	var submissions struct {
		Supported   bool
		Submissions []any
		Summary     map[string]int
	}
	if status != http.StatusOK || json.Unmarshal(body, &submissions) != nil || submissions.Supported || submissions.Submissions == nil || submissions.Summary == nil {
		t.Fatalf("submissions route: %d %s", status, trim(body))
	}
	payload, _ = json.Marshal(map[string]any{"guardrail": map[string]any{"guardrail_name": "xgo-http", "litellm_params": map[string]any{"guardrail": "custom_code", "custom_code_language": "xgo", "mode": "pre_call", "custom_code": code}}})
	status, body = authed(t, base, key, http.MethodPost, "/guardrails", payload)
	var saved struct {
		GuardrailID string `json:"guardrail_id"`
	}
	if status != http.StatusOK || json.Unmarshal(body, &saved) != nil || saved.GuardrailID == "" {
		t.Fatalf("save script: %d %s", status, trim(body))
	}
	status, body = authed(t, base, key, http.MethodDelete, "/guardrails/"+saved.GuardrailID, nil)
	if status != http.StatusOK {
		t.Fatalf("delete script: %d %s", status, trim(body))
	}
	resp, err := http.Post(base+"/guardrails/test_custom_code", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized && resp.StatusCode != http.StatusForbidden {
		t.Fatalf("script endpoint unauthenticated: %d", resp.StatusCode)
	}
}
