package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sunqirui1987/xhub/internal/config"
)

func TestNativeOpsProxyAndHeaders(t *testing.T) {
	s, master := testEnv(t)
	var lastPath string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lastPath = r.URL.Path
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "/images"):
			_ = json.NewEncoder(w).Encode(map[string]any{"created": 1, "data": []any{map[string]any{"url": "https://example.invalid/img/1"}}})
		case strings.Contains(r.URL.Path, "/audio/speech"):
			w.Header().Set("Content-Type", "audio/mpeg")
			_, _ = w.Write([]byte("ID3"))
		case strings.Contains(r.URL.Path, "/audio/transcriptions"), strings.Contains(r.URL.Path, "/audio/translations"):
			_ = json.NewEncoder(w).Encode(map[string]any{"text": "hello", "language": "en", "duration": 1.0, "segments": []any{}})
		case strings.Contains(r.URL.Path, "/moderations"):
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "modr_test", "model": body["model"], "results": []any{map[string]any{"flagged": false, "categories": map[string]any{}, "category_scores": map[string]any{}}}})
		case strings.Contains(r.URL.Path, "/rerank"):
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "rerank_test", "results": []any{map[string]any{"index": 0, "relevance_score": 0.9}}, "meta": map[string]any{"tokens": map[string]any{"input_tokens": 1}}})
		case strings.Contains(r.URL.Path, "/responses"):
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "resp_test", "object": "response", "status": "completed", "model": body["model"], "created_at": 1, "output": []any{}, "usage": map[string]any{"input_tokens": 8, "output_tokens": 2, "total_tokens": 10}})
		case strings.Contains(r.URL.Path, "/videos"):
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "video_test", "object": "video", "status": "queued", "model": body["model"]})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(up.Close)
	for i := range s.Cfg.ModelList {
		s.Cfg.ModelList[i].LiteLLMParams["api_base"] = up.URL
	}
	h := s.Handler()
	gen := doJSON(t, h, "POST", "/key/generate", master, map[string]any{
		"key_type": "llm_api", "tpm_limit": 100000, "rpm_limit": 60, "max_budget": 10,
	})
	g := decodeBody(t, gen.Body.Bytes())
	sk := g["key"].(string)

	cases := []struct {
		path   string
		body   map[string]any
		keys   []string
		upPath string
	}{
		{"/v1/images/generations", map[string]any{"model": "dall-e-3", "prompt": "cat"}, []string{"created", "data"}, "/images/generations"},
		{"/v1/images/edits", map[string]any{"model": "dall-e-3", "prompt": "cat"}, []string{"created", "data"}, "/images/edits"},
		{"/v1/audio/transcriptions", map[string]any{"model": "whisper-1"}, []string{"text", "language", "duration", "segments"}, "/audio/transcriptions"},
		{"/v1/audio/translations", map[string]any{"model": "whisper-1"}, []string{"text", "language", "duration", "segments"}, "/audio/translations"},
		{"/v1/rerank", map[string]any{"model": "rerank-english-v3.0", "query": "q", "documents": []any{"a", "b"}}, []string{"id", "results", "meta"}, "/rerank"},
		{"/v1/responses", map[string]any{"model": "gpt-4o-mini", "input": "hi"}, []string{"id", "object", "status", "output", "usage", "model", "created_at"}, "/responses"},
		{"/v1/videos", map[string]any{"model": "sora", "prompt": "clip"}, []string{"id", "object", "status"}, "/videos"},
		{"/v1/moderations", map[string]any{"model": "omni-moderation-latest", "input": "x"}, []string{"id", "model", "results"}, "/moderations"},
	}
	for _, c := range cases {
		lastPath = ""
		rec := doJSON(t, h, "POST", c.path, sk, c.body)
		if rec.Code != 200 {
			t.Fatalf("%s %d %s", c.path, rec.Code, rec.Body.String())
		}
		if !strings.Contains(lastPath, c.upPath) {
			t.Fatalf("%s upstream path %q want %q", c.path, lastPath, c.upPath)
		}
		mustKeys(t, decodeBody(t, rec.Body.Bytes()), c.keys...)
		if rec.Result().Header.Get("x-litellm-model-name") == "" {
			t.Fatalf("%s missing model-name", c.path)
		}
		if rec.Result().Header.Get("x-litellm-key-tpm-limit") == "" {
			t.Fatalf("%s missing tpm header", c.path)
		}
		if rec.Result().Header.Get("x-litellm-response-cost") == "" {
			t.Fatalf("%s missing cost header %v", c.path, rec.Result().Header)
		}
		if rec.Result().Header.Get("x-litellm-response-cost-original") == "" {
			t.Fatalf("%s missing cost-original", c.path)
		}
		if rec.Result().Header.Get("x-litellm-response-cost-input") == "" || rec.Result().Header.Get("x-litellm-response-cost-output") == "" {
			t.Fatalf("%s missing cost split", c.path)
		}
	}

	lastPath = ""
	speech := doJSON(t, h, "POST", "/v1/audio/speech", sk, map[string]any{"model": "tts-1", "input": "hi", "voice": "alloy"})
	if speech.Code != 200 || speech.Body.Len() == 0 {
		t.Fatalf("speech %d %s", speech.Code, speech.Body.String())
	}
	if !strings.Contains(lastPath, "/audio/speech") {
		t.Fatalf("speech upstream path %q", lastPath)
	}
	if speech.Result().Header.Get("x-litellm-model-name") == "" {
		t.Fatal("speech missing model-name")
	}

	s.Cfg.ModelList = append(s.Cfg.ModelList, config.ModelEntry{
		ModelName: "cohere-rerank",
		LiteLLMParams: map[string]any{
			"model": "cohere/rerank-v3", "api_key": "k", "api_base": "http://127.0.0.1:9",
		},
	})
	bad := doJSON(t, h, "POST", "/v1/rerank", sk, map[string]any{"model": "cohere-rerank", "query": "q", "documents": []any{"a"}})
	if bad.Code == 400 && strings.Contains(bad.Body.String(), "provider_not_implemented") {
		t.Fatalf("cohere must be a real adapter, got %s", bad.Body.String())
	}
	if bad.Code != 200 && bad.Code != 502 {
		t.Fatalf("cohere adapter want 200/502 got %d %s", bad.Code, bad.Body.String())
	}
}

