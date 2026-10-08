package router

import (
	"github.com/sunqirui1987/xhub/internal/config"
	"math"
	"testing"
)

func TestSplitExclusionsApplyToEveryAttempt(t *testing.T) {
	for _, splits := range []*SplitState{nil, NewSplitState()} {
		st := State{Splits: splits}
		for _, weight := range []float64{0, -1, math.NaN(), math.Inf(1)} {
			disabled := deployment("shared-name", "disabled", weight)
			if got := Pick([]config.ModelEntry{disabled}, "shared-name", "weighted-split", st); got != nil {
				t.Fatalf("singleton weight %v selected", weight)
			}
			if got := Order([]config.ModelEntry{disabled}, "shared-name", "weighted-split", st); len(got) != 0 {
				t.Fatalf("disabled candidate resurrected: %v", got)
			}
			pool := Order([]config.ModelEntry{disabled, deployment("shared-name", "enabled", 1)}, "shared-name", "weighted-split", st)
			if len(pool) != 1 || pool[0].ParamString("api_base", "") != "enabled" {
				t.Fatalf("retry list contains disabled candidate: %v", pool)
			}
		}
	}
}

func TestNamedCredentialCooldownAndRetryIsolation(t *testing.T) {
	a := deployment("shared-name", "same", 1)
	b := deployment("shared-name", "same", 1)
	a.LiteLLMParams["litellm_credential_name"] = "first"
	b.LiteLLMParams["litellm_credential_name"] = "second"
	if DeploymentID(a) != DeploymentID(b) || CooldownID(a) == CooldownID(b) {
		t.Fatal("credential identity is not isolated")
	}
	st := State{Cooldown: map[string]bool{CooldownID(a): true}, Splits: NewSplitState()}
	for _, strategy := range []string{"weighted-split", "simple-shuffle"} {
		got := Pick([]config.ModelEntry{a, b}, "shared-name", strategy, st)
		if got == nil || CooldownID(*got) != CooldownID(b) {
			t.Fatal("healthy credential was cooled with its sibling")
		}
	}
	pool := Order([]config.ModelEntry{a, b}, "shared-name", "simple-shuffle", st)
	if len(pool) != 1 || CooldownID(pool[0]) != CooldownID(b) {
		t.Fatalf("non-split retry pool contains cooled credential: %v", pool)
	}
	allCooled := State{Cooldown: map[string]bool{CooldownID(a): true, CooldownID(b): true}}
	pool = Order([]config.ModelEntry{a, b}, "shared-name", "simple-shuffle", allCooled)
	if len(pool) != 2 {
		t.Fatalf("all-cooled non-split pool should fail open, got %v", pool)
	}
	pool = Order([]config.ModelEntry{a, b}, "shared-name", "weighted-split", State{Splits: NewSplitState()})
	if len(pool) != 2 {
		t.Fatal("alternate credential lost from retry list")
	}
	counts := map[string]int{}
	st = State{Splits: NewSplitState()}
	for i := 0; i < 10; i++ {
		counts[CooldownID(*Pick([]config.ModelEntry{a, b}, "shared-name", "weighted-split", st))]++
	}
	if counts[CooldownID(a)] != 5 || counts[CooldownID(b)] != 5 {
		t.Fatalf("credential split: %v", counts)
	}
}

func TestCooldownIDUsesConfiguredStablePricingIdentity(t *testing.T) {
	a := deployment("shared-name", "same", 1)
	b := deployment("shared-name", "same", 1)
	for _, entry := range []*config.ModelEntry{&a, &b} {
		entry.LiteLLMParams["litellm_credential_name"] = "shared"
	}
	a.LiteLLMParams["pricing_id"] = "price-row-a"
	b.LiteLLMParams["pricing_id"] = "price-row-b"
	if DeploymentID(a) != DeploymentID(b) {
		t.Fatal("test setup must keep the physical deployment id equal")
	}
	if CooldownID(a) == CooldownID(b) {
		t.Fatal("configured stable deployment identities must isolate cooldown and billing state")
	}
}

func TestRuntimeMetricsUseCredentialAwareIDs(t *testing.T) {
	a := deployment("shared-name", "same", 1)
	b := deployment("shared-name", "same", 1)
	a.LiteLLMParams["litellm_credential_name"] = "first"
	b.LiteLLMParams["litellm_credential_name"] = "second"

	tests := []struct {
		strategy string
		state    State
	}{
		{"least-busy", State{Busy: map[string]int{CooldownID(a): 9, CooldownID(b): 1}}},
		{"lowest-latency", State{Latency: map[string]float64{CooldownID(a): 900, CooldownID(b): 10}}},
		{"lowest-tpm-rpm", State{Usage: map[string]float64{CooldownID(a): 900, CooldownID(b): 10}}},
	}
	for _, tc := range tests {
		got := Pick([]config.ModelEntry{a, b}, "shared-name", tc.strategy, tc.state)
		if got == nil || CooldownID(*got) != CooldownID(b) {
			t.Fatalf("%s ignored credential-aware metric: %v", tc.strategy, got)
		}
	}
}

func TestRuntimeMetricsAcceptLegacyPhysicalID(t *testing.T) {
	a := deployment("shared-name", "a", 1)
	b := deployment("shared-name", "b", 1)
	a.LiteLLMParams["litellm_credential_name"] = "first"
	b.LiteLLMParams["litellm_credential_name"] = "second"
	got := Pick([]config.ModelEntry{a, b}, "shared-name", "lowest-latency", State{Latency: map[string]float64{
		DeploymentID(a): 100, DeploymentID(b): 1,
	}})
	if got == nil || CooldownID(*got) != CooldownID(b) {
		t.Fatalf("legacy physical metric was not used: %v", got)
	}
}
