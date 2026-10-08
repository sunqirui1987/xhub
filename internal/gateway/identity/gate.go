// Package identity defines the process capabilities account, organization,
// team, member and project management need. *gateway.Server implements them.
// This package does not import gateway.
package identity

import (
	"net/http"

	"github.com/sunqirui1987/xhub/internal/auth"
	"github.com/sunqirui1987/xhub/internal/authz"
	"github.com/sunqirui1987/xhub/internal/iam"
	"github.com/sunqirui1987/xhub/internal/logx"
)

// Gate is what these handlers ask the process for. It offers identity and
// authorization and nothing else: a handler resolves who is calling, asks for
// one decision per object, and only then touches identity data.
type Gate interface {
	// RequireUser accepts any signed-in session or virtual key. Most routes here use it rather than RequireManage, because a member reads their own team, its projects and its members; the authorization decision narrows the reach, not the gate.
	// 参数 w（http.ResponseWriter）：调用方的 HTTP 响应，状态码和正文写在这里；r（*http.Request）：入站 HTTP 请求，用来读路径、头和正文。
	// 返回 *auth.Principal（*auth.Principal）：已经解析的调用方，含用户、团队和密钥。
	// 调用：gateway/identity/handlers.go、gateway/identity/members.go、gateway/keys/admin.go、gateway/keys/generate.go
	// 测试：无直接单测
	RequireUser(w http.ResponseWriter, r *http.Request) *auth.Principal
	// RequireManage requires a platform administrator session.
	// 参数 w（http.ResponseWriter）：调用方的 HTTP 响应，状态码和正文写在这里；r（*http.Request）：入站 HTTP 请求，用来读路径、头和正文。
	// 返回 *auth.Principal（*auth.Principal）：已经解析的调用方，含用户、团队和密钥。
	// 调用：gateway/family/handlers.go、gateway/family/host.go、gateway/guard/guard.go、gateway/guard/host.go
	// 测试：guard_test.go
	RequireManage(w http.ResponseWriter, r *http.Request) *auth.Principal
	// Authorize decides one action for an already resolved caller. Ownership is read from the database inside the decision, so a handler cannot widen its reach by naming a team the caller does not belong to
	// .
	// 参数 r（*http.Request）：入站 HTTP 请求；p（*auth.Principal）：已经鉴权的调用方，含用户、团队、密钥哈希和别名；action（authz.Action）：要判定的动作，例如读日志、管理密钥或发起推理；obj（authz.Object）：要判定的对象，含类型、团队、组织和归属用户。
	// 返回 error（error）：失败原因，nil 表示这一步成功。
	// 调用：authz/decide.go、gateway/identity/handlers.go、gateway/identity/members.go、gateway/keys/admin.go
	// 测试：authz_test.go
	Authorize(r *http.Request, p *auth.Principal, action authz.Action, obj authz.Object) error
	// TeamFilter returns the teams whose rows a listing may include. A nil slice means every team, which only a platform administrator receives; an empty slice means no team.
	// 参数 r（*http.Request）：入站 HTTP 请求，用来读路径、头和正文；p（*auth.Principal）：已经解析的调用方，含用户、团队和密钥。
	// 返回 []string（[]string）：团队过滤。没有匹配时为空切片；error（error）：失败原因。nil 表示这一步成功。
	// 调用：authz/decide.go、gateway/identity/handlers.go、gateway/wire.go
	// 测试：authz_test.go
	TeamFilter(r *http.Request, p *auth.Principal) ([]string, error)
	// RouterDocument is the merged platform router settings - the bottom of the
	// inheritance chain a scope falls back to when it selects no template. It is
	// here so the console resolves effective settings through the same code the
	// request path uses rather than a second copy that can disagree with it.
	// 参数：无。
	// 返回 map[string]any（map[string]any）：合并后的平台路由设置。
	// 调用：gateway/identity/route_template.go
	// 测试：route_template_test.go
	RouterDocument() map[string]any
	// Identity is the store. A handler reads its rows from here, but only after Authorize has permitted the action that reads them.
	// 参数：无。
	// 返回 *iam.DB（*iam.DB）：处理函数读取用户、团队和密钥的库，而且必须先通过 Authorize。没有数据库时为 nil。
	// 调用：dataplane/host.go、dataplane/live.go、gateway/identity/handlers.go、gateway/identity/members.go
	// 测试：bypass_logic_test.go、failure_log_test.go
	Identity() *iam.DB
	// WriteAuthz turns a refused authorization into the response and reports whether one was written.
	// 参数 w（http.ResponseWriter）：调用方的 HTTP 响应，状态码和正文写在这里；r（*http.Request）：入站 HTTP 请求，用来读路径、头和正文；err（error）：失败原因，nil 表示这一步成功。
	// 返回 bool（bool）：已经把拒绝写进响应时返回真。对象不可见是 404，可见但不允许是 403，读归属失败是 500。
	// 调用：gateway/identity/handlers.go、gateway/identity/members.go、gateway/keys/admin.go、gateway/keys/generate.go
	// 测试：无直接单测
	WriteAuthz(w http.ResponseWriter, r *http.Request, err error) bool
	// WriteIAMError turns a store failure into the response and reports whether one was written.
	// 参数 w（http.ResponseWriter）：调用方的 HTTP 响应，状态码和正文写在这里；r（*http.Request）：入站 HTTP 请求，用来读路径、头和正文；err（error）：失败原因，nil 表示这一步成功。
	// 返回 bool（bool）：已经把存储错误写进响应时为真。err 为 nil 时为假，调用方继续用正常结果。
	// 调用：gateway/identity/handlers.go、gateway/identity/members.go
	// 测试：无直接单测
	WriteIAMError(w http.ResponseWriter, r *http.Request, err error) bool
}

// traceModule records that identity routes are being mounted.
// 参数 name（string）：正在挂载的模块名，只写进进程日志。
// 返回：无。只写进程日志，不写 HTTP 响应。
// 调用：identity 的 mount。
// 测试：无直接单测
func traceModule(name string) {
	logx.Trace("mount %s", name)
}
