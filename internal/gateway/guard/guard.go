// Package guard runs guardrails before a request is sent. A blocking match stops the data plane from calling the upstream.
package guard

import (
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/sunqirui1987/xhub/internal/httpx"
	"github.com/sunqirui1987/xhub/internal/logx"
	"sync"
)

var logTraceOnceGuard sync.Once

// Apply is the management trial for a guardrail. It does not change storage.
// 参数 s（Host）：应用使用的数据面宿主；w（http.ResponseWriter）：调用方的 HTTP 响应，状态和正文写在这里；r（*http.Request）：入站 HTTP 请求。
// 返回：无。状态码和正文写进调用方的响应。
// 调用：authz/scope.go、gateway/guard/mount.go
// 测试：authz_test.go、guard_test.go
func Apply(s Host, w http.ResponseWriter, r *http.Request) {
	logTraceOnceGuard.Do(func() { logx.Trace("enter guard.Apply") })

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
		// response_text is the field the console's test playground renders.
		"response_text": out,
		"output":        map[string]any{"text": out},
	})
}

// PreCall runs guardrails before chat is sent. A block returns true and a reason, and the data plane does not call the upstream.
// 参数 s（Host）：Pre调用使用的数据面宿主；body（map[string]any）：已解析或原始的 JSON。
// 返回 bool（bool）：护栏拦截了这次聊天时返回真，数据面不再访问上游；string（string）：给调用方看的拦截说明。未拦截时为空串。
// 调用：仅在 guard.go 内使用
// 测试：guard_test.go
func PreCall(s Host, body map[string]any) (bool, string) {
	blocked, message, _ := Evaluate(s, body)
	return blocked, message
}

// Evaluate runs every default-on pre-call rule and reports each one. The logs drawer reads these rows as guardrail monitoring. A block stops the scan: later rules did not run, so they are not reported as if they had.
// 参数 s（Host）：Evaluate使用的数据面宿主；body（map[string]any）：已解析或原始的 JSON。
// 返回 blocked（bool）：有规则要求拦截时返回真。拦截会停掉后续规则，那些规则不会被记成已经执行；message（string）：给调用方看的拦截说明。没有拦截时为空串；findings（[]map[string]any）：已经跑过的护栏结果。没有规则命中时为空切片。
// 调用：gateway/wire.go
// 测试：guard_test.go
func Evaluate(s Host, body map[string]any) (blocked bool, message string, findings []map[string]any) {
	text := guardrailText(body)
	for _, g := range listGuardrails(s) {
		params, _ := g["litellm_params"].(map[string]any)
		if params == nil {
			params = map[string]any{}
		}
		if !boolOf(params["default_on"]) && !boolOf(g["default_on"]) {
			continue
		}
		mode := str(params["mode"])
		if mode != "" && mode != "pre_call" && mode != "redact" {
			continue
		}
		action, out := matchGuardrail(g, text)
		findings = append(findings, findingJSON(g, action, out != text))
		if action == "block" {
			return true, "Guardrail blocked the request: " + findingName(g), findings
		}
		if action == "redact" {
			visitRequestText(body, func(value string) string {
				_, masked := matchGuardrail(g, value)
				return masked
			})
			text = guardrailText(body)
		}
	}
	return false, "", findings
}

// 从护栏结果里取名称，没有时用 guardrail。
// 参数 g（map[string]any）：发现名称读到的 JSON 对象。缺键表示没有该字段。
// 返回 string（string）：护栏名称。上游没给时用 guardrail。
// 调用：仅在 guard.go 内使用
// 测试：无直接单测
func findingName(g map[string]any) string {
	name := str(g["guardrail_name"])
	if name == "" {
		name = str(g["name"])
	}
	if name == "" {
		name = "guardrail"
	}
	return name
}

// 把一次护栏结果收成日志对象，按放行、拦截或打码填写状态。
// 参数 g（map[string]any）：发现JSON读到的 JSON 对象。缺键表示没有该字段；action（string）：护栏或鉴权动作，例如 block、redact、allow；redacted（bool）：为真时护栏改写了正文，响应里要标出。
// 返回 map[string]any（map[string]any）：给响应或报表用的 JSON 对象。键是前端已经约定的字段，缺键表示这项没有数据。
// 调用：仅在 guard.go 内使用
// 测试：无直接单测
func findingJSON(g map[string]any, action string, redacted bool) map[string]any {
	status := "success"
	switch action {
	case "block":
		status = "blocked"
	case "redact":
		status = "guardrail_flagged"
	}
	id := str(g["guardrail_id"])
	if id == "" {
		id = str(g["id"])
	}
	now := float64(time.Now().UnixNano()) / 1e9
	masked := map[string]any{}
	if redacted {
		masked["redacted"] = 1
	}
	return map[string]any{
		"guardrail_id":        id,
		"guardrail_name":      findingName(g),
		"guardrail_mode":      "pre_call",
		"guardrail_provider":  "xhub",
		"guardrail_status":    status,
		"start_time":          now,
		"end_time":            now,
		"duration":            0,
		"guardrail_response":  map[string]any{"action": action},
		"masked_entity_count": masked,
	}
}

