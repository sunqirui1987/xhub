// 推理发出前的扩展契约。实现按稳定名字注册，本包不引用任何实现，实现也不必引用网关进程。
package plugin

import (
	"fmt"
	"sync"
)

// Call 是扩展能读到的这次推理。数据面在访问上游之前填好它。
type Call struct {
	Op    string
	Model string
	Path  string
}

// Decision 是扩展的结论。Refuse 为真时数据面不读缓存、也不访问上游。
// Header 会写到响应上；继续放行时也保留，方便调用方看到这个扩展确实跑过。
type Decision struct {
	Refuse  bool
	Status  int
	Code    string
	Message string
	Header  map[string]string
}

// Extension 是一个具名扩展。Name 在同一张注册表里必须唯一。
type Extension interface {
	Name() string
	BeforeUpstream(call Call) Decision
}

// Registry 按注册顺序保存扩展。零值不能用，请用 New。
type Registry struct {
	mu    sync.Mutex
	order []Extension
	by    map[string]Extension
}

// New 返回一张空表。没有注册项时 Run 直接放行。
func New() *Registry {
	return &Registry{by: map[string]Extension{}}
}

// Register 把扩展加到表尾。名字为空或重复时返回错误，不改已有顺序。
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

// Names 按注册顺序返回名字的副本。调用方改这个切片不会改表。
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

// Invoke 按注册时的名字调用一个扩展。没有这个名字时返回错误，不调用其他扩展。
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

// Run 按注册顺序调用全部扩展。第一个拒绝会停住后面的扩展，并带上已经写下的响应头。
// 没有注册项时返回零值，数据面照常往下走。
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
