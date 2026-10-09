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

	"github.com/sunqirui1987/xhub/internal/logx"
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
// 返回：请求结构开销 32，加上输入文本长度除以 4。输出上限不属于输入用量。
// 调用：Serve、ServeBypass、gateway estimateTokens。没有单独的表驱动测试。
func EstimateTokens(body map[string]any) int {
	n := 32
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
	case int64:
		return int(t)
	case json.Number:
		n, _ := t.Int64()
		return int(n)
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
	_, promptReported := usageCount(usage, "prompt_tokens", "input_tokens")
	_, completionReported := usageCount(usage, "completion_tokens", "output_tokens")
	// A count that had to be estimated is worth a line: it means this call was
	// billed from an approximation rather than from what the upstream reported.
	estimated := false
	if !promptReported {
		if n := EstimateTokens(body); n > 0 {
			usage["prompt_tokens"] = n
			estimated = true
		}
	}
	if !completionReported {
		if n := outputTokens(streamed); n > 0 {
			usage["completion_tokens"] = n
			estimated = true
		}
	}
	if estimated {
		logx.Debug("usage completed by estimate prompt=%v completion=%v", usage["prompt_tokens"], usage["completion_tokens"])
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
	pt, _ = usageCount(usage, "prompt_tokens", "input_tokens")
	ct, _ = usageCount(usage, "completion_tokens", "output_tokens")
	return pt, ct
}

// usageCount returns the first reported numeric field, preserving the
// difference between an explicit zero and an absent value.
// 参数 usage：供应商用量对象；keys：按优先级排列的字段别名。
// 返回：字段值和是否明确报告。
// 调用：completeUsage、usageCounts、usageFromDocument。测试：usage_stream_test.go。
func usageCount(usage map[string]any, keys ...string) (int, bool) {
	for _, key := range keys {
		if value, ok := usage[key]; ok {
			switch value.(type) {
			case float64, int, int64, json.Number:
				return asInt(value), true
			}
		}
	}
	return 0, false
}

// usageFromDocument finds provider usage in ordinary, Responses, and Gemini
// envelopes and normalizes Gemini's field names for the billing path.
// 参数 doc：一条已解析的同步响应或流事件。
// 返回：规范化的用量对象；文档没有用量时返回 nil。
// 调用：streamUsageParser.consumeLine、bodyUsage。测试：usage_stream_test.go。
func usageFromDocument(doc map[string]any) map[string]any {
	if usage, ok := doc["usage"].(map[string]any); ok {
		return usage
	}
	if response, ok := doc["response"].(map[string]any); ok {
		if usage, ok := response["usage"].(map[string]any); ok {
			return usage
		}
	}
	metadata, ok := doc["usageMetadata"].(map[string]any)
	if !ok {
		return nil
	}
	usage := map[string]any{}
	if n, present := usageCount(metadata, "promptTokenCount"); present {
		usage["prompt_tokens"] = n
	}
	completion, candidatesPresent := usageCount(metadata, "candidatesTokenCount")
	thoughts, thoughtsPresent := usageCount(metadata, "thoughtsTokenCount")
	if candidatesPresent || thoughtsPresent {
		usage["completion_tokens"] = completion + thoughts
	}
	if n, present := usageCount(metadata, "cachedContentTokenCount"); present {
		usage["prompt_tokens_details"] = map[string]any{"cached_tokens": n}
	}
	return usage
}

// mergeUsage overlays reported fields while retaining fields omitted by later
// partial events. Nested detail objects are merged recursively.
// 参数 dst：此前累计的用量；src：新事件报告的部分用量。
// 返回：合并后的用量对象。
// 调用：streamUsageParser.consumeLine 及自身递归。测试：usage_stream_test.go。
func mergeUsage(dst, src map[string]any) map[string]any {
	if len(src) == 0 {
		return dst
	}
	if dst == nil {
		dst = map[string]any{}
	}
	for key, value := range src {
		if incoming, ok := value.(map[string]any); ok {
			existing, _ := dst[key].(map[string]any)
			dst[key] = mergeUsage(existing, incoming)
			continue
		}
		dst[key] = value
	}
	return dst
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
	usage := usageFromDocument(doc)
	return usageCounts(usage)
}
