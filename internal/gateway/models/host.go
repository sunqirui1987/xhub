// Package models defines the process capabilities model management needs. *gateway.Server implements them. This package does not import gateway, which avoids an import cycle.
package models

import (
	"net/http"

	"github.com/sunqirui1987/xhub/internal/auth"
	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/iam"
	"github.com/sunqirui1987/xhub/internal/logx"
	"github.com/sunqirui1987/xhub/internal/store"
)

// Host is what create, update, and list ask the process for.
// Update, delete, and block write the database while holding the model lock. The lock covers the same critical section as before this package was split.
type Host interface {
	// 要求当前请求具备管理权限。失败时已经写好响应并返回 nil。
	// 参数 w（http.ResponseWriter）：调用方的 HTTP 响应，状态码和正文写在这里；r（*http.Request）：入站 HTTP 请求，用来读路径、头和正文。
	// 返回 *auth.Principal（*auth.Principal）：已经解析的调用方，含用户、团队和密钥。
	// 调用：gateway/family/handlers.go、gateway/family/host.go、gateway/guard/guard.go、gateway/guard/host.go
	// 测试：guard_test.go
	RequireManage(w http.ResponseWriter, r *http.Request) *auth.Principal
	// RecordStore holds the framework records this package writes: proxy models, the price-map reload plan, and provider credentials. It never answers an authorization question.
	// 参数：无。
	// 返回 *store.Store（*store.Store）：这个包写入的库，存代理模型、价格重载计划和供应商凭据。没有库时为 nil。
	// 调用：gateway/family/handlers.go、gateway/family/host.go、gateway/guard/guard.go、gateway/guard/host.go
	// 测试：guard_test.go
	RecordStore() *store.Store
	// Identity owns every identity question: which models a team or a key may reach. The model list and the inference path both read it, so a listed model is always a usable model.
	// 参数：无。
	// 返回 *iam.DB（*iam.DB）：用来判断团队或密钥能否使用某个模型的身份库。没有数据库时为 nil。
	// 调用：dataplane/host.go、dataplane/live.go、gateway/identity/gate.go、gateway/identity/handlers.go
	// 测试：bypass_logic_test.go、failure_log_test.go
	Identity() *iam.DB
	// LockModels and UnlockModels are a pair. Lock before changing the model table on a request path.
	// 参数：无。
	// 调用：gateway/models/admin.go、gateway/models/available.go、gateway/models/builtin.go、gateway/models/list.go
	// 测试：无直接单测
	// 返回：无。模型表已锁上。改表之前调用，必须配 UnlockModels。
	LockModels()
	// 放开模型表的写锁。读完或改完部署后必须调用，避免表一直被锁住。
	// 参数：无。
	// 返回：无。模型表的写锁已放开。读完或改完部署后必须调用。
	// 调用：gateway/models/admin.go、gateway/models/available.go、gateway/models/builtin.go、gateway/models/list.go
	// 测试：无直接单测
	UnlockModels()
	// ModelTable returns a pointer to the in-process model slice. Change it only while holding the lock. LoadStored runs before the process serves traffic, when there are no concurrent requests.
	// 参数：无。
	// 调用：gateway/models/admin.go、gateway/models/available.go、gateway/models/builtin.go、gateway/models/list.go
	// 测试：无直接单测
	// 返回：进程内模型切片的指针。修改前必须持有锁。LoadStored 发生在开始接流量之前。
	ModelTable() *[]config.ModelEntry
	// Resolve parses a session or a virtual key. On failure it does not write a response. The caller chooses the 401 body.
	// 参数 r（*http.Request）：入站 HTTP 请求，用来读路径、头和正文。
	// 返回 *auth.Principal（*auth.Principal）：已经解析的调用方，含用户、团队和密钥；error（error）：失败原因，nil 表示这一步成功。
	// 调用：auth/auth.go、gateway/models/available.go、gateway/models/list.go、gateway/session.go
	// 测试：无直接单测
	Resolve(r *http.Request) (*auth.Principal, error)
	// AllowLLM reports whether this identity may call inference. The master key may not unless allow_master_key_llm is on.
	// 参数 p（*auth.Principal）：已经解析的调用方，含用户、团队和密钥。
	// 返回 bool（bool）：这个身份可以发起推理时为真。主密钥默认不可以，除非打开了 allow_master_key_llm。
	// 调用：gateway/models/available.go、gateway/models/list.go
	// 测试：无直接单测
	AllowLLM(p *auth.Principal) bool
}

// traceModule records that model routes are being mounted.
// 参数 name（string）：正在挂载的模块名，只写进进程日志。
// 返回：无。只写进程日志，不写 HTTP 响应。
// 调用：models 的 mount。
// 测试：无直接单测
func traceModule(name string) {
	logx.Trace("mount %s", name)
}
