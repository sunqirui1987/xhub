package models

import (
	"testing"

	"github.com/sunqirui1987/xhub/internal/config"
)

func TestProxyModelNamesSkipsProviderShells(t *testing.T) {
	names := proxyModelNames([]config.ModelEntry{
		{ModelName: "qiniu", ModelInfo: map[string]any{"role": "provider"}},
		{ModelName: "fennoai", ModelInfo: map[string]any{"role": "provider"}},
		{ModelName: "gpt-5.5", ModelInfo: map[string]any{"role": "model"}},
	})
	if len(names) != 1 || names[0] != "gpt-5.5" {
		t.Fatalf("provider shells leaked into the model list: %v", names)
	}
}

func TestProxyModelNamesSkipsLegacyProviderNamesWithoutRole(t *testing.T) {
	names := proxyModelNames([]config.ModelEntry{
		{ModelName: "qiniu"},
		{ModelName: "fennoai"},
		{ModelName: "GPT-5.5"},
	})
	if len(names) != 1 || names[0] != "GPT-5.5" {
		t.Fatalf("legacy provider names leaked into the model list: %v", names)
	}
}

func TestNonModelEntryKeepsOrdinaryModels(t *testing.T) {
	if nonModelEntry(config.ModelEntry{ModelName: "gpt-5.5"}) {
		t.Fatal("ordinary model was classified as a provider entry")
	}
}
