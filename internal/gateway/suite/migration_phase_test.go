package suite

import (
	"encoding/json"
	"github.com/sunqirui1987/xhub/internal/catalog"
	"github.com/sunqirui1987/xhub/internal/gateway"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sunqirui1987/xhub/internal/llm"
)

func TestMigrationKeyRoundTrip(t *testing.T) {
	s, master := testEnv(t)
	created := doJSON(t, s.Handler(), "POST", "/key/generate", master, map[string]any{
		"key_alias": "phase0", "key_type": "llm_api",
	})
	if created.Code != 200 {
		t.Fatal(created.Body.String())
	}
	body := decodeBody(t, created.Body.Bytes())
	if str(body["key"]) == "" {
		t.Fatalf("key missing %s", created.Body.String())
	}
	listed := doJSON(t, s.Handler(), "GET", "/key/list", master, nil)
	if listed.Code != 200 || !strings.Contains(listed.Body.String(), "phase0") {
		t.Fatalf("list %d %s", listed.Code, listed.Body.String())
	}
}

func TestMigrationOpenAIAndGeminiBuilders(t *testing.T) {
	chat, err := llm.Build(t.Context(), llm.Request{
		Op: "chat", Provider: "openai", APIBase: "https://relay.example/v1", APIKey: "sk-test", Model: "gpt-5.6-sol",
		Body: map[string]any{"messages": []any{map[string]any{"role": "user", "content": "hi"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(chat.URL, "/chat/completions") {
		t.Fatalf("openai url %s", chat.URL)
	}
	if chat.Header.Get("Authorization") != "Bearer sk-test" {
		t.Fatalf("openai auth %q", chat.Header.Get("Authorization"))
	}
	var doc map[string]any
	if err := json.Unmarshal(chat.Body, &doc); err != nil {
		t.Fatal(err)
	}
	if doc["model"] != "gpt-5.6-sol" {
		t.Fatalf("openai model %v", doc["model"])
	}

	gem, err := llm.Build(t.Context(), llm.Request{
		Op: "chat", Provider: "gemini", APIBase: "https://relay.example", APIKey: "gk", Model: "gemini-2",
		Body: map[string]any{"messages": []any{map[string]any{"role": "user", "content": "hello"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(gem.URL, ":generateContent") {
		t.Fatalf("gemini url %s", gem.URL)
	}
	if gem.Header.Get("X-Goog-Api-Key") != "gk" {
		t.Fatalf("gemini auth %#v", gem.Header)
	}
	var g map[string]any
	if err := json.Unmarshal(gem.Body, &g); err != nil {
		t.Fatal(err)
	}
	if _, ok := g["contents"]; !ok {
		t.Fatalf("gemini body %s", gem.Body)
	}
	if _, ok := g["messages"]; ok {
		t.Fatalf("openai messages leaked %s", gem.Body)
	}
}

func TestMigrationImagesRouteRegistered(t *testing.T) {
	s, master := testEnv(t)
	found := false
	for _, rt := range s.GinRoutes() {
		if rt.Method == http.MethodPost && rt.Path == "/v1/images/generations" {
			found = true
		}
		if rt.Path == "/*path" || rt.Path == "/*filepath" {
			t.Fatalf("catch-all route %s", rt.Path)
		}
	}
	if !found {
		t.Fatal("POST /v1/images/generations is not a registered gin route")
	}
	sk := mintLLM(t, s, master)
	rec := doJSON(t, s.Handler(), "POST", "/v1/images/generations", sk, map[string]any{
		"model": "dall-e-3", "prompt": "a cat",
	})
	if rec.Code != 200 {
		t.Fatalf("images %d %s", rec.Code, rec.Body.String())
	}
}

func TestMigrationFileRowPersists(t *testing.T) {
	s, master := testEnv(t)
	sk := mintLLM(t, s, master)
	rec := doJSON(t, s.Handler(), "POST", "/v1/files", sk, map[string]any{
		"purpose": "assistants", "filename": "notes.txt",
	})
	if rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	body := decodeBody(t, rec.Body.Bytes())
	id := str(body["id"])
	if id == "" {
		t.Fatalf("file id missing %s", rec.Body.String())
	}
	row, err := s.Store.GetKV("files", id)
	if err != nil {
		t.Fatal(err)
	}
	if str(row["filename"]) != "notes.txt" || str(row["purpose"]) != "assistants" {
		t.Fatalf("stored row %#v", row)
	}
	if str(row["object"]) != "file" {
		t.Fatalf("object %#v", row["object"])
	}
}

func TestMigrationProviderCodecs(t *testing.T) {
	base := "https://upstream.example/v1"
	body := map[string]any{"messages": []any{map[string]any{"role": "user", "content": "hi"}}}
	zai, err := llm.Build(t.Context(), llm.Request{Op: "chat", Provider: "zai", APIBase: base, APIKey: "sk-zai", Model: "glm", Body: body})
	if err != nil {
		t.Fatal(err)
	}
	if zai.URL != base+"/chat/completions" {
		t.Fatalf("zai url %s", zai.URL)
	}
	if zai.Header.Get("Authorization") != "Bearer sk-zai" {
		t.Fatalf("zai auth %q", zai.Header.Get("Authorization"))
	}

	az, err := llm.Build(t.Context(), llm.Request{Op: "chat", Provider: "azure", APIBase: base, APIKey: "az-key", Model: "dep", Body: body})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(az.URL, "/openai/deployments/dep/chat/completions") {
		t.Fatalf("azure url %s", az.URL)
	}
	if az.Header.Get("api-key") != "az-key" {
		t.Fatalf("azure auth %#v", az.Header)
	}

	co, err := llm.Build(t.Context(), llm.Request{Op: "chat", Provider: "cohere", APIBase: base, APIKey: "ck", Model: "command", Body: body})
	if err != nil {
		t.Fatal(err)
	}
	// LiteLLM 在给了 api_base 时把它当作完整 URL，正文仍是 messages，而不是 /chat/completions。
	if co.URL != base || strings.HasSuffix(co.URL, "/chat/completions") {
		t.Fatalf("cohere url %s", co.URL)
	}
	var cbody map[string]any
	if err := json.Unmarshal(co.Body, &cbody); err != nil {
		t.Fatal(err)
	}
	if _, ok := cbody["messages"]; !ok {
		t.Fatalf("cohere body %s", co.Body)
	}
	if co.Header.Get("Authorization") != "Bearer ck" {
		t.Fatalf("cohere auth %q", co.Header.Get("Authorization"))
	}
}

func TestMigrationCatalogRoutesAndUnknown404(t *testing.T) {
	s, master := testEnv(t)
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "docs", "_inventory", "catalog.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cat struct {
		HTTPRoutes []struct {
			Method string `json:"method"`
			Path   string `json:"path"`
		} `json:"http_routes"`
	}
	if err := json.Unmarshal(raw, &cat); err != nil {
		t.Fatal(err)
	}
	if len(cat.HTTPRoutes) != 779 {
		t.Fatalf("routes %d", len(cat.HTTPRoutes))
	}
	sk := mintLLM(t, s, master)
	for _, rt := range cat.HTTPRoutes {
		path := fillPath(rt.Path)
		method := rt.Method
		tok := master
		if catalog.IsPublicPath(method, path) {
			tok = ""
		} else if catalog.IsDataPlanePath(path) {
			tok = sk
		}
		rec := doJSON(t, s.Handler(), method, path, tok, catalogBody(path))
		if gateway.IsRemovedColumn(path) {
			if rec.Code != 404 {
				t.Fatalf("%s %s removed column want 404 got %d", method, path, rec.Code)
			}
			continue
		}
		if rec.Code == 404 && isTypedResource404(rec) {
			continue
		}
		if rec.Code == 404 {
			t.Fatalf("%s %s -> 404 %s", method, path, truncate(rec.Body.String(), 160))
		}
	}
	unknown := doJSON(t, s.Handler(), "GET", "/not-a-litellm-route", master, nil)
	if unknown.Code != 404 {
		t.Fatalf("unknown path %d %s", unknown.Code, unknown.Body.String())
	}
}

func TestMigrationReadiness(t *testing.T) {
	s, _ := testEnv(t)
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	for i := 0; i < 2; i++ {
		res, err := http.Get(srv.URL + "/health/readiness")
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if res.StatusCode != 200 || !strings.Contains(string(b), "ready") {
			t.Fatalf("readiness %d %s", res.StatusCode, b)
		}
	}
}
