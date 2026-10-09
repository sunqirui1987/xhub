package dataplane

import (
	"bufio"
	"bytes"
	"fmt"
	"github.com/sunqirui1987/xhub/internal/provider"
	"io"
	"net/http"
	"strings"
)

// ForwardResponse 保存一次上游转发结果，流式正文已发送时不再写入 Body。
// StatusCode 保留上游状态；Header 在返回时按白名单过滤；Streamed 防止重复发送。
// Usage 只保存协议实测事实；缺失事实或价格维度用 pricing_blocked 表示。
type ForwardResponse struct {
	StatusCode int
	Header     http.Header
	Body       []byte
	Streamed   bool
	Usage      map[string]any
	ResponseID string // 成功原生流的存储响应归属，非流由正文读取。
}

// forwardOfficial 向登记的供应商地址发送一次原生请求并观察响应事实。
// 参数 h：数据面依赖；r：请求上下文；method/address/key：上游操作与凭据；body/in：原始载荷和头；transport：协议注册；w：可选流式写入器。
// 返回 ForwardResponse、error：状态、允许的响应头、原始正文和实测用量；网络、事件超限或流未完成返回错误。
// 不跟随重定向；调用方负责重试策略、任务固定和唯一结算。
// 调用：serveBypassCreate、serveBypassFollow。测试：native_bypass_test.go、fal_test.go。
func forwardOfficial(h Bypass, r *http.Request, method, address, key string, body []byte, in http.Header, transport provider.Transport, w http.ResponseWriter) (ForwardResponse, error) {
	var result ForwardResponse
	req, err := http.NewRequestWithContext(r.Context(), method, address, bytes.NewReader(body))
	if err != nil {
		return result, err
	}
	for k, values := range in {
		if !bypassHeader(k, in) {
			continue
		}
		for _, v := range values {
			req.Header.Add(k, v)
		}
	}
	req.Header.Set("Accept-Encoding", "identity")
	if transport.Auth.Header == "" {
		return result, fmt.Errorf("upstream authentication is not configured")
	}
	value := key
	if transport.Auth.Prefix != "" {
		value = transport.Auth.Prefix + " " + key
	}
	req.Header.Set(transport.Auth.Header, value)
	for k, v := range transport.Headers {
		if req.Header.Get(k) == "" {
			req.Header.Set(k, v)
		}
	}
	if body != nil && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}
	client := *h.HTTPClient()
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	if _, deadline := r.Context().Deadline(); deadline {
		client.Timeout = 0
	}
	resp, err := client.Do(req)
	if err != nil {
		return result, err
	}
	defer resp.Body.Close()
	result.StatusCode = resp.StatusCode
	result.Header = resp.Header.Clone()
	if w != nil && resp.StatusCode >= 200 && resp.StatusCode < 300 && strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		result.Streamed = true
		copyBypassResponseHeaders(w, result.Header)
		w.WriteHeader(resp.StatusCode)
		scanner := bufio.NewScanner(resp.Body)
		scanner.Split(sseLines)
		scanner.Buffer(make([]byte, 4096), 8<<20)
		var event bytes.Buffer
		var streamState provider.NativeStreamState
		request, _ := parseBypassBody(body, in.Get("Content-Type"))
		for scanner.Scan() {
			line := scanner.Bytes()
			if _, err = w.Write(line); err != nil {
				return result, err
			}
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
			if event.Len()+len(line) > 8<<20 {
				return result, fmt.Errorf("bypass SSE event exceeds 8 MiB")
			}
			event.Write(line)
			if bytes.Equal(line, []byte("\n")) || bytes.Equal(line, []byte("\r\n")) {
				streamState.Observe(transport.Protocol, event.Bytes())
				for k, v := range provider.NativeStreamUsage(transport.Protocol, event.Bytes(), request.Fields) {
					if result.Usage == nil {
						result.Usage = map[string]any{}
					}
					// 多张图片可能各自带用量和规格。当前结算是单规格账单，不能覆盖
					// 前一张的事实后按最后一张规格计价；保留事实并阻止自动结算。
					if transport.Protocol == "openai-images" && streamState.Images > 1 {
						result.Usage["pricing_blocked"] = "multiple_image_events_require_itemized_pricing"
					}
					if k == "pricing_blocked" && result.Usage[k] != nil {
						continue
					}
					result.Usage[k] = v
				}
				event.Reset()
			}
		}
		if err := scanner.Err(); err != nil {
			return result, err
		}
		if streamState.Failed || !streamState.Completed {
			return result, fmt.Errorf("native stream ended without successful completion")
		}
		result.ResponseID = streamState.ResponseID
		if result.Usage == nil {
			result.Usage = map[string]any{"pricing_blocked": "upstream_usage_missing"}
		}
		if transport.Protocol == "openai-images" {
			result.Usage["images"] = streamState.Images
		}
		return result, nil
	}
	result.Body, err = io.ReadAll(io.LimitReader(resp.Body, (64<<20)+1))
	if err == nil && len(result.Body) > 64<<20 {
		err = fmt.Errorf("bypass response exceeds 64 MiB")
	}
	return result, err
}

