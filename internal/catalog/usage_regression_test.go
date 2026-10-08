package catalog

import "testing"

func TestNormalizeUsagePreservesExplicitZeroOverAliases(t *testing.T) {
	tests := []struct {
		name  string
		usage map[string]any
		want  Usage
	}{
		{"tokens", map[string]any{"prompt_tokens": 0, "input_tokens": 99, "completion_tokens": 0, "output_tokens": 88}, Usage{}},
		{"cache creation", map[string]any{"cache_creation_input_tokens": 0, "cache_write_tokens": 77}, Usage{}},
		{"separate cache", map[string]any{"input_tokens": 10, "cache_read_input_tokens": 0, "cache_read_tokens": 66, "cached_tokens": 55}, Usage{PromptTokens: 10}},
		{"top level cache", map[string]any{"prompt_tokens": 10, "cached_tokens": 0, "prompt_tokens_details": map[string]any{"cached_tokens": 5}}, Usage{PromptTokens: 10}},
		{"nested cache", map[string]any{"prompt_tokens": 10, "prompt_tokens_details": map[string]any{"cached_tokens": 0}, "input_tokens_details": map[string]any{"cached_tokens": 5}}, Usage{PromptTokens: 10}},
		{"null permits alias", map[string]any{"prompt_tokens": nil, "input_tokens": 9, "cache_read_input_tokens": nil, "cache_read_tokens": 3}, Usage{PromptTokens: 12, CachedTokens: 3}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := NormalizeUsage(tc.usage); got != tc.want {
				t.Fatalf("got %+v want %+v", got, tc.want)
			}
		})
	}
}
