package guard

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sunqirui1987/xhub/internal/auth"
	"github.com/sunqirui1987/xhub/internal/store"
	"github.com/sunqirui1987/xhub/internal/testsupport"
)

// savedHost is the process surface the guardrail tests need: a real key-value
// store, and a switch for whether the caller may run a trial.
type savedHost struct {
	store *store.Store
	allow bool
}

func (h *savedHost) RequireManage(w http.ResponseWriter, r *http.Request) *auth.Principal {
	if !h.allow {
		w.WriteHeader(http.StatusUnauthorized)
		return nil
	}
	return &auth.Principal{UserID: "admin"}
}

func (h *savedHost) RecordStore() *store.Store { return h.store }

func openGuardStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(testsupport.Postgres(t, "guard"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() {
		if st.Engine != nil {
			_ = st.Engine.Close()
		}
	})
	return st
}

func putGuardrail(t *testing.T, st *store.Store, id string, row map[string]any) {
	t.Helper()
	raw, err := json.Marshal(row)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.PutKV("guardrails", id, string(raw)); err != nil {
		t.Fatalf("put guardrail: %v", err)
	}
}

func TestGuardrailTextScansAllProtocolFields(t *testing.T) {
	body := map[string]any{
		"text":     "from text",
		"input":    "from input",
		"prompt":   "from prompt",
		"messages": []any{map[string]any{"role": "user", "content": "from messages"}},
	}
	if got := guardrailText(body); got != "from text from input from prompt from messages" {
		t.Fatalf("text wins: %q", got)
	}
	delete(body, "text")
	if got := guardrailText(body); got != "from input from prompt from messages" {
		t.Fatalf("input wins: %q", got)
	}
	delete(body, "input")
	if got := guardrailText(body); got != "from prompt from messages" {
		t.Fatalf("prompt wins: %q", got)
	}
	delete(body, "prompt")
	if got := guardrailText(body); got != "from messages" {
		t.Fatalf("messages: %q", got)
	}
}

func TestGuardrailTextJoinsMessageStringsAndSkipsOtherShapes(t *testing.T) {
	body := map[string]any{
		"messages": []any{
			map[string]any{"role": "system", "content": "be brief"},
			"not a message",
			map[string]any{"role": "user", "content": []any{map[string]any{"type": "text", "text": "bomb"}}},
			map[string]any{"role": "user", "content": "and hello"},
		},
	}
	if got := guardrailText(body); got != "be brief not a message bomb and hello" {
		t.Fatalf("joined: %q", got)
	}
	if got := guardrailText(nil); got != "" {
		t.Fatalf("nil body: %q", got)
	}
}

func TestExtraWordsAcceptsTheShapesASavedRuleUses(t *testing.T) {
	if got := extraWords([]any{"bomb", "", 3, "gun"}); len(got) != 2 || got[0] != "bomb" || got[1] != "gun" {
		t.Fatalf("mixed list: %#v", got)
	}
	if got := extraWords([]string{"a", "b"}); len(got) != 2 {
		t.Fatalf("string list: %#v", got)
	}
	if got := extraWords("one"); len(got) != 1 || got[0] != "one" {
		t.Fatalf("single string: %#v", got)
	}
	if got := extraWords(nil); len(got) != 0 {
		t.Fatalf("nil: %#v", got)
	}
	if got := extraWords(1); len(got) != 0 {
		t.Fatalf("number: %#v", got)
	}
}

func TestBoolOfReadsTheFormsJSONProduces(t *testing.T) {
	if !boolOf(true) || boolOf(false) {
		t.Fatal("bool")
	}
	if !boolOf("true") || !boolOf("1") || boolOf("false") || boolOf("yes") {
		t.Fatal("string")
	}
	if !boolOf(float64(1)) || boolOf(float64(0)) {
		t.Fatal("number")
	}
	if boolOf(nil) || boolOf(1) {
		t.Fatal("missing or int")
	}
}

