package llm

import (
	"encoding/json"
	"fmt"
)

// parseGoogleDialogue 把 Gemini/Vertex 正文转为共享文本和函数调用。
// 参数为网关已注入别名的正文；返回已验证对话或能力错误；统一入口调用，不丢弃未知多模态字段。
func parseGoogleDialogue(body map[string]any) (Dialogue, error) {
	if err := dialogueFields(body, "model", "stream", "contents", "systemInstruction", "generationConfig", "tools", "toolConfig", "litellm_session_id"); err != nil {
		return Dialogue{}, err
	}
	chat := map[string]any{"model": body["model"]}
	if v, ok := body["stream"]; ok {
		chat["stream"] = v
	}
	messages := []any{}
	if sys, ok := body["systemInstruction"]; ok {
		m := dialogueMap(sys)
		if m == nil {
			return Dialogue{}, fmt.Errorf("systemInstruction must be object")
		}
		if err := dialogueFields(m, "role", "parts"); err != nil {
			return Dialogue{}, err
		}
		text, err := googleText(m["parts"])
		if err != nil {
			return Dialogue{}, err
		}
		messages = append(messages, map[string]any{"role": "system", "content": text})
	}
	names := map[string][]string{}
	used := map[string]bool{}
	contents, ok := body["contents"].([]any)
	if !ok || len(contents) == 0 {
		return Dialogue{}, fmt.Errorf("nonempty contents is required")
	}
	for turnIndex, raw := range contents {
		c := dialogueMap(raw)
		if err := dialogueFields(c, "role", "parts"); err != nil {
			return Dialogue{}, err
		}
		role := dialogueString(c["role"])
		if role == "" {
			role = "user"
		}
		if role == "model" {
			role = "assistant"
		}
		if role != "user" && role != "assistant" {
			return Dialogue{}, fmt.Errorf("invalid Google content role")
		}
		parts, ok := c["parts"].([]any)
		if !ok || len(parts) == 0 {
			return Dialogue{}, fmt.Errorf("nonempty Google parts required")
		}
		text := ""
		calls := []any{}
		for i, raw := range parts {
			p := dialogueMap(raw)
			if len(p) != 1 {
				return Dialogue{}, fmt.Errorf("unsupported Google part")
			}
			switch {
			case p["text"] != nil:
				s, ok := p["text"].(string)
				if !ok {
					return Dialogue{}, fmt.Errorf("Google text must be string")
				}
				text += s
			case p["functionCall"] != nil:
				f := dialogueMap(p["functionCall"])
				if err := dialogueFields(f, "id", "name", "args"); err != nil {
					return Dialogue{}, err
				}
				if role != "assistant" {
					return Dialogue{}, fmt.Errorf("functionCall requires model role")
				}
				if _, ok := f["args"].(map[string]any); !ok {
					return Dialogue{}, fmt.Errorf("function args must be object")
				}
				id := dialogueString(f["id"])
				name := dialogueString(f["name"])
				if id == "" {
					id = fmt.Sprintf("google_%s_%d_%d", name, turnIndex, i)
				}
				if name == "" || used[id] {
					return Dialogue{}, fmt.Errorf("missing function name or duplicate function ID")
				}
				used[id] = true
				names[name] = append(names[name], id)
				args, _ := json.Marshal(f["args"])
				calls = append(calls, map[string]any{"id": id, "type": "function", "function": map[string]any{"name": name, "arguments": string(args)}})
			case p["functionResponse"] != nil:
				f := dialogueMap(p["functionResponse"])
				if err := dialogueFields(f, "id", "name", "response"); err != nil {
					return Dialogue{}, err
				}
				if role != "user" {
					return Dialogue{}, fmt.Errorf("functionResponse requires user role")
				}
				if _, ok := f["response"].(map[string]any); !ok {
					return Dialogue{}, fmt.Errorf("function response must be object")
				}
				id := dialogueString(f["id"])
				if id == "" {
					matches := names[dialogueString(f["name"])]
					if len(matches) != 1 {
						return Dialogue{}, fmt.Errorf("ambiguous Google function response requires explicit ID")
					}
					id = matches[0]
				}
				name := dialogueString(f["name"])
				matched := false
				for i, candidate := range names[name] {
					if candidate == id {
						names[name] = append(names[name][:i], names[name][i+1:]...)
						matched = true
						break
					}
				}
				if !matched {
					return Dialogue{}, fmt.Errorf("Google function response has no pending matching call")
				}
				response, _ := json.Marshal(f["response"])
				messages = append(messages, map[string]any{"role": "tool", "tool_call_id": id, "content": string(response)})
			default:
				return Dialogue{}, fmt.Errorf("unsupported Google content capability")
			}
		}
		if text != "" || len(calls) > 0 {
			m := map[string]any{"role": role, "content": text}
			if len(calls) > 0 {
				m["tool_calls"] = calls
			}
			messages = append(messages, m)
		}
	}
	chat["messages"] = messages
	if raw, exists := body["generationConfig"]; exists {
		cfg := dialogueMap(raw)
		if cfg == nil {
			return Dialogue{}, fmt.Errorf("Google configuration must be object")
		}
		if err := dialogueFields(cfg, "temperature", "topP", "maxOutputTokens", "candidateCount"); err != nil {
			return Dialogue{}, err
		}
		if v, ok := cfg["candidateCount"]; ok {
			if n, valid := dialogueNumber(v); !valid || n != 1 {
				return Dialogue{}, fmt.Errorf("only one Google candidate supported")
			}
		}
		for source, target := range map[string]string{"temperature": "temperature", "topP": "top_p", "maxOutputTokens": "max_tokens"} {
			if v, ok := cfg[source]; ok {
				chat[target] = v
			}
		}
	}
	tools := []any{}
	if raw, exists := body["tools"]; exists {
		groups, ok := raw.([]any)
		if !ok {
			return Dialogue{}, fmt.Errorf("tools must be array")
		}
		for _, raw := range groups {
			group := dialogueMap(raw)
			if group == nil {
				return Dialogue{}, fmt.Errorf("Google tool must be object")
			}
			if err := dialogueFields(group, "functionDeclarations"); err != nil {
				return Dialogue{}, err
			}
			decls, ok := group["functionDeclarations"].([]any)
			if !ok {
				return Dialogue{}, fmt.Errorf("functionDeclarations must be array")
			}
			for _, raw := range decls {
				f := dialogueMap(raw)
				if err := dialogueFields(f, "name", "description", "parameters"); err != nil {
					return Dialogue{}, err
				}
				tools = append(tools, map[string]any{"type": "function", "function": f})
			}
		}
	}
	if len(tools) > 0 {
		chat["tools"] = tools
	}
	if raw, exists := body["toolConfig"]; exists {
		cfg := dialogueMap(raw)
		if cfg == nil {
			return Dialogue{}, fmt.Errorf("Google configuration must be object")
		}
		if err := dialogueFields(cfg, "functionCallingConfig"); err != nil {
			return Dialogue{}, err
		}
		f := dialogueMap(cfg["functionCallingConfig"])
		if f == nil {
			return Dialogue{}, fmt.Errorf("functionCallingConfig must be object")
		}
		if err := dialogueFields(f, "mode", "allowedFunctionNames"); err != nil {
			return Dialogue{}, err
		}
		mode := dialogueString(f["mode"])
		choice, ok := map[string]string{"AUTO": "auto", "ANY": "required", "NONE": "none"}[mode]
		if !ok {
			return Dialogue{}, fmt.Errorf("unsupported Google function mode")
		}
		if raw, exists := f["allowedFunctionNames"]; exists {
			names := dialogueList(raw)
			if len(names) != 1 || mode != "ANY" {
				return Dialogue{}, fmt.Errorf("only one forced function supported")
			}
			chat["tool_choice"] = map[string]any{"type": "function", "function": map[string]any{"name": names[0]}}
		} else {
			chat["tool_choice"] = choice
		}
	}
	return ParseDialogue("openai-chat", chat)
}

