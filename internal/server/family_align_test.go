package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sunqirui1987/xhub/internal/httpx"
)

func TestAllFamilyFrozenKeys(t *testing.T) {
	s, master := testEnv(t)
	h := s.Handler()
	sk := mintLLM(t, s, master)

	type tc struct {
		name   string
		method string
		path   string
		tok    string
		body   map[string]any
		keys   []string
	}
	cases := []tc{
		{"images", "POST", "/v1/images/generations", sk, map[string]any{"model": "dall-e-3", "prompt": "cat"}, []string{"created", "data"}},
		{"moderations", "POST", "/v1/moderations", sk, map[string]any{"model": "omni-moderation-latest", "input": "x"}, []string{"id", "model", "results"}},
		{"rerank", "POST", "/v1/rerank", sk, map[string]any{"model": "r", "query": "q", "documents": []any{"a"}}, []string{"id", "results", "meta"}},
		{"transcriptions", "POST", "/v1/audio/transcriptions", sk, map[string]any{"model": "whisper-1"}, []string{"text", "language", "duration", "segments"}},
		{"responses", "POST", "/v1/responses", sk, map[string]any{"model": "gpt-4o-mini", "input": "hi"}, []string{"id", "object", "status", "output", "usage", "model", "created_at"}},
		{"files", "POST", "/v1/files", sk, map[string]any{"purpose": "batch", "filename": "a.jsonl"}, []string{"id", "object", "bytes", "created_at", "filename", "purpose", "status"}},
		{"batches", "POST", "/v1/batches", sk, map[string]any{"input_file_id": "file_1", "endpoint": "/v1/chat/completions"}, []string{"id", "object", "endpoint", "status", "input_file_id", "output_file_id", "request_counts", "created_at"}},
		{"assistants", "POST", "/v1/assistants", sk, map[string]any{"model": "gpt-4o-mini", "name": "bot"}, []string{"id", "object", "created_at", "model"}},
		{"threads", "POST", "/v1/threads", sk, map[string]any{"messages": []any{}}, []string{"id", "object", "created_at"}},
		{"fine_tuning", "POST", "/v1/fine_tuning/jobs", sk, map[string]any{"model": "gpt-4o-mini", "training_file": "file_1"}, []string{"id", "object", "model", "status", "created_at", "fine_tuned_model"}},
		{"containers", "POST", "/v1/containers", sk, map[string]any{"name": "c1"}, []string{"id", "object", "name", "status", "created_at"}},
		{"videos", "POST", "/v1/videos", sk, map[string]any{"model": "sora", "prompt": "clip"}, []string{"id", "object", "status", "model", "created_at"}},
		{"ocr", "POST", "/v1/ocr", sk, map[string]any{"url": "https://x"}, []string{"results", "usage"}},
		{"rag", "POST", "/v1/rag/query", sk, map[string]any{"query": "q"}, []string{"results", "usage"}},
		{"indexes", "POST", "/v1/indexes", sk, map[string]any{"name": "idx"}, []string{"id", "object", "name", "status", "created_at"}},
		{"evals", "POST", "/v1/evals", sk, map[string]any{"name": "qa-eval"}, []string{"id", "object", "name", "status", "created_at"}},
		{"interactions", "POST", "/v1beta/interactions", sk, map[string]any{"model": "gemini-2.5-flash", "input": "hi"}, []string{"id", "status", "output"}},
		{"realtime", "POST", "/v1/realtime/client_secrets", sk, map[string]any{"model": "gpt-4o-realtime", "modalities": []any{"text"}, "voice": "alloy"}, []string{"id", "object", "model", "client_secret"}},
		{"gemini", "POST", "/v1beta/models/gemini-pro:generateContent", sk, map[string]any{"contents": []any{}}, []string{"candidates", "usageMetadata"}},
		{"guardrails", "POST", "/guardrails", master, map[string]any{"guardrail_name": "pii", "litellm_params": map[string]any{"guardrail": "presidio"}}, []string{"guardrail_id", "guardrail_name", "litellm_params", "created_at"}},
		{"credentials", "POST", "/credentials", master, map[string]any{"credential_name": "openai", "credential_values": map[string]any{"api_key": "sk-secret"}}, []string{"credential_name", "credential_info"}},
		{"customer", "POST", "/customer/new", master, map[string]any{"user_id": "end_123", "max_budget": 5.0}, []string{"user_id", "spend", "max_budget", "blocked"}},
		{"invitation", "POST", "/invitation/new", master, map[string]any{"user_email": "n@x.com"}, []string{"id", "user_id", "is_accepted", "expires"}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			no := doJSON(t, h, c.method, c.path, "", c.body)
			if no.Code != 401 {
				t.Fatalf("%s unauth want 401 got %d %s", c.path, no.Code, no.Body.String())
			}
			if no.Result().Header.Get("x-litellm-call-id") == "" {
				t.Fatalf("%s 401 missing call-id", c.path)
			}
			rec := doJSON(t, h, c.method, c.path, c.tok, c.body)
			if rec.Code != 200 {
				t.Fatalf("%s %d %s", c.path, rec.Code, rec.Body.String())
			}
			assertNotStub(t, rec.Body.String())
			assertLiteLLMHeaders(t, rec, c.path)
			m := decodeBody(t, rec.Body.Bytes())
			mustKeys(t, m, c.keys...)
			if c.name == "credentials" {
				if _, ok := m["credential_values"]; ok {
					t.Fatal("credential_values leaked")
				}
				if strings.Contains(rec.Body.String(), "sk-secret") {
					t.Fatalf("secret leaked %s", rec.Body.String())
				}
			}
		})
	}

	chat := doJSON(t, h, "POST", "/v1/chat/completions", sk, map[string]any{
		"model": "gpt-4o-mini", "messages": []any{map[string]any{"role": "user", "content": "hi"}},
	})
	cm := decodeBody(t, chat.Body.Bytes())
	mustKeys(t, cm, "id", "object", "created", "model", "choices", "usage", "system_fingerprint")

	emb := doJSON(t, h, "POST", "/v1/embeddings", sk, map[string]any{"model": "text-embedding-3-small", "input": "hi"})
	mustKeys(t, decodeBody(t, emb.Body.Bytes()), "object", "data", "model", "usage")

	usr := doJSON(t, h, "POST", "/user/new", master, map[string]any{"user_email": "a@b.c", "user_role": "internal_user"})
	mustKeys(t, decodeBody(t, usr.Body.Bytes()), "user_id", "user_email", "user_role", "spend", "max_budget", "teams", "created_at")

	tm := doJSON(t, h, "POST", "/team/new", master, map[string]any{"team_alias": "eng"})
	mustKeys(t, decodeBody(t, tm.Body.Bytes()), "team_id", "team_alias", "models", "spend", "max_budget", "members_with_roles")

	pj := doJSON(t, h, "POST", "/project/new", master, map[string]any{"project_alias": "p"})
	mustKeys(t, decodeBody(t, pj.Body.Bytes()), "project_id", "project_alias", "blocked", "created_at")

	hd := doJSON(t, h, "GET", "/health/readiness/details", "", nil)
	mustKeys(t, decodeBody(t, hd.Body.Bytes()), "status", "healthy_count", "unhealthy_count", "details")

	cs := doJSON(t, h, "GET", "/cache/settings", master, nil)
	if cs.Code != http.StatusNotFound {
		t.Fatalf("cache settings want 404 got %d %s", cs.Code, cs.Body.String())
	}

	mi := doJSON(t, h, "GET", "/model/info", master, nil)
	var mj map[string]any
	_ = json.Unmarshal(mi.Body.Bytes(), &mj)
	data, _ := mj["data"].([]any)
	if len(data) == 0 {
		t.Fatal("model info empty")
	}
	row, _ := data[0].(map[string]any)
	mustKeys(t, row, "model_name", "litellm_params", "model_info", "blocked")

	bg := doJSON(t, h, "POST", "/budget/new", master, map[string]any{
		"max_budget": 10.0, "soft_budget": 8.0, "tpm_limit": 1000, "rpm_limit": 60, "budget_duration": "30d",
	})
	if bg.Code != 200 {
		t.Fatal(bg.Body.String())
	}
	bm := decodeBody(t, bg.Body.Bytes())
	mustKeys(t, bm, "budget_id", "max_budget", "soft_budget", "tpm_limit", "rpm_limit", "budget_duration", "budget_reset_at")
	if bm["budget_reset_at"] == nil || bm["budget_reset_at"] == "" {
		t.Fatalf("budget_reset_at empty %v", bm)
	}
	if bm["budget_duration"] != "30d" {
		t.Fatalf("budget_duration %v", bm["budget_duration"])
	}
}

