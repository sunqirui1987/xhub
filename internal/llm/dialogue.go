package llm

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
)

// Turn 是文本与函数工具的内部对话项；ID 是工具调用关联 ID，Arguments 保留 JSON 参数字符串。
// 输入、上游与输出分别编码一次，避免按供应商和协议组合维护转换函数。
type Turn struct {
	Role, Text, ID, Name, Arguments string
	Calls                           []Turn
}

// Dialogue 保存统一对话请求；Options 只保存已实现协议可表达的已验证参数。
type Dialogue struct {
	Turns   []Turn
	Tools   []map[string]any
	Options map[string]any
	Stream  bool
}

// DialogueResult 保存实测回复与用量；ID 来源上游，由网关按该 ID 保存临时续接上下文。
type DialogueResult struct {
	ID, Text, Stop string
	Calls          []Turn
	Usage          map[string]any
}

// dialogueString 读取协议字符串；参数为动态字段，返回原字符串；解析器调用，无副作用。
func dialogueString(v any) string { s, _ := v.(string); return s }

// dialogueMap 安全读取协议对象；参数为动态值，返回对象或空值；解析器调用。
func dialogueMap(v any) map[string]any { m, _ := v.(map[string]any); return m }

// dialogueList 安全读取协议数组；参数为动态值，返回数组或空值；解析器调用。
func dialogueList(v any) []any { a, _ := v.([]any); return a }

// textContent 校验纯文本块；参数为内容字段，返回文本或明确能力错误，不丢弃图片等未知块。
// 调用：统一请求和工具结果解析；空内容允许仅工具调用的助手消息。
func textContent(v any) (string, error) {
	if v == nil {
		return "", nil
	}
	if s, ok := v.(string); ok {
		return s, nil
	}
	parts, ok := v.([]any)
	if !ok {
		return "", fmt.Errorf("content must be text or text blocks")
	}
	var out strings.Builder
	for _, p := range parts {
		m := dialogueMap(p)
		switch dialogueString(m["type"]) {
		case "text", "input_text", "output_text":
			out.WriteString(dialogueString(m["text"]))
		default:
			return "", fmt.Errorf("unsupported content capability %v", m["type"])
		}
	}
	return out.String(), nil
}

