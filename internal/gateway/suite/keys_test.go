package suite

import (
	"strings"
	"testing"
)

func TestKeyGenerateFrozenResponse(t *testing.T) {
	s, master := testEnv(t)
	h := s.Handler()
	rec := doJSON(t, h, "POST", "/key/generate", master, map[string]any{
		"key_alias": "checkout-prod", "models": []string{"gpt-4o-mini"}, "max_budget": 10.0,
		"soft_budget": 8.0, "tpm_limit": 1000, "rpm_limit": 60, "budget_duration": "30d",
		"key_type": "llm_api", "metadata": map[string]any{"app": "pay"}, "tags": []string{"prod"},
	})
	if rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	m := decodeBody(t, rec.Body.Bytes())
	mustKeys(t, m, "key", "key_name", "key_alias", "expires", "token_id", "user_id", "team_id", "models", "max_budget", "spend",
		"soft_budget", "tpm_limit", "rpm_limit", "max_parallel_requests", "budget_duration", "budget_reset_at", "created_at", "litellm_budget_table", "metadata", "blocked")
	if m["key_alias"] != "checkout-prod" {
		t.Fatalf("alias %v", m["key_alias"])
	}
	if m["soft_budget"] != 8.0 {
		t.Fatalf("soft_budget %v", m["soft_budget"])
	}
	if m["budget_duration"] != "30d" {
		t.Fatalf("budget_duration %v", m["budget_duration"])
	}
	if m["budget_reset_at"] == nil || m["budget_reset_at"] == "" {
		t.Fatalf("budget_reset_at %v", m["budget_reset_at"])
	}
	tbl, _ := m["litellm_budget_table"].(map[string]any)
	if tbl["soft_budget"] != 8.0 {
		t.Fatalf("budget table %v", tbl)
	}
	if strings.Contains(doJSON(t, h, "GET", "/key/list", master, nil).Body.String(), m["key"].(string)) {
		t.Fatal("list leaked plaintext")
	}
}

func TestKeyRegenerateAndResetSpend(t *testing.T) {
	s, master := testEnv(t)
	h := s.Handler()
	gen := doJSON(t, h, "POST", "/key/generate", master, map[string]any{"key_alias": "rot", "key_type": "llm_api", "max_budget": 5})
	g := decodeBody(t, gen.Body.Bytes())
	old := g["key"].(string)
	rot := doJSON(t, h, "POST", "/key/"+old+"/regenerate", master, map[string]any{"key_alias": "rot2"})
	if rot.Code != 200 {
		t.Fatal(rot.Body.String())
	}
	rm := decodeBody(t, rot.Body.Bytes())
	mustKeys(t, rm, "key", "key_name", "key_alias", "token_id", "spend")
	neu := rm["key"].(string)
	if neu == old {
		t.Fatal("regenerate returned same key")
	}
	if rm["key_alias"] != "rot2" {
		t.Fatalf("alias %v", rm["key_alias"])
	}
	oldChat := doJSON(t, h, "POST", "/v1/chat/completions", old, map[string]any{
		"model": "gpt-4o-mini", "messages": []any{map[string]any{"role": "user", "content": "hi"}},
	})
	if oldChat.Code != 401 {
		t.Fatalf("old key still works %d %s", oldChat.Code, oldChat.Body.String())
	}
	newChat := doJSON(t, h, "POST", "/v1/chat/completions", neu, map[string]any{
		"model": "gpt-4o-mini", "messages": []any{map[string]any{"role": "user", "content": "hi"}},
	})
	if newChat.Code != 200 {
		t.Fatalf("new key %d %s", newChat.Code, newChat.Body.String())
	}

	rs := doJSON(t, h, "POST", "/key/"+neu+"/reset_spend", master, map[string]any{"reset_to": 0})
	if rs.Code != 200 {
		t.Fatal(rs.Body.String())
	}
	sm := decodeBody(t, rs.Body.Bytes())
	if sm["spend"] != 0.0 && sm["spend"] != 0 {
		t.Fatalf("spend after reset %v", sm["spend"])
	}
}

