// 网关功能模块。实现只依赖这个注册面，不引用进程类型。进程按名字挂上模块，不写死每条路径。
package module

import "net/http"

// Registrar 是模块能用的路由面。Handle 的 pattern 是「方法 路径」，例如 "GET /health"。
type Registrar interface {
	Handle(pattern string, h http.HandlerFunc)
}

// Module 是一块可以装到网关上的功能。Name 在同一进程里必须唯一。
type Module interface {
	Name() string
	Mount(reg Registrar)
}

// Bind 用名字和挂载函数做成一个模块。名字为空时 Name 返回空串，进程会拒绝注册。
func Bind(name string, mount func(Registrar)) Module {
	return bound{name: name, mount: mount}
}

type bound struct {
	name  string
	mount func(Registrar)
}

// Name 是注册时用的稳定名字。
func (b bound) Name() string { return b.name }

// Mount 把这个模块的路径挂到注册面上。没有挂载函数时什么也不做。
func (b bound) Mount(reg Registrar) {
	if b.mount != nil && reg != nil {
		b.mount(reg)
	}
}