func TestMissingResourceTyped404(t *testing.T) {
	s, master := testEnv(t)
	h := s.Handler()
	sk := mintLLM(t, s, master)

	key := doJSON(t, h, "GET", "/key/info?key=sk-does-not-exist", master, nil)
	if !isTypedResource404(key) {
		t.Fatalf("key info %d %s", key.Code, key.Body.String())
	}
	usr := doJSON(t, h, "GET", "/user/info?user_id=missing-user", master, nil)
	if !isTypedResource404(usr) {
		t.Fatalf("user info %d %s", usr.Code, usr.Body.String())
	}
	tm := doJSON(t, h, "GET", "/team/info?team_id=missing-team", master, nil)
	if !isTypedResource404(tm) {
		t.Fatalf("team info %d %s", tm.Code, tm.Body.String())
	}
	org := doJSON(t, h, "GET", "/organization/info?organization_id=missing-org", master, nil)
	if !isTypedResource404(org) {
		t.Fatalf("org info %d %s", org.Code, org.Body.String())
	}
	pj := doJSON(t, h, "GET", "/project/info?project_id=missing-proj", master, nil)
	if !isTypedResource404(pj) {
		t.Fatalf("project info %d %s", pj.Code, pj.Body.String())
	}
	fl := doJSON(t, h, "GET", "/v1/files/file_missing", sk, nil)
	if !isTypedResource404(fl) {
		t.Fatalf("file %d %s", fl.Code, fl.Body.String())
	}
	bt := doJSON(t, h, "GET", "/v1/batches/batch_missing", sk, nil)
	if !isTypedResource404(bt) {
		t.Fatalf("batch %d %s", bt.Code, bt.Body.String())
	}

	created := doJSON(t, h, "POST", "/user/new", master, map[string]any{"user_id": "real-u", "user_email": "r@x", "auto_create_key": false})
	if created.Code != 200 {
		t.Fatal(created.Body.String())
	}
	ok := doJSON(t, h, "GET", "/user/info?user_id=real-u", master, nil)
	if ok.Code != 200 {
		t.Fatalf("existing user %d %s", ok.Code, ok.Body.String())
	}

	miss := doJSON(t, h, "GET", "/this-route-is-not-in-catalog", master, nil)
	if miss.Code != 404 {
		t.Fatalf("mux miss %d %s", miss.Code, miss.Body.String())
	}
	mm := decodeBody(t, miss.Body.Bytes())
	errObj, _ := mm["error"].(map[string]any)
	if errObj["type"] != "not_found" {
		t.Fatalf("mux 404 envelope %s", miss.Body.String())
	}
}

