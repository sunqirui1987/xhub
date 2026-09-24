package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestKeyDeleteRevokesChat(t *testing.T) {
	s, master := testEnv(t)
	var hits atomic.Int32
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "c", "object": "chat.completion", "model": "gpt-4o-mini",
			"choices": []any{map[string]any{"index": 0, "message": map[string]any{"role": "assistant", "content": "hi"}, "finish_reason": "stop"}},
			"usage":   map[string]any{"prompt_tokens": 8, "completion_tokens": 2, "total_tokens": 10},
		})
	}))
	t.Cleanup(up.Close)
	s.Cfg.ModelList[0].LiteLLMParams["api_base"] = up.URL
	h := s.Handler()
	sk := mintLLM(t, s, master)
	ok := doJSON(t, h, "POST", "/v1/chat/completions", sk, map[string]any{
		"model": "gpt-4o-mini", "messages": []any{map[string]any{"role": "user", "content": "hi"}},
	})
	if ok.Code != 200 {
		t.Fatalf("before delete %d %s", ok.Code, ok.Body.String())
	}
	before := hits.Load()
	del := doJSON(t, h, "POST", "/key/delete", master, map[string]any{"keys": []string{sk}})
	if del.Code != 200 {
		t.Fatal(del.Body.String())
	}
	rec := doJSON(t, h, "POST", "/v1/chat/completions", sk, map[string]any{
		"model": "gpt-4o-mini", "messages": []any{map[string]any{"role": "user", "content": "hi"}},
	})
	if rec.Code != 401 {
		t.Fatalf("deleted key want 401 got %d %s", rec.Code, rec.Body.String())
	}
	em := decodeBody(t, rec.Body.Bytes())
	errObj, _ := em["error"].(map[string]any)
	if errObj["message"] == nil || errObj["type"] == nil {
		t.Fatalf("openai envelope %s", rec.Body.String())
	}
	if hits.Load() != before {
		t.Fatalf("upstream after delete %d -> %d", before, hits.Load())
	}
}

func TestKeyBlockRevokesChatThenUnblock(t *testing.T) {
	s, master := testEnv(t)
	var hits atomic.Int32
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "c", "object": "chat.completion", "model": "gpt-4o-mini",
			"choices": []any{map[string]any{"index": 0, "message": map[string]any{"role": "assistant", "content": "hi"}, "finish_reason": "stop"}},
			"usage":   map[string]any{"prompt_tokens": 8, "completion_tokens": 2, "total_tokens": 10},
		})
	}))
	t.Cleanup(up.Close)
	s.Cfg.ModelList[0].LiteLLMParams["api_base"] = up.URL
	h := s.Handler()
	sk := mintLLM(t, s, master)
	blk := doJSON(t, h, "POST", "/key/block", master, map[string]any{"key": sk})
	if blk.Code != 200 {
		t.Fatal(blk.Body.String())
	}
	before := hits.Load()
	rec := doJSON(t, h, "POST", "/v1/chat/completions", sk, map[string]any{
		"model": "gpt-4o-mini", "messages": []any{map[string]any{"role": "user", "content": "hi"}},
	})
	if rec.Code != 401 {
		t.Fatalf("blocked key want 401 got %d %s", rec.Code, rec.Body.String())
	}
	if hits.Load() != before {
		t.Fatalf("upstream after block %d", hits.Load())
	}
	un := doJSON(t, h, "POST", "/key/unblock", master, map[string]any{"key": sk})
	if un.Code != 200 {
		t.Fatal(un.Body.String())
	}
	ok := doJSON(t, h, "POST", "/v1/chat/completions", sk, map[string]any{
		"model": "gpt-4o-mini", "messages": []any{map[string]any{"role": "user", "content": "hi"}},
	})
	if ok.Code != 200 {
		t.Fatalf("unblocked key %d %s", ok.Code, ok.Body.String())
	}
	if hits.Load() <= before {
		t.Fatal("expected upstream after unblock")
	}
}