func TestMatchGuardrailBlocksRedactsAndAllows(t *testing.T) {
	block := map[string]any{
		"litellm_params": map[string]any{
			"guardrail":     "blocked_words",
			"blocked_words": []any{"bomb"},
			"keywords":      []any{"gun"},
		},
		"blocked_words": []any{"knife"},
	}
	action, out := matchGuardrail(block, "a Bomb here")
	if action != "block" || out != "a Bomb here" {
		t.Fatalf("case-insensitive block: %s %q", action, out)
	}
	action, out = matchGuardrail(block, "carry a gun")
	if action != "block" {
		t.Fatalf("keyword: %s", action)
	}
	action, out = matchGuardrail(block, "a knife")
	if action != "block" {
		t.Fatalf("top-level words: %s", action)
	}
	action, out = matchGuardrail(block, "hello")
	if action != "allow" || out != "hello" {
		t.Fatalf("clean: %s %q", action, out)
	}

	redact := map[string]any{"litellm_params": map[string]any{"guardrail": "redact", "blocked_words": []any{"bomb"}}}
	action, out = matchGuardrail(redact, "see bomb now")
	if action != "redact" || out != "see [REDACTED] now" {
		t.Fatalf("redact kind: %s %q", action, out)
	}
	// The match ignores case. The replacement uses the spelling saved on the rule.
	action, out = matchGuardrail(redact, "see Bomb now")
	if action != "redact" || out != "see [REDACTED] now" {
		t.Fatalf("redact must match case-insensitively: %s %q", action, out)
	}

	byMode := map[string]any{"litellm_params": map[string]any{"mode": "redact", "keywords": []any{"ssn"}}}
	action, out = matchGuardrail(byMode, "ssn 123")
	if action != "redact" || out != "[REDACTED] 123" {
		t.Fatalf("redact mode: %s %q", action, out)
	}

	always := map[string]any{"litellm_params": map[string]any{"guardrail": "always_block"}}
	action, out = matchGuardrail(always, "hello")
	if action != "block" || out != "hello" {
		t.Fatalf("always block: %s %q", action, out)
	}

	emptyWord := map[string]any{"litellm_params": map[string]any{"blocked_words": []any{""}}}
	action, _ = matchGuardrail(emptyWord, "hello")
	if action != "allow" {
		t.Fatalf("empty word must not match everything: %s", action)
	}
}

func TestEvaluateReportsEachDefaultRule(t *testing.T) {
	st := openGuardStore(t)
	h := &savedHost{store: st}
	putGuardrail(t, st, "live", map[string]any{
		"guardrail_id":   "live",
		"guardrail_name": "no-bombs",
		"litellm_params": map[string]any{"mode": "pre_call", "default_on": true, "blocked_words": []any{"bomb"}},
	})
	putGuardrail(t, st, "mask", map[string]any{
		"guardrail_name": "mask",
		"litellm_params": map[string]any{"guardrail": "redact", "mode": "pre_call", "default_on": true, "blocked_words": []any{"secret"}},
	})
	putGuardrail(t, st, "off", map[string]any{
		"guardrail_name": "opt-in",
		"litellm_params": map[string]any{"default_on": false, "blocked_words": []any{"secret"}},
	})

	blocked, _, findings := Evaluate(h, map[string]any{"text": "hello"})
	if blocked || len(findings) != 2 {
		t.Fatalf("clean call findings: blocked=%v %#v", blocked, findings)
	}
	for _, row := range findings {
		if row["guardrail_status"] != "success" || row["guardrail_mode"] != "pre_call" {
			t.Fatalf("passed row: %#v", row)
		}
	}

	blocked, message, findings := Evaluate(h, map[string]any{"text": "my secret"})
	if blocked || message != "" || len(findings) != 2 {
		t.Fatalf("redact findings: blocked=%v %q %#v", blocked, message, findings)
	}
	flagged := false
	for _, row := range findings {
		if row["guardrail_name"] == "mask" && row["guardrail_status"] == "guardrail_flagged" {
			flagged = true
		}
		if row["guardrail_name"] == "opt-in" {
			t.Fatal("an off rule was reported as if it ran")
		}
	}
	if !flagged {
		t.Fatalf("redact was not flagged: %#v", findings)
	}
}

func TestPreCallOnlyStopsOnADefaultPreCallBlock(t *testing.T) {
	st := openGuardStore(t)
	h := &savedHost{store: st, allow: true}
	putGuardrail(t, st, "off", map[string]any{
		"guardrail_name": "opt-in",
		"litellm_params": map[string]any{"default_on": false, "blocked_words": []any{"secret"}},
	})
	putGuardrail(t, st, "later", map[string]any{
		"guardrail_name": "after",
		"litellm_params": map[string]any{"mode": "post_call", "default_on": true, "blocked_words": []any{"later"}},
	})
	putGuardrail(t, st, "mask", map[string]any{
		"guardrail_name": "mask",
		"litellm_params": map[string]any{"guardrail": "redact", "mode": "pre_call", "default_on": "true", "blocked_words": []any{"mask"}},
	})
	putGuardrail(t, st, "live", map[string]any{
		"guardrail_name": "no-bombs",
		"default_on":     float64(1),
		"litellm_params": map[string]any{"mode": "pre_call", "blocked_words": []any{"bomb"}},
	})

	if blocked, msg := PreCall(h, map[string]any{"messages": []any{map[string]any{"content": "a secret later mask"}}}); blocked {
		t.Fatalf("words from skipped rules blocked the call: %s", msg)
	}
	blocked, msg := PreCall(h, map[string]any{"prompt": "drop the bomb"})
	if !blocked || msg != "Guardrail blocked the request: no-bombs" {
		t.Fatalf("default rule: blocked=%v msg=%q", blocked, msg)
	}
	if blocked, _ := PreCall(nil, map[string]any{"text": "bomb"}); blocked {
		t.Fatal("a missing host must not block")
	}
	if blocked, _ := PreCall(&savedHost{}, map[string]any{"text": "bomb"}); blocked {
		t.Fatal("a missing store must not block")
	}
}

