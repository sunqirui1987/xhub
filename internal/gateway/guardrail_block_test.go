package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/sunqirui1987/xhub/cmd/regression/testsupport"
	"github.com/sunqirui1987/xhub/internal/authz"
	"github.com/sunqirui1987/xhub/internal/cache"
	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/hooks"
	"github.com/sunqirui1987/xhub/internal/iam"
	"github.com/sunqirui1987/xhub/internal/plugin"
	"github.com/sunqirui1987/xhub/internal/store"
)

// TestChatStopsBeforeUpstreamWhenADefaultGuardrailBlocks is the data-plane
// half of a guardrail. A default-on pre-call rule refuses the chat itself, and
// the provider is not dialed. A word that belongs only to a rule that is off,
// or to a rule that runs after the call, still goes through.
func TestChatStopsBeforeUpstreamWhenADefaultGuardrailBlocks(t *testing.T) {
	st, err := store.Open(testsupport.Postgres(t, "guardrail"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() {
		if st.Engine != nil {
			_ = st.Engine.Close()
		}
	})
	put := func(id string, row map[string]any) {
		t.Helper()
		raw, err := json.Marshal(row)
		if err != nil {
			t.Fatal(err)
		}
		if err := st.PutKV("guardrails", id, string(raw)); err != nil {
			t.Fatalf("put %s: %v", id, err)
		}
	}
	put("live", map[string]any{
		"guardrail_name": "no-bombs",
		"litellm_params": map[string]any{
			"guardrail": "blocked_words", "mode": "pre_call", "default_on": true,
			"blocked_words": []any{"bomb"},
		},
	})
	put("opt", map[string]any{
		"guardrail_name": "opt-in",
		"litellm_params": map[string]any{"default_on": false, "blocked_words": []any{"secret"}},
	})
	put("after", map[string]any{
		"guardrail_name": "after",
		"litellm_params": map[string]any{"mode": "post_call", "default_on": true, "blocked_words": []any{"later"}},
	})

	captured := make(chan map[string]any, 4)
	var hits atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		captured <- request
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"x","object":"chat.completion","choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
	}))
	t.Cleanup(upstream.Close)

	db := testIdentityStore(t)
	user, err := db.CreateUser(context.Background(), testActor, iam.UserInput{
		Email: "guard@example.com", Name: "Guard", Password: "password123", Role: iam.RoleUser,
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	s := &Server{
		engine:     newEngine(),
		registered: map[string]struct{}{},
		idem:       map[string]*idemRec{},
		IAM:        db,
		Authz:      authz.New(db),
		Store:      st,
		sessions:   map[string]sessionRec{},
		Cfg: &config.Config{
			ModelList: []config.ModelEntry{{
				ModelName: "gpt-4o-mini",
				ModelInfo: map[string]any{"transport": "bypass_openai_chat", "endpoint_types": []string{"chat"}},
				LiteLLMParams: map[string]any{
					"model":    "openai/gpt-4o-mini",
					"api_key":  "sk-test",
					"api_base": upstream.URL + "/v1",
				},
			}},
			RouterSettings: config.RouterSettings{RoutingStrategy: "simple-shuffle", NumRetries: 0},
		},
		Cache:  cache.New(),
		Hooks:  hooks.New(),
		Busy:   map[string]int{},
		Client: upstream.Client(),
	}
	s.extensions = plugin.New()
	if !s.rememberSession("sess-guard", user.ID, user.SessionVersion) {
		t.Fatal("persist session")
	}
	token := signSessionJWT("sess-guard", user.ID, user.SessionVersion, user.Role, user.Email, "")
	s.Handle("POST /v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		s.dataPlane(w, r, "chat")
	})

	call := func(content string) *httptest.ResponseRecorder {
		t.Helper()
		raw, _ := json.Marshal(map[string]any{
			"model":    "gpt-4o-mini",
			"messages": []any{map[string]any{"role": "user", "content": content}},
		})
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(raw))
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		return rec
	}

	blocked := call("drop the bomb")
	if blocked.Code != http.StatusBadRequest || !bytes.Contains(blocked.Body.Bytes(), []byte("guardrail_failed")) || !bytes.Contains(blocked.Body.Bytes(), []byte("no-bombs")) {
		t.Fatalf("blocked chat: %d %s", blocked.Code, blocked.Body.String())
	}
	if hits.Load() != 0 {
		t.Fatalf("provider was called %d times for a blocked chat", hits.Load())
	}

	allowed := call("a secret later")
	if allowed.Code == http.StatusBadRequest && bytes.Contains(allowed.Body.Bytes(), []byte("guardrail_failed")) {
		t.Fatalf("an off rule blocked the chat: %s", allowed.Body.String())
	}
	if hits.Load() != 1 {
		t.Fatalf("provider hits after the allowed chat: %d body %s", hits.Load(), allowed.Body.String())
	}

	events, err := db.ListUsage(context.Background(), iam.UsageQuery{Limit: 10})
	if err != nil {
		t.Fatalf("list logs: %v", err)
	}
	var sawBlock, sawPass bool
	for _, event := range events {
		switch event.Status {
		case "error":
			if !bytes.Contains([]byte(event.Guardrail), []byte(`"guardrail_name":"no-bombs"`)) || !bytes.Contains([]byte(event.Guardrail), []byte(`"guardrail_status":"blocked"`)) || !bytes.Contains([]byte(event.Guardrail), []byte(`"reason":"Guardrail blocked the request: no-bombs"`)) {
				t.Fatalf("blocked log: %s", event.Guardrail)
			}
			sawBlock = true
		case "success":
			if !bytes.Contains([]byte(event.Guardrail), []byte(`"guardrail_status":"success"`)) {
				t.Fatalf("allowed log: %s", event.Guardrail)
			}
			sawPass = true
		}
	}
	if !sawBlock || !sawPass {
		t.Fatalf("logs missing a guardrail outcome: blocked=%v passed=%v", sawBlock, sawPass)
	}
	<-captured // The earlier allowed request.
	put("xgo", map[string]any{
		"guardrail_name": "xgo-policy",
		"litellm_params": map[string]any{
			"guardrail": "custom_code", "custom_code_language": "xgo", "mode": "pre_call", "default_on": true,
			"custom_code": `import . "xhub/guardrail"
func ApplyGuardrail(texts []string, requestData map[string]any, inputType string) map[string]any {
 for i, text := range texts {
  if Contains(text, "credential") { return Block("script rejected credential") }
  texts[i] = RegexReplace(text, "1[3-9][0-9]{9}", "[PHONE]")
 }
 return Modify(texts)
}`,
		},
	})
	blocked = call("credential")
	if blocked.Code != http.StatusBadRequest || !bytes.Contains(blocked.Body.Bytes(), []byte("script rejected credential")) || hits.Load() != 1 {
		t.Fatalf("XGo failed to stop upstream: %d hits=%d %s", blocked.Code, hits.Load(), blocked.Body.String())
	}
	allowed = call("contact 13812345678")
	if allowed.Code != http.StatusOK || hits.Load() != 2 {
		t.Fatalf("XGo modified call: %d %s", allowed.Code, allowed.Body.String())
	}
	forwarded := <-captured
	messages, ok := forwarded["messages"].([]any)
	if !ok || len(messages) != 1 || messages[0].(map[string]any)["content"] != "contact [PHONE]" {
		t.Fatalf("original sensitive text sent upstream: %#v", forwarded)
	}

}
