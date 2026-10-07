// Package llm converts chat messages between the OpenAI shape and a provider shape.
package llm

import (
	"github.com/sunqirui1987/xhub/internal/logx"
	"strings"
	"sync"
)

var logTraceOnceMessage sync.Once

// IsWildcardModel reports whether a model name is a wildcard route.  LiteLLM uses * in a name to mean a group of models, so openai/* can catch openai/gpt-4o. An exact name wins. The router looks at these patterns only when no exact deployment exists.
// 参数 model（string）：对外模型名，用来选部署和记用量。
// 返回 bool（bool）：模型名里含 * 时为真，表示一组模型而不是一条精确部署。
// 调用：router/router.go
// 测试：无直接单测
func IsWildcardModel(model string) bool {
	logTraceOnceMessage.Do(func() { logx.Trace("enter llm.IsWildcardModel") })

	return strings.Contains(model, "*")
}

// NormalizeStreamOptions reads include_obfuscation from Responses stream options. The field must be a boolean. A missing field, a wrong type, or options that are not an object all mean the caller did not give a usable switch, so ok is false. Do not treat the default as false and write it back onto the request.
// 参数 opts（map[string]any）：这一步的可选参数。缺键表示用默认。
// 返回 includeObfuscation（bool）：include_obfuscation 被设成布尔真时返回真；ok（bool）：该字段确实是布尔值时返回真。缺字段或类型不对时返回假，不要把缺省写回请求。
// 调用：仅在 message.go 内使用
// 测试：无直接单测
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

// ShapeResponsesMessage reshapes one chat message into a form the Responses API accepts. An assistant message stays as it is. A user message whose content is a list of blocks, with a type of text, rewrites those blocks to input_text. Responses does not accept the chat-completion text type. Without such a block the message is returned unchanged, so a message that is already input_text is not wrappedagain.
// 参数 message（map[string]any）：ShapeResponses消息读到的 JSON 对象。缺键表示没有该字段。
// 返回 map[string]any（map[string]any）：给响应或报表用的 JSON 对象。键是前端已经约定的字段，缺键表示这项没有数据。
// 调用：llm/call.go
// 测试：无直接单测
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

// hasChatText reports whether the message content has at least one text part. A tool-only call does not count as text.
// 参数 content（[]any）：是否有对话文本使用的any。
// 返回 bool（bool）：消息内容里至少有一段文本时返回真。只有工具调用不算有文本。
// 调用：仅在 message.go 内使用
// 测试：无直接单测
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

// asInputText turns one content part into input text. A non-text part is left as it is for the caller to decide.
// 参数 part（any）：作为输入文本接到的动态值。类型在函数体内收窄。
// 返回 any（any）：作为输入文本的结果。具体类型由调用方断言。
// 调用：仅在 message.go 内使用
// 测试：无直接单测
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
