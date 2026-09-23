package server

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/router"
)

func TestTeamAllowlistAndBudget(t *testing.T) {
	s, master := testEnv(t)
	tr := doJSON(t, s.Handler(), "POST", "/team/new", master, map[string]any{
		"team_alias": "eng", "models": []string{"gpt-4o-mini"}, "max_budget": 0,
	})
	if tr.Code != 200 {
		t.Fatal(tr.Body.String())
	}
	var team map[string]any
	_ = json.Unmarshal(tr.Body.Bytes(), &team)
	tid := team["team_id"].(string)
	gen := doJSON(t, s.Handler(), "POST", "/key/generate", master, map[string]any{
		"key_type": "llm_api", "team_id": tid, "models": []string{"gpt-4o-mini", "other"},
	})
	var g map[string]any
	_ = json.Unmarshal(gen.Body.Bytes(), &g)
	sk := g["key"].(string)
	rec := doJSON(t, s.Handler(), "POST", "/v1/chat/completions", sk, map[string]any{
		"model": "gpt-4o-mini", "messages": []any{map[string]any{"role": "user", "content": "hi"}},
	})
	if rec.Code != 429 {
		t.Fatalf("team budget want 429 got %d %s", rec.Code, rec.Body.String())
	}
	// wider key still blocked by team allowlist
	s2, master2 := testEnv(t)
	tr = doJSON(t, s2.Handler(), "POST", "/team/new", master2, map[string]any{
		"team_alias": "eng", "models": []string{"gpt-4o-mini"},
	})
	_ = json.Unmarshal(tr.Body.Bytes(), &team)
	tid = team["team_id"].(string)
	gen = doJSON(t, s2.Handler(), "POST", "/key/generate", master2, map[string]any{
		"key_type": "llm_api", "team_id": tid, "models": []string{"other-model"},
	})
	_ = json.Unmarshal(gen.Body.Bytes(), &g)
	sk = g["key"].(string)
	rec = doJSON(t, s2.Handler(), "POST", "/v1/chat/completions", sk, map[string]any{
		"model": "other-model", "messages": []any{map[string]any{"role": "user", "content": "hi"}},
	})
	if rec.Code != 401 && rec.Code != 400 {
		t.Fatalf("allowlist want 401/400 got %d %s", rec.Code, rec.Body.String())
	}
}

func TestUserOrgBudgetCRUD(t *testing.T) {
	s, master := testEnv(t)
	h := s.Handler()
	created := doJSON(t, h, "POST", "/user/new", master, map[string]any{
		"user_id": "user_login", "user_email": "a@b.c", "user_role": "internal_user", "password": "pw-a",
	})
	if created.Code != 200 {
		t.Fatal("user", created.Body.String())
	}
	if strings.Contains(created.Body.String(), "pw-a") || strings.Contains(created.Body.String(), `"password"`) {
		t.Fatalf("user/new leaked password %s", created.Body.String())
	}
	if doJSON(t, h, "GET", "/user/list", master, nil).Code != 200 {
		t.Fatal("user list")
	}
	if doJSON(t, h, "POST", "/organization/new", master, map[string]any{"organization_alias": "acme"}).Code != 200 {
		t.Fatal("org")
	}
	if doJSON(t, h, "POST", "/budget/new", master, map[string]any{"max_budget": 10.0}).Code != 200 {
		t.Fatal("budget")
	}
	if doJSON(t, h, "POST", "/project/new", master, map[string]any{"project_alias": "p"}).Code != 200 {
		t.Fatal("project")
	}

	emailLogin := doJSON(t, h, "POST", "/v2/login", "", map[string]any{"username": "a@b.c", "password": "pw-a"})
	if emailLogin.Code != 200 {
		t.Fatalf("email login %d %s", emailLogin.Code, emailLogin.Body.String())
	}
	var el map[string]any
	_ = json.Unmarshal(emailLogin.Body.Bytes(), &el)
	if el["user_role"] != "internal_user" || el["user_id"] != "user_login" {
		t.Fatalf("email login identity %v", el)
	}
	idLogin := doJSON(t, h, "POST", "/v2/login", "", map[string]any{"username": "user_login", "password": "pw-a"})
	if idLogin.Code != 200 {
		t.Fatalf("user_id login %d %s", idLogin.Code, idLogin.Body.String())
	}
	list := doJSON(t, h, "GET", "/user/list", master, nil)
	info := doJSON(t, h, "GET", "/user/info?user_id=user_login", master, nil)
	for _, rec := range []*httptest.ResponseRecorder{list, info} {
		if rec.Code != 200 {
			t.Fatal(rec.Body.String())
		}
		if strings.Contains(rec.Body.String(), "pw-a") || strings.Contains(rec.Body.String(), `"password"`) {
			t.Fatalf("password in JSON %s", rec.Body.String())
		}
	}
}

