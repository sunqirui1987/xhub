package usage

import (
	"net/http"

	"github.com/sunqirui1987/xhub/internal/httpx"
	"github.com/sunqirui1987/xhub/internal/logx"
)

// UsageAIChat is POST /usage/ai/chat. The panel reads either assistant text or this error. No upstream model is invented: with nothing configured, or no completion path, the body is an error.
// 参数 s（Host）：用量AIChat使用的数据面宿主；w（http.ResponseWriter）：调用方的 HTTP 响应，状态和正文写在这里；r（*http.Request）：入站 HTTP 请求。
// 返回：无。状态码和正文写进调用方的响应。
// 调用：gateway/usage/mount.go
// 测试：无直接单测
func UsageAIChat(s Host, w http.ResponseWriter, r *http.Request) {
	if s.RequireManage(w, r) == nil {
		return
	}
	_ = readMap(r)
	msg := "Usage AI could not complete the request"
	if len(s.ModelList()) == 0 {
		msg = "No model is configured for usage AI"
		logx.Error("usage ai chat path=%s reason=no model", r.URL.Path)
	} else {
		logx.Error("usage ai chat path=%s reason=unavailable", r.URL.Path)
	}
	httpx.WriteError(w, http.StatusBadRequest, "usage_ai_unavailable", msg)
}
