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
// 参数 h（Host）：实现这一步所需能力的数据面宿主。聊天、直通和刷写各自只依赖自己的方法。
// 返回 httpx.Module（httpx.Module）：可挂到网关上的模块。
// 调用：gateway/family/mount.go、gateway/identity/mount.go、gateway/keys/mount.go、gateway/models/mount.go
// 测试：无直接单测
func Module(h Host) httpx.Module {
	logTraceOnceMount.Do(func() { logx.Trace("enter guard.Module") })

	traceModule("guard")
	return httpx.Bind("guard", func(reg httpx.Registrar) {
		reg.Handle("POST /apply_guardrail", func(w http.ResponseWriter, r *http.Request) { Apply(h, w, r) })
		reg.Handle("POST /guardrails/apply_guardrail", func(w http.ResponseWriter, r *http.Request) { Apply(h, w, r) })
	})
}