func TestCacheHit(t *testing.T) {
	s, master := testEnv(t)
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "c", "object": "chat.completion", "model": "gpt-4o-mini",
			"choices": []any{map[string]any{"index": 0, "message": map[string]any{"role": "assistant", "content": "cached"}, "finish_reason": "stop"}},
			"usage":   map[string]any{"prompt_tokens": 11, "completion_tokens": 7, "total_tokens": 18},
		})
	}))
	t.Cleanup(up.Close)
	s.Cfg.ModelList = []config.ModelEntry{{
		ModelName:     "gpt-4o-mini",
		LiteLLMParams: map[string]any{"model": "openai/gpt-4o-mini", "api_key": "k", "api_base": up.URL},
	}}
	gen := doJSON(t, s.Handler(), "POST", "/key/generate", master, map[string]any{"key_type": "llm_api"})
	var g map[string]any
	_ = json.Unmarshal(gen.Body.Bytes(), &g)
	sk := g["key"].(string)
	body := map[string]any{"model": "gpt-4o-mini", "messages": []any{map[string]any{"role": "user", "content": "cache-me"}}}
	r1 := doJSON(t, s.Handler(), "POST", "/v1/chat/completions", sk, body)
	if r1.Code != 200 {
		t.Fatal(r1.Body.String())
	}
	r2 := doJSON(t, s.Handler(), "POST", "/v1/chat/completions", sk, body)
	if r2.Result().Header.Get("x-litellm-cache-key") == "" {
		t.Fatalf("expected cache key on second call %s", r2.Header())
	}
	if r2.Result().Header.Get("cache_hit") != "true" && r2.Result().Header.Get("x-litellm-cache-hit") != "true" {
		t.Fatalf("expected cache_hit on second call, headers=%v", r2.Result().Header)
	}
	if r1.Result().Header.Get("x-litellm-response-cost") == "" || r1.Result().Header.Get("x-litellm-response-cost") == "0" {
		t.Fatalf("priced miss cost %q", r1.Result().Header.Get("x-litellm-response-cost"))
	}
	if r2.Result().Header.Get("x-litellm-response-cost") != r1.Result().Header.Get("x-litellm-response-cost") {
		t.Fatalf("cache hit must price from cached usage, miss=%s hit=%s",
			r1.Result().Header.Get("x-litellm-response-cost"), r2.Result().Header.Get("x-litellm-response-cost"))
	}
	rows := spendRows(t, s, master)
	assertHashedAPIKey(t, rows)
	var sawHit bool
	for _, m := range rows {
		if cacheHitTrue(m["cache_hit"]) {
			sawHit = true
			if num(m["prompt_tokens"]) != 11 || num(m["completion_tokens"]) != 7 {
				t.Fatalf("cache hit spend must use cached usage, got %+v", m)
			}
			if num(m["spend"]) == 0 {
				t.Fatalf("cache hit spend recorded as 0: %+v", m)
			}
		}
	}
	if !sawHit {
		t.Fatalf("no cache_hit spend log: %v", rows)
	}
}

func TestAnthropicAdapterURL(t *testing.T) {
	s, master := testEnv(t)
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/v1/messages") {
			t.Errorf("url %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "msg", "content": []any{}, "usage": map[string]any{"prompt_tokens": 1, "completion_tokens": 1}})
	}))
	t.Cleanup(up.Close)
	s.Cfg.ModelList = []config.ModelEntry{{
		ModelName:     "claude",
		LiteLLMParams: map[string]any{"model": "anthropic/claude", "api_key": "k", "api_base": up.URL},
	}}
	gen := doJSON(t, s.Handler(), "POST", "/key/generate", master, map[string]any{"key_type": "llm_api"})
	var g map[string]any
	_ = json.Unmarshal(gen.Body.Bytes(), &g)
	rec := doJSON(t, s.Handler(), "POST", "/v1/chat/completions", g["key"].(string), map[string]any{
		"model": "claude", "messages": []any{map[string]any{"role": "user", "content": "hi"}},
	})
	if rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
}

func TestUnknownPriceNotZero(t *testing.T) {
	s, master := testEnv(t)
	s.Cfg.ModelList[0].ModelName = "mystery"
	s.Cfg.ModelList[0].LiteLLMParams["model"] = "openai/mystery"
	gen := doJSON(t, s.Handler(), "POST", "/key/generate", master, map[string]any{"key_type": "llm_api"})
	var g map[string]any
	_ = json.Unmarshal(gen.Body.Bytes(), &g)
	sk := g["key"].(string)
	before := spendRows(t, s, master)
	rec := doJSON(t, s.Handler(), "POST", "/v1/chat/completions", sk, map[string]any{
		"model": "mystery", "messages": []any{map[string]any{"role": "user", "content": "hi"}},
	})
	if rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	if rec.Result().Header.Get("x-litellm-response-cost") == "0" {
		t.Fatal("unknown price recorded as 0")
	}
	info := doJSON(t, s.Handler(), "GET", "/key/info?key="+sk, master, nil)
	var inf map[string]any
	_ = json.Unmarshal(info.Body.Bytes(), &inf)
	sp := inf["info"].(map[string]any)["spend"].(float64)
	if sp != 0 {
		t.Fatalf("spend should stay 0 when price unknown, got %v", sp)
	}
	after := spendRows(t, s, master)
	if len(after) != len(before) {
		for _, m := range after[len(before):] {
			if num(m["spend"]) == 0 {
				t.Fatalf("unknown-price chat wrote spend 0 row %+v", m)
			}
			t.Fatalf("unknown-price chat must not write spend_logs, got %+v", m)
		}
	}
	for _, m := range after {
		if m["model"] == "mystery" {
			t.Fatalf("mystery must not appear in spend_logs %+v", m)
		}
	}
}

func TestViewOnlyWrite403(t *testing.T) {
	s, master := testEnv(t)
	h := s.Handler()
	shortcut := doJSON(t, h, "POST", "/v2/login", "", map[string]any{"username": "admin-viewer", "password": master})
	if shortcut.Code != 401 {
		t.Fatalf("admin-viewer+master must 401, got %d %s", shortcut.Code, shortcut.Body.String())
	}
	created := doJSON(t, h, "POST", "/user/new", master, map[string]any{
		"user_email": "viewer@local", "user_role": "proxy_admin_viewer", "password": "view-pass",
	})
	if created.Code != 200 {
		t.Fatal(created.Body.String())
	}
	login := doJSON(t, h, "POST", "/v2/login", "", map[string]any{"username": "viewer@local", "password": "view-pass"})
	if login.Code != 200 {
		t.Fatalf("viewer login %d %s", login.Code, login.Body.String())
	}
	var j map[string]any
	_ = json.Unmarshal(login.Body.Bytes(), &j)
	if j["user_role"] != "proxy_admin_viewer" {
		t.Fatalf("stored role %v", j["user_role"])
	}
	tok, _ := j["token"].(string)
	if tok == "" {
		t.Fatal("empty token")
	}
	rec := doJSON(t, h, "POST", "/key/generate", tok, map[string]any{"key_alias": "x"})
	if rec.Code != 403 {
		t.Fatalf("want 403 got %d %s", rec.Code, rec.Body.String())
	}
}