func TestPreCallNamesAnUnnamedRule(t *testing.T) {
	st := openGuardStore(t)
	h := &savedHost{store: st}
	raw := `{"litellm_params":{"default_on":true,"guardrail":"always_block"}}`
	if err := st.PutKV("guardrail", "bare", raw); err != nil {
		t.Fatal(err)
	}
	blocked, msg := PreCall(h, map[string]any{"text": "anything"})
	if !blocked || msg != "Guardrail blocked the request: guardrail" {
		t.Fatalf("unnamed: blocked=%v msg=%q", blocked, msg)
	}
}

func TestEvalNamedRunsTheAskedRuleAndOtherwiseTheDefault(t *testing.T) {
	st := openGuardStore(t)
	h := &savedHost{store: st}
	putGuardrail(t, st, "id-secret", map[string]any{
		"guardrail_id":   "id-secret",
		"guardrail_name": "secrets",
		"litellm_params": map[string]any{"default_on": false, "blocked_words": []any{"secret"}},
	})
	putGuardrail(t, st, "id-live", map[string]any{
		"guardrail_name": "live",
		"litellm_params": map[string]any{"default_on": true, "blocked_words": []any{"bomb"}},
	})

	action, out := evalNamed(h, "secrets", "a secret")
	if action != "block" || out != "a secret" {
		t.Fatalf("by name: %s %q", action, out)
	}
	action, _ = evalNamed(h, "id-secret", "a secret")
	if action != "block" {
		t.Fatalf("by id: %s", action)
	}
	action, _ = evalNamed(h, "missing", "drop the bomb")
	if action != "block" {
		t.Fatalf("unknown name falls through to the default rule: %s", action)
	}
	action, out = evalNamed(h, "", "hello")
	if action != "allow" || out != "hello" {
		t.Fatalf("no default match: %s %q", action, out)
	}

	only := openGuardStore(t)
	putGuardrail(t, only, "quiet", map[string]any{
		"guardrail_name": "quiet",
		"litellm_params": map[string]any{"default_on": false, "blocked_words": []any{"secret"}},
	})
	action, out = evalNamed(&savedHost{store: only}, "nope", "a secret")
	if action != "allow" || out != "a secret" {
		t.Fatalf("no default rule: %s %q", action, out)
	}
}

func TestApplyTrialsOneRuleWithoutWritingIt(t *testing.T) {
	st := openGuardStore(t)
	h := &savedHost{store: st, allow: true}
	putGuardrail(t, st, "id-secret", map[string]any{
		"guardrail_name": "secrets",
		"litellm_params": map[string]any{"guardrail": "redact", "default_on": false, "blocked_words": []any{"secret"}},
	})
	before, err := st.GetKV("guardrails", "id-secret")
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/guardrails/apply_guardrail", bytes.NewReader([]byte(`{"guardrail_name":"secrets","text":"my secret"}`)))
	rec := httptest.NewRecorder()
	Apply(h, rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["action"] != "redact" || body["blocked"] != false || body["response_text"] != "my [REDACTED]" || body["text"] != "my [REDACTED]" {
		t.Fatalf("trial: %#v", body)
	}
	output, _ := body["output"].(map[string]any)
	if output["text"] != "my [REDACTED]" {
		t.Fatalf("output: %#v", body["output"])
	}
	after, err := st.GetKV("guardrails", "id-secret")
	if err != nil {
		t.Fatal(err)
	}
	if after["guardrail_name"] != before["guardrail_name"] {
		t.Fatalf("trial wrote the row: %#v", after)
	}

	denied := httptest.NewRecorder()
	h.allow = false
	Apply(h, denied, httptest.NewRequest(http.MethodPost, "/apply_guardrail", bytes.NewReader([]byte(`{"guardrail":"secrets","text":"my secret"}`))))
	if denied.Code != http.StatusUnauthorized {
		t.Fatalf("non-admin trial: %d %s", denied.Code, denied.Body.String())
	}
}
