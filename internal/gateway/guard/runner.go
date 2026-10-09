package guard

import (
	"encoding/json"
	"fmt"
	"github.com/sunqirui1987/xhub/internal/config"
)

// priority 读取 JSON 或 YAML 数值优先级，不支持类型按零处理。
// 参数：v：int 或 float64 动态值；严格范围由 Validate 检查。
// 返回：float64 优先级，默认 0。
// 调用：Validate、sortRules、Azure 阈值校验。
// 测试：TestSelectionDefaultsAndOrdering。
func priority(v any) float64 {
	switch n := v.(type) {
	case int:
		return float64(n)
	case float64:
		return n
	}
	return 0
}

// requestedRules 解析请求指定的护栏名称或 ID，拒绝每请求参数覆盖对象。
// 参数：v：nil、兼容单字符串、字符串切片或 JSON 数组。
// 返回：名称列表及错误；空数组合法，元素非字符串或空字符串非法。
// 调用：Evaluate 的顶层及 metadata.guardrails 选择。
// 测试：TestSelectionDefaultsAndOrdering。
func requestedRules(v any) ([]string, error) {
	if v == nil {
		return nil, nil
	}
	switch list := v.(type) {
	case string:
		if list != "" {
			return []string{list}, nil
		}
	case []string:
		return list, nil
	case []any:
		out := []string{}
		for _, row := range list {
			name, ok := row.(string)
			if !ok || name == "" {
				return nil, fmt.Errorf("guardrails must be an array of names")
			}
			out = append(out, name)
		}
		return out, nil
	}
	return nil, fmt.Errorf("guardrails must be an array of names; per-request parameter overrides are not supported")
}

// runRule 提供不需要 Flag 元数据的统一规则执行包装。
// 参数：g：规则对象；body：可被脱敏修改的请求正文。
// 返回：action、reason、err：统一执行结果。
// 调用：兼容测试及辅助调用。
// 测试：engine_test.go、external_test.go。
func runRule(g, body map[string]any) (action, reason string, err error) {
	action, reason, _, err = runRuleDetailed(g, body)
	return
}

// runRuleDetailed 统一提取可见文本并分发本地、XGo、秘密检测和外部适配器，再校验数量和大小后回写。
// 参数：g：完整规则；body：协议请求对象，会在 modify/redact 成功后原位更新。
// 返回：action、reason、metadata、err：执行动作、说明、Flag 附加数据和错误；失败返回 block。
// 调用：Evaluate、Apply、runRule。
// 测试：engine_test.go、external_test.go、primitives_test.go。
func runRuleDetailed(g, body map[string]any) (action, reason string, metadata map[string]any, err error) {
	p, _ := g["litellm_params"].(map[string]any)
	texts := []string{}
	visitGuardedText(body, p, func(text string) string { texts = append(texts, text); return text })
	size := 0
	for _, text := range texts {
		size += len(text)
	}
	if size > maxCustomText || len(texts) > 10000 {
		return "block", "", nil, fmt.Errorf("guardrail input too large")
	}
	out := append([]string{}, texts...)
	action = "allow"
	if str(p["guardrail"]) == "custom_code" {
		action, reason, out, metadata, err = runCustomDetailed(str(p["custom_code"]), texts, body, "request")
		if err != nil {
			return "block", "", nil, err
		}
	} else if str(p["guardrail"]) == "hide-secrets" {
		action, reason, out = runSecrets(p, texts)
	} else if _, ok := externalProviders[str(p["guardrail"])]; ok {
		action, reason, out, err = runExternal(p, texts)
		if err != nil {
			return "block", "", nil, err
		}
	} else {
		match := localMatcher(g)
		if len(texts) == 0 {
			action, _ = match("")
		}
		for i, text := range texts {
			a, value := match(text)
			if a == "block" {
				return "block", "", nil, nil
			}
			if a == "redact" {
				action = "redact"
				out[i] = value
			}
		}
	}
	if action == "modify" || action == "redact" {
		if len(out) != len(texts) {
			return "block", "", nil, fmt.Errorf("guardrail output count mismatch")
		}
		size = 0
		for _, text := range out {
			size += len(text)
		}
		if size > maxCustomText {
			return "block", "", nil, fmt.Errorf("guardrail output too large")
		}
		i := 0
		visitGuardedText(body, p, func(string) string { v := out[i]; i++; return v })
	}
	return action, reason, metadata, nil
}