// ParseDialogue 将用户 OpenAI、Anthropic、Gemini/Vertex 对话解码成共享表示。参数为协议及正文；返回对话或字段错误。
// 调用：统一入口在发起上游前；不支持的能力明确拒绝，Bypass 入口不调用此函数。
func ParseDialogue(protocol string, body map[string]any) (Dialogue, error) {
	if protocol == "gemini" || protocol == "vertex" {
		return parseGoogleDialogue(body)
	}
	d := Dialogue{Options: map[string]any{}}
	if !strings.Contains("|openai-chat|openai-responses|anthropic-messages|", "|"+protocol+"|") {
		return d, fmt.Errorf("unregistered dialogue protocol %s", protocol)
	}
	if v, exists := body["stream"]; exists {
		var ok bool
		d.Stream, ok = v.(bool)
		if !ok {
			return d, fmt.Errorf("stream must be boolean")
		}
	}
	allowed := map[string]bool{}
	for _, k := range []string{"model", "stream", "stream_options", "tools", "tool_choice", "max_tokens", "max_completion_tokens", "max_output_tokens", "temperature", "top_p", "metadata", "user", "litellm_session_id", "prompt_cache_key"} {
		allowed[k] = true
	}
	switch protocol {
	case "openai-chat":
		allowed["messages"] = true
	case "openai-responses":
		allowed["input"], allowed["instructions"] = true, true
	case "anthropic-messages":
		allowed["messages"], allowed["system"] = true, true
	}
	// 不可转换的附加参数必须明确拒绝；会话标签由网关单独读取。
	for _, key := range []string{"metadata", "user", "prompt_cache_key"} {
		delete(allowed, key)
	}
	if value, exists := body["stream_options"]; exists {
		if err := dialogueFields(dialogueMap(value), "include_usage"); err != nil {
			return d, err
		}
		if _, ok := dialogueMap(value)["include_usage"].(bool); !ok {
			return d, fmt.Errorf("stream_options.include_usage must be boolean")
		}
	}
	for k, v := range body {
		if !allowed[k] && v != nil {
			return d, fmt.Errorf("unsupported unified capability %s; submit full history or use a compatible native endpoint", k)
		}
	}
	for _, k := range []string{"temperature", "top_p"} {
		if v, ok := body[k]; ok {
			n, valid := dialogueNumber(v)
			if !valid || n < 0 || (k == "top_p" && n > 1) || (k == "temperature" && n > 2) {
				return d, fmt.Errorf("invalid %s", k)
			}
			d.Options[k] = v
		}
	}
	for _, k := range []string{"max_tokens", "max_completion_tokens", "max_output_tokens"} {
		if v, ok := body[k]; ok {
			n, valid := dialogueNumber(v)
			if !valid || n <= 0 || n != math.Trunc(n) {
				return d, fmt.Errorf("invalid %s", k)
			}
			if _, exists := d.Options["max_tokens"]; exists {
				return d, fmt.Errorf("token limits are mutually exclusive")
			}
			d.Options["max_tokens"] = v
		}
	}
	if v, ok := body["tool_choice"]; ok {
		d.Options["tool_choice"] = v
	}
	if protocol == "openai-responses" {
		if s := dialogueString(body["instructions"]); s != "" {
			d.Turns = append(d.Turns, Turn{Role: "system", Text: s})
		}
	}
	if protocol == "anthropic-messages" {
		s, e := textContent(body["system"])
		if e != nil {
			return d, e
		}
		if s != "" {
			d.Turns = append(d.Turns, Turn{Role: "system", Text: s})
		}
	}
	raw := body["messages"]
	if protocol == "openai-responses" {
		raw = body["input"]
		if s, ok := raw.(string); ok {
			raw = []any{map[string]any{"role": "user", "content": s}}
		}
	}
	list, ok := raw.([]any)
	if !ok || len(list) == 0 {
		return d, fmt.Errorf("nonempty conversation is required")
	}
	for _, item := range list {
		m := dialogueMap(item)
		if m == nil {
			return d, fmt.Errorf("conversation item must be an object")
		}
		fields := []string{"role", "content"}
		if protocol == "openai-chat" {
			fields = append(fields, "tool_call_id", "tool_calls")
		}
		if protocol == "openai-responses" {
			fields = append(fields, "type", "call_id", "name", "arguments", "output", "id", "status")
		}
		if err := dialogueFields(m, fields...); err != nil {
			return d, err
		}
		role := dialogueString(m["role"])
		kind := dialogueString(m["type"])
		if protocol == "openai-responses" && kind != "" && kind != "message" && kind != "function_call" && kind != "function_call_output" {
			return d, fmt.Errorf("unsupported input item %s", kind)
		}
		if protocol == "openai-responses" && kind == "function_call" {
			d.Turns = append(d.Turns, Turn{Role: "assistant", Calls: []Turn{{ID: dialogueString(m["call_id"]), Name: dialogueString(m["name"]), Arguments: dialogueString(m["arguments"])}}})
			continue
		}
		if protocol == "openai-responses" && kind == "function_call_output" {
			s, e := textContent(m["output"])
			if e != nil {
				return d, e
			}
			d.Turns = append(d.Turns, Turn{Role: "tool", ID: dialogueString(m["call_id"]), Text: s})
			continue
		}
		if !strings.Contains("|system|developer|user|assistant|tool|", "|"+role+"|") {
			return d, fmt.Errorf("unsupported message role %s", role)
		}
		turn := Turn{Role: role, ID: dialogueString(m["tool_call_id"])}
		if protocol == "anthropic-messages" {
			if parts, ok := m["content"].([]any); ok {
				for _, p := range parts {
					b := dialogueMap(p)
					if err := dialogueFields(b, "type", "text", "id", "name", "input", "tool_use_id", "content", "is_error"); err != nil {
						return d, err
					}
					switch dialogueString(b["type"]) {
					case "text":
						turn.Text += dialogueString(b["text"])
					case "tool_use":
						a, e := json.Marshal(b["input"])
						if e != nil {
							return d, e
						}
						turn.Calls = append(turn.Calls, Turn{ID: dialogueString(b["id"]), Name: dialogueString(b["name"]), Arguments: string(a)})
					case "tool_result":
						if b["is_error"] == true {
							return d, fmt.Errorf("unsupported tool result is_error")
						}
						s, e := textContent(b["content"])
						if e != nil {
							return d, e
						}
						d.Turns = append(d.Turns, Turn{Role: "tool", ID: dialogueString(b["tool_use_id"]), Text: s})
					default:
						return d, fmt.Errorf("unsupported content capability %v", b["type"])
					}
				}
			} else {
				s, e := textContent(m["content"])
				if e != nil {
					return d, e
				}
				turn.Text = s
			}
		} else {
			s, e := textContent(m["content"])
			if e != nil {
				return d, e
			}
			turn.Text = s
			if value, exists := m["tool_calls"]; exists {
				if _, ok := value.([]any); !ok {
					return d, fmt.Errorf("tool_calls must be an array")
				}
			}
			for _, rawCall := range dialogueList(m["tool_calls"]) {
				c := dialogueMap(rawCall)
				if c["type"] != "function" {
					return d, fmt.Errorf("only function tools are supported")
				}
				if err := dialogueFields(c, "id", "type", "function"); err != nil {
					return d, err
				}
				f := dialogueMap(c["function"])
				if err := dialogueFields(f, "name", "arguments"); err != nil {
					return d, err
				}
				turn.Calls = append(turn.Calls, Turn{ID: dialogueString(c["id"]), Name: dialogueString(f["name"]), Arguments: dialogueString(f["arguments"])})
			}
		}
		if turn.Text != "" || len(turn.Calls) > 0 || role == "tool" {
			d.Turns = append(d.Turns, turn)
		}
	}
	if value, exists := body["tools"]; exists {
		if _, ok := value.([]any); !ok {
			return d, fmt.Errorf("tools must be an array")
		}
	}
	for _, rawTool := range dialogueList(body["tools"]) {
		tool := dialogueMap(rawTool)
		f := tool
		if protocol == "openai-chat" {
			if tool["type"] != "function" {
				return d, fmt.Errorf("only function tools are supported")
			}
			if err := dialogueFields(tool, "type", "function"); err != nil {
				return d, err
			}
			f = dialogueMap(tool["function"])
		}
		if protocol == "openai-responses" && tool["type"] != "function" {
			return d, fmt.Errorf("only function tools are supported")
		}
		if err := dialogueFields(f, "type", "name", "parameters", "input_schema", "description", "strict"); err != nil {
			return d, err
		}
		parameters := f["parameters"]
		if protocol == "anthropic-messages" {
			parameters = f["input_schema"]
		}
		if _, ok := parameters.(map[string]any); !ok {
			return d, fmt.Errorf("tool schema must be an object")
		}
		if v, exists := f["strict"]; exists {
			if _, ok := v.(bool); !ok {
				return d, fmt.Errorf("tool strict must be boolean")
			}
		}
		if dialogueString(f["name"]) == "" {
			return d, fmt.Errorf("tool name is required")
		}
		normalized := map[string]any{"name": f["name"], "parameters": parameters}
		if v, ok := f["description"]; ok {
			normalized["description"] = v
		}
		if v, ok := f["strict"]; ok {
			normalized["strict"] = v
		}
		d.Tools = append(d.Tools, normalized)
	}
	for _, t := range d.Turns {
		if t.Role == "tool" && t.ID == "" {
			return d, fmt.Errorf("tool result ID is required")
		}
		for _, c := range t.Calls {
			if c.ID == "" || c.Name == "" || !json.Valid([]byte(c.Arguments)) {
				return d, fmt.Errorf("tool call requires ID, name and valid JSON arguments")
			}
		}
	}
	if len(d.Turns) == 0 {
		return d, fmt.Errorf("conversation has no supported content")
	}
	return d, nil
}

