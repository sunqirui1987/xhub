package guard

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Validate 验证可执行的规则合约，拒绝未实现阶段和配置，XGo 保存时同时编译。
// 参数：g：包含 guardrail_name 和 litellm_params 的完整规则。
// 返回：error：校验失败原因；nil：配置可被本引擎执行。
// 调用：Manage、Apply、configuredRules。
// 测试：custom_test.go、engine_test.go、external_test.go。
func Validate(g map[string]any) error {
	p, ok := g["litellm_params"].(map[string]any)
	if !ok {
		return fmt.Errorf("litellm_params must be an object")
	}
	if strings.TrimSpace(str(g["guardrail_name"])) == "" {
		return fmt.Errorf("guardrail_name is required")
	}
	kind := str(p["guardrail"])
	_, external := externalProviders[kind]
	switch kind {
	case "", "local", "blocked_words", "redact", "block", "always_block", "custom_code":
	default:
		if !external {
			return fmt.Errorf("provider %q is not implemented", kind)
		}
	}
	for _, mode := range extraWords(p["mode"]) {
		if mode != "pre_call" && mode != "redact" {
			return fmt.Errorf("only pre_call is supported")
		}
	}
	if p["mode"] != nil && len(extraWords(p["mode"])) == 0 {
		return fmt.Errorf("mode must be pre_call")
	}
	if str(g["team_id"]) != "" {
		return fmt.Errorf("team-scoped guardrails are not implemented; create a global local rule")
	}
	for _, key := range []string{"api_base", "api_key", "categories", "pii_entities_config"} {
		if value, exists := p[key]; exists && value != nil && !external {
			return fmt.Errorf("%s is not supported by the local engine", key)
		}
	}
	action := str(p["action"])
	if action != "" && action != "block" && action != "redact" {
		return fmt.Errorf("action must be block or redact")
	}
	for _, key := range []string{"default_on", "skip_system_message_in_guardrail", "skip_tool_message_in_guardrail"} {
		if v, exists := p[key]; exists && v != nil {
			if _, ok := v.(bool); !ok {
				return fmt.Errorf("%s must be a boolean", key)
			}
		}
	}
	if v, exists := p["priority"]; exists {
		n := priority(v)
		_, okFloat := v.(float64)
		_, okInt := v.(int)
		ok := okFloat || okInt
		if !ok || n < 0 || n > 10000 || n != float64(int(n)) {
			return fmt.Errorf("priority must be an integer from 0 to 10000")
		}
	}
	if v, exists := p["replacement"]; exists {
		if text, ok := v.(string); !ok || len(text) > 4096 {
			return fmt.Errorf("replacement must be a string of at most 4096 bytes")
		}
	}
	for _, key := range []string{"blocked_words", "keywords", "patterns"} {
		if value, exists := p[key]; exists && value != nil {
			rows, ok := value.([]any)
			if list, yes := value.([]string); yes {
				ok = true
				rows = make([]any, len(list))
				for i, x := range list {
					rows[i] = x
				}
			}
			if !ok {
				return fmt.Errorf("%s must be an array of strings", key)
			}
			if len(rows) > 500 {
				return fmt.Errorf("%s allows at most 500 entries", key)
			}
			for _, row := range rows {
				text, ok := row.(string)
				if !ok || strings.TrimSpace(text) == "" || len(text) > 4096 {
					return fmt.Errorf("%s entries must be non-empty strings of at most 4096 bytes", key)
				}
				if key == "patterns" {
					re, err := regexp.Compile(text)
					if err != nil {
						return fmt.Errorf("invalid regular expression: %v", err)
					}
					if re.MatchString("") {
						return fmt.Errorf("patterns must not match empty text")
					}
				}
			}
		}
	}
	if external {
		return validateExternal(p)
	}
	if kind == "custom_code" {
		if lang := str(p["custom_code_language"]); lang != "" && lang != "xgo" {
			return fmt.Errorf("custom code language must be xgo")
		}
		_, err := compileCustom(str(p["custom_code"]))
		return err
	}
	if kind != "always_block" && kind != "block" && len(extraWords(p["blocked_words"]))+len(extraWords(p["keywords"]))+len(extraWords(g["blocked_words"]))+len(extraWords(p["patterns"])) == 0 {
		return fmt.Errorf("add at least one keyword or regular expression")
	}
	return nil
}