func TestManagementKeyCannotChat(t *testing.T) {
	s, master := testEnv(t)
	h := s.Handler()
	gen := doJSON(t, h, "POST", "/key/generate", master, map[string]any{"key_type": "management"})
	if gen.Code != 200 {
		t.Fatal(gen.Body.String())
	}
	sk := decodeBody(t, gen.Body.Bytes())["key"].(string)
	rec := doJSON(t, h, "POST", "/v1/chat/completions", sk, map[string]any{
		"model": "gpt-4o-mini", "messages": []any{map[string]any{"role": "user", "content": "hi"}},
	})
	if rec.Code != 401 {
		t.Fatalf("management key chat want 401 got %d %s", rec.Code, rec.Body.String())
	}
	lst := doJSON(t, h, "GET", "/user/list", sk, nil)
	if lst.Code != 200 {
		t.Fatalf("management key user/list %d %s", lst.Code, lst.Body.String())
	}
}

func TestAllowMasterKeyLLM(t *testing.T) {
	s, master := testEnv(t)
	s.Cfg.GeneralSettings.AllowMasterKeyLLM = true
	rec := doJSON(t, s.Handler(), "POST", "/v1/chat/completions", master, map[string]any{
		"model": "gpt-4o-mini", "messages": []any{map[string]any{"role": "user", "content": "hi"}},
	})
	if rec.Code != 200 {
		t.Fatalf("allow_master_key_llm %d %s", rec.Code, rec.Body.String())
	}
}

func TestInvalidBearerChat401(t *testing.T) {
	s, _ := testEnv(t)
	rec := doJSON(t, s.Handler(), "POST", "/v1/chat/completions", "sk-not-a-real-key", map[string]any{
		"model": "gpt-4o-mini", "messages": []any{map[string]any{"role": "user", "content": "hi"}},
	})
	if rec.Code != 401 {
		t.Fatalf("invalid bearer want 401 got %d %s", rec.Code, rec.Body.String())
	}
	em := decodeBody(t, rec.Body.Bytes())
	errObj, _ := em["error"].(map[string]any)
	if errObj["type"] == nil || errObj["message"] == nil {
		t.Fatalf("envelope %s", rec.Body.String())
	}
}

func TestMixedRoutesAcceptLLMKey(t *testing.T) {
	s, master := testEnv(t)
	h := s.Handler()
	sk := mintLLM(t, s, master)
	rec := doJSON(t, h, "POST", "/v1/agents", sk, map[string]any{"agent_name": "ops"})
	if rec.Code != 404 {
		t.Fatalf("removed agents route want 404 got %d %s", rec.Code, rec.Body.String())
	}
	kept := doJSON(t, h, "POST", "/v1/chat/completions", sk, map[string]any{
		"model": "gpt-4o-mini", "messages": []any{map[string]any{"role": "user", "content": "hi"}},
	})
	if kept.Code != 200 {
		t.Fatalf("llm_api chat %d %s", kept.Code, kept.Body.String())
	}
}

func TestBudgetExceededEnvelopeChat(t *testing.T) {
	s, master := testEnv(t)
	gen := doJSON(t, s.Handler(), "POST", "/key/generate", master, map[string]any{"key_type": "llm_api", "max_budget": 0})
	sk := decodeBody(t, gen.Body.Bytes())["key"].(string)
	rec := doJSON(t, s.Handler(), "POST", "/v1/chat/completions", sk, map[string]any{
		"model": "gpt-4o-mini", "messages": []any{map[string]any{"role": "user", "content": "hi"}},
	})
	if rec.Code != 429 {
		t.Fatalf("want 429 got %d %s", rec.Code, rec.Body.String())
	}
	if rec.Result().Header.Get("Retry-After") == "" {
		t.Fatal("missing Retry-After")
	}
	em := decodeBody(t, rec.Body.Bytes())
	errObj, _ := em["error"].(map[string]any)
	if errObj["type"] != "budget_exceeded" {
		t.Fatalf("type %v body %s", errObj["type"], rec.Body.String())
	}
	if _, ok := errObj["param"]; !ok {
		t.Fatalf("missing param %s", rec.Body.String())
	}
}