func TestErrorEnvelopeAlignment(t *testing.T) {
	s, master := testEnv(t)
	h := s.Handler()

	chat := doJSON(t, h, "POST", "/v1/chat/completions", "", map[string]any{
		"model": "gpt-4o-mini", "messages": []any{map[string]any{"role": "user", "content": "hi"}},
	})
	if chat.Code != 401 {
		t.Fatalf("chat 401 got %d %s", chat.Code, chat.Body.String())
	}
	cm := decodeBody(t, chat.Body.Bytes())
	errObj, _ := cm["error"].(map[string]any)
	if errObj["message"] == nil || errObj["type"] == nil {
		t.Fatalf("chat openai envelope %s", chat.Body.String())
	}
	if _, ok := errObj["param"]; !ok {
		t.Fatalf("chat missing param %s", chat.Body.String())
	}
	if _, ok := errObj["code"]; !ok {
		t.Fatalf("chat missing code %s", chat.Body.String())
	}
	if cm["type"] == "error" {
		t.Fatalf("chat used anthropic envelope %s", chat.Body.String())
	}

	msg := doJSON(t, h, "POST", "/v1/messages", "", map[string]any{
		"model": "gpt-4o-mini", "messages": []any{map[string]any{"role": "user", "content": "hi"}}, "max_tokens": 8,
	})
	if msg.Code != 401 {
		t.Fatalf("messages 401 got %d %s", msg.Code, msg.Body.String())
	}
	mm := decodeBody(t, msg.Body.Bytes())
	if mm["type"] != "error" {
		t.Fatalf("messages want type=error %s", msg.Body.String())
	}
	me, _ := mm["error"].(map[string]any)
	if me["type"] == nil || me["message"] == nil {
		t.Fatalf("messages anthropic envelope %s", msg.Body.String())
	}
	if _, ok := me["param"]; ok {
		t.Fatalf("messages should not have openai param %s", msg.Body.String())
	}
	if msg.Result().Header.Get("x-litellm-call-id") == "" {
		t.Fatal("messages 401 missing call-id")
	}

	gem := doJSON(t, h, "POST", "/v1beta/models/gemini-pro:generateContent", "", map[string]any{
		"contents": []any{},
	})
	if gem.Code != 401 {
		t.Fatalf("gemini 401 got %d %s", gem.Code, gem.Body.String())
	}
	gm := decodeBody(t, gem.Body.Bytes())
	ge, _ := gm["error"].(map[string]any)
	if ge["code"] == nil || ge["message"] == nil || ge["status"] != "UNAUTHENTICATED" {
		t.Fatalf("gemini google envelope %s", gem.Body.String())
	}
	if gm["type"] == "error" {
		t.Fatalf("gemini used anthropic envelope %s", gem.Body.String())
	}

	over := doJSON(t, h, "POST", "/key/generate", master, map[string]any{"key_type": "llm_api", "max_budget": 0})
	var g map[string]any
	_ = json.Unmarshal(over.Body.Bytes(), &g)
	broke := doJSON(t, h, "POST", "/v1/messages", g["key"].(string), map[string]any{
		"model": "gpt-4o-mini", "messages": []any{map[string]any{"role": "user", "content": "hi"}}, "max_tokens": 8,
	})
	if broke.Code != 429 {
		t.Fatalf("messages budget want 429 got %d %s", broke.Code, broke.Body.String())
	}
	if broke.Result().Header.Get("Retry-After") == "" {
		t.Fatal("messages 429 missing Retry-After")
	}
	bm := decodeBody(t, broke.Body.Bytes())
	if bm["type"] != "error" {
		t.Fatalf("messages 429 envelope %s", broke.Body.String())
	}
	be, _ := bm["error"].(map[string]any)
	if be["type"] != "budget_exceeded" {
		t.Fatalf("messages 429 type %v", be["type"])
	}

	chat429 := doJSON(t, h, "POST", "/v1/chat/completions", g["key"].(string), map[string]any{
		"model": "gpt-4o-mini", "messages": []any{map[string]any{"role": "user", "content": "hi"}},
	})
	if chat429.Code != 429 {
		t.Fatalf("chat budget want 429 got %d %s", chat429.Code, chat429.Body.String())
	}
	ce := decodeBody(t, chat429.Body.Bytes())
	ceo, _ := ce["error"].(map[string]any)
	if ceo["type"] != "budget_exceeded" || ceo["code"] == nil {
		t.Fatalf("chat 429 openai envelope %s", chat429.Body.String())
	}
}

