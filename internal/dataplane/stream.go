// stream.go copies an upstream SSE body to the client.
// pipeStream 保留原生事件；统一对话转换由 dialogue_stream.go 处理。
// The first byte sets TTFT. At most 2 MiB is kept for the usage log.

package dataplane

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/sunqirui1987/xhub/internal/logx"
)

// captureLimit 是一次流式响应最多保留给用量日志的字节数。超过这个量的正文
// 不再留档，但要记一行：截断之后按正文估算的 token 会偏低。
const captureLimit = 2 << 20

// noteCaptureTruncated 在保留的正文达到上限时记一行。静默截断会让用量偏低，
// 而偏低的原因在日志里看不出来。
// 参数 kept（int）：已经保留的字节数。
// 返回：无。只写进程日志。
// 调用：pipeStream 在拼接捕获正文时。
// 测试：无直接单测
func noteCaptureTruncated(kept int) {
	if kept >= captureLimit {
		logx.Debug("stream capture reached its limit kept=%d; token estimates from the body may run low", kept)
	}
}

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

// pipeStream 把上游 SSE 原样抄给客户端，并尽量从流里抽出 usage。一个字节都没写时 wrote 为 false。
// ttft 是 start 到首字节的时间。空正文时 ttft 保持 0。
// 参数 w：客户端响应。resp：上游响应，函数负责关闭 Body。start：请求开始时间。
// 返回 wrote、usage、ttft、captured：表示是否输出、使用量、首字节耗时和捕获正文，不改写原生事件。
// 调用：Serve 的普通流式路径。测试：failure_log_test.go TestServeLogsEmptyStreamAndUpstreamStatus、TestServeLogsCacheHitAndStreamMetrics。
func pipeStream(w http.ResponseWriter, resp *http.Response, start time.Time) (bool, map[string]any, time.Duration, []byte, error) {
	defer resp.Body.Close()
	buf := make([]byte, 4096)
	flusher, _ := w.(http.Flusher)
	wrote := false
	var ttft time.Duration
	var captured []byte
	parser := streamUsageParser{}
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
				noteCaptureTruncated(len(captured))
			}
			parser.write(buf[:n], false)
		}
		if err != nil {
			parser.write(nil, true)
			if err == io.EOF {
				err = nil
			}
			return wrote, parser.usage, ttft, captured, err
		}
	}
}

// streamUsageParser keeps the unfinished SSE line between body reads. Providers
// may split a data line at any byte, including in the middle of a JSON token.
type streamUsageParser struct {
	pending []byte
	usage   map[string]any
}

// write appends one body fragment and consumes every complete SSE line. final
// also consumes a final line without a newline, as permitted at EOF.
// 参数 raw：本次读取的正文；final：正文是否已经结束。
// 返回：无。解析出的用量合并进 p.usage，未完整的行保留在 p.pending。
// 调用：pipeStream。测试：usage_stream_test.go。
func (p *streamUsageParser) write(raw []byte, final bool) {
	p.pending = append(p.pending, raw...)
	for {
		i := bytes.IndexByte(p.pending, '\n')
		if i < 0 {
			break
		}
		p.consumeLine(p.pending[:i])
		p.pending = p.pending[i+1:]
	}
	if final && len(p.pending) > 0 {
		p.consumeLine(p.pending)
		p.pending = nil
	}
}

// consumeLine parses one complete SSE data line or one newline-delimited JSON
// object and merges any provider usage it contains.
// 参数 line：不含换行符的完整事件行。
// 返回：无。没有有效 JSON 或 usage 时保持原状态。
// 调用：streamUsageParser.write。测试：usage_stream_test.go。
func (p *streamUsageParser) consumeLine(line []byte) {
	line = bytes.TrimSpace(line)
	if bytes.HasPrefix(line, []byte("data:")) {
		line = bytes.TrimSpace(bytes.TrimPrefix(line, []byte("data:")))
	}
	if len(line) == 0 || bytes.Equal(line, []byte("[DONE]")) {
		return
	}
	var doc map[string]any
	if json.Unmarshal(line, &doc) == nil {
		p.usage = mergeUsage(p.usage, usageFromDocument(doc))
	}
}

// streamUsage 在新到达的 SSE 字节里查找 usage，没有新用量时沿用上一次的结果。
// 参数 raw：本次要扫描的字节。prev：上一次找到的 usage，可为 nil。
// 返回：更新后的 usage。从未出现过时返回 prev，可能仍是 nil。
// 调用：pipeStream。无单独测试。
func streamUsage(raw []byte, prev map[string]any) map[string]any {
	p := streamUsageParser{usage: prev}
	p.write(raw, true)
	return p.usage
}
