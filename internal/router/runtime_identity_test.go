package router

import (
	"testing"

	"github.com/sunqirui1987/xhub/internal/config"
)

func databaseDeployment(id string, weight float64) config.ModelEntry {
	return config.ModelEntry{
		ModelName: "shared-name",
		LiteLLMParams: map[string]any{
			"api_base":                "https://same.example/v1",
			"model":                   "upstream-model",
			"litellm_credential_name": "shared-credential",
			"weight":                  weight,
		},
		ModelInfo: map[string]any{"id": id},
	}
}

func TestDatabaseRuntimeIdentityKeepsRetryAndSplitRowsDistinct(t *testing.T) {
	a := databaseDeployment("model-a", 7)
	b := databaseDeployment("model-b", 3)
	if CooldownID(a) == CooldownID(b) {
		t.Fatal("database model_info.id did not distinguish identical deployments")
	}

	ordered := Order([]config.ModelEntry{a, b}, "shared-name", "weighted-split", State{Splits: NewSplitState()})
	if len(ordered) != 2 {
		t.Fatalf("healthy retry pool lost an identical deployment: %#v", ordered)
	}

	counts := map[string]int{}
	state := State{Splits: NewSplitState()}
	for i := 0; i < 10; i++ {
		picked := Pick([]config.ModelEntry{a, b}, "shared-name", "weighted-split", state)
		if picked == nil {
			t.Fatalf("weighted draw %d returned no deployment", i)
		}
		counts[CooldownID(*picked)]++
	}
	if counts[CooldownID(a)] != 7 || counts[CooldownID(b)] != 3 {
		t.Fatalf("weighted split = %#v, want exact 7/3", counts)
	}
}

func TestDatabaseRuntimeIdentityIsolatesCooldown(t *testing.T) {
	a := databaseDeployment("model-a", 7)
	b := databaseDeployment("model-b", 3)
	state := State{
		Cooldown: map[string]bool{CooldownID(a): true},
		Splits:   NewSplitState(),
	}

	for _, strategy := range []string{"weighted-split", "simple-shuffle"} {
		picked := Pick([]config.ModelEntry{a, b}, "shared-name", strategy, state)
		if picked == nil || CooldownID(*picked) != CooldownID(b) {
			t.Fatalf("%s did not isolate database-row cooldown: %#v", strategy, picked)
		}
	}
	ordered := Order([]config.ModelEntry{a, b}, "shared-name", "simple-shuffle", state)
	if len(ordered) != 1 || CooldownID(ordered[0]) != CooldownID(b) {
		t.Fatalf("cooldown retry pool = %#v, want only healthy database row", ordered)
	}
}

func TestDatabaseRuntimeIdentityIsolatesMetrics(t *testing.T) {
	a := databaseDeployment("model-a", 1)
	b := databaseDeployment("model-b", 1)
	tests := []struct {
		name     string
		strategy string
		state    State
	}{
		{"busy", "least-busy", State{Busy: map[string]int{CooldownID(a): 8, CooldownID(b): 1}}},
		{"latency", "lowest-latency", State{Latency: map[string]float64{CooldownID(a): 800, CooldownID(b): 10}}},
		{"usage", "lowest-tpm-rpm", State{Usage: map[string]float64{CooldownID(a): 800, CooldownID(b): 10}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			picked := Pick([]config.ModelEntry{a, b}, "shared-name", tc.strategy, tc.state)
			if picked == nil || CooldownID(*picked) != CooldownID(b) {
				t.Fatalf("%s metric did not select the lower-state database row: %#v", tc.strategy, picked)
			}
		})
	}
}

func TestCooldownIDPreservesParameterIdentityPrecedence(t *testing.T) {
	entry := databaseDeployment("model-info-id", 1)
	entry.LiteLLMParams["deployment_id"] = "params-deployment"
	entry.LiteLLMParams["pricing_id"] = "params-pricing"

	const prefix = "https://same.example/v1|upstream-model|pricing:"
	const credential = "|credential:shared-credential"
	if got, want := CooldownID(entry), prefix+"params-pricing"+credential; got != want {
		t.Fatalf("pricing_id precedence changed: got %q want %q", got, want)
	}
	delete(entry.LiteLLMParams, "pricing_id")
	if got, want := CooldownID(entry), prefix+"params-deployment"+credential; got != want {
		t.Fatalf("deployment_id precedence changed: got %q want %q", got, want)
	}
	delete(entry.LiteLLMParams, "deployment_id")
	if got, want := CooldownID(entry), prefix+"model-info-id"+credential; got != want {
		t.Fatalf("model_info.id fallback: got %q want %q", got, want)
	}
	delete(entry.ModelInfo, "id")
	if got, want := CooldownID(entry), "https://same.example/v1|upstream-model"+credential; got != want {
		t.Fatalf("missing stable id: got %q want %q", got, want)
	}
}
