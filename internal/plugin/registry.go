// Package plugin defines the extension contract that runs before an inference call reaches the upstream. Implementations register by a stable name. This package does not import those implementations, and they do not have to import the gateway process.
package plugin

import (
	"fmt"
	"sync"

	"github.com/sunqirui1987/xhub/internal/logx"
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
	// 返回注册表使用的稳定名字。空名字不会被接受。
	// 参数：无。
	// 返回：注册表里唯一的扩展名。空串会被 Register 拒绝。
	// 调用：gateway/routes.go、httpx/module.go
	// 测试：无直接单测
	Name() string
	// 在联系上游之前做一次检查。拒绝时数据面不再访问上游。
	// 参数 call（Call）：这一次扩展看到的调用，含操作名、模型和路径。
	// 返回 Decision（Decision）：扩展给出的决定，拒绝时调用方不再访问上游。
	// 调用：仅在 registry.go 内使用
	// 测试：无直接单测
	BeforeUpstream(call Call) Decision
}

// Registry stores extensions in registration order. Its zero value is ready to use.
type Registry struct {
	mu    sync.Mutex
	order []Extension
	names []string
	by    map[string]Extension
}

var logTraceOnceRegistry sync.Once

// New returns an empty registry. Run allows the call when nothing is registered.
// 参数：无。
// 调用：authz/authz.go、authz/decide.go、cache/cache.go、dataplane/serve.go
// 测试：activity_http_test.go、authz_test.go、builtin_providers_test.go
// 返回：空注册表，可以立刻 Register。不会返回 nil。
func New() *Registry {
	logTraceOnceRegistry.Do(func() { logx.Trace("enter plugin.New") })

	return &Registry{by: map[string]Extension{}}
}

// Register appends an extension. An empty or duplicate name returns an error and leaves the existing order unchanged.
// 返回 error（error）：失败原因，nil 表示这一步成功。
// 调用：仅在 registry.go 内使用
// 测试：无直接单测
// 参数 ext（Extension）：要注册的扩展。
func (r *Registry) Register(ext Extension) error {
	if r == nil {
		return fmt.Errorf("plugin registry is nil")
	}
	if ext == nil {
		return fmt.Errorf("plugin name is required")
	}
	name := ext.Name()
	if name == "" {
		return fmt.Errorf("plugin name is required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.by == nil {
		r.by = make(map[string]Extension)
	}
	if _, ok := r.by[name]; ok {
		return fmt.Errorf("plugin %q is already registered", name)
	}
	r.by[name] = ext
	r.order = append(r.order, ext)
	r.names = append(r.names, name)
	return nil
}

// Names returns a copy of the registered names in order. Changing the slice does not change the registry.
// 参数：无。
// 调用：仅在 registry.go 内使用
// 测试：无直接单测
// 返回：按注册顺序复制的扩展名。改返回的切片不会改注册表。没有扩展时为空切片。
func (r *Registry) Names() []string {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.names...)
}

// Invoke runs the extension registered under name. A missing name returns an error and does not call any other extension.
// 参数 name（string）：注册表里的扩展名。没有这个名字时返回错误，不会顺手调用别的扩展；call（Call）：这一次扩展看到的调用，含操作名、模型和路径。
// 返回 Decision（Decision）：该扩展的决定。拒绝时调用方不再访问上游；error（error）：注册表里没有这个名字时非 nil。
// 调用：仅在 registry.go 内使用
// 测试：无直接单测
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
// 调用：dataplane/serve.go、gateway/server.go、live/redis.go
// 测试：authz_test.go、catalog_reads_test.go、chains_test.go
// 参数 call（Call）：这一次扩展看到的调用，含操作名、模型和路径。
// 返回 Decision（Decision）：扩展给出的决定，拒绝时调用方不再访问上游。
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
