package suite

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sunqirui1987/xhub/internal/config"
)

func TestChatCompletionsPreflightAllowsBrowserSDK(t *testing.T) {
	s, _ := testEnv(t)
	req := httptest.NewRequest(http.MethodOptions, "/chat/completions", nil)
	req.Header.Set("Origin", "http://127.0.0.1:3000")
	req.Header.Set("Access-Control-Request-Method", "POST")
	req.Header.Set("Access-Control-Request-Headers", "authorization,content-type,x-stainless-os,x-stainless-runtime,x-stainless-arch,x-stainless-lang,x-stainless-timeout,x-stainless-package-version,x-stainless-runtime-version,x-stainless-retry-count")
	req.Header.Set("Access-Control-Request-Private-Network", "true")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("preflight status %d", rec.Code)
	}
	if rec.Header().Get("Access-Control-Allow-Origin") != "http://127.0.0.1:3000" {
		t.Fatalf("origin %q", rec.Header().Get("Access-Control-Allow-Origin"))
	}
	if rec.Header().Get("Access-Control-Allow-Private-Network") != "true" {
		t.Fatal("missing private network allow")
	}
	allowed := strings.ToLower(rec.Header().Get("Access-Control-Allow-Headers"))
	for _, name := range []string{"authorization", "content-type", "x-stainless-runtime", "x-stainless-timeout"} {
		if !strings.Contains(allowed, name) {
			t.Fatalf("allow-headers %q missing %s", allowed, name)
		}
	}
}

func TestNoAuthChat401(t *testing.T) {
	s, _ := testEnv(t)
	rec := doJSON(t, s.Handler(), "POST", "/v1/chat/completions", "", map[string]any{
		"model": "gpt-4o-mini", "messages": []any{map[string]any{"role": "user", "content": "hi"}},
	})
	if rec.Code != 401 {
		t.Fatalf("code %d body %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"error"`) {
		t.Fatalf("envelope %s", rec.Body.String())
	}
}

func TestMasterKeyCannotChat(t *testing.T) {
	s, master := testEnv(t)
	rec := doJSON(t, s.Handler(), "POST", "/v1/chat/completions", master, map[string]any{
		"model": "gpt-4o-mini", "messages": []any{map[string]any{"role": "user", "content": "hi"}},
	})
	if rec.Code != 401 {
		t.Fatalf("code %d body %s", rec.Code, rec.Body.String())
	}
}

func TestGenerateAndChat(t *testing.T) {
	s, master := testEnv(t)
	gen := doJSON(t, s.Handler(), "POST", "/key/generate", master, map[string]any{
		"key_alias": "t", "models": []string{"gpt-4o-mini"}, "key_type": "llm_api",
	})
	if gen.Code != 200 {
		t.Fatalf("generate %d %s", gen.Code, gen.Body.String())
	}
	var g map[string]any
	_ = json.Unmarshal(gen.Body.Bytes(), &g)
	sk, _ := g["key"].(string)
	if !strings.HasPrefix(sk, "sk-") {
		t.Fatalf("plain key %v", g)
	}
	list := doJSON(t, s.Handler(), "GET", "/key/list", master, nil)
	if strings.Contains(list.Body.String(), sk) {
		t.Fatalf("list leaked plaintext")
	}
	rec := doJSON(t, s.Handler(), "POST", "/v1/chat/completions", sk, map[string]any{
		"model": "gpt-4o-mini", "messages": []any{map[string]any{"role": "user", "content": "hi"}},
	})
	if rec.Code != 200 {
		t.Fatalf("chat %d %s", rec.Code, rec.Body.String())
	}
	if rec.Result().Header.Get("x-litellm-call-id") == "" {
		t.Fatal("missing call-id")
	}
	if rec.Result().Header.Get("x-litellm-response-cost") == "" {
		t.Fatal("missing cost header")
	}
	var chat map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &chat)
	if chat["model"] != "gpt-4o-mini" {
		t.Fatalf("alias model %v", chat["model"])
	}
	info := doJSON(t, s.Handler(), "GET", "/key/info?key="+sk, master, nil)
	if !strings.Contains(info.Body.String(), `"spend"`) {
		t.Fatalf("info %s", info.Body.String())
	}
	var inf map[string]any
	_ = json.Unmarshal(info.Body.Bytes(), &inf)
	infoObj := inf["info"].(map[string]any)
	if infoObj["spend"].(float64) <= 0 {
		t.Fatalf("spend not incremented %v", infoObj["spend"])
	}
	assertHashedAPIKey(t, spendRows(t, s, master))
}

