package dataplane

import (
	"testing"
	"time"

	"github.com/sunqirui1987/xhub/internal/config"
)

func TestPreferDeploymentMovesThePinnedOneFirst(t *testing.T) {
	pool := []config.ModelEntry{
		{ModelName: "m", LiteLLMParams: map[string]any{"api_base": "https://a.example", "model": "m"}},
		{ModelName: "m", LiteLLMParams: map[string]any{"api_base": "https://b.example", "model": "m"}},
	}
	got := preferDeployment(pool, "https://b.example|m")
	if got[0].ParamString("api_base", "") != "https://b.example" {
		t.Fatalf("first deployment %s", got[0].ParamString("api_base", ""))
	}
	if len(got) != 2 {
		t.Fatalf("len %d", len(got))
	}
}

func TestPreferDeploymentLeavesAnUnknownPinAlone(t *testing.T) {
	pool := []config.ModelEntry{
		{ModelName: "m", LiteLLMParams: map[string]any{"api_base": "https://a.example", "model": "m"}},
	}
	got := preferDeployment(pool, "missing")
	if got[0].ParamString("api_base", "") != "https://a.example" {
		t.Fatalf("pool changed: %+v", got)
	}
}

func TestOutputTokensCountsStreamedTextWhenUsageIsMissing(t *testing.T) {
	raw := []byte("data: {\"choices\":[{\"delta\":{\"content\":\"Hello\"}}]}\n\ndata: {\"choices\":[{\"delta\":{\"content\":\" world\"}}]}\n\n")
	if got := outputTokens(raw); got < 1 {
		t.Fatalf("output tokens %d", got)
	}
}

func TestTTFTMillisOmitsAnUnmeasuredDelay(t *testing.T) {
	if ttftMillis(0) != nil {
		t.Fatal("zero ttft must stay unset")
	}
	got := ttftMillis(1500 * time.Millisecond)
	if got == nil || *got != 1500 {
		t.Fatalf("ttft %v", got)
	}
}
