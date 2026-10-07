// Package keys defines the process capabilities virtual-key management needs. *gateway.Server implements them. This package does not import gateway.
package keys

import (
	"net/http"

	"github.com/sunqirui1987/xhub/internal/auth"
	"github.com/sunqirui1987/xhub/internal/authz"
	"github.com/sunqirui1987/xhub/internal/iam"
	"github.com/sunqirui1987/xhub/internal/logx"
)

// Host is what create, list, and update ask the process for. It offers identity
// and authorization and nothing else: a handler resolves who is calling, asks
// for one decision per object, and only then touches identity data.
type Host interface {
	// RequireUser accepts any signed-in session or virtual key. Key routes use it rather than RequireManage, because a member mints and reads their own keys; the authorization decision narrows the reach, not the gate.
	// 参数 w（http.ResponseWriter）：调用方的 HTTP 响应，状态码和正文写在这里；r（*http.Request）：入站 HTTP 请求，用来读路径、头和正文。
	// 返回 *auth.Principal（*auth.Principal）：已经解析的调用方，含用户、团队和密钥。
	// 调用：gateway/identity/gate.go、gateway/identity/handlers.go、gateway/identity/members.go、gateway/keys/admin.go
	// 测试：无直接单测
	RequireUser(w http.ResponseWriter, r *http.Request) *auth.Principal
	// RequireManage requires a platform administrator session.
	// 参数 w（http.ResponseWriter）：调用方的 HTTP 响应，状态码和正文写在这里；r（*http.Request）：入站 HTTP 请求，用来读路径、头和正文。
	// 返回 *auth.Principal（*auth.Principal）：已经解析的调用方，含用户、团队和密钥。
	// 调用：gateway/family/handlers.go、gateway/family/host.go、gateway/guard/guard.go、gateway/guard/host.go
	// 测试：guard_test.go
	RequireManage(w http.ResponseWriter, r *http.Request) *auth.Principal
	// RequireMixed accepts a management identity or an inference identity. The liveness check uses it.
	// 参数 w（http.ResponseWriter）：调用方的 HTTP 响应，状态码和正文写在这里；r（*http.Request）：入站 HTTP 请求，用来读路径、头和正文。
	// 返回 *auth.Principal（*auth.Principal）：已经解析的调用方，含用户、团队和密钥。
	// 调用：gateway/family/handlers.go、gateway/family/host.go、gateway/keys/admin.go、gateway/wire.go
	// 测试：无直接单测
	RequireMixed(w http.ResponseWriter, r *http.Request) *auth.Principal
	// Authorize decides one action for an already resolved caller. Ownership is read from the database inside the decision, so a handler cannot widen its reach by naming a team the key does not belong to.
	// 参数 r（*http.Request）：入站 HTTP 请求；p（*auth.Principal）：已经鉴权的调用方，含用户、团队、密钥哈希和别名；action（authz.Action）：要判定的动作，例如读日志、管理密钥或发起推理；obj（authz.Object）：要判定的对象，含类型、团队、组织和归属用户。
	// 返回 error（error）：失败原因，nil 表示这一步成功。
	// 调用：authz/decide.go、gateway/identity/gate.go、gateway/identity/handlers.go、gateway/identity/members.go 测试：authz_test.go
	Authorize(r *http.Request, p *auth.Principal, action authz.Action, obj authz.Object) error
	// KeysScope returns the key-listing scope, so a listing is narrowed in SQL instead of being filtered after the rows are read.
	// 参数 r（*http.Request）：入站 HTTP 请求，用来读路径、头和正文；p（*auth.Principal）：已经解析的调用方，含用户、团队和密钥。
	// 返回 *authz.Scope（*authz.Scope）：密钥列表的范围，列表在 SQL 里收窄，而不是先查出再过滤；error（error）：身份还不能列出密钥。nil 表示范围已经定好。
	// 调用：authz/decide.go、gateway/keys/admin.go、gateway/keys/generate.go、gateway/wire.go
	// 测试：authz_test.go
	KeysScope(r *http.Request, p *auth.Principal) (*authz.Scope, error)
	// Identity is the store. A handler reads its rows from here, but only after Authorize has permitted the action that reads them.
	// 参数：无。
	// 返回 *iam.DB（*iam.DB）：密钥处理函数读取密钥行的库，而且必须先通过 Authorize。没有数据库时为 nil。
	// 调用：dataplane/host.go、dataplane/live.go、gateway/identity/gate.go、gateway/identity/handlers.go
	// 测试：bypass_logic_test.go、failure_log_test.go
	Identity() *iam.DB
	// WriteAuthz turns a refused authorization into the response and reports whether one was written.
	// 参数 w（http.ResponseWriter）：调用方的 HTTP 响应，状态码和正文写在这里；r（*http.Request）：入站 HTTP 请求，用来读路径、头和正文；err（error）：失败原因，nil 表示这一步成功。
	// 返回 bool（bool）：已经把拒绝写进响应时返回真。对象不可见是 404，可见但不允许是 403，读归属失败是 500。
	// 调用：gateway/identity/gate.go、gateway/identity/handlers.go、gateway/identity/members.go、gateway/keys/admin.go
	// 测试：无直接单测
	WriteAuthz(w http.ResponseWriter, r *http.Request, err error) bool
	// WriteIAMError turns a store failure into the response and reports whether one was written.
	// 参数 w（http.ResponseWriter）：调用方的 HTTP 响应，状态码和正文写在这里；r（*http.Request）：入站 HTTP 请求，用来读路径、头和正文；err（error）：失败原因，nil 表示这一步成功。
	// 返回 bool（bool）：已经把存储错误写进响应时为真。err 为 nil 时为假，调用方继续用正常结果。
	// 调用：gateway/keys/admin.go、gateway/keys/generate.go
	// 测试：无直接单测
	WriteIAMError(w http.ResponseWriter, r *http.Request, err error) bool
}

// traceModule records that virtual-key routes are being mounted.
// 参数 name（string）：正在挂载的模块名，只写进进程日志。
// 返回：无。只写进程日志，不写 HTTP 响应。
// 调用：keys 的 mount。
// 测试：无直接单测
func traceModule(name string) {
	logx.Trace("mount %s", name)
}