// EncodeDialogue 按显式上游协议编码对话，型号原样传递。参数为内部表示、协议、真实型号；返回正文或能力错误。
// 调用：统一执行器；工具 ID 和参数不重新生成，Messages 工具结果按相邻角色合并。
func EncodeDialogue(d Dialogue, protocol, model string) (map[string]any, error) {
	if protocol == "gemini" || protocol == "vertex" {
		return encodeGoogleDialogue(d)
	}
	out := map[string]any{"model": model, "stream": d.Stream}
	if d.Stream && protocol == "openai-chat" {
		out["stream_options"] = map[string]any{"include_usage": true}
	}
	for k, v := range d.Options {
		if k != "tool_choice" && k != "max_tokens" {
			out[k] = v
		}
	}
	if v, ok := d.Options["max_tokens"]; ok {
		key := "max_tokens"
		if protocol == "openai-responses" {
			key = "max_output_tokens"
		}
		out[key] = v
	} else if protocol == "anthropic-messages" {
		out["max_tokens"] = 4096
	}
	var messages []any
	var system []string
	for _, t := range d.Turns {
		switch protocol {
		case "openai-chat":
			m := map[string]any{"role": t.Role, "content": t.Text}
			if t.Role == "tool" {
				m["tool_call_id"] = t.ID
			}
			if len(t.Calls) > 0 {
				var calls []any
				for _, c := range t.Calls {
					calls = append(calls, map[string]any{"id": c.ID, "type": "function", "function": map[string]any{"name": c.Name, "arguments": c.Arguments}})
				}
				m["tool_calls"] = calls
			}
			messages = append(messages, m)
		case "openai-responses":
			if t.Role == "tool" {
				messages = append(messages, map[string]any{"type": "function_call_output", "call_id": t.ID, "output": t.Text})
				continue
			}
			if t.Text != "" {
				messages = append(messages, map[string]any{"role": t.Role, "content": t.Text})
			}
			for _, c := range t.Calls {
				messages = append(messages, map[string]any{"type": "function_call", "call_id": c.ID, "name": c.Name, "arguments": c.Arguments})
			}
		case "anthropic-messages":
			if t.Role == "system" || t.Role == "developer" {
				system = append(system, t.Text)
				continue
			}
			role := t.Role
			var parts []any
			if role == "tool" {
				role = "user"
				parts = append(parts, map[string]any{"type": "tool_result", "tool_use_id": t.ID, "content": t.Text})
			} else {
				if t.Text != "" {
					parts = append(parts, map[string]any{"type": "text", "text": t.Text})
				}
				for _, c := range t.Calls {
					var args any
					if json.Unmarshal([]byte(c.Arguments), &args) != nil {
						return nil, fmt.Errorf("invalid tool arguments")
					}
					parts = append(parts, map[string]any{"type": "tool_use", "id": c.ID, "name": c.Name, "input": args})
				}
			}
			if len(messages) > 0 && dialogueMap(messages[len(messages)-1])["role"] == role {
				prev := dialogueMap(messages[len(messages)-1])
				prev["content"] = append(dialogueList(prev["content"]), parts...)
			} else {
				messages = append(messages, map[string]any{"role": role, "content": parts})
			}
		default:
			return nil, fmt.Errorf("unregistered dialogue protocol %s", protocol)
		}
	}
	key := "messages"
	if protocol == "openai-responses" {
		key = "input"
		out["store"] = false
	}
	out[key] = messages
	if len(system) > 0 {
		out["system"] = strings.Join(system, "\n")
	}
	if len(d.Tools) > 0 {
		var tools []any
		for _, f := range d.Tools {
			cp := map[string]any{}
			for k, v := range f {
				cp[k] = v
			}
			switch protocol {
			case "openai-chat":
				tools = append(tools, map[string]any{"type": "function", "function": cp})
			case "openai-responses":
				cp["type"] = "function"
				tools = append(tools, cp)
			case "anthropic-messages":
				if cp["strict"] != nil {
					return nil, fmt.Errorf("strict tools cannot be expressed by Messages")
				}
				cp["input_schema"] = cp["parameters"]
				delete(cp, "parameters")
				tools = append(tools, cp)
			}
		}
		out["tools"] = tools
	}
	if choice, ok := d.Options["tool_choice"]; ok {
		s := dialogueString(choice)
		name := ""
		if m := dialogueMap(choice); m != nil {
			kind := dialogueString(m["type"])
			if kind == "auto" {
				s = "auto"
			} else if kind == "any" {
				s = "required"
			} else if kind == "none" {
				s = "none"
			} else {
				name = dialogueString(m["name"])
				if f := dialogueMap(m["function"]); f != nil {
					name = dialogueString(f["name"])
				}
			}
		}
		if name != "" {
			switch protocol {
			case "openai-chat":
				out["tool_choice"] = map[string]any{"type": "function", "function": map[string]any{"name": name}}
			case "openai-responses":
				out["tool_choice"] = map[string]any{"type": "function", "name": name}
			default:
				out["tool_choice"] = map[string]any{"type": "tool", "name": name}
			}
		} else if s == "auto" || s == "required" || s == "none" {
			if protocol == "anthropic-messages" {
				if s == "required" {
					s = "any"
				}
				out["tool_choice"] = map[string]any{"type": s}
			} else {
				out["tool_choice"] = s
			}
		} else {
			return nil, fmt.Errorf("unsupported tool_choice")
		}
	}
	return out, nil
}

