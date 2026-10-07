// Package family mounts the Responses API itself. The process still registers every other catalog path from routes.json.
package family

import (
	"net/http"

	"github.com/sunqirui1987/xhub/internal/httpx"
	"github.com/sunqirui1987/xhub/internal/logx"
	"sync"
)

var logTraceOnceMount sync.Once

// Module sends the Responses API to the data plane. Other catalog resources are not part of this module.
// 参数 h（Host）：实现这一步所需能力的数据面宿主。聊天、直通和刷写各自只依赖自己的方法。
// 返回 httpx.Module（httpx.Module）：可挂到网关上的模块。
// 调用：gateway/guard/mount.go、gateway/identity/mount.go、gateway/keys/mount.go、gateway/models/mount.go
// 测试：无直接单测
func Module(h Host) httpx.Module {
	logTraceOnceMount.Do(func() { logx.Trace("enter family.Module") })

	traceModule("family")
	return httpx.Bind("family", func(reg httpx.Registrar) {
		reg.Handle("POST /v1/responses", func(w http.ResponseWriter, r *http.Request) { Responses(h, w, r) })
		reg.Handle("POST /responses", func(w http.ResponseWriter, r *http.Request) { Responses(h, w, r) })
	})
}
