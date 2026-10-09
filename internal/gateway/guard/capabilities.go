package guard

// credentialMask 是管理接口统一占位符；它不是可发送到外部服务的凭据。
const credentialMask = "********"

// secretFields 列出需要遮蔽和保留更新语义的外部服务凭据字段。
var secretFields = map[string]bool{"api_key": true, "aws_access_key_id": true, "aws_secret_access_key": true, "aws_session_token": true}

// publicRule 复制管理响应并遮蔽凭据字段，避免修改真实执行配置或把密钥发给浏览器。
// 参数：row：包含 litellm_params 的规则对象。
// 返回：浅层规则副本及独立参数副本；非空密钥替换为 ********。
// 调用：Manage 的列表、详情及保存响应。
// 测试：TestSecretsAndCredentialMasking、TestExternalCredentialUpdatePreservesSecret。
func publicRule(row map[string]any) map[string]any {
	out := map[string]any{}
	for key, value := range row {
		out[key] = value
	}
	if params, ok := row["litellm_params"].(map[string]any); ok {
		copy := map[string]any{}
		for key, value := range params {
			if secretFields[key] && str(value) != "" {
				copy[key] = credentialMask
			} else {
				copy[key] = value
			}
		}
		out["litellm_params"] = copy
	}
	return out
}

// publicRules 为列表中的每条规则生成遮蔽凭据的管理视图。
// 参数：rows：内部规则列表，可以为空。
// 返回：非 nil 的规则数组；保留原列表顺序。
// 调用：Manage 的 list 接口。
// 测试：external_test.go、engine_test.go。
func publicRules(rows []map[string]any) []map[string]any {
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		out = append(out, publicRule(row))
	}
	return out
}

// providerDirectory 生成已实现服务的显示名到协议标识映射。
// 参数：无；读取外部适配器目录。
// 返回：包含 Local、XGo 和已接入服务的 map；未移植伙伴不在执行能力列表中。
// 调用：Manage 的 add_guardrail_settings 接口。
// 测试：engine_test.go 的管理接口测试。
func providerDirectory() map[string]string {
	out := map[string]string{"Local": "local", "XGo": "custom_code"}
	for key, name := range externalProviders {
		out[name] = key
	}
	return out
}
