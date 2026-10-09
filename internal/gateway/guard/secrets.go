package guard

import "regexp"

// secretPatterns 是可解释的确定性规则子集，覆盖常见密钥、JWT 和私钥。
// 不实现 detect-secrets 的所有插件、熵检测或远端有效性校验。
// secretPatterns 检测明确密钥格式；只覆盖这些确定性模式，不等同于 detect-secrets 全插件。
var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile("\\b(?:AKIA|ASIA)[A-Z0-9]{16}\\b"),
	regexp.MustCompile("\\bsk-(?:proj-|ant-)?[A-Za-z0-9_-]{20,}\\b"),
	regexp.MustCompile("\\b(?:gh[pousr]_[A-Za-z0-9]{20,}|github_pat_[A-Za-z0-9_]{20,})\\b"),
	regexp.MustCompile("\\beyJ[A-Za-z0-9_-]{8,}\\.[A-Za-z0-9_-]{8,}\\.[A-Za-z0-9_-]{8,}\\b"),
	regexp.MustCompile("(?s)-----BEGIN (?:RSA |EC |OPENSSH )?PRIVATE KEY-----.*?-----END (?:RSA |EC |OPENSSH )?PRIVATE KEY-----"),
}

// secretAssignment 的第一捕获组保留赋值键与分隔符，第二组是需要隐藏的值。
// secretAssignment 检测敏感赋值，第一捕获组保留字段名与分隔符，第二组为待隐藏值。
var secretAssignment = regexp.MustCompile("(?i)([a-z0-9_]*(?:api[_-]?key|secret|password|token)[a-z0-9_]*[ \\t]*[:=][ \\t]*[\\\"']?)([^\\s\\\"',;]{6,})")

// runSecrets 执行本地密钥检测；匹配密钥和敏感赋值时拦截或用字面量替换脱敏。
// 参数：p：action=block 或默认 redact、replacement；texts：有序文本，入口统一限制输入及输出总量。
// 返回：动作、命中说明及文本副本；未命中保持原文，替换不展开 $ 捕获组。
// 调用：runRuleDetailed 的 hide-secrets 分支。
// 测试：TestSecretsAndCredentialMasking。
func runSecrets(p map[string]any, texts []string) (string, string, []string) {
	out := append([]string{}, texts...)
	action := "allow"
	replacement := str(p["replacement"])
	if replacement == "" {
		replacement = "[REDACTED]"
	}
	for i, text := range texts {
		matched := false
		for _, pattern := range secretPatterns {
			if pattern.MatchString(text) {
				matched = true
				text = pattern.ReplaceAllStringFunc(text, func(string) string { return replacement })
			}
		}
		if secretAssignment.MatchString(text) {
			matched = true
			text = secretAssignment.ReplaceAllStringFunc(text, func(value string) string {
				indexes := secretAssignment.FindStringSubmatchIndex(value)
				return value[:indexes[4]] + replacement
			})
		}
		if matched {
			if str(p["action"]) == "block" {
				return "block", "Secret detection found credentials", out
			}
			action = "redact"
			out[i] = text
		}
	}
	return action, "", out
}