func TestIdentityPersistMutations(t *testing.T) {
	s, master := testEnv(t)
	h := s.Handler()

	u := doJSON(t, h, "POST", "/user/new", master, map[string]any{
		"user_id": "u1", "user_email": "old@x", "user_role": "internal_user", "password": "old-pw", "max_budget": 5.0,
	})
	if u.Code != 200 {
		t.Fatal(u.Body.String())
	}
	upd := doJSON(t, h, "POST", "/user/update", master, map[string]any{
		"user_id": "u1", "user_email": "new@x", "user_role": "internal_user", "max_budget": 42.0, "user_alias": "renamed",
	})
	if upd.Code != 200 {
		t.Fatal(upd.Body.String())
	}
	if strings.Contains(upd.Body.String(), `"password"`) || strings.Contains(upd.Body.String(), "old-pw") {
		t.Fatalf("update leaked password %s", upd.Body.String())
	}
	info := doJSON(t, h, "GET", "/user/info?user_id=u1", master, nil)
	if info.Code != 200 || !strings.Contains(info.Body.String(), "new@x") || !strings.Contains(info.Body.String(), "renamed") {
		t.Fatalf("user info after update %s", info.Body.String())
	}
	if !strings.Contains(info.Body.String(), "42") {
		t.Fatalf("user max_budget after update %s", info.Body.String())
	}
	list := doJSON(t, h, "GET", "/user/list", master, nil)
	if !strings.Contains(list.Body.String(), "new@x") {
		t.Fatalf("user list after update %s", list.Body.String())
	}
	del := doJSON(t, h, "POST", "/user/delete", master, map[string]any{"user_ids": []string{"u1"}})
	if del.Code != 200 {
		t.Fatal(del.Body.String())
	}
	gone := doJSON(t, h, "GET", "/user/list", master, nil)
	if strings.Contains(gone.Body.String(), "u1") || strings.Contains(gone.Body.String(), "new@x") {
		t.Fatalf("user still listed %s", gone.Body.String())
	}
	noLogin := doJSON(t, h, "POST", "/v2/login", "", map[string]any{"username": "new@x", "password": "old-pw"})
	if noLogin.Code != 401 {
		t.Fatalf("deleted user still logs in %d %s", noLogin.Code, noLogin.Body.String())
	}

	tr := doJSON(t, h, "POST", "/team/new", master, map[string]any{"team_alias": "eng", "max_budget": 10.0})
	var team map[string]any
	_ = json.Unmarshal(tr.Body.Bytes(), &team)
	tid, _ := team["team_id"].(string)
	if tid == "" {
		t.Fatal(tr.Body.String())
	}
	tu := doJSON(t, h, "POST", "/team/update", master, map[string]any{"team_id": tid, "team_alias": "platform", "max_budget": 99.0})
	if tu.Code != 200 {
		t.Fatal(tu.Body.String())
	}
	ti := doJSON(t, h, "GET", "/team/info?team_id="+tid, master, nil)
	if !strings.Contains(ti.Body.String(), "platform") || !strings.Contains(ti.Body.String(), "99") {
		t.Fatalf("team info %s", ti.Body.String())
	}
	tl := doJSON(t, h, "GET", "/team/list", master, nil)
	if !strings.Contains(tl.Body.String(), "platform") {
		t.Fatalf("team list %s", tl.Body.String())
	}
	td := doJSON(t, h, "POST", "/team/delete", master, map[string]any{"team_ids": []string{tid}})
	if td.Code != 200 {
		t.Fatal(td.Body.String())
	}
	tl2 := doJSON(t, h, "GET", "/team/list", master, nil)
	if strings.Contains(tl2.Body.String(), tid) || strings.Contains(tl2.Body.String(), "platform") {
		t.Fatalf("team still listed %s", tl2.Body.String())
	}

	or := doJSON(t, h, "POST", "/organization/new", master, map[string]any{"organization_alias": "acme", "max_budget": 1.0})
	var org map[string]any
	_ = json.Unmarshal(or.Body.Bytes(), &org)
	oid, _ := org["organization_id"].(string)
	ou := doJSON(t, h, "PATCH", "/organization/update", master, map[string]any{"organization_id": oid, "organization_alias": "globex", "max_budget": 77.0})
	if ou.Code != 200 {
		t.Fatal(ou.Body.String())
	}
	oi := doJSON(t, h, "GET", "/organization/info?organization_id="+oid, master, nil)
	if !strings.Contains(oi.Body.String(), "globex") || !strings.Contains(oi.Body.String(), "77") {
		t.Fatalf("org info %s", oi.Body.String())
	}
	ol := doJSON(t, h, "GET", "/organization/list", master, nil)
	if !strings.Contains(ol.Body.String(), "globex") {
		t.Fatalf("org list %s", ol.Body.String())
	}
	od := doJSON(t, h, "DELETE", "/organization/delete", master, map[string]any{"organization_ids": []string{oid}})
	if od.Code != 200 {
		t.Fatal(od.Body.String())
	}
	ol2 := doJSON(t, h, "GET", "/organization/list", master, nil)
	if strings.Contains(ol2.Body.String(), oid) || strings.Contains(ol2.Body.String(), "globex") {
		t.Fatalf("org still listed %s", ol2.Body.String())
	}

	br := doJSON(t, h, "POST", "/budget/new", master, map[string]any{"max_budget": 10.0, "budget_duration": "30d", "soft_budget": 8.0})
	var bud map[string]any
	_ = json.Unmarshal(br.Body.Bytes(), &bud)
	bid, _ := bud["budget_id"].(string)
	if bud["budget_reset_at"] == nil || bud["budget_reset_at"] == "" {
		t.Fatalf("budget_reset_at missing %v", bud)
	}
	if bud["soft_budget"] != 8.0 {
		t.Fatalf("soft_budget %v", bud["soft_budget"])
	}
	bu := doJSON(t, h, "POST", "/budget/update", master, map[string]any{"budget_id": bid, "max_budget": 55.0, "budget_duration": "7d"})
	if bu.Code != 200 {
		t.Fatal(bu.Body.String())
	}
	bi := doJSON(t, h, "POST", "/budget/info", master, map[string]any{"budgets": []string{bid}})
	if !strings.Contains(bi.Body.String(), "55") || !strings.Contains(bi.Body.String(), "7d") {
		t.Fatalf("budget info %s", bi.Body.String())
	}
	bl := doJSON(t, h, "GET", "/budget/list", master, nil)
	if !strings.Contains(bl.Body.String(), "55") {
		t.Fatalf("budget list %s", bl.Body.String())
	}
	bd := doJSON(t, h, "POST", "/budget/delete", master, map[string]any{"id": bid})
	if bd.Code != 200 {
		t.Fatal(bd.Body.String())
	}
	bl2 := doJSON(t, h, "GET", "/budget/list", master, nil)
	if strings.Contains(bl2.Body.String(), bid) {
		t.Fatalf("budget still listed %s", bl2.Body.String())
	}
}

