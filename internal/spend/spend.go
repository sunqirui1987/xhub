package spend

import (
	"strconv"
	"strings"
)

// Prices USD per token. Unknown models return ok=false (must not record 0).
func Cost(model string, prompt, completion int) (total, input, output float64, ok bool) {
	in, out, ok := rates(model)
	if !ok {
		return 0, 0, 0, false
	}
	input = float64(prompt) * in
	output = float64(completion) * out
	return input + output, input, output, true
}

func rates(model string) (input, output float64, ok bool) {
	m := strings.ToLower(model)
	switch {
	case strings.Contains(m, "gpt-4o-mini"):
		return 0.15e-6, 0.60e-6, true
	case strings.Contains(m, "gpt-4o"):
		return 2.5e-6, 10e-6, true
	case strings.Contains(m, "dall-e"), strings.Contains(m, "whisper"), strings.Contains(m, "tts"),
		strings.Contains(m, "sora"), strings.Contains(m, "moderation"), strings.Contains(m, "rerank"):
		return 0.15e-6, 0.60e-6, true
	default:
		return 0, 0, false
	}
}

func Format(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}