// ParseDialogueResult 提取上游文本、工具调用、结束原因和用量事实。参数为协议与响应；返回结果或格式错误。
// 调用：统一同步响应和流终态；不参与价格计算，不推断供应商。
func ParseDialogueResult(protocol string, raw []byte) (DialogueResult, error) {
	if protocol == "gemini" || protocol == "vertex" {
		return parseGoogleDialogueResult(raw)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return DialogueResult{}, err
	}
	if m["error"] != nil {
		return DialogueResult{}, fmt.Errorf("upstream dialogue error")
	}
	if protocol == "openai-responses" && m["status"] != "completed" && m["status"] != "incomplete" {
		return DialogueResult{}, fmt.Errorf("upstream response is not complete")
	}
	r := DialogueResult{ID: dialogueString(m["id"]), Usage: map[string]any{}}
	u := dialogueMap(m["usage"])
	r.Usage = NormalizeDialogueUsage(u)
	switch protocol {
	case "openai-chat":
		r.Usage = NormalizeDialogueUsage(u)
		c := dialogueList(m["choices"])
		if len(c) != 1 {
			return r, fmt.Errorf("missing response choices")
		}
		choice := dialogueMap(c[0])
		r.Stop = dialogueString(choice["finish_reason"])
		msg := dialogueMap(choice["message"])
		if msg["refusal"] != nil && msg["refusal"] != "" {
			return r, fmt.Errorf("unsupported response refusal")
		}
		var err error
		r.Text, err = textContent(msg["content"])
		if err != nil {
			return r, err
		}
		for _, v := range dialogueList(msg["tool_calls"]) {
			call := dialogueMap(v)
			f := dialogueMap(call["function"])
			r.Calls = append(r.Calls, Turn{ID: dialogueString(call["id"]), Name: dialogueString(f["name"]), Arguments: dialogueString(f["arguments"])})
		}
	case "openai-responses":
		for _, v := range dialogueList(m["output"]) {
			item := dialogueMap(v)
			if item["type"] == "function_call" {
				r.Calls = append(r.Calls, Turn{ID: dialogueString(item["call_id"]), Name: dialogueString(item["name"]), Arguments: dialogueString(item["arguments"])})
				continue
			}
			if item["type"] != "message" {
				return r, fmt.Errorf("unsupported response item %v", item["type"])
			}
			s, e := textContent(item["content"])
			if e != nil {
				return r, e
			}
			r.Text += s
		}
		r.Stop = "stop"
		if m["status"] == "incomplete" {
			r.Stop = "length"
		}
	case "anthropic-messages":
		for _, v := range dialogueList(m["content"]) {
			b := dialogueMap(v)
			switch b["type"] {
			case "text":
				r.Text += dialogueString(b["text"])
			case "tool_use":
				args, _ := json.Marshal(b["input"])
				r.Calls = append(r.Calls, Turn{ID: dialogueString(b["id"]), Name: dialogueString(b["name"]), Arguments: string(args)})
			default:
				return r, fmt.Errorf("unsupported response content %v", b["type"])
			}
		}
		r.Stop = dialogueString(m["stop_reason"])
		if r.Stop == "max_tokens" {
			r.Stop = "length"
		} else if r.Stop != "tool_use" {
			r.Stop = "stop"
		}
	default:
		return r, fmt.Errorf("unregistered dialogue protocol %s", protocol)
	}
	for _, call := range r.Calls {
		if call.ID == "" || call.Name == "" || !json.Valid([]byte(call.Arguments)) {
			return r, fmt.Errorf("invalid upstream tool call")
		}
	}
	if len(r.Calls) > 0 {
		r.Stop = "tool_calls"
	}
	return r, nil
}

