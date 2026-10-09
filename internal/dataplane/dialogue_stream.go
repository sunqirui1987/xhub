package dataplane

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/sunqirui1987/xhub/internal/llm"
)

// dialogueStream 保存单次流转换状态，工具按稳定输出索引收集参数增量；不跨请求共享。
type dialogueStream struct {
	writer                          http.ResponseWriter
	capture                         bytes.Buffer
	protocol, model, id, text, stop string
	calls                           map[int]*llm.Turn
	blocks                          map[int]int
	usage                           map[string]any
	started, wrote, completed       bool
	nextBlock                       int
	textBlock                       int
	sequence                        int
	toolIndices                     map[int]int
	// beforeComplete 在 Responses 完成事件发送前保存续接状态，参数为已验证的最终回复。
	beforeComplete func(llm.DialogueResult)
}

// streamMap 读取事件对象；参数为动态字段，返回对象或空值；流解析器调用。
func streamMap(v any) map[string]any { m, _ := v.(map[string]any); return m }

// streamString 读取事件字符串；参数为动态字段，返回字符串或空值；流解析器调用。
func streamString(v any) string { s, _ := v.(string); return s }

// streamIndex 读取事件索引；参数为 JSON 数字，返回整数；流解析器调用。
func streamIndex(v any) int { n, _ := v.(float64); return int(n) }

// emit 写完整 SSE 事件并立即刷新。参数为事件名和数据；返回错误，同时记录已发出的日志字节。
// 调用：所有输出编码器；第一次输出后即禁止更换部署。
func (s *dialogueStream) emit(event string, data any) error {
	if s.protocol == "openai-responses" {
		if m, ok := data.(map[string]any); ok {
			m["sequence_number"] = s.sequence
			s.sequence++
		}
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return err
	}
	var b strings.Builder
	if event != "" {
		fmt.Fprintf(&b, "event: %s\n", event)
	}
	fmt.Fprintf(&b, "data: %s\n\n", raw)
	chunk := []byte(b.String())
	s.capture.Write(chunk)
	n, err := s.writer.Write(chunk)
	if n > 0 {
		s.wrote = true
	}
	if f, ok := s.writer.(http.Flusher); ok {
		f.Flush()
	}
	return err
}

// start 输出协议规定的开始事件；参数无，返回写入错误，幂等；调用：首个上游事件后。
func (s *dialogueStream) start() error {
	if s.started {
		return nil
	}
	s.started = true
	switch s.protocol {
	case "gemini", "vertex":
		return nil
	case "openai-chat":
		return s.chat(map[string]any{"role": "assistant"}, nil)
	case "openai-responses":
		return s.emit("response.created", map[string]any{"type": "response.created", "response": map[string]any{"id": s.id, "object": "response", "model": s.model, "status": "in_progress", "store": false, "output": []any{}}})
	case "anthropic-messages":
		return s.emit("message_start", map[string]any{"type": "message_start", "message": map[string]any{"id": s.id, "type": "message", "role": "assistant", "model": s.model, "content": []any{}, "stop_reason": nil, "usage": map[string]any{"input_tokens": s.usage["prompt_tokens"], "output_tokens": 0}}})
	}
	return fmt.Errorf("unsupported stream protocol")
}

// chat 编码 Chat 增量或结束原因。参数为增量对象与结束原因；返回写错误；调用：输出事件处理。
func (s *dialogueStream) chat(delta map[string]any, finish any) error {
	return s.emit("", map[string]any{"id": s.id, "object": "chat.completion.chunk", "created": time.Now().Unix(), "model": s.model, "choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": finish}}})
}

