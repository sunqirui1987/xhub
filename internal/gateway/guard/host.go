// Package guard runs content rules before a request is sent upstream. A blocking match stops the data plane from calling the provider.
package guard

import (
	"net/http"

	"github.com/sunqirui1987/xhub/internal/auth"
	"github.com/sunqirui1987/xhub/internal/logx"
	"github.com/sunqirui1987/xhub/internal/store"
)

// Host is what a guardrail trial and a config read ask the process for. *gateway.Server implements it. This package does not import gateway.
type Host interface {
	// 要求当前请求具备管理权限。失败时已经写好响应并返回 nil。
	// 参数 w（http.ResponseWriter）：调用方的 HTTP 响应，状态码和正文写在这里；r（*http.Request）：入站 HTTP 请求，用来读路径、头和正文。
	// 返回 *auth.Principal（*auth.Principal）：已经解析的调用方，含用户、团队和密钥。
	// 调用：gateway/family/handlers.go、gateway/family/host.go、gateway/guard/guard.go、gateway/identity/gate.go 测试：guard_test.go
	RequireManage(w http.ResponseWriter, r *http.Request) *auth.Principal
	// 返回保存代理配置和凭据的库。
	// 参数：无。
	// 返回 Store（*store.Store）：交给调用方的配置库。
	// 调用：gateway/family/handlers.go、gateway/family/host.go、gateway/guard/guard.go、gateway/models/admin.go
	// 测试：guard_test.go
	RecordStore() *store.Store
}

// traceModule records that guardrail routes are being mounted.
// 参数 name（string）：正在挂载的模块名，只写进进程日志。
// 返回：无。只写进程日志，不写 HTTP 响应。
// 调用：guard 的 mount。
// 测试：无直接单测
func traceModule(name string) {
	logx.Trace("mount %s", name)
}