// EncodeDialogueResult 将共享回复输出到用户协议；参数为结果、入口协议及公开型号，返回 JSON。
// 调用：统一同步入口及 SSE 终态；Responses 上游 store=false，网关通过临时上下文支持 previous_response_id。
func EncodeDialogueResult(r DialogueResult, protocol, model string) ([]byte, error) {
	if protocol == "gemini" || protocol == "vertex" {
		return encodeGoogleDialogueResult(r)
	}
	var out map[string]any
	u := r.Usage
	if u == nil {
		u = map[string]any{}
	}
	switch protocol {
	case "openai-chat":
		msg := map[string]any{"role": "assistant", "content": r.Text}
		if len(r.Calls) > 0 {
			var calls []any
			for _, c := range r.Calls {
				calls = append(calls, map[string]any{"id": c.ID, "type": "function", "function": map[string]any{"name": c.Name, "arguments": c.Arguments}})
			}
			msg["tool_calls"] = calls
		}
		out = map[string]any{"id": r.ID, "object": "chat.completion", "model": model, "choices": []any{map[string]any{"index": 0, "message": msg, "finish_reason": r.Stop}}, "usage": u}
	case "openai-responses":
		items := []any{}
		if r.Text != "" {
			items = append(items, map[string]any{"id": r.ID + "_message", "type": "message", "role": "assistant", "status": "completed", "content": []any{map[string]any{"type": "output_text", "text": r.Text, "annotations": []any{}}}})
		}
		for _, c := range r.Calls {
			items = append(items, map[string]any{"id": c.ID, "type": "function_call", "call_id": c.ID, "name": c.Name, "arguments": c.Arguments, "status": "completed"})
		}
		status := "completed"
		if r.Stop == "length" {
			status = "incomplete"
		}
		out = map[string]any{"id": r.ID, "object": "response", "model": model, "status": status, "store": false, "output": items, "usage": dialogueOutputUsage(u, protocol)}
		if status == "incomplete" {
			out["incomplete_details"] = map[string]any{"reason": "max_output_tokens"}
		}
	case "anthropic-messages":
		content := []any{}
		if r.Text != "" {
			content = append(content, map[string]any{"type": "text", "text": r.Text})
		}
		for _, c := range r.Calls {
			var args any
			if json.Unmarshal([]byte(c.Arguments), &args) != nil {
				return nil, fmt.Errorf("invalid response tool arguments")
			}
			content = append(content, map[string]any{"type": "tool_use", "id": c.ID, "name": c.Name, "input": args})
		}
		stop := "end_turn"
		if r.Stop == "length" {
			stop = "max_tokens"
		}
		if len(r.Calls) > 0 {
			stop = "tool_use"
		}
		out = map[string]any{"id": r.ID, "type": "message", "role": "assistant", "model": model, "content": content, "stop_reason": stop, "usage": dialogueOutputUsage(u, protocol)}
	default:
		return nil, fmt.Errorf("unregistered dialogue protocol %s", protocol)
	}
	return json.Marshal(out)
}

