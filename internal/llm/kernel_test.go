package llm

import (
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
)

func TestHydrateFillsEmptyAndFoldsProvider(t *testing.T) {
	t.Setenv("XHUB_KERNEL_KEY", "sk-from-env")
	got := Hydrate(map[string]any{
		"model":    "gpt-5.6-sol",
		"api_key":  "",
		"api_base": "https://keep.example/v1",
	}, map[string]any{
		"api_key":             "os.environ/XHUB_KERNEL_KEY",
		"api_base":            "https://cred.example/v1",
		"custom_llm_provider": "OpenAI",
		"not_a_param":         "drop-me",
	})
	if got["api_key"] != "sk-from-env" {
		t.Fatalf("key %v", got["api_key"])
	}
	if got["api_base"] != "https://keep.example/v1" {
		t.Fatalf("base overwritten: %v", got["api_base"])
	}
	if got["custom_llm_provider"] != "openai" {
		t.Fatalf("provider %v", got["custom_llm_provider"])
	}
	if _, ok := got["not_a_param"]; ok {
		t.Fatal("unknown credential field leaked")
	}
}

func TestCostMatchesPython(t *testing.T) {
	script := `
import ast, json, logging, sys, types
root = "/Users/sunqirui/Downloads/litellm-main/litellm/cost_calculator.py"
src = open(root, encoding="utf-8").read()
tree = ast.parse(src)
def grab(name):
    node = next(n for n in ast.walk(tree) if isinstance(n, ast.FunctionDef) and n.name == name)
    return ast.get_source_segment(src, node)
ns = {}
exec("from typing import Final\nimport logging\n", ns)
litellm = types.ModuleType("litellm")
litellm.cost_discount_config = {"openai": 0.1}
litellm.cost_margin_config = {
    "openai": {"percentage": 0.2, "fixed_amount": 0.5},
    "global": 0.05,
}
sys.modules["litellm"] = litellm
ns["litellm"] = litellm
class Logger:
    def isEnabledFor(self, level):
        return False
    def debug(self, *args, **kwargs):
        pass
ns["verbose_logger"] = Logger()
ns["logging"] = logging
class ServiceTier:
    class AUTO:
        value = "auto"
ns["ServiceTier"] = ServiceTier
# 流量档映射表和两个函数都在这个文件里，按名字取出来执行。
assign = next(n for n in tree.body if isinstance(n, ast.AnnAssign) and getattr(n.target, "id", "") == "_GEMINI_TRAFFIC_TYPE_TO_SERVICE_TIER")
exec(ast.get_source_segment(src, assign), ns)
for name in ("_apply_cost_discount", "_apply_cost_margin", "_map_traffic_type_to_service_tier", "_normalize_service_tier"):
    exec("from typing import Final\n" + grab(name), ns)
print(json.dumps({
  "discount": ns["_apply_cost_discount"](10, "openai"),
  "discount_none": ns["_apply_cost_discount"](10, None),
  "margin": ns["_apply_cost_margin"](10, "openai"),
  "margin_global": ns["_apply_cost_margin"](10, "other"),
  "traf": ns["_map_traffic_type_to_service_tier"]("ON_DEMAND_PRIORITY"),
  "traf_std": ns["_map_traffic_type_to_service_tier"]("ON_DEMAND"),
  "traf_miss": ns["_map_traffic_type_to_service_tier"]("NOPE"),
  "norm_auto": ns["_normalize_service_tier"]("auto"),
  "norm_flex": ns["_normalize_service_tier"]("Flex"),
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
	final, pct, amt := ApplyDiscount(10, "openai", map[string]float64{"openai": 0.1})
	eq3(t, "discount", py["discount"], final, pct, amt)
	final, pct, amt = ApplyDiscount(10, "", nil)
	eq3(t, "discount_none", py["discount_none"], final, pct, amt)
	mf, mp, mfix, mtot := ApplyMargin(10, "openai", map[string]Margin{
		"openai": {Percent: 0.2, FixedAmount: 0.5, HasPercent: true, HasFixed: true},
		"global": {Percent: 0.05, IsPercent: true},
	})
	eq4(t, "margin", py["margin"], mf, mp, mfix, mtot)
	mf, mp, mfix, mtot = ApplyMargin(10, "other", map[string]Margin{
		"global": {Percent: 0.05, IsPercent: true},
	})
	eq4(t, "margin_global", py["margin_global"], mf, mp, mfix, mtot)
	tier, known, standard := MapTrafficType("ON_DEMAND_PRIORITY")
	if !known || standard || tier != py["traf"] {
		t.Fatalf("traf %v %v %v py %v", tier, known, standard, py["traf"])
	}
	_, known, standard = MapTrafficType("ON_DEMAND")
	if !known || !standard || py["traf_std"] != nil {
		t.Fatalf("std %v %v py %v", known, standard, py["traf_std"])
	}
	_, known, _ = MapTrafficType("NOPE")
	if known || py["traf_miss"] != nil {
		t.Fatalf("miss py %v", py["traf_miss"])
	}
	if _, ok := NormalizeServiceTier("auto", true); ok || py["norm_auto"] != nil {
		t.Fatal("auto")
	}
	got, ok := NormalizeServiceTier("Flex", true)
	if !ok || got != py["norm_flex"] {
		t.Fatalf("flex %v py %v", got, py["norm_flex"])
	}
	inCost, outCost := TokenCost(3, 4, 0.5, 0.25)
	if inCost != 1.5 || outCost != 1 {
		t.Fatalf("token cost %v %v", inCost, outCost)
	}
}

func eq3(t *testing.T, name string, py any, a, b, c float64) {
	t.Helper()
	arr := py.([]any)
	if arr[0].(float64) != a || arr[1].(float64) != b || arr[2].(float64) != c {
		t.Fatalf("%s go %v %v %v py %v", name, a, b, c, arr)
	}
}

func eq4(t *testing.T, name string, py any, a, b, c, d float64) {
	t.Helper()
	arr := py.([]any)
	if arr[0].(float64) != a || arr[1].(float64) != b || arr[2].(float64) != c || arr[3].(float64) != d {
		t.Fatalf("%s go %v %v %v %v py %v", name, a, b, c, d, arr)
	}
}
