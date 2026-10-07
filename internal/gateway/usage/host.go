// Package usage serves spend reports, usage summaries and health probes. Every
// number comes from the usage_events and usage_daily tables in PostgreSQL;
// nothing is recomputed from a log file at request time.
//
// This package never decides who may read what. It asks the process for a scope
// (authz.UsageScope for usage, authz.LogsScope for request logs) and hands that
// scope to iam, which applies it as a WHERE fragment. A handler that forgot to
// ask for a scope would have nothing to pass, because iam takes the scope as a
// required argument rather than a default.
package usage

import (
	"net/http"

	"github.com/sunqirui1987/xhub/internal/auth"
	"github.com/sunqirui1987/xhub/internal/authz"
	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/iam"
	"github.com/sunqirui1987/xhub/internal/logx"
)

// Host is what the usage handlers ask the process for. *gateway.Server implements
// it. This package does not import gateway.
type Host interface {
	// RequireManage requires a platform administrator session. The global spend reports use it: they are a platform-wide view by definition.
	// 参数 w（http.ResponseWriter）：调用方的 HTTP 响应，状态码和正文写在这里；r（*http.Request）：入站 HTTP 请求，用来读路径、头和正文。
	// 返回 *auth.Principal（*auth.Principal）：已经解析的调用方，含用户、团队和密钥。
	// 调用：gateway/family/handlers.go、gateway/family/host.go、gateway/guard/guard.go、gateway/guard/host.go
	// 测试：guard_test.go
	RequireManage(w http.ResponseWriter, r *http.Request) *auth.Principal
	// RequireUser accepts any signed-in caller. The per-user and per-team summaries use it, then narrow the rows with a scope.
	// 参数 w（http.ResponseWriter）：调用方的 HTTP 响应，状态码和正文写在这里；r（*http.Request）：入站 HTTP 请求，用来读路径、头和正文。
	// 返回 *auth.Principal（*auth.Principal）：已经解析的调用方，含用户、团队和密钥。
	// 调用：gateway/identity/gate.go、gateway/identity/handlers.go、gateway/identity/members.go、gateway/keys/admin.go
	// 测试：无直接单测
	RequireUser(w http.ResponseWriter, r *http.Request) *auth.Principal
	// Identity is the usage and key store. A handler reads from it only with a scope it obtained from one of the two methods below.
	// 参数：无。
	// 返回 *iam.DB（*iam.DB）：用量和密钥所在的库。处理函数只能带着已经算好的 scope 来读。没有 PostgreSQL 时为 nil。
	// 调用：dataplane/host.go、dataplane/live.go、gateway/identity/gate.go、gateway/identity/handlers.go
	// 测试：bypass_logic_test.go、failure_log_test.go
	Identity() *iam.DB
	// UsageScope narrows a usage listing to what the caller may see, optionally limited to one team. It is fail-closed: a caller who may see nothing receives a scope that matches no row.
	// 参数 r（*http.Request）：入站 HTTP 请求，用来读路径、头和正文；p（*auth.Principal）：已经解析的调用方，含用户、团队和密钥；teamID（string）：团队 id。空串表示没有指定团队。
	// 返回 *authz.Scope（*authz.Scope）：用量列表的范围，可以再收窄到一个团队。谁都看不见时是恒假条件，不是空列表；error（error）：无权看这个团队。nil 表示范围已经定好。
	// 调用：authz/decide.go、gateway/usage/activity.go、gateway/usage/entity_activity.go、gateway/usage/reports.go
	// 测试：authz_test.go
	UsageScope(r *http.Request, p *auth.Principal, teamID string) (*authz.Scope, error)
	// LogsScope narrows a request-log listing: the caller's own personal logs, plus the service-key logs of the teams they administer.
	// 参数 r（*http.Request）：入站 HTTP 请求，用来读路径、头和正文；p（*auth.Principal）：已经解析的调用方，含用户、团队和密钥。
	// 返回 *authz.Scope（*authz.Scope）：请求日志的范围。本人的个人日志，加上所管理团队的服务密钥日志；error（error）：身份还不能列日志。nil 表示范围已经定好。
	// 调用：authz/decide.go、gateway/usage/reports.go、gateway/wire.go
	// 测试：authz_test.go
	LogsScope(r *http.Request, p *auth.Principal) (*authz.Scope, error)
	// WriteAuthz turns a refused scope into the response and reports whether one was written. A scope failure is answered rather than ignored, because a failed scope is not an empty listing.
	// 参数 w（http.ResponseWriter）：调用方的 HTTP 响应，状态码和正文写在这里；r（*http.Request）：入站 HTTP 请求，用来读路径、头和正文；err（error）：失败原因，nil 表示这一步成功。
	// 返回 bool（bool）：已经把拒绝写进响应时返回真。对象不可见是 404，可见但不允许是 403，读归属失败是 500。
	// 调用：gateway/identity/gate.go、gateway/identity/handlers.go、gateway/identity/members.go、gateway/keys/admin.go
	// 测试：无直接单测
	WriteAuthz(w http.ResponseWriter, r *http.Request, err error) bool
	// WriteIAMError turns a store failure into the response and reports whether one was written.
	// 参数 w（http.ResponseWriter）：调用方的 HTTP 响应，状态码和正文写在这里；r（*http.Request）：入站 HTTP 请求，用来读路径、头和正文；err（error）：失败原因，nil 表示这一步成功。
	// 返回 bool（bool）：已经把存储失败写进响应时返回真。
	// 调用：gateway/identity/gate.go、gateway/identity/handlers.go、gateway/identity/members.go、gateway/keys/admin.go
	// 测试：无直接单测
	WriteIAMError(w http.ResponseWriter, r *http.Request, err error) bool
	// AuditLogRead records that a caller read a request log they do not own. Only a platform administrator reading someone else's content produces a row; reading your own log is not an audited event.
	// 参数 r（*http.Request）：入站 HTTP 请求；p（*auth.Principal）：已经鉴权的调用方，含用户、团队、密钥哈希和别名；requestID（string）：审计日志Read使用的请求标识。空串表示调用方没有提供这项；e（iam.UsageEvent）：审计日志Read使用的用量事件。
	// 返回 error（error）：失败原因，nil 表示这一步成功。
	// 调用：gateway/usage/reports.go、gateway/wire.go、iam/usage_read.go
	// 测试：无直接单测
	AuditLogRead(r *http.Request, p *auth.Principal, requestID string, e iam.UsageEvent) error
	// ModelList returns a copy of the current model table. Benchmarks use it only to find strategy-router names. They do not invent scores when there is no sample.
	// 参数：无。
	// 返回 []config.ModelEntry（[]config.ModelEntry）：当前模型表的副本。没有部署时为空切片。
	// 调用：gateway/usage/benchmark.go
	// 测试：无直接单测
	ModelList() []config.ModelEntry
}

// traceModule records that usage routes are being mounted.
// 参数 name（string）：正在挂载的模块名，只写进进程日志。
// 返回：无。只写进程日志，不写 HTTP 响应。
// 调用：usage 的 mount。
// 测试：无直接单测
func traceModule(name string) {
	logx.Trace("mount %s", name)
}
