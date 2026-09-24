package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/sunqirui1987/xhub/internal/llm"
)

type capturedUp struct {
	mu   sync.Mutex
	hits []capturedReq
}

type capturedReq struct {
	path   string
	auth   string
	model  string
	stream bool
}

func (c *capturedUp) serve(w http.ResponseWriter, r *http.Request) {
	raw, _ := io.ReadAll(r.Body)
	var body map[string]any
	_ = json.Unmarshal(raw, &body)
	model, _ := body["model"].(string)
	stream, _ := body["stream"].(bool)
	c.mu.Lock()
	c.hits = append(c.hits, capturedReq{path: r.URL.Path, auth: r.Header.Get("Authorization"), model: model, stream: stream})
	c.mu.Unlock()
	if stream {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"marker\":\"stream-marker\"}\n\ndata: [DONE]\n\n"))
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"id": "resp_test", "object": "response", "status": "completed",
		"model": model, "output": []any{map[string]any{"type": "message", "content": []any{map[string]any{"type": "output_text", "text": "output-marker"}}}},
		"choices": []any{map[string]any{"message": map[string]any{"role": "assistant", "content": "output-marker"}}},
		"usage":   map[string]any{"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
	})
}

func boolWord(v bool) string {
	if v {
		return "-stream"
	}
	return "-plain"
}

func (c *capturedUp) last() capturedReq {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.hits[len(c.hits)-1]
}

