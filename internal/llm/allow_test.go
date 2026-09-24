package llm

import (
	"encoding/json"
	"os/exec"
	"reflect"
	"strings"
	"testing"
)

func TestAllowlistAndErrorsMatchPython(t *testing.T) {
	script := `
import ast, json, re
allow_src = open("/Users/sunqirui/Downloads/litellm-main/gateway/routes/allowlist.py", encoding="utf-8").read()
tree = ast.parse(allow_src)
def grab(name):
    for n in tree.body:
        if isinstance(n, ast.AnnAssign) and getattr(n.target, "id", "") == name:
            value = n.value
            if isinstance(value, ast.Call) and getattr(value.func, "id", "") == "frozenset":
                value = value.args[0]
            return ast.literal_eval(value)
    raise SystemExit(name)
ns = {"GATEWAY_PATH_PREFIXES": grab("GATEWAY_PATH_PREFIXES"),
      "GATEWAY_EXACT_PATHS": grab("GATEWAY_EXACT_PATHS"),
      "GATEWAY_MOUNT_PATHS": grab("GATEWAY_MOUNT_PATHS")}
class Mount:
    def __init__(self, path):
        self.path = path
ns["Mount"] = Mount
main = open("/Users/sunqirui/Downloads/litellm-main/gateway/main.py", encoding="utf-8").read()
mtree = ast.parse(main)
fn = next(n for n in mtree.body if isinstance(n, ast.FunctionDef) and n.name == "_is_gateway_route")
exec("from typing import Final\n" + ast.get_source_segment(main, fn), ns)
class Route:
    def __init__(self, path):
        self.path = path
def check(path, mount=False):
    cls = Mount if mount else Route
    return ns["_is_gateway_route"](cls(path))
err_src = open("/Users/sunqirui/Downloads/litellm-main/litellm/litellm_core_utils/exception_mapping_utils.py", encoding="utf-8").read()
start = err_src.find("if original_exception.status_code == 400:")
chunk = err_src[start:start+2200]
pairs = re.findall(r"status_code == (\d+):\s*\n\s*raise (\w+)\(", chunk)
print(json.dumps({
  "prefixes": list(ns["GATEWAY_PATH_PREFIXES"]),
  "exact": sorted(ns["GATEWAY_EXACT_PATHS"]),
  "mounts": sorted(ns["GATEWAY_MOUNT_PATHS"]),
  "keep_health": check("/health/liveliness"),
  "keep_chat": check("/v1/chat/completions"),
  "keep_root": check("/"),
  "drop_key": check("/key/generate"),
  "drop_access": check("/v1/access_group"),
  "keep_metrics_mount": check("/metrics", True),
  "drop_ui_mount": check("/ui", True),
  "empty": check(None) if False else ns["_is_gateway_route"](Route(None)),
  "errors": pairs,
}))
`
	cmd := exec.Command("python3", "-c", script)
	raw, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("python: %v\n%s", err, raw)
	}
	line := strings.TrimSpace(string(raw))
	if i := strings.LastIndex(line, "\n"); i >= 0 {
		line = line[i+1:]
	}
	var py map[string]any
	if err := json.Unmarshal([]byte(line), &py); err != nil {
		t.Fatalf("json %v %s", err, line)
	}
	if !reflect.DeepEqual(PathPrefixes(), asStrings(py["prefixes"])) {
		t.Fatalf("prefixes\ngot %v\npy %v", PathPrefixes(), py["prefixes"])
	}
	cases := []struct {
		name  string
		path  string
		mount bool
		key   string
	}{
		{"health", "/health/liveliness", false, "keep_health"},
		{"chat", "/v1/chat/completions", false, "keep_chat"},
		{"root", "/", false, "keep_root"},
		{"key", "/key/generate", false, "drop_key"},
		{"access", "/v1/access_group", false, "drop_access"},
		{"metrics", "/metrics", true, "keep_metrics_mount"},
		{"ui", "/ui", true, "drop_ui_mount"},
	}
	for _, tc := range cases {
		if Allow(tc.path, tc.mount) != py[tc.key].(bool) {
			t.Fatalf("%s go %v py %v", tc.name, Allow(tc.path, tc.mount), py[tc.key])
		}
	}
	if Allow("", false) {
		t.Fatal("empty path")
	}
	for _, item := range py["errors"].([]any) {
		pair := item.([]any)
		code := 0
		for _, c := range pair[0].(string) {
			code = code*10 + int(c-'0')
		}
		name, ok := ExceptionForStatus(code)
		if !ok || name != pair[1].(string) {
			t.Fatalf("status %d go %s %v py %s", code, name, ok, pair[1])
		}
	}
	if _, ok := ExceptionForStatus(418); ok {
		t.Fatal("unknown status")
	}
}

func asStrings(v any) []string {
	arr := v.([]any)
	out := make([]string, len(arr))
	for i, item := range arr {
		out[i] = item.(string)
	}
	return out
}