func TestKeyModelsAllowlistAndListModels(t *testing.T) {
	s, master := testEnv(t)
	h := s.Handler()
	gen := doJSON(t, h, "POST", "/key/generate", master, map[string]any{
		"key_type": "llm_api", "models": []string{"gpt-4o-mini"},
	})
	sk := decodeBody(t, gen.Body.Bytes())["key"].(string)
	ok := doJSON(t, h, "POST", "/v1/chat/completions", sk, map[string]any{
		"model": "gpt-4o-mini", "messages": []any{map[string]any{"role": "user", "content": "hi"}},
	})
	if ok.Code != 200 {
		t.Fatalf("allowed model %d %s", ok.Code, ok.Body.String())
	}
	denied := doJSON(t, h, "POST", "/v1/chat/completions", sk, map[string]any{
		"model": "dall-e-3", "messages": []any{map[string]any{"role": "user", "content": "hi"}},
	})
	if denied.Code != 401 {
		t.Fatalf("disallowed model want 401 got %d %s", denied.Code, denied.Body.String())
	}
	lst := doJSON(t, h, "GET", "/v1/models", sk, nil)
	if lst.Code != 200 {
		t.Fatal(lst.Body.String())
	}
	body := lst.Body.String()
	if !strings.Contains(body, "gpt-4o-mini") {
		t.Fatalf("models missing allowlisted alias %s", body)
	}
	if strings.Contains(body, "dall-e-3") {
		t.Fatalf("models leaked disallowed alias %s", body)
	}
}

func TestExpiredKeyChat401(t *testing.T) {
	s, master := testEnv(t)
	h := s.Handler()
	gen := doJSON(t, h, "POST", "/key/generate", master, map[string]any{
		"key_type": "llm_api", "duration": "-1s",
	})
	if gen.Code != 200 {
		t.Fatal(gen.Body.String())
	}
	sk := decodeBody(t, gen.Body.Bytes())["key"].(string)
	rec := doJSON(t, h, "POST", "/v1/chat/completions", sk, map[string]any{
		"model": "gpt-4o-mini", "messages": []any{map[string]any{"role": "user", "content": "hi"}},
	})
	if rec.Code != 401 {
		t.Fatalf("expired key want 401 got %d %s", rec.Code, rec.Body.String())
	}
}

func TestSessionJWTCanManageLLMAPICannot(t *testing.T) {
	s, master := testEnv(t)
	h := s.Handler()
	login := doJSON(t, h, "POST", "/v2/login", "", map[string]any{"username": "admin", "password": master})
	if login.Code != 200 {
		t.Fatal(login.Body.String())
	}
	tok := decodeBody(t, login.Body.Bytes())["token"].(string)
	lst := doJSON(t, h, "GET", "/user/list", tok, nil)
	if lst.Code != 200 {
		t.Fatalf("session GET /user/list %d %s", lst.Code, lst.Body.String())
	}
	sk := mintLLM(t, s, master)
	no := doJSON(t, h, "GET", "/user/list", sk, nil)
	if no.Code != 401 {
		t.Fatalf("llm_api GET /user/list want 401 got %d %s", no.Code, no.Body.String())
	}
}

func TestKeyModelsWildcardAllowsAll(t *testing.T) {
	s, master := testEnv(t)
	h := s.Handler()
	gen := doJSON(t, h, "POST", "/key/generate", master, map[string]any{
		"key_type": "llm_api", "models": []string{"*"},
	})
	sk := decodeBody(t, gen.Body.Bytes())["key"].(string)
	rec := doJSON(t, h, "POST", "/v1/chat/completions", sk, map[string]any{
		"model": "gpt-4o-mini", "messages": []any{map[string]any{"role": "user", "content": "hi"}},
	})
	if rec.Code != 200 {
		t.Fatalf("wildcard chat %d %s", rec.Code, rec.Body.String())
	}
	img := doJSON(t, h, "POST", "/v1/images/generations", sk, map[string]any{"model": "dall-e-3", "prompt": "cat"})
	if img.Code != 200 {
		t.Fatalf("wildcard images %d %s", img.Code, img.Body.String())
	}
}

