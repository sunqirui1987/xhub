// usage.go estimates tokens when the upstream omits a usage object.
// EstimateTokens is an upper bound for budget and TPM, not a tokenizer.
// completeUsage fills prompt and completion counts from that estimate so a
// stream without a usage object is not billed as zero output.

package dataplane

import (
	"bytes"
	"encoding/json"
	"strings"
	"time"
)

// ttftMillis 把一段耗时收成日志用的毫秒指针。d 小于等于 0 时返回 nil，表示没量到。
// 大于 0 但不足 1 毫秒时记 1，避免整次很快的调用在日志里变成空。
//
// 参数 d：从请求开始到首字节，或到整段同步响应结束。
// 返回：可写入 CallNote.TTFTMs 的毫秒数；nil 表示不写这一列。
// 调用：Serve 的成功路径、dataplaneNote。测试：prefer_test.go TestTTFTMillisOmitsAnUnmeasuredDelay。
func ttftMillis(d time.Duration) *int {
	if d <= 0 {
		return nil
	}
	ms := int(d.Milliseconds())
	if ms < 1 {
		ms = 1
	}
	return &ms
}

// EstimateTokens 用正文长度估一个 token 上界。它不是分词器，只给预算和 TPM 一个扣留数。
//
// 参数 body：已解析的请求 JSON。读 max_tokens、messages、prompt、input。
// 返回：至少 32，加上 max_tokens（没有则 64）和文本长度除以 4。
// 调用：Serve、ServeBypass、gateway estimateTokens。没有单独的表驱动测试。
func EstimateTokens(body map[string]any) int {
	n := 32
	if mt := asInt(body["max_tokens"]); mt > 0 {
		n += mt
	} else {
		n += 64
	}
	switch t := body["messages"].(type) {
	case []any:
		for _, m := range t {
			if mm, ok := m.(map[string]any); ok {
				n += len(textOf(mm["content"])) / 4
			}
		}
	}
	if p, ok := body["prompt"].(string); ok {
		n += len(p) / 4
	}
	if p, ok := body["input"].(string); ok {
		n += len(p) / 4
	}
	return n
}

// textOf 把 v 当成字符串读出。不是字符串时返回空串，不报错。
//
// 参数 v：messages 里的 content，类型不固定。
// 返回：字符串内容，或空串。
// 调用：EstimateTokens。无单独测试。
func textOf(v any) string {
	s, _ := v.(string)
	return s
}

// asInt 把 JSON 数字转成 int。float64 直接截断。其它类型返回 0。
//
// 参数 v：encoding/json 解出的数字，常见是 float64。
// 返回：截断后的整数；无法识别时为 0。
// 调用：EstimateTokens、usageCounts、live.go 的 RecordFailure。无单独测试。
func asInt(v any) int {
	switch t := v.(type) {
	case float64:
		return int(t)
	case int:
		return t
	default:
		return 0
	}
}

// completeUsage 补齐用量里缺失的 prompt_tokens 和 completion_tokens，保证返回的映射不是 nil。
// 参数 streamed：已转发给客户端的 SSE 字节。
// 返回：保证非 nil 的 usage。缺的 prompt_tokens、completion_tokens 被填上。
// 调用：Serve 的流式成功路径。输出侧由 prefer_test.go TestOutputTokensCountsStreamedTextWhenUsageIsMissing 间接覆盖。
// 测试：prefer_test.go TestOutputTokensCountsStreamedTextWhenUsageIsMissing
func completeUsage(usage map[string]any, body map[string]any, streamed []byte) map[string]any {
	if usage == nil {
		usage = map[string]any{}
	}
	pt, ct := usageCounts(usage)
	if pt == 0 {
		if n := EstimateTokens(body); n > 0 {
			usage["prompt_tokens"] = n
		}
	}
	if ct == 0 {
		if n := outputTokens(streamed); n > 0 {
			usage["completion_tokens"] = n
		}
	}
	return usage
}

// outputTokens 从 SSE 里的 delta.content 和 response.output_text.delta 估算完成 token。
// 工具参数文本不计入。没有正文时返回 0。有正文时至少返回 1。
//
// 参数 raw：捕获的 SSE。
// 返回：字符数除以 4。
// 调用：completeUsage。测试：prefer_test.go TestOutputTokensCountsStreamedTextWhenUsageIsMissing。
func outputTokens(raw []byte) int {
	var b strings.Builder
	for _, line := range bytes.Split(raw, []byte("\n")) {
		line = bytes.TrimSpace(bytes.TrimPrefix(bytes.TrimSpace(line), []byte("data:")))
		if len(line) == 0 || bytes.Equal(line, []byte("[DONE]")) {
			continue
		}
		var doc map[string]any
		if json.Unmarshal(line, &doc) != nil {
			continue
		}
		if choices, ok := doc["choices"].([]any); ok && len(choices) > 0 {
			choice, _ := choices[0].(map[string]any)
			delta, _ := choice["delta"].(map[string]any)
			if part, ok := delta["content"].(string); ok {
				b.WriteString(part)
			}
		}
		if doc["type"] == "response.output_text.delta" {
			if part, ok := doc["delta"].(string); ok {
				b.WriteString(part)
			}
		}
	}
	if b.Len() == 0 {
		return 0
	}
	n := b.Len() / 4
	if n < 1 {
		n = 1
	}
	return n
}

// usageCounts 读 prompt 与 completion。没有 OpenAI 字段名时接受 input_tokens 和 output_tokens。
//
// 参数 usage：上游 usage 对象，可为 nil。
// 返回 pt、ct：提示 token 和完成 token。无法读取时都是 0。
// 调用：completeUsage、bodyUsage。无单独测试。
func usageCounts(usage map[string]any) (pt, ct int) {
	if usage == nil {
		return 0, 0
	}
	pt = asInt(usage["prompt_tokens"])
	if pt == 0 {
		pt = asInt(usage["input_tokens"])
	}
	ct = asInt(usage["completion_tokens"])
	if ct == 0 {
		ct = asInt(usage["output_tokens"])
	}
	return pt, ct
}

// bodyUsage 从一段 JSON 响应里读 usage。没有 usage 或不是 JSON 时两个计数都是 0。
//
// 参数 raw：上游非流式响应体。
// 返回 pt、ct：提示 token 和完成 token。
// 调用：Serve 在非流式成功之后记指标。无单独测试。
func bodyUsage(raw []byte) (pt, ct int) {
	var doc map[string]any
	if json.Unmarshal(raw, &doc) != nil {
		return 0, 0
	}
	usage, _ := doc["usage"].(map[string]any)
	return usageCounts(usage)
}

// pipeResponsesAsChat copies a Qiniu bypass Responses stream as chat completion chunks.
// An error status is forwarded unchanged so the client still sees the provider message.
