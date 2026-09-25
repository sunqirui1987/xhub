// Responses 的专用入口。其余目录路径仍由进程按 routes.json 逐条挂上。
package family

import (
	"net/http"

	"github.com/sunqirui1987/xhub/internal/gateway/module"
)

// Module 把 Responses API 交给数据面。目录里其它资源不在这个模块里。
func Module(h Host) module.Module {
	return module.Bind("family", func(reg module.Registrar) {
		reg.Handle("POST /v1/responses", func(w http.ResponseWriter, r *http.Request) { Responses(h, w, r) })
		reg.Handle("POST /responses", func(w http.ResponseWriter, r *http.Request) { Responses(h, w, r) })
	})
}