func TestBudgetSettingsAlignment(t *testing.T) {
	s, master := testEnv(t)
	h := s.Handler()
	created := doJSON(t, h, "POST", "/budget/new", master, map[string]any{"max_budget": 12.5, "budget_duration": "7d"})
	bm := decodeBody(t, created.Body.Bytes())
	id, _ := bm["budget_id"].(string)
	st := doJSON(t, h, "GET", "/budget/settings?budget_id="+id, master, nil)
	if st.Code != 200 {
		t.Fatal(st.Body.String())
	}
	var fields []map[string]any
	if err := json.Unmarshal(st.Body.Bytes(), &fields); err != nil {
		t.Fatalf("settings list %s", st.Body.String())
	}
	names := map[string]bool{}
	for _, f := range fields {
		n, _ := f["field_name"].(string)
		names[n] = true
		if _, ok := f["field_type"]; !ok {
			t.Fatalf("missing field_type %v", f)
		}
		if _, ok := f["field_value"]; !ok {
			t.Fatalf("missing field_value %v", f)
		}
	}
	for _, n := range []string{"max_budget", "soft_budget", "tpm_limit", "rpm_limit", "budget_duration", "max_parallel_requests", "model_max_budget"} {
		if !names[n] {
			t.Fatalf("settings missing %s in %s", n, st.Body.String())
		}
	}
}

