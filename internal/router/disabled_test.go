package router

import (
	"testing"

	"github.com/sunqirui1987/xhub/internal/config"
)

func TestAllExcludesDisabledExactAndWildcardDeployments(t *testing.T) {
	list := []config.ModelEntry{
		{ModelName: "exact", ModelInfo: map[string]any{"id": "disabled-exact", "disabled": true}},
		{ModelName: "exact", ModelInfo: map[string]any{"id": "enabled-exact"}},
		{ModelName: "wild/*", LiteLLMParams: map[string]any{"model": "upstream/*"}, ModelInfo: map[string]any{"id": "disabled-wild", "disabled": true}},
		{ModelName: "wild/*", LiteLLMParams: map[string]any{"model": "upstream/*"}, ModelInfo: map[string]any{"id": "enabled-wild"}},
	}
	exact := All(list, "exact")
	if len(exact) != 1 || exact[0].ModelInfo["id"] != "enabled-exact" {
		t.Fatalf("exact=%#v", exact)
	}
	wild := All(list, "wild/model")
	if len(wild) != 1 || wild[0].ModelInfo["id"] != "enabled-wild" || wild[0].LiteLLMParams["model"] != "upstream/model" {
		t.Fatalf("wild=%#v", wild)
	}
}