// evalNamed runs one guardrail by name. A missing name is treated as a pass.
// 参数 s（Host）：eval命名使用的数据面宿主；name（string）：eval命名要查找或展示的名称。空串表示还没有命名；text（string）：要拼接或展示的文本。空串表示这段没有内容。
// 返回 action（string）：固定文本 "allow"；out（string）：固定文本 "allow"。
// 调用：仅在 guard.go 内使用
// 测试：guard_test.go
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

// listGuardrails returns the guardrails in the current config. An unconfigured proxy returns an empty slice.
// 参数 s（Host）：列出Guardrails使用的数据面宿主。
// 返回 []map[string]any（[]map[string]any）：一组map[string]any。没有匹配时为空切片，不是 nil 分页。
// 调用：仅在 guard.go 内使用
// 测试：无直接单测
func listGuardrails(s Host) []map[string]any {
	if s == nil || s.RecordStore() == nil {
		logx.Debug("guardrails skipped reason=no store")
		return []map[string]any{}
	}
	var out []map[string]any
	for _, kind := range []string{"guardrails", "guardrail"} {
		list, err := s.RecordStore().ListKV(kind)
		if err != nil {
			logx.Error("guardrail lookup failed kind=%s err=%v", kind, err)
			return []map[string]any{{"name": "guardrail_store_unavailable", "default_on": true, "guardrail": "always_block"}}
		}
		out = append(out, list...)
	}
	if out == nil {
		out = []map[string]any{}
	}
	return out
}

// matchGuardrail checks text against one guardrail and returns the action and the replaced text.
// 参数 g（map[string]any）：匹配护栏读到的 JSON 对象。缺键表示没有该字段；text（string）：要拼接或展示的文本。空串表示这段没有内容。
// 返回 action（string）：allow、block 或 redact；out（string）：原文，或把命中词换成 [REDACTED] 之后的文本。
// 调用：仅在 guard.go 内使用
// 测试：guard_test.go
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
				pattern := regexp.MustCompile("(?i)" + regexp.QuoteMeta(w))
				out = pattern.ReplaceAllString(out, "[REDACTED]")
				action = "redact"
				continue
			}
			return "block", out
		}
	}
	if action == "redact" {
		return action, out
	}
	return "allow", out
}

// extraWords reads extra sensitive words from a config value. A non-list is empty.
// 参数 v（any）：JSON 里读出的动态值。数字、字符串和对象都要接住，类型不符时按零值而不是 panic。
// 返回 []string（[]string）：配置里的敏感词。值不是字符串、字符串列表或数组时为空切片。
// 调用：仅在 guard.go 内使用
// 测试：guard_test.go
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

// guardrailText extracts the text a chat body should be checked against.
// 参数 body（map[string]any）：已经解析的 JSON 对象。缺字段表示上游或调用方没有给这项。
// 返回 string（string）：所有支持的文本字段，按协议字段顺序拼接。
// 调用：仅在 guard.go 内使用
// 测试：guard_test.go
func guardrailText(body map[string]any) string {
	var parts []string
	visitRequestText(body, func(value string) string {
		if value != "" {
			parts = append(parts, value)
		}
		return value
	})
	return strings.Join(parts, " ")
}

// visitRequestText visits protocol text fields while leaving roles, image URLs
// and other metadata intact. The visitor may replace each text leaf in place.
// 参数 body：请求正文；visit：文本转换函数。返回：无。
// 调用：Evaluate、guardrailText。测试：guard_test.go。
func visitRequestText(body map[string]any, visit func(string) string) {
	var walk func(any) any
	walk = func(value any) any {
		switch v := value.(type) {
		case string:
			return visit(v)
		case []any:
			for i := range v {
				v[i] = walk(v[i])
			}
		case map[string]any:
			for _, key := range []string{"text", "content", "parts"} {
				if child, ok := v[key]; ok {
					v[key] = walk(child)
				}
			}
		}
		return value
	}
	for _, key := range []string{"text", "input", "prompt", "messages", "contents", "system", "instructions", "systemInstruction"} {
		if value, ok := body[key]; ok {
			body[key] = walk(value)
		}
	}
}
