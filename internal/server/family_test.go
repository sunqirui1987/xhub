package server

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func mintLLM(t *testing.T, s *Server, master string) string {
	t.Helper()
	gen := doJSON(t, s.Handler(), "POST", "/key/generate", master, map[string]any{"key_type": "llm_api"})
	var g map[string]any
	_ = json.Unmarshal(gen.Body.Bytes(), &g)
	sk, _ := g["key"].(string)
	if !strings.HasPrefix(sk, "sk-") {
		t.Fatalf("key %s", gen.Body.String())
	}
	return sk
}

func decodeBody(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("json %s", raw)
	}
	return m
}

func mustKeys(t *testing.T, m map[string]any, keys ...string) {
	t.Helper()
	for _, k := range keys {
		if _, ok := m[k]; !ok {
			t.Fatalf("missing %s in %v", k, m)
		}
	}
}

func assertNotStub(t *testing.T, raw string) {
	t.Helper()
	if isGenericStub(raw) {
		t.Fatalf("generic stub body %s", raw)
	}
}

func isTypedResource404(rec *httptest.ResponseRecorder) bool {
	if rec == nil || rec.Code != 404 {
		return false
	}
	var m map[string]any
	if json.Unmarshal(rec.Body.Bytes(), &m) != nil {
		return false
	}
	errObj, _ := m["error"].(map[string]any)
	if errObj == nil {
		return false
	}
	if errObj["message"] == nil {
		return false
	}
	return errObj["type"] != nil || errObj["code"] != nil
}

func isGenericStub(raw string) bool {
	var m map[string]any
	if json.Unmarshal([]byte(raw), &m) != nil {
		return false
	}
	_, hasStatus := m["status"]
	_, hasID := m["id"]
	_, hasData := m["data"]
	if hasStatus && hasID && len(m) <= 3 && !hasData {
		st, _ := m["status"].(string)
		return st == "ok"
	}
	if hasStatus && hasData && len(m) <= 3 {
		st, _ := m["status"].(string)
		data, _ := m["data"].([]any)
		return st == "ok" && len(data) == 0
	}
	return false
}