// googleText 读取纯文本 parts；参数为动态数组，返回拼接文字或明确格式错误；系统提示解析调用，无副作用。
func googleText(raw any) (string, error) {
	parts, ok := raw.([]any)
	if !ok {
		return "", fmt.Errorf("Google parts must be array")
	}
	text := ""
	for _, raw := range parts {
		p := dialogueMap(raw)
		if err := dialogueFields(p, "text"); err != nil {
			return "", err
		}
		s, ok := p["text"].(string)
		if !ok {
			return "", fmt.Errorf("Google text must be string")
		}
		text += s
	}
	return text, nil
}

// encodeGoogleDialogue 按 Google 原厂形状编码共享对话；参数为对话，返回正文或不可表达能力错误。
// 上游执行调用；不写 model/stream，模型和流操作由注册路径表达，工具结果按调用 ID 找原名称。
func encodeGoogleDialogue(d Dialogue) (map[string]any, error) {
	out := map[string]any{}
	contents := []any{}
	system := ""
	names := map[string]string{}
	for _, t := range d.Turns {
		if t.Role == "system" || t.Role == "developer" {
			if system != "" {
				system += "\n"
			}
			system += t.Text
			continue
		}
		role := "user"
		if t.Role == "assistant" {
			role = "model"
		}
		parts := []any{}
		if t.Role == "tool" {
			name := names[t.ID]
			if name == "" {
				return nil, fmt.Errorf("Google tool result has no preceding call")
			}
			var response map[string]any
			if json.Unmarshal([]byte(t.Text), &response) != nil || response == nil {
				response = map[string]any{"result": t.Text}
			}
			parts = append(parts, map[string]any{"functionResponse": map[string]any{"id": t.ID, "name": name, "response": response}})
		} else {
			if t.Text != "" {
				parts = append(parts, map[string]any{"text": t.Text})
			}
			for _, c := range t.Calls {
				var args map[string]any
				if json.Unmarshal([]byte(c.Arguments), &args) != nil || args == nil {
					return nil, fmt.Errorf("Google function arguments must be object")
				}
				names[c.ID] = c.Name
				parts = append(parts, map[string]any{"functionCall": map[string]any{"id": c.ID, "name": c.Name, "args": args}})
			}
		}
		contents = append(contents, map[string]any{"role": role, "parts": parts})
	}
	out["contents"] = contents
	if system != "" {
		out["systemInstruction"] = map[string]any{"parts": []any{map[string]any{"text": system}}}
	}
	cfg := map[string]any{}
	for source, target := range map[string]string{"temperature": "temperature", "top_p": "topP", "max_tokens": "maxOutputTokens"} {
		if v, ok := d.Options[source]; ok {
			cfg[target] = v
		}
	}
	if len(cfg) > 0 {
		out["generationConfig"] = cfg
	}
	if len(d.Tools) > 0 {
		decls := []any{}
		for _, f := range d.Tools {
			if f["strict"] != nil {
				return nil, fmt.Errorf("Google cannot express strict tools")
			}
			cp := map[string]any{}
			for k, v := range f {
				cp[k] = v
			}
			decls = append(decls, cp)
		}
		out["tools"] = []any{map[string]any{"functionDeclarations": decls}}
	}
	if _, ok := d.Options["tool_choice"]; ok {
		chat, err := EncodeDialogue(d, "openai-chat", "")
		if err != nil {
			return nil, err
		}
		choice := chat["tool_choice"]
		f := map[string]any{}
		if s, ok := choice.(string); ok {
			f["mode"] = map[string]string{"auto": "AUTO", "required": "ANY", "none": "NONE"}[s]
		} else {
			f["mode"] = "ANY"
			f["allowedFunctionNames"] = []any{dialogueMap(dialogueMap(choice)["function"])["name"]}
		}
		out["toolConfig"] = map[string]any{"functionCallingConfig": f}
	}
	return out, nil
}