func assertLiteLLMHeaders(t *testing.T, rec interface{ Result() *http.Response }, path string) {
	t.Helper()
	h := rec.Result().Header
	if h.Get("x-litellm-call-id") == "" {
		t.Fatalf("%s missing x-litellm-call-id", path)
	}
	if h.Get("x-litellm-version") == "" {
		t.Fatalf("%s missing x-litellm-version", path)
	}
	if h.Get("Content-Type") == "" {
		t.Fatalf("%s missing Content-Type", path)
	}
	if isDataPlanePath(path) && h.Get("x-litellm-response-duration-ms") == "" {
		t.Fatalf("%s missing x-litellm-response-duration-ms", path)
	}
}

func TestRequestHeadersAlignment(t *testing.T) {
	s, master := testEnv(t)
	h := s.Handler()
	sk := mintLLM(t, s, master)

	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(
		`{"model":"gpt-4o-mini","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("x-litellm-api-key", sk)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-litellm-tags", "prod")
	req.Header.Set("x-litellm-end-user-id", "u1")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("x-litellm-api-key %d %s", rec.Code, rec.Body.String())
	}
	assertLiteLLMHeaders(t, rec, "/v1/chat/completions")
	if rec.Result().Header.Get("x-litellm-model-name") != "gpt-4o-mini" {
		t.Fatalf("model-name %q", rec.Result().Header.Get("x-litellm-model-name"))
	}
	cm := decodeBody(t, rec.Body.Bytes())
	mustKeys(t, cm, "id", "object", "choices", "usage", "model")

	idem := "idem-" + httpx.CallID()
	first := doJSONHeaders(t, h, "POST", "/v1/images/generations", sk, map[string]any{"model": "dall-e-3", "prompt": "cat"}, map[string]string{"Idempotency-Key": idem})
	if first.Code != 200 {
		t.Fatal(first.Body.String())
	}
	second := doJSONHeaders(t, h, "POST", "/v1/images/generations", sk, map[string]any{"model": "dall-e-3", "prompt": "cat"}, map[string]string{"Idempotency-Key": idem})
	if first.Body.String() != second.Body.String() {
		t.Fatalf("idempotency replay mismatch\n%s\n%s", first.Body.String(), second.Body.String())
	}
}

func doJSONHeaders(t *testing.T, h http.Handler, method, path, token string, body map[string]any, extra map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(method, path, strings.NewReader(string(b)))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	for k, v := range extra {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}