// textDelta 转发文本增量，首次建立内容块；参数为文本，返回写错误；调用：三种上游事件解码。
func (s *dialogueStream) textDelta(text string) error {
	if text == "" {
		return nil
	}
	if err := s.start(); err != nil {
		return err
	}
	s.text += text
	switch s.protocol {
	case "gemini", "vertex":
		return s.emit("", map[string]any{"candidates": []any{map[string]any{"index": 0, "content": map[string]any{"role": "model", "parts": []any{map[string]any{"text": text}}}}}})
	case "openai-chat":
		return s.chat(map[string]any{"content": text}, nil)
	case "openai-responses":
		if s.textBlock < 0 {
			s.textBlock = s.nextBlock
			s.nextBlock++
			if err := s.emit("response.output_item.added", map[string]any{"type": "response.output_item.added", "output_index": s.textBlock, "item": map[string]any{"id": s.id + "_message", "type": "message", "role": "assistant", "status": "in_progress", "content": []any{}}}); err != nil {
				return err
			}
			if err := s.emit("response.content_part.added", map[string]any{"type": "response.content_part.added", "item_id": s.id + "_message", "output_index": s.textBlock, "content_index": 0, "part": map[string]any{"type": "output_text", "text": "", "annotations": []any{}}}); err != nil {
				return err
			}
		}
		return s.emit("response.output_text.delta", map[string]any{"type": "response.output_text.delta", "item_id": s.id + "_message", "output_index": s.textBlock, "content_index": 0, "delta": text})
	case "anthropic-messages":
		if s.textBlock < 0 {
			s.textBlock = s.nextBlock
			s.nextBlock++
			if err := s.emit("content_block_start", map[string]any{"type": "content_block_start", "index": s.textBlock, "content_block": map[string]any{"type": "text", "text": ""}}); err != nil {
				return err
			}
		}
		return s.emit("content_block_delta", map[string]any{"type": "content_block_delta", "index": s.textBlock, "delta": map[string]any{"type": "text_delta", "text": text}})
	}
	return nil
}

// toolDelta 保留工具 ID、名称与 JSON 参数增量，首次创建协议内容块。
// 参数为上游输出索引与增量；返回写错误；调用：上游工具事件，不重新生成工具 ID。
func (s *dialogueStream) toolDelta(index int, id, name, args string) error {
	if err := s.start(); err != nil {
		return err
	}
	call := s.calls[index]
	fresh := call == nil
	if fresh {
		call = &llm.Turn{}
		s.calls[index] = call
		s.toolIndices[index] = len(s.toolIndices)
		s.blocks[index] = s.nextBlock
		s.nextBlock++
	}
	if id != "" {
		call.ID = id
	}
	call.Name += name
	call.Arguments += args
	block := s.blocks[index]
	switch s.protocol {
	case "openai-chat":
		fn := map[string]any{}
		if name != "" {
			fn["name"] = name
		}
		if args != "" {
			fn["arguments"] = args
		}
		c := map[string]any{"index": s.toolIndices[index], "function": fn}
		if fresh || id != "" {
			c["id"] = call.ID
			c["type"] = "function"
		}
		return s.chat(map[string]any{"tool_calls": []any{c}}, nil)
	case "openai-responses":
		if fresh {
			if err := s.emit("response.output_item.added", map[string]any{"type": "response.output_item.added", "output_index": block, "item": map[string]any{"id": call.ID, "type": "function_call", "call_id": call.ID, "name": call.Name, "arguments": "", "status": "in_progress"}}); err != nil {
				return err
			}
		}
		if args != "" {
			return s.emit("response.function_call_arguments.delta", map[string]any{"type": "response.function_call_arguments.delta", "output_index": block, "item_id": call.ID, "delta": args})
		}
	case "anthropic-messages":
		if fresh {
			if err := s.emit("content_block_start", map[string]any{"type": "content_block_start", "index": block, "content_block": map[string]any{"type": "tool_use", "id": call.ID, "name": call.Name, "input": map[string]any{}}}); err != nil {
				return err
			}
		}
		if args != "" {
			return s.emit("content_block_delta", map[string]any{"type": "content_block_delta", "index": block, "delta": map[string]any{"type": "input_json_delta", "partial_json": args}})
		}
	}
	return nil
}

// observeUsage 合并上游实测用量；参数为协议用量，返回无；调用：流开始、增量与终态。
func (s *dialogueStream) observeUsage(u map[string]any) {
	for k, v := range u {
		switch k {
		case "input_tokens":
			s.usage["prompt_tokens"] = v
		case "output_tokens":
			s.usage["completion_tokens"] = v
		default:
			s.usage[k] = v
		}
	}
}