func TestDataPlaneFamilyShapes(t *testing.T) {
	s, master := testEnv(t)
	h := s.Handler()
	sk := mintLLM(t, s, master)

	chat := doJSON(t, h, "POST", "/v1/chat/completions", sk, map[string]any{
		"model": "gpt-4o-mini", "messages": []any{map[string]any{"role": "user", "content": "hi"}},
	})
	if chat.Code != 200 {
		t.Fatal(chat.Body.String())
	}
	cm := decodeBody(t, chat.Body.Bytes())
	mustKeys(t, cm, "id", "object", "choices", "usage", "model")
	if cm["object"] != "chat.completion" {
		t.Fatalf("chat object %v", cm["object"])
	}
	assertNotStub(t, chat.Body.String())

	emb := doJSON(t, h, "POST", "/v1/embeddings", sk, map[string]any{"model": "text-embedding-3-small", "input": "hello"})
	em := decodeBody(t, emb.Body.Bytes())
	mustKeys(t, em, "object", "data", "model")
	assertNotStub(t, emb.Body.String())

	cmp := doJSON(t, h, "POST", "/v1/completions", sk, map[string]any{"model": "gpt-4o-mini", "prompt": "hi"})
	cpm := decodeBody(t, cmp.Body.Bytes())
	mustKeys(t, cpm, "id", "object", "choices", "usage", "model")
	assertNotStub(t, cmp.Body.String())

	msg := doJSON(t, h, "POST", "/v1/messages", sk, map[string]any{
		"model": "gpt-4o-mini", "messages": []any{map[string]any{"role": "user", "content": "hi"}}, "max_tokens": 8,
	})
	mm := decodeBody(t, msg.Body.Bytes())
	mustKeys(t, mm, "id", "type", "role", "content", "model", "stop_reason", "usage")
	assertNotStub(t, msg.Body.String())

	resp := doJSON(t, h, "POST", "/v1/responses", sk, map[string]any{"model": "gpt-4o-mini", "input": "Hello"})
	if resp.Code != 200 {
		t.Fatal(resp.Body.String())
	}
	rm := decodeBody(t, resp.Body.Bytes())
	mustKeys(t, rm, "id", "object", "status", "output", "usage", "model", "created_at")
	if rm["object"] != "response" {
		t.Fatalf("responses object %v", rm["object"])
	}
	assertNotStub(t, resp.Body.String())

	img := doJSON(t, h, "POST", "/v1/images/generations", sk, map[string]any{"model": "dall-e-3", "prompt": "a cat"})
	if img.Code != 200 {
		t.Fatal(img.Body.String())
	}
	im := decodeBody(t, img.Body.Bytes())
	mustKeys(t, im, "created", "data")
	assertNotStub(t, img.Body.String())

	mod := doJSON(t, h, "POST", "/v1/moderations", sk, map[string]any{"model": "omni-moderation-latest", "input": "text"})
	md := decodeBody(t, mod.Body.Bytes())
	mustKeys(t, md, "id", "model", "results")
	assertNotStub(t, mod.Body.String())

	rr := doJSON(t, h, "POST", "/v1/rerank", sk, map[string]any{
		"model": "rerank-english-v3.0", "query": "q", "documents": []any{"a", "b"}, "top_n": 2,
	})
	rrm := decodeBody(t, rr.Body.Bytes())
	mustKeys(t, rrm, "id", "results", "meta")
	assertNotStub(t, rr.Body.String())

	sp := doJSON(t, h, "POST", "/v1/audio/speech", sk, map[string]any{"model": "tts-1", "input": "hi", "voice": "alloy"})
	if sp.Code != 200 {
		t.Fatal(sp.Body.String())
	}
	assertNotStub(t, sp.Body.String())
	if sp.Body.Len() == 0 {
		t.Fatal("empty speech")
	}

	tr := doJSON(t, h, "POST", "/v1/audio/transcriptions", sk, map[string]any{"model": "whisper-1"})
	tm := decodeBody(t, tr.Body.Bytes())
	mustKeys(t, tm, "text")
	assertNotStub(t, tr.Body.String())

	vid := doJSON(t, h, "POST", "/v1/videos", sk, map[string]any{"model": "sora", "prompt": "clip"})
	vm := decodeBody(t, vid.Body.Bytes())
	mustKeys(t, vm, "id", "object", "status", "model", "created_at")
	assertNotStub(t, vid.Body.String())

	fl := doJSON(t, h, "POST", "/v1/files", sk, map[string]any{"purpose": "batch", "filename": "a.jsonl"})
	fm := decodeBody(t, fl.Body.Bytes())
	mustKeys(t, fm, "id", "object", "filename", "purpose", "bytes", "created_at")
	fid, _ := fm["id"].(string)
	got := doJSON(t, h, "GET", "/v1/files/"+fid, sk, nil)
	if got.Code != 200 || !strings.Contains(got.Body.String(), fid) {
		t.Fatalf("file info %s", got.Body.String())
	}
	lst := doJSON(t, h, "GET", "/v1/files", sk, nil)
	if !strings.Contains(lst.Body.String(), fid) {
		t.Fatalf("file list %s", lst.Body.String())
	}
	_ = doJSON(t, h, "DELETE", "/v1/files/"+fid, sk, nil)
	lst2 := doJSON(t, h, "GET", "/v1/files", sk, nil)
	if strings.Contains(lst2.Body.String(), fid) {
		t.Fatalf("file still listed %s", lst2.Body.String())
	}

	bat := doJSON(t, h, "POST", "/v1/batches", sk, map[string]any{"input_file_id": "file_1", "endpoint": "/v1/chat/completions"})
	bm := decodeBody(t, bat.Body.Bytes())
	mustKeys(t, bm, "id", "object", "status", "request_counts", "output_file_id")
	assertNotStub(t, bat.Body.String())

	as := doJSON(t, h, "POST", "/v1/assistants", sk, map[string]any{"model": "gpt-4o-mini", "name": "bot", "instructions": "x"})
	am := decodeBody(t, as.Body.Bytes())
	mustKeys(t, am, "id", "object", "created_at", "model")
	assertNotStub(t, as.Body.String())

	no := doJSON(t, h, "POST", "/v1/images/generations", "", map[string]any{"model": "dall-e-3", "prompt": "a"})
	if no.Code != 401 {
		t.Fatalf("unauth images %d %s", no.Code, no.Body.String())
	}
	mk := doJSON(t, h, "POST", "/v1/images/generations", master, map[string]any{"model": "dall-e-3", "prompt": "a"})
	if mk.Code != 401 {
		t.Fatalf("master images %d %s", mk.Code, mk.Body.String())
	}
}

