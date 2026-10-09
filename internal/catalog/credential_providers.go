package catalog

import (
	"encoding/json"
	"sort"
)

// CredentialProviders 合并 LiteLLM 认证字段快照与本地供应商，供公开字段接口返回。
// 参数：无；返回：按显示名排序的独立字段目录，不修改价格目录或共享切片。
// 调用：PublicBody；相同 provider 标识优先使用专属认证定义，未知本地提供商保留原字段。
// 目录描述配置方式，不承诺执行适配器或外部账户可用；无字段类型需要独立授权流程。
func CredentialProviders() []map[string]any {
	var rows []map[string]any
	if json.Unmarshal(providerFieldsJSON, &rows) != nil {
		rows = []map[string]any{}
	}
	seen := map[string]bool{}
	seenSlugs := map[string]bool{}
	defaults := map[string]string{}
	local := append([]map[string]any(nil), Providers()...)
	for _, item := range extraProviderMaps() {
		if row, ok := item.(map[string]any); ok {
			local = append(local, row)
		}
	}
	for _, row := range local {
		defaults[stringField(row, "litellm_provider")] = stringField(row, "default_api_base")
	}
	for _, row := range rows {
		seen[stringField(row, "provider")] = true
		slug := stringField(row, "litellm_provider")
		seenSlugs[slug] = true
		// LiteLLM 的许多默认地址位于执行适配器；XHub 显式携带地址，避免保存后丢失默认值。
		if fields, ok := row["credential_fields"].([]any); ok {
			for _, item := range fields {
				field, _ := item.(map[string]any)
				if field["key"] == "api_base" && field["default_value"] == nil && field["required"] != true && defaults[slug] != "" {
					field["default_value"] = defaults[slug]
				}
			}
			if slug == "deepseek" {
				row["provider_display_name"] = "DeepSeek"
				row["default_model_placeholder"] = "deepseek/deepseek-chat"
				row["credential_fields"] = append([]any{map[string]any{
					"key": "api_base", "label": "API Base", "field_type": "text", "required": false,
					"default_value": "https://api.deepseek.com", "placeholder": "https://api.deepseek.com",
				}}, fields...)
			}
		}
	}
	for _, row := range local {
		id := stringField(row, "provider")
		if id != "" && !seen[id] && !seenSlugs[stringField(row, "litellm_provider")] {
			// 本地发行方名称可能与认证目录大小写不同；按协议去重，避免重复 DeepSeek。
			// 深拷贝保证接口调用方修改字段时不会污染共享价格数据。
			raw, _ := json.Marshal(row)
			var copy map[string]any
			if json.Unmarshal(raw, &copy) != nil {
				continue
			}
			rows = append(rows, copy)
			seen[id] = true
			seenSlugs[stringField(row, "litellm_provider")] = true
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		return stringField(rows[i], "provider_display_name") < stringField(rows[j], "provider_display_name")
	})
	return rows
}
