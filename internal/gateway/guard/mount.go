// Package guard registers the guardrail HTTP routes. The trial endpoint is mounted here. Blocking still happens in PreCall.
package guard

import (
	"net/http"

	"github.com/sunqirui1987/xhub/internal/httpx"
	"github.com/sunqirui1987/xhub/internal/logx"
	"sync"
)

var logTraceOnceMount sync.Once

// Module is the guardrail trial API. The data plane calls PreCall before the upstream and does not go through these two paths.
func Module(h Host) httpx.Module {
	logTraceOnceMount.Do(func() { logx.Trace("enter guard.Module") })

	traceModule("guard")
	return httpx.Bind("guard", func(reg httpx.Registrar) {
		reg.Handle("POST /apply_guardrail", func(w http.ResponseWriter, r *http.Request) { Apply(h, w, r) })
		reg.Handle("POST /guardrails/apply_guardrail", func(w http.ResponseWriter, r *http.Request) { Apply(h, w, r) })
	})
}