func TestKeyAliasesHealthServiceAccount(t *testing.T) {
	s, master := testEnv(t)
	h := s.Handler()
	_ = doJSON(t, h, "POST", "/key/generate", master, map[string]any{"key_alias": "alpha", "key_type": "llm_api"})
	al := doJSON(t, h, "GET", "/key/aliases", master, nil)
	if al.Code != 200 {
		t.Fatal(al.Body.String())
	}
	am := decodeBody(t, al.Body.Bytes())
	mustKeys(t, am, "aliases", "total_count", "current_page", "total_pages")
	aliases, _ := am["aliases"].([]any)
	found := false
	for _, a := range aliases {
		if a == "alpha" {
			found = true
		}
	}
	if !found {
		t.Fatalf("aliases %v", aliases)
	}

	hh := doJSON(t, h, "POST", "/key/health", master, nil)
	if hh.Code != 200 {
		t.Fatal(hh.Body.String())
	}
	hm := decodeBody(t, hh.Body.Bytes())
	if hm["key"] != "healthy" {
		t.Fatalf("health %v", hm)
	}

	sa := doJSON(t, h, "POST", "/key/service-account/generate", master, map[string]any{
		"key_alias": "svc", "team_id": "team_1", "max_budget": 3.0,
	})
	if sa.Code != 200 {
		t.Fatal(sa.Body.String())
	}
	sm := decodeBody(t, sa.Body.Bytes())
	mustKeys(t, sm, "key", "key_name", "key_alias", "user_id", "team_id", "max_budget", "spend")
	if sm["user_id"] != nil {
		t.Fatalf("service account user_id %v", sm["user_id"])
	}
	if sm["team_id"] != "team_1" {
		t.Fatalf("team %v", sm["team_id"])
	}
}

func TestNativeImagesBudgetExceeded(t *testing.T) {
	s, master := testEnv(t)
	h := s.Handler()
	gen := doJSON(t, h, "POST", "/key/generate", master, map[string]any{"key_type": "llm_api", "max_budget": 0})
	g := decodeBody(t, gen.Body.Bytes())
	img := doJSON(t, h, "POST", "/v1/images/generations", g["key"].(string), map[string]any{"model": "dall-e-3", "prompt": "cat"})
	if img.Code != 429 {
		t.Fatalf("images budget want 429 got %d %s", img.Code, img.Body.String())
	}
	if img.Result().Header.Get("Retry-After") == "" {
		t.Fatal("missing Retry-After")
	}
	em := decodeBody(t, img.Body.Bytes())
	errObj, _ := em["error"].(map[string]any)
	if errObj["type"] != "budget_exceeded" {
		t.Fatalf("envelope %s", img.Body.String())
	}

	resp := doJSON(t, h, "POST", "/v1/responses", g["key"].(string), map[string]any{"model": "gpt-4o-mini", "input": "hi"})
	if resp.Code != 429 {
		t.Fatalf("responses budget want 429 got %d %s", resp.Code, resp.Body.String())
	}
}

func TestKeyUpdatePersistsLimits(t *testing.T) {
	s, master := testEnv(t)
	h := s.Handler()
	gen := doJSON(t, h, "POST", "/key/generate", master, map[string]any{"key_alias": "u", "key_type": "llm_api", "tpm_limit": 10})
	g := decodeBody(t, gen.Body.Bytes())
	upd := doJSON(t, h, "POST", "/key/update", master, map[string]any{
		"key": g["key"], "max_budget": 42.0, "tpm_limit": 99, "soft_budget": 20.0, "budget_duration": "7d",
	})
	if upd.Code != 200 {
		t.Fatal(upd.Body.String())
	}
	um := decodeBody(t, upd.Body.Bytes())
	if um["max_budget"] != 42.0 {
		t.Fatalf("max_budget %v", um["max_budget"])
	}
	if um["tpm_limit"] != float64(99) {
		t.Fatalf("tpm %v", um["tpm_limit"])
	}
	if um["soft_budget"] != 20.0 {
		t.Fatalf("soft %v", um["soft_budget"])
	}
	info := doJSON(t, h, "GET", "/key/info?key="+g["key"].(string), master, nil)
	im := decodeBody(t, info.Body.Bytes())
	row, _ := im["info"].(map[string]any)
	if row["max_budget"] != 42.0 {
		t.Fatalf("info after update %v", row)
	}
}