func TestModelsEndpointAuthAndFilter(t *testing.T) {
	s, master := testEnv(t)
	h := s.Handler()
	no := doJSON(t, h, "GET", "/v1/models", "", nil)
	if no.Code != 401 {
		t.Fatalf("unauth /v1/models want 401 got %d %s", no.Code, no.Body.String())
	}
	sk := mintLLM(t, s, master)
	all := doJSON(t, h, "GET", "/v1/models", sk, nil)
	if all.Code != 200 || !strings.Contains(all.Body.String(), "gpt-4o-mini") {
		t.Fatalf("open key models %d %s", all.Code, all.Body.String())
	}
}

func TestChatIncrementsUserAndTeamSpend(t *testing.T) {
	s, master := testEnv(t)
	h := s.Handler()
	ur := doJSON(t, h, "POST", "/user/new", master, map[string]any{
		"user_id": "u_spend", "user_email": "sp@x.com", "auto_create_key": false,
	})
	if ur.Code != 200 {
		t.Fatal(ur.Body.String())
	}
	tr := doJSON(t, h, "POST", "/team/new", master, map[string]any{"team_alias": "spend-team"})
	tid := decodeBody(t, tr.Body.Bytes())["team_id"].(string)
	gen := doJSON(t, h, "POST", "/key/generate", master, map[string]any{
		"key_type": "llm_api", "user_id": "u_spend", "team_id": tid,
	})
	sk := decodeBody(t, gen.Body.Bytes())["key"].(string)
	chat := doJSON(t, h, "POST", "/v1/chat/completions", sk, map[string]any{
		"model": "gpt-4o-mini", "messages": []any{map[string]any{"role": "user", "content": "hi"}},
	})
	if chat.Code != 200 {
		t.Fatal(chat.Body.String())
	}
	ui := doJSON(t, h, "GET", "/user/info?user_id=u_spend", master, nil)
	um := decodeBody(t, ui.Body.Bytes())
	info, _ := um["user_info"].(map[string]any)
	if info == nil {
		info = um
	}
	if spendOf(info) <= 0 {
		t.Fatalf("user spend not incremented %v", um)
	}
	ti := doJSON(t, h, "GET", "/team/info?team_id="+tid, master, nil)
	tm := decodeBody(t, ti.Body.Bytes())
	tinfo, _ := tm["info"].(map[string]any)
	if tinfo == nil {
		tinfo = tm
	}
	if spendOf(tinfo) <= 0 {
		t.Fatalf("team spend not incremented %v", tm)
	}
}

func TestMaxParallelRequests429(t *testing.T) {
	s, master := testEnv(t)
	started := make(chan struct{})
	release := make(chan struct{})
	var inFlight atomic.Int32
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		inFlight.Add(1)
		select {
		case started <- struct{}{}:
		default:
		}
		<-release
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "c", "object": "chat.completion", "model": "gpt-4o-mini",
			"choices": []any{map[string]any{"index": 0, "message": map[string]any{"role": "assistant", "content": "hi"}, "finish_reason": "stop"}},
			"usage":   map[string]any{"prompt_tokens": 8, "completion_tokens": 2, "total_tokens": 10},
		})
	}))
	t.Cleanup(up.Close)
	s.Cfg.ModelList[0].LiteLLMParams["api_base"] = up.URL
	h := s.Handler()
	gen := doJSON(t, h, "POST", "/key/generate", master, map[string]any{
		"key_type": "llm_api", "max_parallel_requests": 1,
	})
	sk := decodeBody(t, gen.Body.Bytes())["key"].(string)
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		done <- doJSON(t, h, "POST", "/v1/chat/completions", sk, map[string]any{
			"model": "gpt-4o-mini", "messages": []any{map[string]any{"role": "user", "content": "hi"}},
		})
	}()
	<-started
	second := doJSON(t, h, "POST", "/v1/chat/completions", sk, map[string]any{
		"model": "gpt-4o-mini", "messages": []any{map[string]any{"role": "user", "content": "hi"}},
	})
	close(release)
	first := <-done
	if first.Code != 200 {
		t.Fatalf("first parallel %d %s", first.Code, first.Body.String())
	}
	if second.Code != 429 {
		t.Fatalf("max_parallel want 429 got %d %s", second.Code, second.Body.String())
	}
	if second.Result().Header.Get("Retry-After") == "" {
		t.Fatal("missing Retry-After")
	}
	em := decodeBody(t, second.Body.Bytes())
	errObj, _ := em["error"].(map[string]any)
	if errObj["type"] == nil || errObj["message"] == nil || errObj["code"] == nil {
		t.Fatalf("parallel openai envelope %s", second.Body.String())
	}
}

