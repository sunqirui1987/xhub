package usage

import (
	"testing"

	"github.com/sunqirui1987/xhub/internal/iam"
)

func TestEventRowCarriesGuardrailMonitoring(t *testing.T) {
	rows := eventRows([]iam.UsageEvent{{
		RequestID: "req-1",
		Model:     "m",
		Status:    "error",
		Guardrail: `[{"guardrail_name":"no-bombs","guardrail_status":"blocked","guardrail_mode":"pre_call"}]`,
	}})
	meta := rows[0]["metadata"].(map[string]any)
	info, ok := meta["guardrail_information"].([]any)
	if !ok || len(info) != 1 {
		t.Fatalf("guardrail information: %#v", meta["guardrail_information"])
	}
	entry := info[0].(map[string]any)
	if entry["guardrail_name"] != "no-bombs" || entry["guardrail_status"] != "blocked" {
		t.Fatalf("entry: %#v", entry)
	}

	plain := eventRows([]iam.UsageEvent{{RequestID: "req-2", Model: "m"}})
	if _, ok := plain[0]["metadata"].(map[string]any)["guardrail_information"]; ok {
		t.Fatal("a call that was not checked must not invent a guardrail section")
	}
}
