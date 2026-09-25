// 请求发出前执行护栏。命中拦截时数据面不再访问上游。
package guard

import (
	"net/http"
	"strings"
	"time"

	"github.com/sunqirui1987/xhub/internal/httpx"
)

// 管理接口上的护栏试跑。不改存储。
func Apply(s Host, w http.ResponseWriter, r *http.Request) {
	if s.RequireManage(w, r) == nil {
		return
	}
	body := readMap(r)
	name := str(body["guardrail_name"])
	if name == "" {
		name = str(body["guardrail"])
	}
	text := guardrailText(body)
	action, out := evalNamed(s, name, text)
	httpx.WriteJSON(w, 200, map[string]any{
		"status":         "success",
		"guardrail_name": name,
		"guardrail_id":   name,
		"created_at":     time.Now().UTC().Format(time.RFC3339),
		"action":         action,
		"blocked":        action == "block",
		"text":           out,
		"output":         map[string]any{"text": out},
	})
}

// 聊天发出前跑护栏。拦截时返回 true 和原因，数据面不再访问上游。
func PreCall(s Host, body map[string]any) (bool, string) {
	text := guardrailText(body)
	list := listGuardrails(s)
	for _, g := range list {
		params, _ := g["litellm_params"].(map[string]any)
		if params == nil {
			params = map[string]any{}
		}
		if !boolOf(params["default_on"]) && !boolOf(g["default_on"]) {
			continue
		}
		mode := str(params["mode"])
		if mode != "" && mode != "pre_call" {
			continue
		}
		action, _ := matchGuardrail(g, text)
		if action == "block" {
			name := str(g["guardrail_name"])
			if name == "" {
				name = str(g["name"])
			}
			if name == "" {
				name = "guardrail"
			}
			return true, "Guardrail blocked the request: " + name
		}
	}
	return false, ""
}

// 按名字执行一条护栏。找不到时视为通过。
func evalNamed(s Host, name, text string) (action, out string) {
	if name != "" {
		for _, g := range listGuardrails(s) {
			n := str(g["guardrail_name"])
			if n == "" {
				n = str(g["name"])
			}
			if n == name || str(g["id"]) == name || str(g["guardrail_id"]) == name {
				return matchGuardrail(g, text)
			}
		}
	}
	for _, g := range listGuardrails(s) {
		if boolOf(g["default_on"]) {
			return matchGuardrail(g, text)
		}
		if params, _ := g["litellm_params"].(map[string]any); boolOf(params["default_on"]) {
			return matchGuardrail(g, text)
		}
	}
	return "allow", text
}

// 当前配置的护栏。没有配置时为空切片。
func listGuardrails(s Host) []map[string]any {
	var out []map[string]any
	for _, kind := range []string{"guardrails", "guardrail"} {
		list, _ := s.DB().ListKV(kind)
		out = append(out, list...)
	}
	if out == nil {
		out = []map[string]any{}
	}
	return out
}

// 用护栏规则检查一段文本，返回动作和替换后的文本。
func matchGuardrail(g map[string]any, text string) (action, out string) {
	params, _ := g["litellm_params"].(map[string]any)
	if params == nil {
		params = map[string]any{}
	}
	kind := str(params["guardrail"])
	if kind == "" {
		kind = str(g["guardrail"])
	}
	low := strings.ToLower(text)
	out = text
	if kind == "block" || kind == "always_block" {
		return "block", out
	}
	words := extraWords(params["blocked_words"])
	words = append(words, extraWords(params["keywords"])...)
	words = append(words, extraWords(g["blocked_words"])...)
	for _, w := range words {
		if w != "" && strings.Contains(low, strings.ToLower(w)) {
			if kind == "redact" || str(params["mode"]) == "redact" {
				return "redact", strings.ReplaceAll(out, w, "[REDACTED]")
			}
			return "block", out
		}
	}
	return "allow", out
}

// 从配置值里取出额外敏感词。不是列表时为空。
func extraWords(v any) []string {
	var out []string
	switch t := v.(type) {
	case []any:
		for _, x := range t {
			if s, ok := x.(string); ok && s != "" {
				out = append(out, s)
			}
		}
	case []string:
		out = append(out, t...)
	case string:
		if t != "" {
			out = append(out, t)
		}
	}
	return out
}

// 从聊天正文抽出要检查的文本。
func guardrailText(body map[string]any) string {
	if t := str(body["text"]); t != "" {
		return t
	}
	if t := str(body["input"]); t != "" {
		return t
	}
	if t := str(body["prompt"]); t != "" {
		return t
	}
	var b strings.Builder
	if msgs, ok := body["messages"].([]any); ok {
		for _, raw := range msgs {
			if m, ok := raw.(map[string]any); ok {
				b.WriteString(str(m["content"]))
				b.WriteByte(' ')
			}
		}
	}
	return strings.TrimSpace(b.String())
}
