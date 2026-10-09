package gateway

import (
	"bytes"
	"context"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sunqirui1987/xhub/internal/authz"
	"github.com/sunqirui1987/xhub/internal/cache"
	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/hooks"
	"github.com/sunqirui1987/xhub/internal/iam"
	"github.com/sunqirui1987/xhub/internal/plugin"
)

func TestHandlerDialFailureLogKeepsHostWithoutURL(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()

	// The session is backed by a real account: identity is read from the user
	// row on every request, so a hand-written session record with no row behind
	// it is refused rather than trusted.
	db := testIdentityStore(t)
	user, err := db.CreateUser(context.Background(), testActor, iam.UserInput{
		Email: "dial@example.com", Name: "Dial", Password: "password123", Role: iam.RoleUser,
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
		sessions:   map[string]sessionRec{"session-token": {UserID: user.ID, Version: user.SessionVersion, ExpiresAt: time.Now().Add(time.Hour)}},
		Cfg: &config.Config{
			ModelList: []config.ModelEntry{{
				ModelName: "gpt-4o-mini",
				ModelInfo: map[string]any{"transport": "bypass_openai_chat", "endpoint_types": []string{"chat"}},
				LiteLLMParams: map[string]any{
					"model":    "openai/gpt-4o-mini",
					"api_key":  "sk-local-master",
					"api_base": "http://" + addr + "/v1",
				},
			}},
			RouterSettings: config.RouterSettings{RoutingStrategy: "simple-shuffle", NumRetries: 1},
		},
		Cache: cache.New(),
		Hooks: hooks.New(),
		Busy:  map[string]int{},
		Client: &http.Client{
			Timeout: 2 * time.Second,
		},
	}
	s.extensions = plugin.New()
	s.Handle("POST /v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		s.dataPlane(w, r, "chat")
	})

	var buf bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(prev) })

	body := []byte(`{"model":"gpt-4o-mini","messages":[{"role":"user","content":"hi"}]}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer session-token")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	line := buf.String()
	t.Log(line)
	if !strings.Contains(line, "error POST /v1/chat/completions 502") {
		t.Fatalf("access error log missing: %s", line)
	}
	if !strings.Contains(line, "error upstream dial") || !strings.Contains(line, "127.0.0.1") {
		t.Fatalf("dial log missing host: %s", line)
	}
	if strings.Contains(line, "http://") || strings.Contains(line, "https://") {
		t.Fatalf("log included a full upstream url: %s", line)
	}
	if strings.Contains(line, "sk-local-master") || strings.Contains(line, "Bearer ") || strings.Contains(line, "bearer ") || strings.Contains(line, "sk-") {
		t.Fatalf("log leaked credentials: %s", line)
	}
	if strings.Contains(rec.Body.String(), "http://") || strings.Contains(rec.Body.String(), "https://") {
		t.Fatalf("response body included a full upstream url: %s", rec.Body.String())
	}
}
