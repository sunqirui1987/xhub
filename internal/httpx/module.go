// Package httpx also carries the gateway module registration surface. A feature mounts through Registrar and does not import the process type.
package httpx

import "net/http"

// Registrar is the route surface a module may use. Handle takes a pattern of "METHOD /path", for example "GET /health".
type Registrar interface {
	Handle(pattern string, h http.HandlerFunc)
}

// Module is one feature that can be mounted on the gateway. Name must be unique in the process.
type Module interface {
	Name() string
	Mount(reg Registrar)
}

// Bind builds a module from a name and a mount function. An empty name stays empty, and the process refuses to register it.
func Bind(name string, mount func(Registrar)) Module {
	return bound{name: name, mount: mount}
}

type bound struct {
	name  string
	mount func(Registrar)
}

// Name returns the stable name used at registration.
func (b bound) Name() string { return b.name }

// Mount attaches this module's paths to the registrar. It does nothing when the mount function is missing.
func (b bound) Mount(reg Registrar) {
	if b.mount != nil && reg != nil {
		b.mount(reg)
	}
}
