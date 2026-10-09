// Package guard 在上游调用前执行正文规则，拦截会阻止提供商请求。
package guard

import (
	"net/http"

	"github.com/sunqirui1987/xhub/internal/auth"
	"github.com/sunqirui1987/xhub/internal/logx"
	"github.com/sunqirui1987/xhub/internal/store"
)

// Host 定义管理鉴权和配置存储边界，由 gateway.Server 实现，避免本包反向依赖网关。
type Host interface {
	// RequireManage 校验当前请求的管理权限，统一由宿主解析身份。
	// 参数：w：鉴权失败的响应写入位置；r：携带认证信息的入站请求。
	// 返回：认证后的调用方；失败返回 nil，并已写入 HTTP 错误，调用方必须停止处理。
	// 调用：guard.go、custom_http.go、manage.go 的管理和调试入口。
	// 测试：guard_test.go、custom_test.go；网关管理接口测试验证实际权限。
	RequireManage(w http.ResponseWriter, r *http.Request) *auth.Principal
	// RecordStore 提供保存护栏及代理设置的配置存储。
	// 参数：无。
	// 返回：配置库；存储为空或读取失败时，执行层拒绝把失败当作没有规则。
	// 调用：guard.go 的规则选择、manage.go 的 CRUD 及规则读取辅助方法。
	// 测试：engine_test.go、网关 guardrail_manage_test.go。
	RecordStore() *store.Store
}

// traceModule 记录护栏模块注册，仅用于进程诊断。
// 参数：name：正在挂载的模块名，只写进进程日志。
// 返回：无。只写进程日志，不写 HTTP 响应。
// 调用：guard 的 mount。
// 测试：无直接单测
func traceModule(name string) {
	logx.Trace("mount %s", name)
}
