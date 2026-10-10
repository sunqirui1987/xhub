package dataplane

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

// UpstreamDiagnostics 保存协议转换前的响应事实，与响应缓存及计费用量独立。
// Headers 保留所有名称和多值；UsageReported 区分未上报与明确的零值。
type UpstreamDiagnostics struct {
	StatusCode    int            `json:"status_code"`
	Headers       http.Header    `json:"headers"`
	Trailers      http.Header    `json:"trailers,omitempty"`
	Usage         map[string]any `json:"usage"`
	UsageReported bool           `json:"usage_reported"`
	ParseError    string         `json:"usage_parse_error,omitempty"`
	BillingUsage  map[string]any `json:"billing_usage,omitempty"`
}

// upstreamObserver 透明观察读取的字节，不改变读取结果。
// JSON 最多暂存 64 MiB，SSE 每个事件最多 8 MiB；超限仅标记诊断不完整。
type upstreamObserver struct {
	io.ReadCloser
	response    *http.Response
	diagnostics *UpstreamDiagnostics
	stream      bool
	pending     []byte
	event       []byte
	dropping    bool
	finished    bool
}

// observeUpstream 在任何读取或协议转换前安装观察器。
// 参数 resp 为 HTTP 响应；返回随 Body 读取更新的共享指针。
// 调用：适配及原生转发；不修改响应头，不负责关闭正文。
func observeUpstream(resp *http.Response) *UpstreamDiagnostics {
	diagnostics := &UpstreamDiagnostics{StatusCode: resp.StatusCode, Headers: resp.Header.Clone()}
	resp.Body = &upstreamObserver{ReadCloser: resp.Body, response: resp, diagnostics: diagnostics,
		stream: strings.HasPrefix(strings.ToLower(resp.Header.Get("Content-Type")), "text/event-stream")}
	return diagnostics
}

// Read 原样返回底层读取结果，同时提取完整 JSON 或 SSE 事件的用量。
// 参数 p 为缓冲区；返回原始 n、err。EOF 时处理最后一行及 trailers。
// 错误和超限不影响传输；未读完时不伪造完整统计。
func (o *upstreamObserver) Read(p []byte) (int, error) {
	n, err := o.ReadCloser.Read(p)
	if o.stream {
		for _, b := range p[:n] {
			if b == 10 {
				o.line(o.pending)
				o.pending = nil
			} else if len(o.pending) < 8<<20 {
				o.pending = append(o.pending, b)
			} else {
				o.diagnostics.ParseError = "upstream SSE event exceeds 8 MiB"
				o.dropping = true
			}
		}
	} else if !o.dropping {
		if len(o.pending)+n > 64<<20 {
			o.pending = nil
			o.dropping = true
			o.diagnostics.ParseError = "upstream JSON exceeds 64 MiB"
		} else {
			o.pending = append(o.pending, p[:n]...)
		}
	}
	if err != nil && !o.finished {
		o.finished = true
		if err == io.EOF {
			if o.stream {
				if len(o.pending) > 0 {
					o.line(o.pending)
				}
				o.line(nil)
			} else if !o.dropping {
				o.document(o.pending)
			}
			o.diagnostics.Trailers = o.response.Trailer.Clone()
		} else {
			o.diagnostics.ParseError = "upstream body read failed"
		}
	}
	return n, err
}

// Close 原样关闭上游正文并标记未读到 EOF 的诊断；无参数，返回底层关闭错误。
// 由转发器清理调用，已提取用量仍保留，提前取消不能误报为完整观测。
func (o *upstreamObserver) Close() error {
	if !o.finished && o.diagnostics.ParseError == "" {
		o.diagnostics.ParseError = "upstream body closed before EOF"
	}
	return o.ReadCloser.Close()
}

// line 组装 SSE 多行 data；参数为无换行的行，无返回值。
// 由 Read 调用，空行提交事件，超限后在下一事件继续观察。
func (o *upstreamObserver) line(line []byte) {
	line = bytes.TrimSuffix(line, []byte{13})
	if len(line) == 0 {
		if !o.dropping && len(o.event) > 0 {
			o.document(o.event)
		}
		o.event = nil
		o.dropping = false
		return
	}
	if bytes.HasPrefix(line, []byte("data:")) && !o.dropping {
		data := bytes.TrimPrefix(line[5:], []byte(" "))
		if len(o.event)+len(data)+1 > 8<<20 {
			o.dropping = true
			o.diagnostics.ParseError = "upstream SSE event exceeds 8 MiB"
			o.event = nil
			return
		}
		o.event = append(o.event, data...)
		o.event = append(o.event, 10)
	}
}

// document 解析完整正文或事件 raw，无返回值；由观察器调用。
// 保留扩展字段、数值精度及嵌套统计，忽略 DONE 和无统计的正常事件。
func (o *upstreamObserver) document(raw []byte) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("[DONE]")) {
		return
	}
	var doc map[string]any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&doc); err != nil {
		o.diagnostics.ParseError = "upstream response is not valid JSON"
		return
	}
	o.usage(doc)
}

// usage 提取统计及 Responses、Anthropic 包装对象；参数 doc 为解码文档。
// 由 document 递归调用，无返回值，不改写供应商字段或 token 数值。
func (o *upstreamObserver) usage(doc map[string]any) {
	for _, key := range []string{"usage", "usageMetadata"} {
		if usage, ok := doc[key].(map[string]any); ok {
			o.diagnostics.UsageReported = true
			o.diagnostics.Usage = mergeDiagnosticUsage(o.diagnostics.Usage, usage)
		}
	}
	for _, key := range []string{"response", "message"} {
		if nested, ok := doc[key].(map[string]any); ok {
			o.usage(nested)
		}
	}
}

// mergeDiagnosticUsage 合并 SSE 分段统计；参数 dst、src 为原始字段，返回合并对象。
// 由 usage 调用，递归保留空对象、零值和扩展字段；后续事件覆盖同名标量。
func mergeDiagnosticUsage(dst, src map[string]any) map[string]any {
	if dst == nil {
		dst = map[string]any{}
	}
	for key, value := range src {
		if nested, ok := value.(map[string]any); ok {
			existing, _ := dst[key].(map[string]any)
			dst[key] = mergeDiagnosticUsage(existing, nested)
		} else {
			dst[key] = value
		}
	}
	return dst
}