func TestTPMZeroMessagesAnthropicEnvelope(t *testing.T) {
	s, master := testEnv(t)
	var hits atomic.Int32
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(up.Close)
	s.Cfg.ModelList[0].LiteLLMParams["api_base"] = up.URL
	gen := doJSON(t, s.Handler(), "POST", "/key/generate", master, map[string]any{"key_type": "llm_api", "tpm_limit": 0})
	sk := decodeBody(t, gen.Body.Bytes())["key"].(string)
	rec := doJSON(t, s.Handler(), "POST", "/v1/messages", sk, map[string]any{
		"model": "gpt-4o-mini", "max_tokens": 8,
		"messages": []any{map[string]any{"role": "user", "content": "hi"}},
	})
	if rec.Code != 429 {
		t.Fatalf("messages tpm want 429 got %d %s", rec.Code, rec.Body.String())
	}
	if rec.Result().Header.Get("Retry-After") == "" {
		t.Fatal("missing Retry-After")
	}
	m := decodeBody(t, rec.Body.Bytes())
	if m["type"] != "error" {
		t.Fatalf("anthropic envelope %s", rec.Body.String())
	}
	errObj, _ := m["error"].(map[string]any)
	if errObj["type"] == nil || errObj["message"] == nil {
		t.Fatalf("anthropic error keys %s", rec.Body.String())
	}
	if hits.Load() != 0 {
		t.Fatalf("upstream called %d", hits.Load())
	}
}

func TestChatIncrementsOrgSpend(t *testing.T) {
	s, master := testEnv(t)
	h := s.Handler()
	or := doJSON(t, h, "POST", "/organization/new", master, map[string]any{"organization_alias": "spend-org"})
	if or.Code != 200 {
		t.Fatal(or.Body.String())
	}
	oid := decodeBody(t, or.Body.Bytes())["organization_id"].(string)
	gen := doJSON(t, h, "POST", "/key/generate", master, map[string]any{
		"key_type": "llm_api", "organization_id": oid,
	})
	sk := decodeBody(t, gen.Body.Bytes())["key"].(string)
	chat := doJSON(t, h, "POST", "/v1/chat/completions", sk, map[string]any{
		"model": "gpt-4o-mini", "messages": []any{map[string]any{"role": "user", "content": "hi"}},
	})
	if chat.Code != 200 {
		t.Fatal(chat.Body.String())
	}
	oi := doJSON(t, h, "GET", "/organization/info?organization_id="+oid, master, nil)
	om := decodeBody(t, oi.Body.Bytes())
	info, _ := om["info"].(map[string]any)
	if info == nil {
		info = om
	}
	if spendOf(info) <= 0 {
		t.Fatalf("org spend not incremented %v", om)
	}
}

func TestGetModelsRequiresAuth(t *testing.T) {
	s, master := testEnv(t)
	h := s.Handler()
	for _, path := range []string{"/v1/models", "/models"} {
		rec := doJSON(t, h, "GET", path, "", nil)
		if rec.Code != 401 {
			t.Fatalf("%s unauth want 401 got %d %s", path, rec.Code, rec.Body.String())
		}
	}
	sk := mintLLM(t, s, master)
	ok := doJSON(t, h, "GET", "/models", sk, nil)
	if ok.Code != 200 || !strings.Contains(ok.Body.String(), "object") {
		t.Fatalf("/models with key %d %s", ok.Code, ok.Body.String())
	}
}

func spendOf(m map[string]any) float64 {
	switch v := m["spend"].(type) {
	case float64:
		return v
	case int:
		return float64(v)
	default:
		return 0
	}
}