// parseGoogleDialogueResult 提取单候选文本、函数调用和实测用量；参数为原厂响应，返回结果或格式/安全阻断错误。
// 同步与流解码调用；缺失用量禁止计价，未知内容不静默丢弃，工具 ID 缺失时按名称和位置关联。
func parseGoogleDialogueResult(raw []byte) (DialogueResult, error) {
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return DialogueResult{}, err
	}
	r := DialogueResult{ID: dialogueString(m["responseId"]), Stop: "stop"}
	if m["error"] != nil {
		return r, fmt.Errorf("Google upstream error")
	}
	candidates := dialogueList(m["candidates"])
	if len(candidates) != 1 {
		return r, fmt.Errorf("one Google candidate required")
	}
	c := dialogueMap(candidates[0])
	stop := dialogueString(c["finishReason"])
	if stop == "MAX_TOKENS" {
		r.Stop = "length"
	} else if stop != "" && stop != "STOP" {
		return r, fmt.Errorf("Google candidate blocked: %s", stop)
	}
	for i, raw := range dialogueList(dialogueMap(c["content"])["parts"]) {
		p := dialogueMap(raw)
		if len(p) != 1 {
			return r, fmt.Errorf("unsupported Google response part")
		}
		if text, ok := p["text"].(string); ok {
			r.Text += text
		} else if f := dialogueMap(p["functionCall"]); f != nil {
			if _, ok := f["args"].(map[string]any); !ok {
				return r, fmt.Errorf("invalid Google function args")
			}
			args, _ := json.Marshal(f["args"])
			name := dialogueString(f["name"])
			if name == "" {
				return r, fmt.Errorf("missing Google function name")
			}
			id := dialogueString(f["id"])
			if id == "" {
				id = fmt.Sprintf("google_%s_%d", name, i)
			}
			r.Calls = append(r.Calls, Turn{ID: id, Name: name, Arguments: string(args)})
		} else {
			return r, fmt.Errorf("unsupported Google response capability")
		}
	}
	u := dialogueMap(m["usageMetadata"])
	usage := map[string]any{}
	for source, target := range map[string]string{"promptTokenCount": "prompt_tokens", "candidatesTokenCount": "completion_tokens", "cachedContentTokenCount": "cache_read_input_tokens"} {
		if v, ok := u[source]; ok {
			usage[target] = v
		}
	}
	if candidate, exists := u["candidatesTokenCount"]; exists {
		n, ok := dialogueNumber(candidate)
		if !ok || n < 0 || n != float64(int64(n)) {
			return r, fmt.Errorf("invalid Google candidate token count")
		}
		if thought, exists := u["thoughtsTokenCount"]; exists {
			v, ok := dialogueNumber(thought)
			if !ok || v < 0 || v != float64(int64(v)) {
				return r, fmt.Errorf("invalid Google thoughts token count")
			}
			n += v
		}
		usage["completion_tokens"] = n
	}
	r.Usage = NormalizeDialogueUsage(usage)
	if len(r.Calls) > 0 {
		r.Stop = "tool_calls"
	}
	return r, nil
}

