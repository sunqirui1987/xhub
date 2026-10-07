// Package httpx also carries the gateway module registration surface. A feature mounts through Registrar and does not import the process type.
package httpx

import (
	"net/http"
	"sync"

	"github.com/sunqirui1987/xhub/internal/logx"
)

// Registrar is the route surface a module may use. Handle takes a pattern of "METHOD /path", for example "GET /health".
type Registrar interface {
	// Handle 登记一条方法和路径。同名路径先登记的生效。
	// 参数 pattern（string）：方法和路径，例如 GET /health。查询串不参与匹配；h（http.HandlerFunc）：这条路径的处理函数。
	// 返回：无。处理函数以后才写响应。
	// 调用：各模块的 mount。
	// 测试：access_log_test.go、console_split_test.go、dial_log_test.go
	Handle(pattern string, h http.HandlerFunc)
}

// Module is one feature that can be mounted on the gateway. Name must be unique in the process.
type Module interface {
	// Name 返回注册时使用的稳定名字。空名字不会被接受。
	// 参数：无。
	// 返回 string（string）：注册表里唯一的模块名。空串会被拒绝。
	// 调用：gateway/routes.go。
	// 测试：无直接单测
	Name() string
	// Mount 把本模块的路由挂到网关。先挂的路径优先。
	// 参数 reg（Registrar）：路由登记口。nil 时什么都不挂。
	// 返回：无。只登记路由，不写 HTTP 响应。
	// 调用：gateway/routes.go。
	// 测试：无直接单测
	Mount(reg Registrar)
}

var logTraceOnceModule sync.Once

// Bind builds a module from a name and a mount function. An empty name stays empty, and the process refuses to register it.
// 参数 name（string）：模块的稳定名字。空串会在注册时被拒绝；mount（func(Registrar)）：把路由登记到网关的函数。
// 返回 Module（Module）：可挂到网关上的模块。不会返回 nil。
// 调用：各模块的 mount.go。
// 测试：无直接单测
func Bind(name string, mount func(Registrar)) Module {
	logTraceOnceModule.Do(func() { logx.Trace("enter httpx.Bind") })

	return bound{name: name, mount: mount}
}

type bound struct {
	name  string
	mount func(Registrar)
}

// Name returns the stable name used at registration.
// 参数：无。
// 返回 string（string）：Bind 时传入的名字。空串表示这个模块不该被注册。
// 调用：gateway/routes.go。
// 测试：无直接单测
func (b bound) Name() string { return b.name }

// Mount attaches this module's paths to the registrar. It does nothing when the mount function is missing.
// 参数 reg（Registrar）：路由登记口。nil 或没有挂载函数时直接返回。
// 返回：无。只登记路由，不写 HTTP 响应。
// 调用：gateway/routes.go。
// 测试：无直接单测
func (b bound) Mount(reg Registrar) {
	if b.mount != nil && reg != nil {
		b.mount(reg)
	}
}