// finish 校验完整工具调用并输出协议终态；无参数，返回写入或能力错误。
// pipeDialogue 在上游完成后调用；Responses 发布终态前回调提交续接历史，失败流不进入此步骤。
func (s *dialogueStream) finish() error {
	if err := s.start(); err != nil {
		return err
	}
	s.usage = llm.NormalizeDialogueUsage(s.usage)
	r := llm.DialogueResult{ID: s.id, Text: s.text, Stop: s.stop, Usage: s.usage}
	if r.Stop == "" {
		r.Stop = "stop"
	}
	indices := make([]int, 0, len(s.calls))
	for i := range s.calls {
		indices = append(indices, i)
	}
	sort.Ints(indices)
	for _, i := range indices {
		if c := s.calls[i]; c != nil {
			if c.Arguments == "" {
				c.Arguments = "{}"
			}
			if c.ID == "" || c.Name == "" || !json.Valid([]byte(c.Arguments)) {
				return fmt.Errorf("incomplete upstream tool call")
			}
			r.Calls = append(r.Calls, *c)
		}
	}
	if len(r.Calls) > 0 {
		r.Stop = "tool_calls"
	}
	switch s.protocol {
	case "gemini", "vertex":
		// 文本已按增量发送；终态仅发送完整函数参数与用量，避免文本重复。
		r.Text = ""
		raw, err := llm.EncodeDialogueResult(r, s.protocol, s.model)
		if err != nil {
			return err
		}
		var out map[string]any
		if err := json.Unmarshal(raw, &out); err != nil {
			return err
		}
		return s.emit("", out)
	case "openai-chat":
		if err := s.chat(map[string]any{}, r.Stop); err != nil {
			return err
		}
		if err := s.emit("", map[string]any{"id": s.id, "object": "chat.completion.chunk", "model": s.model, "choices": []any{}, "usage": s.usage}); err != nil {
			return err
		}
		s.capture.WriteString("data: [DONE]\n\n")
		_, err := io.WriteString(s.writer, "data: [DONE]\n\n")
		if f, ok := s.writer.(http.Flusher); ok {
			f.Flush()
		}
		return err
	case "openai-responses":
		raw, err := llm.EncodeDialogueResult(r, s.protocol, s.model)
		if err != nil {
			return err
		}
		var response map[string]any
		_ = json.Unmarshal(raw, &response)
		// 完成事件按已发送的输出索引重排，保留工具和文本块的生命周期及稳定 ID。
		items := make([]any, s.nextBlock)
		if s.textBlock >= 0 {
			part := map[string]any{"type": "output_text", "text": s.text, "annotations": []any{}}
			item := map[string]any{"id": s.id + "_message", "type": "message", "role": "assistant", "status": "completed", "content": []any{part}}
			items[s.textBlock] = item
			for _, event := range []map[string]any{
				{"type": "response.output_text.done", "item_id": s.id + "_message", "output_index": s.textBlock, "content_index": 0, "text": s.text},
				{"type": "response.content_part.done", "item_id": s.id + "_message", "output_index": s.textBlock, "content_index": 0, "part": part},
				{"type": "response.output_item.done", "output_index": s.textBlock, "item": item},
			} {
				if err := s.emit(streamString(event["type"]), event); err != nil {
					return err
				}
			}
		}
		for _, index := range indices {
			c := s.calls[index]
			block := s.blocks[index]
			item := map[string]any{"id": c.ID, "type": "function_call", "call_id": c.ID, "name": c.Name, "arguments": c.Arguments, "status": "completed"}
			items[block] = item
			if err := s.emit("response.function_call_arguments.done", map[string]any{"type": "response.function_call_arguments.done", "item_id": c.ID, "output_index": block, "arguments": c.Arguments}); err != nil {
				return err
			}
			if err := s.emit("response.output_item.done", map[string]any{"type": "response.output_item.done", "output_index": block, "item": item}); err != nil {
				return err
			}
		}
		response["output"] = items
		event := "response.completed"
		if r.Stop == "length" {
			event = "response.incomplete"
		}
		if s.beforeComplete != nil {
			s.beforeComplete(r)
		}
		return s.emit(event, map[string]any{"type": event, "response": response})
	case "anthropic-messages":
		for i := 0; i < s.nextBlock; i++ {
			if err := s.emit("content_block_stop", map[string]any{"type": "content_block_stop", "index": i}); err != nil {
				return err
			}
		}
		stop := "end_turn"
		if r.Stop == "length" {
			stop = "max_tokens"
		}
		if len(r.Calls) > 0 {
			stop = "tool_use"
		}
		if err := s.emit("message_delta", map[string]any{"type": "message_delta", "delta": map[string]any{"stop_reason": stop, "stop_sequence": nil}, "usage": map[string]any{"output_tokens": s.usage["completion_tokens"]}}); err != nil {
			return err
		}
		return s.emit("message_stop", map[string]any{"type": "message_stop"})
	}
	return nil
}

