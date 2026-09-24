package server

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/sunqirui1987/xhub/internal/llm"
)

// TestPassthroughURLMatchesLiteLLM hits the shipped /openai_passthrough route
// and checks its upstream_url against LiteLLM _join_url_paths. Subpath cases
// call PassthroughSubpath, which is construct_target_url_with_subpath.
func TestPassthroughURLMatchesLiteLLM(t *testing.T) {
	py := os.Getenv("LITELLM_PYTHON")
	if py == "" {
		py = "/Users/sunqirui/Downloads/litellm-main/.venv/bin/python"
	}
	outPath := filepath.Join(t.TempDir(), "pass.json")
	cmd := exec.Command(py, "passthrough_litellm_capture.py", outPath)
	cmd.Env = append(os.Environ(), "PYTHONPATH=/Users/sunqirui/Downloads/litellm-main", "LITELLM_ROOT=/Users/sunqirui/Downloads/litellm-main")
	if msg, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("litellm passthrough: %v\n%s", err, msg)
	}
	raw, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	var pyOut struct {
		URLs     map[string]string `json:"urls"`
		Subpaths map[string]string `json:"subpaths"`
	}
	if err := json.Unmarshal(raw, &pyOut); err != nil {
		t.Fatalf("decode: %v\n%s", err, raw)
	}

	s, master := testEnv(t)
	sk := mintLLM(t, s, master)
	rec := doJSON(t, s.Handler(), "WEBSOCKET", "/openai_passthrough/responses", sk, map[string]any{"input": "hi"})
	if rec.Code != 200 {
		t.Fatalf("passthrough %d %s", rec.Code, rec.Body.String())
	}
	body := decodeBody(t, rec.Body.Bytes())
	if body["upstream_url"] != pyOut.URLs["openai_responses"] {
		t.Fatalf("openai passthrough url go %v py %s", body["upstream_url"], pyOut.URLs["openai_responses"])
	}
	if body["object"] != "passthrough" || body["provider"] != "openai_passthrough" {
		t.Fatalf("passthrough body %s", rec.Body.String())
	}
	gem := llm.PassthroughURL("https://generativelanguage.googleapis.com", "/v1beta/models/gemini-2:generateContent", "gemini")
	if gem != pyOut.URLs["gemini_generate"] {
		t.Fatalf("gemini passthrough url go %s py %s", gem, pyOut.URLs["gemini_generate"])
	}

	cases := []struct {
		name    string
		base    string
		sub     string
		include bool
	}{
		{"with_sub", "https://upstream.example/v1", "responses", true},
		{"empty_sub", "https://upstream.example/v1", "", true},
		{"exclude_sub", "https://upstream.example/v1", "responses", false},
		{"dotdot", "https://upstream.example/v1/", "../secret", true},
	}
	for _, c := range cases {
		got := llm.PassthroughSubpath(c.base, c.sub, c.include)
		if got != pyOut.Subpaths[c.name] {
			t.Fatalf("%s subpath go %s py %s", c.name, got, pyOut.Subpaths[c.name])
		}
	}
}