func TestSSOLogin(t *testing.T) {
	s, _ := testEnv(t)
	g := doJSON(t, s.Handler(), "GET", "/sso/key/generate", "", nil)
	var j map[string]any
	_ = json.Unmarshal(g.Body.Bytes(), &j)
	url, _ := j["url"].(string)
	code := url[strings.Index(url, "code=")+5:]
	if i := strings.Index(code, "&"); i > 0 {
		code = code[:i]
	}
	ex := doJSON(t, s.Handler(), "POST", "/v3/login/exchange", "", map[string]any{"code": code})
	if ex.Code != 200 {
		t.Fatal(ex.Body.String())
	}
}

func TestRouterPickPackage(t *testing.T) {
	list := []config.ModelEntry{
		{ModelName: "m", LiteLLMParams: map[string]any{"model": "openai/a", "api_base": "http://busy", "weight": 10.0, "input_cost_per_token": 9.0}},
		{ModelName: "m", LiteLLMParams: map[string]any{"model": "openai/b", "api_base": "http://cheap", "weight": 1.0, "input_cost_per_token": 0.1}},
	}
	busy := map[string]int{"http://busy|openai/a": 8}
	dWeight := mustPick(t, list, "simple-shuffle", busy)
	dCost := mustPick(t, list, "lowest-cost", busy)
	if dWeight == dCost {
		t.Fatalf("same pick %s", dWeight)
	}
}

func TestRouterStrategiesViaChat(t *testing.T) {
	s, master := testEnv(t)
	heavy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "heavy", "object": "chat.completion", "model": "a",
			"choices": []any{map[string]any{"index": 0, "message": map[string]any{"role": "assistant", "content": "heavy"}, "finish_reason": "stop"}},
			"usage":   map[string]any{"prompt_tokens": 8, "completion_tokens": 2, "total_tokens": 10},
		})
	}))
	t.Cleanup(heavy.Close)
	cheap := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "cheap", "object": "chat.completion", "model": "b",
			"choices": []any{map[string]any{"index": 0, "message": map[string]any{"role": "assistant", "content": "cheap"}, "finish_reason": "stop"}},
			"usage":   map[string]any{"prompt_tokens": 8, "completion_tokens": 2, "total_tokens": 10},
		})
	}))
	t.Cleanup(cheap.Close)
	s.Cfg.ModelList = []config.ModelEntry{
		{ModelName: "gpt-4o-mini", LiteLLMParams: map[string]any{"model": "openai/a", "api_key": "k", "api_base": heavy.URL, "weight": 10.0, "input_cost_per_token": 9.0}},
		{ModelName: "gpt-4o-mini", LiteLLMParams: map[string]any{"model": "openai/b", "api_key": "k", "api_base": cheap.URL, "weight": 1.0, "input_cost_per_token": 0.1}},
	}
	gen := doJSON(t, s.Handler(), "POST", "/key/generate", master, map[string]any{"key_type": "llm_api"})
	var g map[string]any
	_ = json.Unmarshal(gen.Body.Bytes(), &g)
	sk := g["key"].(string)

	s.Cfg.RouterSettings.RoutingStrategy = "simple-shuffle"
	r1 := doJSON(t, s.Handler(), "POST", "/v1/chat/completions", sk, map[string]any{
		"model": "gpt-4o-mini", "messages": []any{map[string]any{"role": "user", "content": "strat-weight"}},
	})
	if r1.Code != 200 {
		t.Fatalf("simple-shuffle %d %s", r1.Code, r1.Body.String())
	}
	base1 := r1.Result().Header.Get("x-litellm-model-api-base")
	if base1 != heavy.URL {
		t.Fatalf("simple-shuffle want heavy %s got %s body=%s", heavy.URL, base1, r1.Body.String())
	}

	s.Cache.Flush()
	s.Cfg.RouterSettings.RoutingStrategy = "lowest-cost"
	r2 := doJSON(t, s.Handler(), "POST", "/v1/chat/completions", sk, map[string]any{
		"model": "gpt-4o-mini", "messages": []any{map[string]any{"role": "user", "content": "strat-cost"}},
	})
	if r2.Code != 200 {
		t.Fatalf("lowest-cost %d %s", r2.Code, r2.Body.String())
	}
	base2 := r2.Result().Header.Get("x-litellm-model-api-base")
	if base2 != cheap.URL {
		t.Fatalf("lowest-cost want cheap %s got %s body=%s", cheap.URL, base2, r2.Body.String())
	}
	if base1 == base2 {
		t.Fatalf("strategies picked the same api_base %s", base1)
	}
}

