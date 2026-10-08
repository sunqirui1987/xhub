package dataplane

import (
	"testing"

	"github.com/sunqirui1987/xhub/internal/config"
)

func TestDropDisabledExcludesDisabled(t *testing.T) {
	active, removed := dropDisabled([]config.ModelEntry{
		{ModelName: "enabled"},
		{ModelName: "disabled", ModelInfo: map[string]any{"disabled": true}},
	})
	if removed != 1 || len(active) != 1 || active[0].ModelName != "enabled" {
		t.Fatalf("active=%#v removed=%d", active, removed)
	}
}