// encodeGoogleDialogueResult 输出 Google 单候选结果；参数为共享回复，返回 JSON 或非法工具参数错误。
// 同步和流终态调用；保持实测用量，安全或失败状态不制造成功结果。
func encodeGoogleDialogueResult(r DialogueResult) ([]byte, error) {
	parts := []any{}
	if r.Text != "" {
		parts = append(parts, map[string]any{"text": r.Text})
	}
	for _, c := range r.Calls {
		var args map[string]any
		if json.Unmarshal([]byte(c.Arguments), &args) != nil || args == nil {
			return nil, fmt.Errorf("invalid Google tool args")
		}
		parts = append(parts, map[string]any{"functionCall": map[string]any{"id": c.ID, "name": c.Name, "args": args}})
	}
	stop := "STOP"
	if r.Stop == "length" {
		stop = "MAX_TOKENS"
	}
	u := map[string]any{}
	for source, target := range map[string]string{"prompt_tokens": "promptTokenCount", "completion_tokens": "candidatesTokenCount", "total_tokens": "totalTokenCount"} {
		if v, ok := r.Usage[source]; ok {
			u[target] = v
		}
	}
	if d := dialogueMap(r.Usage["prompt_tokens_details"]); d != nil {
		u["cachedContentTokenCount"] = d["cached_tokens"]
	}
	return json.Marshal(map[string]any{"responseId": r.ID, "candidates": []any{map[string]any{"index": 0, "content": map[string]any{"role": "model", "parts": parts}, "finishReason": stop}}, "usageMetadata": u})
}