func TestAzureAdapterURL(t *testing.T) {
	s, master := testEnv(t)
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/openai/deployments/") || !strings.HasSuffix(r.URL.Path, "/chat/completions") {
			t.Errorf("azure url %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "az", "object": "chat.completion", "model": "dep",
			"choices": []any{map[string]any{"index": 0, "message": map[string]any{"role": "assistant", "content": "az"}, "finish_reason": "stop"}},
			"usage":   map[string]any{"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
		})
	}))
	t.Cleanup(up.Close)
	s.Cfg.ModelList = []config.ModelEntry{{
		ModelName:     "az-gpt",
		LiteLLMParams: map[string]any{"model": "azure/dep", "api_key": "k", "api_base": up.URL},
	}}
	gen := doJSON(t, s.Handler(), "POST", "/key/generate", master, map[string]any{"key_type": "llm_api"})
	var g map[string]any
	_ = json.Unmarshal(gen.Body.Bytes(), &g)
	rec := doJSON(t, s.Handler(), "POST", "/v1/chat/completions", g["key"].(string), map[string]any{
		"model": "az-gpt", "messages": []any{map[string]any{"role": "user", "content": "hi"}},
	})
	if rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
}

func mustPick(t *testing.T, list []config.ModelEntry, strat string, busy map[string]int) string {
	t.Helper()
	d := pickForTest(list, strat, busy)
	if d == "" {
		t.Fatal("nil")
	}
	return d
}

func pickForTest(list []config.ModelEntry, strat string, busy map[string]int) string {
	d := router.Pick(list, "m", strat, busy)
	if d == nil {
		return ""
	}
	return d.ParamString("api_base", "")
}

func TestCatalogDataPlaneAuth(t *testing.T) {
	s, master := testEnv(t)
	noAuth := doJSON(t, s.Handler(), "POST", "/v1/audio/speech", "", map[string]any{"model": "tts"})
	if noAuth.Code != 401 {
		t.Fatalf("unauthenticated data-plane catalog want 401 got %d %s", noAuth.Code, noAuth.Body.String())
	}
	masterRec := doJSON(t, s.Handler(), "POST", "/v1/audio/speech", master, map[string]any{"model": "tts"})
	if masterRec.Code != 401 {
		t.Fatalf("master data-plane catalog want 401 got %d %s", masterRec.Code, masterRec.Body.String())
	}
	for _, p := range []string{"/v1/agents", "/v1/skills", "/v1/memory", "/v1/workflows/runs", "/v1/tool/list"} {
		rec := doJSON(t, s.Handler(), "GET", p, master, nil)
		if rec.Code != 200 {
			t.Fatalf("master GET %s want 200 got %d %s", p, rec.Code, rec.Body.String())
		}
		noKey := doJSON(t, s.Handler(), "GET", p, "", nil)
		if noKey.Code != 401 {
			t.Fatalf("no-key GET %s want 401 got %d %s", p, noKey.Code, noKey.Body.String())
		}
	}
	pub := doJSON(t, s.Handler(), "GET", "/public/model_hub", "", nil)
	if pub.Code != 200 {
		t.Fatalf("public model hub want 200 got %d %s", pub.Code, pub.Body.String())
	}
	hub := doJSON(t, s.Handler(), "GET", "/model_hub", "", nil)
	if hub.Code != 200 {
		t.Fatalf("model_hub want 200 got %d %s", hub.Code, hub.Body.String())
	}
	gen := doJSON(t, s.Handler(), "POST", "/key/generate", master, map[string]any{"key_type": "llm_api"})
	var g map[string]any
	_ = json.Unmarshal(gen.Body.Bytes(), &g)
	ok := doJSON(t, s.Handler(), "POST", "/v1/audio/speech", g["key"].(string), map[string]any{"model": "tts"})
	if ok.Code == 404 || ok.Code == 401 {
		t.Fatalf("virtual key catalog data-plane %d %s", ok.Code, ok.Body.String())
	}
}

func TestChatFallbackTwoDeployments(t *testing.T) {
	s, master := testEnv(t)
	var failN, okN int
	fail := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		failN++
		http.Error(w, `{"error":"boom"}`, http.StatusInternalServerError)
	}))
	t.Cleanup(fail.Close)
	ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		okN++
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "ok", "object": "chat.completion", "model": "gpt-4o-mini",
			"choices": []any{map[string]any{"index": 0, "message": map[string]any{"role": "assistant", "content": "from-ok"}, "finish_reason": "stop"}},
			"usage":   map[string]any{"prompt_tokens": 8, "completion_tokens": 2, "total_tokens": 10},
		})
	}))
	t.Cleanup(ok.Close)
	s.Cfg.RouterSettings.NumRetries = 2
	s.Cfg.RouterSettings.RoutingStrategy = "simple-shuffle"
	s.Cfg.ModelList = []config.ModelEntry{
		{ModelName: "gpt-4o-mini", LiteLLMParams: map[string]any{"model": "openai/gpt-4o-mini", "api_key": "k", "api_base": fail.URL, "weight": 10.0}},
		{ModelName: "gpt-4o-mini", LiteLLMParams: map[string]any{"model": "openai/gpt-4o-mini", "api_key": "k", "api_base": ok.URL, "weight": 1.0}},
	}
	gen := doJSON(t, s.Handler(), "POST", "/key/generate", master, map[string]any{"key_type": "llm_api"})
	var g map[string]any
	_ = json.Unmarshal(gen.Body.Bytes(), &g)
	rec := doJSON(t, s.Handler(), "POST", "/v1/chat/completions", g["key"].(string), map[string]any{
		"model": "gpt-4o-mini", "messages": []any{map[string]any{"role": "user", "content": "hi"}},
	})
	if rec.Code != 200 {
		t.Fatalf("fallback chat %d %s", rec.Code, rec.Body.String())
	}
	if failN != 2 {
		t.Fatalf("first deployment retries want 2 got %d", failN)
	}
	if okN != 1 {
		t.Fatalf("second deployment hits want 1 got %d", okN)
	}
	if !strings.Contains(rec.Body.String(), "from-ok") {
		t.Fatalf("expected fallback body %s", rec.Body.String())
	}
}

