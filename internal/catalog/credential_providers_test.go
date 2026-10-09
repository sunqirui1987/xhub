package catalog

import (
	"encoding/json"
	"reflect"
	"testing"
)

// TestCredentialProviders 验证专属认证目录、默认地址和本地供应商合并。
// 参数 t：测试上下文；返回无。前置为嵌入目录，断言关键字段、唯一标识；只读无需清理。
func TestCredentialProviders(t *testing.T) {
	modelCostMu.Lock()
	old := append([]ProviderRow(nil), extraProviders...)
	extraProviders = append(extraProviders, ProviderRow{Name: "UnitLocal", Slug: "unit_local", Display: "Unit Local", Fields: []ProviderField{{Key: "api_base", Label: "API Base", Default: "http://local.test"}}})
	modelCostMu.Unlock()
	t.Cleanup(func() { modelCostMu.Lock(); extraProviders = old; modelCostMu.Unlock() })
	rows := CredentialProviders()
	if len(rows) < 118 {
		t.Fatalf("认证目录不足：%d", len(rows))
	}
	seen := map[string]bool{}
	byID := map[string]map[string]any{}
	deepseek := 0
	for _, row := range rows {
		id := stringField(row, "provider")
		if id == "" || seen[id] {
			t.Fatalf("供应商标识为空或重复：%q", id)
		}
		seen[id] = true
		byID[id] = row
		if row["litellm_provider"] == "deepseek" {
			deepseek++
		}
	}
	if deepseek != 1 {
		t.Fatalf("DeepSeek 重复：%d", deepseek)
	}
	for id, keys := range map[string][]string{"OpenAI": {"api_base", "api_key", "organization"}, "Deepseek": {"api_base", "api_key"}, "Azure": {"api_base", "api_version", "azure_ad_token", "tenant_id"}, "Vertex_AI": {"vertex_project", "vertex_location", "vertex_credentials"}, "Bedrock": {"aws_access_key_id", "aws_region_name", "aws_role_name"}, "Ollama": {"api_base"}} {
		fields := map[string]map[string]any{}
		for _, v := range byID[id]["credential_fields"].([]any) {
			f := v.(map[string]any)
			fields[stringField(f, "key")] = f
		}
		for _, key := range keys {
			if fields[key] == nil {
				t.Fatalf("%s 缺少字段 %s", id, key)
			}
		}
		if id == "Deepseek" && fields["api_base"]["default_value"] != "https://api.deepseek.com" {
			t.Fatal("DeepSeek 默认地址错误")
		}
	}
	if len(byID["CHATGPT"]["credential_fields"].([]any)) != 0 {
		t.Fatal("独立授权类型不应伪造 API Key")
	}
	found := false
	for _, r := range rows {
		if r["litellm_provider"] == "unit_local" {
			found = true
		}
	}
	if !found {
		t.Fatal("本地注册供应商丢失")
	}
}

// TestCredentialProvidersIsolation 验证响应字段变更不污染快照或市场目录。
// 参数 t：测试上下文；返回无。修改独立响应后对比再次读取与价格目录，无全局写入无需清理。
func TestCredentialProvidersIsolation(t *testing.T) {
	before, _ := json.Marshal(Providers())
	rows := CredentialProviders()
	original := CredentialProviders()
	for _, row := range rows {
		row["provider_display_name"] = "changed"
		for _, v := range row["credential_fields"].([]any) {
			v.(map[string]any)["label"] = "changed"
		}
	}
	if !reflect.DeepEqual(original, CredentialProviders()) {
		t.Fatal("响应修改污染后续认证目录")
	}
	after, _ := json.Marshal(Providers())
	if string(before) != string(after) {
		t.Fatal("响应修改污染价格目录")
	}
}