// configuredRules 读取并复制 YAML 规则，验证配置与名称唯一性，避免修改原始配置。
// 参数：s：可选实现 GatewayConfig 的宿主。
// 返回：规则数组；非法 YAML 返回默认拦截哨兵，而非静默丢弃。
// 调用：Manage、storedRules、listGuardrails。
// 测试：TestInvalidYAMLDoesNotSilentlyDisableGuardrails。
func configuredRules(s Host) []map[string]any {
	out := []map[string]any{}
	seen := map[string]bool{}
	if h, ok := s.(interface{ GatewayConfig() *config.Config }); ok && h.GatewayConfig() != nil {
		for _, rule := range h.GatewayConfig().Guardrails {
			raw, err := json.Marshal(rule)
			var copy map[string]any
			if err != nil || json.Unmarshal(raw, &copy) != nil || copy == nil {
				return invalidConfiguredRule("guardrail YAML must be a serializable object")
			}
			copy["id"] = findingName(copy)
			copy["guardrail_id"] = findingName(copy)
			copy["guardrail_definition_location"] = "config"
			copy = normalizeRule(copy)
			if err := Validate(copy); err != nil {
				return invalidConfiguredRule(err.Error())
			}
			name := findingName(copy)
			if seen[name] {
				return invalidConfiguredRule("duplicate YAML guardrail name: " + name)
			}
			seen[name] = true
			out = append(out, copy)
		}
	}
	return out
}

// effectiveDefaults 为未显式配置的跳过消息选项应用 litellm_settings 默认值。
// 参数：s：配置宿主；rows：供本次执行使用的规则副本。
// 返回：无；显式 false 保持，nil 或缺失才继承；不改写原始 YAML。
// 调用：listGuardrails。
// 测试：TestRoleInheritanceAndPositionalModification。
func effectiveDefaults(s Host, rows []map[string]any) {
	h, ok := s.(interface{ GatewayConfig() *config.Config })
	if !ok || h.GatewayConfig() == nil {
		return
	}
	defaults := h.GatewayConfig().LiteLLMSettings
	for _, row := range rows {
		p, _ := row["litellm_params"].(map[string]any)
		if p == nil {
			p = map[string]any{}
			row["litellm_params"] = p
		}
		for _, key := range []string{"skip_system_message_in_guardrail", "skip_tool_message_in_guardrail"} {
			if value, exists := p[key]; !exists || value == nil {
				if v, yes := defaults[key]; yes {
					p[key] = v
				}
			}
		}
	}
}

// invalidConfiguredRule 构造默认启用的失败哨兵，保证配置错误不会绕过保护。
// 参数：reason：配置或存储错误说明。
// 返回：包含 always_block 的单元素规则数组，供正常规则链执行。
// 调用：configuredRules、uniqueRules、listGuardrails。
// 测试：TestInvalidYAMLDoesNotSilentlyDisableGuardrails、TestDuplicateNamesAcrossYAMLAndStoreFailClosed。
func invalidConfiguredRule(reason string) []map[string]any {
	return []map[string]any{{"id": "invalid_guardrail_config", "guardrail_id": "invalid_guardrail_config", "guardrail_name": "invalid_guardrail_config", "guardrail_definition_location": "config", "validation_error": reason, "litellm_params": map[string]any{"guardrail": "always_block", "default_on": true}}}
}

// uniqueRules 检查名称及 ID 的所有别名是否跨规则冲突。
// 参数：rows：YAML 与数据库合并后的规则。
// 返回：无冲突返回原数组；冲突返回默认拦截哨兵。
// 调用：listGuardrails。
// 测试：TestDuplicateNamesAcrossYAMLAndStoreFailClosed。
func uniqueRules(rows []map[string]any) []map[string]any {
	seen := map[string]bool{}
	for _, row := range rows {
		aliases := map[string]bool{}
		for _, key := range []string{"guardrail_name", "name", "guardrail_id", "id"} {
			if value := str(row[key]); value != "" {
				aliases[value] = true
			}
		}
		for alias := range aliases {
			if seen[alias] {
				return invalidConfiguredRule("ambiguous guardrail name or id: " + alias)
			}
			seen[alias] = true
		}
	}
	return rows
}
