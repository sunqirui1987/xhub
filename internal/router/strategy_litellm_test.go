package router

import (
	"os"
	"strings"
	"testing"
)

// TestStrategyNamesMatchLiteLLM reads litellm/router_strategy from the 1.102.0 tree.
// Every module there must be accepted, including the hyphen form the proxy config uses.
func TestStrategyNamesMatchLiteLLM(t *testing.T) {
	dir := "/Users/sunqirui/Downloads/litellm-main/litellm/router_strategy"
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, "__") || strings.HasPrefix(name, ".") {
			continue
		}
		stem := strings.TrimSuffix(name, ".py")
		n++
		if err := ValidateStrategy(stem); err != nil {
			t.Fatalf("%s: %v", stem, err)
		}
		if err := ValidateStrategy(strings.ReplaceAll(stem, "_", "-")); err != nil {
			t.Fatalf("%s hyphen: %v", stem, err)
		}
	}
	if n != 15 {
		t.Fatalf("strategy modules %d", n)
	}
	if err := ValidateStrategy("not-a-real-strategy"); err == nil {
		t.Fatal("unknown strategy was accepted")
	}
}