// consume 解码一种上游 SSE 事件成共享文本/工具/用量事实；参数为协议与 JSON 数据，返回错误。
// 调用：pipeDialogue；支持参数增量、终态及明确失败，不按供应商分支。
func (s *dialogueStream) consume(protocol string, data string) error {
	if data == "[DONE]" {
		if protocol == "openai-chat" {
			s.completed = true
		}
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(data), &m); err != nil {
		return err
	}
	if m["error"] != nil {
		return fmt.Errorf("upstream stream error")
	}
	if id := streamString(m["id"]); id != "" {
		s.id = id
	}
	s.observeUsage(streamMap(m["usage"]))
	typ := streamString(m["type"])
	switch protocol {
	case "gemini", "vertex":
		if m["candidates"] == nil {
			// Google 可在单独末帧提供用量；不能把缺少候选的普通空帧视为完成。
			if streamMap(m["usageMetadata"]) != nil {
				m["candidates"] = []any{map[string]any{}}
				raw, _ := json.Marshal(m)
				result, err := llm.ParseDialogueResult(protocol, raw)
				if err != nil {
					return err
				}
				for k, v := range result.Usage {
					if k != "pricing_blocked" {
						s.usage[k] = v
					}
				}
				return nil
			}
			return fmt.Errorf("Google stream candidate missing")
		}
		r, err := llm.ParseDialogueResult(protocol, []byte(data))
		if err != nil {
			return err
		}
		if r.ID != "" {
			s.id = r.ID
		}
		for k, v := range r.Usage {
			if k != "pricing_blocked" {
				s.usage[k] = v
			}
		}
		if err := s.textDelta(r.Text); err != nil {
			return err
		}
		for _, c := range r.Calls {
			if err := s.googleCall(c); err != nil {
				return err
			}
		}
		choices, _ := m["candidates"].([]any)
		if len(choices) == 1 && streamString(streamMap(choices[0])["finishReason"]) != "" {
			s.completed = true
			s.stop = r.Stop
		}
	case "openai-chat":
		choices, _ := m["choices"].([]any)
		for _, v := range choices {
			choice := streamMap(v)
			if streamIndex(choice["index"]) != 0 {
				return fmt.Errorf("multiple stream choices unsupported")
			}
			delta := streamMap(choice["delta"])
			if err := s.textDelta(streamString(delta["content"])); err != nil {
				return err
			}
			calls, _ := delta["tool_calls"].([]any)
			for _, v := range calls {
				c := streamMap(v)
				f := streamMap(c["function"])
				if err := s.toolDelta(streamIndex(c["index"]), streamString(c["id"]), streamString(f["name"]), streamString(f["arguments"])); err != nil {
					return err
				}
			}
			if stop := streamString(choice["finish_reason"]); stop != "" {
				s.stop = stop
			}
		}
	case "openai-responses":
		if response := streamMap(m["response"]); response != nil {
			s.id = streamString(response["id"])
			s.observeUsage(streamMap(response["usage"]))
		}
		switch typ {
		case "response.output_text.delta":
			return s.textDelta(streamString(m["delta"]))
		case "response.output_item.added":
			item := streamMap(m["item"])
			if item["type"] == "function_call" {
				return s.toolDelta(streamIndex(m["output_index"]), streamString(item["call_id"]), streamString(item["name"]), streamString(item["arguments"]))
			}
		case "response.function_call_arguments.delta":
			return s.toolDelta(streamIndex(m["output_index"]), "", "", streamString(m["delta"]))
		case "response.completed":
			s.completed = true
		case "response.incomplete":
			s.completed = true
			s.stop = "length"
		case "response.failed", "error":
			return fmt.Errorf("upstream response failed")
		}
	case "anthropic-messages":
		switch typ {
		case "message_start":
			msg := streamMap(m["message"])
			s.id = streamString(msg["id"])
			s.observeUsage(streamMap(msg["usage"]))
		case "content_block_start":
			b := streamMap(m["content_block"])
			switch b["type"] {
			case "text":
				return s.textDelta(streamString(b["text"]))
			case "tool_use":
				args := ""
				if input := streamMap(b["input"]); len(input) > 0 {
					raw, _ := json.Marshal(input)
					args = string(raw)
				}
				return s.toolDelta(streamIndex(m["index"]), streamString(b["id"]), streamString(b["name"]), args)
			default:
				return fmt.Errorf("unsupported upstream content block %v", b["type"])
			}
		case "content_block_delta":
			d := streamMap(m["delta"])
			switch d["type"] {
			case "text_delta":
				return s.textDelta(streamString(d["text"]))
			case "input_json_delta":
				return s.toolDelta(streamIndex(m["index"]), "", "", streamString(d["partial_json"]))
			default:
				return fmt.Errorf("unsupported upstream delta %v", d["type"])
			}
		case "message_delta":
			d := streamMap(m["delta"])
			if d["stop_reason"] == "max_tokens" {
				s.stop = "length"
			}
		case "message_stop":
			s.completed = true
		case "error":
			return fmt.Errorf("upstream message failed")
		}
	default:
		return fmt.Errorf("unsupported upstream stream protocol")
	}
	return nil
}