func TestCredentialBackedResponsesAndChat(t *testing.T) {
	s, master := testEnv(t)
	h := s.Handler()
	gen := doJSON(t, h, "POST", "/key/generate", master, map[string]any{"key_type": "llm_api"})
	if gen.Code != 200 {
		t.Fatal(gen.Body.String())
	}
	sk := decodeBody(t, gen.Body.Bytes())["key"].(string)
	upState := &capturedUp{}
	up := httptest.NewServer(http.HandlerFunc(upState.serve))
	t.Cleanup(up.Close)

	created := doJSON(t, h, "POST", "/credentials", master, map[string]any{
		"credential_name": "fennoai",
		"credential_info": map[string]any{"custom_llm_provider": "OpenAI"},
		"credential_values": map[string]any{
			"api_key":             "sk-fenno-test",
			"api_base":            up.URL,
			"custom_llm_provider": "OpenAI",
		},
	})
	if created.Code != 200 {
		t.Fatal(created.Body.String())
	}
	if strings.Contains(created.Body.String(), "credential_values") || strings.Contains(created.Body.String(), "sk-fenno-test") {
		t.Fatalf("create leaked secret %s", created.Body.String())
	}
	listed := doJSON(t, h, "GET", "/credentials", master, nil)
	if listed.Code != 200 || strings.Contains(listed.Body.String(), "sk-fenno-test") || !strings.Contains(listed.Body.String(), up.URL) {
		t.Fatalf("list %d %s", listed.Code, listed.Body.String())
	}
	got := doJSON(t, h, "GET", "/credentials/by_name/fennoai", master, nil)
	if got.Code != 200 || strings.Contains(got.Body.String(), "sk-fenno-test") || !strings.Contains(got.Body.String(), up.URL) {
		t.Fatalf("get %d %s", got.Code, got.Body.String())
	}
	patched := doJSON(t, h, "PATCH", "/credentials/fennoai", master, map[string]any{
		"credential_name": "fennoai",
		"credential_values": map[string]any{
			"api_base": up.URL + "/v1",
			"api_key":  "sk-f****est",
		},
		"credential_info": map[string]any{"custom_llm_provider": "OpenAI"},
	})
	if patched.Code != 200 || strings.Contains(patched.Body.String(), "sk-fenno-test") {
		t.Fatalf("patch %d %s", patched.Code, patched.Body.String())
	}
	if !strings.Contains(patched.Body.String(), up.URL+"/v1") {
		t.Fatalf("patch hid api_base %s", patched.Body.String())
	}

	added := doJSON(t, h, "POST", "/model/new", master, map[string]any{
		"model_name": "gpt-5.6-sol",
		"litellm_params": map[string]any{
			"model":                   "gpt-5.6-sol",
			"litellm_credential_name": "fennoai",
		},
	})
	if added.Code != 200 {
		t.Fatal(added.Body.String())
	}

	call := func(path string, stream bool) {
		t.Helper()
		body := map[string]any{"model": "gpt-5.6-sol", "stream": stream, "user": path + boolWord(stream)}
		if strings.Contains(path, "chat") {
			body["messages"] = []any{map[string]any{"role": "user", "content": path + boolWord(stream)}}
		} else {
			body["input"] = path + boolWord(stream)
		}
		rec := doJSON(t, h, "POST", path, sk, body)
		if rec.Code != 200 {
			t.Fatalf("%s stream=%v %d %s", path, stream, rec.Code, rec.Body.String())
		}
		if strings.Contains(rec.Body.String(), "provider_not_implemented") {
			t.Fatalf("%s still unimplemented %s", path, rec.Body.String())
		}
		want := "output-marker"
		if stream {
			want = "stream-marker"
		}
		if !strings.Contains(rec.Body.String(), want) {
			t.Fatalf("%s missing upstream body %s", path, rec.Body.String())
		}
		hit := upState.last()
		if hit.auth != "Bearer sk-fenno-test" {
			t.Fatalf("%s auth %q", path, hit.auth)
		}
		if hit.model != "gpt-5.6-sol" {
			t.Fatalf("%s model %q", path, hit.model)
		}
		if hit.stream != stream {
			t.Fatalf("%s stream %v", path, hit.stream)
		}
		if !strings.HasPrefix(hit.path, "/") {
			t.Fatalf("path %s", hit.path)
		}
		if strings.Contains(path, "chat") && !strings.HasSuffix(hit.path, "/chat/completions") {
			t.Fatalf("chat upstream %s", hit.path)
		}
		if strings.Contains(path, "responses") && !strings.HasSuffix(hit.path, "/responses") {
			t.Fatalf("responses upstream %s", hit.path)
		}
	}
	call("/v1/chat/completions", false)
	call("/v1/chat/completions", true)
	call("/responses", false)
	call("/responses", true)
	call("/v1/responses", false)
	call("/v1/responses", true)

	explicitUp := &capturedUp{}
	explicitSrv := httptest.NewServer(http.HandlerFunc(explicitUp.serve))
	t.Cleanup(explicitSrv.Close)
	win := doJSON(t, h, "POST", "/model/new", master, map[string]any{
		"model_name": "explicit-sol",
		"litellm_params": map[string]any{
			"model":                   "gpt-5.6-sol",
			"litellm_credential_name": "fennoai",
			"api_key":                 "sk-explicit",
			"api_base":                explicitSrv.URL,
		},
	})
	if win.Code != 200 {
		t.Fatal(win.Body.String())
	}
	rec := doJSON(t, h, "POST", "/v1/chat/completions", sk, map[string]any{
		"model": "explicit-sol", "messages": []any{map[string]any{"role": "user", "content": "hi"}},
	})
	if rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	if explicitUp.last().auth != "Bearer sk-explicit" {
		t.Fatalf("explicit key lost %q", explicitUp.last().auth)
	}

	unknown := doJSON(t, h, "POST", "/model/new", master, map[string]any{
		"model_name": "no-cred",
		"litellm_params": map[string]any{
			"model":                   "gpt-5.6-sol",
			"custom_llm_provider":     "OpenAI",
			"litellm_credential_name": "does-not-exist",
		},
	})
	if unknown.Code != 200 {
		t.Fatal(unknown.Body.String())
	}
	miss := doJSON(t, h, "POST", "/v1/chat/completions", sk, map[string]any{
		"model": "no-cred", "messages": []any{map[string]any{"role": "user", "content": "hi"}},
	})
	if miss.Code == 200 || strings.Contains(miss.Body.String(), "provider_not_implemented") {
		t.Fatalf("missing key %d %s", miss.Code, miss.Body.String())
	}
	if !strings.Contains(miss.Body.String(), "authentication_error") {
		t.Fatalf("want authentication_error %s", miss.Body.String())
	}

	t.Setenv("XHUB_CRED_KEY", "sk-from-env")
	envCred := doJSON(t, h, "POST", "/credentials", master, map[string]any{
		"credential_name": "envcred",
		"credential_values": map[string]any{
			"api_key":  "os.environ/XHUB_CRED_KEY",
			"api_base": up.URL,
		},
	})
	if envCred.Code != 200 {
		t.Fatal(envCred.Body.String())
	}
	envModel := doJSON(t, h, "POST", "/model/new", master, map[string]any{
		"model_name": "env-sol",
		"litellm_params": map[string]any{
			"model":                   "gpt-5.6-sol",
			"litellm_credential_name": "envcred",
		},
	})
	if envModel.Code != 200 {
		t.Fatal(envModel.Body.String())
	}
	envCall := doJSON(t, h, "POST", "/responses", sk, map[string]any{"model": "env-sol", "input": "hi"})
	if envCall.Code != 200 {
		t.Fatal(envCall.Body.String())
	}
	if upState.last().auth != "Bearer sk-from-env" {
		t.Fatalf("env key %q", upState.last().auth)
	}

	folded := llm.Hydrate(map[string]any{"model": "gpt-5.6-sol"}, map[string]any{"custom_llm_provider": "OpenAI", "api_key": "k"})
	if folded["custom_llm_provider"] != "openai" {
		t.Fatalf("provider fold %v", folded["custom_llm_provider"])
	}
	if folded["api_key"] != "k" {
		t.Fatalf("hydrate key %v", folded["api_key"])
	}
}