func TestBudgetExceeded(t *testing.T) {
	s, master := testEnv(t)
	gen := doJSON(t, s.Handler(), "POST", "/key/generate", master, map[string]any{
		"max_budget": 0, "key_type": "llm_api",
	})
	var g map[string]any
	_ = json.Unmarshal(gen.Body.Bytes(), &g)
	sk := g["key"].(string)
	rec := doJSON(t, s.Handler(), "POST", "/v1/chat/completions", sk, map[string]any{
		"model": "gpt-4o-mini", "messages": []any{map[string]any{"role": "user", "content": "hi"}},
	})
	if rec.Code != 429 {
		t.Fatalf("want 429 got %d %s", rec.Code, rec.Body.String())
	}
	if rec.Result().Header.Get("Retry-After") == "" {
		t.Fatal("missing Retry-After")
	}
}

func TestChatAliasAndStream(t *testing.T) {
	s, master := testEnv(t)
	gen := doJSON(t, s.Handler(), "POST", "/key/generate", master, map[string]any{"key_type": "llm_api"})
	var g map[string]any
	_ = json.Unmarshal(gen.Body.Bytes(), &g)
	sk := g["key"].(string)
	rec := doJSON(t, s.Handler(), "POST", "/chat/completions", sk, map[string]any{
		"model": "gpt-4o-mini", "stream": true, "messages": []any{map[string]any{"role": "user", "content": "hi"}},
	})
	if rec.Code != 200 {
		t.Fatalf("stream %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "data: [DONE]") {
		t.Fatalf("no DONE %s", rec.Body.String())
	}
}

func TestProviderNotImplemented(t *testing.T) {
	s, master := testEnv(t)
	s.Cfg.ModelList = append(s.Cfg.ModelList, config.ModelEntry{
		ModelName: "claude",
		LiteLLMParams: map[string]any{
			"model":    "bedrock/claude",
			"api_key":  "x",
			"api_base": "http://127.0.0.1:1",
		},
	})
	gen := doJSON(t, s.Handler(), "POST", "/key/generate", master, map[string]any{"key_type": "llm_api"})
	var g map[string]any
	_ = json.Unmarshal(gen.Body.Bytes(), &g)
	sk := g["key"].(string)
	rec := doJSON(t, s.Handler(), "POST", "/v1/chat/completions", sk, map[string]any{
		"model": "claude", "messages": []any{map[string]any{"role": "user", "content": "hi"}},
	})
	if rec.Code == 400 && strings.Contains(rec.Body.String(), "provider_not_implemented") {
		t.Fatalf("bedrock must be a real adapter, got %s", rec.Body.String())
	}
	if rec.Code != 200 && rec.Code != 502 {
		t.Fatalf("bedrock adapter want 200/502 got %d %s", rec.Code, rec.Body.String())
	}
}

func TestConfigLoadEnv(t *testing.T) {
	t.Setenv("LITELLM_MASTER_KEY", "sk-from-env")
	dir := t.TempDir()
	p := filepath.Join(dir, "c.yaml")
	if err := os.WriteFile(p, []byte("general_settings:\n  master_key: os.environ/LITELLM_MASTER_KEY\n  database_url: postgres://xhub:xhub@127.0.0.1:5433/xhub?sslmode=disable\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := config.Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if c.GeneralSettings.MasterKey != "sk-from-env" {
		t.Fatalf("got %q", c.GeneralSettings.MasterKey)
	}
}

func TestHealth(t *testing.T) {
	s, _ := testEnv(t)
	rec := doJSON(t, s.Handler(), "GET", "/health/liveliness", "", nil)
	if rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	rec = doJSON(t, s.Handler(), "GET", "/health/readiness/details", "", nil)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "litellm_version") {
		t.Fatal(rec.Body.String())
	}
}

func TestLogin(t *testing.T) {
	s, master := testEnv(t)
	h := s.Handler()
	for _, path := range []string{"/login", "/v2/login", "/v3/login"} {
		rec := doJSON(t, h, "POST", path, "", map[string]any{"username": "admin", "password": master})
		if rec.Code != 200 {
			t.Fatalf("%s %d %s", path, rec.Code, rec.Body.String())
		}
		var j map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &j); err != nil {
			t.Fatal(err)
		}
		tok, _ := j["token"].(string)
		if tok == "" {
			t.Fatalf("%s empty token %s", path, rec.Body.String())
		}
		if j["user_role"] != "proxy_admin" {
			t.Fatalf("%s role %v", path, j["user_role"])
		}
		if j["user_id"] == nil || j["redirect_url"] == nil {
			t.Fatalf("%s missing user_id/redirect_url %s", path, rec.Body.String())
		}
		if j["redirect_url"] != "/ui/?login=success" {
			t.Fatalf("%s redirect_url %v", path, j["redirect_url"])
		}
	}

	empty := doJSON(t, h, "POST", "/v2/login", "", map[string]any{"username": "", "password": ""})
	if empty.Code != 400 || !strings.Contains(empty.Body.String(), "Please enter your username / password") {
		t.Fatalf("empty %d %s", empty.Code, empty.Body.String())
	}

	secret := "super-secret-xyz-not-master"
	badUser := doJSON(t, h, "POST", "/v2/login", "", map[string]any{"username": "not-admin", "password": master})
	if badUser.Code != 401 || !strings.Contains(badUser.Body.String(), "Invalid credentials used to access UI") {
		t.Fatalf("wrong username %d %s", badUser.Code, badUser.Body.String())
	}
	if strings.Contains(badUser.Body.String(), master) {
		t.Fatal("401 echoed master key")
	}
	badPass := doJSON(t, h, "POST", "/v2/login", "", map[string]any{"username": "admin", "password": secret})
	if badPass.Code != 401 || !strings.Contains(badPass.Body.String(), "Invalid credentials used to access UI") {
		t.Fatalf("wrong password %d %s", badPass.Code, badPass.Body.String())
	}
	if strings.Contains(badPass.Body.String(), secret) {
		t.Fatal("401 echoed submitted password")
	}

	t.Run("ui_password", func(t *testing.T) {
		s2, master2 := testEnv(t)
		t.Setenv("UI_USERNAME", "admin")
		t.Setenv("UI_PASSWORD", "ui-secret")
		ok := doJSON(t, s2.Handler(), "POST", "/v2/login", "", map[string]any{"username": "admin", "password": "ui-secret"})
		if ok.Code != 200 {
			t.Fatalf("UI_PASSWORD %d %s", ok.Code, ok.Body.String())
		}
		no := doJSON(t, s2.Handler(), "POST", "/v2/login", "", map[string]any{"username": "admin", "password": master2})
		if no.Code != 401 {
			t.Fatalf("master should fail when UI_PASSWORD set, %d %s", no.Code, no.Body.String())
		}
	})

	t.Run("ui_password_equals_master", func(t *testing.T) {
		s2, master2 := testEnv(t)
		t.Setenv("UI_PASSWORD", master2)
		ok := doJSON(t, s2.Handler(), "POST", "/v2/login", "", map[string]any{"username": "admin", "password": master2})
		if ok.Code != 200 {
			t.Fatalf("equal UI_PASSWORD/master %d %s", ok.Code, ok.Body.String())
		}
	})

	t.Run("disable_env_credential_login", func(t *testing.T) {
		s2, master2 := testEnv(t)
		s2.Cfg.GeneralSettings.DisableEnvCredentialLogin = true
		no := doJSON(t, s2.Handler(), "POST", "/v2/login", "", map[string]any{"username": "admin", "password": master2})
		if no.Code != 401 {
			t.Fatalf("disabled env login %d %s", no.Code, no.Body.String())
		}
		if !strings.Contains(no.Body.String(), "Invalid credentials used to access UI") {
			t.Fatalf("message %s", no.Body.String())
		}
		if strings.Contains(no.Body.String(), "UI_USERNAME") {
			t.Fatalf("disabled env login should not mention UI_USERNAME: %s", no.Body.String())
		}
		mk := doJSON(t, s2.Handler(), "POST", "/user/new", master2, map[string]any{
			"user_email": "db@local", "user_role": "internal_user", "password": "db-pass",
		})
		if mk.Code != 200 {
			t.Fatal(mk.Body.String())
		}
		ok := doJSON(t, s2.Handler(), "POST", "/v2/login", "", map[string]any{"username": "db@local", "password": "db-pass"})
		if ok.Code != 200 {
			t.Fatalf("db user login while env disabled %d %s", ok.Code, ok.Body.String())
		}
	})
}

func TestXLiteLLMAPIKeyHeader(t *testing.T) {
	s, master := testEnv(t)
	gen := doJSON(t, s.Handler(), "POST", "/key/generate", master, map[string]any{"key_type": "llm_api"})
	var g map[string]any
	_ = json.Unmarshal(gen.Body.Bytes(), &g)
	sk := g["key"].(string)
	req := httptest.NewRequest("POST", "/v1/chat/completions", bytes.NewReader([]byte(`{"model":"gpt-4o-mini","messages":[{"role":"user","content":"hi"}]}`)))
	req.Header.Set("x-litellm-api-key", sk)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
}