// googleCall 收集 Google 完整函数调用；参数为解码调用，返回写入或冲突错误。
// 流事件可能重复携带同一完整调用，相同 ID 和内容只发送一次；不同内容明确失败，不把完整参数当增量拼接。
func (s *dialogueStream) googleCall(c llm.Turn) error {
	for _, existing := range s.calls {
		if existing.ID == c.ID {
			if existing.Name == c.Name && existing.Arguments == c.Arguments {
				return nil
			}
			return fmt.Errorf("conflicting Google function call ID")
		}
	}
	return s.toolDelta(len(s.calls), c.ID, c.Name, c.Arguments)
}

// pipeDialogue 流式执行共享协议转换；参数为 HTTP 输出、上游响应、时间、公开模型和双方协议。
// 返回已输出标记、用量、首字时间、日志字节和错误；调用：统一入口。输出后不允许跨部署重试。
// beforeComplete 是可选成功回调，Responses 终态前调用；失败或未完成的流不调用。
func pipeDialogue(w http.ResponseWriter, resp *http.Response, start time.Time, alias, upstream, caller string, beforeComplete ...func(llm.DialogueResult)) (bool, map[string]any, time.Duration, []byte, error) {
	defer resp.Body.Close()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	s := dialogueStream{writer: w, protocol: caller, model: alias, calls: map[int]*llm.Turn{}, blocks: map[int]int{}, usage: map[string]any{}, textBlock: -1, toolIndices: map[int]int{}}
	if len(beforeComplete) > 0 {
		s.beforeComplete = beforeComplete[0]
	}
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 4096), 8<<20)
	var lines []string
	var ttft time.Duration
	var err error
	for scanner.Scan() {
		line := scanner.Text()
		if line != "" {
			if strings.HasPrefix(line, "data:") {
				lines = append(lines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
			}
			continue
		}
		if len(lines) == 0 {
			continue
		}
		err = s.consume(upstream, strings.Join(lines, "\n"))
		lines = nil
		if s.wrote && ttft == 0 {
			ttft = time.Since(start)
		}
		if err != nil {
			break
		}
	}
	if err == nil {
		err = scanner.Err()
	}
	if err == nil && len(lines) > 0 {
		err = s.consume(upstream, strings.Join(lines, "\n"))
	}
	if err == nil && !s.completed {
		err = fmt.Errorf("upstream stream ended without completion")
	}
	if err == nil {
		err = s.finish()
	}
	if err != nil && s.wrote { // 已输出后发送明确失败事件，不能生成完成事件或改用另一个部署。
		event := "error"
		payload := map[string]any{"type": "error", "error": map[string]any{"type": "upstream_error", "message": err.Error()}}
		if caller == "openai-responses" {
			event = "response.failed"
			payload = map[string]any{"type": event, "response": map[string]any{"id": s.id, "status": "failed", "error": map[string]any{"code": "upstream_error", "message": err.Error()}}}
		}
		_ = s.emit(event, payload)
	}
	return s.wrote, s.usage, ttft, s.capture.Bytes(), err
}
