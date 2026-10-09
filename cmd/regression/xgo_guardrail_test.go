package regression

import (
	"net/http"
	"strings"
	"testing"
)

// Persist through the management API, then exercise the same script through the
// inference gateway. Trial success alone cannot prove request enforcement.
func TestXGoGuardrailEnforcesPersistedPolicy(t *testing.T) {
	deployment := chatDeployment("regression-xgo")
	deployment.ModelInfo = map[string]any{"endpoint_types": []string{"chat"}, "transport": "bypass_openai_chat"}
	h := newHarness(t, deployment)
	admin := h.adminSession()
	tn := h.provision(t, admin, "xgo")
	source := `import . "xhub/guardrail"
func ApplyGuardrail(texts []string, requestData map[string]any, inputType string) map[string]any {
 if inputType != "request" || requestData["model"] != "regression-xgo" { return Block("wrong request context") }
 for text <- texts { if Contains(Lower(text), "secret") { return Block("sensitive content") } }
 return Allow()
}`
	created := h.ok(http.MethodPost, "/guardrails", admin, map[string]any{
		"guardrail": map[string]any{"guardrail_name": "xgo-policy", "litellm_params": map[string]any{
			"guardrail": "custom_code", "custom_code_language": "xgo", "custom_code": source, "mode": "pre_call", "default_on": false,
		}},
	})
	id := stringField(created.json(), "guardrail_id")
	if id == "" {
		t.Fatal("created policy has no persistent ID")
	}
	stored := h.ok(http.MethodGet, "/guardrails/"+id, admin, nil).json()
	params, _ := stored["litellm_params"].(map[string]any)
	if stringField(params, "custom_code") != source || stringField(params, "custom_code_language") != "xgo" {
		t.Fatal("stored XGo policy differs from the submitted source or language")
	}
	invoke := func(text string, selected bool) reply {
		body := map[string]any{"model": "regression-xgo", "messages": []any{map[string]any{"role": "user", "content": text}}}
		if selected {
			body["guardrails"] = []string{"xgo-policy"}
		}
		return h.do(http.MethodPost, "/v1/chat/completions", tn.key, body)
	}
	h.resetUpstream()
	if r := invoke("SECRET", false); r.status != http.StatusOK {
		t.Fatalf("an inactive policy blocked: %s", r.describe())
	}
	if len(h.upstreamCalls()) != 1 {
		t.Fatal("an inactive policy did not reach the upstream once")
	}
	h.flushSpend()
	before, err := h.db.GetUser(t.Context(), tn.userID)
	if err != nil {
		t.Fatal(err)
	}
	h.resetUpstream()
	if r := invoke("SECRET", true); r.status != http.StatusBadRequest {
		t.Fatalf("selected XGo policy failed to block: %s", r.describe())
	}
	if len(h.upstreamCalls()) != 0 {
		t.Fatal("XGo blocked content reached the upstream")
	}
	after, err := h.db.GetUser(t.Context(), tn.userID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Spend != before.Spend {
		t.Fatal("XGo block charged the user")
	}
	blockedLogs := 0
	for _, row := range logsFor(h.spendLogs(t, admin), "regression-xgo") {
		if stringField(row, "status") == "success" {
			continue
		}
		blockedLogs++
		spend, _ := floatField(row, "spend")
		tokens, _ := floatField(row, "total_tokens")
		if spend != 0 || tokens != 0 {
			t.Fatal("XGo refusal recorded billable spend or tokens")
		}
	}
	if blockedLogs != 1 {
		t.Fatalf("XGo block produced %d refusal logs, want one", blockedLogs)
	}
	if r := invoke("ordinary message", true); r.status != http.StatusOK {
		t.Fatalf("XGo failed to allow ordinary content: %s", r.describe())
	}
	if len(h.upstreamCalls()) != 1 {
		t.Fatal("allowed XGo request did not reach upstream exactly once")
	}
	redact := `import . "xhub/guardrail"
func ApplyGuardrail(texts []string, requestData map[string]any, inputType string) map[string]any {
 for i, text := range texts { texts[i] = RegexReplace(text, "SECRET", "[MASK]") }
 return Modify(texts)
}`
	h.ok(http.MethodPatch, "/guardrails/"+id, admin, map[string]any{"litellm_params": map[string]any{"custom_code": redact, "default_on": true}})
	h.resetUpstream()
	if r := invoke("hello SECRET", false); r.status != http.StatusOK {
		t.Fatalf("enabled XGo modification failed: %s", r.describe())
	}
	calls := h.upstreamCalls()
	if len(calls) != 1 {
		t.Fatalf("modified request reached upstream %d times", len(calls))
	}
	body := string(mustJSON(calls[0].Body))
	if strings.Contains(body, "SECRET") || !strings.Contains(body, "hello [MASK]") {
		t.Fatal("upstream received unmodified or unexpected XGo content")
	}
	if len(h.successRows(t, admin, "regression-xgo")) != 3 {
		t.Fatal("allowed and modified requests did not produce exactly three success bills")
	}
	h.ok(http.MethodDelete, "/guardrails/"+id, admin, nil)
	h.resetUpstream()
	if r := invoke("hello SECRET", false); r.status != http.StatusOK {
		t.Fatalf("deleting XGo policy failed to restore service: %s", r.describe())
	}
	calls = h.upstreamCalls()
	if len(calls) != 1 || !strings.Contains(string(mustJSON(calls[0].Body)), "hello SECRET") {
		t.Fatal("deleted XGo policy still modified the upstream request")
	}
}