func TestStreamNoFallbackAfterChunk(t *testing.T) {
	s, master := testEnv(t)
	var second int
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\ndata: [DONE]\n\n"))
	}))
	t.Cleanup(first.Close)
	sec := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		second++
		w.WriteHeader(200)
	}))
	t.Cleanup(sec.Close)
	s.Cfg.ModelList = []config.ModelEntry{
		{ModelName: "gpt-4o-mini", LiteLLMParams: map[string]any{"model": "openai/gpt-4o-mini", "api_key": "k", "api_base": first.URL, "weight": 10.0}},
		{ModelName: "gpt-4o-mini", LiteLLMParams: map[string]any{"model": "openai/gpt-4o-mini", "api_key": "k", "api_base": sec.URL, "weight": 1.0}},
	}
	gen := doJSON(t, s.Handler(), "POST", "/key/generate", master, map[string]any{"key_type": "llm_api"})
	var g map[string]any
	_ = json.Unmarshal(gen.Body.Bytes(), &g)
	rec := doJSON(t, s.Handler(), "POST", "/v1/chat/completions", g["key"].(string), map[string]any{
		"model": "gpt-4o-mini", "stream": true, "messages": []any{map[string]any{"role": "user", "content": "hi"}},
	})
	if rec.Code != 200 {
		t.Fatalf("stream %d %s", rec.Code, rec.Body.String())
	}
	if second != 0 {
		t.Fatalf("switched provider after first business chunk, second=%d", second)
	}
}

func spendRows(t *testing.T, s *Server, master string) []map[string]any {
	t.Helper()
	logs := doJSON(t, s.Handler(), "GET", "/spend/logs", master, nil)
	if logs.Code != 200 {
		t.Fatalf("spend logs %d %s", logs.Code, logs.Body.String())
	}
	var lj map[string]any
	_ = json.Unmarshal(logs.Body.Bytes(), &lj)
	var out []map[string]any
	switch d := lj["data"].(type) {
	case []any:
		for _, x := range d {
			if m, ok := x.(map[string]any); ok {
				out = append(out, m)
			}
		}
	}
	return out
}

func assertHashedAPIKey(t *testing.T, rows []map[string]any) {
	t.Helper()
	re := regexp.MustCompile(`^[0-9a-f]{64}$`)
	found := false
	for _, m := range rows {
		ak, _ := m["api_key"].(string)
		if ak == "" {
			continue
		}
		if strings.HasPrefix(ak, "sk-") {
			t.Fatalf("spend log api_key is plaintext %s", ak)
		}
		if !re.MatchString(ak) {
			t.Fatalf("api_key not 64-hex: %q", ak)
		}
		found = true
	}
	if !found {
		t.Fatalf("no hashed api_key in %v", rows)
	}
}

func cacheHitTrue(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case float64:
		return t != 0
	case string:
		return t == "true" || t == "1"
	default:
		return false
	}
}

func num(v any) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case int:
		return float64(t)
	case int64:
		return float64(t)
	default:
		return 0
	}
}