// dialogueFields 校验动态对象字段白名单；参数为对象及可表达字段，返回明确错误。
// 调用：统一解析器和工具选择；缺失对象与未知能力在外部调用前拒绝，不修改输入。
func dialogueFields(m map[string]any, fields ...string) error {
	if m == nil {
		return fmt.Errorf("expected protocol object")
	}
	allowed := map[string]bool{}
	for _, key := range fields {
		allowed[key] = true
	}
	for key := range m {
		if !allowed[key] {
			return fmt.Errorf("unsupported unified capability %s", key)
		}
	}
	return nil
}

// dialogueNumber 读取有限数值；参数为动态数值，返回数和有效标记；请求校验调用，无副作用。
func dialogueNumber(v any) (float64, bool) {
	var n float64
	switch value := v.(type) {
	case float64:
		n = value
	case int:
		n = float64(value)
	case json.Number:
		var err error
		n, err = value.Float64()
		if err != nil {
			return 0, false
		}
	default:
		return 0, false
	}
	return n, !math.IsNaN(n) && !math.IsInf(n, 0)
}

// NormalizeDialogueUsage 统一实测用量名称并保留缓存事实；参数为协议用量，返回新对象。
// 调用：同步和流适配器；缺失实测输入或输出时标记禁止自动计价，不估算提示词。
func NormalizeDialogueUsage(u map[string]any) map[string]any {
	out := map[string]any{}
	for key, value := range u {
		out[key] = value
	}
	for source, target := range map[string]string{"input_tokens": "prompt_tokens", "output_tokens": "completion_tokens", "input_tokens_details": "prompt_tokens_details", "output_tokens_details": "completion_tokens_details"} {
		if value, exists := u[source]; exists {
			out[target] = value
		}
		delete(out, source)
	}
	if value, exists := u["cache_read_input_tokens"]; exists {
		out["prompt_tokens_details"] = map[string]any{"cached_tokens": value}
	}
	input, hasInput := dialogueNumber(out["prompt_tokens"])
	output, hasOutput := dialogueNumber(out["completion_tokens"])
	if !hasInput || !hasOutput {
		out["pricing_blocked"] = "upstream_usage_missing"
	} else {
		out["total_tokens"] = input + output
	}
	return out
}

// dialogueOutputUsage 将共享实测事实映射到用户用量协议；参数为事实与协议，返回新对象。
// 调用：同步输出；未知用量通过 pricing_blocked 明示，不生成估算数值。
func dialogueOutputUsage(u map[string]any, protocol string) map[string]any {
	out := map[string]any{}
	for key, value := range u {
		out[key] = value
	}
	for source, target := range map[string]string{"prompt_tokens": "input_tokens", "completion_tokens": "output_tokens", "prompt_tokens_details": "input_tokens_details", "completion_tokens_details": "output_tokens_details"} {
		if value, exists := u[source]; exists {
			out[target] = value
		}
		delete(out, source)
	}
	if protocol == "anthropic-messages" {
		if details := dialogueMap(u["prompt_tokens_details"]); details != nil {
			out["cache_read_input_tokens"] = details["cached_tokens"]
		}
		delete(out, "input_tokens_details")
		delete(out, "output_tokens_details")
		delete(out, "total_tokens")
	}
	return out
}
