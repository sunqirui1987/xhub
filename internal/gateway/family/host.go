// Package family shapes catalog resources that do not have their own handler. Inference operations go to the data plane. The rest are key-value reads and writes.
package family

import (
	"net/http"

	"github.com/sunqirui1987/xhub/internal/auth"
	"github.com/sunqirui1987/xhub/internal/logx"
	"github.com/sunqirui1987/xhub/internal/store"
)

// ProxyVersion is written into the catalog config resource and returned as litellm_version by the process health details.
const ProxyVersion = "xhub-dev"

// Host is what catalog resource handlers ask the process for. *gateway.Server implements it. This package does not import gateway.
type Host interface {
	// 要求当前请求可以发起推理。失败时已经写好响应并返回 nil。
	// 参数 w（http.ResponseWriter）：调用方的 HTTP 响应，状态码和正文写在这里；r（*http.Request）：入站 HTTP 请求，用来读路径、头和正文。
	// 返回 *auth.Principal（*auth.Principal）：已经解析的调用方，含用户、团队和密钥。
	// 调用：gateway/family/handlers.go、gateway/wire.go
	// 测试：无直接单测
	RequireLLM(w http.ResponseWriter, r *http.Request) *auth.Principal
	// 要求当前请求具备控制台或推理权限。失败时已经写好响应并返回 nil。
	// 参数 w（http.ResponseWriter）：调用方的 HTTP 响应，状态码和正文写在这里；r（*http.Request）：入站 HTTP 请求，用来读路径、头和正文。
	// 返回 *auth.Principal（*auth.Principal）：已经解析的调用方，含用户、团队和密钥。
	// 调用：gateway/family/handlers.go、gateway/keys/admin.go、gateway/keys/host.go、gateway/wire.go
	// 测试：无直接单测
	RequireMixed(w http.ResponseWriter, r *http.Request) *auth.Principal
	// 要求当前请求具备管理权限。失败时已经写好响应并返回 nil。
	// 参数 w（http.ResponseWriter）：调用方的 HTTP 响应，状态码和正文写在这里；r（*http.Request）：入站 HTTP 请求，用来读路径、头和正文。
	// 返回 *auth.Principal（*auth.Principal）：已经解析的调用方，含用户、团队和密钥。
	// 调用：gateway/family/handlers.go、gateway/guard/guard.go、gateway/guard/host.go、gateway/identity/gate.go
	// 测试：guard_test.go
	RequireManage(w http.ResponseWriter, r *http.Request) *auth.Principal
	// RecordStore holds the catalog key-value records: guardrails, prompts, skills, credentials and the rest. It answers no authorization question.
	// 参数：无。
	// 返回 *store.Store（*store.Store）：目录用的键值库，存护栏、提示词、技能和凭据。它不回答鉴权问题。没有库时为 nil。
	// 调用：gateway/family/handlers.go、gateway/guard/guard.go、gateway/guard/host.go、gateway/models/admin.go
	// 测试：guard_test.go
	RecordStore() *store.Store
	// DataPlane hands an already recognized inference operation to the upstream loop. Do not call it when op is empty.
	// 参数 w（http.ResponseWriter）：调用方的 HTTP 响应，状态码和正文写在这里；r（*http.Request）：入站 HTTP 请求，用来读路径、头和正文；op（string）：操作名或 call_type，写入用量行并选择协议。
	// 调用：gateway/family/handlers.go、gateway/wire.go
	// 测试：无直接单测
	// 返回：无。推理的状态码和正文由数据面写进响应。
	DataPlane(w http.ResponseWriter, r *http.Request, op string)
	// EnforceIdentityLimits checks the model allow-list, budget, and rate. On rejection it has already written the response and returns false.
	// 参数 w（http.ResponseWriter）：拒绝时错误写在这里；path（string）：请求路径，用来选择错误包络；p（*auth.Principal）：已经鉴权的调用方；alias（string）：对外模型名，用来对允许列表和预算；est（int）：预估的 token 数，用来做 TPM 预检。
	// 返回 bool（bool）：限额通过时为真。拒绝时已经写好 403 或 429，并返回假。
	// 调用：gateway/family/handlers.go、dataplane。
	// 测试：无直接单测
	EnforceIdentityLimits(w http.ResponseWriter, path string, p *auth.Principal, alias string, est int) bool
}

// traceModule records that catalog resource routes are being mounted.
// 参数 name（string）：正在挂载的模块名，只写进进程日志。
// 返回：无。只写进程日志，不写 HTTP 响应。
// 调用：family 的 mount。
// 测试：无直接单测
func traceModule(name string) {
	logx.Trace("mount %s", name)
}
