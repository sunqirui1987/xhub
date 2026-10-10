package gateway

import (
	"encoding/json"
	"net/http"

	"github.com/sunqirui1987/xhub/internal/dataplane"
)

// upstreamDiagnosticJSON 将传输事实和计费统计合并为持久化 JSON。
// 参数 upstream 为原始诊断，可为 nil；usage 为实际计费输入；返回 JSON 或空字符串。
// 调用：recordSpend；复制响应头后打码凭据，不修改原始对象，不保存生成内容。
func upstreamDiagnosticJSON(upstream *dataplane.UpstreamDiagnostics, usage map[string]any) string {
	if upstream == nil {
		return ""
	}
	saved := *upstream
	saved.Headers = diagnosticHeaders(upstream.Headers)
	saved.Trailers = diagnosticHeaders(upstream.Trailers)
	saved.BillingUsage = usage
	raw, err := json.Marshal(saved)
	if err != nil {
		return ""
	}
	return string(raw)
}

// diagnosticHeaders 克隆并脱敏完整多值响应头；参数 headers 为上游头，返回独立副本。
// 调用：诊断持久化；保留全部名称、值数量与非敏感值，密钥和 Cookie 替换为星号。
func diagnosticHeaders(headers http.Header) http.Header {
	result := headers.Clone()
	for key, values := range result {
		for i, value := range values {
			if sensitiveHeader(key) {
				values[i] = "***"
			} else {
				values[i] = redactSecretText(value)
			}
		}
	}
	return result
}
