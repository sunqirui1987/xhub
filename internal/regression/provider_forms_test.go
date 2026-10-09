package regression

import (
	"encoding/json"
	"net/http"
	"testing"
)

// TestProviderFormsContract 验证真实公开目录和供应商凭据 CRUD 契约。
// 参数 t：回归上下文；返回无。前置 PostgreSQL 和网关，断言专属字段、认证、错误、元数据持久化与脱敏。
// 凭据显式删除，harness 自动销毁隔离 schema，不调用外部付费供应商。
func TestProviderFormsContract(t *testing.T) {
	h := newHarness(t)
	admin := h.adminSession()
	response := h.ok(http.MethodGet, "/public/providers/fields", "", nil)
	var rows []map[string]any
	if err := json.Unmarshal(response.body, &rows); err != nil {
		t.Fatalf("公开字段必须返回数组：%v", err)
	}
	byID := map[string]map[string]any{}
	for _, row := range rows {
		id := stringField(row, "provider")
		if byID[id] != nil {
			t.Fatalf("重复标识 %s", id)
		}
		byID[id] = row
	}
	for id, key := range map[string]string{"Deepseek": "api_base", "Azure": "azure_ad_token", "Vertex_AI": "vertex_credentials", "Bedrock": "aws_region_name"} {
		found := false
		for _, value := range byID[id]["credential_fields"].([]any) {
			if value.(map[string]any)["key"] == key {
				found = true
			}
		}
		if !found {
			t.Fatalf("%s 未提供专属字段 %s", id, key)
		}
	}
	const name = "provider-form-deepseek"
	payload := map[string]any{"credential_name": name, "credential_info": map[string]any{"custom_llm_provider": "deepseek", "provider_id": "Deepseek"}, "credential_values": map[string]any{"api_base": "https://api.deepseek.com", "api_key": "form-secret-123456"}}
	denied := h.do(http.MethodPost, "/credentials", "", payload)
	if denied.status != 401 {
		t.Fatalf("匿名创建凭据未拒绝：%s", denied.describe())
	}
	h.ok(http.MethodPost, "/credentials", admin, payload)
	stored, err := h.gw.RecordStore().GetKV("credentials", name)
	if err != nil || stored["credential_info"].(map[string]any)["provider_id"] != "Deepseek" {
		t.Fatalf("表单标识未持久化：%v %v", stored, err)
	}
	listed := h.ok(http.MethodGet, "/credentials", admin, nil).json()
	found := false
	for _, row := range listField(listed, "credentials") {
		if row["credential_name"] == name {
			found = true
			if row["credential_values"].(map[string]any)["api_key"] == "form-secret-123456" {
				t.Fatal("凭据列表泄露密钥")
			}
		}
	}
	if !found {
		t.Fatal("新建凭据未进入列表")
	}
	h.ok(http.MethodPatch, "/credentials/"+name, admin, map[string]any{"credential_values": map[string]any{"api_base": "http://local.example/v1"}})
	stored, err = h.gw.RecordStore().GetKV("credentials", name)
	if err != nil {
		t.Fatal(err)
	}
	values := stored["credential_values"].(map[string]any)
	if values["api_base"] != "http://local.example/v1" || values["api_key"] != "form-secret-123456" {
		t.Fatal("更新地址应保留已保存密钥")
	}
	missing := h.do(http.MethodPatch, "/credentials/"+name, "invalid-session", map[string]any{"credential_values": map[string]any{}})
	if missing.status != 401 {
		t.Fatalf("无效会话编辑错误契约：%s", missing.describe())
	}
	h.ok(http.MethodDelete, "/credentials/"+name, admin, nil)
	for _, row := range listField(h.ok(http.MethodGet, "/credentials", admin, nil).json(), "credentials") {
		if row["credential_name"] == name {
			t.Fatal("已删除凭据仍在列表")
		}
	}
}
