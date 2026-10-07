// stream.go copies an upstream SSE body to the client.
// pipeStream forwards the bytes unchanged. pipeResponsesAsChat rewrites a
// Responses stream into chat chunks when the public operation is chat.
// The first byte sets TTFT. At most 2 MiB is kept for the usage log.

package dataplane

import (
	"bytes"
	"encoding/json"
	"net/http"
	"time"

	"github.com/sunqirui1987/xhub/internal/llm"
)

// responseID 从上游 JSON 或 SSE 里取出第一个响应 id，用来把后续对话钉回同一部署。
// 参数 raw：上游响应或捕获的 SSE。
// 返回：第一个非空 id。没有时返回空串，CommitRoute 就不写响应钉。
// 调用：Serve 的流式和非流式成功路径。无单独测试。
func responseID(raw []byte) string {
	var doc map[string]any
	if json.Unmarshal(raw, &doc) == nil {
		if id, ok := doc["id"].(string); ok {
			return id
		}
	}
	for _, line := range bytes.Split(raw, []byte("\n")) {
		line = bytes.TrimSpace(bytes.TrimPrefix(bytes.TrimSpace(line), []byte("data:")))
		if len(line) == 0 || bytes.Equal(line, []byte("[DONE]")) {
			continue
		}
		if json.Unmarshal(line, &doc) != nil {
			continue
		}
		if id, ok := doc["id"].(string); ok && id != "" {
			return id
		}
	}
	return ""
}

// pipeResponsesAsChat 把七牛 Bypass 的 Responses 流转成 chat completion chunk。
// 错误状态原样转发，调用方仍能看到供应商的报错。
//
// 参数 w：客户端响应。resp：上游响应，函数负责关闭 Body。
// 参数 start：请求开始时间，用来算首字节。model：写进转换后 chunk 的模型名。
// 返回 wrote：是否已经向 w 写过字节。usage：流里最后一次 usage。ttft：首字节耗时，没写过则为 0。
// 返回 captured：最多 2 MiB，交给用量日志。
// 调用：Serve 在操作是 chat 且上游协议是 responses 时。现有流式测试走 pipeStream，不走这条转换。
func pipeResponsesAsChat(w http.ResponseWriter, resp *http.Response, start time.Time, model string) (bool, map[string]any, time.Duration, []byte) {
	defer resp.Body.Close()
	buf := make([]byte, 4096)
	flusher, _ := w.(http.Flusher)
	wrote := false
	var ttft time.Duration
	var pending []byte
	var captured []byte
	var usage map[string]any
	write := func(chunk []byte) {
		if len(chunk) == 0 {
			return
		}
		if !wrote {
			ttft = time.Since(start)
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(resp.StatusCode)
			wrote = true
		}
		_, _ = w.Write(chunk)
		if flusher != nil {
			flusher.Flush()
		}
		if len(captured) < 2<<20 {
			take := len(chunk)
			if len(captured)+take > 2<<20 {
				take = (2 << 20) - len(captured)
			}
			captured = append(captured, chunk[:take]...)
		}
		usage = streamUsage(chunk, usage)
	}
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			pending = append(pending, buf[:n]...)
			emit, rest := llm.ResponsesSSEToChat(pending, model, false)
			pending = rest
			write(emit)
		}
		if err != nil {
			emit, _ := llm.ResponsesSSEToChat(pending, model, true)
			write(emit)
			return wrote, usage, ttft, captured
		}
	}
}

// pipeStream 把上游 SSE 原样抄给客户端，并尽量从流里抽出 usage。一个字节都没写时 wrote 为 false。
// ttft 是 start 到首字节的时间。空正文时 ttft 保持 0。
// 参数 w：客户端响应。resp：上游响应，函数负责关闭 Body。start：请求开始时间。
// 返回 wrote、usage、ttft、captured：含义与 pipeResponsesAsChat 相同，但不改写 chunk。
// 调用：Serve 的普通流式路径。测试：failure_log_test.go TestServeLogsEmptyStreamAndUpstreamStatus、TestServeLogsCacheHitAndStreamMetrics。
func pipeStream(w http.ResponseWriter, resp *http.Response, start time.Time) (bool, map[string]any, time.Duration, []byte) {
	defer resp.Body.Close()
	buf := make([]byte, 4096)
	flusher, _ := w.(http.Flusher)
	wrote := false
	var ttft time.Duration
	var pending []byte
	var captured []byte
	var usage map[string]any
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			if !wrote {
				ttft = time.Since(start)
				w.Header().Set("Content-Type", "text/event-stream")
				w.WriteHeader(resp.StatusCode)
				wrote = true
			}
			_, _ = w.Write(buf[:n])
			if flusher != nil {
				flusher.Flush()
			}
			if len(captured) < 2<<20 {
				take := n
				if len(captured)+take > 2<<20 {
					take = (2 << 20) - len(captured)
				}
				captured = append(captured, buf[:take]...)
			}
			pending = append(pending, buf[:n]...)
			usage = streamUsage(pending, usage)
			if len(pending) > 1<<20 {
				pending = pending[len(pending)-4096:]
			}
		}
		if err != nil {
			return wrote, usage, ttft, captured
		}
	}
}

// streamUsage 在新到达的 SSE 字节里查找 usage，没有新用量时沿用上一次的结果。
// 参数 raw：本次要扫描的字节。prev：上一次找到的 usage，可为 nil。
// 返回：更新后的 usage。从未出现过时返回 prev，可能仍是 nil。
// 调用：pipeStream、pipeResponsesAsChat。无单独测试。
func streamUsage(raw []byte, prev map[string]any) map[string]any {
	for _, line := range bytes.Split(raw, []byte("\n")) {
		line = bytes.TrimSpace(line)
		line = bytes.TrimPrefix(line, []byte("data:"))
		line = bytes.TrimSpace(line)
		if len(line) == 0 || bytes.Equal(line, []byte("[DONE]")) {
			continue
		}
		var doc map[string]any
		if json.Unmarshal(line, &doc) != nil {
			continue
		}
		if u, ok := doc["usage"].(map[string]any); ok {
			prev = u
		}
	}
	return prev
}
