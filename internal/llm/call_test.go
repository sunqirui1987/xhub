package llm

import "testing"

func TestDefaultAPIBase(t *testing.T) {
	if got := DefaultAPIBase("openai"); got != "https://api.openai.com/v1" {
		t.Fatalf("openai base %q", got)
	}
	if got := DefaultAPIBase("OpenAI"); got != "https://api.openai.com/v1" {
		t.Fatalf("OpenAI base %q", got)
	}
	if got := DefaultAPIBase("not-a-provider"); got != "" {
		t.Fatalf("unknown base %q", got)
	}
}
