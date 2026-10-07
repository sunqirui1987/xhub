// bypass.go matches official provider paths before Gin sees the request.
// Adapted endpoint types (chat, embedding, images) return false here and
// stay on the catalog handlers. A match calls dataplane.ServeBypass, which
// records spend through the same RecordSpend path as chat.

package gateway

import (
	"net/http"

	"github.com/sunqirui1987/xhub/internal/dataplane"
	"github.com/sunqirui1987/xhub/internal/httpx"
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

// serveBypass handles a request whose path is a bypass endpoint type or a custom endpoint saved on a deployment. Other paths return false.
// 参数 w（http.ResponseWriter）：调用方的 HTTP 响应，状态码和正文写在这里；r（*http.Request）：入站 HTTP 请求，用来读路径、头和正文。
// 返回 bool（bool）：这条请求已经按官方接口处理并写完响应时为真。不是官方路径时为假，继续走后面的路由。
// 调用：gateway/engine.go
// 测试：无直接单测
func (s *Server) serveBypass(w http.ResponseWriter, r *http.Request) bool {
	if s == nil || s.Cfg == nil {
		return false
	}
	hit, ok := provider.Match(r.Method, r.URL.Path, s.Models())
	if !ok || hit.Type.Kind != provider.KindBypass {
		return false
	}
	dataplane.ServeBypass(s, w, r, hit)
	return true
}