func TestPlaygroundSessionUsesCredential(t *testing.T) {
	s, master := testEnv(t)
	h := s.Handler()
	login := doJSON(t, h, "POST", "/login", "", map[string]any{"username": "admin", "password": master})
	if login.Code != 200 {
		t.Fatal(login.Body.String())
	}
	sess, _ := decodeBody(t, login.Body.Bytes())["key"].(string)
	if !strings.HasPrefix(sess, "sess-") {
		t.Fatalf("session key %q", sess)
	}
	upState := &capturedUp{}
	up := httptest.NewServer(http.HandlerFunc(upState.serve))
	t.Cleanup(up.Close)
	created := doJSON(t, h, "POST", "/credentials", sess, map[string]any{
		"credential_name":   "fennoai",
		"credential_info":   map[string]any{"custom_llm_provider": "OpenAI"},
		"credential_values": map[string]any{"api_key": "sk-session-cred", "api_base": up.URL},
	})
	if created.Code != 200 {
		t.Fatal(created.Body.String())
	}
	added := doJSON(t, h, "POST", "/model/new", sess, map[string]any{
		"model_name":     "gpt-5.6-sol",
		"litellm_params": map[string]any{"model": "gpt-5.6-sol", "litellm_credential_name": "fennoai"},
	})
	if added.Code != 200 {
		t.Fatal(added.Body.String())
	}
	chat := doJSON(t, h, "POST", "/v1/chat/completions", sess, map[string]any{
		"model":    "gpt-5.6-sol",
		"messages": []any{map[string]any{"role": "user", "content": "解释量子计算"}},
	})
	if chat.Code != 200 {
		t.Fatalf("playground session %d %s", chat.Code, chat.Body.String())
	}
	if upState.last().auth != "Bearer sk-session-cred" {
		t.Fatalf("upstream auth %q", upState.last().auth)
	}
}

func TestAutoRouterBenchmarksShape(t *testing.T) {
	s, master := testEnv(t)
	h := s.Handler()
	no := doJSON(t, h, "GET", "/auto_router/benchmarks", "", nil)
	if no.Code != 401 {
		t.Fatalf("unauth %d %s", no.Code, no.Body.String())
	}
	bad := doJSON(t, h, "GET", "/auto_router/benchmarks?start_date=13-09-2026", master, nil)
	if bad.Code != 400 || !strings.Contains(bad.Body.String(), "YYYY-MM-DD") {
		t.Fatalf("bad date %d %s", bad.Code, bad.Body.String())
	}
	added := doJSON(t, h, "POST", "/model/new", master, map[string]any{
		"model_name": "cost-router",
		"litellm_params": map[string]any{
			"model": "auto_router/complexity_router",
		},
	})
	if added.Code != 200 {
		t.Fatal(added.Body.String())
	}
	sem := doJSON(t, h, "POST", "/model/new", master, map[string]any{
		"model_name": "semantic-router",
		"litellm_params": map[string]any{
			"model": "auto_router/semantic-embed",
		},
	})
	if sem.Code != 200 {
		t.Fatal(sem.Body.String())
	}
	rec := doJSON(t, h, "GET", "/auto_router/benchmarks", master, nil)
	if rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	groups, ok := body["groups"].([]any)
	if !ok {
		t.Fatalf("groups type %T %s", body["groups"], rec.Body.String())
	}
	scope, _ := body["routers_in_scope"].(float64)
	if int(scope) != len(groups) {
		t.Fatalf("scope %v len %d", scope, len(groups))
	}
	totals, _ := body["totals"].(map[string]any)
	cache, _ := totals["cache"].(map[string]any)
	same, _ := cache["same_model"].(map[string]any)
	if _, ok := same["turns"].(float64); !ok {
		t.Fatalf("turns %T", same["turns"])
	}
	if totals["classifier_cost"] != float64(0) {
		t.Fatalf("classifier_cost %v", totals["classifier_cost"])
	}
	found := false
	for _, g := range groups {
		row, _ := g.(map[string]any)
		if row["router_name"] == "semantic-router" {
			t.Fatalf("semantic router listed %v", row)
		}
		if row["router_name"] == "cost-router" {
			found = true
			if row["router_type"] != "complexity" || row["sessions"] != float64(0) {
				t.Fatalf("idle group %v", row)
			}
		}
	}
	if !found {
		t.Fatalf("cost-router missing %s", rec.Body.String())
	}
}
