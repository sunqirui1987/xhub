package server

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
)

// TestUtilsTokenCounterMatchesLiteLLM hits POST /utils/token_counter and
// GET /utils/supported_openai_params and checks them against LiteLLM.
func TestUtilsTokenCounterMatchesLiteLLM(t *testing.T) {
	py := os.Getenv("LITELLM_PYTHON")
	if py == "" {
		py = "/Users/sunqirui/Downloads/litellm-main/.venv/bin/python"
	}
	outPath := filepath.Join(t.TempDir(), "utils.json")
	cmd := exec.Command(py, "utils_litellm_capture.py", outPath)
	cmd.Env = append(os.Environ(), "PYTHONPATH=/Users/sunqirui/Downloads/litellm-main", "LITELLM_ROOT=/Users/sunqirui/Downloads/litellm-main")
	if msg, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("litellm utils: %v\n%s", err, msg)
	}
	raw, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	var pyOut struct {
		Model           string   `json:"model"`
		PromptTokens    float64  `json:"prompt_tokens"`
		MessageTokens   float64  `json:"message_tokens"`
		TokenizerType   string   `json:"tokenizer_type"`
		SupportedParams []string `json:"supported_openai_params"`
	}
	if err := json.Unmarshal(raw, &pyOut); err != nil {
		t.Fatalf("decode: %v\n%s", err, raw)
	}

	s, master := testEnv(t)
	h := s.Handler()
	missing := doJSON(t, h, "POST", "/utils/token_counter", master, map[string]any{"model": pyOut.Model})
	if missing.Code != 400 {
		t.Fatalf("empty %d %s", missing.Code, missing.Body.String())
	}
	prompt := doJSON(t, h, "POST", "/utils/token_counter", master, map[string]any{"model": pyOut.Model, "prompt": "hello"})
	if prompt.Code != 200 {
		t.Fatal(prompt.Body.String())
	}
	pb := decodeBody(t, prompt.Body.Bytes())
	if pb["total_tokens"] != pyOut.PromptTokens || pb["request_model"] != pyOut.Model || pb["model_used"] != pyOut.Model || pb["tokenizer_type"] != pyOut.TokenizerType {
		t.Fatalf("prompt go %v py tokens %v type %s", pb, pyOut.PromptTokens, pyOut.TokenizerType)
	}
	msgs := doJSON(t, h, "POST", "/utils/token_counter", master, map[string]any{
		"model":    pyOut.Model,
		"messages": []any{map[string]any{"role": "user", "content": "hi"}},
	})
	if msgs.Code != 200 {
		t.Fatal(msgs.Body.String())
	}
	mb := decodeBody(t, msgs.Body.Bytes())
	if mb["total_tokens"] != pyOut.MessageTokens {
		t.Fatalf("messages go %v py %v", mb["total_tokens"], pyOut.MessageTokens)
	}
	params := doJSON(t, h, "GET", "/utils/supported_openai_params?model="+pyOut.Model, master, nil)
	if params.Code != 200 {
		t.Fatal(params.Body.String())
	}
	got, _ := decodeBody(t, params.Body.Bytes())["supported_openai_params"].([]any)
	want := make([]any, len(pyOut.SupportedParams))
	for i, p := range pyOut.SupportedParams {
		want[i] = p
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("params\n go %v\n py %v", got, want)
	}
}
