// bypass.go matches registered pass-through paths before Gin sees the request.
// Adapted endpoints (chat, embedding, images) return false here and stay on the
// catalog handlers. A match calls dataplane.ServeBypass, which records spend
// through the same RecordSpend path as chat.
//
// The only source of a pass-through is the registry. A deployment cannot carry
// its own path table: a hand-filled table has no structured upstream response,
// so preselect, logging and usage would have nothing to read.

package gateway

import (
	"net/http"

	"github.com/sunqirui1987/xhub/internal/dataplane"
	"github.com/sunqirui1987/xhub/internal/httpx"
	"github.com/sunqirui1987/xhub/internal/logx"
	"github.com/sunqirui1987/xhub/internal/provider"
)

// 返回已登记的端点类型，供调试时看模型和端点如何对应。
// 参数 w（http.ResponseWriter）：调用方的 HTTP 响应，状态码和正文写在这里；r（*http.Request）：入站 HTTP 请求，用来读路径、头和正文。
// 返回：无。状态码和正文写进调用方的响应。
// 调用：仅在 bypass.go 内使用
// 测试：无直接单测
func (s *Server) publicEndpoints(w http.ResponseWriter, r *http.Request) {
	httpx.WriteJSON(w, http.StatusOK, provider.PublicBody())
}

// serveBypass 处理已由 provider 目录登记的 bypass 公开路径。
//
// bypass 的路径、上游动作、任务字段和用量规则都来自已登记 transport；
// 部署配置中的自定义 endpoint 不参与匹配。这样模型编辑器、日志和结算
// 使用同一份结构化事实，不会出现页面能保存但数据面无法执行的声明。
// 参数 w（http.ResponseWriter）：调用方的 HTTP 响应，状态码和正文写在这里；r（*http.Request）：入站 HTTP 请求，用来读路径、头和正文。
// 返回 bool（bool）：这条请求已经按官方接口处理并写完响应时为真。不是官方路径时为假，继续走后面的路由。
// 调用：gateway/engine.go
// 测试：无直接单测
func (s *Server) serveBypass(w http.ResponseWriter, r *http.Request) bool {
	if s == nil || s.Cfg == nil {
		return false
	}
	hit, ok := provider.Match(r.Method, r.URL.Path, s.Models())
	if !ok || hit.Transport.Kind != provider.KindBypass {
		return false
	}
	// bypass 是真实上游调用，也会产生真实费用；记录命中的 transport 和动作，
	// 便于把请求、任务结算和费用明细对应起来。适配路径由自己的循环记录。
	logx.Debug("process %s %s step=bypass transport=%s action=%s", r.Method, r.URL.Path, hit.Transport.ID, hit.Action.Name)
	dataplane.ServeBypass(s, w, r, hit)
	return true
}
