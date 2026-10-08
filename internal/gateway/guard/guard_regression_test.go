package guard

import (
	"strings"
	"testing"
)

func TestGuardrailLookupFailureBlocks(t *testing.T) {
	st := openGuardStore(t)
	_ = st.Engine.Close()
	blocked, message, _ := Evaluate(&savedHost{store: st}, map[string]any{"text": "hello"})
	if !blocked || !strings.Contains(message, "guardrail_store_unavailable") {
		t.Fatalf("blocked=%v message=%q", blocked, message)
	}
}

func TestGuardrailRedactsEveryTextLeafWithoutChangingImageURL(t *testing.T) {
	st := openGuardStore(t)
	putGuardrail(t, st, "mask", map[string]any{"default_on": true, "litellm_params": map[string]any{"mode": "redact", "keywords": []any{"secret", "private"}}})
	image := map[string]any{"type": "image_url", "image_url": map[string]any{"url": "https://example.com/secret.png"}}
	part := map[string]any{"type": "input_text", "text": "SECRET and Private"}
	body := map[string]any{"instructions": "secret", "input": []any{map[string]any{"content": []any{part, image}}}}
	blocked, _, findings := Evaluate(&savedHost{store: st}, body)
	if blocked || len(findings) != 1 || body["instructions"] != "[REDACTED]" || part["text"] != "[REDACTED] and [REDACTED]" {
		t.Fatalf("blocked=%v body=%#v part=%#v findings=%#v", blocked, body, part, findings)
	}
	if image["image_url"].(map[string]any)["url"] != "https://example.com/secret.png" {
		t.Fatal("image URL changed")
	}
}
