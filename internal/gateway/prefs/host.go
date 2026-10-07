// Package prefs serves router settings and general settings. A key present in the database overrides YAML. A key that is absent stays.
package prefs

import (
	"net/http"

	"github.com/sunqirui1987/xhub/internal/auth"
	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/logx"
	"github.com/sunqirui1987/xhub/internal/store"
)

// Host is what settings reads and writes ask the process for. *gateway.Server implements it. This package does not import gateway.
// Config returns the in-process config pointer. ApplyTyped changes its routing strategy, retries, and timeout, the same fields as before this package was split, and it does not add a lock.
type Host interface {
	// 要求当前请求具备管理权限。失败时已经写好响应并返回 nil。
	// 参数 w（http.ResponseWriter）：调用方的 HTTP 响应，状态码和正文写在这里；r（*http.Request）：入站 HTTP 请求，用来读路径、头和正文。
	// 返回 *auth.Principal（*auth.Principal）：已经解析的调用方，含用户、团队和密钥。
	// 调用：gateway/family/handlers.go、gateway/family/host.go、gateway/guard/guard.go、gateway/guard/host.go
	// 测试：guard_test.go
	RequireManage(w http.ResponseWriter, r *http.Request) *auth.Principal
	// 返回保存代理配置和凭据的库。
	// 参数：无。
	// 返回 Store（*store.Store）：交给调用方的配置库。
	// 调用：gateway/family/handlers.go、gateway/family/host.go、gateway/guard/guard.go、gateway/guard/host.go
	// 测试：guard_test.go
	RecordStore() *store.Store
	// 返回当前进程配置，含模型表和数据库地址。
	// 参数：无。
	// 返回 *config.Config（*config.Config）：当前进程配置，含模型表和路由策略，不会复制。
	// 调用：gateway/prefs/settings.go、gateway/wire.go
	// 测试：无直接单测
	Config() *config.Config
}

// traceModule records that settings routes are being mounted.
// 参数 name（string）：正在挂载的模块名，只写进进程日志。
// 返回：无。只写进程日志，不写 HTTP 响应。
// 调用：prefs 的 mount。
// 测试：无直接单测
func traceModule(name string) {
	logx.Trace("mount %s", name)
}