func TestMgmtFamilyPersist(t *testing.T) {
	s, master := testEnv(t)
	h := s.Handler()

	g := doJSON(t, h, "POST", "/guardrails", master, map[string]any{"guardrail_name": "pii", "litellm_params": map[string]any{"guardrail": "presidio", "mode": "post_call"}})
	if g.Code != 200 {
		t.Fatal(g.Body.String())
	}
	gm := decodeBody(t, g.Body.Bytes())
	mustKeys(t, gm, "guardrail_id", "guardrail_name", "created_at")
	assertNotStub(t, g.Body.String())
	gid, _ := gm["guardrail_id"].(string)
	gl := doJSON(t, h, "GET", "/guardrails/list", master, nil)
	if !strings.Contains(gl.Body.String(), "pii") {
		t.Fatalf("guardrail list %s", gl.Body.String())
	}
	gu := doJSON(t, h, "PATCH", "/guardrails/"+gid, master, map[string]any{"guardrail_name": "pii2"})
	if gu.Code != 200 {
		t.Fatal(gu.Body.String())
	}
	gi := doJSON(t, h, "GET", "/guardrails/"+gid, master, nil)
	if !strings.Contains(gi.Body.String(), "pii2") {
		t.Fatalf("guardrail info %s", gi.Body.String())
	}
	_ = doJSON(t, h, "DELETE", "/guardrails/"+gid, master, nil)
	gl2 := doJSON(t, h, "GET", "/guardrails/list", master, nil)
	if strings.Contains(gl2.Body.String(), gid) {
		t.Fatalf("guardrail still listed %s", gl2.Body.String())
	}

	pr := doJSON(t, h, "POST", "/prompts", master, map[string]any{"prompt_id": "p1"})
	if pr.Code != 200 {
		t.Fatal(pr.Body.String())
	}
	prm := decodeBody(t, pr.Body.Bytes())
	mustKeys(t, prm, "prompt_id", "version", "created_at")
	assertNotStub(t, pr.Body.String())
	pl := doJSON(t, h, "GET", "/prompts/list", master, nil)
	if !strings.Contains(pl.Body.String(), "p1") {
		t.Fatalf("prompt list %s", pl.Body.String())
	}

	pj := doJSON(t, h, "POST", "/project/new", master, map[string]any{"project_alias": "checkout", "max_budget": 20.0})
	pm := decodeBody(t, pj.Body.Bytes())
	mustKeys(t, pm, "project_id", "project_alias", "blocked", "created_at")
	if pm["blocked"] != false {
		t.Fatalf("project blocked default %v", pm["blocked"])
	}
	if str(pm["created_at"]) == "" {
		t.Fatal("project created_at empty")
	}
	pid, _ := pm["project_id"].(string)
	pu := doJSON(t, h, "POST", "/project/update", master, map[string]any{"project_id": pid, "project_alias": "pay", "max_budget": 9.0})
	if pu.Code != 200 {
		t.Fatal(pu.Body.String())
	}
	pi := doJSON(t, h, "GET", "/project/info?project_id="+pid, master, nil)
	if !strings.Contains(pi.Body.String(), "pay") || !strings.Contains(pi.Body.String(), "9") {
		t.Fatalf("project info %s", pi.Body.String())
	}
	pim := decodeBody(t, pi.Body.Bytes())
	mustKeys(t, pim, "blocked", "created_at")
	_ = doJSON(t, h, "POST", "/project/delete", master, map[string]any{"project_id": pid})
	plst := doJSON(t, h, "GET", "/project/list", master, nil)
	if strings.Contains(plst.Body.String(), pid) {
		t.Fatalf("project still listed %s", plst.Body.String())
	}

	ag := doJSON(t, h, "POST", "/v1/agents", master, map[string]any{"agent_name": "ops", "model": "gpt-4o-mini", "litellm_params": map[string]any{}})
	am := decodeBody(t, ag.Body.Bytes())
	mustKeys(t, am, "agent_id", "agent_name", "created_at")
	assertNotStub(t, ag.Body.String())
}

