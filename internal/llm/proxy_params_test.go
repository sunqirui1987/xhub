package llm

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
)

// TestResponsesBodyDropsLiteLLMTraceID compares the Responses upstream body with
// litellm.utils.filter_out_litellm_params on one body that sets every proxy field.
func TestResponsesBodyDropsLiteLLMTraceID(t *testing.T) {
	py := os.Getenv("LITELLM_PYTHON")
	if py == "" {
		py = "/Users/sunqirui/Downloads/litellm-main/.venv/bin/python"
	}
	outPath := filepath.Join(t.TempDir(), "filtered.json")
	script := `import json, sys
from litellm.types.utils import all_litellm_params
from litellm.utils import filter_out_litellm_params
body = {name: "x" for name in set(all_litellm_params)}
body.update({"model": "gpt-4o-mini", "input": "hi", "stream": False, "temperature": 0})
json.dump({"input": body, "filtered": filter_out_litellm_params(body)}, open(sys.argv[1], "w"))
`
	scriptPath := filepath.Join(t.TempDir(), "filter.py")
	if err := os.WriteFile(scriptPath, []byte(script), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(py, scriptPath, outPath)
	cmd.Env = append(os.Environ(), "PYTHONPATH=/Users/sunqirui/Downloads/litellm-main", "LITELLM_LOCAL_MODEL_COST_MAP=True")
	if msg, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("litellm filter: %v\n%s", err, msg)
	}
	raw, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	var captured struct {
		Input    map[string]any `json:"input"`
		Filtered map[string]any `json:"filtered"`
	}
	if err := json.Unmarshal(raw, &captured); err != nil {
		t.Fatal(err)
	}
	if len(captured.Input) < 200 {
		t.Fatalf("expected the full proxy param set, got %d keys", len(captured.Input))
	}
	up, err := Build(context.Background(), Request{
		Op: "responses", Provider: "openai", APIBase: "https://relay.example/v1", APIKey: "sk-test",
		Model: "gpt-4o-mini", Body: captured.Input,
	})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(up.Body, &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(normJSON(got), normJSON(captured.Filtered)) {
		gb, _ := json.Marshal(got)
		pb, _ := json.Marshal(captured.Filtered)
		t.Fatalf("body\n go %s\n py %s", gb, pb)
	}
	if path := os.Getenv("LITELLM_PROXY_COMPARE_OUT"); path != "" {
		side := map[string]any{
			"input_keys": len(captured.Input),
			"litellm":    captured.Filtered,
			"go":         got,
		}
		enc, _ := json.MarshalIndent(side, "", "  ")
		if err := os.WriteFile(path, append(enc, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