// sseLines 是保留原始换行的 Scanner 分割函数，避免 SSE 转发改变字节。
// 参数 data：缓冲区；atEOF：输入是否结束。返回：消费字节数、完整行、错误。
// 调用：forwardOfficial。测试：原生 SSE 字节保真测试。
func sseLines(data []byte, atEOF bool) (int, []byte, error) {
	if i := bytes.IndexByte(data, '\n'); i >= 0 {
		return i + 1, data[:i+1], nil
	}
	if atEOF && len(data) > 0 {
		return len(data), data, nil
	}
	return 0, nil, nil
}

// bypassHeader 判断请求头是否可透传，移除客户端凭据和逐跳传输字段。
// 参数 k：头名称；headers：http.Header，用于检查 Connection 声明的逐跳字段。
// 返回 bool：可转发时为 true。
// 调用：forwardOfficial。测试：TestNativeBypassJSONAndUsage。
func bypassHeader(k string, headers http.Header) bool {
	switch strings.ToLower(k) {
	case "x-goog-api-key", "authorization", "host", "content-length", "cookie", "set-cookie", "x-api-key", "api-key", "x-litellm-api-key", "connection", "proxy-connection", "keep-alive", "proxy-authorization", "proxy-authenticate", "te", "trailer", "transfer-encoding", "upgrade":
		return false
	}
	for _, named := range strings.Split(headers.Get("Connection"), ",") {
		if strings.EqualFold(strings.TrimSpace(named), k) {
			return false
		}
	}
	return true
}

// copyBypassResponseHeaders 把协议响应头白名单复制给客户端，避免泄露上游 Cookie。
// 参数 w：http.ResponseWriter；header：上游响应头。返回：无。
// 调用：forwardOfficial、writeThrough。测试：native_bypass_test.go。
func copyBypassResponseHeaders(w http.ResponseWriter, header http.Header) {
	for _, k := range []string{"Content-Type", "Content-Encoding", "Cache-Control", "X-Accel-Buffering", "Retry-After", "Request-Id", "X-Request-Id"} {
		if values := header.Values(k); len(values) > 0 {
			w.Header()[k] = append([]string(nil), values...)
		}
	}
}

// writeThrough 写入尚未流式发送的上游响应，保留状态码和原始响应体。
// 参数 w：客户端写入器；response：上游结果。返回：无；已流式发送的响应直接退出。
// 调用：原生数据面创建和查询。测试：native_bypass_test.go、fal_test.go。
func writeThrough(w http.ResponseWriter, response ForwardResponse) {
	if response.Streamed {
		return
	}
	copyBypassResponseHeaders(w, response.Header)
	status := response.StatusCode
	if status == 0 {
		status = http.StatusBadGateway
	}
	w.WriteHeader(status)
	_, _ = w.Write(response.Body)
}
