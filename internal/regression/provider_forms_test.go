package regression

import (
	"encoding/json"
	"net/http"
	"testing"
)

// TestProviderFormsContract 验证真实公开目录和供应商凭据 CRUD 契约。
// 参数 t：回归上下文；返回无。前置 PostgreSQL 和网关，断言专属字段、Custom 入口、协议目录、认证、错误、元数据持久化与脱敏。
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
	for _, id := range []string{"Qiniu", "QINIU", "Fenno", "FENNO", "FennoAI", "FENNOAI"} {
		if byID[id] != nil {
			t.Fatalf("公开凭据目录不应预置 %s", id)
		}
	}
	for _, id := range []string{"CUSTOM", "CUSTOM_OPENAI"} {
		fields := map[string]bool{}
		for _, value := range byID[id]["credential_fields"].([]any) {
			fields[stringField(value.(map[string]any), "key")] = true
		}
		if !fields["api_base"] || !fields["api_key"] {
			t.Fatalf("%s 缺少自定义地址或密钥字段：%v", id, fields)
		}
	}
	endpoints := h.ok(http.MethodGet, "/public/endpoints", "", nil).json()
	transports := listField(endpoints, "transports")
	foundTransport := map[string]bool{}
	for _, transport := range transports {
		id := stringField(transport, "id")
		if id == "qiniu_contents_generation" || id == "qiniu_fal_kling" {
			foundTransport[id] = true
		}
	}
	if !foundTransport["qiniu_contents_generation"] || !foundTransport["qiniu_fal_kling"] {
		t.Fatalf("Custom 可选的七牛传输不完整：%v", foundTransport)
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

// TestPriceReloadRequiresConfiguredSource 验证显式源缺失时管理接口错误和价格保留。
// 参数 t：回归上下文；返回：无。前置隔离数据库，验证502、400与无计划副作用，harness 清理 schema。
func TestPriceReloadRequiresConfiguredSource(t *testing.T) {
	t.Setenv("XHUB_PRICE_FEED_URL", "")
	h := newHarness(t)
	admin := h.adminSession()
	before := h.ok(http.MethodGet, "/price/catalog", admin, nil).json()
	r := h.do(http.MethodPost, "/reload/model_cost_map", admin, nil)
	if r.status != 502 {
		t.Fatalf("无源刷新未失败: %s", r.describe())
	}
	r = h.do(http.MethodPost, "/schedule/model_cost_map_reload?hours=24", admin, nil)
	if r.status != 400 {
		t.Fatalf("无源仍能定时刷新: %s", r.describe())
	}
	after := h.ok(http.MethodGet, "/price/catalog", admin, nil).json()
	if before["count"] != after["count"] || before["source"] != after["source"] {
		t.Fatal("失败刷新改变价格")
	}
	if h.ok(http.MethodGet, "/schedule/model_cost_map_reload/status", admin, nil).json()["scheduled"] != false {
		t.Fatal("失败创建污染定时计划")
	}
}