// localMatcher 构建本地关键词与 RE2 正则匹配器，关键词转义并优先匹配较长词。
// 参数：g：完整规则；包含 action、blocked_words/keywords、patterns、replacement。
// 返回：转换函数：返回 allow/block/redact 和原文或脱敏文本；无效类型或表达式按拦截处理。
// 调用：runRuleDetailed、matchGuardrail。
// 测试：engine_test.go 的消息边界和位置测试。
func localMatcher(g map[string]any) func(string) (string, string) {
	p, _ := g["litellm_params"].(map[string]any)
	kind := str(p["guardrail"])
	if kind == "" {
		kind = str(g["guardrail"])
	}
	if kind == "block" || kind == "always_block" {
		return func(text string) (string, string) { return "block", text }
	}
	switch kind {
	case "", "local", "blocked_words", "redact":
	default:
		return func(text string) (string, string) { return "block", text }
	}
	action := str(p["action"])
	if action == "" {
		action = "block"
		if kind == "redact" || str(p["mode"]) == "redact" {
			action = "redact"
		}
	}
	words := append(extraWords(p["blocked_words"]), extraWords(p["keywords"])...)
	words = append(words, extraWords(g["blocked_words"])...)
	sort.SliceStable(words, func(i, j int) bool { return len(words[i]) > len(words[j]) })
	var alternatives []string
	for _, word := range words {
		if word != "" {
			alternatives = append(alternatives, regexp.QuoteMeta(word))
		}
	}
	if len(alternatives) > 0 {
		alternatives = []string{"(?i:" + strings.Join(alternatives, "|") + ")"}
	}
	for _, pattern := range extraWords(p["patterns"]) {
		re, err := regexp.Compile(pattern)
		if err != nil || re.MatchString("") {
			return func(text string) (string, string) { return "block", text }
		}
		alternatives = append(alternatives, "(?:"+pattern+")")
	}
	if len(alternatives) == 0 {
		return func(text string) (string, string) { return "allow", text }
	}
	re, err := regexp.Compile(strings.Join(alternatives, "|"))
	if err != nil {
		return func(text string) (string, string) { return "block", text }
	}
	replacement := str(p["replacement"])
	if replacement == "" {
		replacement = "[REDACTED]"
	}
	return func(text string) (string, string) {
		if !re.MatchString(text) {
			return "allow", text
		}
		if action != "redact" {
			return "block", text
		}
		return "redact", re.ReplaceAllStringFunc(text, func(string) string { return replacement })
	}
}

// visitGuardedText 过滤需要跳过的系统/工具消息，遍历协议文本后按原位置回写。
// 参数：body：可原地修改的请求；p：已合并全局默认的规则参数；visit：每段文本的检查或转换函数。
// 返回：无；只改变选中的文本字段，保留角色、图片 URL、元数据及被跳过消息。
// 调用：runRuleDetailed、ruleText。
// 测试：TestRoleInheritanceAndPositionalModification、TestModificationPreservesSkippedPositionsAndStringInputs。
func visitGuardedText(body map[string]any, p map[string]any, visit func(string) string) {
	filtered := make(map[string]any, len(body))
	for key, value := range body {
		filtered[key] = value
	}
	if boolOf(p["skip_system_message_in_guardrail"]) {
		for _, key := range []string{"system", "instructions", "systemInstruction"} {
			delete(filtered, key)
		}
	}
	// 记录过滤数组与原数组的下标关系，替换时不会覆盖被跳过消息。
	positions := map[string][]int{}
	for _, key := range []string{"messages", "input", "contents"} {
		if rows, ok := body[key].([]any); ok {
			kept := make([]any, 0, len(rows))
			for index, row := range rows {
				message, _ := row.(map[string]any)
				role := str(message["role"])
				if (role == "system" || role == "developer") && boolOf(p["skip_system_message_in_guardrail"]) {
					continue
				}
				if (role == "tool" || str(message["type"]) == "function_call_output") && boolOf(p["skip_tool_message_in_guardrail"]) {
					continue
				}
				kept = append(kept, row)
				positions[key] = append(positions[key], index)
			}
			filtered[key] = kept
		}
	}
	visitRequestText(filtered, visit)
	for key, indexes := range positions {
		original := body[key].([]any)
		updated := filtered[key].([]any)
		for i, index := range indexes {
			original[index] = updated[i]
		}
	}
	for key, updated := range filtered {
		if key != "messages" && key != "input" && key != "contents" {
			body[key] = updated
			continue
		}
		if _, isString := body[key].(string); isString {
			body[key] = updated
		}
	}

}

// ruleText 提取单条规则可见的非空文本并用空格连接，用于兼容展示。
// 参数：body：请求正文；p：跳过消息等参数；提取回调不改写原文。
// 返回：连接后的文本；不存在文本时为空串。
// 调用：护栏兼容辅助路径。
// 测试：engine_test.go 的角色过滤测试。
func ruleText(body map[string]any, p map[string]any) string {
	var parts []string
	visitGuardedText(body, p, func(text string) string {
		if text != "" {
			parts = append(parts, text)
		}
		return text
	})
	return strings.Join(parts, " ")
}