func TestDashboardSessionShapes(t *testing.T) {
	s, master := testEnv(t)
	h := s.Handler()
	login := doJSON(t, h, "POST", "/v2/login", "", map[string]any{"username": "admin", "password": master})
	if login.Code != 200 {
		t.Fatal(login.Body.String())
	}
	var lj map[string]any
	_ = json.Unmarshal(login.Body.Bytes(), &lj)
	sess, _ := lj["key"].(string)
	jwt, _ := lj["token"].(string)
	if sess == "" || jwt == "" || !strings.Contains(jwt, ".") {
		t.Fatalf("login token/key %s", login.Body.String())
	}

	v2 := doJSON(t, h, "GET", "/v2/user/info?user_id=admin", sess, nil)
	if v2.Code != 200 {
		t.Fatalf("v2 user info %d %s", v2.Code, v2.Body.String())
	}
	var u map[string]any
	_ = json.Unmarshal(v2.Body.Bytes(), &u)
	if u["user_id"] != "admin" || u["user_role"] != "proxy_admin" {
		t.Fatalf("v2 user info %+v", u)
	}

	god := doJSON(t, h, "GET", "/user/info?user_id=admin", sess, nil)
	if god.Code != 200 || !strings.Contains(god.Body.String(), `"user_info"`) {
		t.Fatalf("user info %d %s", god.Code, god.Body.String())
	}

	mi := doJSON(t, h, "GET", "/v2/model/info", sess, nil)
	if mi.Code != 200 {
		t.Fatal(mi.Body.String())
	}
	var md map[string]any
	_ = json.Unmarshal(mi.Body.Bytes(), &md)
	if _, ok := md["total_count"]; !ok {
		t.Fatalf("model info missing total_count %s", mi.Body.String())
	}

	tl := doJSON(t, h, "GET", "/team/list", sess, nil)
	if tl.Code != 200 || tl.Body.Bytes()[0] != '[' {
		t.Fatalf("team list want array %s", tl.Body.String())
	}
	tlv := doJSON(t, h, "GET", "/v2/team/list", sess, nil)
	if !strings.Contains(tlv.Body.String(), `"teams"`) {
		t.Fatalf("v2 team list %s", tlv.Body.String())
	}

	ol := doJSON(t, h, "GET", "/organization/list", sess, nil)
	if ol.Code != 200 || len(ol.Body.Bytes()) == 0 || ol.Body.Bytes()[0] != '[' {
		t.Fatalf("org list want array %s", ol.Body.String())
	}

	kl := doJSON(t, h, "GET", "/key/list", sess, nil)
	if !strings.Contains(kl.Body.String(), `"current_page"`) {
		t.Fatalf("key list %s", kl.Body.String())
	}
	ul := doJSON(t, h, "GET", "/user/list", sess, nil)
	if ul.Code != 200 || !strings.Contains(ul.Body.String(), `"admin"`) {
		t.Fatalf("user list after login want admin %s", ul.Body.String())
	}
	mg := doJSON(t, h, "GET", "/model_group/info", sess, nil)
	if mg.Code != 200 || !strings.Contains(mg.Body.String(), "model_group") {
		t.Fatalf("model_group/info %s", mg.Body.String())
	}

	chat := doJSON(t, h, "POST", "/v1/chat/completions", sess, map[string]any{
		"model": "gpt-4o-mini", "messages": []any{map[string]any{"role": "user", "content": "hi"}},
	})
	if chat.Code != 200 {
		t.Fatalf("admin session playground chat %d %s", chat.Code, chat.Body.String())
	}

	ui := doJSON(t, h, "GET", "/sso/get/ui_settings", sess, nil)
	if ui.Code != 200 || !strings.Contains(ui.Body.String(), "enable_chat_ui") {
		t.Fatalf("sso ui settings %d %s", ui.Code, ui.Body.String())
	}

	mustArray := func(path string) []any {
		t.Helper()
		rec := doJSON(t, h, "GET", path, sess, nil)
		if rec.Code != 200 {
			t.Fatalf("%s %d %s", path, rec.Code, rec.Body.String())
		}
		var v any
		if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
			t.Fatalf("%s json %v %s", path, err, rec.Body.String())
		}
		a, ok := v.([]any)
		if !ok {
			t.Fatalf("%s want array got %s", path, rec.Body.String())
		}
		return a
	}
	mustObj := func(path string) map[string]any {
		t.Helper()
		rec := doJSON(t, h, "GET", path, sess, nil)
		if rec.Code != 200 {
			t.Fatalf("%s %d %s", path, rec.Code, rec.Body.String())
		}
		return decodeBody(t, rec.Body.Bytes())
	}
	mustKeyArray := func(path, key string) {
		t.Helper()
		m := mustObj(path)
		if _, ok := m[key].([]any); !ok {
			t.Fatalf("%s.%s want array got %v from %s", path, key, m[key], mustObj(path))
		}
	}

	for _, p := range []string{
		"/v1/mcp/server", "/v1/mcp/server/health", "/v1/mcp/toolset", "/v1/mcp/user-env-vars/status",
		"/v1/access_group", "/tag/list", "/policy/templates", "/callbacks/configs", "/alerting/settings",
		"/v1/agents", "/config/list", "/global/spend/logs", "/global/spend/keys",
		"/global/spend/models", "/global/spend/provider", "/global/activity/model",
	} {
		mustArray(p)
	}
	mustKeyArray("/v2/guardrails/list", "guardrails")
	mustKeyArray("/guardrails/list", "guardrails")
	mustKeyArray("/policies/list", "policies")
	mustKeyArray("/prompts/list", "prompts")
	mustKeyArray("/search_tools/list", "search_tools")
	mustKeyArray("/vector_store/list", "data")
	mustKeyArray("/v1/memory", "memories")
	mustKeyArray("/v1/workflows/runs", "runs")
	mustKeyArray("/claude-code/plugins", "plugins")
	mustKeyArray("/v1/tool/list", "tools")
	mustKeyArray("/v1/mcp/access_groups", "access_groups")
	mustKeyArray("/v1/mcp/server/submissions", "items")
	mustKeyArray("/user/daily/activity/aggregated", "results")
	mustKeyArray("/user/daily/activity", "results")
	mustKeyArray("/gateway/daily/activity", "by_route")
	ov := mustObj("/guardrails/usage/overview")
	if _, ok := ov["totalRequests"].(float64); !ok {
		t.Fatalf("guardrails usage overview totalRequests %v", ov)
	}
	if _, ok := ov["rows"].([]any); !ok {
		t.Fatalf("guardrails usage overview rows %v", ov)
	}
	ius := mustObj("/get/internal_user_settings")
	if _, ok := ius["values"].(map[string]any); !ok {
		t.Fatalf("internal_user_settings.values %v", ius)
	}
	if _, ok := ius["field_schema"].(map[string]any); !ok {
		t.Fatalf("internal_user_settings.field_schema %v", ius)
	}
	crs := mustObj("/coordination_redis/settings")
	if _, ok := crs["values"].(map[string]any); !ok {
		t.Fatalf("coordination redis values %v", crs)
	}
	mustKeyArray("/search_tools/ui/available_providers", "providers")
	mustKeyArray("/credentials", "credentials")
	mustKeyArray("/global/spend/tags", "spend_per_tag")
	mustKeyArray("/global/spend/all_tag_names", "tag_names")

	cb := mustObj("/get/config/callbacks")
	rs, _ := cb["router_settings"].(map[string]any)
	if rs == nil {
		t.Fatalf("callbacks missing router_settings %v", cb)
	}
	if _, ok := rs["model_group_retry_policy"]; !ok {
		t.Fatalf("router_settings.model_group_retry_policy missing %v", rs)
	}
	if _, ok := cb["callbacks"].([]any); !ok {
		t.Fatalf("callbacks array missing %v", cb)
	}
	if _, ok := cb["alerts"].([]any); !ok {
		t.Fatalf("alerts array missing %v", cb)
	}

	rt := mustObj("/router/settings")
	if _, ok := rt["fields"].([]any); !ok {
		t.Fatalf("router/settings.fields missing %v", rt)
	}

	teams := mustObj("/global/spend/teams")
	for _, k := range []string{"daily_spend", "teams", "total_spend_per_team"} {
		if teams[k] == nil {
			t.Fatalf("global/spend/teams missing %s %v", k, teams)
		}
	}
	if _, ok := teams["total_spend_per_team"].([]any); !ok {
		t.Fatalf("total_spend_per_team want array %v", teams["total_spend_per_team"])
	}

	cache := mustObj("/cache/settings")
	if _, ok := cache["current_values"].(map[string]any); !ok {
		t.Fatalf("cache/settings.current_values missing %v", cache)
	}

	mem := mustObj("/v1/memory")
	if _, ok := mem["total"].(float64); !ok {
		t.Fatalf("memory.total missing %v", mem)
	}
	pl := mustObj("/claude-code/plugins")
	if _, ok := pl["count"].(float64); !ok {
		t.Fatalf("plugins.count missing %v", pl)
	}

	fields := doJSON(t, h, "GET", "/public/agents/fields", "", nil)
	if fields.Code != 200 {
		t.Fatalf("public agents fields %d %s", fields.Code, fields.Body.String())
	}
	var fv any
	_ = json.Unmarshal(fields.Body.Bytes(), &fv)
	if _, ok := fv.([]any); !ok {
		t.Fatalf("public/agents/fields want array got %s", fields.Body.String())
	}

	raw, err := base64.RawURLEncoding.DecodeString(strings.Split(jwt, ".")[1])
	if err != nil {
		t.Fatalf("jwt payload %v", err)
	}
	if !strings.Contains(string(raw), `"premium_user":true`) {
		t.Fatalf("jwt missing premium_user %s", raw)
	}

	inv := doJSON(t, h, "POST", "/invitation/new", sess, map[string]any{"user_id": "admin"})
	if inv.Code != 200 {
		t.Fatalf("invitation %d %s", inv.Code, inv.Body.String())
	}
	im := decodeBody(t, inv.Body.Bytes())
	mustKeys(t, im, "id", "user_id", "is_accepted", "expires", "expires_at", "created_at")
	if im["user_id"] != "admin" {
		t.Fatalf("invitation user_id %v", im["user_id"])
	}

	ul2 := mustObj("/user/list")
	users, _ := ul2["users"].([]any)
	if len(users) == 0 {
		t.Fatalf("user list empty after login %s", ul.Body.String())
	}
	adminRow, _ := users[0].(map[string]any)
	if _, ok := adminRow["key_count"]; !ok {
		t.Fatalf("user list missing key_count %v", adminRow)
	}
	if _, ok := adminRow["updated_at"]; !ok {
		t.Fatalf("user list missing updated_at %v", adminRow)
	}

	ag := doJSON(t, h, "POST", "/v1/access_group", sess, map[string]any{
		"access_group_name": "eng-group", "access_model_names": []any{"gpt-4o-mini"},
	})
	if ag.Code != 200 {
		t.Fatalf("access group %d %s", ag.Code, ag.Body.String())
	}
	agm := decodeBody(t, ag.Body.Bytes())
	mustKeys(t, agm, "access_group_id", "access_group_name", "access_model_names", "access_mcp_server_ids", "assigned_key_ids", "created_at", "updated_at")
	agl := doJSON(t, h, "GET", "/v1/access_group", sess, nil)
	if agl.Body.Bytes()[0] != '[' || !strings.Contains(agl.Body.String(), "eng-group") {
		t.Fatalf("access group list %s", agl.Body.String())
	}

	br := doJSON(t, h, "POST", "/budget/new", sess, map[string]any{"budget_id": "dash-budget", "max_budget": 12.0})
	if br.Code != 200 {
		t.Fatalf("budget new %s", br.Body.String())
	}
	bm := decodeBody(t, br.Body.Bytes())
	mustKeys(t, bm, "budget_id", "created_at", "updated_at")
	paged := mustObj("/management/v1/budgets")
	if _, ok := paged["data"].([]any); !ok {
		t.Fatalf("management budgets data %v", paged)
	}
	meta, _ := paged["meta"].(map[string]any)
	if meta["total_count"] == nil {
		t.Fatalf("management budgets meta %v", paged)
	}
	if _, ok := paged["links"].(map[string]any); !ok {
		t.Fatalf("management budgets links %v", paged)
	}

	projList := doJSON(t, h, "GET", "/project/list", sess, nil)
	if projList.Code != 200 || len(projList.Body.Bytes()) == 0 || projList.Body.Bytes()[0] != '[' {
		t.Fatalf("project list want array %s", projList.Body.String())
	}

	avail := doJSON(t, h, "GET", "/team/available", sess, nil)
	if avail.Code != 200 || avail.Body.Bytes()[0] != '[' {
		t.Fatalf("team available want array %s", avail.Body.String())
	}

	eu := mustObj("/management/v1/spend_logs/end_users")
	if _, ok := eu["data"].([]any); !ok {
		t.Fatalf("spend log end users %v", eu)
	}
}

func TestUISessionSurvivesRestart(t *testing.T) {
	s, master := testEnv(t)
	h := s.Handler()
	login := doJSON(t, h, "POST", "/v2/login", "", map[string]any{"username": "admin", "password": master})
	if login.Code != 200 {
		t.Fatal(login.Body.String())
	}
	var lj map[string]any
	_ = json.Unmarshal(login.Body.Bytes(), &lj)
	sess, _ := lj["key"].(string)
	if sess == "" {
		t.Fatal("missing session key")
	}

	s.mu.Lock()
	s.sessions = map[string]sessionRec{}
	s.mu.Unlock()

	tl := doJSON(t, h, "GET", "/team/list", sess, nil)
	if tl.Code != 200 || len(tl.Body.Bytes()) == 0 || tl.Body.Bytes()[0] != '[' {
		t.Fatalf("session after restart %d %s", tl.Code, tl.Body.String())
	}

	req := httptest.NewRequest(http.MethodGet, "/organization/list", nil)
	req.Header.Set("api-key", "Bearer "+sess)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("api-key header %d %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/key/list", nil)
	req.Header.Set("x-api-key", sess)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("x-api-key header %d %s", rec.Code, rec.Body.String())
	}
}
