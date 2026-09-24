package server

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sunqirui1987/xhub/internal/store"
)

// TestKeyCreateListMatchesLiteLLM drives POST /key/generate and GET /key/list
// on the shipped engine and checks the consumed fields against LiteLLM's
// generate_key_helper_fn for the same alias, key type, and budget.
func TestKeyCreateListMatchesLiteLLM(t *testing.T) {
	py := os.Getenv("LITELLM_PYTHON")
	if py == "" {
		py = "/Users/sunqirui/Downloads/litellm-main/.venv/bin/python"
	}
	outPath := filepath.Join(t.TempDir(), "key.json")
	cmd := exec.Command(py, "key_litellm_capture.py", outPath)
	cmd.Env = append(os.Environ(), "PYTHONPATH=/Users/sunqirui/Downloads/litellm-main", "LITELLM_ROOT=/Users/sunqirui/Downloads/litellm-main")
	if msg, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("litellm key helper: %v\n%s", err, msg)
	}
	raw, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	var pyOut struct {
		Create struct {
			Method string `json:"method"`
			Path   string `json:"path"`
			Status int    `json:"status"`
			Fields struct {
				KeyAlias  string  `json:"key_alias"`
				KeyType   string  `json:"key_type"`
				MaxBudget float64 `json:"max_budget"`
				Spend     float64 `json:"spend"`
				Models    []any   `json:"models"`
				Blocked   any     `json:"blocked"`
				HasKey    bool    `json:"has_key"`
				KeyPrefix string  `json:"key_prefix"`
			} `json:"fields"`
		} `json:"create"`
		List struct {
			Method      string `json:"method"`
			Path        string `json:"path"`
			Status      int    `json:"status"`
			TotalCount  int    `json:"total_count"`
			CurrentPage int    `json:"current_page"`
			TotalPages  int    `json:"total_pages"`
			KeyAlias    string `json:"key_alias"`
		} `json:"list"`
		Stored struct {
			KeyAlias  string  `json:"key_alias"`
			KeyType   string  `json:"key_type"`
			MaxBudget float64 `json:"max_budget"`
			Spend     float64 `json:"spend"`
			Blocked   any     `json:"blocked"`
		} `json:"stored"`
	}
	if err := json.Unmarshal(raw, &pyOut); err != nil {
		t.Fatalf("decode: %v\n%s", err, raw)
	}
	if pyOut.Create.Method != "POST" || pyOut.Create.Path != "/key/generate" || pyOut.Create.Status != 200 {
		t.Fatalf("litellm create route %#v", pyOut.Create)
	}
	if pyOut.List.Method != "GET" || pyOut.List.Path != "/key/list" || pyOut.List.Status != 200 {
		t.Fatalf("litellm list route %#v", pyOut.List)
	}

	s, master := testEnv(t)
	created := doJSON(t, s.Handler(), pyOut.Create.Method, pyOut.Create.Path, master, map[string]any{
		"key_alias":  pyOut.Create.Fields.KeyAlias,
		"key_type":   pyOut.Create.Fields.KeyType,
		"max_budget": pyOut.Create.Fields.MaxBudget,
	})
	if created.Code != pyOut.Create.Status {
		t.Fatalf("create %d %s", created.Code, created.Body.String())
	}
	body := decodeBody(t, created.Body.Bytes())
	if body["key_alias"] != pyOut.Create.Fields.KeyAlias {
		t.Fatalf("alias go %v py %s", body["key_alias"], pyOut.Create.Fields.KeyAlias)
	}
	if body["key_type"] != pyOut.Create.Fields.KeyType {
		t.Fatalf("key_type go %v py %s", body["key_type"], pyOut.Create.Fields.KeyType)
	}
	if n, _ := body["max_budget"].(float64); n != pyOut.Create.Fields.MaxBudget {
		t.Fatalf("max_budget go %v py %v", body["max_budget"], pyOut.Create.Fields.MaxBudget)
	}
	if n, _ := body["spend"].(float64); n != pyOut.Create.Fields.Spend {
		t.Fatalf("spend go %v py %v", body["spend"], pyOut.Create.Fields.Spend)
	}
	key, _ := body["key"].(string)
	if pyOut.Create.Fields.HasKey && !strings.HasPrefix(key, pyOut.Create.Fields.KeyPrefix) {
		t.Fatalf("key prefix %q", key)
	}
	if pyOut.Create.Fields.Blocked == nil {
		if body["blocked"] != nil {
			t.Fatalf("blocked go %v py null", body["blocked"])
		}
	} else if body["blocked"] != pyOut.Create.Fields.Blocked {
		t.Fatalf("blocked go %v py %v", body["blocked"], pyOut.Create.Fields.Blocked)
	}
	row, err := s.Store.GetByHash(store.HashKey(key))
	if err != nil {
		t.Fatal(err)
	}
	if row.KeyAlias != pyOut.Stored.KeyAlias || row.KeyType != pyOut.Stored.KeyType {
		t.Fatalf("stored identity go %s/%s py %s/%s", row.KeyAlias, row.KeyType, pyOut.Stored.KeyAlias, pyOut.Stored.KeyType)
	}
	if !row.MaxBudget.Valid || row.MaxBudget.Float64 != pyOut.Stored.MaxBudget {
		t.Fatalf("stored max_budget go %v py %v", row.MaxBudget, pyOut.Stored.MaxBudget)
	}
	if row.Spend != pyOut.Stored.Spend {
		t.Fatalf("stored spend go %v py %v", row.Spend, pyOut.Stored.Spend)
	}
	if pyOut.Stored.Blocked == nil {
		if row.Blocked.Valid {
			t.Fatalf("stored blocked go %v py null", row.Blocked.Bool)
		}
	} else if want, ok := pyOut.Stored.Blocked.(bool); !ok || !row.Blocked.Valid || row.Blocked.Bool != want {
		t.Fatalf("stored blocked go %v py %v", row.Blocked, pyOut.Stored.Blocked)
	}

	listed := doJSON(t, s.Handler(), pyOut.List.Method, pyOut.List.Path+"?return_full_object=true", master, nil)
	if listed.Code != pyOut.List.Status {
		t.Fatalf("list %d %s", listed.Code, listed.Body.String())
	}
	page := decodeBody(t, listed.Body.Bytes())
	if page["current_page"] != float64(pyOut.List.CurrentPage) {
		t.Fatalf("page %v", page["current_page"])
	}
	if page["total_count"] == float64(0) {
		t.Fatalf("empty list %s", listed.Body.String())
	}
	if !strings.Contains(listed.Body.String(), pyOut.List.KeyAlias) {
		t.Fatalf("list missing alias %s", listed.Body.String())
	}
	if path := os.Getenv("LITELLM_KEY_COMPARE_OUT"); path != "" {
		side := map[string]any{
			"litellm":           pyOut,
			"go_create":         body,
			"go_stored_blocked": nullBoolJSON(row.Blocked),
			"py_stored":         pyOut.Stored,
			"go_list_status":    listed.Code,
			"go_list_has_alias": strings.Contains(listed.Body.String(), pyOut.List.KeyAlias),
		}
		enc, _ := json.MarshalIndent(side, "", "  ")
		if err := os.WriteFile(path, append(enc, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
