package llm

import "strings"

// IsWildcardModel 判断模型名是不是通配路由。
//
// LiteLLM 用名字里的 * 表示一组模型，例如 openai/* 可以接住 openai/gpt-4o。
// 精确名字优先；只有没有精确部署时，路由才来看这些模式。
func IsWildcardModel(model string) bool {
	return strings.Contains(model, "*")
}

// NormalizeStreamOptions 读取 Responses 流式选项里的 include_obfuscation。
//
// 这个字段必须是布尔值。缺字段、类型不对、或者整个选项不是对象，都表示
// 调用方没有给出可用的开关，返回 ok=false。不要把缺省当成 false 再写回请求。
func NormalizeStreamOptions(opts map[string]any) (includeObfuscation bool, ok bool) {
	if opts == nil {
		return false, false
	}
	value, exists := opts["include_obfuscation"]
	if !exists {
		return false, false
	}
	flag, isBool := value.(bool)
	if !isBool {
		return false, false
	}
	return flag, true
}

// ShapeResponsesMessage 把一条聊天消息收成 Responses API 能接受的形状。
//
// 助手消息保持原样。用户消息如果内容是一块块的列表，并且其中有 type=text，
// 这些块要改成 input_text。Responses 不接受聊天补全的 text 类型。
// 没有这种块时原样返回，避免把已经是 input_text 的消息再包一层。
func ShapeResponsesMessage(message map[string]any) map[string]any {
	if message == nil {
		return nil
	}
	if role, _ := message["role"].(string); role == "assistant" {
		return message
	}
	content, ok := message["content"].([]any)
	if !ok {
		return message
	}
	if !hasChatText(content) {
		return message
	}
	shaped := make([]any, len(content))
	for i, part := range content {
		shaped[i] = asInputText(part)
	}
	out := map[string]any{}
	for key, value := range message {
		out[key] = value
	}
	out["content"] = shaped
	return out
}

func hasChatText(content []any) bool {
	for _, part := range content {
		item, ok := part.(map[string]any)
		if !ok {
			continue
		}
		kind, _ := item["type"].(string)
		if kind == "text" {
			return true
		}
	}
	return false
}

func asInputText(part any) any {
	item, ok := part.(map[string]any)
	if !ok {
		return part
	}
	kind, _ := item["type"].(string)
	if kind != "text" {
		return part
	}
	out := map[string]any{}
	for key, value := range item {
		out[key] = value
	}
	out["type"] = "input_text"
	return out
}
