package settings

import "testing"

func TestOverlayDatabaseWinsAndKeepsYAML(t *testing.T) {
	base := map[string]any{
		"routing_strategy": "simple-shuffle",
		"routing_groups":   []any{map[string]any{"group_name": "yaml"}},
		"num_retries":      2,
	}
	db := map[string]any{
		"routing_groups": []any{map[string]any{"group_name": "db"}},
	}
	got := Overlay(base, db)
	if got["routing_strategy"] != "simple-shuffle" || got["num_retries"] != 2 {
		t.Fatalf("yaml keys lost %#v", got)
	}
	groups, _ := got["routing_groups"].([]any)
	if len(groups) != 1 || groups[0].(map[string]any)["group_name"] != "db" {
		t.Fatalf("db group %#v", got["routing_groups"])
	}
}

func TestMergePatchKeepsUnrelatedKeys(t *testing.T) {
	current := map[string]any{
		"routing_strategy":      "least-busy",
		"routing_strategy_args": map[string]any{"ttl": 10},
		"fallbacks":             []any{map[string]any{"gpt-4o": []any{"gpt-4o-mini"}}},
	}
	patch := map[string]any{
		"routing_groups":        []any{map[string]any{"group_name": "prod"}},
		"routing_strategy_args": map[string]any{"lowest_latency_buffer": 0.1},
	}
	got := MergePatch(current, patch)
	if got["routing_strategy"] != "least-busy" {
		t.Fatal(got["routing_strategy"])
	}
	args := got["routing_strategy_args"].(map[string]any)
	if args["ttl"] != 10 || args["lowest_latency_buffer"] != 0.1 {
		t.Fatalf("args %#v", args)
	}
	if _, ok := got["fallbacks"].([]any); !ok {
		t.Fatal("fallbacks wiped")
	}
	if len(got["routing_groups"].([]any)) != 1 {
		t.Fatal(got["routing_groups"])
	}
}
