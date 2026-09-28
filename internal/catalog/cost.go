// Package catalog prices a token count from the built-in price map. An unknown model returns ok false and must not be recorded as zero.
package catalog

import (
	"github.com/sunqirui1987/xhub/internal/logx"
	"strconv"
	"sync"
)

var logTraceOnceCost sync.Once

// Cost returns the dollar total, input cost, and output cost for a token count. An unknown model returns ok false so the caller does not record a free call.
func Cost(model string, prompt, completion int) (total, input, output float64, ok bool) {
	logTraceOnceCost.Do(func() { logx.Trace("enter catalog.Cost") })

	in, out, ok := rates(model)
	if !ok {
		return 0, 0, 0, false
	}
	input = float64(prompt) * in
	output = float64(completion) * out
	return input + output, input, output, true
}

// rates returns the per-token dollar price for input and output from the built-in price map. An unknown model returns ok false.
func rates(model string) (input, output float64, ok bool) {
	return TokenRates(model)
}

// Format renders a dollar amount as a decimal string without trailing zeros.
func Format(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}
