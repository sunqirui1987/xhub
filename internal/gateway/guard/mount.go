// Package guard 注册护栏调试路由；真实请求拦截由 Evaluate 在上游调用前执行。
package guard

import (
	"net/http"

	"github.com/sunqirui1987/xhub/internal/httpx"
	"github.com/sunqirui1987/xhub/internal/logx"
	"sync"
)

var logTraceOnceMount sync.Once

// Module 注册两个共用 Apply 处理器的管理调试接口。
// 参数：h：提供管理鉴权和规则存储能力的宿主。
// 返回：httpx.Module：可挂载路由模块；调试不修改持久化规则。
// 调用：gateway/engine.go 的模块安装流程。
// 测试：guard_test.go 验证 Apply；模块安装无独立单测。
func Module(h Host) httpx.Module {
	logTraceOnceMount.Do(func() { logx.Trace("enter guard.Module") })

	traceModule("guard")
	return httpx.Bind("guard", func(reg httpx.Registrar) {
		reg.Handle("POST /apply_guardrail", func(w http.ResponseWriter, r *http.Request) { Apply(h, w, r) })
		reg.Handle("POST /guardrails/apply_guardrail", func(w http.ResponseWriter, r *http.Request) { Apply(h, w, r) })
	})
}