func TestFrozenFamilyLLMKey(t *testing.T) {
	s, master := testEnv(t)
	h := s.Handler()
	sk := mintLLM(t, s, master)

	no := doJSON(t, h, "POST", "/v1/agents", "", map[string]any{"agent_name": "x"})
	if no.Code != 401 {
		t.Fatalf("agents no key %d %s", no.Code, no.Body.String())
	}
	ag := doJSON(t, h, "POST", "/v1/agents", sk, map[string]any{
		"agent_name": "researcher", "litellm_params": map[string]any{"model": "gpt-4o-mini"},
	})
	if ag.Code != 200 {
		t.Fatalf("agents llm_api %d %s", ag.Code, ag.Body.String())
	}
	mustKeys(t, decodeBody(t, ag.Body.Bytes()), "agent_id", "agent_name", "litellm_params", "created_at")
	assertNotStub(t, ag.Body.String())

	ix := doJSON(t, h, "POST", "/v1beta/interactions", sk, map[string]any{"model": "gemini-2.5-flash", "input": "hi"})
	if ix.Code != 200 {
		t.Fatalf("interactions %d %s", ix.Code, ix.Body.String())
	}
	mustKeys(t, decodeBody(t, ix.Body.Bytes()), "id", "status", "output")
	ix2 := doJSON(t, h, "POST", "/interactions", sk, map[string]any{"model": "gemini-2.5-flash", "input": "hi"})
	if ix2.Code != 200 {
		t.Fatalf("interactions alias %d %s", ix2.Code, ix2.Body.String())
	}
	mustKeys(t, decodeBody(t, ix2.Body.Bytes()), "id", "status", "output")

	se := doJSON(t, h, "POST", "/v1/search", sk, map[string]any{"query": "what is xhub", "max_results": 5})
	if se.Code != 200 {
		t.Fatalf("search %d %s", se.Code, se.Body.String())
	}
	mustKeys(t, decodeBody(t, se.Body.Bytes()), "results", "usage")

	vs := doJSON(t, h, "POST", "/v1/vector_stores", sk, map[string]any{"name": "docs"})
	if vs.Code != 200 {
		t.Fatalf("vector_stores %d %s", vs.Code, vs.Body.String())
	}
	mustKeys(t, decodeBody(t, vs.Body.Bytes()), "id", "object", "name", "status", "file_counts", "created_at")

	wf := doJSON(t, h, "POST", "/v1/workflows/runs", sk, map[string]any{"content": "continue"})
	if wf.Code != 200 {
		t.Fatalf("workflows %d %s", wf.Code, wf.Body.String())
	}
	mustKeys(t, decodeBody(t, wf.Body.Bytes()), "run_id", "status", "events", "messages")
}
