// 护栏模块的路由。试跑接口从这里挂上，命中拦截的逻辑仍在 PreCall。
package guard

import (
	"net/http"

	"github.com/sunqirui1987/xhub/internal/gateway/module"
)

// Module 是护栏试跑。数据面在发上游之前另调 PreCall，不经过这两条路径。
func Module(h Host) module.Module {
	return module.Bind("guard", func(reg module.Registrar) {
		reg.Handle("POST /apply_guardrail", func(w http.ResponseWriter, r *http.Request) { Apply(h, w, r) })
		reg.Handle("POST /guardrails/apply_guardrail", func(w http.ResponseWriter, r *http.Request) { Apply(h, w, r) })
	})
}
