package llm

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLiteLLMWireMatchesBuild(t *testing.T) {
	py := os.Getenv("LITELLM_PYTHON")
	if py == "" {
		py = "/Users/sunqirui/Downloads/litellm-main/.venv/bin/python"
	}
	script := filepath.Join("litellm_capture.py")
	outPath := filepath.Join(t.TempDir(), "litellm.json")
	cmd := exec.Command(py, script, outPath)
	cmd.Env = append(os.Environ(), "PYTHONPATH=/Users/sunqirui/Downloads/litellm-main", "LITELLM_ROOT=/Users/sunqirui/Downloads/litellm-main")
	if msg, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("litellm capture: %v\n%s", err, msg)
	}
	raw, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Wires map[string]struct {
			Error string            `json:"error"`
			URL   string            `json:"url"`
			Auth  map[string]string `json:"auth"`
			Body  map[string]any    `json:"body"`
		} `json:"wires"`
		Strategies []string `json:"strategies"`
		ModelInfo  struct {
			MaxInputTokens float64 `json:"max_input_tokens"`
			Mode           string  `json:"mode"`
			Error          string  `json:"error"`
		} `json:"model_info"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("decode capture: %v\n%s", err, raw)
	}
	cases := map[string]Request{
		"openai":    {Op: "chat", Provider: "openai", APIBase: "https://relay.example/v1", APIKey: "sk-test", Model: "gpt-4o-mini"},
		"azure":     {Op: "chat", Provider: "azure", APIBase: "https://relay.example/v1", APIKey: "az-key", Model: "dep"},
		"anthropic": {Op: "chat", Provider: "anthropic", APIBase: "https://relay.example", APIKey: "ant-key", Model: "claude-3-5-sonnet-latest"},
		"gemini":    {Op: "chat", Provider: "gemini", APIBase: "https://relay.example", APIKey: "gk", Model: "gemini-2"},
		"vertex":    {Op: "chat", Provider: "vertex_ai", APIBase: "https://relay.example", APIKey: "vk", Model: "gemini-2", VertexProject: "vertex-project", VertexLocation: "us-central1"},
		"zai":       {Op: "chat", Provider: "zai", APIBase: "https://upstream.example/v1", APIKey: "sk-zai", Model: "glm"},
		"deepseek":  {Op: "chat", Provider: "deepseek", APIBase: "https://upstream.example/v1", APIKey: "sk-ds", Model: "deepseek-chat"},
		"cohere":    {Op: "chat", Provider: "cohere", APIBase: "https://upstream.example/v1", APIKey: "ck", Model: "command"},
	}
	body := map[string]any{
		"messages":   []any{map[string]any{"role": "user", "content": "hi"}},
		"max_tokens": 16,
	}
	report := map[string]map[string]any{}
	for name, req := range cases {
		pyw, ok := got.Wires[name]
		if !ok || pyw.URL == "" {
			t.Fatalf("%s: litellm produced no request (%s)", name, pyw.Error)
		}
		req.Body = body
		up, err := Build(context.Background(), req)
		if err != nil {
			t.Fatalf("%s build: %v", name, err)
		}
		if up.URL != pyw.URL {
			t.Fatalf("%s url\n go %s\n py %s", name, up.URL, pyw.URL)
		}
		for hk, hv := range pyw.Auth {
			if !strings.EqualFold(up.Header.Get(hk), hv) {
				t.Fatalf("%s header %s\n go %q\n py %q", name, hk, up.Header.Get(hk), hv)
			}
		}
		var goBody map[string]any
		if err := json.Unmarshal(up.Body, &goBody); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(normJSON(goBody), normJSON(pyw.Body)) {
			gb, _ := json.Marshal(goBody)
			pb, _ := json.Marshal(pyw.Body)
			t.Fatalf("%s body\n go %s\n py %s", name, gb, pb)
		}
		report[name] = map[string]any{"url": up.URL, "auth": pyw.Auth, "body": pyw.Body}
	}
	if img, ok := got.Wires["images"]; ok && img.URL != "" {
		up, err := Build(context.Background(), Request{
			Op: "images", Provider: "openai", APIBase: "https://relay.example/v1", APIKey: "sk-test", Model: "dall-e-3",
			Body: map[string]any{"prompt": "a cat"},
		})
		if err != nil {
			t.Fatal(err)
		}
		if up.URL != img.URL {
			t.Fatalf("images url\n go %s\n py %s", up.URL, img.URL)
		}
		if !strings.EqualFold(up.Header.Get("Authorization"), img.Auth["authorization"]) {
			t.Fatalf("images auth go %q py %q", up.Header.Get("Authorization"), img.Auth["authorization"])
		}
		var goBody map[string]any
		if err := json.Unmarshal(up.Body, &goBody); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(normJSON(goBody), normJSON(img.Body)) {
			gb, _ := json.Marshal(goBody)
			pb, _ := json.Marshal(img.Body)
			t.Fatalf("images body\n go %s\n py %s", gb, pb)
		}
		report["images"] = map[string]any{"url": up.URL, "auth": img.Auth, "body": img.Body}
	}
	extra := []struct {
		name string
		req  Request
	}{
		{"rerank", Request{Op: "rerank", Provider: "cohere", APIBase: "https://upstream.example", APIKey: "ck", Model: "rerank-english-v3.0", Body: map[string]any{"query": "q", "documents": []any{"a", "b"}, "top_n": 2}}},
		{"audio_speech", Request{Op: "audio_speech", Provider: "openai", APIBase: "https://relay.example/v1", APIKey: "sk-test", Model: "tts-1", Body: map[string]any{"input": "hi", "voice": "alloy"}}},
		{"moderations", Request{Op: "moderations", Provider: "openai", APIBase: "https://relay.example/v1", APIKey: "sk-test", Model: "openai/omni-moderation-latest", Body: map[string]any{"input": "hi"}}},
		{"responses", Request{Op: "responses", Provider: "openai", APIBase: "https://relay.example/v1", APIKey: "sk-test", Model: "gpt-4o-mini", Body: map[string]any{"input": "hi"}}},
	}
	for _, c := range extra {
		pyw, ok := got.Wires[c.name]
		if !ok || pyw.URL == "" {
			t.Fatalf("%s: litellm produced no request (%s)", c.name, pyw.Error)
		}
		up, err := Build(context.Background(), c.req)
		if err != nil {
			t.Fatalf("%s build: %v", c.name, err)
		}
		if up.URL != pyw.URL {
			t.Fatalf("%s url\n go %s\n py %s", c.name, up.URL, pyw.URL)
		}
		for hk, hv := range pyw.Auth {
			if !strings.EqualFold(up.Header.Get(hk), hv) {
				t.Fatalf("%s header %s\n go %q\n py %q", c.name, hk, up.Header.Get(hk), hv)
			}
		}
		var goBody map[string]any
		if err := json.Unmarshal(up.Body, &goBody); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(normJSON(goBody), normJSON(pyw.Body)) {
			gb, _ := json.Marshal(goBody)
			pb, _ := json.Marshal(pyw.Body)
			t.Fatalf("%s body\n go %s\n py %s", c.name, gb, pb)
		}
		report[c.name] = map[string]any{"url": up.URL, "auth": pyw.Auth, "body": pyw.Body}
	}
	rt := got.Wires["realtime_client_secret"]
	if rt.URL == "" {
		t.Fatalf("realtime client secret: %s", rt.Error)
	}
	if RealtimeClientSecretsURL("https://relay.example/v1") != rt.URL {
		t.Fatalf("realtime url go %s py %s", RealtimeClientSecretsURL("https://relay.example/v1"), rt.URL)
	}
	if rt.Auth["authorization"] != "Bearer sk-test" {
		t.Fatalf("realtime auth %v", rt.Auth)
	}
	report["realtime_client_secret"] = map[string]any{"url": rt.URL, "auth": rt.Auth}
	if got.ModelInfo.Error != "" {
		t.Fatal(got.ModelInfo.Error)
	}
	costRaw, err := os.ReadFile(filepath.Join("..", "server", "publicdata", "model_cost_map.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cost map[string]map[string]any
	if err := json.Unmarshal(costRaw, &cost); err != nil {
		t.Fatal(err)
	}
	row := cost["gpt-4o-mini"]
	if row["mode"] != any(got.ModelInfo.Mode) {
		t.Fatalf("mode go %v py %s", row["mode"], got.ModelInfo.Mode)
	}
	tokens, _ := row["max_input_tokens"].(float64)
	if tokens != got.ModelInfo.MaxInputTokens {
		t.Fatalf("max_input_tokens go %v py %v", tokens, got.ModelInfo.MaxInputTokens)
	}
	report["model_info"] = map[string]any{"model": "gpt-4o-mini", "max_input_tokens": got.ModelInfo.MaxInputTokens, "mode": got.ModelInfo.Mode}
	report["strategies"] = map[string]any{"names": got.Strategies}
	if path := os.Getenv("LITELLM_COMPARE_OUT"); path != "" {
		enc, _ := json.MarshalIndent(report, "", "  ")
		if err := os.WriteFile(path, append(enc, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func normJSON(v any) any {
	raw, err := json.Marshal(v)
	if err != nil {
		return v
	}
	var out any
	if json.Unmarshal(raw, &out) != nil {
		return v
	}
	return out
}
