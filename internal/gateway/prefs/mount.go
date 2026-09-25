// 设置模块的路由。数据库键覆盖 YAML 的规则在处理函数里，不在注册这层。
package prefs

import (
	"net/http"

	"github.com/sunqirui1987/xhub/internal/gateway/module"
)

// Module 是路由设置和通用设置的读写。
func Module(h Host) module.Module {
	return module.Bind("prefs", func(reg module.Registrar) {
		reg.Handle("GET /router/settings", func(w http.ResponseWriter, r *http.Request) { Page(h, w, r) })
		reg.Handle("GET /router/fields", func(w http.ResponseWriter, r *http.Request) { Page(h, w, r) })
		reg.Handle("GET /get/config/callbacks", func(w http.ResponseWriter, r *http.Request) { Callbacks(h, w, r) })
		reg.Handle("GET /config/list", func(w http.ResponseWriter, r *http.Request) { List(h, w, r) })
		reg.Handle("POST /config/update", func(w http.ResponseWriter, r *http.Request) { Update(h, w, r) })
		reg.Handle("POST /config/field/update", func(w http.ResponseWriter, r *http.Request) { FieldUpdate(h, w, r) })
		reg.Handle("POST /config/field/delete", func(w http.ResponseWriter, r *http.Request) { FieldDelete(h, w, r) })
	})
}
