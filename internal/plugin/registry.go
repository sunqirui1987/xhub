// Package plugin defines the extension contract that runs before an inference call reaches the upstream. Implementations register by a stable name. This package does not import those implementations, and they do not have to import the gateway process.
package plugin

import (
	"fmt"
	"sync"
)

// Call is the inference attempt an extension can read. The data plane fills it before contacting the upstream.
type Call struct {
	Op    string
	Model string
	Path  string
}

// Decision is an extension's result. When Refuse is true the data plane skips the cache and does not contact the upstream. Header is copied onto the response even when the call is allowed, so the caller can see that the extension ran.
type Decision struct {
	Refuse  bool
	Status  int
	Code    string
	Message string
	Header  map[string]string
}

// Extension is one named pre-call check. Name must be unique in a registry.
type Extension interface {
	Name() string
	BeforeUpstream(call Call) Decision
}

// Registry stores extensions in registration order. The zero value is not usable; call New.
type Registry struct {
	mu    sync.Mutex
	order []Extension
	by    map[string]Extension
}

// New returns an empty registry. Run allows the call when nothing is registered.
func New() *Registry {
	return &Registry{by: map[string]Extension{}}
}

// Register appends an extension. An empty or duplicate name returns an error and leaves the existing order unchanged.
func (r *Registry) Register(ext Extension) error {
	if r == nil {
		return fmt.Errorf("plugin registry is nil")
	}
	if ext == nil || ext.Name() == "" {
		return fmt.Errorf("plugin name is required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	name := ext.Name()
	if _, ok := r.by[name]; ok {
		return fmt.Errorf("plugin %q is already registered", name)
	}
	r.by[name] = ext
	r.order = append(r.order, ext)
	return nil
}

// Names returns a copy of the registered names in order. Changing the slice does not change the registry.
func (r *Registry) Names() []string {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, len(r.order))
	for i, ext := range r.order {
		out[i] = ext.Name()
	}
	return out
}

// Invoke runs the extension registered under name. A missing name returns an error and does not call any other extension.
func (r *Registry) Invoke(name string, call Call) (Decision, error) {
	if r == nil {
		return Decision{}, fmt.Errorf("plugin %q is not registered", name)
	}
	r.mu.Lock()
	ext, ok := r.by[name]
	r.mu.Unlock()
	if !ok {
		return Decision{}, fmt.Errorf("plugin %q is not registered", name)
	}
	return ext.BeforeUpstream(call), nil
}

// Run calls every extension in registration order. The first refusal stops the rest and keeps headers already set. An empty registry returns a zero Decision so the data plane continues.
func (r *Registry) Run(call Call) Decision {
	if r == nil {
		return Decision{}
	}
	r.mu.Lock()
	list := append([]Extension(nil), r.order...)
	r.mu.Unlock()
	merged := map[string]string{}
	for _, ext := range list {
		d := ext.BeforeUpstream(call)
		for k, v := range d.Header {
			merged[k] = v
		}
		if d.Refuse {
			d.Header = merged
			return d
		}
	}
	if len(merged) == 0 {
		return Decision{}
	}
	return Decision{Header: merged}
}