func TestGuardrailApplyAndChatBlock(t *testing.T) {
	s, master := testEnv(t)
	h := s.Handler()
	sk := mintLLM(t, s, master)
	gr := doJSON(t, h, "POST", "/guardrails", master, map[string]any{
		"guardrail_name": "block-secret",
		"litellm_params": map[string]any{
			"guardrail": "keyword", "mode": "pre_call", "default_on": true,
			"blocked_words": []any{"BLOCKME"},
		},
	})
	if gr.Code != 200 {
		t.Fatal(gr.Body.String())
	}
	ap := doJSON(t, h, "POST", "/apply_guardrail", master, map[string]any{
		"guardrail_name": "block-secret", "text": "please BLOCKME now",
	})
	if ap.Code != 200 {
		t.Fatal(ap.Body.String())
	}
	am := decodeBody(t, ap.Body.Bytes())
	if am["action"] != "block" || am["blocked"] != true {
		t.Fatalf("apply %v", am)
	}
	ap2 := doJSON(t, h, "POST", "/guardrails/apply_guardrail", master, map[string]any{
		"guardrail_name": "block-secret", "text": "hello",
	})
	if decodeBody(t, ap2.Body.Bytes())["action"] != "allow" {
		t.Fatalf("allow %s", ap2.Body.String())
	}
	chat := doJSON(t, h, "POST", "/v1/chat/completions", sk, map[string]any{
		"model":    "gpt-4o-mini",
		"messages": []any{map[string]any{"role": "user", "content": "do BLOCKME"}},
	})
	if chat.Code != 400 || !strings.Contains(chat.Body.String(), "Guardrail") {
		t.Fatalf("chat guardrail %d %s", chat.Code, chat.Body.String())
	}
	ok := doJSON(t, h, "POST", "/v1/chat/completions", sk, map[string]any{
		"model":    "gpt-4o-mini",
		"messages": []any{map[string]any{"role": "user", "content": "hello"}},
	})
	if ok.Code != 200 {
		t.Fatalf("chat allow %d %s", ok.Code, ok.Body.String())
	}
}

func TestCustomerCacheCallbackSCIM(t *testing.T) {
	s, master := testEnv(t)
	h := s.Handler()
	cr := doJSON(t, h, "POST", "/customer/new", master, map[string]any{"user_id": "end_123", "max_budget": 5.0})
	if cr.Code != 200 {
		t.Fatal(cr.Body.String())
	}
	lst := doJSON(t, h, "GET", "/customer/list", master, nil)
	if lst.Code != 200 {
		t.Fatal(lst.Body.String())
	}
	lm := decodeBody(t, lst.Body.Bytes())
	data, _ := lm["data"].([]any)
	if len(data) == 0 {
		t.Fatalf("customer list empty %v", lm)
	}
	row, _ := data[0].(map[string]any)
	mustKeys(t, row, "user_id", "spend", "blocked")
	el := doJSON(t, h, "GET", "/end_user/list", master, nil)
	if el.Code != 200 {
		t.Fatal(el.Body.String())
	}

	ping := doJSON(t, h, "GET", "/cache/ping", master, nil)
	if ping.Code != 200 {
		t.Fatal(ping.Body.String())
	}
	pm := decodeBody(t, ping.Body.Bytes())
	if pm["status"] == "ok" && pm["ping"] == nil {
		t.Fatalf("ping stub %v", pm)
	}
	if pm["ping"] != "pong" && pm["status"] != "healthy" {
		t.Fatalf("ping payload %v", pm)
	}

	_ = doJSON(t, h, "POST", "/callbacks/configs", master, map[string]any{"callback_name": "langfuse"})
	cb := doJSON(t, h, "GET", "/get/config/callbacks", master, nil)
	if cb.Code != 200 {
		t.Fatal(cb.Body.String())
	}
	if !strings.Contains(cb.Body.String(), "langfuse") && !strings.Contains(cb.Body.String(), "callbacks") {
		t.Fatalf("callbacks get %s", cb.Body.String())
	}
	del := doJSON(t, h, "POST", "/config/callback/delete", master, map[string]any{"callback_name": "langfuse"})
	if del.Code != 200 {
		t.Fatal(del.Body.String())
	}
	after := doJSON(t, h, "GET", "/get/config/callbacks", master, nil)
	if strings.Contains(after.Body.String(), "langfuse") {
		t.Fatalf("callback still listed %s", after.Body.String())
	}

	users := doJSON(t, h, "GET", "/Users", master, nil)
	if users.Code != 200 {
		t.Fatal(users.Body.String())
	}
	um := decodeBody(t, users.Body.Bytes())
	if _, ok := um["schemas"]; !ok {
		t.Fatalf("scim users schemas %v", um)
	}
	if _, ok := um["Resources"]; !ok {
		t.Fatalf("scim users Resources %v", um)
	}
	groups := doJSON(t, h, "GET", "/Groups", master, nil)
	gm := decodeBody(t, groups.Body.Bytes())
	if _, ok := gm["Resources"]; !ok {
		t.Fatalf("scim groups %v", gm)
	}
}
